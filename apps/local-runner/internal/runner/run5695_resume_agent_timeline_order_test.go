package runner

// Additive regression for run-5695 (CP-51 A1 live desktop):
// After server restart, multi-child flow restore must keep agent lifecycle
// interleaved as spawn→result per child (matching live SSE order), not
// cluster every agent_spawned_by_user before every agent_result_injected.

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun5695RestoreInterleavesAgentSpawnAndResultPerChild(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	const parentRunID = "run-5695-parent"
	// Two parallel reviewers (distinct child runs) + one coder — mirrors
	// Review Loop round-0 after both reviewers complete.
	children := []struct {
		runID, name, started, updated, msg string
	}{
		{"run-5700", "coder", "2026-07-20T11:03:26Z", "2026-07-20T11:04:46Z", "implemented round 0"},
		{"run-6246", "reviewer", "2026-07-20T11:04:48Z", "2026-07-20T11:05:42Z", "request changes A"},
		{"run-6254", "reviewer-agent", "2026-07-20T11:04:49Z", "2026-07-20T11:05:43Z", "request changes B"},
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: parentRunID, ProviderKey: ProviderKeyGrok, ProviderSessionID: "thread-parent",
		RunKind: "workflow", Status: RunStatusCompleted,
		StartedAt: "2026-07-20T11:03:25Z", UpdatedAt: "2026-07-20T11:05:44Z",
	}); err != nil {
		t.Fatalf("parent session: %v", err)
	}
	for _, child := range children {
		if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
			RunID: child.runID, ParentRunID: parentRunID, AgentName: child.name, Role: child.name,
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "thread-" + child.runID, RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: child.started, UpdatedAt: child.updated, LastMessage: child.msg,
		}); err != nil {
			t.Fatalf("child session %s: %v", child.runID, err)
		}
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id: parentRunID, providerKey: ProviderKeyGrok, providerSessionID: "thread-parent",
		createdAt: "2026-07-20T11:03:25Z", flowEngineDriven: true,
		events: []ProviderEvent{
			{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug 1+1 != 2", OccurredAt: "2026-07-20T11:03:25Z"},
			{Seq: 2, Type: EventMessageCompleted, Text: "Both reviewers requested changes", OccurredAt: "2026-07-20T11:05:44Z"},
		},
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}

	svc.appendResumedParentAnnotations(rs)

	// Live-equivalent chronology for this fixture:
	//   coder spawn→result, then concurrent reviewers spawn×2 → result×2, then synth.
	// Forbidden (the run-5695 restart bug): spawn×N … result×N with synthesis in between
	// or every result dumped after synthesis.
	wantTypes := []ProviderEventType{
		EventTurnStarted,
		EventAgentSpawnedByUser, EventAgentResultInjected, // coder (serial)
		EventAgentSpawnedByUser, EventAgentSpawnedByUser, // reviewer cohort start
		EventAgentResultInjected, EventAgentResultInjected, // reviewer cohort join
		EventMessageCompleted,
	}
	if len(rs.events) != len(wantTypes) {
		t.Fatalf("restored events = %d, want %d: %+v", len(rs.events), len(wantTypes), summarizeEventTypes(rs.events))
	}
	for i, want := range wantTypes {
		if rs.events[i].Type != want {
			t.Fatalf("events[%d].Type = %s, want %s; full=%v", i, rs.events[i].Type, want, summarizeEventTypes(rs.events))
		}
	}
	// Every result's spawn must appear earlier for the same ChildRunID.
	for i, ev := range rs.events {
		if ev.Type != EventAgentResultInjected {
			continue
		}
		foundSpawn := false
		for j := 0; j < i; j++ {
			if rs.events[j].Type == EventAgentSpawnedByUser && rs.events[j].ChildRunID == ev.ChildRunID {
				foundSpawn = true
				break
			}
		}
		if !foundSpawn {
			t.Fatalf("result for %s at %d has no earlier spawn: %v", ev.ChildRunID, i, summarizeEventTypes(rs.events))
		}
	}
	// Synthesis must not sit between the first child spawn and the last child result
	// (that was the "agents then res" dump with synth in the middle).
	firstSpawn, lastResult, synthIdx := -1, -1, -1
	for i, ev := range rs.events {
		switch ev.Type {
		case EventAgentSpawnedByUser:
			if firstSpawn < 0 {
				firstSpawn = i
			}
		case EventAgentResultInjected:
			lastResult = i
		case EventMessageCompleted:
			synthIdx = i
		}
	}
	if synthIdx >= 0 && firstSpawn >= 0 && lastResult >= 0 && synthIdx > firstSpawn && synthIdx < lastResult {
		t.Fatalf("synthesis interleaved inside agent lifecycle block: %v", summarizeEventTypes(rs.events))
	}
	if last := rs.events[len(rs.events)-1]; last.Type != EventMessageCompleted {
		t.Fatalf("last event = %s, want synthesis message_completed", last.Type)
	}
}

// TestRun9034RestorePlacesRoundClustersAroundHubMessages is the desktop
// expectation from run-9034: coder + reviewers R0, then hub response, then
// reinvoke coder + reviewers R1 — never coder+review×4 stacked above one synth.
func TestRun9034RestorePlacesRoundClustersAroundHubMessages(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	const parentRunID = "run-9034-order-parent"
	children := []struct {
		runID, name, started, updated, msg string
		turns                              int
	}{
		// lifecycle:reinvoke — same run id, two turns → two main-chat cards
		{"run-9039", "coder", "2026-07-20T11:22:02Z", "2026-07-20T11:25:20Z", "round1 remediate", 2},
		{"run-9620", "reviewer", "2026-07-20T11:23:03Z", "2026-07-20T11:24:17Z", "round0 A", 1},
		{"run-9628", "reviewer-agent", "2026-07-20T11:23:04Z", "2026-07-20T11:23:54Z", "round0 B", 1},
		// ~2min gap (hub synthesis + continue) starts cluster 1
		{"run-12508", "reviewer", "2026-07-20T11:25:21Z", "2026-07-20T11:26:17Z", "round1 A", 1},
		{"run-12516", "reviewer-agent", "2026-07-20T11:25:22Z", "2026-07-20T11:26:08Z", "round1 B", 1},
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: parentRunID, ProviderKey: ProviderKeyGrok, ProviderSessionID: "thread-parent",
		RunKind: "workflow", Status: RunStatusCompleted,
		StartedAt: "2026-07-20T11:22:00Z", UpdatedAt: "2026-07-20T11:26:36Z",
	}); err != nil {
		t.Fatalf("parent session: %v", err)
	}
	for _, child := range children {
		if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
			RunID: child.runID, ParentRunID: parentRunID, AgentName: child.name, Role: child.name,
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "thread-" + child.runID, RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: child.started, UpdatedAt: child.updated,
			LastMessage: child.msg, TurnCount: child.turns,
		}); err != nil {
			t.Fatalf("child session %s: %v", child.runID, err)
		}
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id: parentRunID, providerKey: ProviderKeyGrok, providerSessionID: "thread-parent",
		createdAt: "2026-07-20T11:22:00Z", flowEngineDriven: true,
		// No OccurredAt on messages → forces cluster placement (not pure time-sort),
		// matching Grok frames that lack timestamps and used to dump every agent first.
		events: []ProviderEvent{
			{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug 1+1 != 2", OccurredAt: "2026-07-20T11:22:00Z"},
			{Seq: 2, Type: EventMessageCompleted, Text: "Round 0: changes requested"},
			{Seq: 3, Type: EventMessageCompleted, Text: "Round 1: APPROVE"},
		},
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}

	svc.appendResumedParentAnnotations(rs)

	// Expect structure:
	// prompt | (coder#1 + R0 reviewers) | synth0 | (coder#2 + R1 reviewers) | synth1
	var phase int // 0=before synth0, 1=between, 2=after synth1
	var sawR0, sawR1, sawSynth0, sawSynth1 bool
	coderSpawns := 0
	for _, ev := range rs.events {
		switch {
		case ev.Type == EventMessageCompleted && strings.Contains(ev.Text, "Round 0"):
			if !sawR0 {
				t.Fatalf("synth0 before any round-0 agent: %v", summarizeEventTypes(rs.events))
			}
			if sawR1 {
				t.Fatalf("round-1 agent before synth0: %v", summarizeEventTypes(rs.events))
			}
			if coderSpawns < 1 {
				t.Fatalf("expected at least one coder card before synth0: %v", summarizeEventTypes(rs.events))
			}
			sawSynth0 = true
			phase = 1
		case ev.Type == EventMessageCompleted && strings.Contains(ev.Text, "Round 1"):
			if !sawSynth0 || !sawR1 {
				t.Fatalf("synth1 without synth0+R1 agents first: %v", summarizeEventTypes(rs.events))
			}
			if coderSpawns < 2 {
				t.Fatalf("expected reinvoke second coder card before synth1, got %d: %v", coderSpawns, summarizeEventTypes(rs.events))
			}
			sawSynth1 = true
			phase = 2
		case ev.Type == EventAgentSpawnedByUser && ev.ChildRunID == "run-9039":
			coderSpawns++
			// First coder in phase 0; reinvoke coder in phase 1 (between synths).
			if coderSpawns == 1 && phase != 0 {
				t.Fatalf("first coder spawn in phase %d: %v", phase, summarizeEventTypes(rs.events))
			}
			if coderSpawns == 2 && phase != 1 {
				t.Fatalf("reinvoke coder spawn in phase %d (want between synths): %v", phase, summarizeEventTypes(rs.events))
			}
		case ev.Type == EventAgentSpawnedByUser && (ev.ChildRunID == "run-9620" || ev.ChildRunID == "run-9628"):
			if phase != 0 {
				t.Fatalf("round-0 agent %s in phase %d: %v", ev.ChildRunID, phase, summarizeEventTypes(rs.events))
			}
			sawR0 = true
		case ev.Type == EventAgentSpawnedByUser && (ev.ChildRunID == "run-12508" || ev.ChildRunID == "run-12516"):
			if phase != 1 {
				t.Fatalf("round-1 agent %s in phase %d (want between synths): %v", ev.ChildRunID, phase, summarizeEventTypes(rs.events))
			}
			sawR1 = true
		}
	}
	if !sawR0 || !sawR1 || !sawSynth0 || !sawSynth1 || coderSpawns != 2 {
		t.Fatalf("incomplete structure sawR0=%v sawR1=%v sawSynth0=%v sawSynth1=%v coderSpawns=%d full=%v",
			sawR0, sawR1, sawSynth0, sawSynth1, coderSpawns, summarizeEventTypes(rs.events))
	}
}

func TestMergeTurnLogAssistantsIntoTranscriptAddsMissingHubSynthesis(t *testing.T) {
	historical := []ProviderEvent{
		{Type: EventTurnStarted, Prompt: "fix bug"},
		{Type: EventMessageCompleted, Text: "Round 0: changes requested"},
	}
	entries := []turnLogLine{
		{Kind: turnLogKindTranscriptTurn, TurnID: "turn-1", Assistant: "Round 0: changes requested"},
		{Kind: turnLogKindTranscriptTurn, TurnID: "turn-2", Assistant: "Round 1: APPROVE — both reviewers agreed"},
	}
	got := mergeTurnLogAssistantsIntoTranscript(historical, entries)
	var messages []string
	for _, ev := range got {
		if ev.Type == EventMessageCompleted {
			messages = append(messages, ev.Text)
		}
	}
	if len(messages) != 2 {
		t.Fatalf("messages = %v, want both synthesis turns", messages)
	}
	if messages[1] != "Round 1: APPROVE — both reviewers agreed" {
		t.Fatalf("second message = %q", messages[1])
	}
}

// Single restored hub message + two agent rounds must still place the response
// *after* both review cohorts (not between them).
func TestRun9034SingleHubMessageLandsAfterAllAgentClusters(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	const parentRunID = "run-9034-single-msg"
	children := []struct {
		runID, name, started, updated, msg string
		turns                              int
	}{
		{"run-c1", "coder", "2026-07-20T11:22:02Z", "2026-07-20T11:25:20Z", "done", 2},
		{"run-r0a", "reviewer", "2026-07-20T11:23:03Z", "2026-07-20T11:24:17Z", "r0a", 1},
		{"run-r0b", "reviewer-agent", "2026-07-20T11:23:04Z", "2026-07-20T11:23:54Z", "r0b", 1},
		{"run-r1a", "reviewer", "2026-07-20T11:25:21Z", "2026-07-20T11:26:17Z", "r1a", 1},
		{"run-r1b", "reviewer-agent", "2026-07-20T11:25:22Z", "2026-07-20T11:26:08Z", "r1b", 1},
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: parentRunID, ProviderKey: ProviderKeyGrok, ProviderSessionID: "thread-parent",
		RunKind: "workflow", Status: RunStatusCompleted,
		StartedAt: "2026-07-20T11:22:00Z", UpdatedAt: "2026-07-20T11:26:36Z",
	}); err != nil {
		t.Fatalf("parent: %v", err)
	}
	for _, child := range children {
		if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
			RunID: child.runID, ParentRunID: parentRunID, AgentName: child.name, Role: child.name,
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "thread-" + child.runID, RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: child.started, UpdatedAt: child.updated,
			LastMessage: child.msg, TurnCount: child.turns,
		}); err != nil {
			t.Fatalf("child %s: %v", child.runID, err)
		}
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id: parentRunID, providerKey: ProviderKeyGrok, providerSessionID: "thread-parent",
		createdAt: "2026-07-20T11:22:00Z", flowEngineDriven: true,
		events: []ProviderEvent{
			{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug", OccurredAt: "2026-07-20T11:22:00Z"},
			// Only final synthesis restored (the run-9034 Grok resume failure mode).
			{Seq: 2, Type: EventMessageCompleted, Text: "Verdict: APPROVE"},
		},
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.appendResumedParentAnnotations(rs)

	synthIdx := -1
	lastAgentIdx := -1
	for i, ev := range rs.events {
		if ev.Type == EventMessageCompleted && strings.Contains(ev.Text, "APPROVE") {
			synthIdx = i
		}
		if ev.Type == EventAgentSpawnedByUser {
			lastAgentIdx = i
		}
	}
	if synthIdx < 0 || lastAgentIdx < 0 {
		t.Fatalf("missing synth or agents: %v", summarizeEventTypes(rs.events))
	}
	if synthIdx < lastAgentIdx {
		t.Fatalf("final synthesis at %d before last agent at %d: %v", synthIdx, lastAgentIdx, summarizeEventTypes(rs.events))
	}
}

func TestRun5695RestoreMultiRoundReviewersKeepChronologicalSpawnResultPairs(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	const parentRunID = "run-5695-multiround"
	// Round 0 reviewers + round 1 reviewers (new child runs each round).
	children := []struct {
		runID, name, started, updated, msg string
	}{
		{"run-6246", "reviewer", "2026-07-20T11:04:48Z", "2026-07-20T11:05:42Z", "round0 A"},
		{"run-6254", "reviewer-agent", "2026-07-20T11:04:49Z", "2026-07-20T11:05:43Z", "round0 B"},
		{"run-8875", "reviewer", "2026-07-20T11:07:03Z", "2026-07-20T11:07:19Z", "round1 A"},
		{"run-8883", "reviewer-agent", "2026-07-20T11:07:04Z", "2026-07-20T11:07:23Z", "round1 B"},
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: parentRunID, ProviderKey: ProviderKeyGrok, ProviderSessionID: "thread-parent",
		RunKind: "workflow", Status: RunStatusCompleted,
		StartedAt: "2026-07-20T11:03:25Z", UpdatedAt: "2026-07-20T11:07:24Z",
	}); err != nil {
		t.Fatalf("parent session: %v", err)
	}
	for _, child := range children {
		if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
			RunID: child.runID, ParentRunID: parentRunID, AgentName: child.name, Role: child.name,
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "thread-" + child.runID, RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: child.started, UpdatedAt: child.updated, LastMessage: child.msg,
		}); err != nil {
			t.Fatalf("child session %s: %v", child.runID, err)
		}
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id: parentRunID, providerKey: ProviderKeyGrok, providerSessionID: "thread-parent",
		createdAt: "2026-07-20T11:03:25Z", flowEngineDriven: true,
		events: []ProviderEvent{
			{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug 1+1 != 2", OccurredAt: "2026-07-20T11:03:25Z"},
			// Hub synthesis between rounds — must not force every spawn before both results.
			{Seq: 2, Type: EventMessageCompleted, Text: "Round 0 synthesis", OccurredAt: "2026-07-20T11:05:44Z"},
			{Seq: 3, Type: EventMessageCompleted, Text: "Round 1 synthesis", OccurredAt: "2026-07-20T11:07:24Z"},
		},
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}

	svc.appendResumedParentAnnotations(rs)

	// With durable timestamps, round-0 pair must complete before round-0 synth,
	// and round-1 pair before round-1 synth — not all four spawns then all four results.
	var sawRound0Result, sawRound0Synth, sawRound1Spawn bool
	for _, ev := range rs.events {
		switch {
		case ev.Type == EventAgentResultInjected && (ev.ChildRunID == "run-6246" || ev.ChildRunID == "run-6254"):
			sawRound0Result = true
			if sawRound0Synth {
				t.Fatalf("round-0 result %s appeared after round-0 synthesis: %v", ev.ChildRunID, summarizeEventTypes(rs.events))
			}
		case ev.Type == EventMessageCompleted && ev.Text == "Round 0 synthesis":
			if !sawRound0Result {
				t.Fatalf("round-0 synthesis before any round-0 result: %v", summarizeEventTypes(rs.events))
			}
			sawRound0Synth = true
		case ev.Type == EventAgentSpawnedByUser && (ev.ChildRunID == "run-8875" || ev.ChildRunID == "run-8883"):
			sawRound1Spawn = true
			if !sawRound0Synth {
				t.Fatalf("round-1 spawn %s before round-0 synthesis: %v", ev.ChildRunID, summarizeEventTypes(rs.events))
			}
		}
	}
	if !sawRound1Spawn {
		t.Fatalf("missing round-1 spawns: %v", summarizeEventTypes(rs.events))
	}

	// Local pair integrity: no two consecutive spawns without a result between
	// them for *different* completed children when both have results — allow
	// concurrent cohort (spawn, spawn, result, result) but forbid global
	// spawn×N … result×N dump across the whole timeline.
	spawnStreak, resultStreak, maxSpawnStreak, maxResultStreak := 0, 0, 0, 0
	for _, ev := range rs.events {
		switch ev.Type {
		case EventAgentSpawnedByUser:
			spawnStreak++
			resultStreak = 0
			if spawnStreak > maxSpawnStreak {
				maxSpawnStreak = spawnStreak
			}
		case EventAgentResultInjected:
			resultStreak++
			spawnStreak = 0
			if resultStreak > maxResultStreak {
				maxResultStreak = resultStreak
			}
		default:
			spawnStreak, resultStreak = 0, 0
		}
	}
	// Cohort size is 2, so streaks of 2 are OK; a streak of 4 means the old dump.
	if maxSpawnStreak >= 4 || maxResultStreak >= 4 {
		t.Fatalf("global spawn/result dump detected (maxSpawnStreak=%d maxResultStreak=%d): %v",
			maxSpawnStreak, maxResultStreak, summarizeEventTypes(rs.events))
	}
}

func summarizeEventTypes(events []ProviderEvent) []string {
	out := make([]string, 0, len(events))
	for _, ev := range events {
		label := string(ev.Type)
		if ev.ChildRunID != "" {
			label = fmt.Sprintf("%s:%s", ev.Type, ev.ChildRunID)
		}
		if ev.Type == EventMessageCompleted && ev.Text != "" {
			label = fmt.Sprintf("%s(%q)", ev.Type, truncateForTest(ev.Text, 24))
		}
		out = append(out, label)
	}
	return out
}

func truncateForTest(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
