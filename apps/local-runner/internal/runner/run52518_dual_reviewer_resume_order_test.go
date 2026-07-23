package runner

// run-52518 regression: after BUG-314, reinvoke-lifecycle coder activation
// *times* came from the step-transition wall-clock log. On a dual-reviewer
// Review Loop with a short inter-round gap (< 45s), those exact stamps collapse
// R1+R2 into one cluster under clusterFlowAgentPairsByStartGap, so after a
// server restart every agent card lands before the first hub synthesis.
//
// Pre-BUG-314 (and the fix): step log still owns *count*, but placement times
// use resumeActivationTimestamps wave parking whenever peer waves can place
// every activation (multi-reviewer). Single-reviewer BUG-314 keeps log times
// when len(waves) < activations.
//
// additive-tests-only: new file only — never edit bug314_* or other resume-order
// tests. Exercises the mixed empty-OccurredAt hub path that seedFlowHubTranscriptFromTurnLog
// produces (BUG-314 tests seed timestamped hub events and miss this).

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestRun52518DualReviewerReinvokeResumeOrderPreservesRounds is the live
// run-52518 shape: Grok hub, reinvoke coder (one run, two activations), spawn-
// lifecycle dual reviewers each round (fresh run ids), R1→R2 gap under 45s,
// two hub synthesis frames with empty OccurredAt.
func TestRun52518DualReviewerReinvokeResumeOrderPreservesRounds(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	const parentRunID = "run-52518-order-parent"
	const coderRunID = "run-52518-coder"
	const revCR1 = "run-52518-rev-c-r1"
	const revSR1 = "run-52518-rev-s-r1"
	const revCR2 = "run-52518-rev-c-r2"
	const revSR2 = "run-52518-rev-s-r2"

	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: parentRunID, ProviderKey: ProviderKeyGrok, ProviderSessionID: "thread-parent",
		RunKind: "workflow", Status: RunStatusCompleted,
		StartedAt: "2026-07-23T14:19:12.017056Z", UpdatedAt: "2026-07-23T14:23:52.96938Z",
	}); err != nil {
		t.Fatalf("parent session: %v", err)
	}
	for _, child := range []ProviderSessionState{
		{
			RunID: coderRunID, ParentRunID: parentRunID, AgentName: "coder", Label: "coder", Role: "coder",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-coder", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T14:19:13.985607Z", UpdatedAt: "2026-07-23T14:22:37.705921Z",
			LastMessage: "round 2 fix", TurnCount: 2,
		},
		{
			RunID: revCR1, ParentRunID: parentRunID, AgentName: "reviewer", Label: "reviewer_correctness", Role: "reviewer",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-c1", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T14:20:17.725425Z", UpdatedAt: "2026-07-23T14:21:39.299308Z",
			LastMessage: "changes_requested r1", TurnCount: 1,
		},
		{
			RunID: revSR1, ParentRunID: parentRunID, AgentName: "reviewer", Label: "reviewer_security", Role: "reviewer",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-s1", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T14:20:18.076429Z", UpdatedAt: "2026-07-23T14:21:23.974992Z",
			LastMessage: "changes_requested r1", TurnCount: 1,
		},
		{
			RunID: revCR2, ParentRunID: parentRunID, AgentName: "reviewer", Label: "reviewer_correctness", Role: "reviewer",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-c2", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T14:22:37.954189Z", UpdatedAt: "2026-07-23T14:23:37.317614Z",
			LastMessage: "approved r2", TurnCount: 1,
		},
		{
			RunID: revSR2, ParentRunID: parentRunID, AgentName: "reviewer", Label: "reviewer_security", Role: "reviewer",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-s2", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T14:22:38.389772Z", UpdatedAt: "2026-07-23T14:23:38.200187Z",
			LastMessage: "approved r2", TurnCount: 1,
		},
	} {
		if err := store.UpsertProviderSession(context.Background(), child); err != nil {
			t.Fatalf("child %s: %v", child.RunID, err)
		}
	}
	// Live run-52518 spacing: R1 synthesis RUNNING 14:21:39 → R2 coder RUNNING 14:22:05 ≈ 26s < 45s.
	for _, line := range []stepTransitionLine{
		{RunID: parentRunID, NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T14:19:14.075076Z"},
		{RunID: parentRunID, NodeID: "coder", Status: "DONE", TS: "2026-07-23T14:20:17.001671Z"},
		{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "RUNNING", TS: "2026-07-23T14:20:17.802401Z"},
		{RunID: parentRunID, NodeID: "reviewer_security", Status: "RUNNING", TS: "2026-07-23T14:20:18.150587Z"},
		{RunID: parentRunID, NodeID: "reviewer_security", Status: "DONE", TS: "2026-07-23T14:21:24.02095Z"},
		{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "DONE", TS: "2026-07-23T14:21:39.341438Z"},
		{RunID: parentRunID, NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T14:21:39.342395Z"},
		{RunID: parentRunID, NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T14:22:05.056061Z"},
		{RunID: parentRunID, NodeID: "coder", Status: "DONE", TS: "2026-07-23T14:22:37.764235Z"},
		{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "RUNNING", TS: "2026-07-23T14:22:38.035049Z"},
		{RunID: parentRunID, NodeID: "reviewer_security", Status: "RUNNING", TS: "2026-07-23T14:22:38.4633Z"},
		{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "DONE", TS: "2026-07-23T14:23:37.388864Z"},
		{RunID: parentRunID, NodeID: "reviewer_security", Status: "DONE", TS: "2026-07-23T14:23:38.250087Z"},
		{RunID: parentRunID, NodeID: "synthesis", Status: "DONE", TS: "2026-07-23T14:23:52.96938Z"},
	} {
		if err := store.AppendStepTransition(context.Background(), parentRunID, line); err != nil {
			t.Fatalf("step transition: %v", err)
		}
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	// Empty OccurredAt on hub frames mirrors seedFlowHubTranscriptFromTurnLog —
	// mixed stamps strip all times and preserve insertion Seq order.
	rs := &interactiveRun{
		id: parentRunID, providerKey: ProviderKeyGrok, providerSessionID: "thread-parent",
		createdAt: "2026-07-23T14:19:12.017056Z", flowEngineDriven: true,
		events: []ProviderEvent{
			{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug 1+1 != 2"},
			{Seq: 2, Type: EventMessageCompleted, Text: "R1 synthesis: changes_requested"},
			{Seq: 3, Type: EventMessageCompleted, Text: "R2 synthesis: approved"},
		},
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.appendResumedParentAnnotations(rs)

	if got := countSpawnsByChild(rs.events, coderRunID); got != 2 {
		t.Fatalf("coder spawns = %d, want 2: %v", got, summarizeEventTypes(rs.events))
	}
	for _, id := range []string{revCR1, revSR1, revCR2, revSR2} {
		if got := countSpawnsByChild(rs.events, id); got != 1 {
			t.Fatalf("child %s spawns = %d, want 1: %v", id, got, summarizeEventTypes(rs.events))
		}
	}

	skel := spawnMessageSkeleton(rs.events)
	want := []string{
		"prompt",
		"spawn:coder:" + coderRunID,
		"spawn:reviewer_correctness:" + revCR1,
		"spawn:reviewer_security:" + revSR1,
		"synth:R1 synthesis: changes_requested",
		"spawn:coder:" + coderRunID,
		"spawn:reviewer_correctness:" + revCR2,
		"spawn:reviewer_security:" + revSR2,
		"synth:R2 synthesis: approved",
	}
	if !stringSlicesEqual(skel, want) {
		t.Fatalf("spawn/message order wrong after restart:\n got: %v\nwant: %v\nall:  %v",
			skel, want, summarizeEventTypes(rs.events))
	}

	// Mixed-timestamp path: after strip+resequence, no partial stamps remain.
	for _, ev := range rs.events {
		if strings.TrimSpace(ev.OccurredAt) != "" {
			t.Fatalf("expected all OccurredAt stripped on mixed hub path, got %q on %s: %v",
				ev.OccurredAt, ev.Type, summarizeEventTypes(rs.events))
		}
	}
}

// TestRun52518SingleSynthesisMessageKeepsRoundOrder covers len(msgIdxs)==1:
// both clusters may land before the sole hub message, but R0 agents must still
// precede R1 agents (not one merged clump and not bottom-appended after synth).
func TestRun52518SingleSynthesisMessageKeepsRoundOrder(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	const parentRunID = "run-52518-single-synth-parent"
	const coderRunID = "run-52518-ss-coder"
	const revCR1 = "run-52518-ss-rev-c-r1"
	const revSR1 = "run-52518-ss-rev-s-r1"
	const revCR2 = "run-52518-ss-rev-c-r2"
	const revSR2 = "run-52518-ss-rev-s-r2"

	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: parentRunID, ProviderKey: ProviderKeyGrok, ProviderSessionID: "thread-parent",
		RunKind: "workflow", Status: RunStatusCompleted,
		StartedAt: "2026-07-23T14:19:12Z", UpdatedAt: "2026-07-23T14:23:52Z",
	}); err != nil {
		t.Fatalf("parent session: %v", err)
	}
	for _, child := range []ProviderSessionState{
		{
			RunID: coderRunID, ParentRunID: parentRunID, AgentName: "coder", Label: "coder", Role: "coder",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-coder", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T14:19:13.985607Z", UpdatedAt: "2026-07-23T14:22:37.705921Z",
			LastMessage: "round 2 fix", TurnCount: 2,
		},
		{
			RunID: revCR1, ParentRunID: parentRunID, AgentName: "reviewer", Label: "reviewer_correctness", Role: "reviewer",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-c1", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T14:20:17.725425Z", UpdatedAt: "2026-07-23T14:21:39.299308Z",
			LastMessage: "r1", TurnCount: 1,
		},
		{
			RunID: revSR1, ParentRunID: parentRunID, AgentName: "reviewer", Label: "reviewer_security", Role: "reviewer",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-s1", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T14:20:18.076429Z", UpdatedAt: "2026-07-23T14:21:23.974992Z",
			LastMessage: "r1", TurnCount: 1,
		},
		{
			RunID: revCR2, ParentRunID: parentRunID, AgentName: "reviewer", Label: "reviewer_correctness", Role: "reviewer",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-c2", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T14:22:37.954189Z", UpdatedAt: "2026-07-23T14:23:37.317614Z",
			LastMessage: "r2", TurnCount: 1,
		},
		{
			RunID: revSR2, ParentRunID: parentRunID, AgentName: "reviewer", Label: "reviewer_security", Role: "reviewer",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-s2", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T14:22:38.389772Z", UpdatedAt: "2026-07-23T14:23:38.200187Z",
			LastMessage: "r2", TurnCount: 1,
		},
	} {
		if err := store.UpsertProviderSession(context.Background(), child); err != nil {
			t.Fatalf("child %s: %v", child.RunID, err)
		}
	}
	for _, line := range []stepTransitionLine{
		{RunID: parentRunID, NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T14:19:14.075076Z"},
		{RunID: parentRunID, NodeID: "coder", Status: "DONE", TS: "2026-07-23T14:20:17.001671Z"},
		{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "RUNNING", TS: "2026-07-23T14:20:17.802401Z"},
		{RunID: parentRunID, NodeID: "reviewer_security", Status: "RUNNING", TS: "2026-07-23T14:20:18.150587Z"},
		{RunID: parentRunID, NodeID: "reviewer_security", Status: "DONE", TS: "2026-07-23T14:21:24.02095Z"},
		{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "DONE", TS: "2026-07-23T14:21:39.341438Z"},
		{RunID: parentRunID, NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T14:21:39.342Z"},
		{RunID: parentRunID, NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T14:22:05.056061Z"},
		{RunID: parentRunID, NodeID: "coder", Status: "DONE", TS: "2026-07-23T14:22:37.764235Z"},
		{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "RUNNING", TS: "2026-07-23T14:22:38.035049Z"},
		{RunID: parentRunID, NodeID: "reviewer_security", Status: "RUNNING", TS: "2026-07-23T14:22:38.4633Z"},
		{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "DONE", TS: "2026-07-23T14:23:37.388864Z"},
		{RunID: parentRunID, NodeID: "reviewer_security", Status: "DONE", TS: "2026-07-23T14:23:38.250087Z"},
		{RunID: parentRunID, NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T14:23:38.250Z"},
	} {
		if err := store.AppendStepTransition(context.Background(), parentRunID, line); err != nil {
			t.Fatalf("step transition: %v", err)
		}
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id: parentRunID, providerKey: ProviderKeyGrok, providerSessionID: "thread-parent",
		createdAt: "2026-07-23T14:19:12Z", flowEngineDriven: true,
		events: []ProviderEvent{
			{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug 1+1 != 2"},
			{Seq: 2, Type: EventMessageCompleted, Text: "final synthesis only"},
		},
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.appendResumedParentAnnotations(rs)

	skel := spawnMessageSkeleton(rs.events)
	want := []string{
		"prompt",
		"spawn:coder:" + coderRunID,
		"spawn:reviewer_correctness:" + revCR1,
		"spawn:reviewer_security:" + revSR1,
		"spawn:coder:" + coderRunID,
		"spawn:reviewer_correctness:" + revCR2,
		"spawn:reviewer_security:" + revSR2,
		"synth:final synthesis only",
	}
	if !stringSlicesEqual(skel, want) {
		t.Fatalf("single-synth round order wrong:\n got: %v\nwant: %v\nall:  %v",
			skel, want, summarizeEventTypes(rs.events))
	}
	// Sole synthesis must not appear mid-round-0 or before any agents.
	synthIdx := -1
	for i, s := range skel {
		if strings.HasPrefix(s, "synth:") {
			synthIdx = i
			break
		}
	}
	if synthIdx != len(skel)-1 {
		t.Fatalf("synthesis must be last in skeleton (not bottom-append of agents after it, not mid-clump): idx=%d skel=%v", synthIdx, skel)
	}
}

// TestRun58237FlowModeLongR1ReviewerResumeOrderPreservesRounds is the live
// flow-mode shape (run-58237): custom workflow nodes grok-coder / grok-review /
// my-reviewer, reinvoke coder, dual reviewers with a *long* R1 (my-reviewer
// finishes ~2.5m after start). The 90% inter-wave park lands only ~39s after
// R1's last result edge — under the 45s cluster gap — so both rounds merged
// before synth1 after restart even when step-log count used wave parking.
// Fix: park reinvoke act i just before peer wave i (~60s after R1 edge).
func TestRun58237FlowModeLongR1ReviewerResumeOrderPreservesRounds(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	const parentRunID = "run-58237-order-parent"
	const coderRunID = "run-58237-coder"
	const revGR1 = "run-58237-grok-review-r1"
	const revMR1 = "run-58237-my-reviewer-r1"
	const revGR2 = "run-58237-grok-review-r2"
	const revMR2 = "run-58237-my-reviewer-r2"

	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: parentRunID, ProviderKey: ProviderKeyGrok, ProviderSessionID: "thread-parent",
		RunKind: "workflow", Status: RunStatusCompleted,
		StartedAt: "2026-07-23T15:07:30.809716Z", UpdatedAt: "2026-07-23T15:14:05.978468Z",
		ChatFlowRef: "9ecadf22-0a0e-4963-a45e-22812f0f9700",
	}); err != nil {
		t.Fatalf("parent session: %v", err)
	}
	for _, child := range []ProviderSessionState{
		{
			RunID: coderRunID, ParentRunID: parentRunID, AgentName: "coder", Label: "grok-coder", Role: "coder",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-coder", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T15:07:32.832201Z", UpdatedAt: "2026-07-23T15:12:03.311796Z",
			LastMessage: "round 2 fix", TurnCount: 2,
		},
		{
			RunID: revGR1, ParentRunID: parentRunID, AgentName: "reviewer", Label: "grok-review", Role: "reviewer",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-gr1", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T15:08:26.231635Z", UpdatedAt: "2026-07-23T15:09:15.503341Z",
			LastMessage: "request changes r1", TurnCount: 1,
		},
		{
			RunID: revMR1, ParentRunID: parentRunID, AgentName: "reviewer-agent", Label: "my-reviewer", Role: "reviewer",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-mr1", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T15:08:26.780793Z", UpdatedAt: "2026-07-23T15:11:02.97943Z",
			LastMessage: "request changes r1", TurnCount: 1,
		},
		{
			RunID: revGR2, ParentRunID: parentRunID, AgentName: "reviewer", Label: "grok-review", Role: "reviewer",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-gr2", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T15:12:03.864917Z", UpdatedAt: "2026-07-23T15:12:46.547001Z",
			LastMessage: "approve r2", TurnCount: 1,
		},
		{
			RunID: revMR2, ParentRunID: parentRunID, AgentName: "reviewer-agent", Label: "my-reviewer", Role: "reviewer",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-mr2", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T15:12:04.451498Z", UpdatedAt: "2026-07-23T15:12:51.929039Z",
			LastMessage: "approve r2", TurnCount: 1,
		},
	} {
		if err := store.UpsertProviderSession(context.Background(), child); err != nil {
			t.Fatalf("child %s: %v", child.RunID, err)
		}
	}
	// Live run-58237 step log: long my-reviewer R1 (15:08:26→15:11:03), synthesis
	// at 15:11:03, R2 coder 15:11:22, R2 reviewers 15:12:03. R1 edge→R2 peer
	// wave ≈ 60s; 90% inter-wave park would only be ~39s after R1 edge.
	for _, line := range []stepTransitionLine{
		{RunID: parentRunID, NodeID: "grok-coder", Status: "RUNNING", TS: "2026-07-23T15:07:32.924793Z"},
		{RunID: parentRunID, NodeID: "grok-coder", Status: "DONE", TS: "2026-07-23T15:08:25.533842Z"},
		{RunID: parentRunID, NodeID: "grok-review", Status: "RUNNING", TS: "2026-07-23T15:08:26.30603Z"},
		{RunID: parentRunID, NodeID: "my-reviewer", Status: "RUNNING", TS: "2026-07-23T15:08:26.858897Z"},
		{RunID: parentRunID, NodeID: "grok-review", Status: "DONE", TS: "2026-07-23T15:09:15.559158Z"},
		{RunID: parentRunID, NodeID: "my-reviewer", Status: "DONE", TS: "2026-07-23T15:11:03.038937Z"},
		{RunID: parentRunID, NodeID: "grok-synthesis", Status: "RUNNING", TS: "2026-07-23T15:11:03.039698Z"},
		{RunID: parentRunID, NodeID: "grok-coder", Status: "RUNNING", TS: "2026-07-23T15:11:22.77927Z"},
		{RunID: parentRunID, NodeID: "grok-coder", Status: "DONE", TS: "2026-07-23T15:12:03.363882Z"},
		{RunID: parentRunID, NodeID: "grok-review", Status: "RUNNING", TS: "2026-07-23T15:12:03.954791Z"},
		{RunID: parentRunID, NodeID: "my-reviewer", Status: "RUNNING", TS: "2026-07-23T15:12:04.527306Z"},
		{RunID: parentRunID, NodeID: "grok-review", Status: "DONE", TS: "2026-07-23T15:12:46.621116Z"},
		{RunID: parentRunID, NodeID: "my-reviewer", Status: "DONE", TS: "2026-07-23T15:12:51.969527Z"},
		{RunID: parentRunID, NodeID: "grok-synthesis", Status: "RUNNING", TS: "2026-07-23T15:12:51.970185Z"},
		{RunID: parentRunID, NodeID: "grok-synthesis", Status: "DONE", TS: "2026-07-23T15:13:08.50777Z"},
	} {
		if err := store.AppendStepTransition(context.Background(), parentRunID, line); err != nil {
			t.Fatalf("step transition: %v", err)
		}
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id: parentRunID, providerKey: ProviderKeyGrok, providerSessionID: "thread-parent",
		createdAt: "2026-07-23T15:07:30.809716Z", flowEngineDriven: true,
		events: []ProviderEvent{
			{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug 1+1 != 2"},
			{Seq: 2, Type: EventMessageCompleted, Text: "Review outcome submitted: changes_requested"},
			{Seq: 3, Type: EventMessageCompleted, Text: "Consolidated review outcome submitted: approved"},
		},
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.appendResumedParentAnnotations(rs)

	if got := countSpawnsByChild(rs.events, coderRunID); got != 2 {
		t.Fatalf("coder spawns = %d, want 2: %v", got, summarizeEventTypes(rs.events))
	}
	for _, id := range []string{revGR1, revMR1, revGR2, revMR2} {
		if got := countSpawnsByChild(rs.events, id); got != 1 {
			t.Fatalf("child %s spawns = %d, want 1: %v", id, got, summarizeEventTypes(rs.events))
		}
	}

	skel := spawnMessageSkeleton(rs.events)
	want := []string{
		"prompt",
		"spawn:grok-coder:" + coderRunID,
		"spawn:grok-review:" + revGR1,
		"spawn:my-reviewer:" + revMR1,
		"synth:Review outcome submitted: changes_requested",
		"spawn:grok-coder:" + coderRunID,
		"spawn:grok-review:" + revGR2,
		"spawn:my-reviewer:" + revMR2,
		"synth:Consolidated review outcome submitted: approved",
	}
	if !stringSlicesEqual(skel, want) {
		t.Fatalf("run-58237 flow-mode order wrong after restart:\n got: %v\nwant: %v\nall:  %v",
			skel, want, summarizeEventTypes(rs.events))
	}
}

// TestRun52518OrderPreservedAcrossProviders locks shared-path parity: the same
// dual-reviewer short-gap shape restores R0→synth→R1 order on codex/claude/grok.
func TestRun52518OrderPreservedAcrossProviders(t *testing.T) {
	for _, provider := range []ProviderKey{ProviderKeyCodex, ProviderKeyClaude, ProviderKeyGrok} {
		t.Run(string(provider), func(t *testing.T) {
			store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
			if err != nil {
				t.Fatalf("NewLocalFileSessionStore: %v", err)
			}
			parentRunID := "run-52518-" + string(provider) + "-parent"
			coderRunID := "run-52518-" + string(provider) + "-coder"
			revCR1 := "run-52518-" + string(provider) + "-c1"
			revSR1 := "run-52518-" + string(provider) + "-s1"
			revCR2 := "run-52518-" + string(provider) + "-c2"
			revSR2 := "run-52518-" + string(provider) + "-s2"

			if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
				RunID: parentRunID, ProviderKey: provider, ProviderSessionID: "thread-parent",
				RunKind: "workflow", Status: RunStatusCompleted,
				StartedAt: "2026-07-23T14:19:12Z", UpdatedAt: "2026-07-23T14:23:52Z",
			}); err != nil {
				t.Fatalf("parent session: %v", err)
			}
			for _, child := range []ProviderSessionState{
				{
					RunID: coderRunID, ParentRunID: parentRunID, AgentName: "coder", Label: "coder", Role: "coder",
					ProviderKey: provider, ProviderSessionID: "sess-coder", RunKind: "chat",
					Status: RunStatusCompleted, StartedAt: "2026-07-23T14:19:13.985607Z", UpdatedAt: "2026-07-23T14:22:37.705921Z",
					LastMessage: "round 2 fix", TurnCount: 2,
				},
				{
					RunID: revCR1, ParentRunID: parentRunID, AgentName: "reviewer", Label: "reviewer_correctness", Role: "reviewer",
					ProviderKey: provider, ProviderSessionID: "sess-c1", RunKind: "chat",
					Status: RunStatusCompleted, StartedAt: "2026-07-23T14:20:17.725425Z", UpdatedAt: "2026-07-23T14:21:39.299308Z",
					LastMessage: "r1", TurnCount: 1,
				},
				{
					RunID: revSR1, ParentRunID: parentRunID, AgentName: "reviewer", Label: "reviewer_security", Role: "reviewer",
					ProviderKey: provider, ProviderSessionID: "sess-s1", RunKind: "chat",
					Status: RunStatusCompleted, StartedAt: "2026-07-23T14:20:18.076429Z", UpdatedAt: "2026-07-23T14:21:23.974992Z",
					LastMessage: "r1", TurnCount: 1,
				},
				{
					RunID: revCR2, ParentRunID: parentRunID, AgentName: "reviewer", Label: "reviewer_correctness", Role: "reviewer",
					ProviderKey: provider, ProviderSessionID: "sess-c2", RunKind: "chat",
					Status: RunStatusCompleted, StartedAt: "2026-07-23T14:22:37.954189Z", UpdatedAt: "2026-07-23T14:23:37.317614Z",
					LastMessage: "r2", TurnCount: 1,
				},
				{
					RunID: revSR2, ParentRunID: parentRunID, AgentName: "reviewer", Label: "reviewer_security", Role: "reviewer",
					ProviderKey: provider, ProviderSessionID: "sess-s2", RunKind: "chat",
					Status: RunStatusCompleted, StartedAt: "2026-07-23T14:22:38.389772Z", UpdatedAt: "2026-07-23T14:23:38.200187Z",
					LastMessage: "r2", TurnCount: 1,
				},
			} {
				if err := store.UpsertProviderSession(context.Background(), child); err != nil {
					t.Fatalf("child %s: %v", child.RunID, err)
				}
			}
			// synthesis RUNNING lines are durable round boundaries (Terra) —
			// without them every activation stays cohort 0 and cards clump.
			for _, line := range []stepTransitionLine{
				{RunID: parentRunID, NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T14:19:14.075076Z"},
				{RunID: parentRunID, NodeID: "coder", Status: "DONE", TS: "2026-07-23T14:20:17.001671Z"},
				{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "RUNNING", TS: "2026-07-23T14:20:17.802401Z"},
				{RunID: parentRunID, NodeID: "reviewer_security", Status: "RUNNING", TS: "2026-07-23T14:20:18.150587Z"},
				{RunID: parentRunID, NodeID: "reviewer_security", Status: "DONE", TS: "2026-07-23T14:21:24.02095Z"},
				{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "DONE", TS: "2026-07-23T14:21:39.341438Z"},
				{RunID: parentRunID, NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T14:21:39.342395Z"},
				{RunID: parentRunID, NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T14:22:05.056061Z"},
				{RunID: parentRunID, NodeID: "coder", Status: "DONE", TS: "2026-07-23T14:22:37.764235Z"},
				{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "RUNNING", TS: "2026-07-23T14:22:38.035049Z"},
				{RunID: parentRunID, NodeID: "reviewer_security", Status: "RUNNING", TS: "2026-07-23T14:22:38.4633Z"},
				{RunID: parentRunID, NodeID: "reviewer_correctness", Status: "DONE", TS: "2026-07-23T14:23:37.388864Z"},
				{RunID: parentRunID, NodeID: "reviewer_security", Status: "DONE", TS: "2026-07-23T14:23:38.250087Z"},
				{RunID: parentRunID, NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T14:23:38.250865Z"},
			} {
				if err := store.AppendStepTransition(context.Background(), parentRunID, line); err != nil {
					t.Fatalf("step transition: %v", err)
				}
			}

			svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
			rs := &interactiveRun{
				id: parentRunID, providerKey: provider, providerSessionID: "thread-parent",
				createdAt: "2026-07-23T14:19:12Z", flowEngineDriven: true,
				events: []ProviderEvent{
					{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug 1+1 != 2"},
					{Seq: 2, Type: EventMessageCompleted, Text: "R1 synthesis"},
					{Seq: 3, Type: EventMessageCompleted, Text: "R2 synthesis"},
				},
				subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
			}
			svc.appendResumedParentAnnotations(rs)

			skel := spawnMessageSkeleton(rs.events)
			want := []string{
				"prompt",
				"spawn:coder:" + coderRunID,
				"spawn:reviewer_correctness:" + revCR1,
				"spawn:reviewer_security:" + revSR1,
				"synth:R1 synthesis",
				"spawn:coder:" + coderRunID,
				"spawn:reviewer_correctness:" + revCR2,
				"spawn:reviewer_security:" + revSR2,
				"synth:R2 synthesis",
			}
			if !stringSlicesEqual(skel, want) {
				t.Fatalf("provider=%s order wrong:\n got: %v\nwant: %v", provider, skel, want)
			}
			if got := countSpawnsByChild(rs.events, coderRunID); got != 2 {
				t.Fatalf("provider=%s coder spawns = %d, want 2", provider, got)
			}
		})
	}
}

// TestDurableCohortOneSecondInterRoundGap proves order does NOT depend on the
// old 45s wall-clock cluster gap: R1→R2 is only 1s apart in the log timestamps,
// but synthesis RUNNING still splits cohorts correctly.
func TestDurableCohortOneSecondInterRoundGap(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	const parentRunID = "run-1s-gap-parent"
	const coderRunID = "run-1s-gap-coder"
	const revR1 = "run-1s-gap-rev-r1"
	const revR2 = "run-1s-gap-rev-r2"

	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: parentRunID, ProviderKey: ProviderKeyClaude, ProviderSessionID: "thread-parent",
		RunKind: "workflow", Status: RunStatusCompleted,
		StartedAt: "2026-07-23T10:00:00Z", UpdatedAt: "2026-07-23T10:00:10Z",
	}); err != nil {
		t.Fatalf("parent: %v", err)
	}
	for _, child := range []ProviderSessionState{
		{
			RunID: coderRunID, ParentRunID: parentRunID, Label: "coder", AgentName: "coder", Role: "coder",
			ProviderKey: ProviderKeyClaude, ProviderSessionID: "c", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T10:00:01Z", UpdatedAt: "2026-07-23T10:00:08Z",
			LastMessage: "done", TurnCount: 2,
		},
		{
			RunID: revR1, ParentRunID: parentRunID, Label: "my-reviewer", AgentName: "reviewer", Role: "reviewer",
			ProviderKey: ProviderKeyClaude, ProviderSessionID: "r1", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T10:00:02Z", UpdatedAt: "2026-07-23T10:00:03Z",
			LastMessage: "changes", TurnCount: 1,
		},
		{
			RunID: revR2, ParentRunID: parentRunID, Label: "my-reviewer", AgentName: "reviewer", Role: "reviewer",
			ProviderKey: ProviderKeyClaude, ProviderSessionID: "r2", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-23T10:00:05Z", UpdatedAt: "2026-07-23T10:00:06Z",
			LastMessage: "approve", TurnCount: 1,
		},
	} {
		if err := store.UpsertProviderSession(context.Background(), child); err != nil {
			t.Fatalf("child: %v", err)
		}
	}
	// 1-second inter-round wall clock — would merge under any 45s heuristic.
	for _, line := range []stepTransitionLine{
		{RunID: parentRunID, NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T10:00:01.000Z"},
		{RunID: parentRunID, NodeID: "coder", Status: "DONE", TS: "2026-07-23T10:00:01.500Z"},
		{RunID: parentRunID, NodeID: "my-reviewer", Status: "RUNNING", TS: "2026-07-23T10:00:02.000Z"},
		{RunID: parentRunID, NodeID: "my-reviewer", Status: "DONE", TS: "2026-07-23T10:00:03.000Z"},
		{RunID: parentRunID, NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T10:00:03.100Z"},
		{RunID: parentRunID, NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T10:00:04.000Z"},
		{RunID: parentRunID, NodeID: "coder", Status: "DONE", TS: "2026-07-23T10:00:04.500Z"},
		{RunID: parentRunID, NodeID: "my-reviewer", Status: "RUNNING", TS: "2026-07-23T10:00:05.000Z"},
		{RunID: parentRunID, NodeID: "my-reviewer", Status: "DONE", TS: "2026-07-23T10:00:06.000Z"},
		{RunID: parentRunID, NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T10:00:06.100Z"},
	} {
		if err := store.AppendStepTransition(context.Background(), parentRunID, line); err != nil {
			t.Fatalf("step: %v", err)
		}
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id: parentRunID, providerKey: ProviderKeyClaude, providerSessionID: "thread-parent",
		createdAt: "2026-07-23T10:00:00Z", flowEngineDriven: true,
		events: []ProviderEvent{
			{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug"},
			{Seq: 2, Type: EventMessageCompleted, Text: "synth0"},
			{Seq: 3, Type: EventMessageCompleted, Text: "synth1"},
		},
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.appendResumedParentAnnotations(rs)

	skel := spawnMessageSkeleton(rs.events)
	want := []string{
		"prompt",
		"spawn:coder:" + coderRunID,
		"spawn:my-reviewer:" + revR1,
		"synth:synth0",
		"spawn:coder:" + coderRunID,
		"spawn:my-reviewer:" + revR2,
		"synth:synth1",
	}
	if !stringSlicesEqual(skel, want) {
		t.Fatalf("1s-gap durable order wrong:\n got: %v\nwant: %v", skel, want)
	}
}

// TestDurableCohortSingleReviewerShortGapOrder is BUG-314 shape (one reviewer,
// reinvoke) with a short R0→R1 wall-clock gap: count stays 2 and order still
// splits on synthesis RUNNING (no peer waves required).
func TestDurableCohortSingleReviewerShortGapOrder(t *testing.T) {
	for _, provider := range []ProviderKey{ProviderKeyCodex, ProviderKeyClaude, ProviderKeyGrok} {
		t.Run(string(provider), func(t *testing.T) {
			store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
			if err != nil {
				t.Fatalf("store: %v", err)
			}
			parent := "run-sr-" + string(provider)
			coderID := parent + "-coder"
			revID := parent + "-rev"
			if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
				RunID: parent, ProviderKey: provider, ProviderSessionID: "p",
				RunKind: "workflow", Status: RunStatusCompleted,
				StartedAt: "2026-07-23T02:51:42Z", UpdatedAt: "2026-07-23T02:58:59Z",
			}); err != nil {
				t.Fatalf("parent: %v", err)
			}
			for _, child := range []ProviderSessionState{
				{
					RunID: coderID, ParentRunID: parent, Label: "my-coder", AgentName: "coder", Role: "coder",
					ProviderKey: provider, ProviderSessionID: "c", RunKind: "chat",
					Status: RunStatusCompleted, StartedAt: "2026-07-23T02:51:44Z", UpdatedAt: "2026-07-23T02:55:28Z",
					LastMessage: "r2", TurnCount: 3,
				},
				{
					RunID: revID, ParentRunID: parent, Label: "my-reviewer", AgentName: "reviewer", Role: "reviewer",
					ProviderKey: provider, ProviderSessionID: "r", RunKind: "chat",
					Status: RunStatusCompleted, StartedAt: "2026-07-23T02:53:31Z", UpdatedAt: "2026-07-23T02:55:44Z",
					LastMessage: "ok", TurnCount: 2,
				},
			} {
				if err := store.UpsertProviderSession(context.Background(), child); err != nil {
					t.Fatalf("child: %v", err)
				}
			}
			for _, line := range []stepTransitionLine{
				{RunID: parent, NodeID: "my-coder", Status: "RUNNING", TS: "2026-07-23T02:51:44Z"},
				{RunID: parent, NodeID: "my-coder", Status: "DONE", TS: "2026-07-23T02:53:30Z"},
				{RunID: parent, NodeID: "my-reviewer", Status: "RUNNING", TS: "2026-07-23T02:53:31Z"},
				{RunID: parent, NodeID: "my-reviewer", Status: "DONE", TS: "2026-07-23T02:54:46Z"},
				{RunID: parent, NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T02:54:47Z"},
				// Only 5s later — peer-wave heuristics would struggle; cohort must not.
				{RunID: parent, NodeID: "my-coder", Status: "RUNNING", TS: "2026-07-23T02:54:52Z"},
				{RunID: parent, NodeID: "my-coder", Status: "DONE", TS: "2026-07-23T02:55:28Z"},
				{RunID: parent, NodeID: "my-reviewer", Status: "RUNNING", TS: "2026-07-23T02:55:29Z"},
				{RunID: parent, NodeID: "my-reviewer", Status: "DONE", TS: "2026-07-23T02:55:44Z"},
				{RunID: parent, NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T02:55:45Z"},
			} {
				if err := store.AppendStepTransition(context.Background(), parent, line); err != nil {
					t.Fatalf("step: %v", err)
				}
			}
			svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
			rs := &interactiveRun{
				id: parent, providerKey: provider, providerSessionID: "p",
				createdAt: "2026-07-23T02:51:42Z", flowEngineDriven: true,
				events: []ProviderEvent{
					{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug 1+1 != 2"},
					{Seq: 2, Type: EventMessageCompleted, Text: "round1"},
					{Seq: 3, Type: EventMessageCompleted, Text: "round2"},
				},
				subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
			}
			svc.appendResumedParentAnnotations(rs)
			if got := countSpawnsByChild(rs.events, coderID); got != 2 {
				t.Fatalf("coder spawns=%d want 2: %v", got, summarizeEventTypes(rs.events))
			}
			if got := countSpawnsByChild(rs.events, revID); got != 2 {
				t.Fatalf("reviewer spawns=%d want 2: %v", got, summarizeEventTypes(rs.events))
			}
			skel := spawnMessageSkeleton(rs.events)
			want := []string{
				"prompt",
				"spawn:my-coder:" + coderID,
				"spawn:my-reviewer:" + revID,
				"synth:round1",
				"spawn:my-coder:" + coderID,
				"spawn:my-reviewer:" + revID,
				"synth:round2",
			}
			if !stringSlicesEqual(skel, want) {
				t.Fatalf("provider=%s single-reviewer short-gap order wrong:\n got: %v\nwant: %v", provider, skel, want)
			}
		})
	}
}

// spawnMessageSkeleton keeps only user prompt, agent spawns, and non-empty hub
// synthesis messages — the durable-replay order contract surface for this bug.
func spawnMessageSkeleton(events []ProviderEvent) []string {
	out := make([]string, 0, len(events))
	for _, ev := range events {
		switch ev.Type {
		case EventTurnStarted:
			if strings.TrimSpace(ev.Prompt) != "" && !isSystemPrompt(ev.Prompt) {
				out = append(out, "prompt")
			}
		case EventAgentSpawnedByUser:
			out = append(out, "spawn:"+ev.AgentName+":"+ev.ChildRunID)
		case EventMessageCompleted:
			if text := strings.TrimSpace(ev.Text); text != "" {
				out = append(out, "synth:"+text)
			}
		}
	}
	return out
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
