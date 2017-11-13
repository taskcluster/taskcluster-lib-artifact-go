package artifactTests

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/taskcluster/slugid-go/slugid"
	tcclient "github.com/taskcluster/taskcluster-client-go"
	"github.com/taskcluster/taskcluster-client-go/queue"
	artifact "github.com/taskcluster/taskcluster-lib-artifact-go"
)

type testEnv struct {
	input    *os.File
	output   *os.File
	filename string
	body     []byte
}

// Copied from the generic-worker's artifact tests (thanks Pete!)
func testTask(t *testing.T, taskGroupID string) *queue.TaskDefinitionRequest {
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

// Set up the test environment.  Return
func setup(t *testing.T) (testEnv, func()) {
	var err error

	env := testEnv{}

	env.input, err = ioutil.TempFile(".", "test-file")
	if err != nil {
		t.Error(err)
	}

	env.output, err = ioutil.TempFile(".", env.input.Name()+"_output")
	if err != nil {
		t.Error(err)
	}

	env.body = prepareFile(t, env.input)

	return env, func() {
		err := env.input.Close()
		if err != nil {
			t.Error(err)
		}
		err = os.Remove(env.input.Name())
		if err != nil {
			t.Error(err)
		}
		err = env.output.Close()
		if err != nil {
			t.Error(err)
		}
		err = os.Remove(env.output.Name())
		if err != nil {
			t.Error(err)
		}
	}
}

func prepareFile(t *testing.T, file *os.File) []byte {
	var buf bytes.Buffer
	out := io.MultiWriter(file, &buf)

	rbuf := make([]byte, 1024*1024) // 1MB

	for i := 0; i < 10; i++ {
		rand.Read(rbuf)
		nBytes, err := out.Write(rbuf)
		if err != nil {
			t.Fatal(err)
		}
		if nBytes != 1024*1024 {
			t.Fatalf("did not write the expected number of bytes(%d)", nBytes)
		}
	}

	// Put the file back into the state we found it in
	_, err := file.Seek(0, io.SeekStart)
	if err != nil {
		t.Error(err)
	}

	return buf.Bytes()
}

func downloadCheck(t *testing.T, client *artifact.Client, expected []byte, taskID, runID, name string) {
	// Download a specific resource

	var output bytes.Buffer

	err := client.Download(taskID, runID, name, &output)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(output.Bytes(), expected) {
		t.Fatal("Downloaded body does not match expected body")
	}

	t.Logf("Downloaded specific artifact %s-%s-%s", taskID, runID, name)
	output.Reset()
	err = client.DownloadLatest(taskID, name, &output)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(output.Bytes(), expected) {
		t.Fatal("Downloaded body does not match expected body")
	}
	t.Logf("Downloaded latest artifact %s-%s-%s", taskID, runID, name)
}

// Create and claim a task, returning a pointer to a Queue which is configured
// with the credentials for the task
func createTask(t *testing.T, taskGroupID, taskID, runID string) *queue.Queue {
	q, err := queue.New(nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = q.CreateTask(taskID, testTask(t, taskGroupID))
	if err != nil {
		t.Fatal(err)
	}

	tcr := queue.TaskClaimRequest{WorkerGroup: "my-worker-group", WorkerID: "my-worker"}
	tcres, err := q.ClaimTask(taskID, "0", &tcr)
	if err != nil {
		t.Fatal(err)
	}

	// We have to restructure the response's credentials into tcclient.Credentials
	taskQ, err := queue.New(&tcclient.Credentials{
		ClientID:    tcres.Credentials.ClientID,
		AccessToken: tcres.Credentials.AccessToken,
		Certificate: tcres.Credentials.Certificate,
	})
	if err != nil {
		t.Fatal(err)
	}
	return taskQ
}

func testUploadAndDownload(t *testing.T, client *artifact.Client, taskID, runID, name string, gzip, mp bool) {
	env, teardown := setup(t)
	defer teardown()
	err := client.Upload(taskID, runID, name, env.input, env.output, gzip, mp)
	if err != nil {
		t.Fatal(err)
	}
	downloadCheck(t, client, env.body, taskID, runID, name)

}

func TestUploadAndDownload(t *testing.T) {
	artifact.SetLogOutput(newUnitTestLogWriter(t))

	taskGroupID := slugid.Nice()
	taskID := slugid.Nice()
	runID := "0"

	t.Logf("Task Group ID: %s, Task ID: %s, Run ID: %s", taskGroupID, taskID, runID)

	taskQ := createTask(t, taskGroupID, taskID, runID)

	client := artifact.New(taskQ)

	t.Run("single part identity", func(t *testing.T) {
		testUploadAndDownload(t, client, taskID, runID, "public/sp-id", false, false)
	})

	t.Run("multi part identity", func(t *testing.T) {
		testUploadAndDownload(t, client, taskID, runID, "public/mp-id", false, true)
	})

	t.Run("single part gzip", func(t *testing.T) {
		testUploadAndDownload(t, client, taskID, runID, "public/sp-gz", true, false)
	})

	t.Run("multi part gzip", func(t *testing.T) {
		testUploadAndDownload(t, client, taskID, runID, "public/mp-gz", true, true)
	})
}
