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
	"github.com/taskcluster/taskcluster-client-go/tcqueue"
)

// TODO implement an in memory 'file'
// TODO implement 'redirect' and 'error' artifact types?

// Client knows how to upload and download blob artifacts
type Client struct {
	agent                   client
	queue                   *tcqueue.Queue
	chunkSize               int
	multipartPartChunkCount int
	AllowInsecure           bool
	clientForBlindRedirects *http.Client
}

// DefaultChunkSize is 128KB
const DefaultChunkSize int = 128 * 1024

// DefaultPartSize is 100MB
const DefaultPartSize int = 100 * 1024 * 1024 / DefaultChunkSize

// So in the ideal world, what we'd do is change this library's agent to
// support content-sha256-secure redirect checking and have it happen for all
// requests which aren't error, reference, s3 or azure artifact types.  This
// would be done by writing a checkRedirect function which verified the
// proposed redirect has valid and correct hashes and lengths then also redoing
// https-checking by looking for a non-nil req.TLS value.  That's a pretty
// large overhaul of how the library itself works, so what we're going to do
// for the timebeing is have a second http client for running these types of
// requests

// New creates a Client for use
func New(queue *tcqueue.Queue) *Client {
	a := newAgent()
	transport := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	}
	_client := &http.Client{
		Transport: transport,
	}
	return &Client{
		agent:                   a,
		queue:                   queue,
		chunkSize:               DefaultChunkSize,
		multipartPartChunkCount: DefaultPartSize,
		clientForBlindRedirects: _client,
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

// CreateError creates an Error artifact.
func (c *Client) CreateError(taskID, runID, name, reason, message string) error {
	errorreq := &tcqueue.ErrorArtifactRequest{
		Expires:     tcclient.Time(time.Now().UTC().AddDate(0, 0, 1)),
		Message:     message,
		Reason:      reason,
		StorageType: "error",
	}

	cap, err := json.Marshal(&errorreq)
	if err != nil {
		panic(newErrorf(err, "serializing json request body for createArtifact queue call during creation of error to %s/%s/%s", taskID, runID, name))
	}

	pareq := tcqueue.PostArtifactRequest(json.RawMessage(cap))

	_, err = c.queue.CreateArtifact(taskID, runID, name, &pareq)
	if err != nil {
		return newErrorf(err, "making createArtifact queue call during error creation of %s/%s/%s", taskID, runID, name)
	}

	return nil
}

// CreateReference creates a Reference artifact.
func (c *Client) CreateReference(taskID, runID, name, url string) error {
	refreq := &tcqueue.RedirectArtifactRequest{
		// What?!? Why does a 302 redirect have a content-type???
		// Since this doesn't really make any sense, we're just going to
		// make up one which is safe
		ContentType: "application/octet-stream",
		Expires:     tcclient.Time(time.Now().UTC().AddDate(0, 0, 1)),
		StorageType: "reference",
		URL:         url,
	}

	cap, err := json.Marshal(&refreq)
	if err != nil {
		panic(newErrorf(err, "serializing json request body for createArtifact queue call during creation of reference %s/%s/%s", taskID, runID, name))
	}

	pareq := tcqueue.PostArtifactRequest(json.RawMessage(cap))

	_, err = c.queue.CreateArtifact(taskID, runID, name, &pareq)
	if err != nil {
		return newErrorf(err, "making createArtifact queue call during reference creation of %s/%s/%s", taskID, runID, name)
	}

	return nil
}

// Upload an artifact.  The contents of input will be copied to the beginning
// of output, optionally with gzip encoding.  Output must be an
// io.ReadWriteSeeker which has 0 bytes (thus position 0).  We need the output
// to be able to Read, Write and Seek because we'll pass over the file one time
// to copy it to the output, then seek back to the beginning and read it in
// again for the upload.  When this artifact is downloaded with this library,
// the resulting output will be written as a once encoded gzip file
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
	// We check for graceful EOF to handle the case of a file which has no contents
	if err != nil && err != io.EOF {
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

	bareq := &tcqueue.BlobArtifactRequest{
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
		parts := make([]tcqueue.MultipartPart, len(u.Parts))
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

	pareq := tcqueue.PostArtifactRequest(json.RawMessage(cap))

	resp, err := c.queue.CreateArtifact(taskID, runID, name, &pareq)
	if err != nil {
		return newErrorf(err, "making createArtifact queue call during upload of %s to %s/%s/%s", findName(input), taskID, runID, name)
	}

	var bares tcqueue.BlobArtifactResponse

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
		var req request
		req, err = newRequestFromStringMap(r.URL, r.Method, r.Headers)
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

		var cs callSummary
		cs, _, err = c.agent.run(req, b, c.chunkSize, &outputBuf, false)
		if err != nil {
			logger.Printf("%s\n%v", cs, &outputBuf)
			return newErrorf(err, "reading bytes %d to %d of %s for %s to %s to upload to %s/%s/%s", start, end, findName(input), r.Method, r.URL, taskID, runID, name)
		}

		outputBuf.Reset()

		etags[i] = cs.ResponseHeader.Get("etag")
	}

	careq := tcqueue.CompleteArtifactRequest{
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

// TODO Support downloading non-blob artifacts

// DownloadURL downloads a URL to the specified output.  Because we generate
// different URLs based on whether we're asking for latest or not DownloadURL
// will take a string that is a Queue URL to an artifact and download it to the
// outputWriter.  If an error occurs during the download, the response body of
// the error message will be written instead of the artifact's content.  This
// is so that we can stream the response to the output instead of buffering it
// in memory.  It is the callers responsibility to delete the contents of the
// output on failure if needed.  If the output also implements the io.Seeker
// interface, a check that the output is already empty will occur.  The most
// common output option is likely an ioutil.TempFile() instance.  If artifact
// is an Error type, the contents of the error message will be written to the
// output and the function will return an ErrErr method.
//
// Based on the value of the x-taskcluster-artifact-storage-type http header on
// the redirect from the queue, the client will handle the download
// appropriately.  This value is what is set as 'storageType' on artifact
// creation.  Error objects write the error message to the output Writer and
// return a non-nil error, ErrErr.  Reference, s3 and azure storage types
// blindly follow redirects and write the response to output.  Blob artifacts
// handle redirections and validation appropriately.
func (c *Client) DownloadURL(u string, output io.Writer) (err error) {

	// If we can stat the output, let's see that the size is 0 bytes.  This is an
	// extra safety check, so we're only going to fail if *can* stat the output
	// and that response indicates an invalid value.
	if s, ok := output.(stater); ok {
		var fi os.FileInfo
		fi, err = s.Stat()
		// We don't care about errors calling Stat().  We'll just ignore the call
		// and continue.  This is an extra check, not a mandatory one
		if err == nil && fi.Size() != 0 {
			return ErrBadOutputWriter
		}
	}

	// If we can seek the output, let's do that and ensure it's 0 bytes. If we
	// encounter an error doing the Seek, we ignore this check.  We only fail if
	// the .Seek() method succeeded but the response was invalid.  This is to be
	// able to handle things like os.Stdout, which implement this interface but
	// which will always return an error when called.  If we can seek the output,
	// let's seek 0 bytes from the end and determine the new offset which is the
	// file's size
	if s, ok := output.(io.Seeker); ok {
		var size int64
		size, err = s.Seek(0, io.SeekEnd)
		if err == nil && size != 0 {
			return ErrBadOutputWriter
		}
	}

	r := newRequest(u, "GET", &http.Header{})

	var redirectBuf bytes.Buffer

	var cs callSummary
	cs, _, err = c.agent.run(r, nil, c.chunkSize, &redirectBuf, false)

	var storageType string
	if cs.ResponseHeader != nil {
		storageType = cs.ResponseHeader.Get("x-taskcluster-artifact-storage-type")
	}

	if err != nil && storageType != "error" {
		logger.Printf("%s\n%v", cs, &redirectBuf)
		return newErrorf(err, "running redirect request for %s", u)
	}

	logger.Printf("Storage Type: %s", storageType)

	// We have enough information at this point to determine if we have an error
	// artifact type and how to handle it if so
	if storageType == "error" {
		_, err = io.Copy(output, &redirectBuf)
		if err != nil {
			return newErrorf(err, "copying redirect buffer to output writer")
		}
		logger.Print("error artifact written")
		return ErrErr
	}

	location := cs.ResponseHeader.Get("Location")

	if location == "" {
		return ErrBadRedirect
	}

	var resourceURL *url.URL
	resourceURL, err = url.Parse(location)
	if err != nil {
		return newErrorf(err, "parsing Location header value %s for %s", location, u)
	}

	if !c.AllowInsecure && resourceURL.Scheme != "https" {
		return ErrHTTPS
	}

	// For the reference, s3 and azure, there's nothing to check or verify.
	if storageType == "reference" || storageType == "s3" || storageType == "azure" {
		logger.Printf("following blind redirect of %s artifact", storageType)
		var resp *http.Response
		resp, err = http.Get(location)
		if err != nil {
			return newErrorf(err, "fetching %s", location)
		}
		// if we have an error closing the body, we should return the error, but only
		// if no other error has already been set
		defer func() {
			closeErr := resp.Body.Close()
			if closeErr != nil && err == nil {
				err = closeErr
			}
		}()
		_, err = io.Copy(output, resp.Body)
		if err != nil {
			return newErrorf(err, "copying %s response body to output", location)
		}
		return nil
	}

	if cs.StatusCode < 300 || cs.StatusCode >= 400 {
		return ErrExpectedRedirect
	}

	// Make sure we release the memory stored in the redirect buffer
	redirectBuf.Reset()

	// Now let's make the required request
	r = newRequest(location, "GET", &http.Header{})

	// Now we're going to request the artifact for real.  We're going to write directly
	// to the outputWriter.  This does mean, unfortunately, that the outputWriter will
	// contain the potatoes.
	cs, _, err = c.agent.run(r, nil, c.chunkSize, output, true)
	if err != nil {
		return
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
// that the output is already empty will occur.  The most common output option
// is likely an ioutil.TempFile() instance.
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

	return c.DownloadURL(url.String(), output)

}

// DownloadLatest will download the named artifact from the latest run of a
// task.  If an error occurs during the download, the response body of the
// error message will be written instead of the artifact's content.  This is so
// that we can stream the response to the output instead of buffering it in
// memory.  It is the callers responsibility to delete the contents of the
// output on failure if needed.  If the output also implements the io.Seeker
// interface, a check that the output is already empty will occur.  The most
// common output option is likely an ioutil.TempFile() instance.
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
	}

	return c.DownloadURL(url.String(), output)
}
