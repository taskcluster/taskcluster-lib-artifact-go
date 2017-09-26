package main

import (
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	//"bytes"
)

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
	Url     string
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
			return request{}, fmt.Errorf("Header key %s already exists", k)
		}
	}
	return request{url, method, &httpHeaders}, nil
}

func (r request) String() string {
	return fmt.Sprintf("%s %s %+v", strings.ToUpper(r.Method), r.Url, r.Headers)
}

type runner struct {
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

// Create and return a new runner with the Transport and Clients already set up
// for use
func newRunner() runner {
	transport := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	}
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: checkRedirect,
	}
	return runner{transport, client}
}

// TODO: Create accessor methods for the client and transport

func (r runner) run(request request, body io.Reader, chunkSize int, outputFile string) error {

	httpRequest, err := http.NewRequest(request.Method, request.Url, body)
	if err != nil {
		return err
	}

  // If we have headers in the request, let's set them
  if request.Headers != nil {
	  httpRequest.Header = *request.Headers
  }

  // Run the actual request
	resp, err := r.client.Do(httpRequest)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

  // We want to store the content and transfer sizes
	var expectedSize int64
	var expectedTransferSize int64

	// Figure out what content size we're expecting
	if cSize := resp.Header.Get("x-amz-meta-content-length"); cSize == "" {
		return fmt.Errorf("Expected Header X-Amz-Meta-Content-Length to have a value")
	} else {
		i, err := strconv.ParseInt(cSize, 10, 64)
		if err != nil {
			return err
		}
		expectedSize = i
	}


	// Figure out which transfer size we're expecting
	if tSize := resp.Header.Get("x-amz-meta-transfer-length"); tSize == "" {
		expectedTransferSize = expectedSize
	} else {
		i, err := strconv.ParseInt(tSize, 10, 64)
		if err != nil {
			return err
		}
		expectedTransferSize = i
	}

	// Let's get the text out that we need
	expectedSha256 := resp.Header.Get("x-amz-meta-content-sha256")
	expectedTransferSha256 := resp.Header.Get("x-amz-meta-transfer-sha256")

	if expectedSha256 == "" {
		return fmt.Errorf("Expected a X-Amz-Meta-Content-Sha256 to have a value")
	} else if len(expectedSha256) != 64 {
		return fmt.Errorf("Expected X-Amz-Meta-Content-Sha256 to be 64 chars, not %d", len(expectedSha256))
  }

	if expectedTransferSha256 == "" {
		expectedTransferSha256 = expectedSha256
	}

	// We're going to need to have the Sha256 calculated of both the bytes
	// transfered and the bytes decompressed if there's gzip compression
	// happening
	transferHash := sha256.New()
	contentHash := sha256.New()

	// We'll create a counter to count the number of bytes written to it
	transferCounter := &byteCountingWriter{0}

	// Set up the input.  In all cases, we want to tee the response body directly
	// to the transferHash so that we're able to calculate the hash of the raw
	// response body without any intermediate transformations (e.g. gzip)
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
		if expectedSha256 != expectedTransferSha256 {
			return fmt.Errorf("Identity encoding requires content and transfer sha256 to be equal")
		}
    if expectedTransferSize != expectedSize {
			return fmt.Errorf("Identity encoding requires content and transfer length to be equal")
    }
	case "gzip":
		zr, err := gzip.NewReader(input)
		if err != nil {
			return err
		}
		input = zr
	default:
		return fmt.Errorf("Unexpected content-encoding: %s", enc)
	}

	// The output io.Writer is going to be set up as a chain of io.Writers where
	// the uncompressed data should be written to.  This is the contentHash and
	// optionally the output file
	var output io.Writer

	// We want to know how many bytes are in the content
	contentCounter := &byteCountingWriter{0}

	if outputFile == "" {
		output = io.MultiWriter(contentHash, contentCounter)
	} else {
		of, err := os.Create(outputFile)
		if err != nil {
			return err
		}
		defer of.Close()
		output = io.MultiWriter(of, contentHash, contentCounter)
	}

	// Read buffer
	buf := make([]byte, chunkSize)

  _, err = io.CopyBuffer(output, input, buf)
  if err != nil {
    return err
  }

	transferBytes := transferCounter.count
	contentBytes := contentCounter.count
  sContentHash := fmt.Sprintf("%x", contentHash.Sum(nil))
  sTransferHash := fmt.Sprintf("%x", transferHash.Sum(nil))

	if expectedTransferSize != transferBytes {
		return fmt.Errorf("Expected transfer of %d bytes, received %d", expectedTransferSize, transferBytes)
	}

	if expectedTransferSha256 != sTransferHash {
		return fmt.Errorf("Expected transfer-sha256 %s but have %s", expectedTransferSha256, sTransferHash)
	}

	if expectedSize != contentBytes {
		return fmt.Errorf("Expected %d bytes of content but have %d", expectedSize, contentBytes)
	}

	if expectedSha256 != sContentHash {
		return fmt.Errorf("Expected content-sha256 %s but have %s", expectedSha256, sContentHash)
	}
	return nil
}
