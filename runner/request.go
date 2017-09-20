package runner

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

func (c byteCountingWriter) Write(p []byte) (n int, err error) {
	nBytes := len(p)
	c.count += int64(nBytes)
	return nBytes, nil
}

// The request type contains the information needed to run an HTTP method
type Request struct {
	Url     string
	Method  string
	Headers http.Header
}

func NewRequest(url, method string, headers http.Header) Request {
	return Request{url, method, headers}
}

func NewRequestFromStringMap(url, method string, headers map[string]string) Request {
	httpHeaders := make(http.Header)
	for k, v := range headers {
		if httpHeaders.Get(k) == "" {
			httpHeaders.Add(k, v)
		} else {
			panic(fmt.Errorf("Header key %s already exists", k))
		}
	}
	return Request{url, method, httpHeaders}
}

func (r Request) String() string {
	return fmt.Sprintf("%s %s %+v", strings.ToUpper(r.Method), r.Url, r.Headers)
}

type Runner struct {
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

// Create and return a new Runner with the Transport and Clients already set up
// for use
func NewRunner() Runner {
	transport := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	}
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: checkRedirect,
	}
	return Runner{transport, client}
}

// TODO: Create accessor methods for the client and transport

func (r Runner) RunWithDetails(request Request, body Body, chunkSize int, outputFile string) error {

	httpRequest, err := http.NewRequest(request.Method, request.Url, body)
	if err != nil {
		panic(err)
	}

	httpRequest.Header = request.Headers

	resp, err := r.client.Do(httpRequest)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	var expectedSize int64
	var expectedTransferSize int64

	// Figure out what content size we're expecting
	if cSize := resp.Header.Get("x-amz-meta-content-length"); cSize == "" {
		panic(fmt.Errorf("Expected Header X-Amz-Meta-Content-Length to have a value"))
	} else {
		i, err := strconv.ParseInt(cSize, 10, 64)
		if err != nil {
			panic(err)
		}
		expectedSize = i
	}

	// Figure out which transfer size we're expecting
	if tSize := resp.Header.Get("x-amz-meta-transfer-length"); tSize == "" {
		expectedTransferSize = expectedSize
	} else {
		i, err := strconv.ParseInt(tSize, 10, 64)
		if err != nil {
			panic(err)
		}
		expectedTransferSize = i
	}

	// Let's get the text out that we need
	expectedSha256 := resp.Header.Get("x-amz-meta-content-sha256")
	expectedTransferSha256 := resp.Header.Get("x-amz-transfer-sha256")

	if expectedSha256 == "" {
		panic(fmt.Errorf("Expected a content-sha256"))
	}

	if expectedTransferSha256 == "" {
		expectedTransferSha256 = expectedSha256
	}

	// We're going to need to have the Sha256 calculated of both the bytes
	// transfered and the bytes decompressed if there's gzip compression
	// happening
	transferHash := sha256.New()
	contentHash := sha256.New()

	// Set up the input.  In all cases, we want to tee the response body directly
	// to the transferHash so that we're able to calculate the hash of the raw
	// response body without any intermediate transformations (e.g. gzip)
	input := io.TeeReader(resp.Body, transferHash)

	// We'll create a counter to count the number of bytes written to it
	transferCounter := byteCountingWriter{0}
	input = io.TeeReader(input, transferCounter)

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
			panic(fmt.Errorf("Identity encoding requires content and transfer sha256 to be equal"))
		}
	case "gzip":
		zr, err := gzip.NewReader(input)
		if err != nil {
			panic(err)
		}
		input = zr
	default:
		panic(fmt.Errorf("Unexpected content-encoding: %s", enc))
	}

	// The output io.Writer is going to be set up as a chain of io.Writers where
	// the uncompressed data should be written to.  This is the contentHash and
	// optionally the output file
	var output io.Writer

	// We want to know how many bytes are in the content
	contentCounter := byteCountingWriter{0}

	if &outputFile == nil {
		output = contentHash
	} else {
		of, err := os.Create(outputFile)
		if err != nil {
			panic(err)
		}
		defer of.Close()
		output = io.MultiWriter(of, contentHash)
	}

	// Hook up the content counter
	output = io.MultiWriter(output, contentCounter)

	// Read buffer
	buf := make([]byte, chunkSize)

	for {
		nBytes, err := input.Read(buf)
		if nBytes == 0 {
			break
		}
		if err != nil {
			panic(err)
		}
		output.Write(buf[:nBytes])
	}

	transferBytes := transferCounter.count
	contentBytes := contentCounter.count
	//contentHash =
	//transferHash

	if expectedSize != contentBytes {
		panic(fmt.Errorf("Expected transfer of %d bytes, received %d", expectedSize, contentBytes))
	}

	if expectedTransferSize != transferBytes {
		panic(fmt.Errorf("Expected %d bytes of content but have %d", expectedTransferSize, transferBytes))
	}

	return nil
}
