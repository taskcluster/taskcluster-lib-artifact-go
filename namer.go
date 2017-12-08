package artifact

type namer interface {
	Name() string
}

// It'd be neat if the newErrorf would do this automatically to allow
// io.Reader/io.Writer's which are files to format down to a string instead of
// needing this function
// Maybe also do this for requests
func findName(n interface{}) string {
	switch v := n.(type) {
	case namer:
		return v.Name()
	default:
		return "<unnamed>"
	}
}
