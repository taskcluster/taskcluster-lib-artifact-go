package artifact

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
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

func fileinfo(t *testing.T, filename string) (int64, []byte) {
	f, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	hash := sha256.New()

	nBytes, err := io.Copy(hash, f)
	if err != nil {
		t.Fatal(err)
	}
	return nBytes, hash.Sum(nil)
}

func testUpload(t *testing.T, gzip bool, mp bool, filename string) {
	chunkSize := 128 * 1024

	input, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()

	output, err := ioutil.TempFile("test-files", "sp-gz_")

	t.Log(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	//defer os.Remove(output.Name())

	u, err := singlePartUpload(input, output, gzip, chunkSize)
	if err != nil {
		t.Fatal(err)
	}

	if err := output.Close(); err != nil {
		t.Fatal(err)
	}

	inputSize, inputHash := fileinfo(t, filename)
	outputSize, outputHash := fileinfo(t, output.Name())

	if gzip && u.ContentEncoding != "gzip" {
		t.Errorf("Incorrect content encoding: %s", u.ContentEncoding)
	}

	if !gzip && u.ContentEncoding != "identity" {
		t.Errorf("Incorrect content encoding: %s", u.ContentEncoding)
	}

	if inputSize != u.Size {
		t.Errorf("Input size %d did not match prepared size %d", inputSize, u.Size)
	}

	if outputSize != u.TransferSize {
		t.Errorf("Input size %d did not match prepared size %d", outputSize, u.TransferSize)
	}

	if !bytes.Equal(inputHash, u.Sha256) {
		t.Errorf("Input sha256 %x did not match prepared sha256 %x", inputHash, u.Sha256)
	}

	if !bytes.Equal(outputHash, u.TransferSha256) {
		t.Errorf("Output sha256 %x did not match prepared sha256 %x", outputHash, u.Sha256)
	}

	if mp {
		for i, part := range u.Parts {
			phash := sha256.New()
			reader := io.NewSectionReader(input, part.Start, part.Size)
			partBytes, err := io.Copy(phash, reader)
			if err != nil {
				t.Fatal(err)
			}
			if part.Size != partBytes {
				t.Errorf("Part %d size %d did not match prepared size %d", i, partBytes, part.Size)
			}

			if !bytes.Equal(phash.Sum(nil), part.Sha256) {
				t.Errorf("Part %d sha256 %d did not match prepared sha256 %d", i, phash.Sum(nil), part.Sha256)
			}

		}
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
		testUpload(t, true, true, filename)
	})

	t.Run("multipart identity", func(t *testing.T) {
		testUpload(t, false, true, filename)
	})

	t.Run("singlepart gzip", func(t *testing.T) {
		testUpload(t, true, false, filename)
	})

	t.Run("singlepart identity", func(t *testing.T) {
		testUpload(t, false, false, filename)
	})
}

func BenchmarkPrepare(b *testing.B) {

	// Chunk Sizes to test, slice items are the number of KB in the chunk
	chunkSizes := []int{1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2048, 4096, 8192}

	// File Sizes to test, slice items are the number of MB in the file
	fileSizes := []int{1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024}

	rbuf := make([]byte, 1024*1024)

	for _, gzip := range []bool{false, true} {
		for _, fileSize := range fileSizes {
			filename := fmt.Sprintf("test-files/%d-mb.dat")
			createFile, err := os.Create(filename)
			if err != nil {
				b.Fatal(err)
			}
			for i := 0; i < fileSize; i++ {
				_, err := rand.Read(rbuf)
				if err != nil {
					b.Fatal(err)
				}
				_, err = createFile.Write(rbuf)
				if err != nil {
					b.Fatal(err)
				}
			}

			for _, chunkSize := range chunkSizes {
				b.Run(fmt.Sprintf("FileSize=%dMB ChunkSize=%dKB Gzip=%t SinglePart", fileSize, chunkSize, gzip), func(b *testing.B) {
					input, err := os.Open(filename)
					if err != nil {
						b.Fatal(err)
					}
					defer input.Close()

					output, err := ioutil.TempFile("", "bench")
					if err != nil {
						b.Fatal(err)
					}
					defer output.Close()
					defer os.Remove(output.Name())

					b.ResetTimer()
					singlePartUpload(input, output, gzip, chunkSize)
					b.StopTimer()

				})

				b.Run(fmt.Sprintf("FileSize=%dMB ChunkSize=%dKB Gzip=%t MultiPart", fileSize, chunkSize, gzip), func(b *testing.B) {
					input, err := os.Open(filename)
					if err != nil {
						b.Fatal(err)
					}
					defer input.Close()

					output, err := ioutil.TempFile("", "bench")
					if err != nil {
						b.Fatal(err)
					}
					defer output.Close()
					defer os.Remove(output.Name())

					b.ResetTimer()
					multiPartUpload(input, output, gzip, chunkSize, 10*1024*1024/chunkSize)
					b.StopTimer()

				})
			}
		}
	}

}
