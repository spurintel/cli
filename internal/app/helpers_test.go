package app

import "io"

// bufSink adapts an already-open writer to the lazy-open sink signature the
// streaming helpers take, for tests that stream into a buffer.
func bufSink(w io.Writer) func() (io.Writer, func() error, error) {
	return func() (io.Writer, func() error, error) {
		return w, func() error { return nil }, nil
	}
}
