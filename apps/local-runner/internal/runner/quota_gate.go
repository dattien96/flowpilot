package runner

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// Quota gate (Task-449 / CP-87 P-5/P-7): one admission path decides what a
// provider-backed turn does when its binding is provably unusable — proceed,
// park on a durable quota_route_required decision card, auto-rotate onto an
// exact candidate, or block. Manual is the default; auto commits only
// high-confidence candidates. Every routing change mints a claim or a new
// leg — never a live-session migration, never a global active-account write.

// QuotaPreflightOutcome is the resolver verdict the execution path acts on.
type QuotaPreflightOutcome string

const (
	QuotaProceed QuotaPreflightOutcome = "proceed"
	QuotaGate    QuotaPreflightOutcome = "gate"
	QuotaRotate  QuotaPreflightOutcome = "rotate"
	QuotaBlocked QuotaPreflightOutcome = "blocked"
)

// quotaRouteQuestionKind marks engine-emitted quota decision cards so
// AnswerQuestion routes the resolved choice to applyQuotaRouteAnswer — same
// pattern as usage_budget_exceeded / context_pressure cards.
const quotaRouteQuestionKind = "quota_route_required"

// Typed route events — quota_route_committed is the T-3 notice (requested →
// resolved binding), quota_route_stopped the structured stop outcome, and
// quota_route_blocked the fail-closed "no candidate" record.
const (
	EventQuotaRouteCommitted ProviderEventType = "quota_route_committed"
	EventQuotaRouteStopped   ProviderEventType = "quota_route_stopped"
	EventQuotaRouteBlocked   ProviderEventType = "quota_route_blocked"
)

// QuotaRoutePayload carries the route decision on the typed events above.
type QuotaRoutePayload struct {
	FromProvider ProviderKey `json:"fromProvider,omitempty"`
	FromAccount  string      `json:"fromAccount,omitempty"`
	ToProvider   ProviderKey `json:"toProvider,omitempty"`
	ToAccount    string      `json:"toAccount,omitempty"`
	ToModel      string      `json:"toModel,omitempty"`
	Scope        string      `json:"scope,omitempty"` // once | run
	Reason       string      `json:"reason,omitempty"`
	// Task-450 audit fields: the policy that produced the route, the headroom
	// evidence on the selected candidate, and the same-provider cooldown
	// window when the claim minted one.
	PolicyVersion     int              `json:"policyVersion,omitempty"`
	Headroom          *AccountHeadroom `json:"headroom,omitempty"`
	CooldownStartedAt string           `json:"cooldownStartedAt,omitempty"`
	CooldownUntil     string           `json:"cooldownUntil,omitempty"`
	CooldownReason    string           `json:"cooldownReason,omitempty"`
}

// QuotaResolution is the durable decision produced by ResolveQuotaPreflight
// and consumed by CommitQuotaResolution — one contract for manual + auto.
type QuotaResolution struct {
	Outcome QuotaPreflightOutcome `json:"outcome"`
	Demand  ExecutionDemand       `json:"demand"`
	// Accounts are the same-provider candidates (Task-447 ranking, current
	// account flagged); Candidates is the cross-provider set (Task-448).
	Accounts   []AccountCandidate `json:"accounts,omitempty"`
	Candidates CandidateSet       `json:"candidates"`
	// Selected is set only for QuotaRotate — the committed route.
	Selected *RouteCandidate `json:"selected,omitempty"`
	Reason   string          `json:"reason,omitempty"`
	// Scope is "once" or "run" for user-chosen routes (empty for auto) — it
	// lands on the committed payload so the audit trail records the choice's
	// intended breadth.
	Scope         string `json:"scope,omitempty"`
	PolicyVersion int    `json:"policyVersion"`
	// PendingPrompt is an in-memory carrier (BUG-517): the turn whose
	// admission triggered routing never reached dispatch, so a flow-child
	// respawn must seed THIS prompt — not the child's stale lastPrompt.
	PendingPrompt string `json:"-"`
}

// hardQuotaRejects mark an account unusable — the current binding failing one
// of these is what moves a demand past "proceed". Soft states (unknown/stale/
// low headroom, cooldown) never gate a binding already in place.
var hardQuotaRejects = map[string]bool{
	QuotaRejectBillingRequired: true,
	QuotaRejectExhaustedQuota:  true,
	QuotaRejectAccountClaimed:  true,
	QuotaRejectNoConnected:     true,
}

// QuotaRejectNoConnected covers a demand whose requested account vanished
// from the connected set entirely.
const QuotaRejectNoConnected = "account_disconnected"

func hasHardReject(reasons []string) bool {
	for _, r := range reasons {
		if hardQuotaRejects[r] {
			return true
		}
	}
	return false
}

// ResolveQuotaPreflight evaluates one execution demand: proceed when the
// current binding is usable; otherwise pick a route — auto mode rotates an
// exact/high-confidence candidate, everything else parks at the gate.
func (s *InteractiveService) ResolveQuotaPreflight(ctx context.Context, demand ExecutionDemand) (QuotaResolution, error) {
	settings := s.quotaSettingsForRun(demand.RunID)
	res := QuotaResolution{
		Demand:        demand,
		PolicyVersion: QuotaRoutingPolicyVersion,
	}

	accounts, err := s.SameProviderCandidates(ctx, demand)
	if err != nil {
		return res, err
	}
	res.Accounts = accounts

	var current *AccountCandidate
	for i := range accounts {
		c := &accounts[i]
		if c.IsCurrent || (demand.RequestedAccountID != "" && c.AccountID == demand.RequestedAccountID) {
			current = c
			break
		}
	}
	if current != nil && !hasHardReject(current.RejectionReasons) {
		res.Outcome = QuotaProceed
		return res, nil
	}
	if current == nil {
		res.Reason = QuotaRejectNoConnected
	}

	// Current binding unusable — collect alternates. Same-provider accounts
	// become route candidates on the demand's own provider+model binding.
	var alternates []RouteCandidate
	for _, a := range accounts {
		if current != nil && a.AccountID == current.AccountID {
			continue
		}
		if hasHardReject(a.RejectionReasons) {
			continue
		}
		alternates = append(alternates, RouteCandidate{
			ProviderKey:      demand.RequestedProvider,
			Model:            demand.RequestedModel,
			AccountID:        a.AccountID,
			DisplayName:      a.DisplayName,
			SlotIndex:        a.SlotIndex,
			WorkloadClass:    demand.WorkloadClass,
			Headroom:         a.Headroom,
			AutoEligible:     a.AutoEligible,
			RejectionReasons: a.RejectionReasons,
		})
	}
	set, err := s.CrossProviderCandidates(ctx, demand, settings)
	if err != nil {
		return res, err
	}
	res.Candidates = set
	alternates = append(alternates, set.Eligible...)
	alternates = append(alternates, set.ManualOnly...)

	if len(alternates) == 0 {
		res.Outcome = QuotaBlocked
		if res.Reason == "" {
			res.Reason = "no_candidates"
		}
		return res, nil
	}

	if settings.Mode == QuotaRotationAuto {
		// Same-provider auto-eligible first (no provider migration), then the
		// ranked cross-provider eligible set. Uncertain inputs always gate.
		for i := range alternates {
			if alternates[i].ProviderKey == demand.RequestedProvider && alternates[i].AutoEligible {
				sel := alternates[i]
				res.Selected = &sel
				res.Outcome = QuotaRotate
				return res, nil
			}
		}
		for i := range set.Eligible {
			sel := set.Eligible[i]
			res.Selected = &sel
			res.Outcome = QuotaRotate
			return res, nil
		}
	}
	res.Outcome = QuotaGate
	return res, nil
}

// CommitQuotaResolution makes the resolution durable and performs its
// execution effect: proceed is a no-op; gate emits the durable decision card;
// rotate commits the selected binding (claim or new leg) with a typed notice;
// blocked emits the structured no-candidate record.
func (s *InteractiveService) CommitQuotaResolution(ctx context.Context, res QuotaResolution) error {
	switch res.Outcome {
	case QuotaProceed:
		return nil
	case QuotaBlocked:
		s.mu.Lock()
		rs := s.runs[res.Demand.RunID]
		if rs != nil {
			s.emitLocked(rs, ProviderEvent{
				Type: EventQuotaRouteBlocked,
				QuotaRoute: &QuotaRoutePayload{
					FromProvider: res.Demand.RequestedProvider,
					FromAccount:  res.Demand.RequestedAccountID,
					Reason:       res.Reason,
				},
			})
		}
		s.mu.Unlock()
		return nil
	case QuotaGate:
		s.mu.Lock()
		rs := s.runs[res.Demand.RunID]
		s.mu.Unlock()
		if rs == nil {
			return fmt.Errorf("quota_gate: run %q not found", res.Demand.RunID)
		}
		s.emitQuotaRouteCard(rs, res)
		return nil
	case QuotaRotate:
		if res.Selected == nil {
			return fmt.Errorf("quota_gate: rotate resolution has no selected candidate")
		}
		return s.commitQuotaRotation(ctx, res)
	}
	return fmt.Errorf("quota_gate: unknown outcome %q", res.Outcome)
}

// commitQuotaRotation applies a committed route: same-provider re-pins via the
// durable claim ledger; cross-provider mints a new leg — chat legs through the
// handoff machinery, flow children through a replacement spawn. The committed
// route is persisted (claim / closed-leg lineage) before any provider call.
func (s *InteractiveService) commitQuotaRotation(ctx context.Context, res QuotaResolution) error {
	sel := res.Selected
	d := res.Demand
	if sel.ProviderKey == d.RequestedProvider {
		// Same-provider account switch — durable claim repins this leg. The
		// committed notice emits after a successful claim so it carries the
		// minted cooldown window; a failed claim leaves no committed record.
		binding, err := s.claimAccountForLeg(ctx, d, AccountCandidate{
			AccountID:   sel.AccountID,
			ProviderKey: sel.ProviderKey,
			Headroom:    sel.Headroom,
		}, res.Reason != "user_choice")
		if err != nil {
			return err
		}
		s.emitQuotaRouteCommitted(res, &binding)
		return nil
	}
	s.mu.Lock()
	rs := s.runs[d.RunID]
	s.mu.Unlock()
	if rs == nil {
		return fmt.Errorf("quota_gate: run %q not found", d.RunID)
	}
	if rs.parentRunID == "" && rs.chatID != "" {
		// Hub/chat leg (incl. vibe): the leg machinery owns provider switches —
		// new leg + compact handoff, old leg closed durable. Notice lands on the
		// source leg's durable stream before the switch closes it.
		s.emitQuotaRouteCommitted(res, nil)
		_, aerr := s.switchChatLeg(ctx, rs.chatID, chatSwitchRequest{
			TargetProviderKey: sel.ProviderKey,
			Model:             sel.Model,
			ProviderAccountID: sel.AccountID,
		}, false)
		if aerr != nil {
			return fmt.Errorf("quota_gate: %s", aerr.msg)
		}
		return nil
	}
	if rs.parentRunID != "" {
		// Flow child: the leg is the child run — close it and respawn the same
		// node binding on the new provider/account.
		s.emitQuotaRouteCommitted(res, nil)
		return s.respawnChildOnRoute(ctx, rs, sel, res.PendingPrompt)
	}
	return fmt.Errorf("quota_gate: run %q has no leg rotation path", d.RunID)
}

// emitQuotaRouteCommitted publishes the T-3 requested→resolved notice on the
// routed run's durable stream — with policy version, the selected headroom
// evidence, and the minted cooldown window when the claim produced one.
func (s *InteractiveService) emitQuotaRouteCommitted(res QuotaResolution, binding *LegBinding) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[res.Demand.RunID]
	if rs == nil || res.Selected == nil {
		return
	}
	payload := &QuotaRoutePayload{
		FromProvider: res.Demand.RequestedProvider,
		FromAccount:  res.Demand.RequestedAccountID,
		ToProvider:   res.Selected.ProviderKey,
		ToAccount:    res.Selected.AccountID,
		ToModel:      res.Selected.Model,
		Scope:        res.Scope,
		Reason:       res.Reason,
	}
	payload.PolicyVersion = res.PolicyVersion
	if res.Selected.Headroom.State != "" {
		h := res.Selected.Headroom
		payload.Headroom = &h
	}
	if binding != nil {
		payload.CooldownStartedAt = binding.CooldownStartedAt
		payload.CooldownUntil = binding.CooldownUntil
		payload.CooldownReason = binding.CooldownReason
	}
	s.emitLocked(rs, ProviderEvent{
		Type:       EventQuotaRouteCommitted,
		QuotaRoute: payload,
	})
	// Observability: the committed repin/switch must be visible in runner
	// logs, not only the durable event stream (Grok review round 3).
	log.Printf("[quota] route_committed run=%s %s/%s -> %s/%s scope=%s reason=%q",
		res.Demand.RunID, res.Demand.RequestedProvider, res.Demand.RequestedAccountID,
		res.Selected.ProviderKey, res.Selected.AccountID, res.Scope, res.Reason)
}

// respawnChildOnRoute closes the child's exhausted leg and spawns a
// replacement run on the committed binding — the flow engine sees a fresh
// child for the same node label. pendingPrompt is the turn that admission
// refused (BUG-517): it never dispatched, so the replacement must start
// from it — falling back to the child's full prompt, never the 100-char
// display-truncated lastPrompt.
func (s *InteractiveService) respawnChildOnRoute(ctx context.Context, child *interactiveRun, sel *RouteCandidate, pendingPrompt string) error {
	s.mu.Lock()
	parentID := child.parentRunID
	child.legState = LegStateClosed
	child.legClosedReason = LegClosedReasonProviderSwitch
	prompt := pendingPrompt
	if strings.TrimSpace(prompt) == "" {
		prompt = child.lastFullPrompt
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = child.lastPrompt
	}
	in := SpawnAgentInput{
		Agent:             child.agentName,
		Prompt:            prompt,
		Label:             child.label,
		Provider:          string(sel.ProviderKey),
		Model:             sel.Model,
		ProviderAccountID: sel.AccountID,
	}
	s.mu.Unlock()
	if err := s.persistProviderSession(sessionStateOf(child)); err != nil {
		return fmt.Errorf("quota_gate: close leg: %w", err)
	}
	if _, err := s.spawnChildRun(ctx, parentID, in); err != nil {
		return fmt.Errorf("quota_gate: respawn child: %w", err)
	}
	return nil
}

// RotateUsageBudgetRun is the usageBudgetRouter implementation (CP-86 seam):
// a node's usage-budget / context-reset headroom failure enters candidate
// routing here — resolve → commit, same contract as the provider-limit path.
func (s *InteractiveService) RotateUsageBudgetRun(runID string) error {
	return s.enterQuotaGate(context.Background(), runID, "usage_budget_exceeded")
}

// flowNodeForRun resolves the flow node a child run executes (same
// stepID/label mapping as flowNodeProfileFor); nil for hubs/roots.
func (s *InteractiveService) flowNodeForRun(rs *interactiveRun) (agentpack.FlowNode, bool) {
	if s == nil || rs == nil || strings.TrimSpace(rs.parentRunID) == "" {
		return agentpack.FlowNode{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	parent := s.runs[rs.parentRunID]
	if parent == nil || len(parent.activeFlowNodes) == 0 {
		return agentpack.FlowNode{}, false
	}
	stepID, label := strings.TrimSpace(rs.stepID), strings.TrimSpace(rs.label)
	for _, n := range parent.activeFlowNodes {
		if n.ID == stepID || (label != "" && n.ID == label) {
			return n, true
		}
	}
	return agentpack.FlowNode{}, false
}

// enterQuotaGate runs resolve → commit for a run whose trigger fired (typed
// provider_limit event, usage budget rotate answer, context-reset headroom
// escalation). Trigger names stay machine-readable for the audit surface.
func (s *InteractiveService) enterQuotaGate(ctx context.Context, runID, trigger string) error {
	res, err := s.resolveQuotaGate(ctx, runID, trigger)
	if err != nil {
		return err
	}
	return s.CommitQuotaResolution(ctx, res)
}

// resolveQuotaGate is the resolve half of enterQuotaGate — it returns the
// routing resolution without committing it so the caller can act on the
// outcome (BUG-511 round 2: admission must distinguish gate / rotate /
// blocked instead of collapsing every outcome into the same 409).
func (s *InteractiveService) resolveQuotaGate(ctx context.Context, runID, trigger string) (QuotaResolution, error) {
	s.mu.Lock()
	rs := s.runs[runID]
	s.mu.Unlock()
	if rs == nil {
		return QuotaResolution{}, fmt.Errorf("quota_gate: run %q not found", runID)
	}
	var node *agentpack.FlowNode
	if n, ok := s.flowNodeForRun(rs); ok {
		node = &n
	}
	demand, err := s.ResolveExecutionDemand(ctx, runID, node)
	if err != nil {
		return QuotaResolution{}, err
	}
	// A provider-limit trigger is live evidence about the current binding —
	// carry it so the just-failed account hard-rejects even before telemetry
	// or the durable block ledger reflect it.
	switch ProviderLimitKind(trigger) {
	case ProviderLimitQuotaExhausted, ProviderLimitCreditsExhausted, ProviderLimitBillingRequired:
		demand.ObservedLimit = ProviderLimitKind(trigger)
	}
	res, err := s.ResolveQuotaPreflight(ctx, demand)
	if err != nil {
		return res, err
	}
	if res.Reason == "" {
		res.Reason = trigger
	}
	return res, nil
}

// pinnedAccountHardVeto reports a limit trigger when the run's pinned account
// is already known-unusable — the durable block ledger first (its recorded
// reason is a ProviderLimitKind when it came from a provider 402/403), then
// live telemetry exhaustion that the ledger has not observed yet (BUG-511
// round 2: a telemetry-exhausted pin must not slip through merely because
// noteAccountBlockedLocked never ran). "" means no veto.
func (s *InteractiveService) pinnedAccountHardVeto(ctx context.Context, rs *interactiveRun) string {
	if s == nil || rs == nil {
		return ""
	}
	s.mu.Lock()
	acctID := strings.TrimSpace(rs.providerAccountID)
	provider := string(rs.providerKey)
	runID := rs.id
	s.mu.Unlock()
	if acctID == "" {
		return ""
	}
	s.quotaMu.Lock()
	reason := s.loadQuotaRuntimeState().accountBlockReason(acctID)
	s.quotaMu.Unlock()
	if reason != "" {
		return reason
	}
	if s.quotaTelemetryFn == nil {
		return ""
	}
	accounts, err := s.listProviderAccounts()
	if err != nil {
		return ""
	}
	for _, a := range accounts {
		if a.ID != acctID || a.ProviderKey != provider {
			continue
		}
		head := NormalizeAccountHeadroom(s.quotaTelemetryFn(ctx, a), s.quotaNow(), s.quotaSettingsForRun(runID))
		if head.State == "exhausted" {
			return string(ProviderLimitQuotaExhausted)
		}
		return ""
	}
	return ""
}

// emitQuotaRouteCard parks the run on a durable quota_route_required decision
// card. Options encode the commit target in the label so the answer alone —
// no extra state — resolves the route: use_once|p|a|m, use_for_run|p|a|m,
// stop. Persisted before emit; mirrored to the flow root like every engine
// decision card.
func (s *InteractiveService) emitQuotaRouteCard(rs *interactiveRun, res QuotaResolution) {
	s.mu.Lock()
	if s.quotaRoutePendingQuestionLocked(rs.id) {
		s.mu.Unlock()
		return
	}
	options := []QuestionOption{}
	appendRoute := func(c RouteCandidate) {
		key := string(c.ProviderKey) + "|" + c.AccountID + "|" + c.Model
		desc := fmt.Sprintf("%s account %s model %s", c.ProviderKey, c.AccountID, c.Model)
		if len(c.RejectionReasons) > 0 {
			desc += " (" + strings.Join(c.RejectionReasons, ",") + ")"
		}
		options = append(options,
			QuestionOption{Label: "use_for_run|" + key, Description: "for this run: " + desc},
			QuestionOption{Label: "use_once|" + key, Description: "once only: " + desc})
	}
	for _, a := range res.Accounts {
		if hasHardReject(a.RejectionReasons) {
			continue
		}
		if a.AccountID == res.Demand.RequestedAccountID {
			continue
		}
		appendRoute(RouteCandidate{
			ProviderKey: res.Demand.RequestedProvider, Model: res.Demand.RequestedModel,
			AccountID: a.AccountID, RejectionReasons: a.RejectionReasons,
		})
	}
	for _, c := range res.Candidates.Eligible {
		appendRoute(c)
	}
	for _, c := range res.Candidates.ManualOnly {
		appendRoute(c)
	}
	options = append(options, QuestionOption{Label: "stop", Description: "stop — emit quota_route_stopped"})

	expiresAt := time.Now().UTC().Add(s.questionTTL).Format(time.RFC3339Nano)
	decision := buildQuotaRouteDecision(res)
	rec := &questionRecord{
		id:            s.nextID("q"),
		runID:         rs.id,
		prompt:        quotaRouteQuestionKind + ": binding " + string(res.Demand.RequestedProvider) + "/" + res.Demand.RequestedAccountID + " unusable (" + res.Reason + ")",
		options:       options,
		status:        "pending",
		resolve:       make(chan questionResolveResult, 1),
		expiresAt:     expiresAt,
		revision:      1,
		createdAt:     time.Now().UTC().Format(time.RFC3339Nano),
		kind:          quotaRouteQuestionKind,
		quotaDecision: decision,
	}
	s.questions[rec.id] = rec
	rs.pendingQuestionID = rec.id
	rs.status = RunStatusWaitingQuestion
	rs.agentStatus = string(RunStatusWaitingQuestion)
	questionID := rec.id
	snapshot := questionStateFromRecord(rec, "", expiresAt)
	s.mu.Unlock()

	if err := s.persistQuestion(snapshot); err != nil {
		s.mu.Lock()
		delete(s.questions, questionID)
		if cur := s.runs[rs.id]; cur != nil && cur.pendingQuestionID == questionID {
			cur.pendingQuestionID = ""
		}
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	if cur := s.runs[rs.id]; cur != nil {
		s.emitLocked(cur, ProviderEvent{
			Type:          EventUserQuestionRequired,
			QuestionID:    questionID,
			Prompt:        rec.prompt,
			Options:       options,
			QuotaDecision: decision,
		})
		if rootID := s.flowRootIDLocked(cur); rootID != "" && rootID != cur.id {
			if root := s.runs[rootID]; root != nil {
				s.emitLocked(root, ProviderEvent{
					Type:          EventUserQuestionRequired,
					QuestionID:    questionID,
					Prompt:        rec.prompt,
					Options:       options,
					QuotaDecision: decision,
				})
			}
		}
	}
	s.mu.Unlock()
}

func (s *InteractiveService) quotaRoutePendingQuestionLocked(runID string) bool {
	for _, rec := range s.questions {
		if rec == nil || rec.runID != runID {
			continue
		}
		if questionRecordKind(rec) == quotaRouteQuestionKind && rec.status == "pending" {
			return true
		}
	}
	return false
}

// questionRecordKind resolves the card kind; rehydrated records lose their
// in-memory kind flag, so the persisted prompt prefix is the durable fallback.
func questionRecordKind(rec *questionRecord) string {
	if rec.kind != "" {
		return rec.kind
	}
	for _, k := range []string{quotaRouteQuestionKind, usageBudgetQuestionKind, contextPressureQuestionKind} {
		if strings.HasPrefix(rec.prompt, k+":") {
			return k
		}
	}
	return ""
}

// ResumeQuotaGate applies the user's durable card answer. Called from
// AnswerQuestion's kind routing (decisionID is the question id); optionID is
// the chosen label. `stop` emits quota_route_stopped; use_* commits the route
// through the same rotation machinery as auto mode.
func (s *InteractiveService) ResumeQuotaGate(ctx context.Context, runID, decisionID, optionID string) error {
	s.mu.Lock()
	rs := s.runs[runID]
	s.mu.Unlock()
	if rs == nil {
		return fmt.Errorf("quota_gate: run %q not found", runID)
	}
	optionID = strings.TrimSpace(optionID)
	if optionID == "stop" {
		s.mu.Lock()
		if cur := s.runs[runID]; cur != nil {
			s.emitLocked(cur, ProviderEvent{
				Type: EventQuotaRouteStopped,
				QuotaRoute: &QuotaRoutePayload{
					FromProvider: cur.providerKey, FromAccount: cur.providerAccountID,
					Reason: "user_stopped",
				},
			})
		}
		s.mu.Unlock()
		if rs.parentRunID != "" {
			if _, err := s.applyFlowControl(rs.parentRunID, FlowControlInput{
				Status:  "escalate",
				Summary: "quota_route_required: user chose stop on the quota routing card",
			}); err != nil {
				return err
			}
		}
		return nil
	}
	var rest, scope string
	switch {
	case strings.HasPrefix(optionID, "use_once|"):
		rest, scope = strings.TrimPrefix(optionID, "use_once|"), "once"
	case strings.HasPrefix(optionID, "use_for_run|"):
		rest, scope = strings.TrimPrefix(optionID, "use_for_run|"), "run"
	default:
		return fmt.Errorf("quota_gate: unknown option %q", optionID)
	}
	parts := strings.SplitN(rest, "|", 3)
	if len(parts) != 3 {
		return fmt.Errorf("quota_gate: malformed option %q", optionID)
	}
	sel := &RouteCandidate{ProviderKey: ProviderKey(parts[0]), AccountID: parts[1], Model: parts[2]}
	demand, err := s.ResolveExecutionDemand(ctx, runID, nil)
	if err != nil {
		return err
	}
	return s.commitQuotaRotation(ctx, QuotaResolution{
		Outcome: QuotaRotate, Demand: demand, Selected: sel,
		PolicyVersion: QuotaRoutingPolicyVersion, Reason: "user_choice", Scope: scope,
	})
}

// applyQuotaRouteAnswer is the AnswerQuestion routing shim — keeps the kind
// switch in interactive_service.go a one-liner like its siblings.
func (s *InteractiveService) applyQuotaRouteAnswer(rs *interactiveRun, questionID, optionID string) {
	if err := s.ResumeQuotaGate(context.Background(), rs.id, questionID, optionID); err != nil {
		log.Printf("[quota-gate] answer apply failed run=%s err=%v", rs.id, err)
	}
}
