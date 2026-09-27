package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// quota_exhausted durable block (review finding): a turn that fails with a
// live-observed quota_exhausted limit must land in the durable routing ledger
// just like billing_required/credits_exhausted — the emit path at
// finishTurn skipped quota_exhausted, so after a runner restart the account
// silently re-entered rotation and could be re-picked (telemetry is often
// absent/stale right after boot, exactly when the veto matters most).
//
// The block is bounded, not perpetual: quota windows self-heal, so a recorded
// quota_exhausted block lifts when fresh exact telemetry shows headroom —
// billing/credits blocks still stand until the operator fixes the account.

func TestQuotaExhaustedBlockPersistsAndSurvivesRestart(t *testing.T) {
	now := time.Now().UTC()
	t.Setenv("HOME", t.TempDir()) // isolate: account sync must not leak host accounts
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	statePath := filepath.Join(t.TempDir(), "quota-routing-state.json")

	// Live leg: codex fails the turn with a quota_exhausted-classified error.
	regFail := newProviderRegistry()
	regFail.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				return fmt.Errorf("usage limit reached; try again later")
			})
		},
	})
	svc := task447Service(t)
	svc.quotaRuntimePath = statePath
	svc.registry = regFail
	rs := task449ChatRun(svc, "run-1", "cx-0", "dev")

	if _, apiErr := svc.startTurn(rs.id, TurnInput{StepID: "s1", Prompt: "hi"}, "", ""); apiErr != nil {
		t.Fatalf("first turn must dispatch and fail on the provider, got %v", apiErr)
	}
	waitLoop(t, "turn failed with provider limit", 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return svc.runs[rs.id].status == RunStatusFailed
	})

	// The observed limit must already be in the durable ledger — this is the
	// row a restart rehydrates. RED: quota_exhausted never reached it.
	state := svc.loadQuotaRuntimeState()
	if reason := state.accountBlockReason("cx-0"); reason == "" {
		t.Fatal("quota_exhausted observation must persist a durable account block")
	} else if reason != string(ProviderLimitQuotaExhausted) {
		t.Fatalf("block reason = %q, want quota_exhausted", reason)
	}

	// Simulated restart: a fresh service shares only the ledger file — no
	// in-memory state carries over, telemetry is absent at boot.
	svc2 := task447Service(t)
	svc2.quotaRuntimePath = statePath
	regOK := newProviderRegistry()
	regOK.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc2.registry = regOK
	svc2.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, now.Format(time.RFC3339))
	rs2 := task449ChatRun(svc2, "run-9", "cx-0", "dev")

	if _, apiErr := svc2.startTurn(rs2.id, TurnInput{StepID: "s1", Prompt: "keep working"}, "", ""); apiErr == nil {
		t.Fatal("restart must not dispatch onto the durably-blocked account")
	} else if !strings.HasPrefix(apiErr.code, "quota_route") {
		// gate vs blocked both prove the durable veto fired; ambient account
		// discovery (host homes leak on darwin) decides which alternates exist.
		t.Fatalf("blocked pin after restart must enter the quota gate, got %v", apiErr)
	}
}

// The block is evidence-bounded, not permanent: once fresh exact telemetry
// shows the account has headroom again (quota window reset), a recorded
// quota_exhausted block must lift — the account re-enters rotation without an
// operator having to purge state.
func TestQuotaExhaustedBlockLiftsOnFreshHealthyTelemetry(t *testing.T) {
	now := time.Now().UTC()
	t.Setenv("HOME", t.TempDir()) // isolate: account sync must not leak host accounts
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	statePath := filepath.Join(t.TempDir(), "quota-routing-state.json")

	svc := task447Service(t)
	svc.quotaRuntimePath = statePath
	svc.mu.Lock()
	svc.noteAccountBlockedLocked("codex", "cx-0", "quota_exhausted")
	svc.mu.Unlock()

	// Telemetry now reports the window refilled: the recorded block is stale
	// and must not veto the pin.
	svc.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 90, "cx-1": 50}, now.Format(time.RFC3339))
	rs := task449ChatRun(svc, "run-1", "cx-0", "dev")
	if veto := svc.pinnedAccountHardVeto(context.Background(), rs); veto != "" {
		t.Fatalf("fresh healthy telemetry must lift a quota_exhausted block, veto=%q", veto)
	}

	// Candidate pass agrees: the recovered account carries no hard reject.
	cands, err := svc.SameProviderCandidates(context.Background(), task448Demand(ProviderKeyCodex, "dev"))
	if err != nil {
		t.Fatalf("SameProviderCandidates: %v", err)
	}
	found := false
	for _, c := range cands {
		if c.AccountID == "cx-0" {
			found = true
			for _, r := range c.RejectionReasons {
				if r == QuotaRejectExhaustedQuota || r == QuotaRejectBillingRequired {
					t.Fatalf("lifted quota_exhausted block must not reject cx-0, reasons=%v", c.RejectionReasons)
				}
			}
		}
	}
	if !found {
		t.Fatal("cx-0 must still appear in same-provider candidates")
	}

	// Stale/unknown telemetry must NOT lift it — the last hard observation
	// stands until the provider proves recovery.
	svc.quotaTelemetryFn = task447Telemetry(map[string]int{}, now.Add(-2*time.Hour).Format(time.RFC3339))
	if veto := svc.pinnedAccountHardVeto(context.Background(), rs); veto == "" {
		t.Fatal("no fresh telemetry must keep the recorded block")
	}
}
