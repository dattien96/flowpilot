package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// Task-447 (CP-87 P-3/P-4): same-provider account preflight + per-leg pinning.
// The resolver ranks connected accounts of the demand's provider by normalized
// headroom; claims are durable, cooldown-guarded (exactly 20s,
// reason=same_provider_ip_safety), capped at two automatic switches per run,
// and never mutate the machine-global active account.

// ---- fixtures ---------------------------------------------------------------

// writeTask447Accounts installs a hermetic provider-accounts store: one JSON
// doc under FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH plus a minimal valid auth
// file per account home so syncProviderAccounts reports them connected.
func writeTask447Accounts(t *testing.T, accounts []ProviderAccount) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "provider-accounts.json")
	for i := range accounts {
		home := accounts[i].HomePath
		if home == "" {
			home = filepath.Join(dir, fmt.Sprintf("home-%d", i))
			accounts[i].HomePath = home
		}
		writeTask447AuthFile(t, accounts[i].ProviderKey, home)
	}
	raw, err := json.Marshal(providerAccountState{Accounts: accounts})
	if err != nil {
		t.Fatalf("marshal accounts: %v", err)
	}
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		t.Fatalf("write accounts: %v", err)
	}
	t.Setenv("FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH", cfgPath)
}

func writeTask447AuthFile(t *testing.T, providerKey, home string) {
	t.Helper()
	var rel, body string
	switch providerKey {
	case "codex":
		rel, body = "auth.json", `{"tokens":{"id_token":"x"}}`
	case "claude":
		rel, body = filepath.Join(".claude", ".credentials.json"), `{"access_token":"x","refresh_token":"y"}`
	case "grok":
		rel, body = "auth.json", `{"issuer::uid":{"refresh_token":"x","email":"a@b.c"}}`
	case "devin":
		rel, body = "credentials.toml", `windsurf_api_key = "x"`
	case "opencode":
		rel, body = "auth.json", `{"openai":{"key":"sk-x"}}`
	default:
		rel, body = "auth.json", `{"token":"x"}`
	}
	path := filepath.Join(home, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir auth: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write auth: %v", err)
	}
}

func task447Account(id, provider string, slot int, active bool) ProviderAccount {
	return ProviderAccount{
		ID:          id,
		ProviderKey: provider,
		DisplayName: id,
		SlotIndex:   slot,
		IsActive:    active,
		AuthStatus:  "connected",
		CreatedAt:   "2026-01-01T00:00:00Z",
		ExtraEnv:    map[string]string{},
	}
}

// task447Telemetry returns a telemetry seam driven by a per-account percent map.
func task447Telemetry(pct map[string]int, observedAt string) func(context.Context, ProviderAccount) ProviderAccountSummary {
	return func(_ context.Context, a ProviderAccount) ProviderAccountSummary {
		sum := ProviderAccountSummary{ID: a.ID, ProviderKey: a.ProviderKey, UsageSource: "provider_api", ObservedAt: observedAt}
		if p, ok := pct[a.ID]; ok {
			v := p
			sum.Remaining5hPercent = &v
		}
		return sum
	}
}

func task447Service(t *testing.T) *InteractiveService {
	t.Helper()
	s := NewInteractiveService()
	s.quotaRuntimePath = filepath.Join(t.TempDir(), "quota-routing-state.json")
	return s
}

func task447PinnedRun(s *InteractiveService, runID string, provider ProviderKey, accountID string) *interactiveRun {
	rs := &interactiveRun{
		id:                runID,
		providerKey:       provider,
		modelName:         "gpt-5.4",
		providerAccountID: accountID,
		chatID:            "chat-1",
		status:            RunStatusIdle,
		runKind:           "chat",
	}
	s.mu.Lock()
	s.runs[runID] = rs
	s.mu.Unlock()
	return rs
}

// ---- T-1: demand resolution --------------------------------------------------

func TestTask447_HubDemandUsesPinnedLeg(t *testing.T) {
	s := task447Service(t)
	rs := task447PinnedRun(s, "run-hub", ProviderKeyCodex, "acc-pinned")
	rs.accountPinned = true // claim-pinned leg must surface its pin, not the global active

	demand, err := s.ResolveExecutionDemand(context.Background(), "run-hub", nil)
	if err != nil {
		t.Fatalf("ResolveExecutionDemand: %v", err)
	}
	if !demand.IsMainHub {
		t.Fatal("hub run demand must be marked IsMainHub")
	}
	if demand.RequestedProvider != ProviderKeyCodex || demand.RequestedModel != "gpt-5.4" {
		t.Fatalf("demand carries the leg's pinned binding, got %+v", demand)
	}
	if demand.RequestedAccountID != "acc-pinned" {
		t.Fatalf("demand account = pinned leg account, got %q", demand.RequestedAccountID)
	}
}

func TestTask447_ChildDemandUsesResolvedNodeModel(t *testing.T) {
	s := task447Service(t)
	task447PinnedRun(s, "run-parent", ProviderKeyClaude, "cl-1")
	node := &agentpack.FlowNode{
		ID:            "implement",
		Behavior:      "agent.code",
		WorkloadClass: agentpack.WorkloadCoding,
		Model:         "gpt-5.4",
	}
	demand, err := s.ResolveExecutionDemand(context.Background(), "run-parent", node)
	if err != nil {
		t.Fatalf("ResolveExecutionDemand: %v", err)
	}
	if demand.IsMainHub {
		t.Fatal("child node demand must not be marked IsMainHub")
	}
	if demand.RequestedModel != "gpt-5.4" {
		t.Fatalf("child demand resolves the node's effective model (Task-320), got %q", demand.RequestedModel)
	}
	if demand.RequestedProvider != ProviderKeyCodex {
		t.Fatalf("model gpt-5.4 derives provider codex, got %q", demand.RequestedProvider)
	}
	if demand.WorkloadClass != agentpack.WorkloadCoding {
		t.Fatalf("child demand carries the node's workload class, got %q", demand.WorkloadClass)
	}
}

// ---- T-2: candidate ranking -------------------------------------------------

func TestTask447_HealthySameProviderAccountWins(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
		task447Account("cx-2", "codex", 2, false),
	})
	s := task447Service(t)
	s.quotaTelemetryFn = task447Telemetry(map[string]int{
		"cx-0": 5, "cx-1": 80, "cx-2": 60,
	}, now.Format(time.RFC3339))

	demand := ExecutionDemand{
		RunID: "run-1", RequestedProvider: ProviderKeyCodex,
		RequestedModel: "gpt-5.4", RequestedAccountID: "cx-0",
	}
	cands, err := s.SameProviderCandidates(context.Background(), demand)
	if err != nil {
		t.Fatalf("SameProviderCandidates: %v", err)
	}
	if len(cands) == 0 {
		t.Fatal("expected candidates")
	}
	best := cands[0]
	if best.AccountID != "cx-1" {
		t.Fatalf("highest fresh headroom wins, got %q", best.AccountID)
	}
	if !best.AutoEligible {
		t.Fatalf("healthy exact+fresh account must be auto-eligible, reasons=%v", best.RejectionReasons)
	}
}

func TestTask447_UnknownQuotaManualOnly(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task447Service(t)
	// cx-1 has NO percent evidence -> headroom unknown -> manual candidate only.
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0}, now.Format(time.RFC3339))

	cands, err := s.SameProviderCandidates(context.Background(), ExecutionDemand{
		RunID: "run-1", RequestedProvider: ProviderKeyCodex, RequestedAccountID: "cx-0",
	})
	if err != nil {
		t.Fatalf("SameProviderCandidates: %v", err)
	}
	var unknown *AccountCandidate
	for i := range cands {
		if cands[i].AccountID == "cx-1" {
			unknown = &cands[i]
		}
	}
	if unknown == nil {
		t.Fatal("unknown-quota account must remain a manual candidate")
	}
	if unknown.AutoEligible {
		t.Fatal("unknown quota is never auto-eligible (T-2)")
	}
	if unknown.Headroom.State != "unknown" {
		t.Fatalf("headroom state = unknown, got %q", unknown.Headroom.State)
	}
}

// ---- T-4: cooldown + cap ----------------------------------------------------

func TestTask447_CooldownBlocksRapidSwitch(t *testing.T) {
	base := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
		task447Account("cx-2", "codex", 2, false),
	})
	s := task447Service(t)
	cur := base
	s.quotaNowFn = func() time.Time { return cur }
	s.quotaTelemetryFn = task447Telemetry(map[string]int{
		"cx-0": 0, "cx-1": 80, "cx-2": 70,
	}, base.Format(time.RFC3339))
	rs := task447PinnedRun(s, "run-1", ProviderKeyCodex, "cx-0")

	demand := ExecutionDemand{RunID: "run-1", RequestedProvider: ProviderKeyCodex,
		RequestedModel: "gpt-5.4", RequestedAccountID: "cx-0"}
	cands, err := s.SameProviderCandidates(context.Background(), demand)
	if err != nil {
		t.Fatalf("candidates: %v", err)
	}
	var target *AccountCandidate
	for i := range cands {
		if cands[i].AccountID == "cx-1" && cands[i].AutoEligible {
			target = &cands[i]
		}
	}
	if target == nil {
		t.Fatal("cx-1 should be auto-eligible pre-cooldown")
	}
	binding, err := s.ClaimAccountForLeg(context.Background(), demand, *target)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if binding.AccountID != "cx-1" {
		t.Fatalf("binding pins claimed account, got %q", binding.AccountID)
	}
	if binding.CooldownReason != "same_provider_ip_safety" || binding.CooldownUntil == "" {
		t.Fatalf("claim publishes the cooldown projection, got %+v", binding)
	}

	// 5s later: a second same-provider switch is inside the 20s window — the
	// claim must refuse and no provider call may happen yet.
	cur = base.Add(5 * time.Second)
	rs.providerAccountID = "cx-1" // the first switch landed
	demand.RequestedAccountID = "cx-1"
	cands2, err := s.SameProviderCandidates(context.Background(), demand)
	if err != nil {
		t.Fatalf("candidates2: %v", err)
	}
	var next *AccountCandidate
	for i := range cands2 {
		if cands2[i].AccountID == "cx-2" {
			next = &cands2[i]
		}
	}
	if next == nil {
		t.Fatal("cx-2 missing")
	}
	if next.AutoEligible {
		t.Fatal("cooldown must strip auto-eligibility inside 20s")
	}
	if next.CooldownReason != "same_provider_ip_safety" || next.CooldownUntil == "" {
		t.Fatalf("candidate carries cooldown projection, got %+v", next)
	}
	if _, err := s.ClaimAccountForLeg(context.Background(), demand, *next); err == nil {
		t.Fatal("claim during active cooldown must refuse — no provider call before 20s")
	}

	// At exactly 20s the window is over.
	cur = base.Add(20 * time.Second)
	cands3, err := s.SameProviderCandidates(context.Background(), demand)
	if err != nil {
		t.Fatalf("candidates3: %v", err)
	}
	for i := range cands3 {
		if cands3[i].AccountID == "cx-2" {
			if !cands3[i].AutoEligible {
				t.Fatalf("at exactly 20s the cooldown has elapsed, reasons=%v", cands3[i].RejectionReasons)
			}
		}
	}
}

func TestTask447_CooldownPublishesStableTimestamps(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second)
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task447Service(t)
	cur := base
	s.quotaNowFn = func() time.Time { return cur }
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, base.Format(time.RFC3339))
	task447PinnedRun(s, "run-1", ProviderKeyCodex, "cx-0")

	demand := ExecutionDemand{RunID: "run-1", RequestedProvider: ProviderKeyCodex, RequestedAccountID: "cx-0"}
	cands, _ := s.SameProviderCandidates(context.Background(), demand)
	var target *AccountCandidate
	for i := range cands {
		if cands[i].AccountID == "cx-1" {
			target = &cands[i]
		}
	}
	binding, err := s.ClaimAccountForLeg(context.Background(), demand, *target)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	wantUntil := base.Add(20 * time.Second).Format(time.RFC3339)
	if binding.CooldownStartedAt != base.Format(time.RFC3339) || binding.CooldownUntil != wantUntil {
		t.Fatalf("cooldown stamps are server-authoritative started=%q until=%q want until=%q",
			binding.CooldownStartedAt, binding.CooldownUntil, wantUntil)
	}

	// A refresh mid-window must report the SAME deadline, not a re-based one.
	cur = base.Add(7 * time.Second)
	cands2, _ := s.SameProviderCandidates(context.Background(), ExecutionDemand{
		RunID: "run-1", RequestedProvider: ProviderKeyCodex, RequestedAccountID: "cx-1"})
	for i := range cands2 {
		if cands2[i].AccountID == "cx-0" && cands2[i].CooldownUntil != "" {
			if cands2[i].CooldownUntil != wantUntil {
				t.Fatalf("refresh re-based the deadline: got %q want %q", cands2[i].CooldownUntil, wantUntil)
			}
		}
	}
}

func TestTask447_MaxTwoAutoSwitchesThenGate(t *testing.T) {
	base := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
		task447Account("cx-2", "codex", 2, false),
		task447Account("cx-3", "codex", 3, false),
	})
	s := task447Service(t)
	cur := base
	s.quotaNowFn = func() time.Time { return cur }
	s.quotaTelemetryFn = task447Telemetry(map[string]int{
		"cx-0": 0, "cx-1": 80, "cx-2": 70, "cx-3": 60,
	}, base.Format(time.RFC3339))
	rs := task447PinnedRun(s, "run-1", ProviderKeyCodex, "cx-0")

	demand := ExecutionDemand{RunID: "run-1", RequestedProvider: ProviderKeyCodex, RequestedAccountID: "cx-0"}
	claimAuto := func(toID string) {
		cands, err := s.SameProviderCandidates(context.Background(), demand)
		if err != nil {
			t.Fatalf("candidates: %v", err)
		}
		for i := range cands {
			if cands[i].AccountID == toID && cands[i].AutoEligible {
				if _, err := s.claimAccountForLeg(context.Background(), demand, cands[i], true); err != nil {
					t.Fatalf("auto claim %s: %v", toID, err)
				}
				rs.providerAccountID = toID
				demand.RequestedAccountID = toID
				return
			}
		}
		t.Fatalf("candidate %s not auto-eligible", toID)
	}

	claimAuto("cx-1")
	cur = cur.Add(21 * time.Second)
	claimAuto("cx-2")
	cur = cur.Add(21 * time.Second)

	// Third automatic switch exceeds the per-run cap -> everything demotes to manual.
	cands, err := s.SameProviderCandidates(context.Background(), demand)
	if err != nil {
		t.Fatalf("candidates: %v", err)
	}
	for i := range cands {
		if cands[i].AccountID == "cx-3" {
			if cands[i].AutoEligible {
				t.Fatal("third auto switch must gate the user (cap=2)")
			}
		}
	}
}

func TestTask447_NoGlobalSetActiveAccount(t *testing.T) {
	base := time.Now().UTC()
	accounts := []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	}
	writeTask447Accounts(t, accounts)
	s := task447Service(t)
	s.quotaNowFn = func() time.Time { return base }
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, base.Format(time.RFC3339))
	task447PinnedRun(s, "run-1", ProviderKeyCodex, "cx-0")

	// Baseline AFTER the first store read — ListProviderAccounts syncs host
	// accounts and normalizes the active set once; the claim must flip nothing.
	baseAccounts, err := (&Runner{}).ListProviderAccounts()
	if err != nil {
		t.Fatalf("baseline list: %v", err)
	}
	baseActive := map[string]bool{}
	for _, a := range baseAccounts {
		baseActive[a.ID] = a.IsActive
	}

	demand := ExecutionDemand{RunID: "run-1", RequestedProvider: ProviderKeyCodex, RequestedAccountID: "cx-0"}
	cands, _ := s.SameProviderCandidates(context.Background(), demand)
	for i := range cands {
		if cands[i].AccountID == "cx-1" {
			if _, err := s.ClaimAccountForLeg(context.Background(), demand, cands[i]); err != nil {
				t.Fatalf("claim: %v", err)
			}
		}
	}

	// The store must be byte-identical on IsActive: the resolver pins per leg,
	// never mutates the machine-global active account.
	after, err := (&Runner{}).ListProviderAccounts()
	if err != nil {
		t.Fatalf("relist: %v", err)
	}
	for _, a := range after {
		if a.IsActive != baseActive[a.ID] {
			t.Fatalf("active flag mutated for %s: got %v want %v (SetActiveAccount must not fire)", a.ID, a.IsActive, baseActive[a.ID])
		}
	}
}

// ---- claims: durability + concurrency ----------------------------------------

func TestTask447_ConcurrentClaimsDoNotOverbook(t *testing.T) {
	base := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task447Service(t)
	s.quotaNowFn = func() time.Time { return base }
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, base.Format(time.RFC3339))
	// Two runs racing for the same account: only one may hold an active claim.
	task447PinnedRun(s, "run-a", ProviderKeyCodex, "cx-0")
	task447PinnedRun(s, "run-b", ProviderKeyCodex, "cx-0")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	bindings := make([]LegBinding, 2)
	for i, runID := range []string{"run-a", "run-b"} {
		wg.Add(1)
		go func(i int, runID string) {
			defer wg.Done()
			demand := ExecutionDemand{RunID: runID, RequestedProvider: ProviderKeyCodex, RequestedAccountID: "cx-0"}
			cands, err := s.SameProviderCandidates(context.Background(), demand)
			if err != nil {
				errs[i] = err
				return
			}
			for j := range cands {
				if cands[j].AccountID == "cx-1" {
					bindings[i], errs[i] = s.ClaimAccountForLeg(context.Background(), demand, cands[j])
				}
			}
		}(i, runID)
	}
	wg.Wait()
	won := 0
	for i := range errs {
		if errs[i] == nil {
			won++
		}
	}
	if won != 1 {
		t.Fatalf("concurrent claims must serialize — exactly one wins, got %d (errs=%v)", won, errs)
	}
}

func TestTask447_RestartPreservesClaimAndCooldown(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second)
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task447Service(t)
	statePath := s.quotaRuntimePath
	cur := base
	s.quotaNowFn = func() time.Time { return cur }
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, base.Format(time.RFC3339))
	task447PinnedRun(s, "run-1", ProviderKeyCodex, "cx-0")

	demand := ExecutionDemand{RunID: "run-1", RequestedProvider: ProviderKeyCodex, RequestedAccountID: "cx-0"}
	cands, _ := s.SameProviderCandidates(context.Background(), demand)
	for i := range cands {
		if cands[i].AccountID == "cx-1" {
			if _, err := s.ClaimAccountForLeg(context.Background(), demand, cands[i]); err != nil {
				t.Fatalf("claim: %v", err)
			}
		}
	}

	// Restart: new service over the same runtime state file, 10s later —
	// the cooldown deadline and the held claim must both survive.
	s2 := task447Service(t)
	s2.quotaRuntimePath = statePath
	cur = base.Add(10 * time.Second)
	s2.quotaNowFn = func() time.Time { return cur }
	s2.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, base.Format(time.RFC3339))

	demand2 := ExecutionDemand{RunID: "run-2", RequestedProvider: ProviderKeyCodex, RequestedAccountID: "cx-1"}
	cands2, err := s2.SameProviderCandidates(context.Background(), demand2)
	if err != nil {
		t.Fatalf("candidates after restart: %v", err)
	}
	for i := range cands2 {
		if cands2[i].AccountID == "cx-0" && cands2[i].CooldownUntil == "" {
			t.Fatal("cooldown projection lost across restart")
		}
		if cands2[i].AccountID == "cx-1" {
			// cx-1 is claimed by run-1 — a different run must not auto-claim it.
			if cands2[i].AutoEligible {
				t.Fatal("held claim must not be auto-eligible for another run after restart")
			}
		}
	}
}
