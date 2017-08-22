package runner

import (
	"fmt"
	"http"
)

// Just so that you don't need to import http to create a set of headers
type Headers http.Header

// The request type contains the information needed to run an HTTP method
type Request struct {
	Url     string
	Method  string
	Headers Headers
}

func (r Request) String() string {
	return fmt.Sprintf("%s %s %+v", strings.ToUpper(r.Method), r.Url, r.Headers)
}
