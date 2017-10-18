package artifact

import (
	"io"
	"log"
	"os"
)

// By default, we're creating a logger which is used to print to standard
// output.  We are doing to ensure that by default, consumers of this library
// get useful logs for debugging.  We do, however, want to allow the users to
// change where logging should go, so we'll expose methods to change the
// behaviour of logging.
var logger = log.New(os.Stdout, "artifact:", log.Lshortfile|log.LUTC)

// SetLogOutput will change the io.Writer which log messages should be written
// to
func SetLogOutput(w io.Writer) {
	logger.SetOutput(w)
}

// SetLogPrefix will change the prefix used by this logger
func SetLogPrefix(p string) {
	logger.SetPrefix(p)
}

// SetLogFlags will change the flags which are used by this logger
func SetLogFlags(f int) {
	logger.SetFlags(f)
}
