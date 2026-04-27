package inetutil

import "io"

type CountingReader struct {
	Reader io.Reader
	Bytes  int64
}

func (r *CountingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.Bytes += int64(n)
	return n, err
}
