package prepare

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
  "testing"
  "github.com/taskcluster/taskcluster-lib-artifact-go/runner"
  "crypto/rand"
)

func testMPUpload(upload MultiPartUpload) error {
	of, err := os.Create("output")
	if err != nil {
    return err
	}
  defer of.Close()

	overallHash := sha256.New()
	hash := sha256.New()

	var totalBytes int64 = 0

	for _, part := range upload.Parts {
		hash.Reset()

		// Inefficient, but this is but a test, so meh
		buf := make([]byte, part.Size)

		body, err := runner.NewBody(upload.Filename, part.Start, part.Size)
		if err != nil {
      return err
		}
		defer body.Close()
    body.Print()

		nBytes, err := body.Read(buf)
		if err != nil {
      return err
		}

		hash.Write(buf[:nBytes])
		overallHash.Write(buf[:nBytes])
		of.Write(buf[:nBytes])

		totalBytes += int64(nBytes)

		if !bytes.Equal(hash.Sum(nil), part.Sha256) {
			return fmt.Errorf("Checksum mismatch: %s != %s\n", hash.Sum(nil), part.Sha256)
		}
		if int64(nBytes) != part.Size {
			return fmt.Errorf("Size mismatch: %s != %s\n", nBytes, part.Size)
		}
	}

	if !bytes.Equal(overallHash.Sum(nil), upload.TransferSha256) {
		return fmt.Errorf("Checksum mismatch: %s != %s\n", overallHash.Sum(nil), upload.TransferSha256)
	}
	if totalBytes != upload.TransferSize {
		return fmt.Errorf("Size mismatch: %s != %s\n", totalBytes, upload.TransferSize)
	}

  return nil
}

func testSPUpload(upload SinglePartUpload) error {
	of, err := os.Create("output")
	if err != nil {
    return err
	}

	hash := sha256.New()

	buf := make([]byte, upload.TransferSize)

	body, err := runner.NewBody(upload.Filename, 0, upload.TransferSize)
	if err != nil {
    return err
	}
	defer body.Close()

	nBytes, err := body.Read(buf)
	if err != nil {
    return err
	}

	hash.Write(buf[:nBytes])
	of.Write(buf[:nBytes])

	if !bytes.Equal(hash.Sum(nil), upload.TransferSha256) {
		return fmt.Errorf("Checksum mismatch: %s != %s\n", hash.Sum(nil), upload.TransferSha256)
	}
	if int64(nBytes) != upload.TransferSize {
		return fmt.Errorf("Size mismatch: %s != %s\n", nBytes, upload.TransferSize)
	}

  return nil
}

func TestUploadPreperation(t *testing.T) {
  filename := "_test.dat"
  // We want to do a little bit of setup before running the tests
  if _, err := os.Stat(filename); os.IsNotExist(err) {
    of, err := os.Create(filename);
    if err != nil {
      t.Error(err)
    }
    for i := 0; i < 10 * 1024; i++ {
      c := 1024
      b := make([]byte, c)
      _, err := rand.Read(b)
      if err != nil {
        t.Error(err)
      }
      of.Write(b)
    }
    of.Close()
  }
  
  /*t.Run("multipart gzip", func(t *testing.T) {
    t.Parallel()
    upload := NewGzipMultiPartUpload(filename)
    t.Log(upload.String())
    err := testMPUpload(upload)
    if err != nil {
      t.Error(err)
    }
    t.Log("Completed")
  })*/

  t.Run("multipart identity", func(t *testing.T) {
    t.Parallel()
    upload := NewMultiPartUpload(filename)
    t.Log(upload.String())
    err := testMPUpload(upload)
    if err != nil {
      t.Error(err)
    }
    t.Log("Completed")
  })

  /*t.Run("singlepart gzip", func(t *testing.T) {
    t.Parallel()
    upload := NewGzipSinglePartUpload(filename)
    err := testSPUpload(upload)
    if err != nil {
      t.Error(err)
    }
    t.Log("Completed")
  })

  t.Run("singlepart identity", func(t *testing.T) {
    t.Parallel()
    upload := NewSinglePartUpload(filename)
    err := testSPUpload(upload)
    if err != nil {
      t.Error(err)
    }
    t.Log("Completed")
  })*/

  // Now let's run the tests
  // ...
}
