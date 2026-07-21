package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestClaudeRestartRestoresFirstPromptWhenHubNeverCalledProviderOnTurnOne
// reproduces a real production symptom (run-17987 / run-18371): a flow-hub's
// turn 1 spawns its entry child directly and never itself makes a real
// provider call, so the hub's Claude session file starts directly at the
// round-0 synthesis turn — there is no "user" frame anywhere in the session
// for the original prompt. Before the fix, overlayRawTurnPrompts had no
// turn_started slot to overlay the raw prompt onto, so it was silently
// dropped from the restored transcript after a server restart, even though
// it rendered fine in the live (pre-restart) chat.
func TestClaudeRestartRestoresFirstPromptWhenHubNeverCalledProviderOnTurnOne(t *testing.T) {
	root := t.TempDir()
	acctHome := filepath.Join(root, "claude-home")
	sessionID := "claude-hub-session"
	claudeDir := filepath.Join(acctHome, ".claude", "projects", "project")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	joinedNote := "[FlowPilot system note — sub-agents started in this session via the UI (not by you):\n" +
		"- [flow-engine] An agent has already been spawned to work on this request.]\n\n" +
		"[flow-engine joined result note]\nFlow round 0 — 1 result joined.\n" +
		"\"reviewer\" (claude): APPROVE\n---\n" +
		"[flow-engine] Agent results ready. You must call submit_review_outcome."
	// The session file's ONLY user frame is the synthesis turn — no frame at
	// all carries the original "fix bug 1+1 != 2" prompt, because turn 1 never
	// made a real provider call.
	writeLinesToPath083(t, filepath.Join(claudeDir, sessionID+".jsonl"), []string{
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":` + jsonQuote083(joinedNote) + `}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Both reviewers approved."}]}}`,
	})

	store := newRun2334TurnLogStore(t, root, "run-hub-first-turn", []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "turn-1", Prompt: "fix bug 1+1 != 2"},
		{Kind: turnLogKindPrompt, TurnID: "turn-2", Prompt: joinedNote},
	})
	writeProviderAccountsConfig083(t, root, []ProviderAccount{{
		ID: "acct-claude", ProviderKey: string(ProviderKeyClaude), HomePath: acctHome,
		SlotIndex: 1, AuthStatus: "connected", IsActive: true,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := newRun2334ResumedChat("run-hub-first-turn", ProviderKeyClaude, "acct-claude", sessionID)

	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	svc.seedTranscriptFromDisk(rs)

	var sawOriginal bool
	for _, e := range rs.events {
		if e.Type == EventTurnStarted && e.Prompt == "fix bug 1+1 != 2" {
			sawOriginal = true
		}
		if e.Type == EventTurnStarted && isSystemPrompt(e.Prompt) {
			t.Fatalf("internal joined-result-note prompt must not replay as a user-facing bubble: %q", e.Prompt)
		}
	}
	if !sawOriginal {
		t.Fatalf("original first prompt was dropped from the restored transcript: %+v", rs.events)
	}
}

func jsonQuote083(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
