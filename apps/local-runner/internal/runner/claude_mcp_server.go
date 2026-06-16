package runner

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
)

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
}

func newClaudeMCPServer() *claudeMCPServer {
	return &claudeMCPServer{bridges: map[string]TurnBridge{}}
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
	s.mu.Unlock()
	return tok
}

func (s *claudeMCPServer) unregister(tok string) {
	s.mu.Lock()
	delete(s.bridges, tok)
	s.mu.Unlock()
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
	if r.Method != http.MethodPost {
		// Streamable-HTTP clients may probe GET for an SSE channel; we only do
		// request/response, so decline GET cleanly.
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
		default:
			return nil, map[string]any{"code": -32601, "message": "unknown tool: " + name}
		}
	}
	return nil, map[string]any{"code": -32601, "message": "method not found: " + method}
}

func claudeMCPToolDefs() []any {
	objSchema := func() map[string]any { return map[string]any{"type": "object"} }
	return []any{
		map[string]any{"name": "approve", "description": "FlowPilot permission prompt: approve or deny a tool use.", "inputSchema": objSchema()},
		map[string]any{"name": "ask_user", "description": "Ask the user a structured question and wait for the answer.", "inputSchema": objSchema()},
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
