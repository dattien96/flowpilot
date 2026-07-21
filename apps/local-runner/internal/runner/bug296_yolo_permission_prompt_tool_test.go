package runner

// BUG-296: a flow coding child (agent.delegate, non-reviewer) running on Claude
// under YOLO=true resolves a posture of ClaudePermissionMode="default" (gated)
// + RunnerAutoApprove=true (V9-21 forceShellBridge, so the git-commit denylist
// stays reachable). claudeArgs used to gate --permission-prompt-tool on
// !posture.RunnerAutoApprove — false here — so it launched Claude gated with no
// configured way to answer its own permission prompt. Headless Claude (no TTY)
// then failed every gated tool call closed on its own side, before FlowPilot's
// approval bridge (which would have auto-approved) ever saw a request — hence
// zero approvals.ndjson entries for a run whose dispatch envelope recorded
// yolo:true throughout.
//
// The fix (claude_permission_mcp.go) keys --permission-prompt-tool off whether
// ClaudePermissionMode is actually gated (!= "bypassPermissions"), independent
// of the separate RunnerAutoApprove auto-decide flag.
//
// These tests pin: (1) the fix itself and its non-regression case for Claude,
// (2) the shared RequestApproval decision logic all three providers rely on,
// and (3) that Codex's approval WIRING still reaches the bridge under the same
// forceShellBridge+YOLO combination that broke Claude (a gap that existed
// alongside Claude's — no prior test exercised this exact combination for
// Codex either). Grok is not given a new test here: its handleInbound
// (grok_adapter.go) auto-approves via its OWN yolo branch before ever
// consulting GrokPermissionMode/forceShellBridge, so the existing
// TestGrokAdapterYoloOnAutoApprovesViaRunnerPolicyNotBridge (grok_adapter_test.go)
// already covers this exact combination unchanged.
//
// additive-tests-only: no existing test is modified.

import (
	"context"
	"testing"
)

// ---- Claude: the fix + its non-regression case -----------------------------

func TestBug296ClaudeArgsAttachesPermissionPromptToolUnderForceShellBridge(t *testing.T) {
	// V9-21 flow-coding-child-under-YOLO posture: gated mode, but RunnerAutoApprove
	// stays true. Before the fix, this combination made claudeArgs omit the flag.
	posture := resolveYoloPostureForTurn(true, true)
	if posture.ClaudePermissionMode != "default" || !posture.RunnerAutoApprove {
		t.Fatalf("precondition: posture = %+v, want ClaudePermissionMode=default, RunnerAutoApprove=true", posture)
	}
	args := claudeArgs(posture, "", "/tmp/mcp.json", "", "", nil)
	if !flagHasValue(args, "--permission-prompt-tool", claudeApproveToolName) {
		t.Fatalf("BUG-296: gated ClaudePermissionMode must still attach --permission-prompt-tool even when RunnerAutoApprove=true, got: %v", args)
	}
	if !flagHasValue(args, "--permission-mode", "default") {
		t.Fatalf("expected --permission-mode default, got: %v", args)
	}
}

func TestBug296ClaudeArgsOmitsPermissionPromptToolUnderPlainYolo(t *testing.T) {
	// Non-regression: plain YOLO (no forceShellBridge) still fully bypasses —
	// ClaudePermissionMode="bypassPermissions" — so the flag must stay omitted,
	// exactly as before this fix.
	posture := resolveYoloPostureForTurn(true, false)
	if posture.ClaudePermissionMode != "bypassPermissions" {
		t.Fatalf("precondition: ClaudePermissionMode = %q, want bypassPermissions", posture.ClaudePermissionMode)
	}
	args := claudeArgs(posture, "", "/tmp/mcp.json", "", "", nil)
	if argIndex(args, "--permission-prompt-tool") >= 0 {
		t.Fatalf("plain YOLO (bypassPermissions) must NOT attach --permission-prompt-tool, got: %v", args)
	}
}

// ---- Shared bridge: the decision logic every provider relies on -----------

func TestBug296RequestApprovalAutoApprovesOrdinaryWriteUnderYolo(t *testing.T) {
	// Provider-neutral: turnBridge.RequestApproval is the single shared decision
	// point Claude/Codex/Grok all route (or, for Claude, are SUPPOSED to route)
	// approval requests through. Under yolo=true it must auto-approve an
	// ordinary (non-commit) write, regardless of which provider is asking.
	svc := NewInteractiveService()
	rs := &interactiveRun{id: "run-296", workspaceCwd: t.TempDir()}
	svc.runs[rs.id] = rs
	b := &turnBridge{svc: svc, rs: rs, ctx: context.Background(), yolo: true}

	decision, err := b.RequestApproval(ApprovalDetails{Kind: "write", Command: "edit calc.go"})
	if err != nil || decision != "approve" {
		t.Fatalf("yolo=true ordinary write: got (%q, %v), want (approve, nil)", decision, err)
	}
}

// ---- Codex: wiring still reaches the bridge under the same combination ----

func TestBug296CodexForceShellBridgeYoloStillRoutesApprovalRequest(t *testing.T) {
	// Same class of combination that broke Claude (gated-but-auto-approve under
	// a flow coding child's YOLO): CodexApprovalMode stays "untrusted" (not
	// "never") under forceShellBridge, so Codex is expected to still SEND the
	// approval request. Unlike Claude, Codex's handleInbound has no launch-time
	// gate that could silently drop the channel — this pins that it doesn't.
	d, fc := startFakeCodex(t, nil)
	adapter := newCodexAdapter(d, "/workspace")
	d.setInbound(adapter.handleInbound)

	threadParams := make(chan map[string]any, 1)
	approvalReplies := make(chan map[string]any, 1)
	fc.serve(func(fc *fakeCodex, m map[string]any) {
		method, _ := m["method"].(string)
		switch method {
		case "thread/start":
			params, _ := m["params"].(map[string]any)
			threadParams <- params
			fc.reply(m["id"], map[string]any{"threadId": "th1"})
		case "turn/start":
			fc.reply(m["id"], map[string]any{"turnId": "ct1"})
			fc.send(map[string]any{"jsonrpc": "2.0", "id": 600, "method": "item/fileChange/requestApproval", "params": map[string]any{
				"threadId": "th1", "turnId": "ct1", "itemId": "item1", "path": "calc.go",
			}})
		default:
			if res, ok := m["result"].(map[string]any); ok {
				approvalReplies <- res
				fc.notify("turn.completed", map[string]any{"threadId": "th1", "finalMessage": "ok"})
			}
		}
	})

	bridge := &captureBridge{approveWith: "approve"}
	err := adapter.SendTurn(context.Background(), TurnRequest{
		RunID: "r1", Prompt: "fix calc.go", YoloMode: true, ForceShellBridge: true,
	}, bridge)
	if err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	select {
	case params := <-threadParams:
		if params["sandbox"] != "danger-full-access" || params["approvalMode"] != "untrusted" {
			t.Fatalf("thread/start sandbox=%v approvalMode=%v, want danger-full-access/untrusted (V9-21 forceShellBridge)", params["sandbox"], params["approvalMode"])
		}
	default:
		t.Fatal("did not observe thread/start params")
	}
	if got := bridge.approvalRequestCount(); got != 1 {
		t.Fatalf("BUG-296 class: forceShellBridge+YOLO must still route the file-change approval to the runner bridge, got %d requests", got)
	}
	select {
	case res := <-approvalReplies:
		if res["decision"] != "accept" {
			t.Fatalf("decision = %v, want accept", res["decision"])
		}
	default:
		t.Fatal("approval reply was not sent")
	}
}
