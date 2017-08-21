package main

import (
	"fmt"
	"github.com/taskcluster/taskcluster-lib-artifact-go/prepare"
	"github.com/taskcluster/taskcluster-lib-artifact-go/runner"
)

func main() {
	/*identityUpload := prepare.NewSinglePartUpload("borap.mp3")
	fmt.Printf("%+v\n", identityUpload)
	gzipUpload := prepare.NewGzipSinglePartUpload("borap.mp3")
	fmt.Printf("%+v\n", gzipUpload)
	identityMUpload := prepare.NewMultiPartUpload("borap.mp3")
	fmt.Printf("%s\n", identityMUpload)
  for _, x := range identityMUpload.Parts {
    fmt.Printf("%s\n", x)
  }*/
	gzipMUpload := prepare.NewGzipMultiPartUpload("borap.mp3")
	fmt.Printf("%+v\n", gzipMUpload)
  for _, x := range gzipMUpload.Parts {
    fmt.Printf("%s\n", x)
  }

  headers := make(runner.Headers)
  headers["User-Agent"] = "taskcluster-lib-artifact-go"
  request := runner.Request{"http://www.google.com", "GET", headers}
  fmt.Printf("%+v\n", request)
}
