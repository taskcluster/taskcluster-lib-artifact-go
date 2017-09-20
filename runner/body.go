package runner

import (
	"errors"
	"fmt"
	"io"
	"os"
)

type Body struct {
	File   *os.File
	Reader io.Reader
	Offset int64
	Size   int64
}

// Create a body.  A body is an io.Reader instance which reads from the file at
// filename, starting at the `offset`th byte and reading up to `size` bytes in
// total.
func NewBody(filename string, offset, size int64) (*Body, error) {
	if size == 0 {
		return nil, errors.New("Cannot specify a size of 0")
	}

	file, err := os.Open(filename)

	if err != nil {
		return nil, err
	}

	newBody := Body{file, nil, offset, size}

	newBody.Reset()

	return &newBody, nil
}

func (b *Body) Reset() error {
	if _, err := b.File.Seek(b.Offset, 0); err != nil {
		return err
	}

	b.Reader = io.LimitReader(b.File, b.Size)
	return nil
}

func (b Body) Read(p []byte) (int, error) {
	return b.Reader.Read(p)
}

func (b *Body) Close() error {
	if err := b.File.Close(); err != nil {
		return err
	}

	b.File = nil
	b.Reader = nil

	return nil
}

func (b Body) Print() {
	fmt.Printf("filename: %s offset: %d size: %d\n", b.File.Name(), b.Offset, b.Size)
}
