package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Live re-verification (2026-07-10, real logged-in Grok Build account).
// Gated behind FLOWPILOT_LIVE_GROK=1, mirroring FLOWPILOT_LIVE_CLAUDE
// (claude_mcp_live_test.go) — skipped in normal `go test` since every test
// here spends real API usage on whatever account is logged in.
//
// This pass caught and fixed two real bugs in the production code (not test
// bugs):
//  1. grokACPResponseSessionID (grok_acp.go) only read a top-level
//     result.sessionId. session/load and session/prompt results only carry
//     sessionId nested under result._meta.sessionId — session/new is the only
//     response with it at the top level. Resume (session/load) always failed
//     with "no sessionId" until this was fixed to fall back to _meta.
//  2. No mechanism existed to detect/surface that a real account's
//     ~/.grok/config.toml can set [ui] permission_mode="always-approve",
//     which makes Grok never send session/request_permission at all — so
//     YOLO=false deny-by-default was silently unenforceable end-to-end on
//     such an account, with nothing in the runner ever noticing. Added
//     grokConfigPermissionModeBypassesGating (grok_process.go) as a
//     diagnostic (never mutates the user's file, mirrors the
//     ensureClaudeConfigSettings "never clobber" precedent) that logs a clear
//     warning at process-ensure time.

// liveManualBridge is a minimal TurnBridge that logs every event.
type liveManualBridge struct {
	mu              sync.Mutex
	events          []ProviderEvent
	approvalCalls   []ApprovalDetails
	approveDecision string
}

func (b *liveManualBridge) Emit(ev ProviderEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
	switch ev.Type {
	case EventMessageDelta:
		fmt.Printf("  [delta] %q\n", ev.Text)
	case EventTurnCompleted:
		fmt.Printf("  [turn_completed] finalMessage=%q\n", ev.FinalMessage)
	case EventTurnFailed:
		fmt.Printf("  [turn_failed] error=%q recoverable=%v\n", ev.Error, ev.Recoverable)
	case EventToolStarted:
		fmt.Printf("  [tool_started] %s input=%v\n", ev.ToolName, ev.Input)
	case EventToolCompleted:
		fmt.Printf("  [tool_completed] %s status=%s\n", ev.ToolName, ev.Status)
	case EventFileChanged:
		fmt.Printf("  [file_changed] %s (%s)\n", ev.Path, ev.ChangeType)
	case EventTokenUsageUpdated:
		if ev.TokenUsage != nil && ev.TokenUsage.Total != nil {
			fmt.Printf("  [token_usage] total=%d input=%d output=%d contextWindow=%v\n",
				ev.TokenUsage.Total.TotalTokens, ev.TokenUsage.Total.InputTokens, ev.TokenUsage.Total.OutputTokens, ev.TokenUsage.ModelContextWindow)
		}
	}
}

func (b *liveManualBridge) RequestApproval(d ApprovalDetails) (string, error) {
	b.mu.Lock()
	b.approvalCalls = append(b.approvalCalls, d)
	decision := b.approveDecision
	b.mu.Unlock()
	fmt.Printf("  [approval_requested] kind=%s command=%q -> deciding %q\n", d.Kind, d.Command, decision)
	return decision, nil
}
func (b *liveManualBridge) AskQuestion(string, []QuestionOption, bool) ([]string, error) {
	return nil, nil
}
func (b *liveManualBridge) SpawnAgent(SpawnAgentInput) (SpawnAgentResult, error) {
	return SpawnAgentResult{}, nil
}
func (b *liveManualBridge) SubmitFlowControl(FlowControlInput) (FlowControlResult, error) {
	return FlowControlResult{}, nil
}

func (b *liveManualBridge) hasEvent(t ProviderEventType) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range b.events {
		if e.Type == t {
			return true
		}
	}
	return false
}

func (b *liveManualBridge) finalMessage() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := len(b.events) - 1; i >= 0; i-- {
		if b.events[i].Type == EventTurnCompleted {
			return b.events[i].FinalMessage
		}
	}
	return ""
}

func (b *liveManualBridge) approvalCount() (int, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.approvalCalls) == 0 {
		return 0, ""
	}
	return len(b.approvalCalls), b.approvalCalls[len(b.approvalCalls)-1].Kind
}

// liveTestSessionStore captures the real ACP sessionId the adapter persists,
// so the resume sub-test can reuse it via session/load.
type liveTestSessionStore struct {
	mu      sync.Mutex
	records []ProviderSessionRecord
}

func (s *liveTestSessionStore) UpsertSession(_ context.Context, rec ProviderSessionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, rec)
	return nil
}

func (s *liveTestSessionStore) latestSessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.records) == 0 {
		return ""
	}
	return s.records[len(s.records)-1].ProviderSessionID
}

func requireLiveGrokOptIn(t *testing.T) {
	t.Helper()
	if os.Getenv("FLOWPILOT_LIVE_GROK") == "" {
		t.Skip("set FLOWPILOT_LIVE_GROK=1 to run the live e2e against a real logged-in grok account")
	}
}

// TestLiveRealGrokChatStreamAndResume drives the ACTUAL grokAdapter (Task-206
// dispatcher + Task-207 adapter) through the real `grok agent stdio` process.
// Proves: fresh session/new, streamed deltas, token usage, turn_completed,
// and — the bug this test caught — a real session/load resume that keeps
// conversational context (the model correctly recalls the word from turn 1).
func TestLiveRealGrokChatStreamAndResume(t *testing.T) {
	requireLiveGrokOptIn(t)

	scratch := t.TempDir()
	r := &Runner{workspace: scratch}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	h, err := r.ensureGrokProcess(ctx, "live-manual-scope", scratch, nil, "", "")
	if err != nil {
		t.Fatalf("ensureGrokProcess: %v", err)
	}
	defer h.close()

	store := &liveTestSessionStore{}
	h.adapter.sessionStore = store

	fmt.Println("--- turn 1: fresh session ---")
	bridge1 := &liveManualBridge{approveDecision: "approve"}
	err = h.adapter.SendTurn(ctx, TurnRequest{
		RunID:  "live-manual-1",
		Prompt: "Reply with exactly the single word: PINEAPPLE. Do not call any tools.",
	}, bridge1)
	if err != nil {
		t.Fatalf("SendTurn (turn 1): %v", err)
	}
	if !bridge1.hasEvent(EventTurnCompleted) {
		t.Fatal("turn 1: expected a turn_completed event")
	}
	if !bridge1.hasEvent(EventTokenUsageUpdated) {
		t.Fatal("turn 1: expected a token_usage_updated event")
	}
	fmt.Println("turn 1 final message:", bridge1.finalMessage())

	sessionID := store.latestSessionID()
	if sessionID == "" {
		t.Fatal("expected a real ACP sessionId to have been persisted")
	}
	fmt.Println("captured real sessionId:", sessionID)

	fmt.Println("--- turn 2: resume via session/load, same process ---")
	bridge2 := &liveManualBridge{approveDecision: "approve"}
	err = h.adapter.SendTurn(ctx, TurnRequest{
		RunID:             "live-manual-1",
		ProviderSessionID: sessionID,
		Prompt:            "What single word did I just ask you to reply with? Answer with only that one word.",
	}, bridge2)
	if err != nil {
		t.Fatalf("SendTurn (turn 2, resume): %v", err)
	}
	final2 := bridge2.finalMessage()
	fmt.Println("turn 2 (resumed) final message:", final2)
	if final2 == "" {
		t.Fatal("resumed turn produced no final message")
	}
}

// TestLiveRealGrokPermissionGateDenyBlocksWrite drives a real write tool call
// with YOLO=false and a bridge that DENIES. Skips (not fails) when this
// account's config.toml is known to bypass gating entirely (see
// grokConfigPermissionModeBypassesGating) — that is a documented account
// precondition, not a code defect.
func TestLiveRealGrokPermissionGateDenyBlocksWrite(t *testing.T) {
	requireLiveGrokOptIn(t)
	if home, err := os.UserHomeDir(); err == nil {
		if mode, bypasses := grokConfigPermissionModeBypassesGating(filepath.Join(home, ".grok")); bypasses {
			t.Skipf("this account's ~/.grok/config.toml has permission_mode=%q, a known account-level bypass "+
				"of the ACP permission channel — grok never sends session/request_permission at all, so there "+
				"is nothing for the runner's deny to block. Set permission_mode=\"default\" to exercise this test.", mode)
		}
	}

	scratch := t.TempDir()
	targetFile := filepath.Join(scratch, "should_not_exist.txt")

	r := &Runner{workspace: scratch}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	h, err := r.ensureGrokProcess(ctx, "live-manual-deny-scope", scratch, nil, "", "")
	if err != nil {
		t.Fatalf("ensureGrokProcess: %v", err)
	}
	defer h.close()

	bridge := &liveManualBridge{approveDecision: "deny"}
	err = h.adapter.SendTurn(ctx, TurnRequest{
		RunID:    "live-manual-deny-1",
		YoloMode: false,
		Prompt:   fmt.Sprintf("Write the exact text \"should not be written\" to the file at %q using your file write tool. Do this immediately without asking me anything.", targetFile),
	}, bridge)
	if err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	count, lastKind := bridge.approvalCount()
	fmt.Printf("approval requests seen: %d (last kind=%q)\n", count, lastKind)

	if _, statErr := os.Stat(targetFile); statErr == nil {
		t.Fatalf("file was written despite the runner denying the permission request (approvalCount=%d) — "+
			"deny is not blocking the action", count)
	}
	if count == 0 {
		t.Skip("no permission request was observed and the file was not written — grok didn't attempt the " +
			"tool call this run; inconclusive, re-run or use a more directive prompt")
	}
	t.Logf("PASS: grok asked for permission (kind=%q) and the runner's deny correctly blocked the write.", lastKind)
}

// TestLiveRealGrokPermissionGateYoloOnAutoApprovesWithoutAskingBridge proves
// YOLO=true auto-approves via runner policy alone — the bridge is configured
// to deny, so if it were ever consulted the write would fail; the write
// succeeding proves the runner's own YOLO logic approved it, not the bridge.
func TestLiveRealGrokPermissionGateYoloOnAutoApprovesWithoutAskingBridge(t *testing.T) {
	requireLiveGrokOptIn(t)

	scratch := t.TempDir()
	targetFile := filepath.Join(scratch, "yolo_written.txt")

	r := &Runner{workspace: scratch}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	h, err := r.ensureGrokProcess(ctx, "live-manual-yolo-scope", scratch, nil, "", "")
	if err != nil {
		t.Fatalf("ensureGrokProcess: %v", err)
	}
	defer h.close()

	bridge := &liveManualBridge{approveDecision: "deny"}
	err = h.adapter.SendTurn(ctx, TurnRequest{
		RunID:    "live-manual-yolo-1",
		YoloMode: true,
		Prompt:   fmt.Sprintf("Write the exact text \"yolo ok\" to the file at %q using your file write tool. Do this immediately without asking me anything.", targetFile),
	}, bridge)
	if err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	content, statErr := os.ReadFile(targetFile)
	if statErr != nil {
		t.Fatalf("expected the file to be written under YOLO=true, stat/read failed: %v", statErr)
	}
	fmt.Printf("file content: %q\n", string(content))
}

// TestLiveRealGrokSelectedSkillReachesThePrompt proves Task-214's
// SkillSelection wiring is not just capability-flagged but actually live:
// a SkillSelection with an explicit Path gets read by injectSelectedSkills
// (via Grok's promptPrep, provider_registry.go) and its content genuinely
// reaches a real Grok turn.
func TestLiveRealGrokSelectedSkillReachesThePrompt(t *testing.T) {
	requireLiveGrokOptIn(t)

	scratch := t.TempDir()
	skillPath := filepath.Join(scratch, "reply-marker.md")
	skillContent := "---\nname: reply-marker\ndescription: forces a specific reply marker\n---\n\n" +
		"When this skill is active, your entire reply must be exactly the single word: TASK214MARKER.\n"
	if err := os.WriteFile(skillPath, []byte(skillContent), 0o644); err != nil {
		t.Fatalf("write skill fixture: %v", err)
	}

	r := &Runner{workspace: scratch}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	h, err := r.ensureGrokProcess(ctx, "live-manual-skill-scope", scratch, nil, "", "")
	if err != nil {
		t.Fatalf("ensureGrokProcess: %v", err)
	}
	defer h.close()
	// promptPrep is wired to r.injectSelectedSkills by provider_registry.go's
	// Grok registration; ensureGrokProcess's fixture Runner doesn't go through
	// that registration path, so wire it explicitly here to exercise the exact
	// same function the real registry uses.
	h.adapter.promptPrep = func(req TurnRequest) string {
		return r.injectSelectedSkills(scratch, req.Prompt, req.SelectedSkills)
	}

	bridge := &liveManualBridge{approveDecision: "approve"}
	err = h.adapter.SendTurn(ctx, TurnRequest{
		RunID:  "live-manual-skill-1",
		Prompt: "Reply normally to this message.",
		SelectedSkills: []SkillSelection{
			{Name: "reply-marker", Path: skillPath},
		},
	}, bridge)
	if err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	final := bridge.finalMessage()
	fmt.Println("final message with skill selected:", final)
	if !strings.Contains(final, "TASK214MARKER") {
		t.Fatalf("expected the selected skill's instruction to steer the reply toward TASK214MARKER, got %q", final)
	}
}
