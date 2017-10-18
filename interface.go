package artifact

import (
	"fmt"
	"os"

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
func (a *Client) SinglePartUpload(taskID, runID, name, filename string, gzip bool) error {
	spu, err := newSinglePartUpload(filename, "lala", ChunkSize, gzip)
	if err != nil {
		return err
	}

	fmt.Printf("%+v\n", spu)
	// call createArtifact
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
