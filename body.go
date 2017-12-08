package artifact

import (
	"fmt"
	"io"
)

// A body is an abstraction we have for reading specific sections of a file
// with an offset.  This is done instead of using a SectionReader because
// there's some extra checks we want as well as being able to use things which
// aren't io.ReaderAts
type body struct {
	// The backing reader is the underlying io.ReadSeeker.  In the context of a
	// body which is linked to a file on the filesystem, this would be the
	// reference to an os.File which is what the reads will ultimately be
	// directed to.  This io.ReadSeeker will have .Seek() operations called on it
	// and it must be exclusively used by the body type.
	backingReader io.ReadSeeker
	// The limit reader is an io.LimitReader which ensures we only read up to
	// `size` bytes when reading from the backingReader
	limitReader io.Reader
	offset      int64
	size        int64
}

// Create a body.  A body is an io.Reader instance which reads from the file at
// filename, starting at the `offset`th byte and reading up to `size` bytes in
// total.
func newBody(input io.ReadSeeker, offset, size int64) (*body, error) {
	if size == 0 {
		return nil, newError(nil, "cannot specify a size of 0 for body")
	}

	b := body{input, nil, offset, size}

	err := b.Reset()
	if err != nil {
		return nil, newErrorf(err, "initializing for %s", findName(input))
	}

	return &b, nil
}

// Reset a body to its initial state.  This involves rewinding to the beginning
// and resetting the internal io.LimitReader that's used to read only a certain
// number of bytes.  This is to allow retrying of a file
func (b *body) Reset() error {
	if _, err := b.backingReader.Seek(b.offset, io.SeekStart); err != nil {
		return newErrorf(err, "seeking file %s to positiong %d", findName(b.backingReader), b.offset)
	}

	b.limitReader = io.LimitReader(b.backingReader, b.size)
	return nil
}

// Satisfy the io.Reader interface by reading from the associated file
func (b body) Read(p []byte) (int, error) {
	return b.limitReader.Read(p)
}

// Close a body and return relevant values back to their nil value
// TODO: I'm pretty sure that I don't need this function
func (b *body) Close() error {
	// If the backing reader happens to also support the Closer interface, we'll
	// propogate calls to it
	if closer, ok := b.backingReader.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			return newErrorf(err, "closing backing reader: %s", findName(closer))
		}
	}

	b.backingReader = nil
	b.limitReader = nil

	return nil
}

// Return a string representation of a Body for display
func (b body) String() string {
	return fmt.Sprintf("backing reader: %#v offset: %d size: %d\n", b.backingReader, b.offset, b.size)
}
