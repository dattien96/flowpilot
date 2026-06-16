package runner

// Compatibility smoke tests for Claude Code CLI stream-json and Codex app-server.
//
// These tests capture the exact wire contract we depend on. Run them on any new
// machine or after upgrading the tools to detect breaking changes before they
// surface as runtime bugs.
//
//	FLOWPILOT_COMPAT=1 go test ./internal/runner/ -run TestCompat -v
//
// Tests that send real prompts (TestCompatClaudeStreamJSON, TestCompatCodexInitialize)
// need the binary to be logged in. They skip automatically when credentials are absent.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Tested-good versions — sourced from compat.go constants.
// A patch-level bump is usually safe; a minor/major bump requires protocol review.
const (
	compatClaudeVersion = CompatTestedClaudeVersion // "2.1.178"
	compatCodexVersion  = CompatTestedCodexVersion  // "0.137.0"
)

// Stream-json frame types we parse in claude_event_mapper.go / claude_stream.go.
// A missing type → silent loss of events; a renamed type → same.
// Types not in this list are silently ignored by mapClaudeLine's default case.
var claudeStreamFrameTypes = []string{
	"system",           // init: carries session_id; also status, post_turn_summary (ignored)
	"assistant",        // message.content blocks: text, tool_use
	"user",             // tool_result blocks
	"stream_event",     // content_block_delta.text_delta (--include-partial-messages)
	"result",           // terminal: subtype, is_error, session_id, errors[]
	"control_request",  // permission / ask_user inbound from CLI
	"rate_limit_event", // observed in 2.1.178; not parsed, silently ignored
}

// ── helpers ──────────────────────────────────────────────────────────────────

func compatSkip(t *testing.T) {
	t.Helper()
	if os.Getenv("FLOWPILOT_COMPAT") == "" {
		t.Skip("set FLOWPILOT_COMPAT=1 to run compatibility checks")
	}
}


// ── version checks ────────────────────────────────────────────────────────────

// TestCompatClaudeVersion checks the installed Claude Code CLI version.
// Warns on any drift; fails if major.minor changed (protocol review required).
func TestCompatClaudeVersion(t *testing.T) {
	compatSkip(t)

	out, err := exec.Command("claude", "--version").Output()
	if err != nil {
		t.Fatalf("claude --version: %v", err)
	}
	got := strings.TrimSpace(string(out))

	if strings.Contains(got, compatClaudeVersion) {
		t.Logf("OK  claude version: %s (matches tested %s)", got, compatClaudeVersion)
		return
	}
	gotMM := compatMajorMinor(got)
	wantMM := compatMajorMinor(compatClaudeVersion)
	if gotMM != wantMM {
		t.Errorf("MAJOR/MINOR CHANGED: got %s (major.minor %s), tested against %s (major.minor %s) — manual stream-json protocol review required",
			got, gotMM, compatClaudeVersion, wantMM)
	} else {
		t.Logf("WARN patch drift: got %s, tested against %s (same major.minor — likely safe)", got, compatClaudeVersion)
	}
}

// TestCompatCodexVersion checks the installed Codex version.
func TestCompatCodexVersion(t *testing.T) {
	compatSkip(t)

	out, err := exec.Command("codex", "--version").Output()
	if err != nil {
		t.Fatalf("codex --version: %v", err)
	}
	got := strings.TrimSpace(string(out))

	if strings.Contains(got, compatCodexVersion) {
		t.Logf("OK  codex version: %s (matches tested %s)", got, compatCodexVersion)
		return
	}
	gotMM := compatMajorMinor(got)
	wantMM := compatMajorMinor(compatCodexVersion)
	if gotMM != wantMM {
		t.Errorf("MAJOR/MINOR CHANGED: got %s (major.minor %s), tested against %s (major.minor %s) — manual app-server protocol review required",
			got, gotMM, compatCodexVersion, wantMM)
	} else {
		t.Logf("WARN patch drift: got %s, tested against %s (same major.minor — likely safe)", got, compatCodexVersion)
	}
}

// ── flag existence checks ─────────────────────────────────────────────────────

// TestCompatClaudeFlags checks that every CLI flag we depend on still appears in
// --help. Renamed or removed flags are caught here before they cause silent failures.
func TestCompatClaudeFlags(t *testing.T) {
	compatSkip(t)

	// `claude --help` may exit non-zero; capture combined output.
	out, _ := exec.Command("claude", "--help").CombinedOutput()
	help := string(out)
	if help == "" {
		t.Fatal("claude --help produced no output")
	}

	for _, flag := range compatClaudeFlags {
		if strings.Contains(help, flag) {
			t.Logf("OK  %s", flag)
		} else {
			t.Errorf("MISSING FLAG: %q not in `claude --help` — check if renamed or removed", flag)
		}
	}

	// --permission-prompt-tool is undocumented (not in --help) but we pass it on
	// every gated turn. Probe by passing a dummy value — if the flag is gone the
	// CLI will exit with an "unknown option" / "unexpected argument" error.
	probe, _ := exec.Command("claude", "-p",
		"--permission-prompt-tool", "mcp__compat__probe",
		"--output-format", "stream-json",
		"--dangerously-skip-permissions",
		"echo PROBE",
	).CombinedOutput()
	probeStr := strings.ToLower(string(probe))
	if strings.Contains(probeStr, "unknown option") ||
		strings.Contains(probeStr, "unknown flag") ||
		strings.Contains(probeStr, "unexpected argument") {
		t.Errorf("--permission-prompt-tool removed or renamed: %s", probe)
	} else {
		t.Logf("OK  --permission-prompt-tool (undocumented, probe passed)")
	}
}

// TestCompatCodexAppServerFlags checks that `codex app-server --listen <url>` exists.
func TestCompatCodexAppServerFlags(t *testing.T) {
	compatSkip(t)

	out, _ := exec.Command("codex", "app-server", "--help").CombinedOutput()
	help := string(out)
	if help == "" {
		t.Fatal("`codex app-server --help` produced no output")
	}

	if strings.Contains(help, "--listen") {
		t.Logf("OK  codex app-server --listen")
	} else {
		t.Errorf("MISSING: `codex app-server --listen` not in help — Codex transport changed")
	}
	if strings.Contains(help, "stdio") {
		t.Logf("OK  stdio transport mentioned")
	} else {
		t.Logf("WARN: 'stdio' not mentioned in app-server help — verify --listen stdio:// still works")
	}
}

// ── wire protocol smoke tests ─────────────────────────────────────────────────

// TestCompatClaudeStreamJSON sends a minimal turn through the real `claude -p
// --output-format stream-json` and validates the frame types and field shapes we
// depend on. Skipped when Claude credentials are absent.
func TestCompatClaudeStreamJSON(t *testing.T) {
	compatSkip(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "-p",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
		"--dangerously-skip-permissions",
		"Respond with exactly the single word: PONG",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errStr := strings.ToLower(stderr.String())
		if strings.Contains(errStr, "not logged in") ||
			strings.Contains(errStr, "authentication") ||
			strings.Contains(errStr, "api key") ||
			strings.Contains(errStr, "login") {
			t.Skip("No Claude credentials — skipping live stream-json test")
		}
		t.Logf("stderr: %s", stderr.String())
		t.Fatalf("claude -p failed: %v", err)
	}

	seen := map[string]bool{}
	var resultFrame map[string]any

	scanner := bufio.NewScanner(&stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var frame map[string]any
		if json.Unmarshal(line, &frame) != nil {
			continue
		}
		typ, _ := frame["type"].(string)
		sub, _ := frame["subtype"].(string)
		if typ == "" {
			continue
		}
		seen[typ] = true
		t.Logf("  frame type=%q subtype=%q", typ, sub)
		if typ == "result" {
			resultFrame = frame
		}
	}

	// system and result are mandatory in every invocation.
	for _, required := range []string{"system", "result"} {
		if !seen[required] {
			t.Errorf("MISSING FRAME TYPE %q — stream-json output contract changed", required)
		}
	}
	// At least one of stream_event or assistant must carry content.
	if !seen["stream_event"] && !seen["assistant"] {
		t.Errorf("No content frames (stream_event or assistant) — stream-json changed")
	}

	// Validate result frame fields: subtype, is_error (both used in mapClaudeResult).
	if resultFrame != nil {
		for _, field := range []string{"subtype", "is_error"} {
			if _, ok := resultFrame[field]; !ok {
				t.Errorf("result frame MISSING FIELD %q — mapClaudeResult will be wrong", field)
			}
		}
		if _, ok := resultFrame["session_id"]; !ok {
			t.Logf("WARN: result frame missing 'session_id' — --resume may be broken")
		}
		t.Logf("  result: subtype=%v is_error=%v session_id=%v",
			resultFrame["subtype"], resultFrame["is_error"], resultFrame["session_id"])
	}

	// Validate system/init carries session_id (used for --resume).
	stdout2 := stdout
	scanner2 := bufio.NewScanner(&stdout2)
	scanner2.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner2.Scan() {
		var frame map[string]any
		if json.Unmarshal(scanner2.Bytes(), &frame) != nil {
			continue
		}
		if frame["type"] == "system" && frame["subtype"] == "init" {
			if _, ok := frame["session_id"]; !ok {
				t.Logf("WARN: system/init missing 'session_id' — resume will fail")
			} else {
				t.Logf("OK  system/init has session_id")
			}
		}
	}
}

// TestCompatCodexInitialize starts the app-server, sends the initialize request,
// and validates the response has the fields we read in codexAppServerHandle.supports().
func TestCompatCodexInitialize(t *testing.T) {
	compatSkip(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, codexBinaryName(), "app-server", "--listen", "stdio://")
	cmd.Env = os.Environ()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("codex app-server start: %v", err)
	}
	defer func() { _ = cmd.Process.Kill() }()

	// Send initialize using the exact same params as production (codexInitializeParams).
	req, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  codexInitializeParams(),
	})
	req = append(req, '\n')
	if _, err := stdin.Write(req); err != nil {
		t.Fatalf("write initialize: %v", err)
	}

	type rpcMsg struct {
		Jsonrpc string         `json:"jsonrpc"`
		ID      *int           `json:"id"`
		Method  string         `json:"method"`
		Result  map[string]any `json:"result"`
		Error   map[string]any `json:"error"`
	}
	done := make(chan rpcMsg, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		for scanner.Scan() {
			var msg rpcMsg
			if json.Unmarshal(scanner.Bytes(), &msg) != nil {
				continue
			}
			if msg.ID != nil && *msg.ID == 1 {
				done <- msg
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
		t.Fatal("TIMEOUT: codex app-server did not respond to initialize within 20s — binary may be broken or credentials missing")
	case msg := <-done:
		if msg.Error != nil {
			t.Fatalf("initialize returned error: %v", msg.Error)
		}
		if msg.Result == nil {
			t.Fatal("initialize response has no result — Codex protocol changed")
		}

		if b, err := json.Marshal(msg.Result); err == nil {
			t.Logf("  raw result: %s", b)
		}

		// capabilities is read by codexAppServerHandle.supports(); its absence means
		// the optimistic-true fallback is used (every feature assumed supported).
		// This is expected on codex 0.137.x which returns metadata fields instead.
		for _, field := range []string{"protocolVersion", "capabilities"} {
			if _, ok := msg.Result[field]; ok {
				t.Logf("OK  initialize.%s = %v", field, msg.Result[field])
			} else {
				t.Logf("WARN: initialize.%s absent — supports() optimistic-true (expected on 0.137.x)", field)
			}
		}

		// codex 0.137.x returns these metadata fields; their presence confirms
		// the binary is a real Codex server and not something else.
		for _, field := range []string{"codexHome", "platformOs", "userAgent"} {
			if v, ok := msg.Result[field]; ok {
				t.Logf("OK  initialize.%s = %v", field, v)
			} else {
				t.Logf("WARN: initialize.%s absent (present on 0.137.x)", field)
			}
		}
		t.Logf("PASS codex initialize handshake")
	}
}
