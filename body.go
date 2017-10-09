package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

type body struct {
	File   *os.File
	Reader io.Reader
	Offset int64
	Size   int64
}

// Create a body.  A body is an io.Reader instance which reads from the file at
// filename, starting at the `offset`th byte and reading up to `size` bytes in
// total.
func newBody(filename string, offset, size int64) (*body, error) {
	if size == 0 {
		return nil, errors.New("Cannot specify a size of 0")
	}

	file, err := os.Open(filename)

	if err != nil {
		return nil, err
	}

	b := body{file, nil, offset, size}

  err = b.Reset()
  if err != nil {
    return nil, err
  }

	return &b, nil
}

// Reset a body to its initial state.  This involves rewinding to the beginning
// and resetting the internal io.LimitReader that's used to read only a certain
// number of bytes.  This is to allow retrying of a file
func (b *body) Reset() error {
	if _, err := b.File.Seek(b.Offset, 0); err != nil {
		return err
	}

	b.Reader = io.LimitReader(b.File, b.Size)
	return nil
}

// Satisfy the io.Reader interface by reading from the associated file
func (b body) Read(p []byte) (int, error) {
	return b.Reader.Read(p)
}

// Close a body and return relevant values back to their nil value
func (b *body) Close() error {
	if err := b.File.Close(); err != nil {
		return err
	}

	b.File = nil
	b.Reader = nil

	return nil
}

// Return a string representation of a Body for display
func (b body) String() string {
	return fmt.Sprintf("filename: %s offset: %d size: %d\n", b.File.Name(), b.Offset, b.Size)
}
