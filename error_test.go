package artifact

import (
	"errors"
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
	testError(t, err, "Artifact Error:\n  1. Error")
}

func TestErrorfNoSuper(t *testing.T) {
	err := newErrorf(nil, "%s", "Formatted Error")
	testError(t, err, "Artifact Error:\n  1. Formatted Error")
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
	testError(t, err, "Artifact Error:\n  1. Error\n  2. error")
}

func TestErrorsSuperWithSuper(t *testing.T) {
	var actual error // ensure exact type
	actual = func() error {
		return newError(func() error {
			return newError(func() error {
				return newError(func() error {
					return errors.New("error")
				}(), "Error1")
			}(), "Error2")
		}(), "Error3")
	}()

	/*err := newError(errors.New("error"), "Error1")
	err = newError(err, "Error2")
	err = newError(err, "Error3")*/
	testError(t, actual, "Artifact Error:\n  1. Error3\n  2. Error2\n  3. Error1\n  4. error")
}
