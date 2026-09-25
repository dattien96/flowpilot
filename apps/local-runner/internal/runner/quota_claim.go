package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Quota claims (Task-447 / CP-87 P-4): a claim binds one run's next leg to a
// specific provider account without mutating the machine-global active
// account. Claims, same-provider switch cooldowns, and blocked accounts live
// in one durable machine-local ledger (quota-routing-state.json beside
// quota-routing.json) so a restart replays identical routing decisions.

// quotaMaxAutoSwitchesPerRun bounds automatic same-provider rotation per run
// (Task-447 T-4): the third would-be switch demotes every candidate to manual.
const quotaMaxAutoSwitchesPerRun = 2

// QuotaCooldownReason is the only reason code a same-provider switch publishes.
const QuotaCooldownReason = "same_provider_ip_safety"

const (
	quotaClaimStatusActive     = "active"
	quotaClaimStatusSuperseded = "superseded"
	quotaClaimStatusReleased   = "released"
)

// QuotaClaim is one durable routing decision: run X bound provider account Y
// at time T (automatic or user-picked). Claims are append-only in the ledger;
// a switch supersedes the run's previous active claim for that provider.
type QuotaClaim struct {
	ClaimID       string `json:"claimId"`
	RunID         string `json:"runId"`
	ProviderKey   string `json:"providerKey"`
	AccountID     string `json:"accountId"`
	WorkloadClass string `json:"workloadClass,omitempty"`
	Status        string `json:"status"`
	Automatic     bool   `json:"automatic"`
	ClaimedAt     string `json:"claimedAt"`
	ReleasedAt    string `json:"releasedAt,omitempty"`
}

// LegBinding is the claim's outcome — the account pin the caller stamps onto
// the new leg's StartRunInput (or inherits for a child). Cooldown fields echo
// the just-minted switch window so the gate projection renders the deadline.
type LegBinding struct {
	RunID             string      `json:"runId"`
	ProviderKey       ProviderKey `json:"providerKey"`
	Model             string      `json:"model,omitempty"`
	AccountID         string      `json:"accountId"`
	ClaimID           string      `json:"claimId"`
	Automatic         bool        `json:"automatic"`
	CooldownStartedAt string      `json:"cooldownStartedAt,omitempty"`
	CooldownUntil     string      `json:"cooldownUntil,omitempty"`
	CooldownReason    string      `json:"cooldownReason,omitempty"`
}

// quotaSwitchRecord is one same-provider account switch in the ledger. The
// StartedAt/Until window is server-authoritative — refreshes must report the
// same deadline, never a re-based one (T-4).
type quotaSwitchRecord struct {
	ProviderKey   string `json:"providerKey"`
	FromAccountID string `json:"fromAccountId,omitempty"`
	ToAccountID   string `json:"toAccountId"`
	RunID         string `json:"runId,omitempty"`
	Automatic     bool   `json:"automatic"`
	Reason        string `json:"reason"`
	StartedAt     string `json:"startedAt"`
	Until         string `json:"until"`
}

// quotaBlockedAccount records a live-observed hard limit on an account
// (billing_required / credits_exhausted from Task-445 typed events) so the
// candidate table keeps it out of rotation until telemetry shows recovery.
type quotaBlockedAccount struct {
	ProviderKey string `json:"providerKey"`
	AccountID   string `json:"accountId"`
	Reason      string `json:"reason"`
	At          string `json:"at"`
}

// quotaRoutingRuntimeState is the durable machine-local routing ledger.
type quotaRoutingRuntimeState struct {
	Switches []quotaSwitchRecord   `json:"switches,omitempty"`
	Claims   []QuotaClaim          `json:"claims,omitempty"`
	Blocked  []quotaBlockedAccount `json:"blockedAccounts,omitempty"`
}

func (st *quotaRoutingRuntimeState) accountBlocked(accountID string) bool {
	return st.accountBlockReason(accountID) != ""
}

// accountBlockReason returns the recorded block kind (billing_required /
// credits_exhausted / quota_exhausted) or "" — the candidate table maps it to
// the matching typed rejection reason instead of flattening to billing.
func (st *quotaRoutingRuntimeState) accountBlockReason(accountID string) string {
	for _, b := range st.Blocked {
		if b.AccountID == accountID {
			return b.Reason
		}
	}
	return ""
}

// quotaBlockRejectionReason maps a recorded block kind to its rejection code.
func quotaBlockRejectionReason(kind string) string {
	if kind == string(ProviderLimitBillingRequired) {
		return QuotaRejectBillingRequired
	}
	return QuotaRejectExhaustedQuota
}

// accountClaimedByOtherRun reports whether another run currently holds an
// active claim on accountID — the overbooking guard: one account, one leg.
func (st *quotaRoutingRuntimeState) accountClaimedByOtherRun(accountID, runID string) bool {
	for _, c := range st.Claims {
		if c.AccountID == accountID && c.RunID != runID && c.Status == quotaClaimStatusActive {
			return true
		}
	}
	return false
}

// runAlreadyTried reports whether this run previously claimed accountID — a
// rotation cycle back to a failed account is a manual-only candidate.
func (st *quotaRoutingRuntimeState) runAlreadyTried(runID, providerKey, accountID string) bool {
	for _, c := range st.Claims {
		if c.RunID == runID && c.ProviderKey == providerKey && c.AccountID == accountID {
			return true
		}
	}
	return false
}

// activeSameProviderCooldown returns the still-open switch window for the
// provider, if any. The window is open while now < Until — at exactly Until
// the cooldown has elapsed (T-4's "exactly 20s").
func (st *quotaRoutingRuntimeState) activeSameProviderCooldown(providerKey string, now time.Time) (quotaSwitchRecord, bool) {
	for i := len(st.Switches) - 1; i >= 0; i-- {
		sw := st.Switches[i]
		if sw.ProviderKey != providerKey {
			continue
		}
		until, err := time.Parse(time.RFC3339, sw.Until)
		if err != nil {
			continue
		}
		if now.Before(until) {
			return sw, true
		}
		return sw, false // newest record for the provider decides; older are history
	}
	return quotaSwitchRecord{}, false
}

func (st *quotaRoutingRuntimeState) autoSwitchCountForRun(runID string) int {
	n := 0
	for _, sw := range st.Switches {
		if sw.RunID == runID && sw.Automatic {
			n++
		}
	}
	return n
}

// supersedeRunClaims closes the run's active claims for providerKey before a
// new claim replaces them — the ledger never holds two active claims for one
// (run, provider) pair.
func (st *quotaRoutingRuntimeState) supersedeRunClaims(runID, providerKey string, now time.Time) {
	for i := range st.Claims {
		c := &st.Claims[i]
		if c.RunID == runID && c.ProviderKey == providerKey && c.Status == quotaClaimStatusActive {
			c.Status = quotaClaimStatusSuperseded
			c.ReleasedAt = now.Format(time.RFC3339)
		}
	}
}

// quotaCooldownError refuses a claim while a same-provider switch window is
// still open — the gate maps it to a countdown, not a generic failure.
type quotaCooldownError struct {
	until  string
	reason string
}

func (e *quotaCooldownError) Error() string {
	return fmt.Sprintf("quota_cooldown: same-provider switch blocked until %s (%s)", e.until, e.reason)
}

var errQuotaAccountClaimed = errors.New("quota_claim_conflict: account already claimed by another run")
var errQuotaAutoSwitchCap = errors.New("quota_auto_switch_cap: per-run automatic switch budget exhausted")

// quotaRuntimeStatePath resolves the ledger location (machine-global, beside
// the settings doc). Tests override via s.quotaRuntimePath.
func (s *InteractiveService) quotaRuntimeStatePath() string {
	if s.quotaRuntimePath != "" {
		return s.quotaRuntimePath
	}
	if envPath := strings.TrimSpace(os.Getenv("FLOWPILOT_QUOTA_ROUTING_STATE_FILE")); envPath != "" {
		return envPath
	}
	var base string
	switch runtime.GOOS {
	case "windows":
		base = strings.TrimSpace(os.Getenv("APPDATA"))
		if base != "" {
			return filepath.Join(base, "FlowPilot", "quota-routing-state.json")
		}
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Application Support", "FlowPilot", "quota-routing-state.json")
		}
	default:
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".config", "FlowPilot", "quota-routing-state.json")
		}
	}
	return ""
}

// loadQuotaRuntimeState reads the durable ledger; a missing/corrupt file is a
// zero ledger (fail-open reads — claims still validate through the store),
// while writes are serialized under quotaMu.
func (s *InteractiveService) loadQuotaRuntimeState() *quotaRoutingRuntimeState {
	state := &quotaRoutingRuntimeState{}
	path := s.quotaRuntimeStatePath()
	if path == "" {
		return state
	}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, state) // corrupt state degrades to empty, never crashes routing
	}
	return state
}

func (s *InteractiveService) saveQuotaRuntimeState(state *quotaRoutingRuntimeState) error {
	path := s.quotaRuntimeStatePath()
	if path == "" {
		return os.ErrNotExist
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ClaimAccountForLeg is the manual entry point: the gate's user-picked
// candidate. The automatic path (Task-449) calls claimAccountForLeg with
// automatic=true so the cap + eligibility rules apply.
func (s *InteractiveService) ClaimAccountForLeg(ctx context.Context, demand ExecutionDemand, cand AccountCandidate) (LegBinding, error) {
	return s.claimAccountForLeg(ctx, demand, cand, false)
}

// claimAccountForLeg mints a durable claim + (on a switch) the cooldown
// record, then pins the resident run to the claimed account. Lock order: the
// ledger is written under quotaMu, released BEFORE s.mu is taken for the
// run pin — never hold quotaMu while acquiring s.mu.
func (s *InteractiveService) claimAccountForLeg(ctx context.Context, demand ExecutionDemand, cand AccountCandidate, automatic bool) (LegBinding, error) {
	if cand.ProviderKey != "" && cand.ProviderKey != demand.RequestedProvider {
		return LegBinding{}, fmt.Errorf("quota_preflight: candidate provider %q does not match demand %q", cand.ProviderKey, demand.RequestedProvider)
	}
	// Resolve the run's frozen settings BEFORE taking quotaMu — settings reads
	// s.mu, and the lock order is always s.mu → quotaMu (never the reverse).
	settings := s.quotaSettingsForRun(demand.RunID)
	s.quotaMu.Lock()
	now := s.quotaNow()
	state := s.loadQuotaRuntimeState()

	switching := cand.AccountID != "" && cand.AccountID != demand.RequestedAccountID
	var sw quotaSwitchRecord
	if switching {
		if cd, ok := state.activeSameProviderCooldown(string(demand.RequestedProvider), now); ok {
			s.quotaMu.Unlock()
			return LegBinding{}, &quotaCooldownError{until: cd.Until, reason: cd.Reason}
		}
		if reason := state.accountBlockReason(cand.AccountID); reason != "" {
			s.quotaMu.Unlock()
			return LegBinding{}, fmt.Errorf("quota_preflight: account %q is blocked (%s)", cand.AccountID, quotaBlockRejectionReason(reason))
		}
		if state.accountClaimedByOtherRun(cand.AccountID, demand.RunID) {
			s.quotaMu.Unlock()
			return LegBinding{}, errQuotaAccountClaimed
		}
		if automatic {
			if state.autoSwitchCountForRun(demand.RunID) >= quotaMaxAutoSwitchesPerRun {
				s.quotaMu.Unlock()
				return LegBinding{}, errQuotaAutoSwitchCap
			}
			if cand.Headroom.State != "healthy" || cand.Headroom.Confidence != "exact" {
				s.quotaMu.Unlock()
				return LegBinding{}, fmt.Errorf("quota_preflight: account %q is not auto-eligible (%s)", cand.AccountID, cand.Headroom.State)
			}
		}
		// Re-validate connectivity at claim time — the candidate snapshot may be
		// stale; a disconnected pin must fail closed, not mint a dead binding.
		acct, err := s.resolveConnectedAccount(string(demand.RequestedProvider), cand.AccountID)
		if err != nil {
			s.quotaMu.Unlock()
			return LegBinding{}, err
		}
		_ = acct
		state.supersedeRunClaims(demand.RunID, string(demand.RequestedProvider), now)
		sw = quotaSwitchRecord{
			ProviderKey:   string(demand.RequestedProvider),
			FromAccountID: demand.RequestedAccountID,
			ToAccountID:   cand.AccountID,
			RunID:         demand.RunID,
			Automatic:     automatic,
			Reason:        QuotaCooldownReason,
			StartedAt:     now.Format(time.RFC3339),
			Until:         now.Add(time.Duration(settings.SameProviderCooldownSeconds) * time.Second).Format(time.RFC3339),
		}
		state.Switches = append(state.Switches, sw)
	} else {
		// Same-account re-pin still supersedes stale claims so the ledger holds
		// exactly one active claim per (run, provider).
		state.supersedeRunClaims(demand.RunID, string(demand.RequestedProvider), now)
	}

	claim := QuotaClaim{
		ClaimID:       fmt.Sprintf("qc-%d", now.UnixNano()),
		RunID:         demand.RunID,
		ProviderKey:   string(demand.RequestedProvider),
		AccountID:     cand.AccountID,
		WorkloadClass: string(demand.WorkloadClass),
		Status:        quotaClaimStatusActive,
		Automatic:     automatic,
		ClaimedAt:     now.Format(time.RFC3339),
	}
	state.Claims = append(state.Claims, claim)
	err := s.saveQuotaRuntimeState(state)
	s.quotaMu.Unlock()
	if err != nil {
		return LegBinding{}, fmt.Errorf("quota claim not durable: %w", err)
	}

	// Pin the resident run — this is the claim's execution effect. Non-resident
	// runs (restart pending) re-derive the pin from the ledger on restore.
	s.pinRunAccount(demand.RunID, cand.AccountID, claim.ClaimID)

	return LegBinding{
		RunID:             demand.RunID,
		ProviderKey:       demand.RequestedProvider,
		Model:             demand.RequestedModel,
		AccountID:         cand.AccountID,
		ClaimID:           claim.ClaimID,
		Automatic:         automatic,
		CooldownStartedAt: sw.StartedAt,
		CooldownUntil:     sw.Until,
		CooldownReason:    sw.Reason,
	}, nil
}

// pinRunAccount stamps the claim onto the resident run and persists the
// session snapshot so the pin survives restart. Never called mid-turn —
// claims mint only at admission boundaries (gate answer, spawn, leg mint).
func (s *InteractiveService) pinRunAccount(runID, accountID, claimID string) {
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		return
	}
	rs.providerAccountID = accountID
	rs.accountPinned = true
	rs.quotaClaimID = claimID
	snap := sessionStateOf(rs)
	s.mu.Unlock()
	_ = s.persistProviderSession(snap)
}

// resolveConnectedAccount re-validates that accountID still exists and is
// connected — the fail-closed check shared by claim + pinned admission.
func (s *InteractiveService) resolveConnectedAccount(providerKey, accountID string) (ProviderAccount, error) {
	accounts, err := s.listProviderAccounts()
	if err != nil {
		return ProviderAccount{}, err
	}
	for _, a := range accounts {
		if a.ProviderKey == providerKey && a.ID == accountID {
			if a.AuthStatus != "connected" {
				return ProviderAccount{}, fmt.Errorf("account %q for provider %q is not connected", accountID, providerKey)
			}
			return a, nil
		}
	}
	return ProviderAccount{}, fmt.Errorf("account %q for provider %q not found", accountID, providerKey)
}

// noteAccountBlockedLocked records a live-observed hard limit. Called under
// s.mu from the provider_limit emit path; acquires quotaMu inside (the only
// permitted nesting order — claim never holds quotaMu into s.mu).
func (s *InteractiveService) noteAccountBlockedLocked(providerKey, accountID, reason string) {
	if accountID == "" {
		return
	}
	s.quotaMu.Lock()
	defer s.quotaMu.Unlock()
	state := s.loadQuotaRuntimeState()
	for _, b := range state.Blocked {
		if b.AccountID == accountID {
			return // already recorded — keep the first observation
		}
	}
	state.Blocked = append(state.Blocked, quotaBlockedAccount{
		ProviderKey: providerKey,
		AccountID:   accountID,
		Reason:      reason,
		At:          s.quotaNow().Format(time.RFC3339),
	})
	_ = s.saveQuotaRuntimeState(state)
}
