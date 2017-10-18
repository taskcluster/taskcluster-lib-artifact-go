package artifact

import (
	"io"
)

// A ReadSeekCloser is the minimum we need to perform the work
type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}
