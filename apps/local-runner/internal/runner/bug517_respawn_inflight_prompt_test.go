package runner

// BUG-517 (deep review round 3, 2026-09-26): a flow child whose pinned
// account hard-vetoes admission gets a cross-provider respawn — but
// respawnChildOnRoute seeded the replacement from child.lastPrompt, the
// 100-char display-truncated title from a PREVIOUS turn. The turn that
// triggered routing (in.Prompt) was returned as quota_route_committed
// and never stored, so the replacement child started on a stale,
// truncated instruction — the blocked prompt was swallowed.
//
// Fix: QuotaResolution.PendingPrompt carries the refused turn's prompt
// through the commit; respawn prefers it, then the full lastFullPrompt,
// never the truncated title.

import (
	"context"
	"strings"
	"testing"
	"time"
)

func bug517SpawnChild(t *testing.T, svc *InteractiveService, parentID string) *interactiveRun {
	t.Helper()
	res, err := svc.spawnChildRun(context.Background(), parentID, SpawnAgentInput{
		Agent:  "coder",
		Prompt: "write the first draft",
		Wait:   false,
	})
	if err != nil {
		t.Fatalf("spawnChildRun: %v", err)
	}
	svc.mu.Lock()
	child := svc.runs[res.RunID]
	svc.mu.Unlock()
	if child == nil {
		t.Fatalf("child %q not in runs", res.RunID)
	}
	return child
}

func bug517NewChildPrompt(t *testing.T, svc *InteractiveService, parentID, oldChildID string) string {
	t.Helper()
	svc.mu.Lock()
	newChildID := ""
	for _, rs := range svc.runs {
		if rs.parentRunID == parentID && rs.id != oldChildID {
			newChildID = rs.id
		}
	}
	svc.mu.Unlock()
	if newChildID == "" {
		t.Fatal("no replacement child spawned")
	}
	// The spawn prompt lands on the parent's agent bus as the handoff
	// message — synchronously, before the async first turn (which fails
	// early on fakes and never exposes the prompt).
	svc.agentOrchestrator.mu.Lock()
	defer svc.agentOrchestrator.mu.Unlock()
	for _, m := range svc.agentOrchestrator.bus[parentID] {
		if m.Kind == "handoff" && m.ToRunID == newChildID {
			return m.Message
		}
	}
	t.Fatal("no handoff bus message for replacement child")
	return ""
}

func TestBug517_RespawnSeedsInflightPrompt(t *testing.T) {
	svc, _ := newSwitchTestService(t) // claude fake registered — cross-provider respawn lands on it
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	child := bug517SpawnChild(t, svc, parent.RunID)
	svc.mu.Lock()
	child.lastPrompt = "write the first draft"           // old turn's title
	child.lastFullPrompt = "write the first draft FULL" // old turn's full text
	svc.mu.Unlock()

	sel := &RouteCandidate{ProviderKey: ProviderKeyClaude, Model: "claude-sonnet-4-5", AccountID: "acct-x"}
	pending := "THE-INFLIGHT-PROMPT: implement the second task exactly"
	if err := svc.respawnChildOnRoute(context.Background(), child, sel, pending); err != nil {
		t.Fatalf("respawnChildOnRoute: %v", err)
	}
	got := bug517NewChildPrompt(t, svc, parent.RunID, child.id)
	if got != pending {
		t.Fatalf("replacement child prompt = %q — the in-flight prompt was swallowed", got)
	}
	svc.mu.Lock()
	closed := child.legState
	svc.mu.Unlock()
	if closed != LegStateClosed {
		t.Fatalf("exhausted child leg state = %q, want closed", closed)
	}
}

// Round 4 (review): the two tests above call respawnChildOnRoute with a
// pending prompt already in hand — they would stay green if startTurn
// never assigned res.PendingPrompt. This test enters through the real
// admission path: a child pinned to a ledger-blocked account, auto mode,
// a healthy cross-provider candidate. startTurn must refuse the turn,
// carry in.Prompt through the commit, and the replacement child's handoff
// must be the refused prompt — not the stale lastPrompt title.
func TestBug517_AdmissionCarriesInflightPromptToRespawnedChild(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cl-0", "claude", 0, true),
	})
	svc := task448Service(t, ProviderKeyCodex, task448Provider(ProviderKeyClaude))
	settings := task448Settings(map[string]string{"claude": "x"})
	settings.Mode = QuotaRotationAuto
	task449WriteSettings(t, settings)
	svc.quotaTelemetryFn = task447Telemetry(map[string]int{"cl-0": 88}, now.Format(time.RFC3339))
	parent, child := task449FlowChild(t, svc, "cx-0")
	svc.mu.Lock()
	child.lastPrompt = "stale title from an earlier turn" // must NOT win
	svc.noteAccountBlockedLocked("codex", "cx-0", "quota_exhausted")
	svc.mu.Unlock()

	const inflight = "THE-INFLIGHT-PROMPT: refused by the veto, must reach the replacement child"
	_, apiErr := svc.startTurn(child.id, TurnInput{StepID: "coder", Prompt: inflight}, "", "")
	if apiErr == nil || apiErr.code != "quota_route_committed" {
		t.Fatalf("cross-provider child rotate must return quota_route_committed, got %+v", apiErr)
	}
	got := bug517NewChildPrompt(t, svc, parent.id, child.id)
	if got != inflight {
		t.Fatalf("respawned child prompt = %q, want the refused turn's prompt %q — PendingPrompt was not carried through admission", got, inflight)
	}
	svc.mu.Lock()
	closed := child.legState
	svc.mu.Unlock()
	if closed != LegStateClosed {
		t.Fatalf("exhausted child leg state = %q, want closed", closed)
	}
}

func TestBug517_RespawnFallsBackToFullPromptNotTruncatedTitle(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	child := bug517SpawnChild(t, svc, parent.RunID)
	longPrompt := "instruction-" + strings.Repeat("x", 200) // > 100 chars
	svc.mu.Lock()
	child.lastFullPrompt = longPrompt
	child.lastPrompt = longPrompt[:100] // the truncated title form
	svc.mu.Unlock()

	sel := &RouteCandidate{ProviderKey: ProviderKeyClaude, Model: "claude-sonnet-4-5", AccountID: "acct-x"}
	if err := svc.respawnChildOnRoute(context.Background(), child, sel, ""); err != nil {
		t.Fatalf("respawnChildOnRoute: %v", err)
	}
	got := bug517NewChildPrompt(t, svc, parent.RunID, child.id)
	if got != longPrompt {
		t.Fatalf("without a pending prompt the respawn must use the FULL prior prompt; got %q", got)
	}
}
