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
var logger = log.New(os.Stdout, "artifact:", log.LUTC)

// SetLogOutput will change the io.Writer which log messages should be written
// to.  By default, logs are written to os.Stdout to ensure that consumers of
// this library get useful logs.  If you don't want logs at all, set the log
// output to ioutil.Discard
func SetLogOutput(w io.Writer) {
	logger.SetOutput(w)
}

// SetLogPrefix will change the prefix used by logs in this package
func SetLogPrefix(p string) {
	logger.SetPrefix(p)
}

// SetLogFlags will change the flags which are used by logs in this package
func SetLogFlags(f int) {
	logger.SetFlags(f)
}
