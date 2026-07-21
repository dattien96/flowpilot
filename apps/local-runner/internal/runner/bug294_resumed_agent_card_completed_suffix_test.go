package runner

// BUG-294: on restart, the parent's resumed AgentTimelineCard for a child that
// was still running mid-turn (killed by the restart) rendered
// "coder · Claude · cancelled · claude-sonnet — completed" — the "— completed"
// suffix comes from an EventAgentResultInjected annotation that
// resumedParentAgentAnnotations emitted purely because LastMessage was non-empty,
// without checking the child ever reached a terminal completed state.
//
// The LIVE path only emits EventAgentResultInjected for
// completion.status == RunStatusCompleted (interactive_service.go). The fix
// mirrors that condition on the resume path. These additive tests pin it.

import (
	"context"
	"path/filepath"
	"testing"
)

func TestBug294ResumedAnnotationsSkipResultForNonCompletedChild(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	const (
		parentRunID  = "run-294-parent"
		doneChild    = "run-294-coder-done"
		runningChild = "run-294-coder-running"
	)
	sessions := []ProviderSessionState{
		{RunID: parentRunID, ProviderKey: ProviderKeyClaude, ProviderSessionID: "thread-parent", RunKind: "workflow", Status: RunStatusCompleted},
		{
			RunID: doneChild, ParentRunID: parentRunID, AgentName: "coder", Role: "coder", ProviderKey: ProviderKeyClaude,
			ProviderSessionID: "thread-done", RunKind: "chat", Status: RunStatusCompleted,
			StartedAt: "2026-07-20T03:40:01Z", UpdatedAt: "2026-07-20T03:40:05Z", LastMessage: "Implemented the fix; TestAdd passes.",
		},
		{
			// Killed mid-turn by a server restart: persisted status is still running
			// (normalized to cancelled on resume) but LastMessage is non-empty.
			RunID: runningChild, ParentRunID: parentRunID, AgentName: "coder", Role: "coder", ProviderKey: ProviderKeyClaude,
			ProviderSessionID: "thread-running", RunKind: "chat", Status: RunStatusRunning,
			StartedAt: "2026-07-20T03:41:01Z", UpdatedAt: "2026-07-20T03:41:42Z",
			LastMessage: "The write is still blocked pending your approval in the permission prompt for calc.go.",
		},
	}
	for _, session := range sessions {
		if err := store.UpsertProviderSession(context.Background(), session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	annotations := svc.resumedParentAgentAnnotations(parentRunID)

	spawns := map[string]bool{}
	results := map[string]bool{}
	for _, ev := range annotations {
		switch ev.Type {
		case EventAgentSpawnedByUser:
			spawns[ev.ChildRunID] = true
		case EventAgentResultInjected:
			results[ev.ChildRunID] = true
		}
	}

	// Both children keep a spawn annotation regardless of completion.
	if !spawns[doneChild] || !spawns[runningChild] {
		t.Fatalf("both children must have a spawn annotation, got %+v", spawns)
	}
	// Non-regression: a genuinely completed child still gets the result annotation
	// (its card correctly shows "— completed").
	if !results[doneChild] {
		t.Fatalf("completed child must keep its result annotation, got results=%+v", results)
	}
	// Regression guard: a child that never completed must NOT be annotated as
	// having delivered a result, or its card reads "cancelled … — completed".
	if results[runningChild] {
		t.Fatalf("non-completed child must NOT get a result annotation (BUG-294), got results=%+v", results)
	}
}

// TestBug294ResumedAnnotationsMatchLiveCompletedCondition documents the parity:
// the resume path annotates exactly the children a completed LIVE spawn would,
// i.e. status == RunStatusCompleted with a non-empty final message.
func TestBug294ResumedAnnotationsMatchLiveCompletedCondition(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	const parentRunID = "run-294b-parent"
	// A completed child with an EMPTY final message must also be skipped: the live
	// path guards on result.FinalMessage != "" too.
	sessions := []ProviderSessionState{
		{RunID: parentRunID, ProviderKey: ProviderKeyClaude, ProviderSessionID: "thread-parent", RunKind: "workflow", Status: RunStatusCompleted},
		{
			RunID: "run-294b-empty", ParentRunID: parentRunID, AgentName: "coder", ProviderKey: ProviderKeyClaude,
			ProviderSessionID: "thread-empty", RunKind: "chat", Status: RunStatusCompleted,
			StartedAt: "2026-07-20T03:40:01Z", UpdatedAt: "2026-07-20T03:40:05Z", LastMessage: "",
		},
	}
	for _, session := range sessions {
		if err := store.UpsertProviderSession(context.Background(), session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	for _, ev := range svc.resumedParentAgentAnnotations(parentRunID) {
		if ev.Type == EventAgentResultInjected {
			t.Fatalf("completed child with empty final message must not be annotated, got %+v", ev)
		}
	}
}
