// Package process owns project processes and bounded, concurrent output buffers.
package process

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"rian/internal/tools"
)

const outputLimit = 96 * 1024

type Snapshot struct {
	Command        string
	PID            int
	Started, Ended time.Time
	Running        bool
	Stopped        bool
	Output         string
	Error          string
}

type session struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	state  Snapshot
	buffer []byte
	done   chan struct{}
}

func (s *session) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(data)
	if n >= outputLimit {
		s.buffer = append(s.buffer[:0], data[n-outputLimit:]...)
	} else {
		if len(s.buffer)+n > outputLimit {
			s.buffer = s.buffer[len(s.buffer)+n-outputLimit:]
		}
		s.buffer = append(s.buffer, data...)
	}
	return n, nil
}

func (s *session) snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.state
	state.Output = string(s.buffer)
	return state
}

type Manager struct {
	mu       sync.Mutex
	startMu  sync.Mutex
	sessions map[string]*session
	closed   bool
}

func New() *Manager { return &Manager{sessions: map[string]*session{}} }

func (m *Manager) Start(id, path, command string) (Snapshot, error) {
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return Snapshot{}, fmt.Errorf("this directory is no longer available: %s", path)
	}
	return m.StartCommand(id, command, tools.Shell(path, command))
}

func (m *Manager) StartCommand(id, label string, cmd *exec.Cmd) (Snapshot, error) {
	// Launch/shutdown serialize separately from the map lock. Screen snapshots
	// never wait for an OS process launch (which can be slow on Windows).
	m.startMu.Lock()
	defer m.startMu.Unlock()
	m.mu.Lock()
	closed, existing := m.closed, m.sessions[id]
	m.mu.Unlock()
	if closed {
		return Snapshot{}, fmt.Errorf("process manager is shutting down")
	}
	if existing != nil && existing.snapshot().Running {
		return existing.snapshot(), fmt.Errorf("a command is already running for this project; stop it first")
	}
	s := &session{cmd: cmd, state: Snapshot{Command: label, Started: time.Now()}, done: make(chan struct{})}
	cmd.Stdout, cmd.Stderr = s, s
	prepare(cmd)
	if err := cmd.Start(); err != nil {
		return Snapshot{}, fmt.Errorf("could not start command: %w", err)
	}
	s.state.PID, s.state.Running = cmd.Process.Pid, true
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	go func() {
		err := cmd.Wait()
		s.mu.Lock()
		s.state.Running, s.state.Ended = false, time.Now()
		if err != nil && !s.state.Stopped {
			s.state.Error = err.Error()
		}
		s.mu.Unlock()
		close(s.done)
	}()
	return s.snapshot(), nil
}

func (m *Manager) Snapshot(id string) Snapshot {
	m.mu.Lock()
	s := m.sessions[id]
	m.mu.Unlock()
	if s == nil {
		return Snapshot{}
	}
	return s.snapshot()
}

// Forget releases completed output after its owner has copied it for display.
func (m *Manager) Forget(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.sessions[id]; s != nil && !s.snapshot().Running {
		delete(m.sessions, id)
	}
}

func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	s := m.sessions[id]
	m.mu.Unlock()
	if s == nil || !s.snapshot().Running {
		return nil
	}
	s.mu.Lock()
	s.state.Stopped = true
	s.mu.Unlock()
	if err := stopTree(s.cmd); err != nil {
		s.mu.Lock()
		s.state.Stopped = false
		s.mu.Unlock()
		return err
	}
	select {
	case <-s.done:
		return nil
	case <-time.After(4 * time.Second):
		return fmt.Errorf("process did not exit after stop")
	}
}

func (m *Manager) Active() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, s := range m.sessions {
		if s.snapshot().Running {
			n++
		}
	}
	return n
}

func (m *Manager) Close() {
	m.startMu.Lock()
	m.mu.Lock()
	m.closed = true
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	m.startMu.Unlock()
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id string) { defer wg.Done(); _ = m.Stop(id) }(id)
	}
	wg.Wait()
}
