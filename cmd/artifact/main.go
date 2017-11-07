package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/taskcluster/slugid-go/slugid"
	tcclient "github.com/taskcluster/taskcluster-client-go"
	"github.com/taskcluster/taskcluster-client-go/queue"
	artifact "github.com/taskcluster/taskcluster-lib-artifact-go"
	"github.com/urfave/cli"
)

const (
	ErrInternal = 1
	ErrBadUsage = 2
	ErrCorrupt  = 13
)

// TODO implement an in memory 'file'
// TODO implement 'redirect' and 'error' artifact types?

// It would be nice if this were exposed as a package variable instead
func findQueueDefaultBaseUrl() string {
	q := queue.New(&tcclient.Credentials{})
	return q.BaseURL
}

func main() {
	app := cli.NewApp()

	app.Name = "artifact"
	app.Version = "0.0.1"
	app.Usage = "interact with taskcluster artifacts"

	app.OnUsageError = func(c *cli.Context, err error, isSubcommand bool) error {
		if isSubcommand {
			return err
		}

		fmt.Fprintf(c.App.Writer, "WRONG: %#v\n", err)
		return nil
	}

	baseURL := findQueueDefaultBaseUrl()

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "client-id",
			EnvVar: "TASKCLUSTER_CLIENT_ID",
			Usage:  "set client id to `CLIENT_ID`",
		},
		cli.StringFlag{
			Name:   "access-token",
			EnvVar: "TASKCLUSTER_ACCESS_TOKEN",
			Usage:  "set access token to `ACCESS_TOKEN`",
		},
		cli.StringFlag{
			Name:   "certificate",
			EnvVar: "TASKCLUSTER_CERTIFICATE",
			Usage:  "set certificate to `CERTIFICATE`",
		},
		cli.StringFlag{
			Name: "base-url",
			//EnvVar: "QUEUE_BASE_URL",
			Usage: "set queue's `BASE_URL`",
			Value: baseURL,
		},
		cli.IntFlag{
			Name:   "chunk-size",
			Usage:  "set the I/O chunk size to `CHUNK_SIZE` KB",
			Value:  artifact.DefaultChunkSize / 1024,
			EnvVar: "ARTIFACT_CHUNK_SIZE",
		},
		cli.IntFlag{
			Name:   "part-size",
			Usage:  "set the I/O chunk size to `PART_SIZE` MB",
			Value:  artifact.DefaultPartSize * artifact.DefaultChunkSize / 1024 / 1024,
			EnvVar: "ARTIFACT_PART_SIZE",
		},
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "supress debugging output",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:    "download",
			Aliases: []string{"d"},
			Usage:   "download an artifact",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:   "output, o",
					Usage:  "`FILENAME` to write output to.  If not provided, Standard Output will be assumed",
					EnvVar: "ARTIFACT_OUTPUT",
				},
				cli.BoolFlag{
					Name:  "latest",
					Usage: "request artifact from latest run",
				},
			},
			ArgsUsage: "taskId runId name",
			Action: func(c *cli.Context) error {
				var err error
				q := queue.New(&tcclient.Credentials{
					ClientID:    c.GlobalString("client-id"),
					AccessToken: c.GlobalString("access-token"),
					Certificate: c.GlobalString("certificate"),
				})

				if c.GlobalIsSet("base-url") {
					q.BaseURL = c.GlobalString("base-url")
				}

				client := artifact.New(q)

				fmt.Printf("%+v\n", c.Args())

				if c.Bool("quiet") {
					artifact.SetLogOutput(ioutil.Discard)
				}

				if c.GlobalIsSet("chunk-size") {
					cz := c.GlobalInt("chunk-size")
					_, ps := client.GetInternalSizes()
					err := client.SetInternalSizes(cz*1024, ps)
					if err != nil {
						return cli.NewExitError(err, ErrBadUsage)
					}
				}

				if !c.IsSet("output") {
					return cli.NewExitError("must specify output", ErrBadUsage)
				}

				output, err := os.Create(c.String("output"))
				if err != nil {
					return cli.NewExitError(err, ErrInternal)
				}
				defer output.Close()

				if c.Bool("latest") {
					if c.NArg() != 2 {
						return cli.NewExitError("--latest requires two arguments", ErrBadUsage)
					}
					err = client.DownloadLatest(c.Args().Get(0), c.Args().Get(1), output)
				} else {
					if c.NArg() != 3 {
						return cli.NewExitError("three arguments required", 1)
					}
					err = client.Download(c.Args().Get(0), c.Args().Get(1), c.Args().Get(2), output)

				}

				switch err {
				case nil:
					return nil
				case artifact.ErrBadOutputWriter:
					return cli.NewExitError(err, ErrBadUsage)
				case artifact.ErrCorrupt:
					return cli.NewExitError(err, ErrCorrupt)
				default:
					return cli.NewExitError(err, ErrInternal)
				}
			},
			Category: "Downloading",
		},
		{
			Name:    "upload",
			Aliases: []string{"u"},
			Usage:   "upload an artifact",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:   "input, i",
					Usage:  "`FILENAME` to read as artifact.  Must be seekable",
					EnvVar: "ARTIFACT_INPUT",
				},
				cli.StringFlag{
					Name:   "tmp-dir",
					Usage:  "`DIRECTORY` to write temporary files in",
					EnvVar: "ARTIFACT_TMPDIR",
				},
				cli.BoolFlag{
					Name:  "multi-part",
					Usage: "force multipart upload",
				},
				cli.BoolFlag{
					Name:  "single-part",
					Usage: "force single part upload",
				},
				cli.Int64Flag{
					Name:  "multi-part-size",
					Usage: "number of `MB` before starting to use multi-part uploads",
					Value: 250,
				},
			},
			ArgsUsage: "taskId runId name",
			Action: func(c *cli.Context) error {
				var err error
				client := artifact.New(queue.New(&tcclient.Credentials{
					ClientID:    c.GlobalString("client-id"),
					AccessToken: c.GlobalString("access-token"),
					Certificate: c.GlobalString("certificate"),
				}))

				fmt.Printf("%+v\n", c.Args())

				if c.Bool("quiet") {
					artifact.SetLogOutput(ioutil.Discard)
				}

				var gzip bool
				var mp bool

				if c.Bool("single-part") && c.Bool("multi-part") {
					return cli.NewExitError("can only force single or multi part", ErrBadUsage)
				}

				if c.Bool("single-part") {
					mp = false
				} else if c.Bool("multi-part") {
					mp = true
				} else {
					if fi, err := os.Stat(c.String("input")); err != nil {
						if os.IsNotExist(err) {
							return cli.NewExitError("input does not exist", ErrBadUsage)
						}
						if err != nil {
							return cli.NewExitError(err, ErrInternal)
						}
						mpsize := c.Int64("multi-part-size")
						if fi.Size() >= mpsize*1024*1024 {
							mp = true
						}
					}
				}

				if c.GlobalIsSet("chunk-size") {
					cz := c.GlobalInt("chunk-size")
					_, ps := client.GetInternalSizes()
					err := client.SetInternalSizes(cz*1024, ps)
					if err != nil {
						return cli.NewExitError(err, ErrBadUsage)
					}
				}

				if c.GlobalIsSet("part-size") {
					ps := c.GlobalInt("part-size")
					cz, _ := client.GetInternalSizes()
					err := client.SetInternalSizes(cz, ps*1024*1024)
					if err != nil {
						return cli.NewExitError(err, ErrBadUsage)
					}
				}

				if !c.IsSet("input") {
					return cli.NewExitError("must specify input", ErrBadUsage)
				}

				input, err := os.Open(c.String("input"))
				if err != nil {
					return cli.NewExitError(err, ErrBadUsage)
				}
				defer input.Close()

				output, err := ioutil.TempFile(c.String("tmp-dir"), "tc-artifact")
				if err != nil {
					return cli.NewExitError(err, ErrInternal)
				}
				defer output.Close()
				defer os.Remove(output.Name())

				if c.NArg() != 3 {
					return cli.NewExitError("three arguments required", 1)
				}
				err = client.Upload(c.Args().Get(0), c.Args().Get(1), c.Args().Get(2), input, output, gzip, mp)

				switch err {
				case nil:
					return nil
				case artifact.ErrBadOutputWriter:
					return cli.NewExitError(err, ErrBadUsage)
				case artifact.ErrCorrupt:
					return cli.NewExitError(err, ErrCorrupt)
				default:
					return cli.NewExitError(err, ErrInternal)
				}
			},
			Category: "Uploading",
		},
		{
			Name:  "create-task",
			Usage: "upload an artifact (only for testing)",
			Action: func(c *cli.Context) error {
				taskID := slugid.Nice()
				taskGroupID := slugid.Nice()

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
					Tags:          json.RawMessage(`{"CI":"taskcluster-lib-artifact-go"}`),
					Priority:      "lowest",
					TaskGroupID:   taskGroupID,
					WorkerType:    "my-workertype",
				}

				creds := &tcclient.Credentials{
					ClientID:    c.GlobalString("client-id"),
					AccessToken: c.GlobalString("access-token"),
					Certificate: c.GlobalString("certificate"),
				}

				q := queue.New(creds)

				_, err := q.CreateTask(taskID, taskDef)
				if err != nil {
					return cli.NewExitError(err, ErrInternal)
				}

				tcr := queue.TaskClaimRequest{WorkerGroup: "my-worker-group", WorkerID: "my-worker"}
				tcres, err := q.ClaimTask(taskID, "0", &tcr)
				if err != nil {
					return cli.NewExitError(err, ErrInternal)
				}

				fmt.Printf("export TASKCLUSTER_CLIENT_ID=\"%s\" TASKCLUSTER_ACCESS_TOKEN=\"%s\" TASKCLUSTER_CERTIFICATE=\"%s\" TASKID=\"%s\" RUNID=\"%d\"",
					tcres.Credentials.ClientID,
					tcres.Credentials.AccessToken,
					strings.Replace(tcres.Credentials.Certificate, "\"", "\\\"", -1),
					taskID,
					tcres.RunID,
				)
				return nil
			},
			Category: "Testing",
		},
	}

	app.Run(os.Args)
}
