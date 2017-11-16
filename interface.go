package artifact

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	tcclient "github.com/taskcluster/taskcluster-client-go"
	queue "github.com/taskcluster/taskcluster-client-go/queue"
)

// TODO implement an in memory 'file'
// TODO implement 'redirect' and 'error' artifact types?

// We need this different from the request.go:request type because that struct
// uses http.Header headers and our api returns a different type of headers.
// This would be a great cleanup one day
type apiRequest struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
}

// blobArtifactResponse is the response from queue.CreateArtifact
type blobArtifactResponse struct {
	StorageType string        `json:"storageType"`
	Requests    []apiRequest  `json:"requests"`
	Expires     tcclient.Time `json:"expires"`
}

// Client knows how to upload and download blob artifacts
type Client struct {
	agent                   client
	queue                   *queue.Queue
	chunkSize               int
	multipartPartChunkCount int
	AllowInsecure           bool
}

// DefaultChunkSize is 128KB
const DefaultChunkSize int = 128 * 1024

// DefaultPartSize is 100MB
const DefaultPartSize int = 100 * 1024 * 1024 / DefaultChunkSize

// New creates a Client for use
func New(queue *queue.Queue) *Client {
	a := newAgent()
	return &Client{
		agent:                   a,
		queue:                   queue,
		chunkSize:               DefaultChunkSize,
		multipartPartChunkCount: DefaultPartSize,
	}
}

// SetInternalSizes sets the chunkSize and partSize .  The chunk size is the
// number of bytes that this library will read and write in a single IO
// operation.  In a multipart upload, the whole file is broken into smaller
// portions.  Each of these portions can be uploaded simultaneously.  For the
// sake of simplicity, the part size must be a multiple of the chunk size so
// that we don't have to worry about each individual read or write being split
// across more than one part.  Both are changed in a single call because the
// partSize must always be a multiple of the chunkSize
func (c *Client) SetInternalSizes(chunkSize, partSize int) error {
	if partSize < 5*1024*1024 {
		return newErrorf(nil, "part size %d is not minimum of 5MB", partSize)
	}

	if chunkSize < 1024 {
		return newErrorf(nil, "chunk size %d is not minimum of 1KB", chunkSize)
	}

	if partSize%chunkSize != 0 {
		return newErrorf(nil, "part size %d is not divisible by chunk size %d", partSize, chunkSize)
	}

	c.chunkSize = chunkSize
	c.multipartPartChunkCount = partSize / chunkSize
	return nil
}

// GetInternalSizes returns the chunkSize and partSize, respectively, for this
// Client.
func (c *Client) GetInternalSizes() (int, int) {
	return c.chunkSize, c.multipartPartChunkCount * c.chunkSize
}

// Upload an artifact.  The contents of input will be copied to the beginning
// of output, optionally with gzip encoding.  Output must be an
// io.ReadWriteSeeker which has 0 bytes (thus position 0).  We need the output
// to be able to Read, Write and Seek because we'll pass over the file one time
// to copy it to the output, then seek back to the beginning and read it in
// again for the upload
func (c *Client) Upload(taskID, runID, name string, input io.ReadSeeker, output io.ReadWriteSeeker, gzip, multipart bool) error {

	// Let's check if the output has data already.  The idea here is that if we
	// seek to the end of the io.ReadWriteSeeker and the new position is not 0,
	// we know that there's data.  It's safe to not seek back to 0 from the
	// io.SeekStart because we just asserted that there's 0 bytes in the
	// io.ReadWriteSeeker, so we know that it's position is 0
	outSize, err := output.Seek(0, io.SeekEnd)
	if err != nil {
		return newErrorf(err, "seeking output %s to start for upload", findName(input))
	}
	if outSize != 0 {
		return ErrBadOutputWriter
	}

	// TODO: Decide if we should do this or let the caller figure out the content
	// type themselves.  Realistically, this is more likely to get it right, so
	// I'm really tempted to leave it in and not add another parameter
	//
	// Let's determine the content type of the file.  The mimetype sniffer only looks at
	// the first 512 bytes, so let's read those and then seek the input back to 0
	mimeBuf := make([]byte, 512)
	_, err = input.Read(mimeBuf)
	if err != nil {
		return newErrorf(err, "reading 512 bytes from %s to determine mime type", findName(input))
	}
	_, err = output.Seek(0, io.SeekStart)
	if err != nil {
		return newErrorf(err, "seeking %s back to start after determining mime type", findName(input))
	}
	contentType := http.DetectContentType(mimeBuf)

	var u upload

	if multipart {
		u, err = multipartUpload(input, output, gzip, c.chunkSize, c.multipartPartChunkCount)
		if err != nil {
			return newErrorf(err, "preparing multipart upload of %s to %s/%s/%s", findName(input), taskID, runID, name)
		}
	} else {
		u, err = singlePartUpload(input, output, gzip, c.chunkSize)
		if err != nil {
			return newErrorf(err, "preparing single-part upload of %s to %s/%s/%s", findName(input), taskID, runID, name)
		}
	}

	bareq := &queue.BlobArtifactRequest{
		ContentEncoding: u.ContentEncoding,
		ContentLength:   u.Size,
		ContentSha256:   hex.EncodeToString(u.Sha256),
		TransferLength:  u.TransferSize,
		TransferSha256:  hex.EncodeToString(u.TransferSha256),
		ContentType:     contentType,
		Expires:         tcclient.Time(time.Now().UTC().AddDate(0, 0, 1)),
		StorageType:     "blob",
	}

	if multipart {
		// We don't match the API's structure exactly, so let's do that
		parts := make([]struct {
			Sha256 string "json:\"sha256,omitempty\""
			Size   int64  "json:\"size,omitempty\""
		}, len(u.Parts))
		for i := 0; i < len(u.Parts); i++ {
			parts[i].Sha256 = hex.EncodeToString(u.Parts[i].Sha256)
			parts[i].Size = u.Parts[i].Size
		}
		bareq.Parts = parts
	}

	cap, err := json.Marshal(&bareq)
	if err != nil {
		return newErrorf(err, "serializing json request body for createArtifact queue call during upload of %s to %s/%s/%s", findName(input), taskID, runID, name)
	}

	pareq := queue.PostArtifactRequest(json.RawMessage(cap))

	resp, err := c.queue.CreateArtifact(taskID, runID, name, &pareq)
	if err != nil {
		return newErrorf(err, "making createArtifact queue call during upload of %s to %s/%s/%s", findName(input), taskID, runID, name)
	}

	var bares blobArtifactResponse

	err = json.Unmarshal(*resp, &bares)
	if err != nil {
		return newErrorf(err, "parsing json response body for createArtifact queue call during upload of %s to %s/%s/%s", findName(input), taskID, runID, name)
	}

	etags := make([]string, len(bares.Requests))

	// There's a bit of a difficulty that's going to happen when we start
	// supporting concurrency here.  The underlying ReadSeeker is going to be
	// changing the position in the stream for the other readers.  We're going to
	// have to figure out something to prevent the file from being read from
	// totally random places.  To support this concurrency without passing files
	// (e.g.  using ReadSeekers) we could do something like the following:
	//   1. Create a mutex for file reads
	//   2. Each read to the file will lock the mutex
	//   3. Each read to the file will seek to the correct position
	//   4. Each read to the file will read the number of bytes needed
	//   5. Each reader of the file will keep track of the next place it needs to
	//      read from (e.g. where it seek'ed to + the number of bytes that it read)
	//   6. Each read to the file will unlock the mutex
	// Another option would be to pass in a factory method instead of raw
	// ReadSeekers and have the factory return a ReadSeeker for each
	// request body.  Maybe we really need a ReaderAtSeekCloser...
	for i, r := range bares.Requests {
		req, err := newRequestFromStringMap(r.URL, r.Method, r.Headers)
		if err != nil {
			return newErrorf(err, "creating request %s to %s for upload of %s to %s/%s/%s", r.Method, r.URL, findName(input), taskID, runID, name)
		}

		var b *body

		var start int64
		var end int64

		if u.Parts == nil {
			start = 0
			end = u.TransferSize
		} else {
			start = u.Parts[i].Start
			end = u.Parts[i].Size
		}

		b, err = newBody(output, start, end)
		if err != nil {
			return newErrorf(err, "creating body for bytes %d to %d for upload of %s to %s/%s/%s", start, end, findName(input), taskID, runID, name)
		}

		// In this case, we're going to store the output of the request in memory
		// because we're pretty sure in this method that it's going to be an S3
		// error message and we'd like to print that
		var outputBuf bytes.Buffer

		cs, _, err := c.agent.run(req, b, c.chunkSize, &outputBuf, false)
		if err != nil {
			logger.Printf("%s\n%s", cs, outputBuf.String())
			return newErrorf(err, "reading bytes %d to %d of %s for %s to %s to upload to %s/%s/%s", start, end, findName(input), r.Method, r.URL, taskID, runID, name)
		}

		outputBuf.Reset()

		etags[i] = cs.ResponseHeader.Get("etag")
	}

	careq := queue.CompleteArtifactRequest{
		Etags: etags,
	}

	err = c.queue.CompleteArtifact(taskID, runID, name, &careq)
	if err != nil {
		return newErrorf(err, "completing artifact upload of %s to %s/%s/%s", findName(input), taskID, runID, name)
	}

	logger.Printf("Etags: %#v", etags)
	return nil

}

type stater interface {
	Stat() (os.FileInfo, error)
}

// TODO split this function into a redirect and download section so that we can
// have a download-raw function in the interface and matching command in the
// CLI which lets us do all the verification logic on arbitrary urls

// TODO Support downloading non-blob artifacts

// Because we generate different URLs based on whether we're asking for latest
// or not
func (c *Client) download(u string, outputWriter io.Writer) error {

	// If we can stat the output, let's do that and ensure it's 0 bytes,
	// but we sholdn't fail if we're unable to actually stat the file.
	if s, ok := outputWriter.(stater); ok {
		fi, err := s.Stat()
		if err == nil {
			if fi.Size() != 0 {
				return ErrBadOutputWriter
			}
		}
	}

	// If we can seek the output, let's seek 0 bytes from the end and determine
	// the new offset which is the file's size, but we shouldn't fail if we can't
	// seek the file
	if s, ok := outputWriter.(io.Seeker); ok {
		size, err := s.Seek(0, io.SeekEnd)
		if err == nil {
			if size != 0 {
				return ErrBadOutputWriter
			}
		}
	}

	request := newRequest(u, "GET", &http.Header{})

	var redirectBuf bytes.Buffer

	cs, _, err := c.agent.run(request, nil, c.chunkSize, &redirectBuf, false)
	if err != nil {
		logger.Printf("%s\n%s", cs, redirectBuf.String())
		return newErrorf(err, "running redirect request for %s", u)
	}

	if cs.StatusCode < 300 || cs.StatusCode >= 400 {
		return ErrExpectedRedirect
	}

	// Make sure we release the memory stored in the redirect buffer
	redirectBuf.Reset()

	location := cs.ResponseHeader.Get("Location")

	if location == "" {
		return ErrBadRedirect
	}

	resourceURL, err := url.Parse(location)
	if err != nil {
		return newErrorf(err, "parsing Location header value %s for %s", location, u)
	}

	if !c.AllowInsecure && resourceURL.Scheme != "https" {
		return ErrHTTPS
	}

	request = newRequest(location, "GET", &http.Header{})

	// Now we're going to request the artifact for real.  We're going to write directly
	// to the outputWriter.  This does mean, unfortunately, that the outputWriter will
	// contain the
	cs, _, err = c.agent.run(request, nil, c.chunkSize, outputWriter, true)
	if err != nil {
		return err
	}

	if cs.StatusCode >= 300 {
		return ErrUnexpectedRedirect
	}

	return nil
}

// Download will download the named artifact from a specific run of a task.  If
// an error occurs during the download, the response body of the error message
// will be written instead of the artifact's content.  This is so that we can
// stream the response to the output instead of buffering it in memory.  It is
// the callers responsibility to delete the contents of the output on failure
// if needed.  If the output also implements the io.Seeker interface, a check
// that the output is already empty will occur
func (c *Client) Download(taskID, runID, name string, output io.Writer) error {
	// We need to build the URL because we're going to need to get the redirect's
	// headers.  That's not possible with the q.GetArtifact() method.  Ideally,
	// we'd have a q.GetArtifact_BuildURL method which would allow us to do
	// unauthenticated requests for those resources which have a name starting
	// with "public/"

	// TODO: How long should this signed url really be valid for?
	url, err := c.queue.GetArtifact_SignedURL(taskID, runID, name, time.Duration(3)*time.Hour)
	if err != nil {
		return newErrorf(err, "creating signed URL for %s/%s/%s", taskID, runID, name)
	}

	return c.download(url.String(), output)

}

// DownloadLatest will download the named artifact from the latest run of a
// task.  If an error occurs during the download, the response body of the
// error message will be written instead of the artifact's content.  This is so
// that we can stream the response to the output instead of buffering it in
// memory.  It is the callers responsibility to delete the contents of the
// output on failure if needed.  If the output also implements the io.Seeker
// interface, a check that the output is already empty will occur
func (c *Client) DownloadLatest(taskID, name string, output io.Writer) error {
	// We need to build the URL because we're going to need to get the redirect's
	// headers.  That's not possible with the q.GetArtifact() method.  Ideally,
	// we'd have a q.GetArtifact_BuildURL method which would allow us to do
	// unauthenticated requests for those resources which have a name starting
	// with "public/"

	// TODO: How long should this signed url really be valid for?
	url, err := c.queue.GetLatestArtifact_SignedURL(taskID, name, time.Duration(1)*time.Hour)
	if err != nil {
		return newErrorf(err, "creating signed URL for %s/latest/%s", taskID, name)
		return err
	}

	return c.download(url.String(), output)
}
