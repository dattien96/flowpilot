package runner

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// CP-57 Task-303 gap closure (post-BUG-329): the Opencode YOLO posture loop and
// the adapter-level approval channel. Before this file, ApplyOpencodeYoloPosture
// had no production caller test, the HTTP route was absent, and no adapter-level
// test exercised permission → bridge approvals (deny / YOLO auto-approve /
// question-never-auto-approved). Additive: no pre-existing test is edited.

func writeOpencodeAccountFixture(t *testing.T, home string) {
	t.Helper()
	authDir := filepath.Join(home, ".local", "share", "opencode")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("MkdirAll auth dir: %v", err)
	}
	auth := `{"opencode":{"type":"api","key":"sk-ca-test"},"xai":{"type":"oauth","refresh":"r","access":"a","expires":1787820129233}}`
	if err := os.WriteFile(filepath.Join(authDir, "auth.json"), []byte(auth), 0o600); err != nil {
		t.Fatalf("WriteFile auth.json: %v", err)
	}
}

func TestApplyOpencodeYoloPostureFlipsAutoAndClosesLiveProcesses(t *testing.T) {
	isolatedHome := t.TempDir()
	t.Setenv("HOME", isolatedHome)
	t.Setenv("USERPROFILE", isolatedHome)
	root := t.TempDir()
	ocHome := filepath.Join(root, "oc-home")
	writeOpencodeAccountFixture(t, ocHome)
	writeProviderAccountsConfig083(t, root, []ProviderAccount{
		{ID: "acct-oc", ProviderKey: "opencode", HomePath: ocHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})

	r, _ := New(".")
	r.opencodeProcessMu.Lock()
	r.opencodeProcesses = map[string]*opencodeProcessHandle{
		opencodeProcessKey("acct-oc", "opencode/muse-spark-1.2-contributor-free", "high", false): {
			scopeKey: "acct-oc", dispatcher: newOpencodeDispatcher(io.Discard, nil),
		},
	}
	r.opencodeProcessMu.Unlock()

	if err := r.ApplyOpencodeYoloPosture(context.Background(), true); err != nil {
		t.Fatalf("ApplyOpencodeYoloPosture(true): %v", err)
	}
	r.opencodeProcessMu.Lock()
	auto := r.opencodeDesiredAuto
	live := 0
	for _, h := range r.opencodeProcesses {
		if !h.dispatcher.isClosed() {
			live++
		}
	}
	r.opencodeProcessMu.Unlock()
	if !auto {
		t.Fatal("opencodeDesiredAuto must flip true")
	}
	if live != 0 {
		t.Fatalf("live processes must be closed so the next turn respawns, got %d", live)
	}

	if err := r.ApplyOpencodeYoloPosture(context.Background(), false); err != nil {
		t.Fatalf("ApplyOpencodeYoloPosture(false): %v", err)
	}
	r.opencodeProcessMu.Lock()
	auto = r.opencodeDesiredAuto
	r.opencodeProcessMu.Unlock()
	if auto {
		t.Fatal("opencodeDesiredAuto must flip back false")
	}
}

// denyBridge mirrors fakeOpencodeBridge but denies every approval.
type bugCAIDenyBridge struct {
	fakeOpencodeBridge
}

func (b *bugCAIDenyBridge) RequestApproval(details ApprovalDetails) (string, error) {
	b.approvals = append(b.approvals, details)
	return "deny", nil
}

func bugCAIWaitReply(t *testing.T, fg *fakeOpencode, wantID float64) map[string]any {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case m := <-fg.requests:
			if id, ok := m["id"].(float64); ok && id == wantID {
				return m
			}
		case <-deadline:
			t.Fatalf("timeout waiting for reply to id %v", wantID)
		}
	}
}

func TestOpencodeApprovalDeniedRoutesToBridgeAndRejects(t *testing.T) {
	d, fg := startFakeOpencode(t, nil)
	a := newOpencodeAdapter(d, "/tmp")
	bridge := &bugCAIDenyBridge{}
	a.mu.Lock()
	a.bridges["session-a"] = bridge
	a.yoloModes["session-a"] = false // YOLO off → live gating
	a.mu.Unlock()

	fg.send(map[string]any{"jsonrpc": "2.0", "id": 77, "method": "session/request_permission", "params": map[string]any{
		"sessionId": "session-a",
		"toolCall":  map[string]any{"title": "bash", "rawInput": map[string]any{"command": "rm -rf /"}},
		"options": []any{
			map[string]any{"optionId": "once", "kind": "allow_once"},
			map[string]any{"optionId": "reject", "kind": "reject_once"},
		},
	}})

	reply := bugCAIWaitReply(t, fg, 77)
	outcome, _ := reply["result"].(map[string]any)
	outcomeMap, _ := outcome["outcome"].(map[string]any)
	if got, _ := outcomeMap["optionId"].(string); got != "reject" {
		t.Fatalf("deny decision must reply optionId=reject, got %q", got)
	}
	if len(bridge.approvals) != 1 {
		t.Fatalf("bridge must see exactly one approval request, got %d", len(bridge.approvals))
	}
	if bridge.approvals[0].Command != "rm -rf /" {
		t.Fatalf("approval details must carry the raw command, got %q", bridge.approvals[0].Command)
	}
	if bridge.approvals[0].Kind != "exec" {
		t.Fatalf("bash command must classify as exec, got %q", bridge.approvals[0].Kind)
	}
}

func TestOpencodeYoloAutoApprovesToolPermissionButNotQuestion(t *testing.T) {
	d, fg := startFakeOpencode(t, nil)
	a := newOpencodeAdapter(d, "/tmp")
	bridge := &bugCAIDenyBridge{}
	a.mu.Lock()
	a.bridges["session-a"] = bridge
	a.yoloModes["session-a"] = true // YOLO on
	a.mu.Unlock()

	// 1) Tool permission under YOLO → auto-approve, bridge NOT consulted.
	fg.send(map[string]any{"jsonrpc": "2.0", "id": 81, "method": "session/request_permission", "params": map[string]any{
		"sessionId": "session-a",
		"toolCall":  map[string]any{"title": "write", "rawInput": map[string]any{"filepath": "/tmp/x.go"}},
		"options": []any{
			map[string]any{"optionId": "once", "kind": "allow_once"},
			map[string]any{"optionId": "reject", "kind": "reject_once"},
		},
	}})
	reply := bugCAIWaitReply(t, fg, 81)
	outcome, _ := reply["result"].(map[string]any)
	outcomeMap, _ := outcome["outcome"].(map[string]any)
	if got, _ := outcomeMap["optionId"].(string); got != "once" {
		t.Fatalf("YOLO must auto-approve with an allow optionId, got %q", got)
	}
	if len(bridge.approvals) != 0 {
		t.Fatalf("YOLO tool permission must not reach the bridge, got %d", len(bridge.approvals))
	}

	// 2) ask-user style "question" under YOLO → NEVER auto-approved (spec Q-3).
	fg.send(map[string]any{"jsonrpc": "2.0", "id": 82, "method": "question", "params": map[string]any{
		"sessionId": "session-a",
		"options": []any{
			map[string]any{"optionId": "once", "kind": "allow_once"},
			map[string]any{"optionId": "reject", "kind": "reject_once"},
		},
	}})
	reply = bugCAIWaitReply(t, fg, 82)
	outcome, _ = reply["result"].(map[string]any)
	outcomeMap, _ = outcome["outcome"].(map[string]any)
	if got, _ := outcomeMap["optionId"].(string); got != "reject" {
		t.Fatalf("question must NOT be auto-approved under YOLO, got %q", got)
	}
	if len(bridge.approvals) != 1 {
		t.Fatalf("question must reach the bridge under YOLO, got %d", len(bridge.approvals))
	}
}

func TestOpencodePermissionRequestWithoutBridgeDenies(t *testing.T) {
	d, fg := startFakeOpencode(t, nil)
	_ = newOpencodeAdapter(d, "/tmp") // no bridge registered for session-z
	fg.send(map[string]any{"jsonrpc": "2.0", "id": 91, "method": "session/request_permission", "params": map[string]any{
		"sessionId": "session-z",
		"options": []any{
			map[string]any{"optionId": "once", "kind": "allow_once"},
			map[string]any{"optionId": "reject", "kind": "reject_once"},
		},
	}})
	reply := bugCAIWaitReply(t, fg, 91)
	raw, _ := json.Marshal(reply["result"])
	if !jsonContains(raw, "reject") {
		t.Fatalf("missing bridge must fail closed (reject), got %s", raw)
	}
}

func jsonContains(raw []byte, sub string) bool {
	return len(raw) > 0 && jsonBytesContains(raw, sub)
}

func jsonBytesContains(raw []byte, sub string) bool {
	for i := 0; i+len(sub) <= len(raw); i++ {
		if string(raw[i:i+len(sub)]) == sub {
			return true
		}
	}
	return false
}
