package main

import (
	"fmt"
	"github.com/taskcluster/taskcluster-lib-artifact-go/prepare"
	"github.com/taskcluster/taskcluster-lib-artifact-go/runner"
  "crypto/sha256"
  "bytes"
  "os"
)

func testMPUpload(upload prepare.MultiPartUpload) {
  of, err := os.Create("output")
  if err != nil {
    panic(err)
  }

  overallHash := sha256.New()
  hash := sha256.New()

  var totalBytes int64 = 0

  for partNum, part := range upload.Parts {
    hash.Reset()

    // Inefficient, but this is but a test, so meh
    buf := make([]byte, part.Size)

    body, err := runner.NewBody(upload.Filename, part.Start, part.Size)
    if err != nil { panic(err) }
    defer body.Close()

    nBytes, err := body.Read(buf)
    if err != nil { panic(err) }

    hash.Write(buf[:nBytes])
    overallHash.Write(buf[:nBytes])
    of.Write(buf[:nBytes])

    totalBytes += int64(nBytes)

    if ! bytes.Equal(hash.Sum(nil), part.Sha256) {
      panic(fmt.Errorf("Checksum mismatch: %s != %s\n", hash.Sum(nil), part.Sha256))
    }
    if int64(nBytes) != part.Size {
      panic(fmt.Errorf("Size mismatch: %s != %s\n", nBytes, part.Size))
    }

    fmt.Printf("Part Number %d checks out\n", partNum + 1)
  }

  if ! bytes.Equal(overallHash.Sum(nil), upload.TransferSha256) {
    panic(fmt.Errorf("Checksum mismatch: %s != %s\n", overallHash.Sum(nil), upload.TransferSha256))
  }
  if totalBytes != upload.TransferSize {
    panic(fmt.Errorf("Size mismatch: %s != %s\n", totalBytes, upload.TransferSize))
  }
  fmt.Printf("Multipart overall file checks out\n")
}


func testSPUpload(upload prepare.SinglePartUpload) {
  of, err := os.Create("output")
  if err != nil {
    panic(err)
  }

  hash := sha256.New()

  buf := make([]byte, upload.TransferSize)

  body, err := runner.NewBody(upload.Filename, 0, upload.TransferSize)
  if err != nil { panic(err) }
  defer body.Close()

  nBytes, err := body.Read(buf)
  if err != nil { panic(err) }

  hash.Write(buf[:nBytes])
  of.Write(buf[:nBytes])

  if ! bytes.Equal(hash.Sum(nil), upload.TransferSha256) {
    panic(fmt.Errorf("Checksum mismatch: %s != %s\n", hash.Sum(nil), upload.TransferSha256))
  }
  if int64(nBytes) != upload.TransferSize {
    panic(fmt.Errorf("Size mismatch: %s != %s\n", nBytes, upload.TransferSize))
  }
  fmt.Printf("Single part overall file checks out\n")
}

func testGzipMPUpload(filename string) {
  upload := prepare.NewGzipMultiPartUpload(filename)
  testMPUpload(upload)
}

func testIdentityMPUpload(filename string) {
  upload := prepare.NewMultiPartUpload(filename)
  testMPUpload(upload)
}

func testGzipSPUpload(filename string) {
  upload := prepare.NewGzipSinglePartUpload(filename)
  testSPUpload(upload)
}

func testIdentitySPUpload(filename string) {
  upload := prepare.NewSinglePartUpload(filename)
  testSPUpload(upload)
}



func main() {
  headers := make(runner.Headers)
  headers["User-Agent"] = "taskcluster-lib-artifact-go"
  request := runner.Request{"http://www.google.com", "GET", headers}
  fmt.Printf("%+v\n", request)
  testGzipMPUpload("borap.mp3")
  testIdentityMPUpload("borap.mp3")
  testGzipSPUpload("borap.mp3")
  testIdentitySPUpload("borap.mp3")
}
