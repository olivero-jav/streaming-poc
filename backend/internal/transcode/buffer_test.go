package transcode

import (
	"strings"
	"sync"
	"testing"
)

// TestLockedBufferConcurrentWrites is the regression test for the data race
// fix: two real goroutines (exec.Cmd's stdout and stderr) were writing to the
// same bytes.Buffer. This exercises the same pattern with many more writers.
func TestLockedBufferConcurrentWrites(t *testing.T) {
	t.Parallel()

	b := &lockedBuffer{}
	const writers = 16
	const perWriter = 1000
	const payload = "abc"

	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perWriter {
				_, _ = b.Write([]byte(payload))
			}
		}()
	}
	wg.Wait()

	got := b.String()
	wantLen := writers * perWriter * len(payload)
	if len(got) != wantLen {
		t.Errorf("length: got %d, want %d", len(got), wantLen)
	}
	wantCount := writers * perWriter
	if c := strings.Count(got, payload); c != wantCount {
		t.Errorf("payload occurrences: got %d, want %d", c, wantCount)
	}
}
