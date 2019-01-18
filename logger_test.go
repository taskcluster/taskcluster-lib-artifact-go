package artifact

import (
	"testing"
)

// This is so that unit tests can get the logging output instead of logging
// directly to stdout
type unitTestLogWriter struct {
	t *testing.T
}

func newUnitTestLogWriter(t *testing.T) unitTestLogWriter {
	SetLogPrefix("")
	return unitTestLogWriter{t: t}
}

func (utlw unitTestLogWriter) Write(p []byte) (n int, err error) {
	utlw.t.Logf("%s", p)
	return len(p), nil
}
