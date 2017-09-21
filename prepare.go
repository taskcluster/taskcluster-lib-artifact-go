package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"
)

// Internal struct for hashing a simple single part file without compression
type singlePartFileInfo struct {
	Sha256 []byte
	Size   int64
}

// Internal struct for hashing a simple single part file with compression
type compressedSinglePartFileInfo struct {
	Sha256          []byte
	Size            int64
	TransferSha256  []byte
	TransferSize    int64
	ContentEncoding string
}

// Internal struct for hashing a simple single part file without compression
type multiPartFileInfo struct {
	Sha256 []byte
	Size   int64
	Parts  []part
}

// A singlePartUpload represents the information required to do an upload of a
// single part file with or without compression.
type singlePartUpload struct {
	Filename        string
	Sha256          []byte
	Size            int64
	TransferSha256  []byte
	TransferSize    int64
	ContentEncoding string
}

func (u singlePartUpload) String() string {
	return fmt.Sprintf("Single Part Upload Filename: %s, Sha256: %x, Size: %d, TransferSha256: %x, TransferSize: %d, ContentEncoding: %s",
		u.Filename, u.Sha256, u.Size, u.TransferSha256, u.TransferSize, u.ContentEncoding)
}

// A Part represents the information about a single part of a multipart upload
type part struct {
	Sha256 []byte
	Size   int64
	Start  int64
}

func (u part) String() string {
	return fmt.Sprintf("Sha256: %x, Start: %d, Size: %d", u.Sha256, u.Start, u.Size)
}

// A multiPartUpload represents the information about a multipart upload
type multiPartUpload struct {
	Filename        string
	Sha256          []byte
	Size            int64
	TransferSha256  []byte
	TransferSize    int64
	ContentEncoding string
	Parts           []part
}

func (u multiPartUpload) String() string {
	var partsStrings []string
	for _, part := range u.Parts {
		partsStrings = append(partsStrings, part.String())
	}
	partsString := strings.Join(partsStrings, "}, {")
	return fmt.Sprintf(
    "Multi-part File Upload Filename: %s , Sha256: %x, Size: %d, " +
    "TransferSha256: %x, TransferSize: %d, ContentEncoding: %s, Parts: [{%s}]",
		u.Filename, u.Sha256, u.Size, u.TransferSha256, u.TransferSize, u.ContentEncoding, partsString)
}


// Hash a file and count the number of bytes in it in a single pass.  File is
// streamed and an effort is made to ensure that if the file is modified during
// the process that the function return an error.  The `chunkSize` parameter is
// the maximum number of bytes that will be read in each attempt to read the
// file
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

// Compress a file, hashing the contents and counting the bytes both before and
// after the compressor.  Compression and hashing operations are done using
// streaming.  An effort is made to check if the file has changed while the
// operation was being performed.  The `outFilename` parameter is a file path
// where the output should be written and will truncate an existing file at
// this location.  The `chunkSize` parameter represents the maximum number of
// bytes to read in any given read attempt.
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


// In a single pass, hash a file as well as the parts based on the chunkSize
// and chunksInPart parameters.  The chunkSize is the.  The `chunkSize`
// parameter is the maximum number of bytes to read in a single read attempt.
// The `chunksInPart` parameter is the maximum number of `chunkSize` sized
// chunks in each part.  The maximum part size is therefore `chunkSize *
// chunksInPart`.
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

	// We need to keep track of which chunk we're working on in the current part
	currentPartChunk := 0

	// We need to know the size of the current part we're working on, mainly
	// for the last part so we determine the correct size
	var currentPartSize int64 = 0

	// We need to know the theoretically maximum partSize
	partSize := int64(chunkSize * chunksInPart)
	totalParts := int(math.Ceil(float64(size) / float64(partSize)))
	// TODO the +1 is a bug
	parts := make([]part, totalParts)

	for {
		nBytes, err := f.Read(buf)
		if nBytes == 0 {
			if currentPartSize > 0 {
				parts[currentPart] = part{partHash.Sum(nil), currentPartSize, int64(currentPart) * partSize}
			}
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

// Prepare a single part upload with or without gzip encoding.  If gzip
// encoding is used, the `outFilename` parameter is the file path where the
// intermediate gzip-encoded file is stored.  Any file at this path will be
// truncated.  It is the responsibility of the caller to remove this file after
// they have uploaded the file.  The `chunkSize` parameter is the maximum
// number of bytes that should be read from the `inFilename` file in each read
// attempt
func newSinglePartUpload(inFilename, outFilename string, chunkSize int, gzip bool) (singlePartUpload, error) {
  if _, err := os.Stat(inFilename); os.IsNotExist(err) {
    return singlePartUpload{}, err
  }
	if gzip {
    if outFilename == "" {
      err := fmt.Errorf("When using gzip encoding, an outFilename value must be provided")
      return singlePartUpload{}, err
    }
		gzipped, err := gzipAndHashFile(inFilename, outFilename, chunkSize)
		if err == nil {
			return singlePartUpload{
				Filename:        outFilename,
				Sha256:          gzipped.Sha256,
				Size:            gzipped.Size,
				TransferSha256:  gzipped.TransferSha256,
				TransferSize:    gzipped.TransferSize,
				ContentEncoding: gzipped.ContentEncoding,
			}, nil
		} else {
			return singlePartUpload{}, err
		}
	} else {
		identity, err := hashFile(inFilename, chunkSize)
		if err == nil {
			return singlePartUpload{
				Filename:        inFilename,
				Sha256:          identity.Sha256,
				Size:            identity.Size,
				TransferSha256:  identity.Sha256,
				TransferSize:    identity.Size,
				ContentEncoding: "identity",
			}, nil
		} else {
			return singlePartUpload{}, err
		}
	}
}


// Prepare a multi part upload with or without gzip encoding.  If gzip encoding
// is used, the `outFilename` parameter is the file path where the intermediate
// gzip-encoded file is stored.  Any file at this path will be truncated.  It
// is the responsibility of the caller to remove this file after they have
// uploaded the file.  The `chunkSize` parameter is the maximum number of bytes
// that should be read from the `inFilename` file in each read attempt.  The
// `chunksInPart` parameter is the maximum number of `chunkSize` chunks in each
// part.  Note that the partsize is `chunkSize * chunksInPart`
func newMultiPartUpload(inFilename, outFilename string, chunkSize, chunksInPart int, gzip bool) (multiPartUpload, error) {

	partSize := chunkSize * chunksInPart

	if partSize < 1024*1024*5 {
		err := fmt.Errorf("Partsize must be at least 5 MB, not %d", partSize)
		return multiPartUpload{}, err
	}

	if gzip {
    if outFilename == "" {
      err := fmt.Errorf("When using gzip encoding, an outFilename value must be provided")
      return multiPartUpload{}, err
    }
		gzipped, err := gzipAndHashFile(inFilename, outFilename, chunkSize)
		if err != nil {
			return multiPartUpload{}, err
		}
		hashedParts, err := hashFileParts(outFilename, chunkSize, chunksInPart)
		if err != nil {
			return multiPartUpload{}, err
		}
		// We want to make sure that the same file which we compressed is the file
		// that we broke into parts and hashed the parts
		if !bytes.Equal(hashedParts.Sha256, gzipped.TransferSha256) {
			err := fmt.Errorf("File changed between compression and hashing of parts")
			return multiPartUpload{}, err
		}
		return multiPartUpload{
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
			return multiPartUpload{}, err
		}
		return multiPartUpload{
			Filename:        inFilename,
			Sha256:          hashedParts.Sha256,
			Size:            hashedParts.Size,
			TransferSha256:  hashedParts.Sha256,
			TransferSize:    hashedParts.Size,
			ContentEncoding: "identity",
			Parts:           hashedParts.Parts,
		}, nil
	}
}

