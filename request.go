package artifact

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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
		if ev := httpHeaders.Get(k); ev == "" { // existing value
			httpHeaders.Set(k, v)
		} else {
			return request{}, newErrorf(nil, "header key %s already exists with value %s", k, ev)
		}
	}
	return request{url, method, &httpHeaders}, nil
}

func (r request) String() string {
	var buf bytes.Buffer
	err := r.Header.Write(&buf)
	if err != nil {
		return fmt.Sprintf("Could not read http request headers - error: %v", err)
	}
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

// callSummary is a similar concept to that in the taskcluster-client-go
// library, though modified for use specifically here, where we're dealing with
// multiple mega-byte request and response bodies.  We'll only store string
// hashes instead of payload bodies.  We return this instead of a raw response
// because we need to add extra fields (e.g. hashes and whether it was
// verified).  In this library, the callSummary is expected to be useful for
// programatic acccess to the resulting requests
type callSummary struct {
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

func (cs callSummary) String() string {
	var reqHBuf bytes.Buffer
	if cs.RequestHeader != nil {
		err := cs.RequestHeader.Write(&reqHBuf)
		if err != nil {
			// error not possible
			_, _ = reqHBuf.Write([]byte(fmt.Sprintf("Could not read HTTP request headers - error: %v", err)))
		}
	} else {
		// error not possible
		_, _ = reqHBuf.Write([]byte("No Request Headers"))
	}

	var resHBuf bytes.Buffer
	if cs.ResponseHeader != nil {
		err := cs.ResponseHeader.Write(&resHBuf)
		if err != nil {
			// error not possible
			_, _ = resHBuf.Write([]byte(fmt.Sprintf("Could not read HTTP response headers - error: %v", err)))
		}
	} else {
		// error not possible
		_, _ = resHBuf.Write([]byte("No Response Headers"))
	}

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
		resHBuf.String(),
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
func (c client) run(request request, inputReader io.Reader, chunkSize int, outputWriter io.Writer, verify bool) (cs callSummary, retryable bool, err error) {
	cs.URL = request.URL
	cs.Method = request.Method

	// For debugging, we want to log the SHA256 and Size of the request body that
	// we're going to write to
	reqBodyHash := sha256.New()
	reqBodyCounter := &byteCountingWriter{0}

	var body io.Reader

	if inputReader != nil {
		body = io.TeeReader(inputReader, io.MultiWriter(reqBodyHash, reqBodyCounter))
	} else {
		body = nil
		// We need to write an empty byte slice to the Hash in order to get the
		// internal data structures to be able to run the Sum() method and get
		// useful output
		var emptyslice []byte
		// error not possible
		_, _ = reqBodyHash.Write(emptyslice)
	}

	var httpRequest *http.Request
	httpRequest, err = http.NewRequest(request.Method, request.URL, body)
	if err != nil {
		return cs, false, newErrorf(err, "making %s request to %s", request.Method, request.URL)
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
	var contentLength int64
	hadCL := false
	if len(httpRequest.Header["Content-Length"]) > 0 {
		if contentLength, err = strconv.ParseInt(request.Header.Get("Content-Length"), 10, 64); err != nil {
			return cs, false, newErrorf(err, "parsing content-length for %s to %s", request.Method, request.URL)
		}

		httpRequest.ContentLength = contentLength
		hadCL = true
	}

	cs.RequestHeader = &httpRequest.Header
	cs.RequestLength = reqBodyCounter.count
	cs.RequestSha256 = hex.EncodeToString(reqBodyHash.Sum(nil))
	// Run the actual request
	var resp *http.Response
	resp, err = c.client.Do(httpRequest)
	if err != nil {
		return cs, false, newErrorf(err, "running %s request to %s", request.Method, request.URL)
	}

	// Reassigning the Request headers in case the http library propogates its
	// internal modifications back.  That'd be nice!
	cs.RequestHeader = &httpRequest.Header

	cs.Status = resp.Status
	cs.StatusCode = resp.StatusCode
	cs.ResponseHeader = &resp.Header

	// if we have an error closing the body, we should return the error, but only
	// if no other error has already been set
	defer func() {
		closeErr := resp.Body.Close()
		if closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	// If the HTTP library reads in a different number of bytes than we're
	// expecting to have, we know that something is wrong.  This could also be a
	// panic() call, however, in this case we know we're accessing big disks
	// which are likely not even on the machine running this code.  Given that,
	// let's instead treat this as local I/O corruption and mark it as retryable
	if hadCL && httpRequest.ContentLength != reqBodyCounter.count {
		return cs, true, newErrorf(nil, "read %d bytes from response of %s to %s when we should have read %d",
			reqBodyCounter.count, request.Method, request.URL, contentLength)
	}

	if resp.StatusCode >= 500 {
		var errBody []byte
		if errBody, err = ioutil.ReadAll(resp.Body); err == nil {
			logger.Printf("Retryable Error %s\nBody:\n%s", cs, errBody)
		}
		return cs, true, newErrorf(err, "received %s (retryable)", resp.Status)
	}

	// 400-series errors are never retryable
	if resp.StatusCode >= 400 {
		var errBody []byte
		if errBody, err = ioutil.ReadAll(resp.Body); err == nil {
			logger.Printf("Non-Retryable Error %s\nBody:\n%s", cs, errBody)
		}
		return cs, false, newErrorf(err, "received %s (non-retryable)", resp.Status)
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
	input := io.TeeReader(resp.Body, io.MultiWriter(transferHash, transferCounter))

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
		var zr *gzip.Reader
		zr, err = gzip.NewReader(input)
		if err != nil {
			return cs, false, newErrorf(err, "creating gzip reader for %s to %s", request.Method, request.URL)
		}
		input = zr
		logger.Printf("Resource %s %s is gzip encoded", request.Method, request.URL)
	default:
		return cs, false, newErrorf(nil, "unexpected content-encoding %s for %s to %s", enc, request.Method, request.URL)
	}

	// This io.Writer is a reference to the output stream.  This is at least the
	// plumbing required to calculate the content's hash and size.  Optionally
	// this will also write the output to a file
	var output io.Writer

	if outputWriter == nil {
		output = io.MultiWriter(contentHash, contentCounter)
	} else {
		output = io.MultiWriter(outputWriter, contentHash, contentCounter)
	}

	// Read buffer
	buf := make([]byte, chunkSize)

	_, err = io.CopyBuffer(output, input, buf)
	if err != nil {
		// Retryable because this is likely a local issue only
		return cs, true, newErrorf(err, "writing request %s to %s to output %s", request.Method, request.URL, findName(outputWriter))
	}

	transferBytes := transferCounter.count
	contentBytes := contentCounter.count
	sContentHash := hex.EncodeToString(contentHash.Sum(nil))
	sTransferHash := hex.EncodeToString(transferHash.Sum(nil))

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
			var i int64
			i, err = strconv.ParseInt(cSize, 10, 64)
			if err != nil {
				// Retryable because this is a sign of corrupted data.  Let's try once
				// more
				return cs, true, newErrorf(err, "parsing %s to %s X-Amz-Meta-Content-Length header value %s to int", request.Method, request.URL, cSize)
			}
			expectedSize = i
		}

		// Figure out which transfer size we're expecting
		if tSize := resp.Header.Get("x-amz-meta-transfer-length"); tSize == "" {
			expectedTransferSize = expectedSize
		} else {
			var i int64
			i, err = strconv.ParseInt(tSize, 10, 64)
			if err != nil {
				// Retryable because this is a sign of corrupted data.  Let's try once
				// more
				return cs, true, newErrorf(err, "parsing %s to %s X-Amz-Meta-Transfer-Length header value %s to int", request.Method, request.URL, tSize)
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
