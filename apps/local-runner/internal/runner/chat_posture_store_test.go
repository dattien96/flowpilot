package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Chat posture store (Task-xxx/CA-xxx): GET/PUT /client/chat-posture against a
// runner-owned shared file; both TUI and Desktop read/write only through here.

func TestChatPostureDefaultConfig(t *testing.T) {
	cfg := defaultChatPostureConfig()
	// CA-685: the operator-set default posture is now "non" (no mode).
	if cfg.Active != ChatPostureNon {
		t.Fatalf("default active = %q, want non", cfg.Active)
	}
	if !IsReadOnlyChatPosture(ChatPostureScan) || !IsReadOnlyChatPosture(ChatPosturePlan) {
		t.Fatal("scan/plan must be read-only")
	}
	if IsReadOnlyChatPosture(ChatPostureCode) || IsReadOnlyChatPosture("") {
		t.Fatal("code/empty must not be read-only")
	}
}

func TestChatPostureStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLOWPILOT_CHAT_POSTURE_FILE", filepath.Join(dir, "chat-posture.json"))

	cfg := ChatPostureConfig{
		Active: ChatPostureScan,
		Profiles: map[string]ChatPostureProfile{
			ChatPostureScan: {Provider: "claude", Model: "sonnet", ReasoningEffort: "high"},
			ChatPosturePlan: {Yolo: boolPtr(false)},
		},
	}
	path, err := saveChatPosture(cfg)
	if err != nil {
		t.Fatalf("saveChatPosture: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved file missing: %v", err)
	}
	loaded, loadedPath := loadChatPosture()
	if loadedPath != path {
		t.Fatalf("loadChatPosture path = %q, want %q", loadedPath, path)
	}
	if loaded.Active != ChatPostureScan {
		t.Fatalf("loaded active = %q, want scan", loaded.Active)
	}
	scan := loaded.Profiles[ChatPostureScan]
	if scan.Provider != "claude" || scan.Model != "sonnet" || scan.ReasoningEffort != "high" {
		t.Fatalf("loaded scan profile = %+v", scan)
	}
	plan := loaded.Profiles[ChatPosturePlan]
	if plan.Yolo == nil || *plan.Yolo {
		t.Fatalf("loaded plan yolo = %v, want false", plan.Yolo)
	}
}

func TestChatPostureStore_MissingFileReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLOWPILOT_CHAT_POSTURE_FILE", filepath.Join(dir, "nope.json"))
	cfg, path := loadChatPosture()
	// CA-685: missing file defaults to "non".
	if cfg.Active != ChatPostureNon {
		t.Fatalf("missing file active = %q, want default non", cfg.Active)
	}
	if path == "" {
		t.Fatal("missing file should still report the candidate path")
	}
}

func TestChatPostureStore_Normalize(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLOWPILOT_CHAT_POSTURE_FILE", filepath.Join(dir, "chat-posture.json"))

	// Invalid active falls back to the default ("non" per CA-685); unknown
	// profile keys are dropped; values are trimmed/lowercased.
	cfg := ChatPostureConfig{
		Active: "nonsense",
		Profiles: map[string]ChatPostureProfile{
			ChatPostureCode:  {Provider: "  codex  ", ReasoningEffort: "HIGH"},
			"evil":           {Provider: "claude"},
			ChatPostureScan:  {Model: "x"},
			ChatPosturePlan:  {},
		},
	}
	_, err := saveChatPosture(cfg)
	if err != nil {
		t.Fatalf("saveChatPosture: %v", err)
	}
	loaded, _ := loadChatPosture()
	if loaded.Active != ChatPostureNon {
		t.Fatalf("normalized active = %q, want non", loaded.Active)
	}
	if _, ok := loaded.Profiles["evil"]; ok {
		t.Fatal("unknown profile key must be dropped")
	}
	if got := loaded.Profiles[ChatPostureCode].Provider; got != "codex" {
		t.Fatalf("provider not trimmed = %q", got)
	}
	if got := loaded.Profiles[ChatPostureCode].ReasoningEffort; got != "high" {
		t.Fatalf("reasoning not lowercased = %q", got)
	}
}

func TestResolveTurnChatPosture(t *testing.T) {
	rs := &interactiveRun{chatPosture: ChatPostureCode, runKind: "chat"}

	// Explicit valid posture overrides the run default.
	if got := resolveTurnChatPosture(rs, TurnInput{ChatPosture: ChatPosturePlan}); got != ChatPosturePlan {
		t.Fatalf("explicit plan = %q, want plan", got)
	}
	// Empty input keeps the run default.
	if got := resolveTurnChatPosture(rs, TurnInput{}); got != ChatPostureCode {
		t.Fatalf("default = %q, want code", got)
	}
	// Invalid explicit value falls back to the run default.
	if got := resolveTurnChatPosture(rs, TurnInput{ChatPosture: "evil"}); got != ChatPostureCode {
		t.Fatalf("invalid = %q, want code", got)
	}
	// Flow/workflow runs never run a read-only posture.
	flow := &interactiveRun{chatPosture: ChatPostureScan, runKind: "chat", flowEngineDriven: true}
	if got := resolveTurnChatPosture(flow, TurnInput{}); got != "" {
		t.Fatalf("flow run posture = %q, want empty (never read-only)", got)
	}
	wf := &interactiveRun{chatPosture: ChatPostureScan, runKind: "chat", workflowID: "wf-1"}
	if got := resolveTurnChatPosture(wf, TurnInput{}); got != "" {
		t.Fatalf("workflow run posture = %q, want empty", got)
	}
	// Nil run → empty.
	if got := resolveTurnChatPosture(nil, TurnInput{ChatPosture: ChatPostureScan}); got != "" {
		t.Fatalf("nil run posture = %q, want empty", got)
	}
}

// RequestApproval hook (Task-xxx/CA-xxx): a read-only posture auto-decides
// WITHOUT asking, and wins over the profile's YOLO flag (checked before the
// YOLO auto-approve branch).
func TestReadOnlyPosture_RequestApprovalAutoDecides(t *testing.T) {
	svc := NewInteractiveService()
	rs := &interactiveRun{id: "run-posture-hook", workspaceCwd: t.TempDir()}
	svc.runs[rs.id] = rs

	// YOLO=true but posture=scan: a write must be DENIED (never auto-approved
	// by the YOLO branch), and no pending approval may be left behind.
	b := &turnBridge{svc: svc, rs: rs, ctx: context.Background(), turnID: "t-scan", yolo: true, posture: ChatPostureScan}

	started := make(chan struct{})
	done := make(chan string, 1)
	go func() {
		close(started)
		d, err := b.RequestApproval(ApprovalDetails{Kind: "file", Reason: "Write"})
		if err != nil {
			t.Errorf("RequestApproval: %v", err)
		}
		done <- d
	}()
	<-started
	select {
	case d := <-done:
		if d != "deny" {
			t.Fatalf("scan write decision = %q, want deny (YOLO must not leak a write)", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scan posture must auto-decide, not wait for a human")
	}
	if rs.pendingApprovalID != "" {
		t.Fatalf("scan posture left a pending approval %q", rs.pendingApprovalID)
	}

	// Read: approved without asking, even under YOLO=false.
	b2 := &turnBridge{svc: svc, rs: rs, ctx: context.Background(), turnID: "t-scan-read", yolo: false, posture: ChatPostureScan}
	if d, err := b2.RequestApproval(ApprovalDetails{Kind: "file", Reason: "Read"}); err != nil || d != "approve" {
		t.Fatalf("scan read = %q, %v; want approve", d, err)
	}
}

func TestReadOnlyPosture_CodePostureUnchanged(t *testing.T) {
	svc := NewInteractiveService()
	rs := &interactiveRun{id: "run-posture-code", workspaceCwd: t.TempDir()}
	svc.runs[rs.id] = rs

	// code posture with YOLO=true still auto-approves writes (normal chat).
	b := &turnBridge{svc: svc, rs: rs, ctx: context.Background(), turnID: "t-code", yolo: true, posture: ChatPostureCode}
	if d, err := b.RequestApproval(ApprovalDetails{Kind: "file", Reason: "Write"}); err != nil || d != "approve" {
		t.Fatalf("code+yolo write = %q, %v; want approve", d, err)
	}
}

func TestChatPostureProfileJSONWireName(t *testing.T) {
	// Desktop sends reasoningEffort (matches the turn body). A profile
	// marshalled with that wire name must decode with the value intact.
	canonical := `{"provider":"claude","model":"sonnet","reasoningEffort":"high","yolo":true}`
	var p ChatPostureProfile
	if err := json.Unmarshal([]byte(canonical), &p); err != nil {
		t.Fatalf("decode canonical: %v", err)
	}
	if p.ReasoningEffort != "high" {
		t.Fatalf("canonical reasoningEffort = %q, want high", p.ReasoningEffort)
	}

	// A file written before the rename used "reasoning"; the alias must still
	// load so no older posture document loses its reasoning pin.
	legacy := `{"provider":"claude","model":"sonnet","reasoning":"low","yolo":false}`
	var p2 ChatPostureProfile
	if err := json.Unmarshal([]byte(legacy), &p2); err != nil {
		t.Fatalf("decode legacy alias: %v", err)
	}
	if p2.ReasoningEffort != "low" {
		t.Fatalf("legacy reasoning alias = %q, want low", p2.ReasoningEffort)
	}

	// Re-marshal emits the canonical wire name only (no reasoning alias).
	re, err := json.Marshal(p2)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(re), `"reasoningEffort"`) {
		t.Fatalf("re-marshal missing canonical reasoningEffort: %s", re)
	}
	if strings.Contains(string(re), `"reasoning"`) {
		t.Fatalf("re-marshal leaked legacy reasoning alias: %s", re)
	}
}

func boolPtr(v bool) *bool { return &v }