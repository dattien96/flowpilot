package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// claudeMCPReadyDefaultTimeout bounds how long SendTurn withholds the user prompt waiting
// for claude's MCP client to finish connecting (initialize -> tools/list). claude connects
// --mcp-config servers asynchronously ("running fully async (nonblocking)"); the FIRST turn's
// tool set is otherwise snapshotted before mcp__flowpilot__ask_user is registered, so the
// model never sees ask_user (validated against claude 2.1.179). A normal local connect is
// ~0.1-1.2s; on a slow/failed connect we degrade to sending anyway rather than hang.
//
// waitReady returns the instant FlowPilot's tools/list arrives, so a larger ceiling adds NO
// latency to a healthy turn — it only grants more grace when a slow co-resident MCP server
// in the same --mcp-config (e.g. a google-drive stdio sidecar that has to warm up npx/OAuth)
// delays claude's overall MCP init and would otherwise drop spawn_agent/ask_user from the
// turn. Raised from 10s to 30s after spawn_agent went missing on turns that also load the
// google-drive MCP. (BUG-114)
const claudeMCPReadyDefaultTimeout = 30 * time.Second

// Phase 4 / 07: the runner-hosted MCP server that lets the real `claude` CLI reach
// FlowPilot's approve + ask_user tools (the spike-validated permission path). It speaks
// MCP JSON-RPC over HTTP (streamable-HTTP, request/response only — no SSE) so claude can
// connect via `--mcp-config {"type":"http","url":...}`.
//
// Routing: the runner serves ONE endpoint for all turns; each in-flight turn registers
// its TurnBridge under a CRYPTO-RANDOM token and claude's per-turn --mcp-config URL carries
// `?token=`. tools/call(approve) routes to that turn's bridge.RequestApproval; deny blocks
// the tool (verified against claude 2.1.177). Contract (07 Appendix A): approve args
// {tool_name, input, tool_use_id} -> reply text-JSON {"behavior":"deny"|"allow", ...}.
//
// Security: tokens are unguessable (crypto/rand) and the handler serves loopback callers
// only — claude connects over 127.0.0.1, and a non-loopback RemoteAddr is rejected even if
// the runner is bound to a non-loopback --host.

// ClaudeMCPPath is the runner route the per-turn --mcp-config URL points at.
const ClaudeMCPPath = "/internal/claude-permission-mcp"

type claudeMCPServer struct {
	mu      sync.Mutex
	bridges map[string]TurnBridge
	// ready holds a per-token channel closed the first time claude's MCP client calls
	// tools/list for that token — i.e. the per-turn server is connected and its tools
	// (including ask_user) are live. SendTurn waits on this before delivering the prompt.
	ready map[string]chan struct{}
}

func newClaudeMCPServer() *claudeMCPServer {
	return &claudeMCPServer{bridges: map[string]TurnBridge{}, ready: map[string]chan struct{}{}}
}

// register binds a turn's bridge to a fresh crypto-random token (used in the per-turn
// --mcp-config URL so a remote caller cannot guess an active token).
func (s *claudeMCPServer) register(bridge TurnBridge) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// rand.Read essentially never fails; fall back to a still-unique-enough value.
		b = []byte(strings.Repeat("0", 16))
	}
	tok := hex.EncodeToString(b)
	s.mu.Lock()
	s.bridges[tok] = bridge
	s.ready[tok] = make(chan struct{})
	s.mu.Unlock()
	return tok
}

func (s *claudeMCPServer) unregister(tok string) {
	s.mu.Lock()
	delete(s.bridges, tok)
	delete(s.ready, tok)
	s.mu.Unlock()
}

// signalReady closes the token's ready channel the first time it's called (tools/list seen).
// Idempotent: a second tools/list (or none) is harmless.
func (s *claudeMCPServer) signalReady(tok string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.ready[tok]
	if !ok {
		return
	}
	select {
	case <-ch: // already closed
	default:
		close(ch)
	}
}

// waitReady blocks until claude's MCP client has fetched tools/list for tok (connection
// live), the timeout elapses, or ctx is cancelled. Reports whether the connection became
// ready. An unknown/already-unregistered token returns false immediately.
func (s *claudeMCPServer) waitReady(ctx context.Context, tok string, timeout time.Duration) bool {
	s.mu.Lock()
	ch, ok := s.ready[tok]
	s.mu.Unlock()
	if !ok {
		return false
	}
	if timeout <= 0 {
		timeout = claudeMCPReadyDefaultTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ch:
		return true
	case <-timer.C:
		return false
	case <-ctx.Done():
		return false
	}
}

func (s *claudeMCPServer) bridgeFor(tok string) TurnBridge {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bridges[tok]
}

func (s *claudeMCPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Loopback-only: claude connects over 127.0.0.1; reject anything else even if the
	// runner is bound to a non-loopback host (review finding 1).
	if !isLoopbackRequest(r) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if r.Method == http.MethodGet {
		// Streamable-HTTP: claude opens a GET SSE stream and only marks the server
		// "connected" once it succeeds. Declining it with 405 leaves the server stuck
		// "pending", so its tools (ask_user!) are NEVER exposed to the model — the live-flow
		// defect (validated against claude 2.1.179). We never push server->client messages
		// (approve/ask_user are request/response), so the stream just stays open with
		// keepalive comments until claude disconnects.
		s.serveSSE(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var msg map[string]any
	if err := json.Unmarshal(body, &msg); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	id, hasID := msg["id"]
	if !hasID {
		// JSON-RPC notification (e.g. notifications/initialized) — accept, no body.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	method, _ := msg["method"].(string)
	result, rpcErr := s.dispatch(method, msg, r.URL.Query().Get("token"))

	resp := map[string]any{"jsonrpc": "2.0", "id": id}
	if rpcErr != nil {
		resp["error"] = rpcErr
	} else {
		resp["result"] = result
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// serveSSE answers claude's GET probe with an open text/event-stream so the Streamable-HTTP
// client considers the server connected. FlowPilot never initiates server->client messages,
// so the stream only carries keepalive comments and stays open until claude disconnects
// (request ctx cancelled) — one goroutine per live turn.
func (s *claudeMCPServer) serveSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		// Without flushing the client can't observe the open stream; fail cleanly.
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(": connected\n\n")); err != nil {
		return
	}
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := w.Write([]byte(": keepalive\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *claudeMCPServer) dispatch(method string, msg map[string]any, token string) (any, map[string]any) {
	params, _ := msg["params"].(map[string]any)
	switch method {
	case "initialize":
		pv := "2025-06-18"
		if params != nil {
			if v, ok := params["protocolVersion"].(string); ok && v != "" {
				pv = v
			}
		}
		return map[string]any{
			"protocolVersion": pv,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": claudeMCPServerName, "version": "1.0"},
		}, nil
	case "tools/list":
		// The handshake reached tools/list: the per-turn server is connected and its tools
		// are live. Unblock SendTurn so it can deliver the prompt with ask_user available.
		s.signalReady(token)
		return map[string]any{"tools": claudeMCPToolDefs()}, nil
	case "tools/call":
		name, _ := params["name"].(string)
		args, _ := params["arguments"].(map[string]any)
		if args == nil {
			args = map[string]any{}
		}
		bridge := s.bridgeFor(token)
		if bridge == nil {
			return nil, map[string]any{"code": -32000, "message": "no active turn for token"}
		}
		switch name {
		case "approve":
			return handleClaudeApprove(args, bridge), nil
		case "ask_user":
			return handleClaudeAskUser(args, bridge), nil
		case "spawn_agent":
			return handleClaudeSpawnAgent(args, bridge), nil
		default:
			return nil, map[string]any{"code": -32601, "message": "unknown tool: " + name}
		}
	}
	return nil, map[string]any{"code": -32601, "message": "method not found: " + method}
}

func claudeMCPToolDefs() []any {
	return []any{
		map[string]any{"name": "approve", "description": "FlowPilot permission prompt: approve or deny a tool use.", "inputSchema": map[string]any{"type": "object"}},
		// ask_user MUST advertise its parameter schema (prompt/options/multiSelect) so the model
		// knows how to call it and prefers it over its disabled built-in AskUserQuestion. Mirrors
		// the Codex registration (codexAskUserMcpServer) for cross-provider parity.
		map[string]any{
			"name":        "ask_user",
			"description": "Ask the user a structured question and wait for their answer before continuing. Use when you need a decision or clarification instead of guessing.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt":      map[string]any{"type": "string", "description": "The question to ask the user."},
					"options":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Selectable answer options."},
					"multiSelect": map[string]any{"type": "boolean", "description": "Allow selecting more than one option."},
				},
				"required": []any{"prompt"},
			},
		},
		map[string]any{
			"name":        "spawn_agent",
			"description": "Spawn a child agent run. Use when a sub-task is best delegated to a specialised agent. If wait=true the call blocks until the child's first turn completes and returns its final message.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent":     map[string]any{"type": "string", "description": "Agent name from the catalog (e.g. \"researcher\", \"coder\")."},
					"prompt":    map[string]any{"type": "string", "description": "Initial prompt for the child agent."},
					"provider":  map[string]any{"type": "string", "description": "Override provider key (codex, claude). Omit to inherit parent."},
					"dependsOn": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Run IDs this child must wait for before starting."},
					"wait":      map[string]any{"type": "boolean", "description": "Block until the child's first turn completes (default false)."},
				},
				"required": []any{"agent", "prompt"},
			},
		},
	}
}

// isLoopbackRequest reports whether the HTTP caller is on the loopback interface.
func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return strings.EqualFold(host, "localhost")
}

// ClaudeMCPHandler exposes the runner-hosted MCP server so root.go can mount it at
// ClaudeMCPPath on the runner's HTTP mux.
func (r *Runner) ClaudeMCPHandler() http.Handler { return r.claudeMCP }

// SetMCPBaseURL records the runner's own base URL ("http://host:port"), set once at
// startup so the Claude adapter can build per-turn --mcp-config URLs.
func (r *Runner) SetMCPBaseURL(url string) {
	r.mcpBaseURLMu.Lock()
	r.mcpBaseURL = strings.TrimSpace(url)
	r.mcpBaseURLMu.Unlock()
}

func (r *Runner) mcpBaseURLValue() string {
	r.mcpBaseURLMu.RLock()
	defer r.mcpBaseURLMu.RUnlock()
	return r.mcpBaseURL
}

// writeClaudeMCPConfig writes a per-turn --mcp-config file pointing claude at the runner's
// HTTP MCP endpoint with this turn's token. Uses os.CreateTemp (random name) so concurrent
// turns / runner restarts never collide in a shared temp dir. Returns path + cleanup.
//
// extra carries FlowPilot-managed servers (e.g. google-drive) merged next to the flowpilot
// permission server. Because claudeArgs always passes --strict-mcp-config, claude loads ONLY
// the servers in this file and ignores the account's .claude.json mcpServers — so any
// FlowPilot-managed MCP the user configured (Google Drive setup, step 7) MUST be merged here
// or claude never sees it (unlike Codex, which reads config.toml natively).
func writeClaudeMCPConfig(baseURL, token string, extra map[string]claudeMcpServer) (string, func(), error) {
	url := strings.TrimRight(baseURL, "/") + ClaudeMCPPath + "?token=" + token
	servers := map[string]any{
		claudeMCPServerName: map[string]any{"type": "http", "url": url},
	}
	for name, s := range extra {
		if strings.TrimSpace(name) == "" || name == claudeMCPServerName {
			continue // never let an extra server shadow the permission route
		}
		servers[name] = s
	}
	cfg := map[string]any{"mcpServers": servers}
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", func() {}, err
	}
	f, err := os.CreateTemp("", "flowpilot-claude-mcp-*.json")
	if err != nil {
		return "", func() {}, err
	}
	path := f.Name()
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", func() {}, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", func() {}, err
	}
	return path, func() { _ = os.Remove(path) }, nil
}
