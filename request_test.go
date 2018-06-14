package artifact

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
)

const emptySha256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// Set up a test server.  It is the responsibility of the caller
// to run the .Close() method on the returned server
func createServer(sc int, cl, ch, tl, th, ce string, body []byte) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-amz-meta-content-length", cl)
		w.Header().Set("x-amz-meta-content-sha256", ch)
		if tl != "" {
			w.Header().Set("x-amz-meta-transfer-length", tl)
		}
		if th != "" {
			w.Header().Set("x-amz-meta-transfer-sha256", th)
		}
		if ce != "" {
			w.Header().Set("content-encoding", ce)
		}
		w.WriteHeader(sc)

		if len(body) != 0 {
			w.Write(body)
		}
	}))

	return ts
}

// Name is short for HashBuffer.
func hb(a []byte) string {
	hash := sha256.New()
	hash.Write(a)
	return hex.EncodeToString(hash.Sum(nil))
}

// Name is short for StringLength
func sl(a []byte) string {
	return strconv.Itoa(len(a))
}

func TestRequestRunning(t *testing.T) {
	SetLogOutput(newUnitTestLogWriter(t))

	client := newAgent()

	if err := os.MkdirAll("testdata", os.FileMode(0755)); err != nil {
		t.Fatal(err)
	}

	filename := "testdata/request.dat"

	// We want to do a little bit of setup before running the tests
	if fi, err := os.Stat(filename); os.IsNotExist(err) || fi.Size() != 10*1024*1024 {
		t.Log("input data did not exist or was wrong size, recreating")
		of, err := os.Create(filename)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 10*1024; i++ {
			c := 1024
			b := make([]byte, c)
			_, err := rand.Read(b)
			if err != nil {
				t.Fatal(err)
			}
			of.Write(b)
		}
		of.Close()
	}

	_in, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	body, err := ioutil.ReadAll(_in)
	if err != nil {
		t.Fatal(err)
	}

	var _gzipBody bytes.Buffer
	zw := gzip.NewWriter(&_gzipBody)
	_, err = zw.Write(body)
	if err != nil {
		t.Fatal(err)
	}
	zw.Close()
	gzipBody := _gzipBody.Bytes()

	t.Run("writes request body correctly", func(t *testing.T) {

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			w.Header().Set("Content-Length", "0")
			w.WriteHeader(200)
			var buf bytes.Buffer
			_, err := io.Copy(&buf, r.Body)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(buf.Bytes(), body) {
				t.Fatalf("Request body not as expected. %d bytes vs expected %d", buf.Len(), len(body))
			}
		}))
		defer ts.Close()

		// Maybe write a second test for th

		t.Run("with content length header", func(t *testing.T) {
			header := &http.Header{}
			header.Set("Content-Length", strconv.Itoa(10*1024*1024))
			req := newRequest(ts.URL, "PUT", header)
			bodyFile, err := os.Open(filename)
			if err != nil {
				t.Fatal(err)
			}

			body, err := newBody(bodyFile, 0, 10*1024*1024)
			if err != nil {
				t.Fatal(err)
			}
			defer body.Close()

			_, _, err = client.run(req, body, 1024, nil, false)

			if err != nil {
				t.Fatal(err)
			}
		})

		t.Run("without content length header", func(t *testing.T) {
			req := newRequest(ts.URL, "PUT", nil)
			bodyFile, err := os.Open(filename)
			if err != nil {
				t.Fatal(err)
			}

			body, err := newBody(bodyFile, 0, 10*1024*1024)
			if err != nil {
				t.Fatal(err)
			}
			defer body.Close()

			_, _, err = client.run(req, body, 1024, nil, false)

			if err != nil {
				t.Fatal(err)
			}
		})
	})

	t.Run("reads response body correctly", func(t *testing.T) {

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := io.Copy(w, bytes.NewBuffer(body))
			if err != nil {
				t.Fatal(err)
			}
		}))
		defer ts.Close()

		req := newRequest(ts.URL, "GET", nil)

		var output bytes.Buffer

		_, _, err = client.run(req, nil, 1024, &output, false)

		if !bytes.Equal(output.Bytes(), body) {
			t.Fatalf("Response output does not match expected value")
		}

		if err != nil {
			t.Fatal(err)
		}

	})

	t.Run("sha256 and length verified requests", func(t *testing.T) {
		t.Run("identity encoding", func(t *testing.T) {
			t.Run("can run an empty request", func(t *testing.T) {
				ts := createServer(int(200), "0", emptySha256, "", "", "", []byte(""))
				defer ts.Close()

				req := newRequest(ts.URL, "GET", nil)
				_, _, err := client.run(req, nil, 1024, nil, true)
				if err != nil {
					t.Fatal(err)
				}
			})

			t.Run("can run a request", func(t *testing.T) {
				ts := createServer(http.StatusOK, sl(body), hb(body), "", "", "", body)
				defer ts.Close()

				req := newRequest(ts.URL, "GET", nil)

				_, _, err := client.run(req, nil, 1024, nil, true)

				if err != nil {
					t.Fatal(err)
				}
			})

			t.Run("can run a request with redundant transfer headers", func(t *testing.T) {
				ts := createServer(http.StatusOK, sl(body), hb(body), sl(body), hb(body), "identity", body)
				defer ts.Close()

				req := newRequest(ts.URL, "GET", nil)

				_, _, err := client.run(req, nil, 1024, nil, true)

				if err != nil {
					t.Fatal(err)
				}
			})

			t.Run("can run a request with incorrect redundant transfer hash", func(t *testing.T) {
				ts := createServer(http.StatusOK, sl(body), hb(body), sl(body), hb(gzipBody), "identity", body)
				defer ts.Close()

				req := newRequest(ts.URL, "GET", nil)

				_, _, err := client.run(req, nil, 1024, nil, true)

				if err == nil {
					t.Fatal(err)
				}
			})

			t.Run("returns error when content length is wrong", func(t *testing.T) {
				ts := createServer(http.StatusOK, "123456", hb(body), "", "", "", body)
				defer ts.Close()

				req := newRequest(ts.URL, "GET", nil)

				_, _, err := client.run(req, nil, 1024, nil, true)

				// do better error checking that we got the expected error
				if err == nil {
					t.Fatal(err)
				}
			})

			t.Run("returns error when the content hash is wrong", func(t *testing.T) {
				ts := createServer(http.StatusOK, sl(body), hb([]byte("notcorrect")), "", "", "", body)
				defer ts.Close()

				req := newRequest(ts.URL, "GET", nil)

				_, _, err := client.run(req, nil, 1024, nil, true)

				// do better error checking that we got the expected error
				if err == nil {
					t.Fatal(err)
				}
			})
		})

		t.Run("gzip encoding", func(t *testing.T) {
			t.Run("can run a request", func(t *testing.T) {
				ts := createServer(http.StatusOK, sl(body), hb(body), sl(gzipBody), hb(gzipBody), "gzip", gzipBody)
				defer ts.Close()

				req := newRequest(ts.URL, "GET", nil)

				_, _, err = client.run(req, nil, 1024, nil, true)

				if err != nil {
					t.Fatal(err)
				}
			})

			t.Run("returns error for incorrect transfer hash", func(t *testing.T) {

				ts := createServer(http.StatusOK, sl(body), hb(body), sl(gzipBody), hb(body), "gzip", gzipBody)
				defer ts.Close()

				req := newRequest(ts.URL, "GET", nil)

				_, _, err = client.run(req, nil, 1024, nil, true)

				if err == nil {
					t.Fatal(err)
				}
			})

			t.Run("returns error for incorrect transfer length", func(t *testing.T) {

				ts := createServer(http.StatusOK, sl(body), hb(body), "123456", hb(body), "gzip", gzipBody)
				defer ts.Close()

				req := newRequest(ts.URL, "GET", nil)

				_, _, err = client.run(req, nil, 1024, nil, true)

				if err == nil {
					t.Fatal(err)
				}
			})

			t.Run("returns error for invalid gzip bodies", func(t *testing.T) {

				ts := createServer(http.StatusOK, sl(body), hb(body), sl(body), hb(body), "gzip", body)
				defer ts.Close()

				req := newRequest(ts.URL, "GET", nil)

				_, _, err = client.run(req, nil, 1024, nil, true)

				if err == nil {
					t.Fatal(err)
				}
			})
		})
	})
}
