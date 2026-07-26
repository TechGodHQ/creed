package cmd

import (
	"bytes"
	"sync"
)

// safeBuffer is a goroutine-safe bytes.Buffer for tests that read
// command output while the command is still running in another
// goroutine. cobra's SetOut accepts an io.Writer, so we expose Write
// (guarded) plus a String snapshot for assertions.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func newSafeBuffer() *safeBuffer { return &safeBuffer{} }

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}
