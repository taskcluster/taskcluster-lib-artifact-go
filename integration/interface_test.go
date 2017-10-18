package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/taskcluster/slugid-go/slugid"
	tcclient "github.com/taskcluster/taskcluster-client-go"
	"github.com/taskcluster/taskcluster-client-go/queue"
)

var taskGroupId string = slugid.Nice()

// Copied from the generic-worker's artifact tests (thanks Pete!)
func testTask(t *testing.T) *queue.TaskDefinitionRequest {
	created := time.Now().UTC()
	// reset nanoseconds
	created = created.Add(time.Nanosecond * time.Duration(created.Nanosecond()*-1))
	// deadline in one hour' time
	deadline := created.Add(15 * time.Minute)
	// expiry in one day, in case we need test results
	expires := created.AddDate(0, 0, 1)

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
		TaskGroupID:   taskGroupId,
		WorkerType:    "my-workertype",
	}
}

func TestIntegration(t *testing.T) {
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
	//client := artifact.New(&tcclient.Credentials{})
	taskId := slugid.Nice()
	t.Logf("TaskGroupId: %s Task ID: %s", taskGroupId, taskId)

	_, err := q.CreateTask(taskId, testTask(t))
	if err != nil {
		t.Fatal(err)
	}

	tcr := queue.TaskClaimRequest{WorkerGroup: "my-worker-group", WorkerID: "my-worker"}
	_, err = q.ClaimTask(taskId, "0", &tcr)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("should be able to upload artifact", func(t *testing.T) {
	})

	t.Run("should be able to download artifact from specific run", func(t *testing.T) {

	})

	t.Run("should be able to download artifact from latest run", func(t *testing.T) {

	})

}
