package artifact

import "errors"

// ErrHTTPS is an error caused by the lack of using HTTPS urls
var ErrHTTPS = errors.New("only resources served over https are allowed")

// ErrCorrupt is an error message which is specifically related to an
// artifact being found to be corrupt
var ErrCorrupt = errors.New("corrupt resource")

var ErrExpectedRedirect = errors.New("expected redirect")
var ErrUnexpectedRedirect = errors.New("unexpected redirect")
var ErrBadRedirect = errors.New("malformed redirect")
var ErrBadOutputWriter = errors.New("output writer is not empty")
