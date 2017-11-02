package artifact

type byteCountingWriter struct {
	count int64
}

func (c *byteCountingWriter) Write(p []byte) (n int, err error) {
	nBytes := len(p)
	c.count += int64(nBytes)
	return nBytes, nil
}
