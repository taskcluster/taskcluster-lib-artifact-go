package artifact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	tcclient "github.com/taskcluster/taskcluster-client-go"
	queue "github.com/taskcluster/taskcluster-client-go/queue"
)

// Client is a struct which can upload and download artifacts.  All http
// requests run by the same instance of a Client are run through the same http
// transport
type Client struct {
	agent client
}

// We need this different from the request.go:request type because that struct
// uses http.Header headers and our api returns a different type of headers.
// This would be a great cleanup one day
type apiRequest struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
}

// blobArtifactResponse is the response from queue.CreateArtifact
type blobArtifactResponse struct {
	StorageType string        `json:"storageType"`
	Requests    []apiRequest  `json:"requests"`
	Expires     tcclient.Time `json:"expires"`
}

// New creates a new Client
func New(creds *tcclient.Credentials) *Client {
	a := newAgent()
	return &Client{a}
}

// ChunkSize is the size of each Read() and Write() call
const ChunkSize int = 32 * 1024 // 32KB

// MultiPartSize is the size at which the automatically selecting Upload()
// method will choose to instead do a multi-part upload instead of a single
// part one
const MultiPartSize int64 = 500 * 1024 * 1024 // 500MB

// MultiPartPartChunkCount is the number of CHUNK_SIZE chunks should comprise a
// single multi-part part.
const MultiPartPartChunkCount int = 100 * 1024 * 1024 / ChunkSize // 100MB

// Perform an upload
func (c *Client) Upload(taskID, runID, name string, input io.ReadSeeker, output io.ReadWriteSeeker, gzip, multipart bool, q *queue.Queue) error {

	var err error
	var u upload

	if multipart {
		u, err = multiPartUpload(input, output, gzip, ChunkSize, MultiPartPartChunkCount)
		if err != nil {
			return err
		}
	} else {
		u, err = singlePartUpload(input, output, gzip, ChunkSize)
		if err != nil {
			return err
		}
	}

	bareq := &queue.BlobArtifactRequest{
		ContentEncoding: u.ContentEncoding,
		ContentLength:   u.Size,
		ContentSha256:   fmt.Sprintf("%x", u.Sha256),
		TransferLength:  u.TransferSize,
		TransferSha256:  fmt.Sprintf("%x", u.TransferSha256),
		ContentType:     "application/octet-stream",
		Expires:         tcclient.Time(time.Now().UTC().AddDate(0, 0, 1)),
		StorageType:     "blob",
	}

	if multipart {
		// We don't match the API's structure exactly, so let's do that
		parts := make([]struct {
			Sha256 string "json:\"sha256,omitempty\""
			Size   int64  "json:\"size,omitempty\""
		}, len(u.Parts))
		for i := 0; i < len(u.Parts); i++ {
			parts[i].Sha256 = fmt.Sprintf("%x", u.Parts[i].Sha256)
			parts[i].Size = u.Parts[i].Size
		}
		bareq.Parts = parts
	}

	cap, err := json.Marshal(&bareq)
	if err != nil {
		return err
	}

	pareq := queue.PostArtifactRequest(json.RawMessage(cap))

	resp, err := q.CreateArtifact(taskID, runID, name, &pareq)
	if err != nil {
		return err
	}

	var bares blobArtifactResponse

	// BEGIN HACK
	// TODO There's a slight hack required here because the queue is currently
	// serving the content length with a non-string value.  This has been
	// addressed in Queue PR#220 but we're going to do a quick hack because of
	// the change freeze.  Basically, we're going to rewrite the content-length
	// as a string in json terms.  This is a horrible idea for production, so we
	// really do need #220 to land before deploying this
	clpat := regexp.MustCompile("\"content-length\"[[:space:]]*:[[:space:]]*(\\d*)")
	fixed := clpat.ReplaceAll(*resp, []byte("\"content-length\": \"$1\""))
	err = json.Unmarshal(fixed, &bares)
	// END HACK
	//err = json.Unmarshal(*resp, &bares)
	if err != nil {
		return err
	}

	etags := make([]string, len(bares.Requests))

	// There's a bit of a difficulty that's going to happen when we start
	// supporting concurrency here.  The underlying ReadSeeker is going to be
	// changing the position in the stream for the other readers.  We're going to
	// have to figure out something to prevent the file from being read from
	// totally random places.  To support this concurrency without passing files
	// (e.g.  using ReadSeekers) we could do something like the following:
	//   1. Create a mutex for file reads
	//   2. Each read to the file will lock the mutex
	//   3. Each read to the file will seek to the correct position
	//   4. Each read to the file will read the number of bytes needed
	//   5. Each reader of the file will keep track of the next place it needs to
	//      read from (e.g. where it seek'ed to + the number of bytes that it read)
	//   6. Each read to the file will unlock the mutex
	// Another option would be to pass in a factory method instead of raw
	// ReadSeekers and have the factory return a ReadSeeker for each
	// request body.  Maybe we really need a ReaderAtSeekCloser...
	for i := 0; i < len(bares.Requests); i++ {
		r := bares.Requests[i]
		req, err := newRequestFromStringMap(r.URL, r.Method, r.Headers)
		if err != nil {
			return err
		}

		body, err := newBody(input, u.Parts[i].Start, u.Parts[i].Size)
		if err != nil {
			return err
		}

		// In this case, we're going to store the output of the request in memory
		// because we're pretty sure in this method that it's going to be an S3
		// error message and we'd like to print that
		var outputBuf bytes.Buffer
		cs, _, err := c.agent.run(req, body, ChunkSize, &outputBuf, false)
		if err != nil {
			logger.Printf("%s\n%s", cs, outputBuf.String())
			return err
		}
		outputBuf.Reset()

		etags[i] = cs.ResponseHeader.Get("etag")
	}

	careq := queue.CompleteArtifactRequest{
		Etags: etags,
	}

	err = q.CompleteArtifact(taskID, runID, name, &careq)
	if err != nil {
		return err
	}

	logger.Printf("Etags: %#v", etags)
	return nil

}

// Because we generate different URLs based on whether we're asking for latest
// or not
func (c *Client) download(url string, outputWriter io.Writer, q *queue.Queue) error {

	request := newRequest(url, "GET", &http.Header{})

	var redirectBuf bytes.Buffer

	cs, _, err := c.agent.run(request, nil, ChunkSize, &redirectBuf, false)
	if err != nil {
		logger.Printf("%s\n%s", cs, redirectBuf.String())
		return err
	}
	// Make sure we release the memory stored in the redirect buffer
	redirectBuf.Reset()

	location := cs.ResponseHeader.Get("Location")

	request = newRequest(location, "GET", &http.Header{})

	// Now we're going to request the artifact for real.  We're going to write directly
	// to the outputWriter.  This does mean, unfortunately, that the outputWriter will
	// contain the
	cs, _, err = c.agent.run(request, nil, ChunkSize, outputWriter, true)
	if err != nil {
		logger.Printf("%s", cs)
		return err
	}

	return nil
}

// Download will download the named artifact from a specific run of a task.  If
// an error occurs during the download, the response body of the error message
// will be written instead of the artifact's content.  This is so that we can
// stream the response to the outputWriter instead of buffering it in memory.
// If this behaviour is unacceptable for your use case, you should delete the
// resource that's being written to on an error of this method
func (c *Client) Download(taskID, runID, name string, outputWriter io.Writer, q *queue.Queue) error {
	// We need to build the URL because we're going to need to get the redirect's
	// headers.  That's not possible with the q.GetArtifact() method.  Ideally,
	// we'd have a q.GetArtifact_BuildURL method which would allow us to do
	// unauthenticated requests for those resources which have a name starting
	// with "public/"

	// TODO: How long should this signed url really be valid for?
	url, err := q.GetArtifact_SignedURL(taskID, runID, name, time.Duration(3)*time.Hour)
	if err != nil {
		return err
	}

	return c.download(url.String(), outputWriter, q)

}

// DownloadLatest will download the named artifact from the latest run of a
// task
func (c *Client) DownloadLatest(taskID, name string, outputWriter io.Writer, q *queue.Queue) error {
	// We need to build the URL because we're going to need to get the redirect's
	// headers.  That's not possible with the q.GetArtifact() method.  Ideally,
	// we'd have a q.GetArtifact_BuildURL method which would allow us to do
	// unauthenticated requests for those resources which have a name starting
	// with "public/"

	// TODO: How long should this signed url really be valid for?
	url, err := q.GetLatestArtifact_SignedURL(taskID, name, time.Duration(1)*time.Hour)
	if err != nil {
		return err
	}

	return c.download(url.String(), outputWriter, q)
}
