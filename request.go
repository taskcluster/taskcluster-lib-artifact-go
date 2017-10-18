package artifact

import (
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ErrCorrupt is an error message which is specifically related to an
// artifact being found to be corrupt
var ErrCorrupt = errors.New("corrupt resource")

type byteCountingWriter struct {
	count int64
}

func (c *byteCountingWriter) Write(p []byte) (n int, err error) {
	nBytes := len(p)
	c.count += int64(nBytes)
	return nBytes, nil
}

// The request type contains the information needed to run an HTTP method
type request struct {
	URL     string
	Method  string
	Headers *http.Header
}

func newRequest(url, method string, headers *http.Header) request {
	return request{url, method, headers}
}

func newRequestFromStringMap(url, method string, headers map[string]string) (request, error) {
	httpHeaders := make(http.Header)
	for k, v := range headers {
		if httpHeaders.Get(k) == "" {
			httpHeaders.Set(k, v)
		} else {
			return request{}, fmt.Errorf("header key %s already exists", k)
		}
	}
	return request{url, method, &httpHeaders}, nil
}

func (r request) String() string {
	return fmt.Sprintf("%s %s %+v", strings.ToUpper(r.Method), r.URL, r.Headers)
}

type client struct {
	transport *http.Transport
	client    *http.Client
}

// TODO: We might want to do a couple things here instead of just disabling
// redirects altogether.  Since it's possible that S3 does redirect us, we
// might want to do a couple checks like for HTTPS, same origin, etc, and then
// follow the redirect, but for now let's ensure that the URLs that the Queue
// gives us aren't redirecting
func checkRedirect(req *http.Request, via []*http.Request) error {
	return http.ErrUseLastResponse
}

// Create a new client for running uploads and downloads
func newAgent() client {
	transport := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	}
	_client := &http.Client{
		Transport:     transport,
		CheckRedirect: checkRedirect,
	}
	return client{transport, _client}
}

// TODO: Add logging just before returning an error

// Run a request where x-amz-meta-{transfer,content}-{sha256,length} are
// checked against the request body.  If the outputFile option is passed in,
// create a file and write to that file the response body.  If the response has
// the Content-Encoding header and it's value is gzip, the response body will
// be written post-gzip decompression.  The response struct returned from this
// method will have a body that has had the .Close() method called.  It is
// intended for a caller of this method to be able to inspect the headers or
// other fields
func (c client) run(request request, body io.Reader, chunkSize int, outputFile string, verify bool) (*http.Response, error) {

	httpRequest, err := http.NewRequest(request.Method, request.URL, body)
	if err != nil {
		return nil, err
	}

	// If we have headers in the request, let's set them
	if request.Headers != nil {
		httpRequest.Header = *request.Headers
	}

	// Run the actual request
	resp, err := c.client.Do(httpRequest)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	logger.Printf("Received response from %s %s", request.Method, request.URL)

	if resp.StatusCode >= 300 {
		return resp, fmt.Errorf("only 200-series HTTP response codes are supported")
	}

	// We're going to need to have the Sha256 calculated of both the bytes
	// transfered and the decoded bytes if there's a content-encoding to reverse
	transferHash := sha256.New()
	contentHash := sha256.New()

	// We're going to need to have the number of bytes received and size of the
	// content
	transferCounter := &byteCountingWriter{0}
	contentCounter := &byteCountingWriter{0}

	// This io.Reader is a reference to the response body, after setting up all
	// the required plumbing for doing transfer byte counting and hashing as well
	// as any possible content-decoding
	var input io.Reader
	input = io.TeeReader(resp.Body, io.MultiWriter(transferHash, transferCounter))

	// We want to handle content encoding.  In this case, we only accept the
	// header being unset (implies identity), 'indentity' or 'gzip'.  We do not
	// support having more than one content-encoding scheme.  This switch will
	// set up any changes to the readers needed (e.g. wrapping the reader with a
	// gzip reader) as well as making assertions specific to the content-encoding
	// in question
	switch enc := strings.TrimSpace(resp.Header.Get("content-encoding")); enc {
	case "":
		fallthrough
	case "identity":
		logger.Printf("Resource %s %s is identity encoded", request.Method, request.URL)
	case "gzip":
		zr, err := gzip.NewReader(input)
		if err != nil {
			return resp, err
		}
		input = zr
		logger.Printf("Resource %s %s is gzip encoded", request.Method, request.URL)
	default:
		return resp, fmt.Errorf("unexpected content-encoding: %s", enc)
	}

	// This io.Writer is a reference to the output stream.  This is at least the
	// plumbing required to calculate the content's hash and size.  Optionally
	// this will also write the output to a file
	var output io.Writer

	if outputFile == "" {
		output = io.MultiWriter(contentHash, contentCounter)
	} else {
		of, err := os.Create(outputFile)
		if err != nil {
			return resp, err
		}
		defer of.Close()
		output = io.MultiWriter(of, contentHash, contentCounter)
		logger.Printf("Writing %s %s to file '%s'", request.Method, request.URL, outputFile)
	}

	// Read buffer
	buf := make([]byte, chunkSize)

	_, err = io.CopyBuffer(output, input, buf)
	if err != nil {
		return resp, err
	}

	transferBytes := transferCounter.count
	contentBytes := contentCounter.count
	sContentHash := fmt.Sprintf("%x", contentHash.Sum(nil))
	sTransferHash := fmt.Sprintf("%x", transferHash.Sum(nil))

	// We don't want to do any verification for requests which are not being made
	// to download artifacts.  Example would be requests being run to upload an
	// artifact.
	if verify {

		// We want to find all the ways that the response is invalid and print a
		// message for each so that the user can avoid having to do too many
		// testing cycles to find all the flaws.  This variable will store
		// information on whether this response has been found to be invalid yet or
		// not.
		valid := true

		// We want to store the content and transfer sizes
		var expectedSize int64
		var expectedTransferSize int64
		var expectedSha256 string
		var expectedTransferSha256 string

		// Figure out what content size we're expecting
		if cSize := resp.Header.Get("x-amz-meta-content-length"); cSize == "" {
			logger.Printf("Expected header X-Amz-Meta-Content-Length to have a value")
			valid = false
		} else {
			i, err := strconv.ParseInt(cSize, 10, 64)
			if err != nil {
				return resp, err
			}
			expectedSize = i
		}

		// Figure out which transfer size we're expecting
		if tSize := resp.Header.Get("x-amz-meta-transfer-length"); tSize == "" {
			expectedTransferSize = expectedSize
		} else {
			i, err := strconv.ParseInt(tSize, 10, 64)
			if err != nil {
				return resp, err
			}
			expectedTransferSize = i
		}

		// Let's get the text out that we need
		expectedSha256 = resp.Header.Get("x-amz-meta-content-sha256")
		expectedTransferSha256 = resp.Header.Get("x-amz-meta-transfer-sha256")

		if expectedSha256 == "" {
			logger.Printf("Expected a X-Amz-Meta-Content-Sha256 to have a value")
			valid = false
		} else if len(expectedSha256) != 64 {
			logger.Printf("Expected X-Amz-Meta-Content-Sha256 to be 64 chars, not %d", len(expectedSha256))
			valid = false
		}

		if expectedTransferSha256 == "" {
			expectedTransferSha256 = expectedSha256
		}

		if expectedTransferSize != transferBytes {
			logger.Printf("Resource %s %s has incorrect transfer length.  Expected: %d received: %d",
				request.Method, request.URL, expectedTransferSize, transferBytes)
			valid = false
		}

		if expectedTransferSha256 != sTransferHash {
			logger.Printf("Resource %s %s has incorrect transfer sha256.  Expected: %s received: %s",
				request.Method, request.URL, expectedTransferSha256, sTransferHash)
			valid = false
		}

		if expectedSize != contentBytes {
			logger.Printf("Resource %s %s has incorrect content length.  Expected: %d received: %d",
				request.Method, request.URL, expectedSize, contentBytes)
			valid = false
		}

		if expectedSha256 != sContentHash {
			logger.Printf("Resource %s %s has incorrect content sha256.  Expected: %s received: %s",
				request.Method, request.URL, expectedSha256, sContentHash)
			valid = false
		}

		if !valid {
			logger.Printf("Response %s %s is INVALID. Received: transfer: %s %d bytes content: %s %d bytes",
				request.Method,
				request.URL,
				sTransferHash[:7],
				transferBytes,
				sContentHash[:7],
				contentBytes)
			return resp, ErrCorrupt
		}
	}
	if verify {
		logger.Printf("Response %s %s is valid. transfer: %s %d bytes content: %s %d bytes",
			request.Method,
			request.URL,
			sTransferHash[:7],
			transferBytes,
			sContentHash[:7],
			contentBytes)
	} else {
		logger.Printf("Response %s %s is complete. transfer: %s %d bytes content: %s %d bytes",
			request.Method,
			request.URL,
			sTransferHash[:7],
			transferBytes,
			sContentHash[:7],
			contentBytes)
	}
	return resp, nil
}
