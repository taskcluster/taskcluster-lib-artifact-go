package artifact

// ErrHTTPS is returned when a non-https url is involved in a redirect
var ErrHTTPS = newError(nil, "only resources served over https are allowed")

// ErrCorrupt is returned when an artifact is corrupt
var ErrCorrupt = newError(nil, "corrupt resource")

// ErrExpectedRedirect is returned when a redirect is expected but not received
var ErrExpectedRedirect = newError(nil, "expected redirect")

// ErrUnexpectedRedirect is returned when we expect a redirect but do not
// receive one
var ErrUnexpectedRedirect = newError(nil, "unexpected redirect")

// ErrBadRedirect is returned when a malformed redirect is received.  Example
// would be an empty Location: header or a Location: header with an invalid URL
// as its value
var ErrBadRedirect = newError(nil, "malformed redirect")

// ErrBadOutputWriter is returned when the output writer passed into a function
// was able to be checked for its size and it contained more than 0 bytes
var ErrBadOutputWriter = newError(nil, "output writer is not empty")

var ErrBadSize = newError(nil, "invalid part or chunk size")
