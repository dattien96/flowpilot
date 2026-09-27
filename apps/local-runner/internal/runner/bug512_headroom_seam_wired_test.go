package runner

// BUG-512 (deep review B-2, 2026-09-26): contextResetHeadroomOK — the CP-87
// P-3b seam that decides whether a same-binding context reset can afford
// the reseed or must escalate to the quota routing gate — was only ever
// assigned in tests. In production it stayed nil, and the consumePending
// ContextReset seam treats nil as "enough headroom": the P-3b spec branch
// (reset on an exhausted budget → routing gate) was dead code.
//
// Fix: newInteractiveService wires a real default — reseed is affordable
// when the node's consumed usage plus the estimated reseed prompt fits its
// declared budget (uncapped nodes always pass; the reset is the designed
// path there). An over-budget reseed reports insufficient headroom so the
// existing escalate-to-routing branch fires.

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBug512_ProdHeadroomCallbackIsWired(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	if svc.contextResetHeadroomOK == nil {
		t.Fatal("production must wire the P-3b headroom check — nil silently disables the escalate-to-routing branch")
	}
}

func TestBug512_HeadroomBlocksReseedPastBudget(t *testing.T) {
	svc, _, child := task442ProfiledChild(t)
	if svc.contextResetHeadroomOK == nil {
		t.Fatal("precondition: prod-wired headroom callback")
	}
	// Drive the child's session usage past the coder profile's declared cap.
	task442EmitUsage(svc, child, "leg-a", task442CoderUsageCap+1)
	if svc.contextResetHeadroomOK(child) {
		t.Fatal("reseed on an over-budget binding must report insufficient headroom so the reset escalates to the routing gate")
	}
}

func TestBug512_HeadroomAllowsUncappedAndUnderBudget(t *testing.T) {
	svc, _, child := task442ProfiledChild(t)
	// Under budget: the reset is the designed path — no quota problem.
	task442EmitUsage(svc, child, "leg-a", 1)
	if !svc.contextResetHeadroomOK(child) {
		t.Fatal("under-budget reseed must pass headroom")
	}
	// Unprofiled run (no parent flow profile): no cap — headroom always OK.
	plain := &interactiveRun{id: "run-plain"}
	if !svc.contextResetHeadroomOK(plain) {
		t.Fatal("uncapped run must pass headroom")
	}
}

// Round 2 (review): P-3b is an ACCOUNT-headroom check, not a node-budget
// check. An uncapped node (the common case — no maxUsageTokens declared)
// must still refuse the reset when its pinned account is ledger-blocked or
// telemetry-exhausted: a same-binding reseed there mints a leg that can
// only fail on dispatch, so the quota gate must take over.
func TestBug512_LedgerBlockedPinRejectsReseedOnUncappedNode(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.quotaRuntimePath = filepath.Join(t.TempDir(), "quota-routing-state.json")
	child := &interactiveRun{
		id: "run-plain", providerKey: ProviderKeyCodex,
		providerAccountID: "cx-0", status: RunStatusIdle,
	}
	svc.mu.Lock()
	svc.runs[child.id] = child
	svc.noteAccountBlockedLocked("codex", "cx-0", "quota_exhausted")
	svc.mu.Unlock()

	if svc.contextResetHeadroomOK(child) {
		t.Fatal("ledger-blocked pin must fail the reseed headroom check even on an uncapped node")
	}
}

func TestBug512_TelemetryExhaustedPinRejectsReseedOnUncappedNode(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
	})
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.quotaRuntimePath = filepath.Join(t.TempDir(), "quota-routing-state.json")
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	svc.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0}, now.Format(time.RFC3339))
	child := &interactiveRun{
		id: "run-plain", providerKey: ProviderKeyCodex,
		providerAccountID: "cx-0", status: RunStatusIdle,
	}
	svc.mu.Lock()
	svc.runs[child.id] = child
	svc.mu.Unlock()

	if svc.contextResetHeadroomOK(child) {
		t.Fatal("telemetry-exhausted pin must fail the reseed headroom check even on an uncapped node")
	}
}

func TestBug512_HealthyPinPassesOnUncappedNode(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
	})
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.quotaRuntimePath = filepath.Join(t.TempDir(), "quota-routing-state.json")
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	svc.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 90}, now.Format(time.RFC3339))
	child := &interactiveRun{
		id: "run-plain", providerKey: ProviderKeyCodex,
		providerAccountID: "cx-0", status: RunStatusIdle,
	}
	svc.mu.Lock()
	svc.runs[child.id] = child
	svc.mu.Unlock()

	if !svc.contextResetHeadroomOK(child) {
		t.Fatal("healthy pin on an uncapped node must pass headroom")
	}
}
