package process

import (
	"os"
	"os/exec"
	"sync"
	"time"
)

// Registry tracks running FFmpeg processes keyed by stream key.
// It is safe for concurrent use.
type Registry struct {
	mu        sync.Mutex
	processes map[string]*exec.Cmd
}

func NewRegistry() *Registry {
	return &Registry{processes: make(map[string]*exec.Cmd)}
}

// Register stores a started process under the given key.
func (r *Registry) Register(key string, cmd *exec.Cmd) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.processes[key] = cmd
}

// Kill sends os.Interrupt to the process registered under key so FFmpeg can
// finalize the current HLS segment, then forcefully kills it after 5 seconds.
// It is a no-op if no process is registered for that key.
func (r *Registry) Kill(key string) {
	r.mu.Lock()
	cmd, ok := r.processes[key]
	if ok {
		delete(r.processes, key)
	}
	r.mu.Unlock()

	if !ok || cmd.Process == nil {
		return
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		_ = cmd.Process.Kill()
		return
	}

	go func() {
		time.Sleep(5 * time.Second)
		_ = cmd.Process.Kill()
	}()
}
