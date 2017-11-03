# taskcluster-lib-artifact-go

This is a library which provides a standard API to the Taskcluster Queue's new
Blob Artifacts.  A Blob artifact is different from the other types of artifacts
in the Taskcluster Queue because it supports arbitrarily sized (up to the 5TB
limit imposed by Amazon S3) and provides mechanisms for verifying the integrity
of the artifacts stored and retreived.

The work required to *correctly* use the Blob Artifact API is sufficiently
complex that a library is the best way to make it available.  There is also a
library written for Javascript.  This library also supports automatic gzip
content-encoding on upload and automatic gzip decompression on download.

The key details about the Blob Artifact API are that on upload, we compute the
Sha256 value of the artifact before upload, gzip it if needed and then tell the
Queue about the artifact we're about to upload.  The Queue generates the list
of requests signed with AWS V4 signatures which the uploader can run to
complete the upload.

## Usage
You can install this library by running

```bash
go get -v -u -t github.com/taskcluster/taskcluster-lib-artifact-go
```

And then use it as follows:

```go


```

### Input and Output
The input and output parameters are various types of specialized `io.Reader`
and `io.Writer` types.  The minimum interface for use in the specific function
was chosen.  This library does not do any management of the input and output
objects.  They must be created outside of this library and any cleanup must
occur in calling code.  The most common output option is likely an
`ioutil.TempFile()` instance.

The output must be empty.  For methods which require `io.Seeker` implementing
interfaces (e.g. `io.ReadWriteSeeker`), a check that the output is actually
empty happens.  For those which which do not require `io.Seeker`, this
requirement is still present.  In the case of a method which takes an
`io.Writer`, but the output passed in does implement `io.Seeker`, this check is
also performed.
