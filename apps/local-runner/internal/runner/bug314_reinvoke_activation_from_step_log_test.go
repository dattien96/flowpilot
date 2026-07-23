package runner

// BUG-314: a reinvoke-lifecycle flow node's restored agent-card count came
// from turn-log prompt counting capped by a peer-start-time "wave" heuristic
// (resumeChildActivationCount + the wave cap in resumedParentAgentAnnotations).
// That heuristic depends on OTHER children's start times to detect a second
// Review Loop round — a flow with only one reviewer (no cohort) has no other
// child to form a wave, so round 2's card silently disappeared after a server
// restart even though the child genuinely ran twice. Confirmed live: Claude
// hub run-53157 (my-coder ×2 activations, my-reviewer-claude ×2 activations)
// restored with exactly one card each.
//
// Fix: resumedParentAgentAnnotations now prefers the durable per-node
// step-transition sidecar (Task-239) — each RUNNING transition for a node is
// a real activation, ground truth that isn't inflated by a mid-round gate
// reprompt (an extra turn-log prompt line) and doesn't need another child's
// timing to detect. Only applied when a label has exactly one claimant
// (labelCounts) so spawn-lifecycle nodes that get a fresh run id every round
// (Codex's reviewer_correctness/reviewer_security) — already correct — are
// left on the legacy path.
//
// additive-tests-only: new file only, no existing test touched.
// cross-provider-parity: TestBug314ReinvokeCardsSurviveRestartForEveryProvider
// runs the identical single-reviewer shape across codex/claude/grok.

import (
	"context"
	"path/filepath"
	"testing"
)

// TestBug314SingleReviewerReinvokeRestoresBothRoundCards is the exact live
// repro shape (run-53157): a Review Loop flow with ONE reviewer (no cohort),
// so peerStartWaveTimes has nothing to form a wave from. Both my-coder and
// my-reviewer-claude are reinvoke-lifecycle (one run id, two activations
// each) — a gate reprompt lands as coder's middle turn-log prompt, inflating
// the heuristic count without representing a real third activation.
func TestBug314SingleReviewerReinvokeRestoresBothRoundCards(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	const parentRunID = "run-314-parent"
	const coderRunID = "run-314-coder"
	const reviewerRunID = "run-314-reviewer"

	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: parentRunID, ProviderKey: ProviderKeyClaude, ProviderSessionID: "thread-parent",
		RunKind: "workflow", Status: RunStatusCompleted,
		StartedAt: "2026-07-23T02:51:42Z", UpdatedAt: "2026-07-23T02:58:59Z",
	}); err != nil {
		t.Fatalf("parent session: %v", err)
	}
	for _, child := range []ProviderSessionState{
		{
			RunID: coderRunID, ParentRunID: parentRunID, AgentName: "coder-agent", Label: "my-coder", Role: "coder-agent",
			ProviderKey: ProviderKeyClaude, ProviderSessionID: "thread-coder", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T02:51:44Z", UpdatedAt: "2026-07-23T02:55:28Z",
			LastMessage: "round 2 fix", TurnCount: 3,
		},
		{
			RunID: reviewerRunID, ParentRunID: parentRunID, AgentName: "reviewer", Label: "my-reviewer-claude", Role: "reviewer",
			ProviderKey: ProviderKeyClaude, ProviderSessionID: "thread-reviewer", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T02:53:31Z", UpdatedAt: "2026-07-23T02:55:44Z",
			LastMessage: "round 2 approve", TurnCount: 2,
		},
	} {
		if err := store.UpsertProviderSession(context.Background(), child); err != nil {
			t.Fatalf("child %s: %v", child.RunID, err)
		}
	}
	// Real-shape turn log: the coder's middle prompt is a gate reprompt inside
	// round 1 ("missing change-audit note"), not a genuine third activation —
	// this is exactly what over-counts the legacy turn-log heuristic to 3.
	for _, line := range []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "turn-1", Prompt: "fix bug 1+1 != 2"},
		{Kind: turnLogKindPrompt, TurnID: "turn-2", Prompt: "The flow gate is asking you to add a required document before this step can complete"},
		{Kind: turnLogKindPrompt, TurnID: "turn-3", Prompt: "[flow-engine] Feedback received on your last submission"},
	} {
		if err := store.AppendTurnLog(context.Background(), coderRunID, line); err != nil {
			t.Fatalf("coder turn log: %v", err)
		}
	}
	for _, line := range []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "turn-a", Prompt: "[flow-engine] Review this result from node \"my-coder\""},
		{Kind: turnLogKindPrompt, TurnID: "turn-b", Prompt: "[flow-engine] Review this result from node \"my-coder\""},
	} {
		if err := store.AppendTurnLog(context.Background(), reviewerRunID, line); err != nil {
			t.Fatalf("reviewer turn log: %v", err)
		}
	}
	// Durable step-transition sidecar (Task-239) — the ground truth this fix
	// reads. Mirrors the live run-53157-step-transitions.ndjson shape exactly.
	for _, line := range []stepTransitionLine{
		{RunID: parentRunID, NodeID: "my-coder", Status: "RUNNING", TS: "2026-07-23T02:51:44Z"},
		{RunID: parentRunID, NodeID: "my-coder", Status: "DONE", TS: "2026-07-23T02:53:30Z"},
		{RunID: parentRunID, NodeID: "my-reviewer-claude", Status: "RUNNING", TS: "2026-07-23T02:53:31Z"},
		{RunID: parentRunID, NodeID: "my-reviewer-claude", Status: "DONE", TS: "2026-07-23T02:54:46Z"},
		{RunID: parentRunID, NodeID: "my-coder", Status: "RUNNING", TS: "2026-07-23T02:55:12Z"},
		{RunID: parentRunID, NodeID: "my-coder", Status: "DONE", TS: "2026-07-23T02:55:28Z"},
		{RunID: parentRunID, NodeID: "my-reviewer-claude", Status: "RUNNING", TS: "2026-07-23T02:55:29Z"},
		{RunID: parentRunID, NodeID: "my-reviewer-claude", Status: "DONE", TS: "2026-07-23T02:55:44Z"},
	} {
		if err := store.AppendStepTransition(context.Background(), parentRunID, line); err != nil {
			t.Fatalf("step transition: %v", err)
		}
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id: parentRunID, providerKey: ProviderKeyClaude, providerSessionID: "thread-parent",
		createdAt: "2026-07-23T02:51:42Z", flowEngineDriven: true,
		events: []ProviderEvent{
			{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug 1+1 != 2", OccurredAt: "2026-07-23T02:51:42Z"},
			{Seq: 2, Type: EventMessageCompleted, Text: "Round 2 verdict: approved", OccurredAt: "2026-07-23T02:58:59Z"},
		},
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.appendResumedParentAnnotations(rs)

	coderSpawns, reviewerSpawns := countSpawnsByChild(rs.events, coderRunID), countSpawnsByChild(rs.events, reviewerRunID)
	if coderSpawns != 2 {
		t.Fatalf("coder round-2 card lost on restart: spawns = %d, want 2: %v", coderSpawns, summarizeEventTypes(rs.events))
	}
	if reviewerSpawns != 2 {
		t.Fatalf("reviewer round-2 card lost on restart: spawns = %d, want 2: %v", reviewerSpawns, summarizeEventTypes(rs.events))
	}
}

// TestBug314ReinvokeCardsSurviveRestartForEveryProvider is the cross-provider
// parity proof: the identical single-reviewer reinvoke shape (one hub, one
// coder + one reviewer, each reused across two rounds with its own
// step-transition RUNNING/DONE pairs) restores two cards per child on every
// supported provider — the bug was never provider-specific, it was triggered
// by "no cohort to form a wave from", which any provider's flow can hit.
func TestBug314ReinvokeCardsSurviveRestartForEveryProvider(t *testing.T) {
	for _, provider := range []ProviderKey{ProviderKeyCodex, ProviderKeyClaude, ProviderKeyGrok} {
		t.Run(string(provider), func(t *testing.T) {
			store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
			if err != nil {
				t.Fatalf("NewLocalFileSessionStore: %v", err)
			}
			parentRunID := "run-314-" + string(provider) + "-parent"
			coderRunID := "run-314-" + string(provider) + "-coder"
			reviewerRunID := "run-314-" + string(provider) + "-reviewer"

			if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
				RunID: parentRunID, ProviderKey: provider, ProviderSessionID: "thread-parent",
				RunKind: "workflow", Status: RunStatusCompleted,
				StartedAt: "2026-07-23T02:51:42Z", UpdatedAt: "2026-07-23T02:58:59Z",
			}); err != nil {
				t.Fatalf("parent session: %v", err)
			}
			for _, child := range []ProviderSessionState{
				{
					RunID: coderRunID, ParentRunID: parentRunID, AgentName: "coder-agent", Label: "my-coder", Role: "coder-agent",
					ProviderKey: provider, ProviderSessionID: "thread-coder", RunKind: "chat",
					Status: RunStatusCompleted, StartedAt: "2026-07-23T02:51:44Z", UpdatedAt: "2026-07-23T02:55:28Z",
					LastMessage: "round 2 fix", TurnCount: 3,
				},
				{
					RunID: reviewerRunID, ParentRunID: parentRunID, AgentName: "reviewer", Label: "my-reviewer", Role: "reviewer",
					ProviderKey: provider, ProviderSessionID: "thread-reviewer", RunKind: "chat",
					Status: RunStatusCompleted, StartedAt: "2026-07-23T02:53:31Z", UpdatedAt: "2026-07-23T02:55:44Z",
					LastMessage: "round 2 approve", TurnCount: 2,
				},
			} {
				if err := store.UpsertProviderSession(context.Background(), child); err != nil {
					t.Fatalf("child %s: %v", child.RunID, err)
				}
			}
			for _, line := range []turnLogLine{
				{Kind: turnLogKindPrompt, TurnID: "turn-1", Prompt: "fix bug 1+1 != 2"},
				{Kind: turnLogKindPrompt, TurnID: "turn-2", Prompt: "gate reprompt: missing change-audit note"},
				{Kind: turnLogKindPrompt, TurnID: "turn-3", Prompt: "[flow-engine] feedback received"},
			} {
				if err := store.AppendTurnLog(context.Background(), coderRunID, line); err != nil {
					t.Fatalf("coder turn log: %v", err)
				}
			}
			for _, line := range []turnLogLine{
				{Kind: turnLogKindPrompt, TurnID: "turn-a", Prompt: "[flow-engine] review this result"},
				{Kind: turnLogKindPrompt, TurnID: "turn-b", Prompt: "[flow-engine] review this result"},
			} {
				if err := store.AppendTurnLog(context.Background(), reviewerRunID, line); err != nil {
					t.Fatalf("reviewer turn log: %v", err)
				}
			}
			for _, line := range []stepTransitionLine{
				{RunID: parentRunID, NodeID: "my-coder", Status: "RUNNING", TS: "2026-07-23T02:51:44Z"},
				{RunID: parentRunID, NodeID: "my-coder", Status: "DONE", TS: "2026-07-23T02:53:30Z"},
				{RunID: parentRunID, NodeID: "my-reviewer", Status: "RUNNING", TS: "2026-07-23T02:53:31Z"},
				{RunID: parentRunID, NodeID: "my-reviewer", Status: "DONE", TS: "2026-07-23T02:54:46Z"},
				{RunID: parentRunID, NodeID: "my-coder", Status: "RUNNING", TS: "2026-07-23T02:55:12Z"},
				{RunID: parentRunID, NodeID: "my-coder", Status: "DONE", TS: "2026-07-23T02:55:28Z"},
				{RunID: parentRunID, NodeID: "my-reviewer", Status: "RUNNING", TS: "2026-07-23T02:55:29Z"},
				{RunID: parentRunID, NodeID: "my-reviewer", Status: "DONE", TS: "2026-07-23T02:55:44Z"},
			} {
				if err := store.AppendStepTransition(context.Background(), parentRunID, line); err != nil {
					t.Fatalf("step transition: %v", err)
				}
			}

			svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
			rs := &interactiveRun{
				id: parentRunID, providerKey: provider, providerSessionID: "thread-parent",
				createdAt: "2026-07-23T02:51:42Z", flowEngineDriven: true,
				events: []ProviderEvent{
					{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug 1+1 != 2", OccurredAt: "2026-07-23T02:51:42Z"},
					{Seq: 2, Type: EventMessageCompleted, Text: "Round 2 verdict: approved", OccurredAt: "2026-07-23T02:58:59Z"},
				},
				subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
			}
			svc.appendResumedParentAnnotations(rs)

			coderSpawns, reviewerSpawns := countSpawnsByChild(rs.events, coderRunID), countSpawnsByChild(rs.events, reviewerRunID)
			if coderSpawns != 2 {
				t.Fatalf("provider=%s coder round-2 card lost: spawns = %d, want 2: %v", provider, coderSpawns, summarizeEventTypes(rs.events))
			}
			if reviewerSpawns != 2 {
				t.Fatalf("provider=%s reviewer round-2 card lost: spawns = %d, want 2: %v", provider, reviewerSpawns, summarizeEventTypes(rs.events))
			}
		})
	}
}

// TestBug314SpawnLifecycleMultiRoundReviewersStayOneCardPerChild locks in the
// labelCounts guard: a spawn-lifecycle node gets a FRESH run id every round
// (Codex's reviewer_correctness/reviewer_security, already correct live per
// user report), so its label is claimed by several distinct children in the
// same hub. The step-transition log still has multiple RUNNING/DONE pairs
// under that shared label — if the fix attributed all of them to any one
// claimant, a 3-round flow would wrongly triple the card count for each
// round's reviewer instead of one card per child. Must stay on the legacy
// (already-correct) single-activation path.
func TestBug314SpawnLifecycleMultiRoundReviewersStayOneCardPerChild(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	const parentRunID = "run-314-spawn-parent"
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: parentRunID, ProviderKey: ProviderKeyCodex, ProviderSessionID: "thread-parent",
		RunKind: "workflow", Status: RunStatusCompleted,
		StartedAt: "2026-07-23T02:20:28Z", UpdatedAt: "2026-07-23T02:37:36Z",
	}); err != nil {
		t.Fatalf("parent session: %v", err)
	}
	type childSpec struct {
		runID, label, started, updated string
	}
	children := []childSpec{
		{"run-314-r0a", "reviewer_correctness", "2026-07-23T02:25:40Z", "2026-07-23T02:28:14Z"},
		{"run-314-r0b", "reviewer_security", "2026-07-23T02:25:41Z", "2026-07-23T02:28:14Z"},
		{"run-314-r1a", "reviewer_correctness", "2026-07-23T02:30:50Z", "2026-07-23T02:32:41Z"},
		{"run-314-r1b", "reviewer_security", "2026-07-23T02:30:51Z", "2026-07-23T02:32:45Z"},
		{"run-314-r2a", "reviewer_correctness", "2026-07-23T02:34:25Z", "2026-07-23T02:36:44Z"},
		{"run-314-r2b", "reviewer_security", "2026-07-23T02:34:26Z", "2026-07-23T02:36:44Z"},
	}
	for _, c := range children {
		if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
			RunID: c.runID, ParentRunID: parentRunID, AgentName: "reviewer", Label: c.label, Role: "reviewer",
			ProviderKey: ProviderKeyCodex, ProviderSessionID: "sess-" + c.runID, RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: c.started, UpdatedAt: c.updated,
			LastMessage: "request changes", TurnCount: 1,
		}); err != nil {
			t.Fatalf("child %s: %v", c.runID, err)
		}
	}
	// Same label claimed 3 times across rounds — step-transitions has 3
	// RUNNING/DONE pairs per label, matching live run-46797 exactly.
	for _, line := range []stepTransitionLine{
		{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "RUNNING", TS: "2026-07-23T02:25:40Z"},
		{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "DONE", TS: "2026-07-23T02:28:14Z"},
		{RunID: parentRunID, NodeID: "reviewer_security", Status: "RUNNING", TS: "2026-07-23T02:25:41Z"},
		{RunID: parentRunID, NodeID: "reviewer_security", Status: "DONE", TS: "2026-07-23T02:28:14Z"},
		{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "RUNNING", TS: "2026-07-23T02:30:50Z"},
		{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "DONE", TS: "2026-07-23T02:32:41Z"},
		{RunID: parentRunID, NodeID: "reviewer_security", Status: "RUNNING", TS: "2026-07-23T02:30:51Z"},
		{RunID: parentRunID, NodeID: "reviewer_security", Status: "DONE", TS: "2026-07-23T02:32:45Z"},
		{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "RUNNING", TS: "2026-07-23T02:34:25Z"},
		{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "DONE", TS: "2026-07-23T02:36:44Z"},
		{RunID: parentRunID, NodeID: "reviewer_security", Status: "RUNNING", TS: "2026-07-23T02:34:26Z"},
		{RunID: parentRunID, NodeID: "reviewer_security", Status: "DONE", TS: "2026-07-23T02:36:44Z"},
	} {
		if err := store.AppendStepTransition(context.Background(), parentRunID, line); err != nil {
			t.Fatalf("step transition: %v", err)
		}
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id: parentRunID, providerKey: ProviderKeyCodex, providerSessionID: "thread-parent",
		createdAt: "2026-07-23T02:20:28Z", flowEngineDriven: true,
		events: []ProviderEvent{
			{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug 1+1 != 2", OccurredAt: "2026-07-23T02:20:28Z"},
		},
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.appendResumedParentAnnotations(rs)

	for _, c := range children {
		if got := countSpawnsByChild(rs.events, c.runID); got != 1 {
			t.Fatalf("child %s (label %s) spawns = %d, want exactly 1 (guard against attributing every round's RUNNING/DONE pair to one claimant): %v",
				c.runID, c.label, got, summarizeEventTypes(rs.events))
		}
	}
}

func countSpawnsByChild(events []ProviderEvent, childRunID string) int {
	n := 0
	for _, ev := range events {
		if ev.Type == EventAgentSpawnedByUser && ev.ChildRunID == childRunID {
			n++
		}
	}
	return n
}
