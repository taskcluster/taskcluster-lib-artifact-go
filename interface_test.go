package artifact

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/taskcluster/slugid-go/slugid"
	tcclient "github.com/taskcluster/taskcluster-client-go"
	"github.com/taskcluster/taskcluster-client-go/tcqueue"
)

func setupEnvironment() (*tcqueue.Queue, string, string, error) {
	taskGroupID := slugid.Nice()
	taskID := slugid.Nice()
	runID := "0"

	// The first large amount of code is set up code to get you into the the
	// correct task environment.  The expectation is that those using this
	// library will already have this code set up

	q := tcqueue.NewFromEnv()

	created := time.Now().UTC()
	// reset nanoseconds
	created = created.Add(time.Nanosecond * time.Duration(created.Nanosecond()*-1))
	// deadline in one hour' time
	deadline := created.Add(15 * time.Minute)
	// expiry in one day, in case we need test results
	expires := created.AddDate(0, 0, 2)

	taskDefinition := &tcqueue.TaskDefinitionRequest{
		Created:      tcclient.Time(created),
		Deadline:     tcclient.Time(deadline),
		Expires:      tcclient.Time(expires),
		Extra:        json.RawMessage(`{}`),
		Dependencies: []string{},
		Requires:     "all-completed",
		Metadata: tcqueue.TaskMetadata{
			Description: "taskcluster-lib-artifact-go example",
			Name:        "taskcluster-lib-artifact-go example",
			Owner:       "example@example.com",
			Source:      "https://github.com/taskcluster/taskcluster-lib-artifact-go",
		},
		Payload:       json.RawMessage(`{}`),
		ProvisionerID: "no-provisioner",
		Retries:       1,
		Routes:        []string{},
		SchedulerID:   "test-scheduler",
		Scopes:        []string{},
		Tags:          map[string]string{"CI": "taskcluster-lib-artifact-go"},
		Priority:      "lowest",
		TaskGroupID:   taskGroupID,
		WorkerType:    "my-workertype",
	}
	_, err := q.CreateTask(taskID, taskDefinition)
	if err != nil {
		return nil, taskID, runID, err
	}

	tcr := tcqueue.TaskClaimRequest{WorkerGroup: "my-worker-group", WorkerID: "my-worker"}
	tcres, err := q.ClaimTask(taskID, "0", &tcr)
	if err != nil {
		return nil, taskID, runID, err
	}

	taskQ := tcqueue.New(&tcclient.Credentials{
		ClientID:    tcres.Credentials.ClientID,
		AccessToken: tcres.Credentials.AccessToken,
		Certificate: tcres.Credentials.Certificate,
	}, os.Getenv("TASKCLUSTER_ROOT_URL"))

	return taskQ, taskID, runID, nil

}

func createInput(size int) *bytes.Reader {
	var buf bytes.Buffer

	// 25MB
	rbuf := make([]byte, 1024*1024)

	for i := 0; i < size; i++ {
		_, err := rand.Read(rbuf)

		if err != nil {
			panic(err)
		}

		_, err = buf.Write(rbuf)
		if err != nil {
			panic(err)
		}

	}

	return bytes.NewReader(buf.Bytes())
}

func TestInterface(t *testing.T) {

	// We need a task specific taskcluster-client-go Queue
	taskQ, taskID, runID, err := setupEnvironment()
	if err != nil {
		t.Fatal(err)
	}

	client := New(taskQ)

	t.Run("error-artifacts", func(t *testing.T) {
		err = client.CreateError(taskID, runID, "public/error", "invalid-resource-on-worker", "test error message")
		if err != nil {
			t.Fatal(err)
		}

		var output bytes.Buffer
		err = client.Download(taskID, runID, "public/error", &output)
		if err != ErrErr {
			t.Fatal(err)
		}
	})

	t.Run("reference-artifacts", func(t *testing.T) {
		err = client.CreateReference(taskID, runID, "public/reference", "https://www.google.com")
		if err != nil {
			t.Fatal(err)
		}

		var output bytes.Buffer
		err = client.Download(taskID, runID, "public/reference", &output)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("upload-blob", func(t *testing.T) {
		if err = os.MkdirAll("testdata", 0755); err != nil {
			t.Fatal(err)
		}

		// We're not interested in logs
		//SetLogOutput(ioutil.Discard)

		// Let's tune the chunk and part size for our system so that we have 16KB
		// chunks and 5MB parts
		client.SetInternalSizes(16*1024, 5*1024*1024)

		t.Run("public/single-part-identity", func(t *testing.T) {
			input := createInput(25) // 25MB
			output, err := ioutil.TempFile("testdata", ".scratch")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(output.Name())

			// Let's upload the files four different ways
			err = client.Upload(taskID, runID, "public/single-part-identity", input, output, false, false)
			if err != nil {
				t.Fatal(err)
			}
		})

		t.Run("public/single-part-gzip", func(t *testing.T) {
			input := createInput(25) // 25MB
			output, err := ioutil.TempFile("testdata", ".scratch")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(output.Name())

			err = client.Upload(taskID, runID, "public/single-part-gzip", input, output, true, false)
			if err != nil {
				t.Fatal(err)
			}
		})

		t.Run("public/multipart-identity", func(t *testing.T) {
			input := createInput(25) // 25MB
			output, err := ioutil.TempFile("testdata", ".scratch")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(output.Name())

			err = client.Upload(taskID, runID, "public/multipart-identity", input, output, false, true)
			if err != nil {
				t.Fatal(err)
			}
		})

		t.Run("public/multipart-gzip", func(t *testing.T) {
			input := createInput(25) // 25MB
			output, err := ioutil.TempFile("testdata", ".scratch")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(output.Name())

			err = client.Upload(taskID, runID, "public/multipart-gzip", input, output, true, true)
			if err != nil {
				t.Fatal(err)
			}
		})
	})

	t.Run("download-blob", func(t *testing.T) {
		var output bytes.Buffer
		err := client.Download(taskID, runID, "public/single-part-gzip", &output)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("download-blob-latest", func(t *testing.T) {
		var output bytes.Buffer
		err := client.DownloadLatest(taskID, "public/single-part-gzip", &output)
		if err != nil {
			t.Fatal(err)
		}
	})
}
