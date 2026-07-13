package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Task-231 DOD-4 revisit: firebaseToolsMcpAdapter is a real, wired production
// FirebaseCrashlyticsAdapter. It speaks standard MCP JSON-RPC (the same
// mcpRequest/mcpResponse/mcpError shapes already defined for the Google
// Drive/Telegram proxy SERVERS in this package) as a CLIENT, spawning the
// official `firebase-tools mcp --only crashlytics` process (the sole access
// path CP-05-04 P-1/Q-1 resolved) rather than hand-rolling an unverified
// Crashlytics Management API v1alpha REST client. The exact Crashlytics REST
// surface behind firebase-tools is deliberately none of this adapter's
// concern — that is the entire point of going through the official MCP
// instead of a bespoke REST integration whose contract this codebase cannot
// verify without a live Firebase project.
//
// Protocol logic (building the tools/call request, parsing its response) is
// factored into pure functions below and fully unit tested. The subprocess
// spawn/pipe plumbing itself is intentionally thin and lightly tested,
// consistent with this codebase's existing practice for MCP process glue
// (RunGoogleDriveProxyMcpServer/RunTelegramProxyMcpServer are not
// exhaustively unit tested at the spawn layer either — the protocol they
// speak is what's tested).
type firebaseToolsMcpAdapter struct {
	command string
	args    []string
	runner  *Runner
	timeout time.Duration

	// commandContextFn is injectable so tests can substitute a fake
	// "firebase-tools" binary instead of requiring npx + a real Firebase
	// project — mirrors executeJiraRequestFn's seam-var pattern used
	// elsewhere in this package for the same reason.
	commandContextFn func(ctx context.Context, name string, arg ...string) *exec.Cmd
}

func newFirebaseToolsMcpAdapter(runner *Runner) *firebaseToolsMcpAdapter {
	return &firebaseToolsMcpAdapter{
		command: "npx",
		args:    []string{"-y", "firebase-tools", "mcp", "--only", "crashlytics"},
		runner:  runner,
		timeout: 20 * time.Second,
		commandContextFn: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, arg...)
		},
	}
}

// buildFirebaseCrashlyticsToolCallRequest builds the tools/call request for
// reading one Crashlytics issue. The exact argument key ("issueId") is a
// best-effort match to the official tool's documented purpose
// (crashlytics_get_issue) — this codebase has not verified it against a live
// firebase-tools schema, so a future adjustment here (not to the JSON-RPC
// envelope, just this one argument name) may be needed once tested against
// a real Firebase project.
func buildFirebaseCrashlyticsToolCallRequest(id int, crashRef string) (mcpRequest, error) {
	params, err := json.Marshal(map[string]any{
		"name":      "crashlytics_get_issue",
		"arguments": map[string]any{"issueId": crashRef},
	})
	if err != nil {
		return mcpRequest{}, err
	}
	return mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(fmt.Sprintf("%d", id)),
		Method:  "tools/call",
		Params:  params,
	}, nil
}

// parseMcpToolCallTextResult decodes a raw JSON-RPC response line and
// extracts the tool result's text content — the same result[].content[0].text
// shape textToolResult produces on the server side, read back here as a
// client. Returns an error if the response carries an MCP error, or if the
// shape doesn't match a text tool result.
func parseMcpToolCallTextResult(raw []byte) (string, error) {
	var resp mcpResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("parse MCP response: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("MCP error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		return "", errors.New("MCP tool result is not an object")
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		return "", errors.New("MCP tool result has no content")
	}
	block, ok := content[0].(map[string]any)
	if !ok {
		return "", errors.New("MCP tool result content[0] is not an object")
	}
	text, ok := block["text"].(string)
	if !ok {
		return "", errors.New("MCP tool result content[0] has no text field")
	}
	return text, nil
}

// Fetch resolves the connected Firebase credential, materializes it to disk
// (mirrors EnsureClaudeFirebaseMcpConfig's own writeFirebaseCredentialFile
// call, resolved fresh here rather than relying on a prior provider-config
// call having already written it), spawns the firebase-tools MCP process,
// sends initialize then a crashlytics_get_issue tools/call, and returns the
// bounded text result.
func (a *firebaseToolsMcpAdapter) Fetch(ctx context.Context, crashRef string) (string, error) {
	crashRef = strings.TrimSpace(crashRef)
	if crashRef == "" {
		return "", errors.New("firebase.crashlytics: crash ref is required")
	}
	if a.runner == nil {
		return "", errors.New("firebase.crashlytics: runner not configured")
	}
	creds, err := a.runner.resolveConnectedFirebaseCredential()
	if err != nil {
		return "", fmt.Errorf("firebase.crashlytics: %w", err)
	}
	credentialPath, err := writeFirebaseCredentialFile(a.runner.workspace, creds.ServiceAccountJSON)
	if err != nil {
		return "", fmt.Errorf("firebase.crashlytics: materialize credential: %w", err)
	}

	timeout := a.timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := a.commandContextFn(callCtx, a.command, a.args...)
	cmd.Env = append(os.Environ(), "GOOGLE_APPLICATION_CREDENTIALS="+credentialPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("firebase.crashlytics: open stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("firebase.crashlytics: open stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("firebase.crashlytics: start %s: %w", a.command, err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	reader := bufio.NewScanner(stdout)
	reader.Buffer(make([]byte, 64*1024), 4*1024*1024)

	if err := writeMcpRequestLine(stdin, mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage("1"),
		Method:  "initialize",
	}); err != nil {
		return "", fmt.Errorf("firebase.crashlytics: send initialize: %w", err)
	}
	if !reader.Scan() {
		return "", firebaseToolsMcpScanError(reader, "initialize")
	}

	toolCallReq, err := buildFirebaseCrashlyticsToolCallRequest(2, crashRef)
	if err != nil {
		return "", err
	}
	if err := writeMcpRequestLine(stdin, toolCallReq); err != nil {
		return "", fmt.Errorf("firebase.crashlytics: send tools/call: %w", err)
	}
	if !reader.Scan() {
		return "", firebaseToolsMcpScanError(reader, "tools/call")
	}

	return parseMcpToolCallTextResult(reader.Bytes())
}

func writeMcpRequestLine(w io.Writer, req mcpRequest) error {
	raw, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = w.Write(append(raw, '\n'))
	return err
}

func firebaseToolsMcpScanError(reader *bufio.Scanner, phase string) error {
	if err := reader.Err(); err != nil {
		return fmt.Errorf("firebase.crashlytics: read %s response: %w", phase, err)
	}
	return fmt.Errorf("firebase.crashlytics: no %s response (process exited early)", phase)
}
