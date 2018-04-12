package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/alecthomas/units"
	tcclient "github.com/taskcluster/taskcluster-client-go"
	"github.com/taskcluster/taskcluster-client-go/tcqueue"
	artifact "github.com/taskcluster/taskcluster-lib-artifact-go"
	"github.com/urfave/cli"
)

// These are exit code constants.  They're roughly mapped to the values in
// sysexits.h, but without the granularity availabe in the definitions of that
// file.  We care about distinguishing between errors which are due to bad
// usage and should not be retried ever, errors which are unexplained internal
// issues and should be retried, and errors which are because of corruption.
// We specifically have a corruption case because corruption might need to be
// handled differently than other errors and so is helpful to be easy to detect
const (
	ErrInternal = 70 // EX_SOFTWARE
	ErrCorrupt  = 65 // EX_DATAERR
)

func main() {
	err := _main(os.Args)
	if err == nil {
		os.Exit(0)
	}

	if ecErr, ok := err.(cli.ExitCoder); ok {
		os.Exit(ecErr.ExitCode())
	}

	os.Exit(ErrInternal)
}

func _main(args []string) error {
	// We're going to take care of exiting ourselves
	cli.OsExiter = func(c int) {}

	app := cli.NewApp()

	app.Name = "artifact"
	app.Version = "0.0.1"
	app.Usage = "interact with taskcluster artifacts"

	app.OnUsageError = func(c *cli.Context, err error, isSubcommand bool) error {
		return cli.NewExitError(err.Error(), ErrInternal)
	}

	app.Action = func(c *cli.Context) error {
		cli.ShowAppHelp(c)
		if c.NArg() == 0 {
			return cli.NewExitError("Must specify command", ErrInternal)
		}
		return cli.NewExitError(fmt.Sprintf("%s is not a command", c.Args().Get(0)), ErrInternal)
	}

	app.OnUsageError = func(context *cli.Context, err error, isSubcommand bool) error {
		return cli.NewExitError(err.Error(), ErrInternal)
	}

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
			Name:   "base-url",
			EnvVar: "QUEUE_BASE_URL",
			Usage:  "set queue's `BASE_URL`",
			Value:  tcqueue.DefaultBaseURL,
		},
		cli.StringFlag{
			Name:   "chunk-size",
			Usage:  "set the I/O chunk size to `CHUNK_SIZE`",
			Value:  fmt.Sprintf("%d KB", artifact.DefaultChunkSize),
			EnvVar: "ARTIFACT_CHUNK_SIZE",
		},
		cli.StringFlag{
			Name:   "part-size",
			Usage:  "set the I/O chunk size to `PART_SIZE`",
			Value:  fmt.Sprintf("%d MB", artifact.DefaultPartSize*artifact.DefaultChunkSize),
			EnvVar: "ARTIFACT_PART_SIZE",
		},
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "supress debugging output",
		},
		cli.BoolFlag{
			Name:  "allow-insecure-requests",
			Usage: "allow insecure (http) requests. NOT RECOMMENDED",
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
					Usage:  "`FILENAME` to write output to",
					EnvVar: "ARTIFACT_OUTPUT",
				},
				cli.BoolFlag{
					Name:  "latest",
					Usage: "request artifact from latest run",
				},
				cli.StringFlag{
					Name:   "url",
					Usage:  "use a raw Queue URL instead of specifying taskid, runid or name",
					EnvVar: "ARTIFACT_URL",
				},
			},
			ArgsUsage: "taskId runId name",
			Action: func(c *cli.Context) error {
				var err error
				if c.IsSet("latest") && c.IsSet("url") {
					return cli.NewExitError("Cannot specify --latest and --url", ErrInternal)
				}

				q := tcqueue.New(&tcclient.Credentials{
					ClientID:    c.GlobalString("client-id"),
					AccessToken: c.GlobalString("access-token"),
					Certificate: c.GlobalString("certificate"),
				})

				if c.GlobalIsSet("base-url") {
					q.BaseURL = c.GlobalString("base-url")
				}

				client := artifact.New(q)

				if c.GlobalIsSet("chunk-size") {
					cz, err := units.ParseBase2Bytes(c.String("chunk-size"))
					if err != nil {
						return cli.NewExitError(err.Error(), ErrInternal)
					}
					_, ps := client.GetInternalSizes()
					err = client.SetInternalSizes(int(cz), ps)
					if err != nil {
						return cli.NewExitError(err.Error(), ErrInternal)
					}
				}

				if c.GlobalBool("allow-insecure-requests") {
					client.AllowInsecure = true
				}

				if c.GlobalBool("quiet") {
					artifact.SetLogOutput(ioutil.Discard)
				}

				if !c.IsSet("output") {
					return cli.NewExitError("must specify output", ErrInternal)
				}

				var output *os.File

				if c.String("output") != "-" {
					output, err = os.Create(c.String("output"))
					if err != nil {
						return cli.NewExitError(err.Error(), ErrInternal)
					}
					defer output.Close()
				} else {
					output = os.Stdout
				}

				if c.IsSet("url") {
					if c.NArg() != 0 {
						msg := fmt.Sprintf("--url requires zero arguments, received %v", c.Args())
						return cli.NewExitError(msg, ErrInternal)
					}
					err = client.DownloadURL(c.String("url"), output)
				} else if c.Bool("latest") {
					if c.NArg() != 2 {
						msg := fmt.Sprintf("--latest requires two arguments, received %v", c.Args())
						return cli.NewExitError(msg, ErrInternal)
					}
					err = client.DownloadLatest(c.Args().Get(0), c.Args().Get(1), output)
				} else {
					if c.NArg() != 3 {
						msg := fmt.Sprintf("three arguments, received %v", c.Args())
						return cli.NewExitError(msg, ErrInternal)
					}
					err = client.Download(c.Args().Get(0), c.Args().Get(1), c.Args().Get(2), output)

				}

				if err == artifact.ErrCorrupt {
					return cli.NewExitError(err.Error(), ErrCorrupt)
				}

				return err
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
					Name:  "gzip",
					Usage: "serve artifact with gzip content-encoding",
				},
				cli.BoolFlag{
					Name:  "multipart",
					Usage: "force multipart upload",
				},
				cli.BoolFlag{
					Name:  "single-part",
					Usage: "force single part upload",
				},
				cli.StringFlag{
					Name:  "multipart-part-size",
					Usage: "number of bytes before starting to use multipart uploads",
					Value: "250 MB",
				},
			},
			ArgsUsage: "taskId runId name",
			Action: func(c *cli.Context) error {
				var err error

				q := tcqueue.New(&tcclient.Credentials{
					ClientID:    c.GlobalString("client-id"),
					AccessToken: c.GlobalString("access-token"),
					Certificate: c.GlobalString("certificate"),
				})

				client := artifact.New(q)

				if c.GlobalBool("quiet") {
					artifact.SetLogOutput(ioutil.Discard)
				}

				var gzip bool
				var mp bool

				if c.Bool("single-part") && c.Bool("multipart") {
					return cli.NewExitError("can only force single or multi part", ErrInternal)
				}

				if c.Bool("gzip") {
					gzip = true
				}

				if c.Bool("single-part") {
					mp = false
				} else if c.Bool("multipart") {
					mp = true
				} else {
					if fi, err := os.Stat(c.String("input")); err != nil {
						if os.IsNotExist(err) {
							return cli.NewExitError("input does not exist", ErrInternal)
						}
						if err != nil {
							return cli.NewExitError(err.Error(), ErrInternal)
						}
						mpsize, err := units.ParseBase2Bytes(c.String("multipart-size"))
						if err != nil {
							return err
						}
						if fi.Size() >= int64(mpsize) {
							mp = true
						}
					}
				}

				if c.GlobalIsSet("chunk-size") {
					cz, err := units.ParseBase2Bytes(c.String("chunk-size"))
					if err != nil {
						return cli.NewExitError(err.Error(), ErrInternal)
					}
					_, ps := client.GetInternalSizes()
					err = client.SetInternalSizes(int(cz), ps)
					if err != nil {
						return cli.NewExitError(err.Error(), ErrInternal)
					}
				}

				if c.GlobalIsSet("part-size") {
					ps, err := units.ParseBase2Bytes(c.String("part-size"))
					if err != nil {
						return cli.NewExitError(err.Error(), ErrInternal)
					}
					cz, _ := client.GetInternalSizes()
					err = client.SetInternalSizes(cz, int(ps))
					if err != nil {
						return cli.NewExitError(err.Error(), ErrInternal)
					}
				}

				if !c.IsSet("input") {
					return cli.NewExitError("must specify input", ErrInternal)
				}

				input, err := os.Open(c.String("input"))
				if err != nil {
					return cli.NewExitError(err.Error(), ErrInternal)
				}
				defer input.Close()

				output, err := ioutil.TempFile(c.String("tmp-dir"), "tc-artifact")
				if err != nil {
					return cli.NewExitError(err.Error(), ErrInternal)
				}
				defer func() {
					output.Close()
					os.Remove(output.Name())
				}()

				if c.NArg() != 3 {
					msg := fmt.Sprintf("three arguments, received %v", c.Args())
					return cli.NewExitError(msg, ErrInternal)
				}
				err = client.Upload(c.Args().Get(0), c.Args().Get(1), c.Args().Get(2), input, output, gzip, mp)

				if err == artifact.ErrCorrupt {
					return cli.NewExitError(err.Error(), ErrCorrupt)
				}

				return err
			},
			Category: "Uploading",
		},
	}

	return app.Run(args)
}
