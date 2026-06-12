package runner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// Phase 3/6 (04-03 / 04-06): the live shared `codex app-server` process layer.
//
// This is the runner-owned side that the async dispatcher + adapter (already
// process-agnostic and tested over in-memory pipes) plug into. It spawns one
// `codex app-server --listen stdio://`, runs `initialize` to negotiate the protocol
// version + capabilities, and constructs the shared codexAdapter wired to the
// process's stdio. The handle is bound to a scope key (the active provider account,
// 04-06); switching the account tears it down and re-ensures a fresh process.
//
// Enablement: gated behind FLOWPILOT_CODEX_APPSERVER so the default registry stays
// on the fake adapter (the demo/tests stay green without a real codex binary). When
// enabled and `codex` is on PATH, the live registry returns this adapter.

const codexAppServerEnvFlag = "FLOWPILOT_CODEX_APPSERVER"

// codexAppServerEnabled reports whether the live Codex app-server path is turned on.
func codexAppServerEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(codexAppServerEnvFlag)))
	return v == "1" || v == "true" || v == "yes"
}

// codexBinaryName is the app-server binary; overridable for tests.
var codexBinaryName = func() string {
	if b := strings.TrimSpace(os.Getenv("FLOWPILOT_CODEX_BIN")); b != "" {
		return b
	}
	return "codex"
}

// codexInitTimeout bounds the initialize handshake so a wedged build can't hang the
// ensure (the dispatcher degrades gracefully rather than blocking forever).
var codexInitTimeout = 30 * time.Second

type codexAppServerHandle struct {
	scopeKey   string
	dispatcher *codexDispatcher
	adapter    *codexAdapter
	caps       map[string]any
	kill       func()
}

// supports reports whether the negotiated capabilities advertise a feature; unknown
// → assume supported (optimistic) so a build that omits the caps map still works,
// while a build that explicitly disables a method is respected (graceful degrade).
func (h *codexAppServerHandle) supports(feature string) bool {
	caps, _ := h.caps["capabilities"].(map[string]any)
	if caps == nil {
		return true
	}
	v, ok := caps[feature]
	if !ok {
		return true
	}
	b, _ := v.(bool)
	return b
}

func (h *codexAppServerHandle) close() {
	if h == nil {
		return
	}
	if h.dispatcher != nil {
		h.dispatcher.fail(fmt.Errorf("codex app-server torn down"))
	}
	if h.kill != nil {
		h.kill()
	}
}

// ensureCodexAppServer returns the shared app-server handle for a scope, spawning a
// fresh `codex app-server` (and tearing down any handle bound to a different scope —
// the account-switch recreate, 04-06) when needed. Reuses a live handle for the same
// scope. The dispatcher/adapter wiring is identical to the pipe-tested path; only
// the transport is a real subprocess here.
func (r *Runner) ensureCodexAppServer(ctx context.Context, scopeKey, cwd string) (*codexAppServerHandle, error) {
	r.codexAppServerMu.Lock()
	defer r.codexAppServerMu.Unlock()

	if r.codexAppServer != nil && r.codexAppServer.scopeKey == scopeKey && !r.codexAppServer.dispatcher.isClosed() {
		return r.codexAppServer, nil
	}
	// Different scope or dead process → tear down and recreate.
	if r.codexAppServer != nil {
		r.codexAppServer.close()
		r.codexAppServer = nil
	}

	cmd := commandContextFn(ctx, codexBinaryName(), "app-server", "--listen", "stdio://")
	cmd.Env = os.Environ()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("codex app-server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("codex app-server stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("codex app-server start: %w", err)
	}
	kill := func() { _ = cmd.Process.Kill() }

	dispatcher := newCodexDispatcher(stdin, nil)
	dispatcher.start(stdout)

	// initialize: negotiate version + capabilities (bounded so a bad build can't hang).
	initCtx, cancel := context.WithTimeout(ctx, codexInitTimeout)
	defer cancel()
	caps, err := dispatcher.call(initCtx, "initialize", codexInitializeParams())
	if err != nil {
		dispatcher.fail(err)
		kill()
		return nil, fmt.Errorf("codex initialize: %w", err)
	}

	adapter := newCodexAdapter(dispatcher, cwd)
	// Runner-side prompt assembly before turn/start (04-03): full skill content
	// injection + ask_user reinforcement.
	adapter.promptPrep = func(req TurnRequest) string {
		workspace := cwd
		if req.Cwd != "" {
			workspace = req.Cwd
		}
		prompt := r.injectSkillContent(workspace, req.Prompt, skillIDsOf(req.SelectedSkills))
		return prompt + askUserReinforcement
	}

	h := &codexAppServerHandle{scopeKey: scopeKey, dispatcher: dispatcher, adapter: adapter, caps: caps, kill: kill}
	r.codexAppServer = h
	return h, nil
}

func skillIDsOf(skills []SkillSelection) []string {
	if len(skills) == 0 {
		return nil
	}
	out := make([]string, 0, len(skills))
	for _, s := range skills {
		if s.Name != "" {
			out = append(out, s.Name)
		}
	}
	return out
}

// errorAdapter is returned by the live registry when the app-server cannot be
// ensured, so a turn fails cleanly (typed) instead of hanging.
type errorAdapter struct {
	key ProviderKey
	err error
}

func (a errorAdapter) Key() ProviderKey                   { return a.key }
func (a errorAdapter) Capabilities() ProviderCapabilities { return ProviderCapabilities{} }
func (a errorAdapter) SendTurn(context.Context, TurnRequest, TurnBridge) error {
	return a.err
}
