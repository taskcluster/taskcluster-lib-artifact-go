package main

import (
	"fmt"
	"github.com/taskcluster/taskcluster-lib-artifact-go/prepare"
)

func main() {
	fmt.Printf("%+v\n", prepare.HashFileParts("borap.mp3", 1024, 1024))
	fmt.Printf("%+v\n", prepare.HashFile("borap.mp3", 1024))
}
