package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Task-403 (CP-70): the stdio shim that lets Devin reach the FlowPilot MCP
// tools (ask_user / spawn_agent / flow gates). Devin ACP advertises
// mcpCapabilities{http:false,sse:false} — the HTTP loopback every other
// provider uses cannot be injected via session/new. The adapter therefore
// registers a stdio MCP server instead: `flowpilot devin-mcp-stdio --url
// <mcp-endpoint>?token=<turn-token>` — a per-session child process that
// proxies each JSON-RPC line on stdin to the runner's HTTP MCP endpoint and
// writes the JSON response back to stdout.
//
// Security: the URL carries the per-turn token; the shim never logs it, and
// the endpoint itself is loopback-guarded (isLoopbackRequest). Stderr is the
// only diagnostic channel (stdout is protocol).

// devinMCPShimCommand resolves the command+base-args for the stdio shim.
// Production runs the runner's own binary (os.Executable — valid for the
// process lifetime even in `go run` dev mode since the temp binary persists
// while the parent lives). FLOWPILOT_DEVIN_MCP_SHIM overrides the whole
// command line for tests ("<cmd> <args...>" shell-split on spaces).
func devinMCPShimCommand() (string, []string) {
	if override := strings.TrimSpace(os.Getenv("FLOWPILOT_DEVIN_MCP_SHIM")); override != "" {
		parts := strings.Fields(override)
		return parts[0], parts[1:]
	}
	exe, err := os.Executable()
	if err != nil || strings.TrimSpace(exe) == "" {
		return "", nil
	}
	return exe, []string{"devin-mcp-stdio"}
}

// RunDevinMCPStdio is the `flowpilot devin-mcp-stdio --url <endpoint>`
// subcommand body: newline-delimited JSON-RPC in on stdin, one JSON response
// per request line on stdout. Notifications (no id) are forwarded and get no
// stdout line (the HTTP endpoint answers them 202). Runs until stdin closes.
func RunDevinMCPStdio(ctx context.Context, url string, stdin io.Reader, stdout io.Writer) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return fmt.Errorf("devin-mcp-stdio: --url is required")
	}
	client := &http.Client{Timeout: 0} // ask_user blocks until the user answers — no client-side cap.
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	writer := bufio.NewWriter(stdout)
	defer writer.Flush()
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			writeDevinMCPStdioLine(writer, map[string]any{"jsonrpc": "2.0", "id": nil, "error": map[string]any{"code": -32700, "message": "parse error"}})
			continue
		}
		resp, hasResp, err := devinMCPStdioRoundTrip(ctx, client, url, line)
		if err != nil {
			// Transport failure: reply a JSON-RPC error so the agent's pending
			// call resolves instead of hanging the session.
			writeDevinMCPStdioLine(writer, map[string]any{"jsonrpc": "2.0", "id": msg["id"], "error": map[string]any{"code": -32000, "message": err.Error()}})
			continue
		}
		if hasResp {
			writeDevinMCPStdioLine(writer, resp)
		}
	}
	return scanner.Err()
}

// devinMCPStdioRoundTrip POSTs one raw JSON-RPC line to the HTTP endpoint and
// returns the decoded response. hasResp=false for notifications (HTTP 202).
func devinMCPStdioRoundTrip(ctx context.Context, client *http.Client, url, line string) (map[string]any, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(line))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	httpResp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode == http.StatusAccepted {
		_, _ = io.Copy(io.Discard, httpResp.Body)
		return nil, false, nil
	}
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, false, err
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("mcp endpoint %d: %s", httpResp.StatusCode, strings.TrimSpace(string(body)))
	}
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, false, fmt.Errorf("mcp endpoint non-JSON response")
	}
	return resp, true, nil
}

func writeDevinMCPStdioLine(w *bufio.Writer, msg map[string]any) {
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	_, _ = w.Write(append(b, '\n'))
	_ = w.Flush()
}
