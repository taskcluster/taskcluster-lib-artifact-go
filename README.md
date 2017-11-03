# artifact
--
    import "github.com/taskcluster/taskcluster-lib-artifact-go"

Package artifact provides an interface for working with the Taskcluster Queue's
blob artifacts. Blob artifacts are the way that Taskcluster stores and
distributes the results of a task and replace the old "S3" type of artifact for
storing artifacts in S3. These artifacts have stronger authenticity and
integrity guaruntees than the old type.


### Overview of Blob Artifacts

### Blob artifacts can be between 1 byte and 5GB if uploaded as a single part upload
and between 1 byte and 5TB if uploaded as a multipart upload. To upload this
type of artifact, the uploader must compute the artifacts sha256 and size before
and after optional gzip compression. The sha256 and size values are used by the
### Queue to generate a set of requests which get sent back to the uploader which
can be used to upload the artifact. This ensures that network interuptions or
corruption result in errors when uploading. Once the uploads have completed, the
uploader must tell the Queue that the upload is complete.

### The queue ensures that the sha256 and size values are set as headers on the
### artifacts in S3 so that downloaders can do verification

When downloading a blob artifact, the downloader must verify that the artifact
### downloaded has the same sha256 and sizes before and after optional gzip
decompression.

### Interacting with this API correctly is sufficiently complicated that this
library is the only supported way to upload or download artifacts using Go.


### Input and Output

The input and output parameters are various types of specialized io.Reader and
io.Writer types. The minimum interface for use in the specific function was
chosen. This library does not do any management of the input and output objects.
### They must be created outside of this library and any cleanup must occur in
calling code. The most common output option is likely an ioutil.TempFile()
instance.

The output must be empty. For methods which require io.Seeker implementing
interfaces (e.g. io.ReadWriteSeeker), a check that the output is actually empty
happens. For those which which do not require io.Seeker, this requirement is
still present. In the case of a method which takes an io.Writer, but the output
passed in does implement io.Seeker, this check is also performed. If the passed
io.Writer really does not implement io.Seeker, it is the responsibility of the
### caller to ensure it is refering to an empty resource


### Gzip content encoding

### This package automatically decompresses artifacts which are stored with a
content encoding of 'gzip'. In both uploading and downloading, the gzip encoding
and decoding is done independently of any gzip encoding by the calling code.
This could result in double gzip encoding if a gzip file is passed into Upload()
with the gzip argument set to true. When this artifact is downloaded with this
library, the resulting output will be written as a once encoded gzip file

## Usage

```go
const DefaultChunkSize int = 128 * 1024
```
DefaultChunkSize is 128KB

```go
const DefaultPartSize int = 100 * 1024 * 1024 / DefaultChunkSize
```
DefaultPartSize is 100MB

```go
var ErrBadOutputWriter = errors.New("output writer is not empty")
```
ErrBadOutputWriter is returned when the output writer passed into a function was
able to be checked for its size and it contained more than 0 bytes

```go
var ErrBadRedirect = errors.New("malformed redirect")
```
ErrBadRedirect is returned when a malformed redirect is received. Example would
be an empty Location: header or a Location: header with an invalid URL as its
value

```go
var ErrCorrupt = errors.New("corrupt resource")
```
ErrCorrupt is returned when an artifact is corrupt

```go
var ErrExpectedRedirect = errors.New("expected redirect")
```
ErrExpectedRedirect is returned when a redirect is expected but not received

```go
var ErrHTTPS = errors.New("only resources served over https are allowed")
```
ErrHTTPS is returned when a non-https url is involved in a redirect

```go
var ErrUnexpectedRedirect = errors.New("unexpected redirect")
```
ErrUnexpectedRedirect is returned when we expect a redirect but do not receive
one

#### func  SetLogFlags

```go
func SetLogFlags(f int)
```
SetLogFlags will change the flags which are used by logs in this package This is
a simple convenience method to wrap this package's Logger instance's method.
See: https://golang.org/pkg/log/#Logger.SetFlags

#### func  SetLogOutput

```go
func SetLogOutput(w io.Writer)
```
SetLogOutput will change the prefix used by logs in this package This is a
simple convenience method to wrap this package's Logger instance's method. See:
https://golang.org/pkg/log/#Logger.SetOutput If you wish to unconditionally
supress all logs from this library, you can do the following:

    SetLogOutput(ioutil.Discard)

#### func  SetLogPrefix

```go
func SetLogPrefix(p string)
```
SetLogPrefix will change the prefix used by logs in this package This is a
simple convenience method to wrap this package's Logger instance's method. See:
https://golang.org/pkg/log/#Logger.SetPrefix

#### type Client

```go
type Client struct {
}
```

Client knows how to upload and download blob artifacts

#### func  New

```go
func New(queue *queue.Queue) *Client
```
New creates a Client for use

#### func (*Client) Download

```go
func (c *Client) Download(taskID, runID, name string, output io.Writer) error
```
Download will download the named artifact from a specific run of a task. If an
error occurs during the download, the response body of the error message will be
written instead of the artifact's content. This is so that we can stream the
response to the output instead of buffering it in memory. It is the callers
responsibility to delete the contents of the output on failure if needed. If the
output also implements the io.Seeker interface, a check that the output is
already empty will occur

#### func (*Client) DownloadLatest

```go
func (c *Client) DownloadLatest(taskID, name string, output io.Writer) error
```
DownloadLatest will download the named artifact from the latest run of a task.
If an error occurs during the download, the response body of the error message
will be written instead of the artifact's content. This is so that we can stream
the response to the output instead of buffering it in memory. It is the callers
responsibility to delete the contents of the output on failure if needed. If the
output also implements the io.Seeker interface, a check that the output is
already empty will occur

#### func (*Client) GetInternalSizes

```go
func (c *Client) GetInternalSizes() (int, int)
```
GetInternalSizes returns the chunkSize and partSize, respectively, for this
Client.

#### func (*Client) SetInternalSizes

```go
func (c *Client) SetInternalSizes(chunkSize, partSize int) error
```
SetInternalSizes sets the chunkSize and partSize . The chunk size is the number
of bytes that this library will read and write in a single IO operation. In a
multi-part upload, the whole file is broken into smaller portions. Each of these
portions can be uploaded simultaneously. For the sake of simplicity, the part
size must be a multiple of the chunk size so that we don't have to worry about
each individual read or write being split across more than one part. Both are
changed in a single call because the partSize must always be a multiple of the
chunkSize

#### func (*Client) Upload

```go
func (c *Client) Upload(taskID, runID, name string, input io.ReadSeeker, output io.ReadWriteSeeker, gzip, multipart bool) error
```
Upload an artifact. The contents of input will be copied to the beginning of
output, optionally with gzip encoding. Output must be an io.ReadWriteSeeker
which has 0 bytes (thus position 0). We need the output to be able to Read,
Write and Seek because we'll pass over the file one time to copy it to the
output, then seek back to the beginning and read it in again for the upload
