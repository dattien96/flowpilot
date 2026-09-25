package runner

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// Quota preflight (Task-447 / CP-87 P-3): resolves the normalized execution
// demand at a routing boundary (hub turn or spawned child) and ranks the
// connected accounts of the demand's provider by normalized headroom. This
// file is deliberately decision-only: it reads run state + telemetry and
// returns candidates; the durable side effects live in quota_claim.go.

// ExecutionDemand is the router's normalized view of one execution boundary.
// A nil FlowNode resolves the hub's own pinned leg; a non-nil node resolves
// the child that flow node would spawn, using the same model precedence as
// the executor (resolveFlowNodeModel — Task-320: node step-configured model
// > configured flow/role model > agent definition > parent inheritance).
type ExecutionDemand struct {
	RunID     string `json:"runId"`
	NodeID    string `json:"nodeId,omitempty"`
	IsMainHub bool   `json:"isMainHub,omitempty"`
	// RequestedProvider/Model/AccountID describe the binding the execution
	// would run under. For a hub demand they are the leg's pinned values —
	// never the machine-global active account, which a pinned leg may
	// legitimately diverge from.
	RequestedProvider  ProviderKey             `json:"requestedProvider"`
	RequestedModel     string                  `json:"requestedModel,omitempty"`
	RequestedAccountID string                  `json:"requestedAccountId,omitempty"`
	WorkloadClass      agentpack.WorkloadClass `json:"workloadClass,omitempty"`
	// MaxUsageTokens is the node's real-usage cap (CP-86 schema on
	// FlowNode); zero for hub demands — the router consumes it when sizing
	// whether a candidate can afford the workload, never as a quota proxy.
	MaxUsageTokens int64 `json:"maxUsageTokens,omitempty"`
	// RequiredCaps are the provider capabilities the demand cannot run
	// without (Task-448): hub chat needs streaming; provider-backed flow
	// nodes additionally need the approval + MCP tool bridge.
	RequiredCaps ProviderCapabilities `json:"requiredCaps,omitempty"`
	// MinContextTokens is the smallest acceptable candidate context window;
	// zero disables the filter. Callers size it from the leg's current
	// context pressure, not from quota.
	MinContextTokens int64 `json:"minContextTokens,omitempty"`
}

// AccountCandidate is one connected account of the demand's provider, ranked
// for the rotation gate. AutoEligible is the only flag the automatic router
// may act on; every demotion carries a machine-readable reason so the gate
// surface (Task-450) renders *why* without parsing strings.
type AccountCandidate struct {
	AccountID        string          `json:"accountId"`
	ProviderKey      ProviderKey     `json:"providerKey"`
	DisplayName      string          `json:"displayName,omitempty"`
	SlotIndex        int             `json:"slotIndex"`
	IsCurrent        bool            `json:"isCurrent,omitempty"`
	Headroom         AccountHeadroom `json:"headroom"`
	AutoEligible     bool            `json:"autoEligible"`
	RejectionReasons []string        `json:"rejectionReasons,omitempty"`
	// Cooldown* mirror the durable same-provider switch window when one is
	// active — the gate renders the countdown from these server stamps.
	CooldownStartedAt string `json:"cooldownStartedAt,omitempty"`
	CooldownUntil     string `json:"cooldownUntil,omitempty"`
	CooldownReason    string `json:"cooldownReason,omitempty"`
}

// Rejection reason codes (stable strings — consumed by the Task-450 gate UI).
const (
	QuotaRejectBillingRequired  = "billing_required"
	QuotaRejectExhaustedQuota   = "exhausted_quota"
	QuotaRejectLowHeadroom      = "low_headroom"
	QuotaRejectUnknownQuota     = "unknown_quota"
	QuotaRejectStaleTelemetry   = "stale_telemetry"
	QuotaRejectAlreadyTried     = "already_tried"
	QuotaRejectAccountClaimed   = "account_claimed"
	QuotaRejectCooldown         = "same_provider_ip_safety"
	QuotaRejectAutoSwitchCap    = "auto_switch_cap"
	QuotaRejectMissingModelBind = "missing_model_binding"
)

// ResolveExecutionDemand builds the demand for runID at this boundary. A nil
// node yields the hub's own leg; a non-nil node yields the child demand the
// executor would spawn for it (same model/provider precedence).
func (s *InteractiveService) ResolveExecutionDemand(ctx context.Context, runID string, node *agentpack.FlowNode) (ExecutionDemand, error) {
	s.mu.Lock()
	rs := s.runs[runID]
	s.mu.Unlock()
	if rs == nil {
		return ExecutionDemand{}, fmt.Errorf("run %q not found", runID)
	}
	if node == nil {
		return ExecutionDemand{
			RunID:              runID,
			IsMainHub:          true,
			RequestedProvider:  rs.providerKey,
			RequestedModel:     rs.modelName,
			RequestedAccountID: rs.providerAccountID,
			RequiredCaps:       ProviderCapabilities{Streaming: true},
		}, nil
	}
	model := strings.TrimSpace(s.resolveFlowNodeModel(ctx, runID, *node))
	provider := ProviderKey("")
	if pk, ok := providerKeyFromModel(model); ok {
		provider = pk
	}
	if provider == "" {
		provider = rs.providerKey
	}
	accountID := ""
	if provider == rs.providerKey {
		// A same-provider child inherits the leg's account binding — including
		// a routing pin — so children never silently re-resolve the active
		// account under a claimed leg.
		accountID = rs.providerAccountID
	}
	demand := ExecutionDemand{
		RunID:              runID,
		NodeID:             node.ID,
		RequestedProvider:  provider,
		RequestedModel:     model,
		RequestedAccountID: accountID,
		WorkloadClass:      node.WorkloadClass,
		// Provider-backed nodes run the agentic tool bridge — a cross-provider
		// candidate must carry streaming + approvals + MCP to serve one.
		RequiredCaps: ProviderCapabilities{Streaming: true, ApprovalEvents: true, Mcp: true},
	}
	// The node's real-usage cap lives on its named context profile (Task-341
	// schema); resolve through the parent's flow definition — the same path
	// the executor's packer uses.
	if name := strings.TrimSpace(node.ContextProfile); name != "" {
		if p, ok := s.flowContextProfilesFor(ctx, runID)[name]; ok {
			demand.MaxUsageTokens = int64(p.MaxUsageTokens)
		}
	}
	return demand, nil
}

// SameProviderCandidates ranks every connected account of the demand's
// provider. Order: auto-eligible first, then headroom desc, freshness asc,
// slot asc. The current leg account is always listed (IsCurrent) but is never
// a rotation target itself.
func (s *InteractiveService) SameProviderCandidates(ctx context.Context, demand ExecutionDemand) ([]AccountCandidate, error) {
	if strings.TrimSpace(string(demand.RequestedProvider)) == "" {
		return nil, fmt.Errorf("quota_preflight: demand has no provider")
	}
	settings := s.quotaSettingsForRun(demand.RunID)
	now := s.quotaNow()
	accounts, err := s.listProviderAccounts()
	if err != nil {
		return nil, err
	}
	state := s.loadQuotaRuntimeState()

	out := make([]AccountCandidate, 0, len(accounts))
	for _, acct := range accounts {
		if acct.ProviderKey != string(demand.RequestedProvider) || acct.AuthStatus != "connected" {
			continue
		}
		cand := AccountCandidate{
			AccountID:   acct.ID,
			ProviderKey: demand.RequestedProvider,
			DisplayName: acct.DisplayName,
			SlotIndex:   acct.SlotIndex,
			IsCurrent:   acct.ID == demand.RequestedAccountID,
		}
		var summary ProviderAccountSummary
		if s.quotaTelemetryFn != nil {
			summary = s.quotaTelemetryFn(ctx, acct)
		}
		cand.Headroom = NormalizeAccountHeadroom(summary, now, settings)
		if state.accountBlocked(acct.ID) {
			cand.RejectionReasons = append(cand.RejectionReasons, QuotaRejectBillingRequired)
		}
		if !cand.IsCurrent && state.accountClaimedByOtherRun(acct.ID, demand.RunID) {
			cand.RejectionReasons = append(cand.RejectionReasons, QuotaRejectAccountClaimed)
		}
		if !cand.IsCurrent && state.runAlreadyTried(demand.RunID, string(demand.RequestedProvider), acct.ID) {
			cand.RejectionReasons = append(cand.RejectionReasons, QuotaRejectAlreadyTried)
		}
		switch cand.Headroom.State {
		case "exhausted":
			cand.RejectionReasons = append(cand.RejectionReasons, QuotaRejectExhaustedQuota)
		case "low":
			cand.RejectionReasons = append(cand.RejectionReasons, QuotaRejectLowHeadroom)
		case "unknown":
			cand.RejectionReasons = append(cand.RejectionReasons, QuotaRejectUnknownQuota)
		case "stale":
			cand.RejectionReasons = append(cand.RejectionReasons, QuotaRejectStaleTelemetry)
		}
		out = append(out, cand)
	}

	// An active same-provider switch window demotes every rotation target
	// (the current account is not a switch and never carries the fields).
	if cd, ok := state.activeSameProviderCooldown(string(demand.RequestedProvider), now); ok {
		for i := range out {
			if out[i].IsCurrent {
				continue
			}
			out[i].CooldownStartedAt = cd.StartedAt
			out[i].CooldownUntil = cd.Until
			out[i].CooldownReason = cd.Reason
		}
	}

	autoCap := state.autoSwitchCountForRun(demand.RunID) >= quotaMaxAutoSwitchesPerRun
	for i := range out {
		c := &out[i]
		if c.IsCurrent {
			continue
		}
		eligible := c.Headroom.State == "healthy" && c.Headroom.Confidence == "exact" &&
			len(c.RejectionReasons) == 0 && c.CooldownUntil == ""
		if eligible && autoCap {
			eligible = false
			c.RejectionReasons = append(c.RejectionReasons, QuotaRejectAutoSwitchCap)
		}
		c.AutoEligible = eligible
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.AutoEligible != b.AutoEligible {
			return a.AutoEligible
		}
		ap, bp := -1, -1
		if a.Headroom.RemainingPercent != nil {
			ap = *a.Headroom.RemainingPercent
		}
		if b.Headroom.RemainingPercent != nil {
			bp = *b.Headroom.RemainingPercent
		}
		if ap != bp {
			return ap > bp
		}
		if a.Headroom.FreshnessSeconds != b.Headroom.FreshnessSeconds {
			return a.Headroom.FreshnessSeconds < b.Headroom.FreshnessSeconds
		}
		return a.SlotIndex < b.SlotIndex
	})
	return out, nil
}

// SetQuotaTelemetry injects the quota-telemetry probe (cli wires
// loadAccountLaunchMetadata through it — the runner can't import cli). nil
// keeps every candidate at unknown headroom → manual-only, fail-closed.
func (s *InteractiveService) SetQuotaTelemetry(fn func(ctx context.Context, account ProviderAccount) ProviderAccountSummary) {
	s.quotaMu.Lock()
	defer s.quotaMu.Unlock()
	s.quotaTelemetryFn = fn
}

// quotaNow is the single clock for routing decisions (tests pin it).
func (s *InteractiveService) quotaNow() time.Time {
	if s.quotaNowFn != nil {
		return s.quotaNowFn().UTC()
	}
	return time.Now().UTC()
}

// quotaSettingsForRun returns the run's frozen routing snapshot when resident;
// runs predating Task-446 (or unresolvable) fall back to the machine-global
// document — normalization keeps both paths fail-closed to manual.
func (s *InteractiveService) quotaSettingsForRun(runID string) QuotaRoutingSettings {
	s.mu.Lock()
	rs := s.runs[runID]
	var snap *QuotaRoutingSnapshot
	if rs != nil {
		snap = rs.quotaRouting
	}
	s.mu.Unlock()
	if snap != nil {
		settings := snap.QuotaRoutingSettings
		normalizeQuotaRoutingSettings(&settings)
		return settings
	}
	return mustLoadQuotaRoutingSettings()
}

// listProviderAccounts reads the machine-local account store; a bare service
// (tests) resolves through a zero-value Runner which still honors
// FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH.
func (s *InteractiveService) listProviderAccounts() ([]ProviderAccount, error) {
	if s.runner != nil {
		return s.runner.ListProviderAccounts()
	}
	return (&Runner{}).ListProviderAccounts()
}
