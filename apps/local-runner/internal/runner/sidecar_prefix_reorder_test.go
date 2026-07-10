package runner

// Regression test for BUG-StaleQuestion-Restart-Ordering: after BUG-271/
// Task-183, reconstructRun seeds the CP-41 flow-events sidecar (including a
// restored, answered user_question_required) into rs.events BEFORE the real
// transcript is loaded by the later, separate seedTranscriptFromDisk call —
// so the sidecar-origin event always claimed the lowest Seq numbers and
// rendered pinned above the entire prior conversation after a full server
// restart, regardless of when it actually happened. seedTranscriptFromDisk
// now moves that sidecar prefix (rs.sidecarPrefixCount) to the end of the
// timeline once it has appended the replayed transcript.

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSeedTranscriptFromDiskMovesSidecarPrefixAfterReplayedTranscript(t *testing.T) {
	root := t.TempDir()
	acctHome := filepath.Join(root, "user-home")

	claudeLines := []string{
		`{"type":"system","subtype":"init","session_id":"s-abc","cwd":"/repo"}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"do the thing"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}`,
		`{"type":"result","subtype":"success","result":"done"}`,
	}
	sessionID := "ses-order-1"
	claudeDir := filepath.Join(acctHome, ".claude", "projects", "proj-hash")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeLinesToPath083(t, filepath.Join(claudeDir, sessionID+".jsonl"), claudeLines)

	writeProviderAccountsConfig083(t, root, []ProviderAccount{
		{ID: "acct-c", ProviderKey: "claude", HomePath: acctHome, SlotIndex: 1, AuthStatus: "connected", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), IsActive: true},
	})

	runID := "run-sidecar-order"
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.activeAccountID = "acct-c"

	// rs.events pre-populated exactly as reconstructRun leaves it: one restored,
	// answered user_question_required at Seq 1, with sidecarPrefixCount
	// recording that it (and only it) was seeded before any transcript existed.
	rs := &interactiveRun{
		id:                    runID,
		providerKey:           ProviderKeyClaude,
		providerSessionID:     sessionID,
		realProviderSessionID: sessionID,
		providerAccountID:     "acct-c",
		workspaceCwd:          "/repo",
		runKind:               "chat",
		status:                RunStatusCompleted,
		createdAt:             time.Now().UTC().Format(time.RFC3339Nano),
		resumedFromDisk:       true,
		subs:                  map[int64]chan ProviderEvent{},
		idempotency:           map[string]string{},
		seq:                   1,
		sidecarPrefixCount:    1,
		events: []ProviderEvent{
			{
				Seq:        1,
				Type:       EventUserQuestionRequired,
				QuestionID: "q-1",
				Prompt:     "Use Drive?",
				Answer:     []string{"__skip__"},
			},
		},
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()

	svc.seedTranscriptFromDisk(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	if len(events) < 2 {
		t.Fatalf("expected the replayed transcript to add events, got %d total: %+v", len(events), events)
	}
	if events[0].Type == EventUserQuestionRequired {
		t.Fatalf("sidecar question event still rendered first after transcript replay: %+v", events[0])
	}
	last := events[len(events)-1]
	if last.Type != EventUserQuestionRequired || last.QuestionID != "q-1" {
		t.Fatalf("sidecar question event was not moved to the end: last event = %+v", last)
	}
	for i, ev := range events {
		if ev.Seq != int64(i+1) {
			t.Fatalf("events[%d].Seq = %d, want %d (want a gap-free 1..N renumbering)", i, ev.Seq, i+1)
		}
	}
	if rs.sidecarPrefixCount != 0 {
		t.Fatalf("sidecarPrefixCount = %d, want 0 after it has been consumed", rs.sidecarPrefixCount)
	}
}
