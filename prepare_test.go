package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"os"
	"testing"
)

func testMPUpload(t *testing.T, upload multiPartUpload) {
	overallHash := sha256.New()
	hash := sha256.New()

	var totalBytes int64 = 0

	for _, part := range upload.Parts {
		hash.Reset()

		// Inefficient, but this is but a test, so meh
		buf := make([]byte, part.Size)

		body, err := NewBody(upload.Filename, part.Start, part.Size)
		if err != nil {
			t.Error(err)
		}
		defer body.Close()

		nBytes, err := body.Read(buf)
		if err != nil {
			t.Error(err)
		}

		hash.Write(buf[:nBytes])
		overallHash.Write(buf[:nBytes])

		totalBytes += int64(nBytes)

		if !bytes.Equal(hash.Sum(nil), part.Sha256) {
      t.Errorf("Checksum mismatch (part): %x != %x\n", hash.Sum(nil), part.Sha256)
		}
		if int64(nBytes) != part.Size {
			t.Errorf("Size mismatch (part): %s != %s\n", nBytes, part.Size)
		}
	}

	if !bytes.Equal(overallHash.Sum(nil), upload.TransferSha256) {
		t.Errorf("Checksum mismatch: %x != %x\n", overallHash.Sum(nil), upload.TransferSha256)
	}
	if totalBytes != upload.TransferSize {
		t.Errorf("Size mismatch: %s != %s\n", totalBytes, upload.TransferSize)
	}
}

func testSPUpload(t *testing.T, upload singlePartUpload) {
	hash := sha256.New()

	buf := make([]byte, upload.TransferSize)

	body, err := NewBody(upload.Filename, 0, upload.TransferSize)
	if err != nil {
    t.Error(err)
	}
	defer body.Close()

	nBytes, err := body.Read(buf)
	if err != nil {
    t.Error(err)
	}

	hash.Write(buf[:nBytes])

	if !bytes.Equal(hash.Sum(nil), upload.TransferSha256) {
		t.Errorf("Checksum mismatch: %x != %x\n", hash.Sum(nil), upload.TransferSha256)
	}
	if int64(nBytes) != upload.TransferSize {
		t.Errorf("Size mismatch: %s != %s\n", nBytes, upload.TransferSize)
	}
}

func TestUploadPreperation(t *testing.T) {
	filename := "test-files/10mb.dat"

	// We want to do a little bit of setup before running the tests
	if fi, err := os.Stat(filename); os.IsNotExist(err) || fi.Size() != 10*1024*1024 {
    t.Log("input data did not exist or was wrong size, recreating")
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
		upload, err := newMultiPartUpload(filename, filename+".gz", chunkSize, chunksInPart, true)
		if err != nil {
			t.Error(err)
		}
		testMPUpload(t, upload)
	})

	t.Run("multipart identity", func(t *testing.T) {
    chunkSize := 128 * 1024
    chunksInPart := 5 * 1024 * 1024 / chunkSize
		upload, err := newMultiPartUpload(filename, filename+".gz", chunkSize, chunksInPart, false)
		if err != nil {
			t.Error(err)
		}
		testMPUpload(t, upload)
	})

	t.Run("singlepart gzip", func(t *testing.T) {
    chunkSize := 128 * 1024
		upload, err := newSinglePartUpload(filename, filename + ".gz", chunkSize, true)
		if err != nil {
			t.Error(err)
		}
		testSPUpload(t, upload)
	})

	t.Run("singlepart identity", func(t *testing.T) {
    chunkSize := 128 * 1024
		upload, err := newSinglePartUpload(filename, filename + ".gz", chunkSize, false)
		if err != nil {
			t.Error(err)
		}
		testSPUpload(t, upload)
	})

	// Now let's run the tests
	// ...
}
