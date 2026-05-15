package transcode

import (
	"bytes"
	"sync"
)

// lockedBuffer is a goroutine-safe bytes.Buffer wrapper. exec.Cmd writes to
// Stdout and Stderr from separate goroutines, so when both point at the same
// sink the sink must serialize its writes.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
