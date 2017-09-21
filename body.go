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

	b.Reset()

	return &b, nil
}

func (b *body) Reset() error {
	if _, err := b.File.Seek(b.Offset, 0); err != nil {
		return err
	}

	b.Reader = io.LimitReader(b.File, b.Size)
	return nil
}

func (b body) Read(p []byte) (int, error) {
	return b.Reader.Read(p)
}

func (b *body) Close() error {
  if err := b.File.Close(); err != nil {
    return err
  }

	b.File = nil
	b.Reader = nil

	return nil
}

func (b body) Print() {
	fmt.Printf("filename: %s offset: %d size: %d\n", b.File.Name(), b.Offset, b.Size)
}
