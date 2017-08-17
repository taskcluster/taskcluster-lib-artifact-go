package hashfile

import (
  "fmt"
  "os"
  "crypto/sha256"
)

const chunksize = 1024

func HashFile(filename string) {

  // Create a file handle
  f, err := os.Open(filename)
  if err != nil {
    panic(err)
  }

  // Create a Hash object which we'll write bytes to as they're read in
  hash := sha256.New()

  buf := make([]byte, chunksize)

  for {
    nBytes, err := f.Read(buf)
    if nBytes == 0 {
      break;
    }
    if err != nil {
      panic(err)
    }
    if _, err = hash.Write(buf[:nBytes]); err != nil {
      panic(err)
    }

  }

  fmt.Printf("%x", hash.Sum(nil))
}
