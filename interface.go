package artifact

import (
	"fmt"
	"os"

	tcclient "github.com/taskcluster/taskcluster-client-go"
	queue "github.com/taskcluster/taskcluster-client-go/queue"
)

type ArtifactClient struct {
	queue *queue.Queue
	agent client
}

// Create a new Artifact action runner
func New(creds *tcclient.Credentials) *ArtifactClient {
	q := queue.New(creds)
	a := newAgent()
	return &ArtifactClient{q, a}
}

// The size at which multipart uploading is chosen automatically
const MULTIPART_SIZE int64 = 500 * 1024 * 1024 // 500MB

// Upload a new artifact at the given taskId, runId and name.
// This method automatically selects whether to use the single part
// or multi part codepath
func (a *ArtifactClient) Upload(taskId, runId, name, filename string, gzip bool) error {
	fi, err := os.Stat(filename)
	if err != nil {
		return err
	}

	if fi.Size() > 500*1024*1024 {
		logger.Printf("File %s is %d bytes, choosing multi-part upload", filename, fi.Size())
		return a.MultiPartUpload(taskId, runId, name, filename, gzip)
	} else {
		logger.Printf("File %s is %d bytes, choosing single part upload", filename, fi.Size())
		return a.SinglePartUpload(taskId, runId, name, filename, gzip)
	}
}

// Upload a new artifact at the given taskId, runId and name as in
// Artifact.Upload, except that it forces the single part upload strategy
func (a *ArtifactClient) SinglePartUpload(taskId, runId, name, filename string, gzip bool) error {
	// TODO: FILL THIS REQUEST BODY OUT
	payload := &queue.PostArtifactRequest{}
	response, err := a.queue.CreateArtifact(taskId, runId, name, payload)
	if err != nil {
		logger.Printf("Error calling Queue.CreateArtifact: %s", err)
		return err
	}

	fmt.Printf("%+v\n", response)

	return nil
}

// Upload a new artifact at the given taskId, runId and name as in the
// Artifact.Upload method except that it forces the multi part upload strategy
func (a *ArtifactClient) MultiPartUpload(taskId, runId, name, filename string, gzip bool) error {
	return nil
}

// Download an artifact from the given taskId, runId and name.  The output
// value is the filename where the resulting transfer should be saved.  This
// function will overwrite existing files at this path.  This function will do
// verifications to ensure that the transfered artifact matches the header
// values in the artifact
func (a *ArtifactClient) Download(taskId, runId, name, filename string) error {
	return nil
}

// Download the latest artifact from the given taskId and name.  The output
// value is the filename where the resulting transfer should be saved.  This
// function will overwrite existing files at this path.  This function will do
// verifications to ensure that the transfered artifact matches the header
// values in the artifact
func (a *ArtifactClient) DownloadLatest(taskId, name, filename string) error {
	return nil
}
