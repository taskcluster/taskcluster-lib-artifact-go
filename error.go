package artifact

import (
	"bytes"
	"fmt"
)

// Error is the node type in a linked list of errors interface{}s and instances
// of an internal type.  These errors are used to create a human readable trace
// of things which went wrong inside of this module.
type Error interface {
	// Error implements the error interface
	error
	// SuperError() returns the error or Error this Error wraps, or nil
	SuperError() error
	// Message() returns only the message for this Error and does no extra
	// formatting to create a human readable version of this error
	Message() string
}

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
		super: super,
		msg:   fmt.Sprintf(format, msg...),
	}
	return err
}

// This has to be a seperate function because we can only do type switches on
// values which are passed in as an interface.  Because the Error() receiver is
// getting passed in a self reference to a struct we cannot type switch on the
// receiver of a message
func magic(e error) string {
	var output bytes.Buffer
	var m string

	curErr := e

	for i := 1; curErr != nil; i++ {
		switch v := curErr.(type) {
		case artifactError:
			m = fmt.Sprintf("  %d. %s", i, v.msg)
			curErr = v.super
		case Error:
			m = fmt.Sprintf("  %d. %s", i, v.Message())
			curErr = v.SuperError()
		case error:
			m = fmt.Sprintf("  %d. %s", i, v.Error())
			curErr = nil
		default:
			curErr = nil
		}

		_, err := output.WriteString("\n" + m)

		// If we have an error in a write to an in memory buffer, let's assume
		// that's we're out of memory.  This is probably not a great idea, but I'd
		// like to make sure that errors in error handling are *really* noticeable
		if err != nil {
			panic(err)
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
