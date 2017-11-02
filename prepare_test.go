package artifact

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"io/ioutil"
	"os"
	"testing"
)

func testMPUpload(t *testing.T, filename string, u upload) {
	overallHash := sha256.New()
	hash := sha256.New()

	var totalBytes int64

	for i, part := range u.Parts {
		hash.Reset()

		// Inefficient, but this is but a test, so meh
		buf := make([]byte, part.Size)

		bodyFile, err := os.Open(filename)
		if err != nil {
			t.Fatal(err)
		}
		defer bodyFile.Close()

		body, err := newBody(bodyFile, part.Start, part.Size)
		if err != nil {
			t.Fatal(err)
		}
		defer body.Close()

		nBytes, err := body.Read(buf)
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}

		hash.Write(buf[:nBytes])
		overallHash.Write(buf[:nBytes])

		totalBytes += int64(nBytes)

		if int64(nBytes) != part.Size {
			t.Errorf("Size mismatch (part %d): %d != %d\n", i, nBytes, part.Size)
		}

		if !bytes.Equal(hash.Sum(nil), part.Sha256) {
			t.Errorf("Checksum mismatch (part %d): %x != %x\n", i, hash.Sum(nil), part.Sha256)
		}
	}

	if totalBytes != u.TransferSize {
		t.Errorf("Size mismatch: %d != %d\n", totalBytes, u.TransferSize)
	}
	if !bytes.Equal(overallHash.Sum(nil), u.TransferSha256) {
		t.Errorf("Checksum mismatch: %x != %x\n", overallHash.Sum(nil), u.TransferSha256)
	}
}

func testSPUpload(t *testing.T, filename string, u upload) {
	hash := sha256.New()

	buf := make([]byte, u.TransferSize)

	bodyFile, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer bodyFile.Close()

	body, err := newBody(bodyFile, 0, u.TransferSize)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()

	nBytes, err := body.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	hash.Write(buf[:nBytes])

	if int64(nBytes) != u.TransferSize {
		t.Errorf("Size mismatch: %d != %d\n", nBytes, u.TransferSize)
	}
	if !bytes.Equal(hash.Sum(nil), u.TransferSha256) {
		t.Errorf("Checksum mismatch: %x != %x\n", hash.Sum(nil), u.TransferSha256)
	}
}

func TestUploadPreperation(t *testing.T) {

	SetLogOutput(newUnitTestLogWriter(t))

	filename := "test-files/10mb.dat"

	// We want to do a little bit of setup before running the tests
	if fi, err := os.Stat(filename); os.IsNotExist(err) || fi.Size() != 10*1024*1024 {
		t.Log("input data did not exist or was wrong size, recreating")
		of, err := os.Create(filename)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 10*1024; i++ {
			c := 1024
			b := make([]byte, c)
			_, err := rand.Read(b)
			if err != nil {
				t.Fatal(err)
			}
			of.Write(b)
		}
		of.Close()
	}

	t.Run("multipart gzip", func(t *testing.T) {
		chunkSize := 128 * 1024
		chunksInPart := 5 * 1024 * 1024 / chunkSize
		input, err := os.Open(filename)
		if err != nil {
			t.Fatal(err)
		}
		defer input.Close()
		output, err := ioutil.TempFile("test-files", "mp-gz_")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(output.Name())

		upload, err := multiPartUpload(input, output, true, chunkSize, chunksInPart)
		if err != nil {
			t.Fatal(err)
		}

		testMPUpload(t, filename, upload)
	})

	t.Run("multipart identity", func(t *testing.T) {
		chunkSize := 128 * 1024
		chunksInPart := 5 * 1024 * 1024 / chunkSize
		input, err := os.Open(filename)
		if err != nil {
			t.Fatal(err)
		}
		defer input.Close()
		output, err := ioutil.TempFile("test-files", "mp-id_")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(output.Name())

		upload, err := multiPartUpload(input, output, false, chunkSize, chunksInPart)
		if err != nil {
			t.Fatal(err)
		}

		testMPUpload(t, filename, upload)
	})

	t.Run("singlepart gzip", func(t *testing.T) {
		chunkSize := 128 * 1024
		input, err := os.Open(filename)
		if err != nil {
			t.Fatal(err)
		}
		defer input.Close()
		output, err := ioutil.TempFile("test-files", "sp-gz_")
		// TODO REMOVE THIS
		t.Log(output.Name())
		if err != nil {
			t.Fatal(err)
		}
		// TODO UNCOMMENT THIS DEFER
		//defer os.Remove(output.Name())

		upload, err := singlePartUpload(input, output, true, chunkSize)
		if err != nil {
			t.Fatal(err)
		}

		testSPUpload(t, filename, upload)
	})

	t.Run("singlepart identity", func(t *testing.T) {
		chunkSize := 128 * 1024
		input, err := os.Open(filename)
		if err != nil {
			t.Fatal(err)
		}
		defer input.Close()
		output, err := ioutil.TempFile("test-files", "sp-id_")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(output.Name())

		upload, err := singlePartUpload(input, output, false, chunkSize)
		if err != nil {
			t.Fatal(err)
		}

		testSPUpload(t, filename, upload)
	})
}
