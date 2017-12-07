package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/taskcluster/slugid-go/slugid"
	tcclient "github.com/taskcluster/taskcluster-client-go"
	"github.com/taskcluster/taskcluster-client-go/queue"
	"github.com/urfave/cli"
)

type logger struct {
	t *testing.T
	n string
}

func (l *logger) Write(p []byte) (n int, err error) {
	l.t.Logf("STDOUT: %s", p)
	return len(p), nil
}

type testEnv struct {
	taskID         string
	runID          string
	inputFilename  string
	outputFilename string
	queue          *queue.Queue
	t              *testing.T
}

func (e testEnv) validate() {
	f1, err := ioutil.ReadFile(e.inputFilename)
	if err != nil {
		e.t.Fatal(err)
	}
	f2, err := ioutil.ReadFile(e.outputFilename)
	if err != nil {
		e.t.Fatal(err)
	}
	if !bytes.Equal(f1, f2) {
		e.t.Error("File %s and %s unexpectedly differ", e.inputFilename, e.outputFilename)
	}
}

func setup(t *testing.T) (testEnv, func()) {
	tEnv := testEnv{}
	taskID := slugid.Nice()
	taskGroupID := slugid.Nice()
	tEnv.taskID = taskID
	tEnv.t = t

	err := os.MkdirAll("testdata", 0777)
	if err != nil {
		t.Fatal(err)
	}

	input, err := ioutil.TempFile("testdata", "test-file-input")
	if err != nil {
		t.Fatal(err)
	}
	tEnv.inputFilename = input.Name()
	buf := make([]byte, 1024*1024)
	for i := 0; i < 10; i++ {
		_, err := rand.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		_, err = input.Write(buf)
		if err != nil {
			t.Fatal(err)
		}
	}
	input.Sync()
	err = input.Close()
	if err != nil {
		t.Fatal(err)
	}

	output, err := ioutil.TempFile("testdata", "test-file-output")
	if err != nil {
		t.Fatal(err)
	}
	tEnv.outputFilename = output.Name()
	output.Sync()
	err = output.Close()
	if err != nil {
		t.Fatal(err)
	}

	// This command creates a task that has a deadline in 15 minutes
	created := time.Now().UTC()
	// reset nanoseconds
	created = created.Add(time.Nanosecond * time.Duration(created.Nanosecond()*-1))
	// deadline in one hour' time
	deadline := created.Add(15 * time.Minute)
	// expiry in one day, in case we need test results
	expires := created.AddDate(0, 0, 2)

	taskDef := &queue.TaskDefinitionRequest{
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
		Tags:          json.RawMessage(`{"CI":"taskcluster-lib-artifact-go/cli"}`),
		Priority:      "lowest",
		TaskGroupID:   taskGroupID,
		WorkerType:    "my-workertype",
	}

	q, err := queue.New(nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = q.CreateTask(taskID, taskDef)
	if err != nil {
		t.Fatal(err)
	}

	tcr := queue.TaskClaimRequest{WorkerGroup: "my-worker-group", WorkerID: "my-worker"}
	tcres, err := q.ClaimTask(taskID, "0", &tcr)

	tEnv.runID = strconv.FormatInt(tcres.RunID, 10)
	tEnv.queue, err = queue.New(&tcclient.Credentials{
		ClientID:    tcres.Credentials.ClientID,
		AccessToken: tcres.Credentials.AccessToken,
		Certificate: tcres.Credentials.Certificate,
	})

	if err != nil {
		t.Fatal(err)
	}

	return tEnv, func() {
		err := os.Remove(tEnv.inputFilename)
		if err != nil {
			t.Error(err)
		}
		err = os.Remove(tEnv.outputFilename)
		if err != nil {
			t.Error(err)
		}
	}
}

func run(t *testing.T, args ...string) {
	fullargs := append([]string{"artifact", "-q"}, args...)
	err := _main(fullargs)
	if ecErr, ok := err.(*cli.ExitError); ok {
		if ecErr != nil {
			t.Fatal(ecErr)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
}

func badUsage(t *testing.T, args ...string) {
	t.Run(strings.Join(args, "_"), func(t *testing.T) {
		fullargs := append([]string{"artifact", "-q"}, args...)

		err := _main(fullargs)

		if err == nil {
			t.Fatalf("%s did not fail as expected", fullargs)
		}
	})
}

// All of these should fail because they're bad usage
func TestCLIUsage(t *testing.T) {
	e, teardown := setup(t)
	defer teardown()
	name := "public/cli-usage-test"

	// Obvious cases
	badUsage(t, "asdflasdf")
	badUsage(t, "download")
	badUsage(t, "upload")

	// Conflicting flags
	badUsage(t, "upload", e.taskID, e.runID, name, "--input", e.inputFilename, "--multipart", "--single-part")

	// Missing mandatory flag
	badUsage(t, "upload", e.taskID, e.runID, name)
	badUsage(t, "download", e.taskID, e.runID, name)

	// Wrong flags
	badUsage(t, "upload", e.taskID, e.runID, name, "--output", e.inputFilename)
	badUsage(t, "download", e.taskID, e.runID, name, "--input", e.outputFilename)
	badUsage(t, "download", "--url", "--latest", "--output", e.outputFilename)
}

func TestCorruptedDownloads(t *testing.T) {

	e, teardown := setup(t)
	defer teardown()

	fi, err := os.Stat(e.inputFilename)
	if err != nil {
		t.Fatal(err)
	}

	var ts *httptest.Server

	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/the-redirect" {
			w.Header().Set("location", ts.URL+"/the-redirect")
			w.WriteHeader(302)
		} else {
			w.Header().Set("x-amz-meta-content-length", strconv.FormatInt(fi.Size(), 10))
			w.Header().Set("x-amz-meta-content-sha256", "invalid")
			w.Header().Set("x-amz-meta-content-length", strconv.FormatInt(fi.Size(), 10))
			w.Header().Set("x-amz-meta-content-sha256", "invalid")
			w.Header().Set("content-encoding", "identity")
			w.WriteHeader(200)

			b, err := ioutil.ReadFile(e.inputFilename)
			if err != nil {
				t.Errorf(err.Error())
			}
			w.Write(b)
		}
	}))
	defer ts.Close()

	args := []string{
		"artifact",
		"-q",
		"--allow-insecure-requests",
		"--base-url",
		ts.URL,
		"download",
		e.taskID,
		"cli-corrupt-test",
		"--latest",
		"--output",
		e.outputFilename,
	}
	err = _main(args)

	if ecErr, ok := err.(cli.ExitCoder); ok {
		code := ecErr.ExitCode()
		if code != ErrCorrupt {
			t.Fatalf("Error code %d from %v was not expected %d", code, args, ErrCorrupt)
		}
	} else {
		t.Fatalf("Error %v not expected for %v", err, args)
	}
}

func TestCLIRuns(t *testing.T) {
	e, teardown := setup(t)
	defer teardown()

	// validateUploadOptions tests the downloaded file of an upload with the
	// given options is unaltered from the original file
	validateUploadOptions := func(name string, uploadOptions ...string) {
		t.Run(name, func(t *testing.T) {
			name := "public/" + name
			upargs := []string{"upload", e.taskID, e.runID, name, "--input", e.inputFilename}
			upargs = append(upargs, uploadOptions...)
			run(t, upargs...)
			run(t, "download", e.taskID, e.runID, name, "--output", e.outputFilename)
			e.validate()
			run(t, "download", e.taskID, name, "--latest", "--output", e.outputFilename)
			e.validate()
		})
	}

	validateUploadOptions("auto-identity") // no upload options
	validateUploadOptions("auto-gzip", "--gzip")
	validateUploadOptions("single-part-identity", "--single-part")
	validateUploadOptions("single-part-gzip", "--single-part", "--gzip")
	validateUploadOptions("multipart-identity", "--multipart")
	validateUploadOptions("multipart-gzip", "--multipart", "--gzip")

	t.Run("downloading a url", func(t *testing.T) {
		name := "public/downloading-url"
		url, err := e.queue.GetArtifact_SignedURL(e.taskID, e.runID, name, time.Duration(3)*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		run(t, "upload", e.taskID, e.runID, name, "--input", e.inputFilename)
		run(t, "download", "--url", url.String(), "--output", e.outputFilename)
		e.validate()
	})

	t.Run("download to stdout", func(t *testing.T) {
		name := "public/small"
		filename := "./testdata/small"
		of, err := os.Create(filename)
		if err != nil {
			t.Fatal(err)
		}
		of.WriteString("Testing writing downloads to STDOUT.  Beep Boop.\n")
		of.Close()
		defer os.Remove(filename)

		run(t, "upload", e.taskID, e.runID, name, "--gzip", "--input", filename)
		// Unfortunately this will actually write to standard output.  I don't want
		// to intercept writing to the real standard output, because os.Stdout
		// behaves in a very specific way.  Basically, I just want to make sure no
		// errors are thrown.  Patches welcome!  The specific case that I'm
		// concerned with is that os.Stdout is an io.Seeker, but all calls to
		// Seek() on it immediately fail.  There's probably other things, but this
		// is the minimum that's different
		run(t, "download", e.taskID, e.runID, name, "--output", "-")
	})
}
