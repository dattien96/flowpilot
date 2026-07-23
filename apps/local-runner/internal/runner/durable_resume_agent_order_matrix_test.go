package runner

// Durable resume agent-card ORDER matrix (CA-412 Terra redesign).
//
// additive-tests-only: new file only. Complements run52518_* / bug314_* with
// edge situations that multi-round flow/chat restore must not regress:
// post-flow follow-up clamp, missing synthesis boundaries, more rounds than
// hub messages, grok-synthesis node id, no sidecar, within-cohort append
// order, three-round dual-reviewer, flow-mode × 3 providers, uncompleted child.
//
// durable-replay: Event order + Agent lifecycle (no bottom-append past follow-up).

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// --- pure unit: step log → cohort math ---

func TestStepActivationsFromOrderedLogAssignsCohortsBySynthesisRunning(t *testing.T) {
	lines := []stepTransitionLine{
		{NodeID: "coder", Status: "RUNNING", TS: "t1"},
		{NodeID: "coder", Status: "DONE", TS: "t2"},
		{NodeID: "reviewer", Status: "RUNNING", TS: "t3"},
		{NodeID: "reviewer", Status: "DONE", TS: "t4"},
		{NodeID: "synthesis", Status: "RUNNING", TS: "t5"},
		{NodeID: "coder", Status: "RUNNING", TS: "t6"},
		{NodeID: "coder", Status: "DONE", TS: "t7"},
		{NodeID: "grok-synthesis", Status: "RUNNING", TS: "t8"}, // second boundary (alias name)
		{NodeID: "coder", Status: "RUNNING", TS: "t9"},
	}
	acts := stepActivationsFromOrderedLog(lines)
	if len(acts) != 4 {
		t.Fatalf("activations = %d, want 4 (3 coder + 1 reviewer; synthesis skipped): %+v", len(acts), acts)
	}
	// cohort 0: coder, reviewer; after synthesis RUNNING → cohort 1: coder; after grok-synthesis → cohort 2
	want := []struct {
		node   string
		cohort int
		ord    int
	}{
		{"coder", 0, 0},
		{"reviewer", 0, 1},
		{"coder", 1, 2},
		{"coder", 2, 3},
	}
	for i, w := range want {
		if acts[i].nodeID != w.node || acts[i].cohort != w.cohort || acts[i].ord != w.ord {
			t.Fatalf("act[%d]=%+v, want node=%s cohort=%d ord=%d", i, acts[i], w.node, w.cohort, w.ord)
		}
	}
	// Open RUNNING without DONE still counted (process killed mid-turn).
	if acts[3].doneAt != "" {
		t.Fatalf("open activation should have empty doneAt, got %q", acts[3].doneAt)
	}
}

func TestIsSynthesisStepNodeAcceptsAliases(t *testing.T) {
	for _, id := range []string{"synthesis", "grok-synthesis", "hub-synthesis", "SYNTHESIS"} {
		if !isSynthesisStepNode(id) {
			t.Fatalf("isSynthesisStepNode(%q) = false, want true", id)
		}
	}
	for _, id := range []string{"coder", "my-reviewer", "reviewer_security", "context", ""} {
		if isSynthesisStepNode(id) {
			t.Fatalf("isSynthesisStepNode(%q) = true, want false", id)
		}
	}
}

// --- helpers for matrix fixtures ---

type durableOrderChild struct {
	runID, label, agent, role string
	started, updated          string
	turnCount                 int
	lastMsg                   string
	status                    RunStatus
}

func seedDurableOrderHub(t *testing.T, store *localFileSessionStore, parent string, provider ProviderKey, children []durableOrderChild, steps []stepTransitionLine) {
	t.Helper()
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: parent, ProviderKey: provider, ProviderSessionID: "thread-" + parent,
		RunKind: "workflow", Status: RunStatusCompleted,
		StartedAt: "2026-07-23T10:00:00Z", UpdatedAt: "2026-07-23T11:00:00Z",
	}); err != nil {
		t.Fatalf("parent: %v", err)
	}
	for _, c := range children {
		st := c.status
		if st == "" {
			st = RunStatusCompleted
		}
		tc := c.turnCount
		if tc < 1 {
			tc = 1
		}
		msg := c.lastMsg
		if msg == "" && st == RunStatusCompleted {
			msg = "done"
		}
		if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
			RunID: c.runID, ParentRunID: parent, Label: c.label, AgentName: c.agent, Role: c.role,
			ProviderKey: provider, ProviderSessionID: "sess-" + c.runID, RunKind: "chat",
			Status: st, StartedAt: c.started, UpdatedAt: c.updated,
			LastMessage: msg, TurnCount: tc,
		}); err != nil {
			t.Fatalf("child %s: %v", c.runID, err)
		}
	}
	for _, line := range steps {
		line.RunID = parent
		if err := store.AppendStepTransition(context.Background(), parent, line); err != nil {
			t.Fatalf("step: %v", err)
		}
	}
}

func resumeDurableOrder(t *testing.T, store *localFileSessionStore, parent string, provider ProviderKey, hub []ProviderEvent) *interactiveRun {
	t.Helper()
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id: parent, providerKey: provider, providerSessionID: "thread-" + parent,
		createdAt: "2026-07-23T10:00:00Z", flowEngineDriven: true,
		events: append([]ProviderEvent(nil), hub...),
		subs:   map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.appendResumedParentAnnotations(rs)
	return rs
}

// --- situational matrix ---

// TestDurableOrderAgentsStayBeforePostFlowFollowUp locks run-24377 class clamp:
// restored agent cards must not land between the first post-flow user follow-up
// and its answer ("done rồi hả" / "ok").
func TestDurableOrderAgentsStayBeforePostFlowFollowUp(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	const parent = "run-followup-order"
	const coder = "run-followup-coder"
	const rev = "run-followup-rev"
	seedDurableOrderHub(t, store, parent, ProviderKeyGrok,
		[]durableOrderChild{
			{runID: coder, label: "coder", agent: "coder", role: "coder", started: "2026-07-23T10:00:01Z", updated: "2026-07-23T10:00:10Z", turnCount: 2},
			{runID: rev, label: "reviewer", agent: "reviewer", role: "reviewer", started: "2026-07-23T10:00:02Z", updated: "2026-07-23T10:00:03Z", turnCount: 1},
			{runID: rev + "2", label: "reviewer", agent: "reviewer", role: "reviewer", started: "2026-07-23T10:00:06Z", updated: "2026-07-23T10:00:07Z", turnCount: 1},
		},
		[]stepTransitionLine{
			{NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T10:00:01Z"},
			{NodeID: "coder", Status: "DONE", TS: "2026-07-23T10:00:01.5Z"},
			{NodeID: "reviewer", Status: "RUNNING", TS: "2026-07-23T10:00:02Z"},
			{NodeID: "reviewer", Status: "DONE", TS: "2026-07-23T10:00:03Z"},
			{NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T10:00:03.1Z"},
			{NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T10:00:05Z"},
			{NodeID: "coder", Status: "DONE", TS: "2026-07-23T10:00:05.5Z"},
			{NodeID: "reviewer", Status: "RUNNING", TS: "2026-07-23T10:00:06Z"},
			{NodeID: "reviewer", Status: "DONE", TS: "2026-07-23T10:00:07Z"},
			{NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T10:00:07.1Z"},
		},
	)
	rs := resumeDurableOrder(t, store, parent, ProviderKeyGrok, []ProviderEvent{
		{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug"},
		{Seq: 2, Type: EventMessageCompleted, Text: "synth0"},
		{Seq: 3, Type: EventMessageCompleted, Text: "synth1"},
		// First post-flow follow-up + answer — agents must stay above this block.
		{Seq: 4, Type: EventTurnStarted, Prompt: "done rồi hả"},
		{Seq: 5, Type: EventMessageCompleted, Text: "ok"},
	})

	// Clamp: no agent spawn at/after first post-flow turn_started.
	firstFollow := -1
	userN := 0
	for i, ev := range rs.events {
		if ev.Type == EventTurnStarted && strings.TrimSpace(ev.Prompt) != "" && !isSystemPrompt(ev.Prompt) {
			userN++
			if userN == 2 {
				firstFollow = i
				break
			}
		}
	}
	if firstFollow < 0 {
		t.Fatalf("expected post-flow follow-up turn_started: %v", summarizeEventTypes(rs.events))
	}
	for i := firstFollow; i < len(rs.events); i++ {
		if rs.events[i].Type == EventAgentSpawnedByUser {
			t.Fatalf("agent card at/after post-flow follow-up (idx %d): %v", i, summarizeEventTypes(rs.events))
		}
	}
	// Still have both rounds before follow-up.
	if countSpawnsByChild(rs.events, coder) != 2 {
		t.Fatalf("coder spawns = %d, want 2: %v", countSpawnsByChild(rs.events, coder), summarizeEventTypes(rs.events))
	}
}

// TestDurableOrderMoreCohortsThanHubMessages parks leftover rounds before the
// last synthesis (not after it / not bottom-appended past terminal prose).
func TestDurableOrderMoreCohortsThanHubMessages(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	const parent = "run-3cohort-2msg"
	const coder = "run-3c-coder"
	children := []durableOrderChild{
		{runID: coder, label: "coder", agent: "coder", role: "coder", started: "2026-07-23T10:00:01Z", updated: "2026-07-23T10:00:30Z", turnCount: 3},
	}
	// three rounds of reinvoke coder; only two hub messages restored
	steps := []stepTransitionLine{
		{NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T10:00:01Z"},
		{NodeID: "coder", Status: "DONE", TS: "2026-07-23T10:00:02Z"},
		{NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T10:00:03Z"},
		{NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T10:00:04Z"},
		{NodeID: "coder", Status: "DONE", TS: "2026-07-23T10:00:05Z"},
		{NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T10:00:06Z"},
		{NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T10:00:07Z"},
		{NodeID: "coder", Status: "DONE", TS: "2026-07-23T10:00:08Z"},
		{NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T10:00:09Z"},
	}
	seedDurableOrderHub(t, store, parent, ProviderKeyCodex, children, steps)
	rs := resumeDurableOrder(t, store, parent, ProviderKeyCodex, []ProviderEvent{
		{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug"},
		{Seq: 2, Type: EventMessageCompleted, Text: "only-synth-0"},
		{Seq: 3, Type: EventMessageCompleted, Text: "only-synth-1"},
	})
	if got := countSpawnsByChild(rs.events, coder); got != 3 {
		t.Fatalf("coder spawns = %d, want 3: %v", got, summarizeEventTypes(rs.events))
	}
	skel := spawnMessageSkeleton(rs.events)
	// Expect: prompt, coder, synth0, coder, coder, synth1  (rounds 1+2 both before last synth)
	// or prompt, coder, synth0, coder, synth1 with third before last — never after last synth.
	lastSynth := -1
	for i, s := range skel {
		if strings.HasPrefix(s, "synth:") {
			lastSynth = i
		}
	}
	if lastSynth < 0 {
		t.Fatalf("no synth in skeleton: %v", skel)
	}
	for i := lastSynth + 1; i < len(skel); i++ {
		if strings.HasPrefix(skel[i], "spawn:") {
			t.Fatalf("agent card after last hub synth (bottom-append): %v", skel)
		}
	}
	// All three coder spawns present before last synth.
	coderBeforeLast := 0
	for i := 0; i <= lastSynth; i++ {
		if skel[i] == "spawn:coder:"+coder {
			coderBeforeLast++
		}
	}
	if coderBeforeLast != 3 {
		t.Fatalf("coder spawns before last synth = %d, want 3: %v", coderBeforeLast, skel)
	}
}

// TestDurableOrderMissingSynthesisRunningKeepsCardsButSingleCohort documents
// degraded behavior when the sidecar lacks synthesis RUNNING boundaries:
// every activation is cohort 0 → one cluster before first message (count preserved).
func TestDurableOrderMissingSynthesisRunningKeepsCardsButSingleCohort(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	const parent = "run-no-synth-boundary"
	const coder = "run-nsb-coder"
	const rev1 = "run-nsb-rev1"
	const rev2 = "run-nsb-rev2"
	seedDurableOrderHub(t, store, parent, ProviderKeyClaude,
		[]durableOrderChild{
			{runID: coder, label: "coder", agent: "coder", role: "coder", started: "2026-07-23T10:00:01Z", updated: "2026-07-23T10:00:10Z", turnCount: 2},
			{runID: rev1, label: "rev", agent: "reviewer", role: "reviewer", started: "2026-07-23T10:00:02Z", updated: "2026-07-23T10:00:03Z"},
			{runID: rev2, label: "rev", agent: "reviewer", role: "reviewer", started: "2026-07-23T10:00:06Z", updated: "2026-07-23T10:00:07Z"},
		},
		// No synthesis RUNNING at all.
		[]stepTransitionLine{
			{NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T10:00:01Z"},
			{NodeID: "coder", Status: "DONE", TS: "2026-07-23T10:00:01.5Z"},
			{NodeID: "rev", Status: "RUNNING", TS: "2026-07-23T10:00:02Z"},
			{NodeID: "rev", Status: "DONE", TS: "2026-07-23T10:00:03Z"},
			{NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T10:00:05Z"},
			{NodeID: "coder", Status: "DONE", TS: "2026-07-23T10:00:05.5Z"},
			{NodeID: "rev", Status: "RUNNING", TS: "2026-07-23T10:00:06Z"},
			{NodeID: "rev", Status: "DONE", TS: "2026-07-23T10:00:07Z"},
		},
	)
	rs := resumeDurableOrder(t, store, parent, ProviderKeyClaude, []ProviderEvent{
		{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug"},
		{Seq: 2, Type: EventMessageCompleted, Text: "synth0"},
		{Seq: 3, Type: EventMessageCompleted, Text: "synth1"},
	})
	if countSpawnsByChild(rs.events, coder) != 2 || countSpawnsByChild(rs.events, rev1) != 1 || countSpawnsByChild(rs.events, rev2) != 1 {
		t.Fatalf("counts wrong: %v", summarizeEventTypes(rs.events))
	}
	skel := spawnMessageSkeleton(rs.events)
	// Single cohort → all agents before first synth, then remaining synth.
	wantPrefix := []string{
		"prompt",
		"spawn:coder:" + coder,
		"spawn:rev:" + rev1,
		"spawn:coder:" + coder,
		"spawn:rev:" + rev2,
		"synth:synth0",
	}
	if len(skel) < len(wantPrefix) {
		t.Fatalf("skeleton short: %v", skel)
	}
	for i := range wantPrefix {
		if skel[i] != wantPrefix[i] {
			t.Fatalf("degraded single-cohort order wrong:\n got: %v\nwant prefix: %v", skel, wantPrefix)
		}
	}
}

// TestDurableOrderGrokSynthesisNodeNameSplitsCohorts ensures node_id
// "grok-synthesis" (flow-mode) is treated as a round boundary like "synthesis".
func TestDurableOrderGrokSynthesisNodeNameSplitsCohorts(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	const parent = "run-grok-synth-name"
	const coder = "run-gsn-coder"
	seedDurableOrderHub(t, store, parent, ProviderKeyGrok,
		[]durableOrderChild{
			{runID: coder, label: "grok-coder", agent: "coder", role: "coder", started: "2026-07-23T10:00:01Z", updated: "2026-07-23T10:00:10Z", turnCount: 2},
		},
		[]stepTransitionLine{
			{NodeID: "grok-coder", Status: "RUNNING", TS: "2026-07-23T10:00:01Z"},
			{NodeID: "grok-coder", Status: "DONE", TS: "2026-07-23T10:00:02Z"},
			{NodeID: "grok-synthesis", Status: "RUNNING", TS: "2026-07-23T10:00:03Z"},
			{NodeID: "grok-coder", Status: "RUNNING", TS: "2026-07-23T10:00:04Z"},
			{NodeID: "grok-coder", Status: "DONE", TS: "2026-07-23T10:00:05Z"},
			{NodeID: "grok-synthesis", Status: "RUNNING", TS: "2026-07-23T10:00:06Z"},
		},
	)
	rs := resumeDurableOrder(t, store, parent, ProviderKeyGrok, []ProviderEvent{
		{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug"},
		{Seq: 2, Type: EventMessageCompleted, Text: "s0"},
		{Seq: 3, Type: EventMessageCompleted, Text: "s1"},
	})
	want := []string{
		"prompt",
		"spawn:grok-coder:" + coder,
		"synth:s0",
		"spawn:grok-coder:" + coder,
		"synth:s1",
	}
	if got := spawnMessageSkeleton(rs.events); !stringSlicesEqual(got, want) {
		t.Fatalf("grok-synthesis boundary order wrong:\n got: %v\nwant: %v", got, want)
	}
}

// TestDurableOrderNoSidecarStillRestoresCardsWithoutPanic is the legacy path:
// no step-transition file → wave/gap fallback; at least one card per child.
func TestDurableOrderNoSidecarStillRestoresCardsWithoutPanic(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	const parent = "run-no-sidecar"
	const c1 = "run-ns-c1"
	const c2 = "run-ns-c2"
	// No AppendStepTransition calls.
	seedDurableOrderHub(t, store, parent, ProviderKeyCodex,
		[]durableOrderChild{
			{runID: c1, label: "coder", agent: "coder", role: "coder", started: "2026-07-23T10:00:01Z", updated: "2026-07-23T10:00:02Z"},
			{runID: c2, label: "reviewer", agent: "reviewer", role: "reviewer", started: "2026-07-23T10:00:03Z", updated: "2026-07-23T10:00:04Z"},
		},
		nil,
	)
	rs := resumeDurableOrder(t, store, parent, ProviderKeyCodex, []ProviderEvent{
		{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug"},
		{Seq: 2, Type: EventMessageCompleted, Text: "done"},
	})
	if countSpawnsByChild(rs.events, c1) < 1 || countSpawnsByChild(rs.events, c2) < 1 {
		t.Fatalf("expected at least one spawn per child without sidecar: %v", summarizeEventTypes(rs.events))
	}
}

// TestDurableOrderWithinCohortFollowsStepLogAppendOrder: concurrent reviewers
// restore in step-log RUNNING order (security before correctness if log says so).
func TestDurableOrderWithinCohortFollowsStepLogAppendOrder(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	const parent = "run-within-cohort-ord"
	const sec = "run-wco-sec"
	const cor = "run-wco-cor"
	// Child startedAt order is opposite of log RUNNING order — durable ord must win.
	seedDurableOrderHub(t, store, parent, ProviderKeyClaude,
		[]durableOrderChild{
			{runID: cor, label: "reviewer_correctness", agent: "reviewer", role: "reviewer",
				started: "2026-07-23T10:00:05Z", updated: "2026-07-23T10:00:06Z"}, // later start
			{runID: sec, label: "reviewer_security", agent: "reviewer", role: "reviewer",
				started: "2026-07-23T10:00:01Z", updated: "2026-07-23T10:00:02Z"}, // earlier start
		},
		[]stepTransitionLine{
			// Log order: correctness first, then security (opposite of startedAt sort of children for pairing —
			// spawn-lifecycle pairs by startedAt so sec gets first activation in log for reviewer_security only.
			// Use different labels so each has one activation; within cohort order is log ord.
			{NodeID: "reviewer_correctness", Status: "RUNNING", TS: "2026-07-23T10:00:01Z"},
			{NodeID: "reviewer_security", Status: "RUNNING", TS: "2026-07-23T10:00:01.1Z"},
			{NodeID: "reviewer_correctness", Status: "DONE", TS: "2026-07-23T10:00:02Z"},
			{NodeID: "reviewer_security", Status: "DONE", TS: "2026-07-23T10:00:02.1Z"},
			{NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T10:00:03Z"},
		},
	)
	rs := resumeDurableOrder(t, store, parent, ProviderKeyClaude, []ProviderEvent{
		{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug"},
		{Seq: 2, Type: EventMessageCompleted, Text: "synth"},
	})
	skel := spawnMessageSkeleton(rs.events)
	// correctness RUNNING is ord 0, security ord 1 → correctness spawn first in cohort.
	want := []string{
		"prompt",
		"spawn:reviewer_correctness:" + cor,
		"spawn:reviewer_security:" + sec,
		"synth:synth",
	}
	if !stringSlicesEqual(skel, want) {
		t.Fatalf("within-cohort log order wrong:\n got: %v\nwant: %v", skel, want)
	}
}

// TestDurableOrderThreeRoundDualReviewerAllProviders is the full multi-round
// dual-reviewer skeleton on codex/claude/grok (3 synth anchors).
func TestDurableOrderThreeRoundDualReviewerAllProviders(t *testing.T) {
	for _, provider := range []ProviderKey{ProviderKeyCodex, ProviderKeyClaude, ProviderKeyGrok} {
		t.Run(string(provider), func(t *testing.T) {
			store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
			if err != nil {
				t.Fatalf("store: %v", err)
			}
			parent := "run-3r-" + string(provider)
			coder := parent + "-coder"
			var children []durableOrderChild
			children = append(children, durableOrderChild{
				runID: coder, label: "coder", agent: "coder", role: "coder",
				started: "2026-07-23T10:00:01Z", updated: "2026-07-23T10:00:40Z", turnCount: 3,
			})
			starts := []string{"02", "03", "12", "13", "22", "23"}
			var revIDs []string
			for r := 0; r < 3; r++ {
				for li, lab := range []string{"reviewer_correctness", "reviewer_security"} {
					id := parent + "-" + lab + "-r" + []string{"0", "1", "2"}[r]
					revIDs = append(revIDs, id)
					start := "2026-07-23T10:00:" + starts[r*2+li] + "Z"
					children = append(children, durableOrderChild{
						runID: id, label: lab, agent: "reviewer", role: "reviewer",
						started: start, updated: start,
					})
				}
			}
			steps := []stepTransitionLine{
				{NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T10:00:01Z"},
				{NodeID: "coder", Status: "DONE", TS: "2026-07-23T10:00:01.5Z"},
				{NodeID: "reviewer_correctness", Status: "RUNNING", TS: "2026-07-23T10:00:02Z"},
				{NodeID: "reviewer_security", Status: "RUNNING", TS: "2026-07-23T10:00:03Z"},
				{NodeID: "reviewer_correctness", Status: "DONE", TS: "2026-07-23T10:00:04Z"},
				{NodeID: "reviewer_security", Status: "DONE", TS: "2026-07-23T10:00:04.1Z"},
				{NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T10:00:05Z"},
				{NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T10:00:11Z"},
				{NodeID: "coder", Status: "DONE", TS: "2026-07-23T10:00:11.5Z"},
				{NodeID: "reviewer_correctness", Status: "RUNNING", TS: "2026-07-23T10:00:12Z"},
				{NodeID: "reviewer_security", Status: "RUNNING", TS: "2026-07-23T10:00:13Z"},
				{NodeID: "reviewer_correctness", Status: "DONE", TS: "2026-07-23T10:00:14Z"},
				{NodeID: "reviewer_security", Status: "DONE", TS: "2026-07-23T10:00:14.1Z"},
				{NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T10:00:15Z"},
				{NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T10:00:21Z"},
				{NodeID: "coder", Status: "DONE", TS: "2026-07-23T10:00:21.5Z"},
				{NodeID: "reviewer_correctness", Status: "RUNNING", TS: "2026-07-23T10:00:22Z"},
				{NodeID: "reviewer_security", Status: "RUNNING", TS: "2026-07-23T10:00:23Z"},
				{NodeID: "reviewer_correctness", Status: "DONE", TS: "2026-07-23T10:00:24Z"},
				{NodeID: "reviewer_security", Status: "DONE", TS: "2026-07-23T10:00:24.1Z"},
				{NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T10:00:25Z"},
			}
			seedDurableOrderHub(t, store, parent, provider, children, steps)
			rs := resumeDurableOrder(t, store, parent, provider, []ProviderEvent{
				{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug"},
				{Seq: 2, Type: EventMessageCompleted, Text: "s0"},
				{Seq: 3, Type: EventMessageCompleted, Text: "s1"},
				{Seq: 4, Type: EventMessageCompleted, Text: "s2"},
			})
			if countSpawnsByChild(rs.events, coder) != 3 {
				t.Fatalf("coder spawns=%d want 3: %v", countSpawnsByChild(rs.events, coder), summarizeEventTypes(rs.events))
			}
			for _, id := range revIDs {
				if countSpawnsByChild(rs.events, id) != 1 {
					t.Fatalf("rev %s spawns=%d want 1", id, countSpawnsByChild(rs.events, id))
				}
			}
			skel := spawnMessageSkeleton(rs.events)
			// Structure: prompt, (coder+2rev), s0, (coder+2rev), s1, (coder+2rev), s2
			wantSynthAt := map[int]string{4: "synth:s0", 8: "synth:s1", 12: "synth:s2"}
			// indices: 0 prompt, 1-3 R0, 4 s0, 5-7 R1, 8 s1, 9-11 R2, 12 s2
			if len(skel) != 13 {
				t.Fatalf("provider=%s skeleton len=%d want 13: %v", provider, len(skel), skel)
			}
			if skel[0] != "prompt" {
				t.Fatalf("provider=%s want prompt first: %v", provider, skel)
			}
			for idx, label := range wantSynthAt {
				if skel[idx] != label {
					t.Fatalf("provider=%s skel[%d]=%q want %q full=%v", provider, idx, skel[idx], label, skel)
				}
			}
			// Each round block starts with coder spawn.
			for _, i := range []int{1, 5, 9} {
				if skel[i] != "spawn:coder:"+coder {
					t.Fatalf("provider=%s round block at %d not coder: %v", provider, i, skel)
				}
			}
		})
	}
}

// TestDurableOrderFlowModeShapeAllProviders is run-58237-like labels
// (grok-coder / grok-review / my-reviewer / grok-synthesis) on all providers.
func TestDurableOrderFlowModeShapeAllProviders(t *testing.T) {
	for _, provider := range []ProviderKey{ProviderKeyCodex, ProviderKeyClaude, ProviderKeyGrok} {
		t.Run(string(provider), func(t *testing.T) {
			store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
			if err != nil {
				t.Fatalf("store: %v", err)
			}
			parent := "run-flowshape-" + string(provider)
			coder := parent + "-coder"
			gr1, mr1 := parent+"-gr1", parent+"-mr1"
			gr2, mr2 := parent+"-gr2", parent+"-mr2"
			seedDurableOrderHub(t, store, parent, provider,
				[]durableOrderChild{
					{runID: coder, label: "grok-coder", agent: "coder", role: "coder",
						started: "2026-07-23T15:07:32Z", updated: "2026-07-23T15:12:03Z", turnCount: 2},
					{runID: gr1, label: "grok-review", agent: "reviewer", role: "reviewer",
						started: "2026-07-23T15:08:26Z", updated: "2026-07-23T15:09:15Z"},
					{runID: mr1, label: "my-reviewer", agent: "reviewer-agent", role: "reviewer",
						started: "2026-07-23T15:08:26.5Z", updated: "2026-07-23T15:11:02Z"},
					{runID: gr2, label: "grok-review", agent: "reviewer", role: "reviewer",
						started: "2026-07-23T15:12:03.8Z", updated: "2026-07-23T15:12:46Z"},
					{runID: mr2, label: "my-reviewer", agent: "reviewer-agent", role: "reviewer",
						started: "2026-07-23T15:12:04Z", updated: "2026-07-23T15:12:51Z"},
				},
				[]stepTransitionLine{
					{NodeID: "grok-coder", Status: "RUNNING", TS: "2026-07-23T15:07:32Z"},
					{NodeID: "grok-coder", Status: "DONE", TS: "2026-07-23T15:08:25Z"},
					{NodeID: "grok-review", Status: "RUNNING", TS: "2026-07-23T15:08:26Z"},
					{NodeID: "my-reviewer", Status: "RUNNING", TS: "2026-07-23T15:08:26.5Z"},
					{NodeID: "grok-review", Status: "DONE", TS: "2026-07-23T15:09:15Z"},
					{NodeID: "my-reviewer", Status: "DONE", TS: "2026-07-23T15:11:03Z"},
					{NodeID: "grok-synthesis", Status: "RUNNING", TS: "2026-07-23T15:11:03.1Z"},
					{NodeID: "grok-coder", Status: "RUNNING", TS: "2026-07-23T15:11:22Z"},
					{NodeID: "grok-coder", Status: "DONE", TS: "2026-07-23T15:12:03Z"},
					{NodeID: "grok-review", Status: "RUNNING", TS: "2026-07-23T15:12:03.8Z"},
					{NodeID: "my-reviewer", Status: "RUNNING", TS: "2026-07-23T15:12:04Z"},
					{NodeID: "grok-review", Status: "DONE", TS: "2026-07-23T15:12:46Z"},
					{NodeID: "my-reviewer", Status: "DONE", TS: "2026-07-23T15:12:51Z"},
					{NodeID: "grok-synthesis", Status: "RUNNING", TS: "2026-07-23T15:12:52Z"},
				},
			)
			rs := resumeDurableOrder(t, store, parent, provider, []ProviderEvent{
				{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug 1+1 != 2"},
				{Seq: 2, Type: EventMessageCompleted, Text: "changes_requested"},
				{Seq: 3, Type: EventMessageCompleted, Text: "approved"},
			})
			want := []string{
				"prompt",
				"spawn:grok-coder:" + coder,
				"spawn:grok-review:" + gr1,
				"spawn:my-reviewer:" + mr1,
				"synth:changes_requested",
				"spawn:grok-coder:" + coder,
				"spawn:grok-review:" + gr2,
				"spawn:my-reviewer:" + mr2,
				"synth:approved",
			}
			if got := spawnMessageSkeleton(rs.events); !stringSlicesEqual(got, want) {
				t.Fatalf("provider=%s flow-mode shape wrong:\n got: %v\nwant: %v", provider, got, want)
			}
		})
	}
}

// TestDurableOrderUncompletedChildEmitsSpawnWithoutResult keeps a running/cancelled
// child as a spawn-only card (no completed suffix event) in the right cohort.
func TestDurableOrderUncompletedChildEmitsSpawnWithoutResult(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	const parent = "run-uncompleted"
	const coder = "run-unc-coder"
	const rev = "run-unc-rev"
	seedDurableOrderHub(t, store, parent, ProviderKeyGrok,
		[]durableOrderChild{
			{runID: coder, label: "coder", agent: "coder", role: "coder",
				started: "2026-07-23T10:00:01Z", updated: "2026-07-23T10:00:02Z", status: RunStatusCompleted},
			{runID: rev, label: "reviewer", agent: "reviewer", role: "reviewer",
				started: "2026-07-23T10:00:02Z", updated: "2026-07-23T10:00:03Z",
				status: RunStatusCancelled, lastMsg: "killed mid-turn"},
		},
		[]stepTransitionLine{
			{NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T10:00:01Z"},
			{NodeID: "coder", Status: "DONE", TS: "2026-07-23T10:00:02Z"},
			{NodeID: "reviewer", Status: "RUNNING", TS: "2026-07-23T10:00:02.1Z"},
			// no DONE — still open activation
			{NodeID: "synthesis", Status: "RUNNING", TS: "2026-07-23T10:00:05Z"},
		},
	)
	rs := resumeDurableOrder(t, store, parent, ProviderKeyGrok, []ProviderEvent{
		{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug"},
		{Seq: 2, Type: EventMessageCompleted, Text: "synth"},
	})
	if countSpawnsByChild(rs.events, coder) != 1 || countSpawnsByChild(rs.events, rev) != 1 {
		t.Fatalf("spawn counts: %v", summarizeEventTypes(rs.events))
	}
	// Cancelled child must not get agent_result_injected (BUG-294 parity).
	for _, ev := range rs.events {
		if ev.Type == EventAgentResultInjected && ev.ChildRunID == rev {
			t.Fatalf("cancelled child must not emit result: %v", summarizeEventTypes(rs.events))
		}
	}
	// Coder still has result.
	hasCoderResult := false
	for _, ev := range rs.events {
		if ev.Type == EventAgentResultInjected && ev.ChildRunID == coder {
			hasCoderResult = true
		}
	}
	if !hasCoderResult {
		t.Fatalf("completed coder missing result: %v", summarizeEventTypes(rs.events))
	}
}

// TestDurableOrderNoHubMessagesPlacesAgentsAfterPrompt ensures empty hub prose
// still places agent cards after the user prompt (not above it).
func TestDurableOrderNoHubMessagesPlacesAgentsAfterPrompt(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	const parent = "run-no-hub-msg"
	const coder = "run-nhm-coder"
	seedDurableOrderHub(t, store, parent, ProviderKeyCodex,
		[]durableOrderChild{
			{runID: coder, label: "coder", agent: "coder", role: "coder",
				started: "2026-07-23T10:00:01Z", updated: "2026-07-23T10:00:02Z"},
		},
		[]stepTransitionLine{
			{NodeID: "coder", Status: "RUNNING", TS: "2026-07-23T10:00:01Z"},
			{NodeID: "coder", Status: "DONE", TS: "2026-07-23T10:00:02Z"},
		},
	)
	rs := resumeDurableOrder(t, store, parent, ProviderKeyCodex, []ProviderEvent{
		{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug"},
	})
	skel := spawnMessageSkeleton(rs.events)
	want := []string{"prompt", "spawn:coder:" + coder}
	if !stringSlicesEqual(skel, want) {
		t.Fatalf("no-hub-msg order wrong:\n got: %v\nwant: %v", skel, want)
	}
}
