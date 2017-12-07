// Package artifact provides an interface for working with the Taskcluster
// Queue's blob artifacts.  Blob artifacts are the way that Taskcluster stores
// and distributes the results of a task and replace the old "S3" type of
// artifact for storing artifacts in S3.  These artifacts have stronger
// authenticity and integrity guaruntees than the former type.
//
// Overview of Blob Artifacts
//
// Blob artifacts can be between 1 byte and 5GB if uploaded as a single part
// upload and between 1 byte and 5TB if uploaded as a multipart upload.  To
// upload this type of artifact, the uploader must compute the artifact's
// sha256 and size before and after optional gzip compression.  The sha256 and
// size values are used by the Queue to generate a set of requests which get
// sent back to the uploader which can be used to upload the artifact.  This
// ensures that network interuptions or corruption result in errors when
// uploading.  Once the uploads have completed, the uploader must tell the
// Queue that the upload is complete.
//
// The queue ensures that the sha256 and size values are set as headers on the
// artifacts in S3 so that downloaded content can be verified.
//
// While downloading, the downloader should be counting the number of bytes as
// well as hashing the incoming artifact to determine the sha256 and size to compare
// to the expected values on completion of the request.  It is imperative that
// the downloader perform these verifications
//
// Interacting with this API correctly is sufficiently complicated that this
// library is the only supported way to upload or download artifacts using Go.
//
// Input and Output
//
// The input and output parameters are various types of specialized io.Reader
// and io.Writer types.  The minimum interface for use in the specific function
// was chosen.  This library does not do any management of the input and output
// objects.  They must be created outside of this library and any cleanup must
// occur in calling code.  The most common output option is likely an
// ioutil.TempFile() instance.
//
// The output must be empty.  For methods which require io.Seeker implementing
// interfaces (e.g. io.ReadWriteSeeker), a check that the output is actually
// empty happens.  For those which which do not require io.Seeker, this
// requirement is still present.  In the case of a method which takes an
// io.Writer, but the output passed in does implement io.Seeker, this check is
// also performed.  If the passed io.Writer really does not implement
// io.Seeker, it is the responsibility of the caller to ensure it is refering
// to an empty resource
//
// Gzip content encoding
//
// This package automatically decompresses artifacts which are stored with a
// content encoding of 'gzip'.  In both uploading and downloading, the gzip
// encoding and decoding is done independently of any gzip encoding by the
// calling code.  This could result in double gzip encoding if a gzip file is
// passed into Upload() with the gzip argument set to true.
package artifact
