package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/taskcluster/slugid-go/slugid"
	tcclient "github.com/taskcluster/taskcluster-client-go"
	"github.com/taskcluster/taskcluster-client-go/queue"
	artifact "github.com/taskcluster/taskcluster-lib-artifact-go"
)

var taskGroupID = slugid.Nice()

// Copied from the generic-worker's artifact tests (thanks Pete!)
func testTask(t *testing.T) *queue.TaskDefinitionRequest {
	created := time.Now().UTC()
	// reset nanoseconds
	created = created.Add(time.Nanosecond * time.Duration(created.Nanosecond()*-1))
	// deadline in one hour' time
	deadline := created.Add(15 * time.Minute)
	// expiry in one day, in case we need test results
	expires := created.AddDate(0, 0, 2)

	return &queue.TaskDefinitionRequest{
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
			Description: "taskcluster-lib-artifact-go test",
			Name:        "taskcluster-lib-artifact-go test",
			Owner:       "taskcluster-lib-artifact-go-ci@mozilla.com",
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
}

var allTheBytes = []byte{1, 3, 7, 15, 31, 63, 127, 255}

const filename string = "test-file"

// TODO: Should this still return an error or is a t.Fatal call in here enough?
func prepareFiles(t *testing.T) {
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 256; i++ {
		_, err = file.Write(allTheBytes)
		if err != nil {
			t.Fatal(err)
		}
	}
	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestIntegration(t *testing.T) {
	prepareFiles(t)

	creds := &tcclient.Credentials{}

	if value, present := os.LookupEnv("TASKCLUSTER_CLIENT_ID"); present {
		creds.ClientID = value
	}

	if value, present := os.LookupEnv("TASKCLUSTER_ACCESS_TOKEN"); present {
		creds.AccessToken = value
	}

	if value, present := os.LookupEnv("TASKCLUSTER_CERTIFICATE"); present {
		creds.Certificate = value
	}

	q := queue.New(creds)
	client := artifact.New(creds)
	taskID := slugid.Nice()
	runID := "0"
	t.Logf("TaskGroupId: %s Task ID: %s", taskGroupID, taskID)

	_, err := q.CreateTask(taskID, testTask(t))
	if err != nil {
		t.Fatal(err)
	}

	tcr := queue.TaskClaimRequest{WorkerGroup: "my-worker-group", WorkerID: "my-worker"}
	_, err = q.ClaimTask(taskID, "0", &tcr)
	if err != nil {
		t.Fatal(err)
	}
	// TODO: Do a loop to support gzip and non-gzip, for now only gzip

	t.Run("should be able to upload and download artifact as single part and gzip", func(t *testing.T) {
		name := "public/forced-single-part"
		err = client.SinglePartUpload(taskID, runID, name, filename, false)
		if err != nil {
			t.Fatal(err)
		}
		err = client.Download(taskID, runID, name, filename+"_out")
		if err != nil {
			t.Fatal(err)
		}
	})

	/*t.Run("should be able to upload artifact as multi part and gzip", func(t *testing.T) {
		name := "public/forced-multi-part"
		err = client.MultiPartUpload(taskID, runID, name, filename, true)
		if err != nil {
			t.Fatal(err)
		}
		err = client.Download(taskID, runID, name, filename+"_out")
		if err != nil {
			t.Fatal(err)
		}
	})*/

	// TODO: Write tests for Download vs. DownloadLatest

}
