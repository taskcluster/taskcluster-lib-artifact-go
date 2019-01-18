package artifact

import (
	"errors"
	"net/url"
	"testing"
)

func testError(t *testing.T, actual error, expected string) {
	if actual.Error() != expected {
		t.Errorf("FAIL!\n'%s'\ndoes not match expectation\n'%s'\n", actual.Error(), expected)
	} else {
		t.Logf("Output: '%s'", actual.Error())
	}
}

func TestErrorNoSuper(t *testing.T) {
	err := newError(nil, "Error")
	testError(t, err, "Artifact Error:\n  1. (internal) Error")
}

func TestErrorfNoSuper(t *testing.T) {
	err := newErrorf(nil, "%s", "Formatted Error")
	testError(t, err, "Artifact Error:\n  1. (internal) Formatted Error")
}

func TestErrorIsPassableAsStdError(t *testing.T) {
	err := newError(nil, "Error")
	switch v := err.(type) {
	case error:
	default:
		t.Errorf("%T is not the error interface", v)
	}
}

func TestErrorsPlainErrorSuper(t *testing.T) {
	err := newError(errors.New("error"), "Error")
	testError(t, err, "Artifact Error:\n  1. (internal) Error\n  2. (*errors.errorString) error")
}

func TestErrorsSuperWithSuper(t *testing.T) {
	actual := func() error {
		return newError(func() error {
			return newError(func() error {
				return newError(func() error {
					return errors.New("error")
				}(), "Error1")
			}(), "Error2")
		}(), "Error3")
	}()

	testError(t, actual, "Artifact Error:\n  1. (internal) Error3\n  2. (internal) Error2\n  3. (internal) Error1\n  4. (*errors.errorString) error")
}

func TestErrorsURLErrorWrapsNonInternalError(t *testing.T) {
	err := errors.New("innermost")
	urlErr := &url.Error{Op: "Op", URL: "URL", Err: err}
	err = newError(urlErr, "outermost")
	testError(t, err, "Artifact Error:\n  1. (internal) outermost\n  2. (*url.Error) FAIL Op URL\n  3. (*errors.errorString) innermost")

}

func TestErrorsURLErrorWrapsInternalError(t *testing.T) {
	err := errors.New("innermost")
	urlErr := &url.Error{Op: "Op", URL: "URL", Err: newError(err, "wrapped")}
	err = newError(urlErr, "outermost")
	testError(t, err, "Artifact Error:\n  1. (internal) outermost\n  2. (*url.Error) FAIL Op URL\n  3. (internal) wrapped\n  4. (*errors.errorString) innermost")

}

/*  DISABLED BECAUSE I DONT WANT TO MAKE A CALL SUMMARY BY HAND
func TestErrorsTCErrorWrapsNonInternalError(t *testing.T) {
	err := errors.New("innermost")
	urlErr := &tcclient.APICallException{&tcclient.CallSummary{
		HTTPRequest: &http.Request{},
	}, err}
	err = newError(urlErr, "outermost")
	testError(t, err, "Artifact Error:\n  1. (internal) outermost\n  2. (*url.Error) FAIL Op URL\n  3. (*errors.errorString) innermost")

}

func TestErrorsTCErrorWrapsInternalError(t *testing.T) {
	err := errors.New("innermost")
	urlErr := &tcclient.APICallException{&tcclient.CallSummary{
		HTTPRequest: &http.Request{},
	}, newError(err, "wrapped")}
	err = newError(urlErr, "outermost")
	testError(t, err, "Artifact Error:\n  1. (internal) outermost\n  2. (*url.Error) FAIL Op URL\n  3. (internal) wrapped\n  4. (*errors.errorString) innermost")

}
*/
