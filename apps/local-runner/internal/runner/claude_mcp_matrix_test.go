package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// BUG-073 regression matrix: with the per-turn permission MCP server WIRED (the production
// path, not the in-stream control_request fallback), prove all four combinations of
// {YOLO on/off} × {command-tool gating, FlowPilot MCP-tool availability} hold:
//
//	                | command-tool gating            | MCP availability (ask_user, google-drive)
//	  YOLO=on       | bypass: NO --permission-prompt  | --mcp-config present, servers merged
//	  YOLO=off      | gated: --permission-prompt-tool | --mcp-config present, servers merged
//
// The defects this guards: (1) YOLO=on used to skip --mcp-config entirely (no FlowPilot MCP
// tools at all); (2) FlowPilot-managed servers (google-drive) were never merged into the
// per-turn config and were hidden by --strict-mcp-config.

// claudeSpawnCapture records the args of every spawned `claude` process and the live content
// of each spawn's --mcp-config file (read inside the command hook, BEFORE SendTurn's deferred
// cleanup removes the temp file).
type claudeSpawnCapture struct {
	mu        sync.Mutex
	args      [][]string
	mcpConfig []string
}

func captureClaudeSpawn(t *testing.T, script string) (*claudeSpawnCapture, func()) {
	t.Helper()
	cap := &claudeSpawnCapture{}
	original := commandContextFn
	commandContextFn = func(ctx context.Context, _ string, arg ...string) *exec.Cmd {
		content := ""
		if i := argIndex(arg, "--mcp-config"); i >= 0 && i+1 < len(arg) {
			if b, err := os.ReadFile(arg[i+1]); err == nil {
				content = string(b)
			}
		}
		cap.mu.Lock()
		cap.args = append(cap.args, append([]string{}, arg...))
		cap.mcpConfig = append(cap.mcpConfig, content)
		cap.mu.Unlock()
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	}
	return cap, func() { commandContextFn = original }
}

// wiredClaudeAdapter returns an adapter with the per-turn MCP server + base URL + a google-drive
// extra-server resolver wired (the production shape).
func wiredClaudeAdapter() *claudeAdapter {
	a := newTestClaudeAdapter()
	a.mcpServer = newClaudeMCPServer()
	a.mcpBaseURL = func() string { return "http://127.0.0.1:9999" }
	a.extraMCPServers = func(_ bool) map[string]claudeMcpServer {
		return map[string]claudeMcpServer{
			googleDriveMcpServerName: {Type: "stdio", Command: "flowpilot", Args: []string{"google-drive-mcp"}},
		}
	}
	return a
}

const claudeOkScript = `$null=[Console]::In.ReadLine(); ` +
	`Write-Output '{"type":"system","subtype":"init","session_id":"s1"}'; ` +
	`Write-Output '{"type":"result","subtype":"success","result":"ok"}'`

func TestClaudeSendTurnMcpAvailabilityMatrix(t *testing.T) {
	for _, yolo := range []bool{false, true} {
		t.Run(fmt.Sprintf("yolo=%v", yolo), func(t *testing.T) {
			capture, restore := captureClaudeSpawn(t, claudeOkScript)
			defer restore()

			a := wiredClaudeAdapter()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			if err := a.SendTurn(ctx, TurnRequest{RunID: "r", StepID: "s", Prompt: "hi", YoloMode: yolo}, &fakeClaudeBridge{}); err != nil {
				t.Fatalf("SendTurn: %v", err)
			}

			capture.mu.Lock()
			defer capture.mu.Unlock()
			if len(capture.args) != 1 {
				t.Fatalf("expected 1 spawn, got %d", len(capture.args))
			}
			args := capture.args[0]
			content := capture.mcpConfig[0]

			// MCP availability: --mcp-config + --strict-mcp-config present in BOTH YOLO states.
			if argIndex(args, "--mcp-config") < 0 {
				t.Fatalf("yolo=%v: --mcp-config must be present (MCP tools available): %v", yolo, args)
			}
			if argIndex(args, "--strict-mcp-config") < 0 {
				t.Fatalf("yolo=%v: --strict-mcp-config must always be present: %v", yolo, args)
			}
			// Both the flowpilot permission/ask_user server AND google-drive must be merged.
			if !strings.Contains(content, `"`+claudeMCPServerName+`"`) {
				t.Fatalf("yolo=%v: per-turn config missing flowpilot server: %s", yolo, content)
			}
			if !strings.Contains(content, `"`+googleDriveMcpServerName+`"`) {
				t.Fatalf("yolo=%v: per-turn config missing merged google-drive server: %s", yolo, content)
			}

			// Command-tool gating: --permission-prompt-tool ONLY when YOLO=off.
			hasPromptTool := argIndex(args, "--permission-prompt-tool") >= 0
			if yolo && hasPromptTool {
				t.Fatalf("yolo=on must NOT add --permission-prompt-tool (bypass): %v", args)
			}
			if !yolo && !hasPromptTool {
				t.Fatalf("yolo=off must add --permission-prompt-tool (gated): %v", args)
			}

			// Permission mode tracks YOLO.
			wantMode := "default"
			if yolo {
				wantMode = "bypassPermissions"
			}
			if !flagHasValue(args, "--permission-mode", wantMode) {
				t.Fatalf("yolo=%v: --permission-mode = ?, want %q: %v", yolo, wantMode, args)
			}
		})
	}
}

// TestClaudeSendTurnBaseURLMissingFailClosedVsDegrade guards the conditional fail-closed:
// a gated (YOLO=off) turn MUST fail closed if the MCP base URL is unavailable (never run
// ungated); a YOLO=on turn degrades gracefully (spawns without --mcp-config; ask_user falls
// back to the in-stream control_request route).
func TestClaudeSendTurnBaseURLMissingFailClosedVsDegrade(t *testing.T) {
	t.Run("yolo=off fails closed (no spawn)", func(t *testing.T) {
		capture, restore := captureClaudeSpawn(t, claudeOkScript)
		defer restore()

		a := newTestClaudeAdapter()
		a.mcpServer = newClaudeMCPServer()
		a.mcpBaseURL = func() string { return "" } // base URL not configured

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.SendTurn(ctx, TurnRequest{RunID: "r", StepID: "s", Prompt: "hi", YoloMode: false}, &fakeClaudeBridge{}); err == nil {
			t.Fatalf("yolo=off with no MCP base URL must fail closed (got nil error)")
		}
		capture.mu.Lock()
		defer capture.mu.Unlock()
		if len(capture.args) != 0 {
			t.Fatalf("fail-closed must NOT spawn claude, got %d spawns", len(capture.args))
		}
	})

	t.Run("yolo=on degrades (spawns without --mcp-config)", func(t *testing.T) {
		capture, restore := captureClaudeSpawn(t, claudeOkScript)
		defer restore()

		a := newTestClaudeAdapter()
		a.mcpServer = newClaudeMCPServer()
		a.mcpBaseURL = func() string { return "" }

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := a.SendTurn(ctx, TurnRequest{RunID: "r", StepID: "s", Prompt: "hi", YoloMode: true}, &fakeClaudeBridge{}); err != nil {
			t.Fatalf("yolo=on must degrade gracefully, got: %v", err)
		}
		capture.mu.Lock()
		defer capture.mu.Unlock()
		if len(capture.args) != 1 {
			t.Fatalf("expected 1 spawn, got %d", len(capture.args))
		}
		if argIndex(capture.args[0], "--mcp-config") >= 0 {
			t.Fatalf("yolo=on with no base URL must spawn WITHOUT --mcp-config: %v", capture.args[0])
		}
		// Still gated-by-bypass: never adds the permission prompt tool.
		if argIndex(capture.args[0], "--permission-prompt-tool") >= 0 {
			t.Fatalf("yolo=on must never add --permission-prompt-tool: %v", capture.args[0])
		}
	})
}
