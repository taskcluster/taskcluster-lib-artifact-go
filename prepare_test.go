package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"github.com/taskcluster/taskcluster-lib-artifact-go/runner"
	"os"
	"testing"
)

func testMPUpload(upload multiPartUpload) error {
	of, err := os.Create("_output.dat")
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

		nBytes, err := body.Read(buf)
		if err != nil {
			return err
		}

		hash.Write(buf[:nBytes])
		overallHash.Write(buf[:nBytes])
		of.Write(buf[:nBytes])

		totalBytes += int64(nBytes)

		if !bytes.Equal(hash.Sum(nil), part.Sha256) {
			return fmt.Errorf("Checksum mismatch (part): %+v != %+v\n", hash.Sum(nil), part.Sha256)
		}
		if int64(nBytes) != part.Size {
			return fmt.Errorf("Size mismatch (part): %s != %s\n", nBytes, part.Size)
		}
	}

	if !bytes.Equal(overallHash.Sum(nil), upload.TransferSha256) {
		return fmt.Errorf("Checksum mismatch: %+v != %+v\n", overallHash.Sum(nil), upload.TransferSha256)
	}
	if totalBytes != upload.TransferSize {
		return fmt.Errorf("Size mismatch: %s != %s\n", totalBytes, upload.TransferSize)
	}

	return nil
}

func testSPUpload(upload singlePartUpload) error {
	of, err := os.Create("_output.dat")
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
		return fmt.Errorf("Checksum mismatch: %+v != %+v\n", hash.Sum(nil), upload.TransferSha256)
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
		of, err := os.Create(filename)
		if err != nil {
			t.Error(err)
		}
		for i := 0; i < 10*1024; i++ {
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

	t.Run("multipart gzip", func(t *testing.T) {
    chunkSize := 128 * 1024
    chunksInPart := 5 * 1024 * 1024 / chunkSize
		upload, err := NewMultiPartUpload(filename, filename+".gz", chunkSize, chunksInPart, true)
		if err != nil {
			t.Error(err)
		}
		t.Log(upload.String())
		err = testMPUpload(upload)
		if err != nil {
			t.Error(err)
		}
		t.Log("Completed")
	})

	t.Run("multipart identity", func(t *testing.T) {
    chunkSize := 128 * 1024
    chunksInPart := 5 * 1024 * 1024 / chunkSize
		upload, err := NewMultiPartUpload(filename, filename+".gz", chunkSize, chunksInPart, false)
		if err != nil {
			t.Error(err)
		}
		t.Log(upload.String())
		err = testMPUpload(upload)
		if err != nil {
			t.Error(err)
		}
		t.Log("Completed")
	})

	t.Run("singlepart gzip", func(t *testing.T) {
    chunkSize := 128 * 1024
		upload, err := NewSinglePartUpload(filename, filename + ".gz", chunkSize, true)
		if err != nil {
			t.Error(err)
		}
		err = testSPUpload(upload)
		if err != nil {
			t.Error(err)
		}
		t.Log("Completed")
	})

	t.Run("singlepart identity", func(t *testing.T) {
    chunkSize := 128 * 1024
		upload, err := NewSinglePartUpload(filename, filename + ".gz", chunkSize, false)
		if err != nil {
			t.Error(err)
		}
		err = testSPUpload(upload)
		if err != nil {
			t.Error(err)
		}
		t.Log("Completed")
	})

	// Now let's run the tests
	// ...
}