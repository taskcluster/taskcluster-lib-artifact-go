package artifact

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	URL    string
	Method string
	Header *http.Header
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
	var buf bytes.Buffer
	r.Header.Write(&buf)
	return fmt.Sprintf("%s %s\nHEADERS:\n%s", strings.ToUpper(r.Method), r.URL, buf.String())
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

// CallSummary is a similar concept to that in the taskcluster-client-go
// library, though modified for use specifically here, where we're dealing with
// multiple mega-byte request and response bodies.  We'll only store string
// hashes instead of payload bodies
type CallSummary struct {
	Method         string
	URL            string
	StatusCode     int
	Status         string
	RequestLength  int64
	RequestSha256  string
	RequestHeader  *http.Header
	ResponseLength int64
	ResponseSha256 string
	ResponseHeader *http.Header
	Verified       bool
}

func (cs CallSummary) String() string {
	var reqHBuf bytes.Buffer
	cs.RequestHeader.Write(&reqHBuf)

	var resHBuf bytes.Buffer
	cs.ResponseHeader.Write(&resHBuf)

	var verified string
	if cs.Verified {
		verified = " (verified)"
	}

	return fmt.Sprintf("Call Summary:\n=============\n%s %s%s\nHTTP Status: %s\nRequest Size: %d bytes SHA256: %s\nRequest Headers:\n%s\nResponse Size: %d SHA256: %s\nResponse Headers:\n%s\n",
		strings.ToUpper(cs.Method),
		cs.URL,
		verified,
		cs.Status,
		cs.RequestLength,
		cs.RequestSha256,
		reqHBuf.String(),
		cs.ResponseLength,
		cs.ResponseSha256,
		reqHBuf.String(),
	)

}

// TODO: Add logging just before returning an error

// Run a request where x-amz-meta-{transfer,content}-{sha256,length} are
// checked against the request body.  If the outputFile option is passed in,
// create a file and write to that file the response body.  If the response has
// the Content-Encoding header and it's value is gzip, the response body will
// be written post-gzip decompression.  The response struct returned from this
// method will have a body that has had the .Close() method called.  It is
// intended for a caller of this method to be able to inspect the headers or
// other fields.  The boolean return value reflects whether an error is
// retryable.  Retryable errors are those which aren't fatal to the
// transaction.  Example of a retryable error is a 500 series error or local IO
// failure.  Example of a non-retryable error would be getting passed in a
// request which has an unparsable Content-Length header
func (c client) run(request request, body io.Reader, chunkSize int, outputWriter io.Writer, verify bool) (CallSummary, bool, error) {

	cs := CallSummary{}
	cs.URL = request.URL
	cs.Method = request.Method

	// For debugging, we want to log the SHA256 and Size of the request body that
	// we're going to write to
	reqBodyHash := sha256.New()
	reqBodyCounter := &byteCountingWriter{0}
	_body := io.TeeReader(body, io.MultiWriter(reqBodyHash, reqBodyCounter))

	httpRequest, err := http.NewRequest(request.Method, request.URL, _body)
	if err != nil {
		return cs, false, err
	}

	// If we have headers in the request, let's set them
	if request.Header != nil {
		httpRequest.Header = *request.Header
	}

	// Rather unintuitively, the Go HTTP library will ignore any content-length
	// set in the headers, instead using the http.Request.ContentLength to figure
	// out what to replace it with.... Except that for non-fixed length bodies,
	// it helpfully inserts a -1 value, which results in the header being unset.
	// What an annoying issue.  I did verify that the http.Header values are
	// correctly transformed into the canonical version when Set() is called on
	// them, so we should be safe to do the check this way.  If someone is going
	// to set values inside the Header directly, then that's their problem.  They
	// do expose a public canonicalization function
	if len(httpRequest.Header["Content-Length"]) > 0 {
		var contentLength int64
		if contentLength, err = strconv.ParseInt(request.Header.Get("Content-Length"), 10, 64); err != nil {
			return cs, false, err
		}

		httpRequest.ContentLength = contentLength
	}

	cs.RequestHeader = &httpRequest.Header
	// Run the actual request
	resp, err := c.client.Do(httpRequest)

	// Reassigning the Request headers in case the http library propogates its
	// internal modifications back.  That'd be nice!
	cs.RequestHeader = &httpRequest.Header
	cs.RequestLength = reqBodyCounter.count
	cs.RequestSha256 = fmt.Sprintf("%x", reqBodyHash.Sum(nil))

	cs.Status = resp.Status
	cs.StatusCode = resp.StatusCode
	cs.ResponseHeader = &resp.Header

	defer resp.Body.Close()
	if err != nil {
		return cs, false, err
	}

	// If the HTTP library reads in a different number of bytes than we're
	// expecting to have, we know that something is wrong.  This could also be a
	// panic() call, however, in this case we know we're accessing big disks
	// which are likely not even on the machine running this code.  Given that,
	// let's instead treat this as local I/O corruption and mark it as retryable
	if httpRequest.ContentLength != reqBodyCounter.count {
		return cs, true, fmt.Errorf("Read %d bytes when we should have read %d", reqBodyCounter.count, httpRequest.ContentLength)
	}

	// 500-series errors are always retryable
	if resp.StatusCode >= 500 {
		return cs, true, fmt.Errorf("HTTP Status: %s (retryable)", resp.Status)
	}

	// 300, 400 series errors are never retryable
	if resp.StatusCode >= 300 {
		return cs, false, fmt.Errorf("HTTP Status: %s (not-retryable)", resp.Status)
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
	case "gzip":
		zr, err := gzip.NewReader(input)
		if err != nil {
			return cs, false, err
		}
		input = zr
		logger.Printf("Resource %s %s is gzip encoded", request.Method, request.URL)
	default:
		return cs, false, fmt.Errorf("unexpected content-encoding: %s", enc)
	}

	// This io.Writer is a reference to the output stream.  This is at least the
	// plumbing required to calculate the content's hash and size.  Optionally
	// this will also write the output to a file
	var output io.Writer

	var __dbg bytes.Buffer

	if outputWriter == nil {
		output = io.MultiWriter(contentHash, contentCounter, &__dbg)
	} else {
		output = io.MultiWriter(outputWriter, contentHash, contentCounter, &__dbg)
	}

	// Read buffer
	buf := make([]byte, chunkSize)

	_, err = io.CopyBuffer(output, input, buf)
	if err != nil {
		// Retryable because this is likely a local issue only
		return cs, true, err
	}

	//logger.Printf("RESPONSE BODY: %s", __dbg.String())

	transferBytes := transferCounter.count
	contentBytes := contentCounter.count
	sContentHash := fmt.Sprintf("%x", contentHash.Sum(nil))
	sTransferHash := fmt.Sprintf("%x", transferHash.Sum(nil))

	cs.ResponseLength = transferBytes
	cs.ResponseSha256 = sTransferHash

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
				// Retryable because this is a sign of corrupted data.  Let's try once
				// more
				return cs, true, err
			}
			expectedSize = i
		}

		// Figure out which transfer size we're expecting
		if tSize := resp.Header.Get("x-amz-meta-transfer-length"); tSize == "" {
			expectedTransferSize = expectedSize
		} else {
			i, err := strconv.ParseInt(tSize, 10, 64)
			if err != nil {
				// Retryable because this is a sign of corrupted data.  Let's try once
				// more
				return cs, true, err
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
			// Invalid artifacts are retryable by default because they could be
			// corruption over the wire
			return cs, true, ErrCorrupt
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
	return cs, false, nil
}
