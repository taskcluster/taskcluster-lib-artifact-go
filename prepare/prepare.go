package prepare

import (
	"crypto/sha256"
	"os"
  "fmt"
  "math"
)

type SinglePartFileInfo struct {
  Sha256 []byte
  Size int64
}

// Returns a buffer which represents a SHA256 checksum of the requested file
func HashFile(filename string, chunkSize int) SinglePartFileInfo {
	// Create a file handle
	f, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
  defer f.Close()

  // Determine the filesize
  fi, err := f.Stat()
  if err != nil {
    panic(err)
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
			panic(err)
		}

    // NOTE: Per docs, this function never returns an error
		hash.Write(buf[:nBytes])

    // Even though nBytes is quite small compared to an int64, we must cast it
    // because go will (thankfully) require that the types are the same to add
    // them
		totalBytes += int64(nBytes)

	}

  if totalBytes != size {
    panic(fmt.Errorf("File size changed during hashing from %d to %d", size, totalBytes))
  }
  if fi, err := f.Stat() ; err == nil {
    if size != fi.Size() {
      panic(fmt.Errorf("File size changed during hashing from %d to %d", size, fi.Size()))
    }
  } else {
    panic(err)
  }

	return SinglePartFileInfo{hash.Sum(nil), totalBytes}
}


type Part struct {
  Sha256 []byte
  Size int64
  Start int64
}

type MultiPartFileInfo struct {
  Sha256 []byte
  Size int64
  Parts []Part
}

// Hash a file, but also figure out the hashes of sub parts.  A sub part is the
// number of bytes obtained by multiplying chunkSize by chunksInPart.  This is
// done to simplify the calculation of the parts
func HashFileParts(filename string, chunkSize, chunksInPart int) MultiPartFileInfo {
	// Create a file handle
	f, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
  defer f.Close()

  // Determine the filesize
  fi, err := f.Stat()
  if err != nil {
    panic(err)
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
      parts[len(parts) - 1] = Part{partHash.Sum(nil), currentPartSize, int64(totalParts - 1) * partSize}
			break
		}
		if err != nil {
			panic(err)
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
    panic(fmt.Errorf("File size changed during hashing from %d to %d", size, totalBytes))
  }
  if fi, err := f.Stat() ; err == nil {
    if size != fi.Size() {
      panic(fmt.Errorf("File size changed during hashing from %d to %d", size, fi.Size()))
    }
  } else {
    panic(err)
  }

	return MultiPartFileInfo{hash.Sum(nil), totalBytes, parts}
}
