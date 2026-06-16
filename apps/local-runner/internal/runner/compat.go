package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Tested-good versions — the CLI versions this build was validated against.
// These are the source of truth shared between the HTTP API and compat_test.go.
const (
	CompatTestedClaudeVersion = "2.1.178"
	CompatTestedCodexVersion  = "0.137.0"
)

// compatClaudeFlags are the CLI flags passed on every `claude -p` invocation.
var compatClaudeFlags = []string{
	"--input-format",
	"--output-format",
	"--include-partial-messages",
	"--include-hook-events",
	"--strict-mcp-config",
	"--disallowed-tools",
	"--permission-mode",
	"--mcp-config",
	"--resume",
	"--effort",
}

// CompatVersionInfo is the lightweight GET response: tested vs installed.
type CompatVersionInfo struct {
	TestedClaudeVersion    string `json:"testedClaudeVersion"`
	InstalledClaudeVersion string `json:"installedClaudeVersion"`
	TestedCodexVersion     string `json:"testedCodexVersion"`
	InstalledCodexVersion  string `json:"installedCodexVersion"`
}

type CompatConfig struct {
	TestedClaudeVersion string `json:"testedClaudeVersion"`
	TestedCodexVersion  string `json:"testedCodexVersion"`
}

// CompatItem is one check result.
type CompatItem struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "pass" | "warn" | "fail"
	Detail string `json:"detail"`
}

// CompatCheckResult is the full POST response.
type CompatCheckResult struct {
	CompatVersionInfo
	Items  []CompatItem `json:"items"`
	Passed int          `json:"passed"`
	Warned int          `json:"warned"`
	Failed int          `json:"failed"`
}

// LoadCompatConfig reads the workspace-scoped tested-version config, or falls
// back to the built-in validated baseline when the file does not exist yet.
func (r *Runner) LoadCompatConfig() (CompatConfig, error) {
	raw, err := os.ReadFile(r.compatConfigPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultCompatConfig(), nil
		}
		return CompatConfig{}, err
	}

	var config CompatConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return CompatConfig{}, err
	}
	return normalizeCompatConfig(config), nil
}

func (r *Runner) SaveCompatConfig(input CompatConfig) (CompatConfig, error) {
	config := normalizeCompatConfig(input)
	path := r.compatConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return CompatConfig{}, err
	}
	raw, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return CompatConfig{}, err
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return CompatConfig{}, err
	}
	return config, nil
}

// CompatLoadInfo returns installed versions without running any checks.
// Fast enough to call on every settings page load.
func (r *Runner) CompatLoadInfo(ctx context.Context) CompatVersionInfo {
	config, err := r.LoadCompatConfig()
	if err != nil {
		config = defaultCompatConfig()
	}
	return CompatVersionInfo{
		TestedClaudeVersion:    config.TestedClaudeVersion,
		InstalledClaudeVersion: compatRunVersion(ctx, "claude"),
		TestedCodexVersion:     config.TestedCodexVersion,
		InstalledCodexVersion:  compatRunVersion(ctx, "codex"),
	}
}

// RunCompatCheck executes all compat checks and returns structured results.
// Each check corresponds to a contract assumption in claude_stream.go,
// claude_event_mapper.go, or codex_appserver_process.go.
func (r *Runner) RunCompatCheck(ctx context.Context) CompatCheckResult {
	info := r.CompatLoadInfo(ctx)
	var items []CompatItem

	// 1. Version checks
	items = append(items, compatVersionItem("Claude version", info.InstalledClaudeVersion, info.TestedClaudeVersion))
	items = append(items, compatVersionItem("Codex version", info.InstalledCodexVersion, info.TestedCodexVersion))

	// 2. Claude required flags (each is passed on every `claude -p` invocation)
	claudeHelp := compatRunHelp(ctx, "claude")
	for _, flag := range compatClaudeFlags {
		if strings.Contains(claudeHelp, flag) {
			items = append(items, CompatItem{Name: "claude " + flag, Status: "pass", Detail: "present in --help"})
		} else {
			items = append(items, CompatItem{Name: "claude " + flag, Status: "fail", Detail: "missing from --help — may be renamed or removed"})
		}
	}

	// 3. --permission-prompt-tool is undocumented (not in --help); probe it directly
	items = append(items, compatProbePermissionPromptTool(ctx))

	// 4. Codex app-server transport
	codexAppHelp := compatRunHelp(ctx, "codex", "app-server")
	if strings.Contains(codexAppHelp, "--listen") {
		items = append(items, CompatItem{Name: "codex app-server --listen", Status: "pass", Detail: "present in app-server --help"})
	} else {
		items = append(items, CompatItem{Name: "codex app-server --listen", Status: "fail", Detail: "missing — Codex transport changed"})
	}
	if strings.Contains(codexAppHelp, "stdio") {
		items = append(items, CompatItem{Name: "codex app-server stdio://", Status: "pass", Detail: "stdio transport mentioned"})
	} else {
		items = append(items, CompatItem{Name: "codex app-server stdio://", Status: "warn", Detail: "not mentioned in --help, verify manually"})
	}

	result := CompatCheckResult{CompatVersionInfo: info, Items: items}
	for _, it := range items {
		switch it.Status {
		case "pass":
			result.Passed++
		case "warn":
			result.Warned++
		case "fail":
			result.Failed++
		}
	}
	return result
}

// RunCompatDeepCheck executes the fast checks plus live provider protocol probes.
func (r *Runner) RunCompatDeepCheck(ctx context.Context) CompatCheckResult {
	result := r.RunCompatCheck(ctx)
	items := append([]CompatItem{}, result.Items...)
	items = append(items, compatProbeClaudeStreamJSON(ctx))
	items = append(items, compatProbeCodexInitialize(ctx))

	result.Items = items
	result.Passed = 0
	result.Warned = 0
	result.Failed = 0
	for _, it := range items {
		switch it.Status {
		case "pass":
			result.Passed++
		case "warn":
			result.Warned++
		case "fail":
			result.Failed++
		}
	}
	return result
}

func (r *Runner) compatConfigPath() string {
	return filepath.Join(r.workspace, ".flowpilot", "settings", "compat-config.json")
}

func defaultCompatConfig() CompatConfig {
	return CompatConfig{
		TestedClaudeVersion: CompatTestedClaudeVersion,
		TestedCodexVersion:  CompatTestedCodexVersion,
	}
}

func normalizeCompatConfig(config CompatConfig) CompatConfig {
	config.TestedClaudeVersion = strings.TrimSpace(config.TestedClaudeVersion)
	config.TestedCodexVersion = strings.TrimSpace(config.TestedCodexVersion)
	if config.TestedClaudeVersion == "" {
		config.TestedClaudeVersion = CompatTestedClaudeVersion
	}
	if config.TestedCodexVersion == "" {
		config.TestedCodexVersion = CompatTestedCodexVersion
	}
	return config
}

// ── internal helpers ──────────────────────────────────────────────────────────

func compatVersionItem(name, installed, tested string) CompatItem {
	if installed == "" {
		return CompatItem{Name: name, Status: "fail", Detail: "binary not found on PATH"}
	}
	if strings.Contains(installed, tested) {
		return CompatItem{Name: name, Status: "pass", Detail: installed}
	}
	gotMM := compatMajorMinor(installed)
	wantMM := compatMajorMinor(tested)
	if gotMM != wantMM {
		return CompatItem{Name: name, Status: "fail", Detail: fmt.Sprintf("major/minor changed: %s (tested on %s) — review protocol", installed, tested)}
	}
	return CompatItem{Name: name, Status: "warn", Detail: fmt.Sprintf("patch drift: %s (tested on %s) — likely safe", installed, tested)}
}

func compatProbePermissionPromptTool(ctx context.Context) CompatItem {
	c, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(c, "claude", "-p",
		"--permission-prompt-tool", "mcp__compat__probe",
		"--output-format", "stream-json",
		"--dangerously-skip-permissions",
		"echo PROBE",
	).CombinedOutput()
	s := strings.ToLower(string(out))
	if strings.Contains(s, "unknown option") || strings.Contains(s, "unknown flag") || strings.Contains(s, "unexpected argument") {
		return CompatItem{Name: "claude --permission-prompt-tool", Status: "fail", Detail: "flag removed or renamed"}
	}
	return CompatItem{Name: "claude --permission-prompt-tool", Status: "pass", Detail: "undocumented flag accepted by CLI"}
}

func compatProbeClaudeStreamJSON(ctx context.Context) CompatItem {
	c, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	cmd := exec.CommandContext(c, "claude", "-p",
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
			return CompatItem{Name: "claude stream-json live probe", Status: "warn", Detail: "skipped because Claude credentials are missing"}
		}
		return CompatItem{Name: "claude stream-json live probe", Status: "fail", Detail: fmt.Sprintf("claude -p failed: %v%s", err, compatOutputSnippet(stderr.String()))}
	}

	seen := map[string]bool{}
	var resultFrame map[string]any
	scanner := bufio.NewScanner(&stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		var frame map[string]any
		if json.Unmarshal(scanner.Bytes(), &frame) != nil {
			continue
		}
		typ, _ := frame["type"].(string)
		if typ == "" {
			continue
		}
		seen[typ] = true
		if typ == "result" {
			resultFrame = frame
		}
	}

	for _, required := range []string{"system", "result"} {
		if !seen[required] {
			return CompatItem{Name: "claude stream-json live probe", Status: "fail", Detail: fmt.Sprintf("missing frame type %q", required)}
		}
	}
	if !seen["stream_event"] && !seen["assistant"] {
		return CompatItem{Name: "claude stream-json live probe", Status: "fail", Detail: "no content frame found"}
	}
	if resultFrame == nil {
		return CompatItem{Name: "claude stream-json live probe", Status: "fail", Detail: "result frame missing"}
	}
	for _, field := range []string{"subtype", "is_error"} {
		if _, ok := resultFrame[field]; !ok {
			return CompatItem{Name: "claude stream-json live probe", Status: "fail", Detail: fmt.Sprintf("result.%s missing", field)}
		}
	}
	if _, ok := resultFrame["session_id"]; !ok {
		return CompatItem{Name: "claude stream-json live probe", Status: "warn", Detail: "passed, but result.session_id missing; resume may be broken"}
	}
	return CompatItem{Name: "claude stream-json live probe", Status: "pass", Detail: "required stream-json frames and fields present"}
}

func compatProbeCodexInitialize(ctx context.Context) CompatItem {
	c, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(c, codexBinaryName(), "app-server", "--listen", "stdio://")
	cmd.Env = os.Environ()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return CompatItem{Name: "codex app-server initialize", Status: "fail", Detail: fmt.Sprintf("stdin pipe: %v", err)}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return CompatItem{Name: "codex app-server initialize", Status: "fail", Detail: fmt.Sprintf("stdout pipe: %v", err)}
	}
	if err := cmd.Start(); err != nil {
		return CompatItem{Name: "codex app-server initialize", Status: "fail", Detail: fmt.Sprintf("start failed: %v", err)}
	}
	defer func() { _ = cmd.Process.Kill() }()

	req, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  codexInitializeParams(),
	})
	req = append(req, '\n')
	if _, err := stdin.Write(req); err != nil {
		return CompatItem{Name: "codex app-server initialize", Status: "fail", Detail: fmt.Sprintf("write initialize: %v", err)}
	}

	type rpcMsg struct {
		ID     *int           `json:"id"`
		Result map[string]any `json:"result"`
		Error  map[string]any `json:"error"`
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
	case <-c.Done():
		return CompatItem{Name: "codex app-server initialize", Status: "warn", Detail: "no initialize response; check Codex login or binary"}
	case msg := <-done:
		if msg.Error != nil {
			return CompatItem{Name: "codex app-server initialize", Status: "fail", Detail: fmt.Sprintf("initialize error: %v", msg.Error)}
		}
		if msg.Result == nil {
			return CompatItem{Name: "codex app-server initialize", Status: "fail", Detail: "initialize response has no result"}
		}
		if _, ok := msg.Result["capabilities"]; !ok {
			return CompatItem{Name: "codex app-server initialize", Status: "warn", Detail: "passed, but capabilities absent; using optimistic feature support"}
		}
		return CompatItem{Name: "codex app-server initialize", Status: "pass", Detail: "initialize response returned capabilities"}
	}
}

func compatRunVersion(ctx context.Context, binary string) string {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(c, binary, "--version").Output()
	return strings.TrimSpace(string(out))
}

func compatRunHelp(ctx context.Context, binary string, args ...string) string {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(c, binary, append(args, "--help")...).CombinedOutput()
	return string(out)
}

func compatOutputSnippet(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 160 {
				line = line[:160] + "..."
			}
			return " — " + line
		}
	}
	return ""
}

// compatMajorMinor extracts "X.Y" from version strings like "2.1.178 (Claude Code)"
// or "codex-cli 0.137.0".
func compatMajorMinor(s string) string {
	for _, tok := range strings.Fields(s) {
		segs := strings.Split(tok, ".")
		if len(segs) < 2 {
			continue
		}
		allNum := true
		for _, seg := range segs[:2] {
			for _, c := range seg {
				if c < '0' || c > '9' {
					allNum = false
					break
				}
			}
		}
		if allNum {
			return segs[0] + "." + segs[1]
		}
	}
	return s
}
