package artifact

import (
	"errors"
	"io"
	"log"
	"os"
)

// By default, we're creating a logger which is used to print to standard
// output.  We are doing to ensure that by default, consumers of this library
// get useful logs for debugging.  We do, however, want to allow the users to
// change where logging should go, so we'll expose methods to change the
// behaviour of logging.
var logger = log.New(os.Stdout, "artifacts:", log.Ldate|log.Ltime|log.Lshortfile|log.LUTC)

// SetLogOutput will change the prefix used by logs in this package This is a
// simple convenience method to wrap this package's Logger instance's method.
// See: https://golang.org/pkg/log/#Logger.SetOutput
// If you wish to unconditionally supress all logs from this library, you can
// do the following:
//  SetLogOutput(ioutil.Discard)
func SetLogOutput(w io.Writer) {
	logger.SetOutput(w)
}

// SetLogPrefix will change the prefix used by logs in this package This is a
// simple convenience method to wrap this package's Logger instance's method.
// See: https://golang.org/pkg/log/#Logger.SetPrefix
func SetLogPrefix(p string) {
	logger.SetPrefix(p)
}

// SetLogFlags will change the flags which are used by logs in this package
// This is a simple convenience method to wrap this package's Logger instance's
// method.  See: https://golang.org/pkg/log/#Logger.SetFlags
func SetLogFlags(f int) {
	logger.SetFlags(f)
}

// SetLogger replaces the current logger with the logger specified
func SetLogger(l *log.Logger) error {
	if l == nil {
		return errors.New("new logger must be non-nil")
	}
	logger = l
	return nil
}
