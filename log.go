package main

import (
	"io"
	"log"
	"os"
)

// By default, we're creating a logger which is used to print to the local
// file.  We are doing to ensure that by default, consumers of this library get
// useful logs for debugging.  We do, however, want to allow the users to
// change where logging should go, so we'll expose methods to change the
// behaviour of logging.
var logger = log.New(os.Stdout, "artifact:", log.Lshortfile|log.LUTC)

func SetLogOutput(w io.Writer) {
	logger.SetOutput(w)
}

func SetLogPrefix(p string) {
	logger.SetPrefix(p)
}

func SetLogFlags(f int) {
	logger.SetFlags(f)
}
