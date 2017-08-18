package main

import (
	"fmt"
	"github.com/taskcluster/taskcluster-lib-artifact-go/prepare"
)

func main() {
  identityUpload := prepare.NewSinglePartUpload("borap.mp3")
  fmt.Printf("%+v\n", identityUpload)
  gzipUpload := prepare.NewGzipSinglePartUpload("borap.mp3")
  fmt.Printf("%+v\n", gzipUpload)

  identityMUpload := prepare.NewMultiPartUpload("borap.mp3")
  fmt.Printf("%+v\n", identityMUpload)
  gzipMUpload := prepare.NewGzipMultiPartUpload("borap.mp3")
  fmt.Printf("%+v\n", gzipMUpload)
}
