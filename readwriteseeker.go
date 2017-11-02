package artifact

import (
	"io"
)

// Abstract inputs into this library so that we do not require files directly,
// instead only the actions which we actually depend on
type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}
