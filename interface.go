package artifact

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	tcclient "github.com/taskcluster/taskcluster-client-go"
	queue "github.com/taskcluster/taskcluster-client-go/queue"
)

// Client is a struct which can upload and download artifacts.  All http
// requests run by the same instance of a Client are run through the same http
// transport
type Client struct {
	queue *queue.Queue
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
	q := queue.New(creds)
	a := newAgent()
	return &Client{q, a}
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

// Upload an artifact and let this library decide whether or not to use the
// single part or multi part upload flow
func (a *Client) Upload(taskID, runID, name, filename string, gzip bool) error {
	fi, err := os.Stat(filename)
	if err != nil {
		return err
	}

	if fi.Size() > 500*1024*1024 {
		logger.Printf("File %s is %d bytes, choosing multi-part upload", filename, fi.Size())
		return a.MultiPartUpload(taskID, runID, name, filename, gzip)
	}

	logger.Printf("File %s is %d bytes, choosing single part upload", filename, fi.Size())
	return a.SinglePartUpload(taskID, runID, name, filename, gzip)
}

// SinglePartUpload performs a single part upload
func (c *Client) SinglePartUpload(taskID, runID, name, filename string, gzip bool) error {
	spu, err := newSinglePartUpload(filename, "lala", ChunkSize, gzip)
	if err != nil {
		return err
	}

	cap, err := json.Marshal(&queue.BlobArtifactRequest{
		ContentEncoding: spu.ContentEncoding,
		ContentLength:   spu.Size,
		ContentSha256:   fmt.Sprintf("%x", spu.Sha256),
		TransferLength:  spu.TransferSize,
		TransferSha256:  fmt.Sprintf("%x", spu.TransferSha256),
		ContentType:     "application/octet-stream",
		Expires:         tcclient.Time(time.Now().UTC().AddDate(0, 0, 1)),
		StorageType:     "blob",
	})
	if err != nil {
		return err
	}

	par := queue.PostArtifactRequest(json.RawMessage(cap))

	resp, err := c.queue.CreateArtifact(taskID, runID, name, &par)
	if err != nil {
		return err
	}

	var bar blobArtifactResponse

	err = json.Unmarshal(*resp, &bar)
	if err != nil {
		return err
	}

	for _, r := range bar.Requests {
		req, err := newRequestFromStringMap(r.URL, r.Method, r.Headers)
		if err != nil {
			return err
		}

		body, err := newBody(spu.Filename, 0, spu.Size)
		if err != nil {
			return err
		}

		cs, _, err := c.agent.run(req, body, ChunkSize, "out", false)
		if err != nil {
			logger.Printf("%s", cs)
			return err
		}

	}

	return nil
}

// MultiPartUpload performs a multi part upload
func (a *Client) MultiPartUpload(taskID, runID, name, filename string, gzip bool) error {
	mpu, err := newMultiPartUpload(filename, "lala", ChunkSize, MultiPartPartChunkCount, gzip)
	if err != nil {
		return err
	}

	fmt.Printf("%+v\n", mpu)
	return nil
}

// Download will download the named artifact from a specific run of a task
func (a *Client) Download(taskID, runID, name, filename string) error {
	return nil
}

// DownloadLatest will download the named artifact from the latest run of a
// task
func (a *Client) DownloadLatest(taskID, name, filename string) error {
	return nil
}
