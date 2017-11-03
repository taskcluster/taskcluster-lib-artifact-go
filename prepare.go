package artifact

import (
	"bytes"
	gziplib "compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

// Part is a description of a single part of a multi-part upload
type part struct {
	Sha256 []byte
	Size   int64
	Start  int64
}

// Part should implement the Stringer interface
func (u part) String() string {
	return fmt.Sprintf("Sha256: %x, Start: %d, Size: %d", u.Sha256, u.Start, u.Size)
}

// Upload contains information relevant to program internals about the upload
// being performed
type upload struct {
	Sha256          []byte
	Size            int64
	TransferSha256  []byte
	TransferSize    int64
	ContentEncoding string
	Parts           []part
}

// Upload should implement the Stringer interface
func (u upload) String() string {
	partsString := ""
	if u.Parts != nil {
		var partsStrings []string
		for _, part := range u.Parts {
			partsStrings = append(partsStrings, part.String())
		}
		partsString = ", Parts: [{" + strings.Join(partsStrings, "}, {") + "}]"
	}
	return fmt.Sprintf(
		"Upload Sha256: %x, Size: %d, TransferSha256: %x, TransferSize: %d, ContentEncoding: %s%s",
		u.Sha256, u.Size, u.TransferSha256, u.TransferSize, u.ContentEncoding, partsString)
}

// Detmerine the hash of each chunk of the input as well as the overall hash of
// the file.  This overall hash is calculated and returned to allow the caller
// to ensure that the same file which they have prepared for upload is the one
// for which the parts were calculated.  It is a defect in calling code to not
// compared the []byte return value to that of the the file which is expected
// to be read.  Comparison can be made with bytes.Equal()
func hashFileParts(input io.ReadSeeker, size int64, chunkSize, chunksInPart int) ([]part, []byte, error) {
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return []part{}, []byte{}, err
	}

	hash := sha256.New()
	partHash := sha256.New()

	buf := make([]byte, chunkSize)

	// We need to keep track of which part we're currently working in
	currentPart := 0

	// We need to keep track of which chunk we're working on in the current part
	currentPartChunk := 0

	// We need to know the size of the current part we're working on, mainly
	// for the last part so we determine the correct size
	var currentPartSize int64

	// We need to know the theoretically maximum partSize
	partSize := int64(chunkSize * chunksInPart)
	totalParts := int(math.Ceil(float64(size) / float64(partSize)))

	// We need somewhere to store the parts
	parts := make([]part, totalParts)

	for {
		nBytes, err := input.Read(buf)

		if nBytes == 0 {
			if currentPartSize > 0 {
				parts[currentPart] = part{partHash.Sum(nil), currentPartSize, int64(currentPart) * partSize}
			}
			break
		}

		if err != nil {
			return []part{}, []byte{}, err
		}

		// NOTE: Per docs, this function never returns an error
		hash.Write(buf[:nBytes])
		partHash.Write(buf[:nBytes])

		currentPartSize += int64(nBytes)

		// Since we read data, the file continues to be read, so let's figure out
		// if we're in the last chunk of the part
		if currentPartChunk == (chunksInPart - 1) {
			// If we're in the last chunk, we should set the part information
			parts[currentPart] = part{partHash.Sum(nil), currentPartSize, int64(currentPart) * partSize}
			partHash.Reset()
			currentPartChunk = 0
			currentPart++
			currentPartSize = 0
		} else {
			// If we're not in the last chunk, we'll simply move on to the next until
			// we are or run out of input
			currentPartChunk++
		}
	}

	return parts, hash.Sum(nil), nil
}

// In order to do an upload of a single-part file, we need to do the following things:
//   1. determine the input size
//   2. calculate the input's sha256
//   3. optionally gzip-encode the input
//   4. write the intput to the output
//   5. determine the output size
//   6. calculate the output's sha256
// For both gzip and non-gzip encoded resources, we write from the input to the
// output.  This is done to ensure that the file which is uploaded is exactly
// that which was hashed.
// Calling code is responsible for cleaning up whatever is written to output
func singlePartUpload(input io.ReadSeeker, output io.Writer, gzip bool, chunkSize int) (upload, error) {
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return upload{}, err
	}

	hash := sha256.New()
	buf := make([]byte, chunkSize)

	// When we're compressing using gzip, we're going to use a more complex copy routine
	if gzip {
		transferHash := sha256.New()
		// Unfortunately, the gzip.Writer doesn't track how many bytes were written
		// to the underlying io.Writer, so we need to do that
		transferSize := byteCountingWriter{0}
		gzipWriter := gziplib.NewWriter(io.MultiWriter(transferHash, output, &transferSize))

		// We're setting constant headers so that gzip has deterministic output
		gzipWriter.ModTime = time.Date(2000, time.January, 0, 0, 0, 0, 0, time.UTC)

		_output := io.MultiWriter(gzipWriter, hash)

		contentSize, err := io.CopyBuffer(_output, input, buf)
		if err != nil {
			return upload{}, err
		}

		// We need to close the gzip writer in order to get the Gzip footer.  Note
		// that this does not close the output ReadSeeker that we passed in
		err = gzipWriter.Flush()
		if err != nil {
			return upload{}, err
		}
		err = gzipWriter.Close()
		if err != nil {
			return upload{}, err
		}

		return upload{
			Sha256:          hash.Sum(nil),
			Size:            contentSize,
			TransferSha256:  transferHash.Sum(nil),
			TransferSize:    transferSize.count,
			ContentEncoding: "gzip",
		}, nil
	}

	// Otherwise, identity encoding is drastically simpler
	_output := io.MultiWriter(output, hash)

	totalBytes, err := io.CopyBuffer(_output, input, buf)
	if err != nil {
		return upload{}, err
	}

	return upload{
		Sha256:          hash.Sum(nil),
		Size:            totalBytes,
		TransferSha256:  hash.Sum(nil),
		TransferSize:    totalBytes,
		ContentEncoding: "identity",
	}, nil
}

// This function is similar to singlePartUpload, except the output of the
// copy/gzip operation from singlePartUpload is broken into parts and hashed.
// The chunkSize and chunksInParts can be multiplied to determine the part size
// Calling code is responsible for cleaning up whatever is written to output
func multiPartUpload(input io.ReadSeeker, output io.ReadWriteSeeker, gzip bool, chunkSize, chunksInPart int) (upload, error) {

	// We want to make sure we're at the start of the input
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return upload{}, err
	}

	partSize := chunkSize * chunksInPart

	if partSize < 1024*1024*5 {
		err := fmt.Errorf("partsize must be at least 5 MB, not %d", partSize)
		return upload{}, err
	}

	// First, we'll calculate the SinglePartUpload version of this
	u, err := singlePartUpload(input, output, gzip, chunkSize)
	if err != nil {
		return upload{}, err
	}

	// After we've written single part file over to the new file, we need to seek
	// back to the start so we can break it up into hash chunks
	if _, err := output.Seek(0, io.SeekStart); err != nil {
		return upload{}, err
	}

	parts, hash, err := hashFileParts(output, u.TransferSize, chunkSize, chunksInPart)
	if err != nil {
		return upload{}, err
	}

	// We want to protect against the file changing between when we copied it to the new location
	if !bytes.Equal(hash, u.TransferSha256) {
		return upload{}, errors.New("file changed while determining part information")
	}

	u.Parts = parts
	return u, nil
}
