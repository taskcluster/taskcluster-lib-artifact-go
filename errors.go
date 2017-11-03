package artifact

import "errors"

// ErrHTTPS is returned when a non-https url is involved in a redirect
var ErrHTTPS = errors.New("only resources served over https are allowed")

// ErrCorrupt is returned when an artifact is corrupt
var ErrCorrupt = errors.New("corrupt resource")

// ErrExpectedDirect is returned when a redirect is expected but not received
var ErrExpectedRedirect = errors.New("expected redirect")

// ErrUnexpectedRedirect is returned when we expect a redirect but do not
// receive one
var ErrUnexpectedRedirect = errors.New("unexpected redirect")

// ErrBadRedirect is returned when a malformed redirect is received.  Example
// would be an empty Location: header or a Location: header with an invalid URL
// as its value
var ErrBadRedirect = errors.New("malformed redirect")

// ErrBadOutputWriter is returned when the output writer passed into a function
// was able to be checked for its size and it contained more than 0 bytes
var ErrBadOutputWriter = errors.New("output writer is not empty")
