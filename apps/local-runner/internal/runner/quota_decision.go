package runner

import (
	"context"
	"strings"
)

// Quota decision DTOs (Task-450 / CP-87 P-6): the manual gate card carries a
// structured candidate table so Desktop/TUI render the runner's ranking and
// rejection reasons verbatim — clients never re-rank or re-parse labels. The
// record persists on ProviderQuestionState so restart/replay/device switch
// rehydrate the identical decision surface.

// QuotaRouteCandidateDTO is one row of the gate table: identity (provider,
// model, account), the normalized headroom evidence, confidence/eligibility,
// the machine-readable rejection reasons, and the same-provider cooldown
// window when one is open for this provider.
type QuotaRouteCandidateDTO struct {
	ProviderKey      ProviderKey     `json:"providerKey"`
	Model            string          `json:"model,omitempty"`
	WorkloadClass    string          `json:"workloadClass,omitempty"`
	AccountID        string          `json:"accountId"`
	DisplayName      string          `json:"displayName,omitempty"`
	SlotIndex        int             `json:"slotIndex,omitempty"`
	Headroom         AccountHeadroom `json:"headroom"`
	AutoEligible     bool            `json:"autoEligible"`
	RejectionReasons []string        `json:"rejectionReasons,omitempty"`
	// Cooldown* mirror the durable switch window (server timestamps — the
	// client counts down to Until and never re-bases it).
	CooldownStartedAt string `json:"cooldownStartedAt,omitempty"`
	CooldownUntil     string `json:"cooldownUntil,omitempty"`
	CooldownReason    string `json:"cooldownReason,omitempty"`
}

// QuotaRouteDecision is the durable payload behind a quota_route_required
// card: the unusable binding, the trigger, and every candidate the resolver
// surfaced — Eligible/ManualOnly first (runner order preserved), then
// Rejected rows so the table explains why each was excluded.
type QuotaRouteDecision struct {
	RunID         string                   `json:"runId"`
	Trigger       string                   `json:"trigger"`
	Reason        string                   `json:"reason,omitempty"`
	ProviderKey   ProviderKey              `json:"providerKey"`
	Model         string                   `json:"model,omitempty"`
	AccountID     string                   `json:"accountId,omitempty"`
	PolicyVersion int                      `json:"policyVersion"`
	Candidates    []QuotaRouteCandidateDTO `json:"candidates,omitempty"`
}

// buildQuotaRouteDecision projects a QuotaResolution into the render DTO.
// Runner order is contractual — same-provider accounts (ranked) then the
// cross-provider eligible/manual-only/rejected sets, each already ordered.
func buildQuotaRouteDecision(res QuotaResolution) *QuotaRouteDecision {
	d := res.Demand
	dec := &QuotaRouteDecision{
		RunID:         res.Demand.RunID,
		Trigger:       res.Reason,
		Reason:        res.Reason,
		ProviderKey:   d.RequestedProvider,
		Model:         d.RequestedModel,
		AccountID:     d.RequestedAccountID,
		PolicyVersion: res.PolicyVersion,
	}
	seen := map[string]bool{}
	appendCand := func(c QuotaRouteCandidateDTO) {
		key := string(c.ProviderKey) + "|" + c.AccountID + "|" + c.Model
		if seen[key] {
			return
		}
		seen[key] = true
		dec.Candidates = append(dec.Candidates, c)
	}
	for _, a := range res.Accounts {
		if a.AccountID == d.RequestedAccountID && !hasHardReject(a.RejectionReasons) {
			continue // the healthy current binding is not a route candidate
		}
		appendCand(QuotaRouteCandidateDTO{
			ProviderKey: d.RequestedProvider, Model: d.RequestedModel,
			WorkloadClass: string(d.WorkloadClass),
			AccountID:     a.AccountID, DisplayName: a.DisplayName, SlotIndex: a.SlotIndex,
			Headroom: a.Headroom, AutoEligible: a.AutoEligible,
			RejectionReasons:  a.RejectionReasons,
			CooldownStartedAt: a.CooldownStartedAt, CooldownUntil: a.CooldownUntil,
			CooldownReason: a.CooldownReason,
		})
	}
	appendRoute := func(c RouteCandidate) {
		// Cross-provider candidates never carry same-provider cooldown stamps —
		// the IP-safety window is scoped to one provider's accounts.
		appendCand(QuotaRouteCandidateDTO{
			ProviderKey: c.ProviderKey, Model: c.Model,
			WorkloadClass: string(c.WorkloadClass),
			AccountID:     c.AccountID, DisplayName: c.DisplayName, SlotIndex: c.SlotIndex,
			Headroom: c.Headroom, AutoEligible: c.AutoEligible,
			RejectionReasons: c.RejectionReasons,
		})
	}
	for _, c := range res.Candidates.Eligible {
		appendRoute(c)
	}
	for _, c := range res.Candidates.ManualOnly {
		appendRoute(c)
	}
	for _, c := range res.Candidates.Rejected {
		appendRoute(c)
	}
	return dec
}

// QuotaRoutingAuditRecord is the forensic record for one routing decision:
// the requested binding, the resolved binding, why, the policy version, the
// headroom evidence the decision consumed, and the CP-86 usage figures
// (estimated prompt, node max-usage cap, actual provider usage) so the audit
// drawer can answer "why did execution move and what did it cost".
type QuotaRoutingAuditRecord struct {
	RunID           string               `json:"runId"`
	CommittedAt     string               `json:"committedAt,omitempty"`
	Outcome         string               `json:"outcome"` // committed | stopped | blocked
	FromProvider    ProviderKey          `json:"fromProvider,omitempty"`
	FromAccount     string               `json:"fromAccount,omitempty"`
	ToProvider      ProviderKey          `json:"toProvider,omitempty"`
	ToAccount       string               `json:"toAccount,omitempty"`
	ToModel         string               `json:"toModel,omitempty"`
	Scope           string               `json:"scope,omitempty"`
	Reason          string               `json:"reason,omitempty"`
	PolicyVersion   int                  `json:"policyVersion"`
	Headroom        *AccountHeadroom     `json:"headroom,omitempty"`
	EstPromptTokens *int64               `json:"estPromptTokens,omitempty"`
	MaxUsageTokens  *int64               `json:"maxUsageTokens,omitempty"`
	ActualUsage     *TokenUsageBreakdown `json:"actualUsage,omitempty"`
}

// quotaAuditForRun assembles the audit record from the run's durable event
// stream: the most recent quota_route_* terminal record (committed wins over
// stopped/blocked within one decision), then usage correlation from the
// run's last token_usage event and its demand snapshot.
func (s *InteractiveService) quotaAuditForRun(ctx context.Context, runID string) (*QuotaRoutingAuditRecord, error) {
	s.mu.Lock()
	rs := s.runs[runID]
	s.mu.Unlock()
	if rs == nil {
		return nil, &apiErr{status: 404, code: "not_found", msg: "run not found"}
	}
	rec := &QuotaRoutingAuditRecord{RunID: runID, PolicyVersion: QuotaRoutingPolicyVersion}
	var usage *TokenUsageBreakdown
	s.mu.Lock()
	events := make([]ProviderEvent, len(rs.events))
	copy(events, rs.events)
	estPrompt := rs.lastPrompt
	s.mu.Unlock()
	for _, ev := range events {
		switch ev.Type {
		case EventQuotaRouteCommitted:
			if ev.QuotaRoute == nil {
				continue
			}
			q := ev.QuotaRoute
			rec.Outcome = "committed"
			rec.CommittedAt = ev.OccurredAt
			rec.FromProvider, rec.FromAccount = q.FromProvider, q.FromAccount
			rec.ToProvider, rec.ToAccount, rec.ToModel = q.ToProvider, q.ToAccount, q.ToModel
			rec.Scope, rec.Reason, rec.PolicyVersion = q.Scope, q.Reason, q.PolicyVersion
			rec.Headroom = q.Headroom
		case EventQuotaRouteStopped:
			if rec.Outcome == "" {
				rec.Outcome = "stopped"
				rec.CommittedAt = ev.OccurredAt
				if ev.QuotaRoute != nil {
					rec.FromProvider, rec.FromAccount = ev.QuotaRoute.FromProvider, ev.QuotaRoute.FromAccount
					rec.Reason = ev.QuotaRoute.Reason
				}
			}
		case EventQuotaRouteBlocked:
			if rec.Outcome == "" {
				rec.Outcome = "blocked"
				rec.CommittedAt = ev.OccurredAt
				if ev.QuotaRoute != nil {
					rec.FromProvider, rec.FromAccount = ev.QuotaRoute.FromProvider, ev.QuotaRoute.FromAccount
					rec.Reason = ev.QuotaRoute.Reason
				}
			}
		case EventTokenUsageUpdated:
			if ev.TokenUsage != nil && ev.TokenUsage.Total != nil {
				usage = ev.TokenUsage.Total
				if ev.TokenUsage.EstPromptTokens != nil {
					rec.EstPromptTokens = ev.TokenUsage.EstPromptTokens
				}
			}
		}
	}
	rec.ActualUsage = usage
	if rec.EstPromptTokens == nil && estPrompt != "" {
		est := int64(len(estPrompt)) / 4
		rec.EstPromptTokens = &est
	}
	if node, ok := s.flowNodeForRun(rs); ok {
		if profiles := s.flowContextProfilesFor(ctx, rs.parentRunID); len(profiles) > 0 {
			if p, ok := profiles[node.ContextProfile]; ok && p.MaxUsageTokens > 0 {
				mt := int64(p.MaxUsageTokens)
				rec.MaxUsageTokens = &mt
			}
		}
	}
	if rec.Outcome == "" {
		rec.Outcome = "none"
	}
	return rec, nil
}

// quotaDecisionForRecord extracts the persisted decision off a rehydrated
// question record; nil for non-quota cards.
func quotaDecisionForRecord(rec *questionRecord) *QuotaRouteDecision {
	if rec == nil {
		return nil
	}
	if questionRecordKind(rec) != quotaRouteQuestionKind {
		return nil
	}
	return rec.quotaDecision
}

// quotaTriggerLabel normalizes the audit trigger for display — the card
// prompt already embeds the reason; empty collapses to the generic kind.
func quotaTriggerLabel(trigger string) string {
	if strings.TrimSpace(trigger) == "" {
		return quotaRouteQuestionKind
	}
	return trigger
}
