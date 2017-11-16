package artifact

import (
	"bytes"
	"fmt"
	"net/url"
)

type artifactError struct {
	msg   string
	super error
}

func newError(super error, msg string) error {
	err := artifactError{
		super: super,
		msg:   msg,
	}
	return error(err)
}

func newErrorf(super error, format string, msg ...interface{}) error {
	err := artifactError{
		super: error(super),
		msg:   fmt.Sprintf(format, msg...),
	}
	return error(err)
}

// This has to be a seperate function because we can only do type switches on
// values which are passed in as an interface.  Because the Error() receiver is
// getting passed in a self reference to a struct we cannot type switch on the
// receiver of a message
func magic(e error) string {
	var output bytes.Buffer

	curErr := e

	w := func(f string, a ...interface{}) {
		_, err := output.WriteString("\n" + fmt.Sprintf(f, a...))
		if err != nil {
			panic(err)
		}
	}

	for i := 1; curErr != nil; i++ {

		// Since this error starts in our library, then moves into the
		// standard HTTP library, anything from a url.Error starts a new
		// set of printing.  Go error handling is atrocious!
		//
		// We're going to special case this.  The downside is that non-url.Error
		// instances don't get this nicety, but the upside is that they still
		// will have useful error messages.  I checked the stdlib and it looks
		// like this is the only relevant special case
		switch v := curErr.(type) {
		case *url.Error:
			if _, ok := v.Err.(artifactError); ok {
				w("  %d. (%T) FAIL %s %s", i, v, v.Op, v.URL)
				curErr = v.Err
			} else {
				w("  %d. (%T) FAIL %s %s", i, v, v.Op, v.URL)
				i++
				w("  %d. (%T) %s", i, v.Err, v.Err.Error())
				curErr = nil
			}
		case artifactError:
			w("  %d. (internal) %s", i, v.Message())
			curErr = v.SuperError()
		default:
			w("  %d. (%T) %s", i, curErr, curErr.Error())
			curErr = nil
		}
	}

	return output.String()

}

func (e artifactError) Error() string {
	return fmt.Sprintf("Artifact Error:%s", magic(e))
}

func (e artifactError) SuperError() error {
	return e.super
}

func (e artifactError) Message() string {
	return e.msg
}
