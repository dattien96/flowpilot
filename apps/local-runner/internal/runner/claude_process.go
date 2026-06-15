package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Phase 1 (07 plan): the live `claude` CLI process layer + pool. Mirrors the role of
// codex_appserver_process.go, but Claude has no single shared multi-thread process —
// each (account, cwd, session) is its own `claude -p` process, and the Go runner is
// the multiplexer. The default registry keeps Claude a placeholder for demo/tests,
// while the live runner registry wires the real adapter.
//
// MVP process model (explicitly accepted, 07 plan "Final Decision"): spawn-per-turn
// with --resume continuity, NOT a warm long-lived process per session. Each SendTurn
// spawns one process and releases it at turn end; conversation continuity rides the
// captured real Claude session_id via --resume. Warm-process reuse keyed by
// (account,cwd,session) is a documented future optimization. The pool therefore tracks
// in-flight processes (for account-switch teardown) and the synthetic→real session map.

// claudeBinaryName is the CLI binary; overridable for tests.
var claudeBinaryName = func() string {
	if b := strings.TrimSpace(os.Getenv("FLOWPILOT_CLAUDE_BIN")); b != "" {
		return b
	}
	return "claude"
}

// claudeProcKey identifies an in-flight process. Real runs supply a distinct per-run
// session id; before one is known the key collapses to (account, cwd, "") — the pool is
// used for liveness/cleanup, not turn correctness (the adapter holds its own ref).
type claudeProcKey struct {
	account string
	cwd     string
	session string
}

type claudeProcess struct {
	key      claudeProcKey
	stream   *claudeStream
	cmd      *exec.Cmd
	killOnce sync.Once
	waitOnce sync.Once
}

// shutdown stops the reader, kills the process (idempotent), and reaps it exactly once
// so no OS process handle/zombie leaks (review finding 3). Safe to call on an
// already-exited process (Kill errors are ignored; Wait reaps).
func (proc *claudeProcess) shutdown() {
	if proc == nil {
		return
	}
	proc.stream.stop()
	proc.killOnce.Do(func() {
		if proc.cmd != nil && proc.cmd.Process != nil {
			_ = proc.cmd.Process.Kill()
		}
	})
	proc.waitOnce.Do(func() {
		if proc.cmd != nil {
			_ = proc.cmd.Wait()
		}
	})
}

type claudeProcessPool struct {
	mu    sync.Mutex
	procs map[claudeProcKey]*claudeProcess

	// real maps the FlowPilot per-run session id (synthetic, stable across turns) -> the
	// real Claude session_id captured from system/init|result, so later turns --resume
	// the REAL session and never the synthetic id (review finding 1). Lives on the shared
	// pool because the registry builds a fresh adapter per turn.
	realMu sync.Mutex
	real   map[string]string
}

func newClaudeProcessPool() *claudeProcessPool {
	return &claudeProcessPool{procs: map[claudeProcKey]*claudeProcess{}, real: map[string]string{}}
}

// realSession returns the captured real Claude session id for a FlowPilot session id
// (empty until the first turn's session_id has been observed).
func (p *claudeProcessPool) realSession(fpSessionID string) string {
	if fpSessionID == "" {
		return ""
	}
	p.realMu.Lock()
	defer p.realMu.Unlock()
	return p.real[fpSessionID]
}

// setRealSession records the real Claude session id for a FlowPilot session id (first
// write wins — system/init and result carry the same id for a turn).
func (p *claudeProcessPool) setRealSession(fpSessionID, claudeSessionID string) {
	if fpSessionID == "" || claudeSessionID == "" {
		return
	}
	p.realMu.Lock()
	defer p.realMu.Unlock()
	if p.real[fpSessionID] == "" {
		p.real[fpSessionID] = claudeSessionID
	}
}

// spawn starts a fresh `claude` process for one turn. Concurrent sessions => concurrent
// processes (the Go runner is the multiplexer).
func (p *claudeProcessPool) spawn(ctx context.Context, key claudeProcKey, args []string, env map[string]string, cwd string) (*claudeProcess, error) {
	cmd := commandContextFn(ctx, claudeBinaryName(), args...)
	cmd.Env = os.Environ()
	for k, v := range env {
		if strings.TrimSpace(k) != "" {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	if strings.TrimSpace(cwd) != "" {
		cmd.Dir = cwd
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("claude stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("claude stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude start: %w", err)
	}

	stream := newClaudeStream(stdin)
	stream.start(stdout)

	proc := &claudeProcess{key: key, stream: stream, cmd: cmd}
	p.mu.Lock()
	p.procs[key] = proc
	p.mu.Unlock()
	return proc, nil
}

// release tears down a process the caller is done with (turn end / interrupt). Removes
// it from the map only if the map still points to THIS process (so a key collision from
// a concurrent first-turn spawn never drops the wrong one), then shuts it down (kill+reap).
func (p *claudeProcessPool) release(proc *claudeProcess) {
	if proc == nil {
		return
	}
	p.mu.Lock()
	if cur, ok := p.procs[proc.key]; ok && cur == proc {
		delete(p.procs, proc.key)
	}
	p.mu.Unlock()
	proc.shutdown()
}

// dropAccount tears down every in-flight process for an account (the account-switch
// recreate, 07; mirrors the Codex per-account teardown).
func (p *claudeProcessPool) dropAccount(account string) {
	p.mu.Lock()
	var victims []*claudeProcess
	for k, proc := range p.procs {
		if k.account == account {
			victims = append(victims, proc)
			delete(p.procs, k)
		}
	}
	p.mu.Unlock()
	for _, proc := range victims {
		proc.shutdown()
	}
}

func (p *claudeProcessPool) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.procs)
}
