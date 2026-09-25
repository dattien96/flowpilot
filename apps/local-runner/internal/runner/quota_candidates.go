package runner

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"flowpilot-runner/internal/agentpack"
)

// Cross-provider candidate resolution (Task-448 / CP-87 P-4): when no
// same-provider account qualifies, enumerate OTHER registered providers and
// rank (provider, model, account) candidates. Pure resolver — no claims, no
// leg minting, no store mutation. The router holds zero provider-specific
// knowledge: providers come from the registry, models from the user's
// workload-class bindings, headroom from the shared telemetry seam.

// RouteCandidate is one (provider, model, account) rotation target.
type RouteCandidate struct {
	ProviderKey   ProviderKey             `json:"providerKey"`
	Model         string                  `json:"model,omitempty"`
	AccountID     string                  `json:"accountId"`
	DisplayName   string                  `json:"displayName,omitempty"`
	SlotIndex     int                     `json:"slotIndex"`
	WorkloadClass agentpack.WorkloadClass `json:"workloadClass,omitempty"`
	Headroom      AccountHeadroom         `json:"headroom"`
	AutoEligible  bool                    `json:"autoEligible"`
	// Score is the deterministic rank scalar: configured provider priority
	// dominates, then normalized headroom, then slot. Informational — sort
	// order is produced by RankRouteCandidates, not by trusting the number.
	Score            int      `json:"score"`
	RejectionReasons []string `json:"rejectionReasons,omitempty"`
}

// CandidateSet partitions the cross-provider enumeration exactly once — the
// manual gate and the auto path consume the same result (T-5).
type CandidateSet struct {
	Eligible   []RouteCandidate `json:"eligible"`
	ManualOnly []RouteCandidate `json:"manualOnly"`
	Rejected   []RouteCandidate `json:"rejected"`
}

// Cross-provider rejection reasons (same-provider codes live in
// quota_preflight.go).
const (
	QuotaRejectProviderUnavailable = "provider_unavailable"
	QuotaRejectNoConnectedAccount  = "no_connected_account"
	QuotaRejectContextWindowSmall  = "context_window_too_small"
	QuotaRejectContextWindowUnknwn = "context_window_unknown"
	quotaRejectMissingCapability   = "missing_capability:" // + flag name
)

// CrossProviderCandidates enumerates every registered provider except the
// demand's current one and evaluates its accounts against the demand's class,
// capability, and context requirements (T-1 filter, T-2 rank).
func (s *InteractiveService) CrossProviderCandidates(ctx context.Context, demand ExecutionDemand, settings QuotaRoutingSettings) (CandidateSet, error) {
	if s.registry == nil {
		return CandidateSet{}, fmt.Errorf("quota_preflight: no provider registry")
	}
	normalizeQuotaRoutingSettings(&settings)
	now := s.quotaNow()
	accounts, err := s.listProviderAccounts()
	if err != nil {
		return CandidateSet{}, err
	}
	state := s.loadQuotaRuntimeState()

	set := CandidateSet{}
	for _, reg := range s.registry.List() {
		if reg.Key == demand.RequestedProvider {
			continue // same-provider space is Task-447's SameProviderCandidates
		}
		// T-1 filter 1: provider must be usable at all.
		if reg.Status != ProviderStatusAvailable ||
			(reg.newAdapter == nil && reg.newAdapterForTurn == nil && reg.newAdapterForAccount == nil) {
			set.Rejected = append(set.Rejected, RouteCandidate{
				ProviderKey: reg.Key, WorkloadClass: demand.WorkloadClass,
				RejectionReasons: []string{QuotaRejectProviderUnavailable},
			})
			continue
		}
		// T-1 filter 2: capabilities the demand needs (tool bridge etc.).
		if missing := missingRequiredCaps(reg.Capabilities, demand.RequiredCaps); len(missing) > 0 {
			set.Rejected = append(set.Rejected, RouteCandidate{
				ProviderKey: reg.Key, WorkloadClass: demand.WorkloadClass,
				RejectionReasons: missing,
			})
			continue
		}
		// T-2: explicit (provider, class) binding picks the model; a missing
		// binding keeps the provider's default model but demotes to manual.
		boundModel, bound := modelBindingFor(settings.ModelBindings, reg.Key, demand.WorkloadClass)
		model := boundModel
		if !bound {
			model = defaultModelForProvider(reg.Key)
		}
		// T-1 filter 3: context window must provably fit the demand.
		ctxUnknown := false
		if demand.MinContextTokens > 0 {
			if window := modelContextWindowFor(reg.Key, model); window == nil {
				ctxUnknown = true // unknown size is manual-only, never a hard reject
			} else if *window < demand.MinContextTokens {
				set.Rejected = append(set.Rejected, RouteCandidate{
					ProviderKey: reg.Key, Model: model, WorkloadClass: demand.WorkloadClass,
					RejectionReasons: []string{QuotaRejectContextWindowSmall},
				})
				continue
			}
		}
		// T-1 filter 4: at least one connected account.
		foundAccount := false
		for _, acct := range accounts {
			if acct.ProviderKey != string(reg.Key) || acct.AuthStatus != "connected" {
				continue
			}
			foundAccount = true
			cand := RouteCandidate{
				ProviderKey:   reg.Key,
				Model:         model,
				AccountID:     acct.ID,
				DisplayName:   acct.DisplayName,
				SlotIndex:     acct.SlotIndex,
				WorkloadClass: demand.WorkloadClass,
			}
			var summary ProviderAccountSummary
			if s.quotaTelemetryFn != nil {
				summary = s.quotaTelemetryFn(ctx, acct)
			}
			cand.Headroom = NormalizeAccountHeadroom(summary, now, settings)
			// Hard exclusions — unusable accounts are Rejected outright.
			hard := false
			if reason := state.accountBlockReason(acct.ID); reason != "" {
				cand.RejectionReasons = append(cand.RejectionReasons, quotaBlockRejectionReason(reason))
				hard = true
			}
			if cand.Headroom.State == "exhausted" {
				cand.RejectionReasons = append(cand.RejectionReasons, QuotaRejectExhaustedQuota)
				hard = true
			}
			// Soft demotions — usable but never auto-picked.
			if !bound {
				cand.RejectionReasons = append(cand.RejectionReasons, QuotaRejectMissingModelBind)
			}
			if ctxUnknown {
				cand.RejectionReasons = append(cand.RejectionReasons, QuotaRejectContextWindowUnknwn)
			}
			if state.accountClaimedByOtherRun(acct.ID, demand.RunID) {
				cand.RejectionReasons = append(cand.RejectionReasons, QuotaRejectAccountClaimed)
			}
			if state.runAlreadyTried(demand.RunID, string(reg.Key), acct.ID) {
				cand.RejectionReasons = append(cand.RejectionReasons, QuotaRejectAlreadyTried)
			}
			switch cand.Headroom.State {
			case "low":
				cand.RejectionReasons = append(cand.RejectionReasons, QuotaRejectLowHeadroom)
			case "unknown":
				cand.RejectionReasons = append(cand.RejectionReasons, QuotaRejectUnknownQuota)
			case "stale":
				cand.RejectionReasons = append(cand.RejectionReasons, QuotaRejectStaleTelemetry)
			}
			if hard {
				set.Rejected = append(set.Rejected, cand)
				continue
			}
			// T-3: auto requires an exact class binding AND a healthy account
			// backed by exact, fresh provider evidence.
			cand.AutoEligible = len(cand.RejectionReasons) == 0 &&
				cand.Headroom.State == "healthy" && cand.Headroom.Confidence == "exact"
			if cand.AutoEligible {
				set.Eligible = append(set.Eligible, cand)
			} else {
				set.ManualOnly = append(set.ManualOnly, cand)
			}
		}
		if !foundAccount {
			set.Rejected = append(set.Rejected, RouteCandidate{
				ProviderKey: reg.Key, Model: model, WorkloadClass: demand.WorkloadClass,
				RejectionReasons: []string{QuotaRejectNoConnectedAccount},
			})
		}
	}
	set.Eligible = RankRouteCandidates(set.Eligible, settings.ProviderPriority)
	set.ManualOnly = RankRouteCandidates(set.ManualOnly, settings.ProviderPriority)
	sort.SliceStable(set.Rejected, func(i, j int) bool {
		if set.Rejected[i].ProviderKey != set.Rejected[j].ProviderKey {
			return set.Rejected[i].ProviderKey < set.Rejected[j].ProviderKey
		}
		return set.Rejected[i].AccountID < set.Rejected[j].AccountID
	})
	return set, nil
}

// RankRouteCandidates returns candidates in deterministic order: configured
// provider priority first, then headroom desc, freshness asc, slot asc — with
// provider/model/account ids as final stable tie-breaks.
func RankRouteCandidates(candidates []RouteCandidate, priority []ProviderKey) []RouteCandidate {
	out := append([]RouteCandidate(nil), candidates...)
	for i := range out {
		out[i].Score = routeScore(out[i], priority)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Headroom.FreshnessSeconds != b.Headroom.FreshnessSeconds {
			return a.Headroom.FreshnessSeconds < b.Headroom.FreshnessSeconds
		}
		if a.SlotIndex != b.SlotIndex {
			return a.SlotIndex < b.SlotIndex
		}
		if a.ProviderKey != b.ProviderKey {
			return a.ProviderKey < b.ProviderKey
		}
		if a.Model != b.Model {
			return a.Model < b.Model
		}
		return a.AccountID < b.AccountID
	})
	return out
}

func routeScore(c RouteCandidate, priority []ProviderKey) int {
	weight := 0 // unlisted providers share weight 0, registry order is the tiebreak
	for i, p := range priority {
		if p == c.ProviderKey {
			weight = len(priority) - i
			break
		}
	}
	pct := 0
	if c.Headroom.RemainingPercent != nil {
		pct = *c.Headroom.RemainingPercent
	}
	return weight*1000 + pct
}

// modelBindingFor resolves the (provider, workloadClass) → model binding —
// the only quality mapping the router consults (user/catalog data, T-2).
func modelBindingFor(bindings []ModelClassBinding, provider ProviderKey, class agentpack.WorkloadClass) (string, bool) {
	for _, b := range bindings {
		if b.ProviderKey == provider && b.WorkloadClass == class && strings.TrimSpace(b.Model) != "" {
			return strings.TrimSpace(b.Model), true
		}
	}
	return "", false
}

// missingRequiredCaps returns one missing_capability:<flag> reason per
// capability the demand requires but the registration lacks.
func missingRequiredCaps(have, need ProviderCapabilities) []string {
	var out []string
	if need.Streaming && !have.Streaming {
		out = append(out, quotaRejectMissingCapability+"streaming")
	}
	if need.Resume && !have.Resume {
		out = append(out, quotaRejectMissingCapability+"resume")
	}
	if need.ApprovalEvents && !have.ApprovalEvents {
		out = append(out, quotaRejectMissingCapability+"approvalEvents")
	}
	if need.FileEvents && !have.FileEvents {
		out = append(out, quotaRejectMissingCapability+"fileEvents")
	}
	if need.SkillSelection && !have.SkillSelection {
		out = append(out, quotaRejectMissingCapability+"skillSelection")
	}
	if need.Mcp && !have.Mcp {
		out = append(out, quotaRejectMissingCapability+"mcp")
	}
	if need.Interrupt && !have.Interrupt {
		out = append(out, quotaRejectMissingCapability+"interrupt")
	}
	if need.Vision && !have.Vision {
		out = append(out, quotaRejectMissingCapability+"vision")
	}
	return out
}
