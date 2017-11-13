package artifact

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io/ioutil"
	"os"
	"time"

	"github.com/taskcluster/slugid-go/slugid"
	tcclient "github.com/taskcluster/taskcluster-client-go"
	queue "github.com/taskcluster/taskcluster-client-go/queue"
)

var taskGroupID = slugid.Nice()
var taskID = slugid.Nice()
var runID = "0"

func setupEnvironment() (*queue.Queue, error) {
	// The first large amount of code is set up code to get you into the the
	// correct task environment.  The expectation is that those using this
	// library will already have this code set up

	q, err := queue.New(nil)

	created := time.Now().UTC()
	// reset nanoseconds
	created = created.Add(time.Nanosecond * time.Duration(created.Nanosecond()*-1))
	// deadline in one hour' time
	deadline := created.Add(15 * time.Minute)
	// expiry in one day, in case we need test results
	expires := created.AddDate(0, 0, 2)

	taskDefinition := &queue.TaskDefinitionRequest{
		Created:      tcclient.Time(created),
		Deadline:     tcclient.Time(deadline),
		Expires:      tcclient.Time(expires),
		Extra:        json.RawMessage(`{}`),
		Dependencies: []string{},
		Requires:     "all-completed",
		Metadata: struct {
			Description string `json:"description"`
			Name        string `json:"name"`
			Owner       string `json:"owner"`
			Source      string `json:"source"`
		}{
			Description: "taskcluster-lib-artifact-go example",
			Name:        "taskcluster-lib-artifact-go example",
			Owner:       "example",
			Source:      "https://github.com/taskcluster/taskcluster-lib-artifact-go",
		},
		Payload:       json.RawMessage(`{}`),
		ProvisionerID: "no-provisioner",
		Retries:       1,
		Routes:        []string{},
		SchedulerID:   "test-scheduler",
		Scopes:        []string{},
		Tags:          json.RawMessage(`{"CI":"taskcluster-lib-artifact-go"}`),
		Priority:      "lowest",
		TaskGroupID:   taskGroupID,
		WorkerType:    "my-workertype",
	}
	_, err = q.CreateTask(taskID, taskDefinition)
	if err != nil {
		return nil, err
	}

	tcr := queue.TaskClaimRequest{WorkerGroup: "my-worker-group", WorkerID: "my-worker"}
	tcres, err := q.ClaimTask(taskID, "0", &tcr)
	if err != nil {
		return nil, err
	}

	taskQ, err := queue.New(&tcclient.Credentials{
		ClientID:    tcres.Credentials.ClientID,
		AccessToken: tcres.Credentials.AccessToken,
		Certificate: tcres.Credentials.Certificate,
	})
	if err != nil {
		return nil, err
	}

	return taskQ, nil

}

func createInput(size int) (*bytes.Reader, error) {
	var buf bytes.Buffer

	// 25MB
	rbuf := make([]byte, 1024*1024)

	for i := 0; i < size; i++ {
		_, err := rand.Read(rbuf)

		if err != nil {
			return nil, err
		}

		_, err = buf.Write(rbuf)
		if err != nil {
			return nil, err
		}

	}

	return bytes.NewReader(buf.Bytes()), nil
}

func ExampleClient_Upload() {

	// We need a task specific taskcluster-client-go Queue
	taskQ, err := setupEnvironment()
	if err != nil {
		panic(err)
	}

	client := New(taskQ)

	// We're going to create an input for uploading artifacts
	input, err := createInput(25) // 25MB
	if err != nil {
		panic(err)
	}

	output, err := ioutil.TempFile(".", ".scratch")
	if err != nil {
		panic(err)
	}
	defer os.Remove(output.Name())

	// We're not interested in logs
	SetLogOutput(ioutil.Discard)

	// Let's tune the chunk and part size for our system so that we have 16KB
	// chunks and 5MB parts
	client.SetInternalSizes(16*1024, 5*1024*1024)

	// Let's upload the files four different ways
	err = client.Upload(taskID, runID, "public/single-part-identity", input, output, false, false)
	if err != nil {
		panic(err)
	}

	err = client.Upload(taskID, runID, "public/single-part-gzip", input, output, true, false)
	if err != nil {
		panic(err)
	}

	err = client.Upload(taskID, runID, "public/multi-part-identity", input, output, false, true)
	if err != nil {
		panic(err)
	}

	err = client.Upload(taskID, runID, "public/multi-part-gzip", input, output, true, true)
	if err != nil {
		panic(err)
	}
}

func ExampleClient_Download() {

	var output bytes.Buffer

	// We don't need authenticated Queues for public downloads
	client := New(nil)

	err := client.Download(taskID, runID, "public/single-part-gzip", &output)
	if err != nil {
		panic(err)
	}
}

func ExampleClient_DownloadLatest() {

	var output bytes.Buffer

	// We don't need authenticated Queues for public downloads
	client := New(nil)

	err := client.DownloadLatest(taskID, "public/single-part-gzip", &output)
	if err != nil {
		panic(err)
	}
}
