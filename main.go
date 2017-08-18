package main

import (
	"fmt"
	"github.com/taskcluster/taskcluster-lib-artifact-go/hashfile"
)

func main() {
	fmt.Printf("%+v\n", hashfile.HashFileParts("borap.mp3", 1024, 1024))
}
