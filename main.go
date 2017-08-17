package main

import (
	"github.com/taskcluster/taskcluster-lib-artifact-go/hashfile"
)

func main() {
	hashfile.HashFile("main.go")
}
