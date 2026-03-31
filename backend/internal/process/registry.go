package process

import (
	"os/exec"
	"sync"
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

// Kill sends SIGKILL to the process registered under key and removes it.
// It is a no-op if no process is registered for that key.
func (r *Registry) Kill(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cmd, ok := r.processes[key]
	if !ok {
		return
	}
	delete(r.processes, key)
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
