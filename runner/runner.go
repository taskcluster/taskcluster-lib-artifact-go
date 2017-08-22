package runner

import (
  "fmt"
  "strings"
  "os"
  "io"
)

type Headers map[string]string

// The request type contains the information needed to run an HTTP method
type Request struct {
  Url string
  Method string
  Headers Headers
}

func (r Request) String() string {
  return fmt.Sprintf("%s %s %+v", strings.ToUpper(r.Method), r.Url, r.Headers)
}

type Body struct {
  File *os.File
  Reader io.Reader
  Offset int64
  Size int64
}

// Create a body
func NewBody(filename string, offset, size int64) (*Body, error) {
  file, err := os.Open(filename)

  if err != nil {
    panic (err)
  }

  newBody := Body{file, nil, offset, size}

  // Rather than implementing this again
  newBody.Reset()

  return &newBody, nil
}

func (b *Body) Reset() error {
  if _, err := b.File.Seek(b.Offset, 0); err != nil {
    panic(err)
  }

  b.Reader = io.LimitReader(b.File, b.Size)
  return nil
}

func (b Body) Read(p []byte) (int, error) {
  //return b.File.Read(p)  
  //fmt.Printf("%+v\n", b.Reader)
  return b.Reader.Read(p)  
}

func (b *Body) Close() error {
  if err := b.File.Close(); err != nil {
    panic(err)
  }

  b.File = nil
  b.Reader = nil

  return nil
}

func RunWithbody(request Request, body Body) error {
  return nil
}


