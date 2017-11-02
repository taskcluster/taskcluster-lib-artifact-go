package artifact

import (
	"errors"
	"fmt"
	"io"
)

type body struct {
	backingReader io.ReadSeeker
	limitReader   io.Reader
	offset        int64
	size          int64
}

// Create a body.  A body is an io.Reader instance which reads from the file at
// filename, starting at the `offset`th byte and reading up to `size` bytes in
// total.
func newBody(input io.ReadSeeker, offset, size int64) (*body, error) {
	if size == 0 {
		return nil, errors.New("cannot specify a size of 0")
	}

	b := body{input, nil, offset, size}

	err := b.Reset()
	if err != nil {
		return nil, err
	}

	return &b, nil
}

// Reset a body to its initial state.  This involves rewinding to the beginning
// and resetting the internal io.LimitReader that's used to read only a certain
// number of bytes.  This is to allow retrying of a file
func (b *body) Reset() error {
	if _, err := b.backingReader.Seek(b.offset, 0); err != nil {
		return err
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
			return err
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
