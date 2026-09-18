package lsp

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// defaultHealthInterval is how often a running server is pinged for signs
// of life. Tests override Manager.healthInterval (same package) for speed.
const defaultHealthInterval = 30 * time.Second

// maxAutoRestarts caps crash recovery per session (CP-63 §3.4): after the
// budget is spent the server stays disabled with a warning instead of
// respawn-looping.
const maxAutoRestarts = 1

// shutdownGrace is the best-effort window for a graceful LSP shutdown
// before the process is killed.
const shutdownGrace = 2 * time.Second

// FileURI converts a filesystem path to a file:// URI for LSP messages.
func FileURI(path string) string {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// ServerManager owns one LSP server OS process: spawn, stop, restart and
// health monitoring. Stdin/stdout are piped into a Client (Task-354);
// stderr is drained to the log. At most one server runs per manager —
// multi-workspace/multi-language fan-out lives one layer up (Task-358).
//
// The zero value is usable; Start fills in the lifecycle state.
type ServerManager struct {
	mu  sync.Mutex
	cmd *exec.Cmd
	// Env holds extra KEY=VALUE pairs appended to the server process
	// environment (base is os.Environ), e.g. GOPROXY/GOFLAGS for gopls.
	Env []string
	// InitOptions, when non-nil, is sent as initialize.initializationOptions
	// by WaitReady (e.g. per-workspace Kotlin/Gradle settings from Task-360).
	InitOptions    map[string]any
	stdin          io.WriteCloser
	client         *Client
	binary         string
	args           []string
	root           string
	running        bool
	disabled       bool
	restarts       int
	done           chan struct{} // closed when the current process exits
	stopHealth     chan struct{}
	healthInterval time.Duration
}

// Start spawns binary with args rooted at workspaceRoot and pipes it into a
// fresh Client. It fails if a server is already running (call Stop or
// Restart for an explicit handover).
func (m *ServerManager) Start(ctx context.Context, binary string, args []string, workspaceRoot string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return fmt.Errorf("lsp: server already running")
	}
	if m.healthInterval == 0 {
		m.healthInterval = defaultHealthInterval
	}
	if err := m.startProcessLocked(binary, args, workspaceRoot); err != nil {
		return err
	}
	log.Printf("[lsp] lsp.start binary=%q root=%q", binary, workspaceRoot)
	return nil
}

// startProcessLocked spawns the process and arms the watcher/health loop.
// Caller holds m.mu.
func (m *ServerManager) startProcessLocked(binary string, args []string, workspaceRoot string) error {
	_ = context.Background() // lifecycle is explicit via Stop; ctx kept for API symmetry
	cmd := exec.Command(binary, args...)
	if workspaceRoot != "" {
		cmd.Dir = workspaceRoot
	}
	// Caller holds m.mu; read Env without re-locking (sync.Mutex is not
	// reentrant).
	if len(m.Env) > 0 {
		cmd.Env = append(os.Environ(), append([]string(nil), m.Env...)...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("lsp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("lsp: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("lsp: stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("lsp: spawn %q: %w", binary, err)
	}
	m.cmd = cmd
	m.stdin = stdin
	m.client = NewClient(stdin, stdout)
	m.binary = binary
	m.args = append([]string(nil), args...)
	m.root = workspaceRoot
	m.running = true
	m.disabled = false // explicit Start revives a crash-disabled manager
	m.done = make(chan struct{})
	done := m.done
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	go pumpStderr(binary, stderr)
	m.stopHealth = make(chan struct{})
	stop := m.stopHealth
	go m.healthLoop(stop)
	return nil
}

// Stop shuts the server down (graceful handshake first, kill as backstop)
// and releases the lifecycle. It is idempotent.
func (m *ServerManager) Stop() error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return nil
	}
	cmd := m.cmd
	client := m.client
	stdin := m.stdin
	if m.stopHealth != nil {
		close(m.stopHealth)
		m.stopHealth = nil
	}
	m.running = false
	done := m.done
	m.mu.Unlock()

	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		_ = client.Shutdown(ctx) // best effort; the process may already be gone
		cancel()
		client.Close()
	}
	if stdin != nil {
		_ = stdin.Close() // many servers exit on stdin EOF
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if done != nil {
		<-done // the watcher reaps via Wait; never call Wait here too
	}
	log.Printf("[lsp] lsp.stop")
	return nil
}

// Restart performs an explicit Stop + Start with the same binary/args/root.
// Unlike crash recovery it never consumes the auto-restart budget and it
// works even after a manual Stop.
func (m *ServerManager) Restart() error {
	m.mu.Lock()
	binary, args, root := m.binary, append([]string(nil), m.args...), m.root
	wasConfigured := binary != ""
	m.mu.Unlock()
	if !wasConfigured {
		return fmt.Errorf("lsp: nothing to restart")
	}
	if err := m.Stop(); err != nil {
		return err
	}
	return m.Start(context.Background(), binary, args, root)
}

// IsRunning reports whether a live server process is currently owned.
func (m *ServerManager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running || m.disabled || m.done == nil {
		return false
	}
	select {
	case <-m.done:
		return false
	default:
		return true
	}
}

// Client returns the Client piped to the current server process, or nil if
// never started. Callers must check IsRunning first: after a crash the
// returned client may be dead until a restart replaces it.
func (m *ServerManager) Client() *Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.client
}

// Restarts reports how many crash recoveries happened this session.
func (m *ServerManager) Restarts() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.restarts
}

// WaitReady blocks until the initialize handshake with the running server
// completes or timeout elapses.
func (m *ServerManager) WaitReady(ctx context.Context, timeout time.Duration) error {
	m.mu.Lock()
	client := m.client
	root := m.root
	initOptions := m.InitOptions
	m.mu.Unlock()
	if client == nil || !m.IsRunning() {
		return fmt.Errorf("lsp: server not running")
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	params := InitializeParams{Capabilities: ClientCapabilities{}}
	if strings.TrimSpace(root) != "" {
		uri := FileURI(root)
		params.RootURI = &uri
	}
	if len(initOptions) > 0 {
		params.InitializationOptions = initOptions
	}
	if _, err := client.Initialize(ctx, params); err != nil {
		return fmt.Errorf("lsp: wait ready: %w", err)
	}
	return nil
}

// healthLoop watches the current process and recovers from crashes within
// the per-session budget. It exits when stop is closed (Stop) or after the
// server is disabled.
func (m *ServerManager) healthLoop(stop chan struct{}) {
	t := time.NewTicker(m.healthIntervalForLoop())
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-m.doneForLoop():
			m.mu.Lock()
			if !m.running {
				// Intentional Stop won the race; nothing to do.
				m.mu.Unlock()
				return
			}
			if m.restarts >= maxAutoRestarts {
				m.running = false
				m.disabled = true
				log.Printf("[lsp] lsp.disabled binary=%q root=%q (crash budget spent)", m.binary, m.root)
				m.mu.Unlock()
				return
			}
			m.restarts++
			binary, args, root := m.binary, append([]string(nil), m.args...), m.root
			m.cmd = nil
			m.stopHealth = nil // this loop exits; the replacement arms its own
			if err := m.startProcessLocked(binary, args, root); err != nil {
				m.running = false
				m.disabled = true
				log.Printf("[lsp] lsp.disabled binary=%q root=%q (restart failed: %v)", binary, root, err)
				m.mu.Unlock()
				return
			}
			log.Printf("[lsp] lsp.restart binary=%q root=%q (attempt %d)", binary, root, m.restarts)
			m.mu.Unlock()
			return
		case <-t.C:
			if !m.IsRunning() {
				// Process died without the watcher firing yet; let the done
				// branch handle recovery on its next select pass.
				continue
			}
		}
	}
}

// healthIntervalForLoop snapshots the interval without holding the lock
// across the loop's lifetime.
func (m *ServerManager) healthIntervalForLoop() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.healthInterval <= 0 {
		return defaultHealthInterval
	}
	return m.healthInterval
}

// doneForLoop snapshots the current generation's done channel.
func (m *ServerManager) doneForLoop() <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.done == nil {
		never := make(chan struct{})
		return never
	}
	return m.done
}

// pumpStderr drains a server's stderr to the log so a chatty server can
// never block on a full pipe buffer.
func pumpStderr(binary string, r io.Reader) {
	buf := make([]byte, 4096)
	var line []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			line = append(line, buf[:n]...)
			for {
				idx := -1
				for i, b := range line {
					if b == '\n' {
						idx = i
						break
					}
				}
				if idx < 0 {
					break
				}
				log.Printf("[lsp:stderr:%s] %s", binary, strings.TrimSpace(string(line[:idx])))
				line = append(line[:0], line[idx+1:]...)
			}
		}
		if err != nil {
			if len(line) > 0 {
				log.Printf("[lsp:stderr:%s] %s", binary, strings.TrimSpace(string(line)))
			}
			return
		}
	}
}
