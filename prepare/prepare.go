package prepare

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"time"
  "strings"
)

type singlePartFileInfo struct {
	Sha256 []byte
	Size   int64
}

// Returns a buffer which represents a SHA256 checksum of the requested file
func hashFile(filename string, chunkSize int) (singlePartFileInfo, error) {
	// Create a file handle
	f, err := os.Open(filename)
	if err != nil {
    return singlePartFileInfo{}, err
	}
	defer f.Close()

	// Determine the filesize
	fi, err := f.Stat()
	if err != nil {
    return singlePartFileInfo{}, err
	}
	size := fi.Size()

	// Create a Hash object which we'll write bytes to as they're read in
	hash := sha256.New()

	buf := make([]byte, chunkSize)

	// Since total file sizes can be int64, we want to ensure that our filesize
	// counter handles this and is an int64 as well
	var totalBytes int64 = 0

	for {
		nBytes, err := f.Read(buf)
		if nBytes == 0 {
			break
		}
		if err != nil {
      return singlePartFileInfo{}, err
		}

		// NOTE: Per docs, this function never returns an error
		hash.Write(buf[:nBytes])

		// Even though nBytes is quite small compared to an int64, we must cast it
		// because go will (thankfully) require that the types are the same to add
		// them
		totalBytes += int64(nBytes)
	}

	if totalBytes != size {
		return singlePartFileInfo{}, fmt.Errorf("File size changed during hashing from %d to %d", size, totalBytes)
	}
	if fi, err := f.Stat(); err == nil {
		if size != fi.Size() {
			return singlePartFileInfo{}, fmt.Errorf("File size changed during hashing from %d to %d", size, fi.Size())
		}
	} else {
    return singlePartFileInfo{}, err
	}

	return singlePartFileInfo{hash.Sum(nil), totalBytes}, nil
}

type compressedSinglePartFileInfo struct {
	Sha256          []byte
	Size            int64
	TransferSha256  []byte
	TransferSize    int64
	ContentEncoding string
}

// Compress a file and return metadata for its upload
func gzipAndHashFile(inFilename, outFilename string, chunkSize int) (compressedSinglePartFileInfo, error) {
	// Create a file handle
	f, err := os.Open(inFilename)
	if err != nil {
		return compressedSinglePartFileInfo{}, err
	}
	defer f.Close()

	of, err := os.Create(outFilename)
	if err != nil {
		return compressedSinglePartFileInfo{}, err
	}
	defer of.Close()

	// Determine the filesize
	fi, err := f.Stat()
	if err != nil {
		return compressedSinglePartFileInfo{}, err
	}
	size := fi.Size()

	// Create a Hash object which we'll write bytes to as they're read in
	hash := sha256.New()
	transferHash := sha256.New()

	buf := make([]byte, chunkSize)

	// Since total file sizes can be int64, we want to ensure that our filesize
	// counter handles this and is an int64 as well
	var totalBytes int64 = 0

	// The Gzip writer will have anything written to it compressed then written
	// to the underlying io.Writer.  Does not return an error
	gzipWriter := gzip.NewWriter(io.MultiWriter(transferHash, of))
	defer gzipWriter.Close()

	// We're setting constant headers so that gzip has deterministic output
	gzipWriter.ModTime = time.Date(2000, time.January, 0, 0, 0, 0, 0, time.UTC)

	output := io.MultiWriter(gzipWriter, hash)

	for {
		nBytes, err := f.Read(buf)
		if nBytes == 0 {
			break
		}
		if err != nil {
		  return compressedSinglePartFileInfo{}, err
		}

		if _, err := output.Write(buf[:nBytes]); err != nil {
		  return compressedSinglePartFileInfo{}, err
		}

		// Even though nBytes is quite small compared to an int64, we must cast it
		// because go will (thankfully) require that the types are the same to add
		// them
		totalBytes += int64(nBytes)

	}

	if totalBytes != size {
    err := fmt.Errorf("File size changed during hashing from %d to %d", size, totalBytes)
		return compressedSinglePartFileInfo{}, err
	}
	if fi, err := f.Stat(); err == nil {
		if size != fi.Size() {
      err := fmt.Errorf("File size changed during hashing from %d to %d", size, fi.Size())
		  return compressedSinglePartFileInfo{}, err
		}
	} else {
		return compressedSinglePartFileInfo{}, err
	}

	// We need to close the Gzip writer otherwise we don't
	gzipWriter.Close()
	if err := of.Sync(); err != nil {
		panic(err)
	}

	ofi, err := of.Stat()
	if err != nil {
		return compressedSinglePartFileInfo{}, err
	}

	return compressedSinglePartFileInfo{hash.Sum(nil), totalBytes, transferHash.Sum(nil), ofi.Size(), "gzip"}, nil
}

type Part struct {
	Sha256 []byte
	Size   int64
	Start  int64
}

func (u Part) String() string {
	return fmt.Sprintf("Sha256: %x, Start: %d, Size: %d", u.Sha256, u.Start, u.Size)
}

type multiPartFileInfo struct {
	Sha256 []byte
	Size   int64
	Parts  []Part
}

// Hash a file, but also figure out the hashes of sub parts.  A sub part is the
// number of bytes obtained by multiplying chunkSize by chunksInPart.  This is
// done to simplify the calculation of the parts
func hashFileParts(filename string, chunkSize, chunksInPart int) (multiPartFileInfo, error) {
	// Create a file handle
	f, err := os.Open(filename)
	if err != nil {
    return multiPartFileInfo{}, err
	}
	defer f.Close()

	// Determine the filesize
	fi, err := f.Stat()
	if err != nil {
    return multiPartFileInfo{}, err
	}
	size := fi.Size()

	// Create a Hash object which we'll write bytes to as they're read in
	hash := sha256.New()
	partHash := sha256.New()

	buf := make([]byte, chunkSize)

	// Since total file sizes can be int64, we want to ensure that our filesize
	// counter handles this and is an int64 as well
	var totalBytes int64 = 0

	// We need to keep track of which part we're currently working in
	currentPart := 0
	// We need to keep track of which chunk we're working on in the current chunk
	currentPartChunk := 0
	// We need to know the size of the current part we're working on, mainly
	// for the last part so we determine the correct size
	var currentPartSize int64 = 0

	partSize := int64(chunkSize * chunksInPart)
	totalParts := int(math.Ceil(float64(size) / float64(partSize)))
	parts := make([]Part, totalParts)

	for {
		nBytes, err := f.Read(buf)
		if nBytes == 0 {
			break
		}
		if err != nil {
      return multiPartFileInfo{}, err
		}

		// NOTE: Per docs, this function never returns an error
		hash.Write(buf[:nBytes])
		partHash.Write(buf[:nBytes])

		// Even though nBytes is quite small compared to an int64, we must cast it
		// because go will (thankfully) require that the types are the same to add
		// them
		totalBytes += int64(nBytes)
		currentPartSize += int64(nBytes)

		if currentPartChunk == (chunksInPart - 1) {
			parts[currentPart] = Part{partHash.Sum(nil), currentPartSize, int64(currentPart) * partSize}
			partHash.Reset()
			currentPartChunk = 0
			currentPart++
			currentPartSize = 0
		} else {
			currentPartChunk++
		}
	}

	if totalBytes != size {
    err := fmt.Errorf("File size changed during hashing from %d to %d", size, totalBytes)
		return multiPartFileInfo{}, err
	}
	if fi, err := f.Stat(); err == nil {
		if size != fi.Size() {
      err := fmt.Errorf("File size changed during hashing from %d to %d", size, fi.Size())
		  return multiPartFileInfo{}, err
		}
	} else {
		return multiPartFileInfo{}, err
	}

	return multiPartFileInfo{hash.Sum(nil), totalBytes, parts}, nil
}

type SinglePartUpload struct {
	Filename        string
	Sha256          []byte
	Size            int64
	TransferSha256  []byte
	TransferSize    int64
	ContentEncoding string
}

func (u SinglePartUpload) String() string {
	return fmt.Sprintf("Single Part Upload Filename: %s, Sha256: %x, Size: %d, TransferSha256: %x, TransferSize: %d, ContentEncoding: %s",
		u.Filename, u.Sha256, u.Size, u.TransferSha256, u.TransferSize, u.ContentEncoding)
}

// Prepare a single part file upload with a scratch file for gzip encoding if requested
// Cleaning up the scratch file is the responsibility of the caller
func NewSinglePartUploadWithDetails(inFilename, outFilename string, gzip bool) (SinglePartUpload, error) {
	chunkSize := 1024 * 128 // 128KB
	if gzip {
		gzipped, err := gzipAndHashFile(inFilename, outFilename, chunkSize)
    if err == nil {
      return SinglePartUpload{
        Filename:        outFilename,
        Sha256:          gzipped.Sha256,
        Size:            gzipped.Size,
        TransferSha256:  gzipped.TransferSha256,
        TransferSize:    gzipped.TransferSize,
        ContentEncoding: gzipped.ContentEncoding,
      }, nil
    } else {
      return SinglePartUpload{}, err
    }
	} else {
		identity, err := hashFile(inFilename, chunkSize)
    if err == nil {
      return SinglePartUpload{
        Filename:        inFilename,
        Sha256:          identity.Sha256,
        Size:            identity.Size,
        TransferSha256:  identity.Sha256,
        TransferSize:    identity.Size,
        ContentEncoding: "identity",
      }, nil
    } else {
      return SinglePartUpload{}, err
    }
	}
}

// Prepare a new gzip-encoded single part upload using a temporary file in the
// same directory as the current process
//
// NOTE: This process creates a file, which is in the return value's Filename
// property for which cleanup is the responsibility of the caller of this
// function
func NewGzipSinglePartUpload(filename string) (SinglePartUpload, error) {
  // TODO: Make the output filename a parameter and update tests to match that
	return NewSinglePartUploadWithDetails(filename, filename + ".gz", true)
}

// Prepare a new identity-encoded (e.g. no encoding) single part upload.  This does not
// create any temporary files
func NewSinglePartUpload(filename string) (SinglePartUpload, error) {
	return NewSinglePartUploadWithDetails(filename, filename, false)
}

type MultiPartUpload struct {
	Filename        string
	Sha256          []byte
	Size            int64
	TransferSha256  []byte
	TransferSize    int64
	ContentEncoding string
	Parts           []Part
}

func (u MultiPartUpload) String() string {
  var partsStrings []string
  for _, part := range u.Parts {
    partsStrings = append(partsStrings, part.String())
  }
  partsString := strings.Join(partsStrings, "}, {")
	return fmt.Sprintf("Multi-part File Upload Filename: %s, Sha256: %x, Size: %d, TransferSha256: %x, TransferSize: %d, ContentEncoding: %s, Parts: [{%s}]",
		u.Filename, u.Sha256, u.Size, u.TransferSha256, u.TransferSize, u.ContentEncoding, partsString)
}

// Prepare a single part file upload with a scratch file for gzip encoding if requested
// Cleaning up the scratch file is the responsibility of the caller
func NewMultiPartUploadWithDetails(inFilename, outFilename string, gzip bool) (MultiPartUpload, error) {

	chunkSize := 1024 * 128     // 128KB
	partSize := 1024 * 1024 * 5 // 5MB Chunks

	if partSize < 1024*1024*5 {
	  err := fmt.Errorf("Partsize must be at least 5 MB, not %d", partSize)
    return MultiPartUpload{}, err
	}

	chunksInPart := partSize / chunkSize

	if gzip {
		gzipped, err := gzipAndHashFile(inFilename, outFilename, chunkSize)
    if err != nil {
      return MultiPartUpload{}, err
    }
		hashedParts, err := hashFileParts(outFilename, chunkSize, chunksInPart)
    if err != nil {
      return MultiPartUpload{}, err
    }
		// We want to make sure that the same file which we compressed is the file
		// that we broke into parts and hashed the parts
		if !bytes.Equal(hashedParts.Sha256, gzipped.TransferSha256) {
      err := fmt.Errorf("File changed between compression and hashing of parts")
      return MultiPartUpload{}, err
		}
		return MultiPartUpload{
			Filename:        outFilename,
			Sha256:          gzipped.Sha256,
			Size:            gzipped.Size,
			TransferSha256:  gzipped.TransferSha256,
			TransferSize:    gzipped.TransferSize,
			ContentEncoding: gzipped.ContentEncoding,
			Parts:           hashedParts.Parts,
		}, nil
	} else {
		hashedParts, err := hashFileParts(inFilename, chunkSize, chunksInPart)
    if err != nil {
      return MultiPartUpload{}, err
    }
		return MultiPartUpload{
			Filename:        outFilename,
			Sha256:          hashedParts.Sha256,
			Size:            hashedParts.Size,
			TransferSha256:  hashedParts.Sha256,
			TransferSize:    hashedParts.Size,
			ContentEncoding: "identity",
			Parts:           hashedParts.Parts,
		}, nil
	}
}

// Prepare a new gzip-encoded multi part upload using a temporary file in the
// same directory as the current process
//
// NOTE: This process creates a file, which is in the return value's Filename
// property for which cleanup is the responsibility of the caller of this
// function
func NewGzipMultiPartUpload(filename string) (MultiPartUpload, error) {
	cwd, err := os.Getwd()
	if err != nil {
    return MultiPartUpload{}, err
	}

	tmpfile, err := ioutil.TempFile(cwd, filename+".gz_")
	if err != nil {
    return MultiPartUpload{}, err
	}

	// We immediately close the file because we're only using it to create the
	// name
	if err := tmpfile.Close(); err != nil {
    return MultiPartUpload{}, err
	}

	return NewMultiPartUploadWithDetails(filename, tmpfile.Name(), true)
}

// Prepare a new identity-encoded (e.g. no encoding) multi part upload.  This does not
// create any temporary files
func NewMultiPartUpload(filename string) (MultiPartUpload, error) {
	return NewMultiPartUploadWithDetails(filename, filename, false)
}
