package runner

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Task-443 (CP-86 P-4): flag-gated context-pressure ladder + provider
// self-compaction detection.
//
// Window context is a SESSION problem, not an account-quota problem: when a
// provider session fills its context window the provider either auto-compacts
// (opaque, lossy — FlowPilot cannot prevent or trigger it) or hard-fails.
// The correct response is a same-binding LEG RESET (fresh provider session
// reseeded from durable FlowPilot state), not account/provider routing —
// routing is entered only when the pinned account cannot afford the reseed.
//
// rotate_leg is meaningful ONLY on long-lived sessions (root hub/chat runs
// that persist across turns): per-step child runs already get a fresh
// context window on the next step, so offering a reset there is a no-op and
// is suppressed (continue/stop only).
//
// Never mid-turn: a rotate_leg answer commits durably (resolved question +
// contextResetPending) and takes effect at the NEXT admission boundary; the
// in-flight turn always runs to its natural end. The intent expires if the
// run reaches a terminal state first — committed work is never replayed for
// context reasons.

const contextPressureEnvFlag = "FLOWPILOT_CONTEXT_PRESSURE"

// contextPressureQuestionKind marks engine-emitted context-pressure cards.
const contextPressureQuestionKind = "context_pressure_90"

// maxContextLegResets bounds same-binding leg resets per run — a context
// reset does not ride the 20s account-switch cooldown (no account switch
// happens) but is still bounded so a pathological loop gates the user.
const maxContextLegResets = 2

const (
	contextPressureAwareRatio = 0.80 // ≥80% → awareness event only
	contextPressureAskRatio   = 0.90 // ≥90% → user decision card
	compactionDropRatio       = 0.30 // >30% TotalTokens drop within one leg
)

// MVP: always ON — the flag path no longer exists as a runtime option;
// FLOWPILOT_CONTEXT_PRESSURE is ignored. Rollback for a bad rollout is a
// revert commit, not a flag flip (same posture as chatSSOTEnabled and
// ReproduceGateEnabled).
func contextPressureEnabled() bool {
	return true
}

// pressureTier maps a usage/window ratio onto the ladder tier.
func pressureTier(ratio float64) string {
	switch {
	case ratio >= contextPressureAskRatio:
		return "ask"
	case ratio >= contextPressureAwareRatio:
		return "aware"
	default:
		return ""
	}
}

// isProviderCompaction reports a sharp SAME-LEG TotalTokens drop — the only
// observable signature of a provider compacting its own session. Drops across
// a leg boundary (session id change) are normal resets, never compaction.
func isProviderCompaction(prev, cur int64) bool {
	return prev > 0 && cur < prev && float64(prev-cur)/float64(prev) > compactionDropRatio
}

// evalContextPressureLocked inspects a just-appended token_usage_updated
// event and emits the follow-up events through emitLocked (non-recursive:
// pressure events are not usage events). Caller must hold s.mu.
//
// Allocation-cheap per the Task-443 constraint: reads only rs fields and the
// last event — no I/O, no goroutines, no scans of rs.events.
func (s *InteractiveService) evalContextPressureLocked(rs *interactiveRun, ev ProviderEvent) {
	snap := ev.TokenUsage
	if snap == nil {
		return
	}
	legID := ev.ProviderSessionID

	// --- Compaction detection (T-3): sharp TotalTokens drop, same leg. ---
	if snap.Total != nil {
		cur := snap.Total.TotalTokens
		// BUG-513: query-scoped legs (Claude `print` respawns emit per-query
		// usage frames) have NO trustworthy session-context signal — Total is
		// the seam's monotonic accumulator (cap accounting only, can never
		// drop) and Last is the single query's token aggregate, not session
		// occupancy. A long query followed by a short one satisfies every
		// drop-based heuristic — including the round-3 "previous read ≥80%
		// of window" gate — without the provider compacting anything.
		// Compaction detection therefore stays limited to cumulative legs
		// whose Total genuinely tracks the session; query-scoped legs keep
		// accumulating Total for caps but never emit provider_compacted.
		_, queryScoped := rs.legUsageTotals[legID]
		if !queryScoped && legID == rs.lastUsageSessionID && isProviderCompaction(rs.lastUsageTotalTokens, cur) {
			s.emitLocked(rs, ProviderEvent{
				Type: EventProviderCompacted,
				ContextPressure: &ContextPressurePayload{
					UsedTokens: cur,
					PrevTokens: rs.lastUsageTotalTokens,
					LegID:      legID,
				},
			})
			// The leg's provider-held context is no longer trustworthy — mark
			// it context_degraded so the next admission may offer rotate_leg.
			if rs.contextDegradedLegs == nil {
				rs.contextDegradedLegs = map[string]bool{}
			}
			rs.contextDegradedLegs[legID] = true
		}
		rs.lastUsageSessionID = legID
		rs.lastUsageTotalTokens = cur
	}

	// --- Pressure ladder (T-1/T-2): Last/window ratio, deduped per leg. ---
	if snap.Last == nil || snap.ModelContextWindow == nil || *snap.ModelContextWindow <= 0 {
		return // unknown window → evaluate nothing (never guess)
	}
	used := snap.Last.TotalTokens
	window := *snap.ModelContextWindow
	tier := pressureTier(float64(used) / float64(window))
	if tier == "" {
		return
	}
	if rs.pressureTierFired == nil {
		rs.pressureTierFired = map[string]bool{}
	}
	dedupeKey := legID + "|" + tier
	if rs.pressureTierFired[dedupeKey] {
		return
	}
	rs.pressureTierFired[dedupeKey] = true

	s.emitLocked(rs, ProviderEvent{
		Type: EventContextPressure,
		ContextPressure: &ContextPressurePayload{
			Tier:         tier,
			Ratio:        float64(used) / float64(window),
			UsedTokens:   used,
			WindowTokens: window,
			LegID:        legID,
		},
	})
	if tier != "ask" {
		return
	}
	s.emitContextPressureCardLocked(rs, used, window)
}

// emitContextPressureCardLocked emits the ≥90% decision card through the
// existing question machinery. rotate_leg appears only where a leg reset is
// meaningful — long-lived root sessions; a per-step child already gets a
// fresh window on the next step so the option is a no-op there (T-4).
// Caller must hold s.mu.
func (s *InteractiveService) emitContextPressureCardLocked(rs *interactiveRun, used, window int64) {
	for _, rec := range s.questions {
		if rec != nil && rec.runID == rs.id && rec.kind == contextPressureQuestionKind &&
			(rec.status == "pending" || rec.status == "resolving") {
			return // one pending pressure card per run — never a flood
		}
	}
	options := []QuestionOption{}
	if contextResetEligibleLocked(rs) {
		options = append(options, QuestionOption{Label: "rotate_leg", Description: "reset the provider session on the same account/model, reseeded from durable context"})
	}
	options = append(options,
		QuestionOption{Label: "continue", Description: "keep running — the provider may compact its own context (quality may drift)"},
		QuestionOption{Label: "stop", Description: "stop the session instead of riding the window to the wall"},
	)
	expiresAt := time.Now().UTC().Add(s.questionTTL).Format(time.RFC3339Nano)
	prompt := fmt.Sprintf("context_pressure: this session is at %d%% of its context window (%d/%d tokens)", int(float64(used)/float64(window)*100), used, window)
	if window <= 0 {
		// Degraded-leg admission offer when the window is unknown: the provider
		// compacted — the honest message is the fact, not a fake percentage.
		prompt = "context_pressure: the provider compacted this leg's context — reset the session or keep running degraded"
	}
	rec := &questionRecord{
		id:        s.nextID("q"),
		runID:     rs.id,
		prompt:    prompt,
		options:   options,
		status:    "pending",
		resolve:   make(chan questionResolveResult, 1),
		expiresAt: expiresAt,
		revision:  1,
		createdAt: time.Now().UTC().Format(time.RFC3339Nano),
		kind:      contextPressureQuestionKind,
	}
	s.questions[rec.id] = rec
	rs.pendingQuestionID = rec.id
	snapshot := questionStateFromRecord(rec, rs.currentTurnID, expiresAt)
	questionID := rec.id
	// Persist pending before emit (BUG-288 P1-07); a persist failure rolls the
	// record back so no ghost card exists without a durable question.
	s.mu.Unlock()
	err := s.persistQuestion(snapshot)
	s.mu.Lock()
	if err != nil {
		delete(s.questions, questionID)
		if rs.pendingQuestionID == questionID {
			rs.pendingQuestionID = ""
		}
		return
	}
	s.emitLocked(rs, ProviderEvent{
		Type:       EventUserQuestionRequired,
		QuestionID: questionID,
		Prompt:     prompt,
		Options:    options,
	})
	if rootID := s.flowRootIDLocked(rs); rootID != "" {
		if root := s.runs[rootID]; root != nil {
			s.emitLocked(root, ProviderEvent{
				Type:       EventUserQuestionRequired,
				QuestionID: questionID,
				Prompt:     prompt,
				Options:    options,
			})
		}
	}
}

// contextResetEligibleLocked reports whether a same-binding leg reset is
// meaningful for this run: only long-lived root sessions persist one
// provider session across turns. Caller must hold s.mu.
func contextResetEligibleLocked(rs *interactiveRun) bool {
	return rs.parentRunID == "" && rs.chatID != "" && rs.legState == LegStateActive
}

// applyContextPressureAnswer applies a resolved context_pressure_90 choice.
// `rotate_leg` commits the reset intent durably; it executes at the next
// admission boundary (consumePendingContextReset / startTurn), never
// mid-turn. `continue` is terminal — provider compaction is accepted (the
// leg is marked degraded on detection). `stop` cancels the run.
func (s *InteractiveService) applyContextPressureAnswer(rs *interactiveRun, optionID string) {
	if s == nil || rs == nil {
		return
	}
	switch strings.TrimSpace(optionID) {
	case "rotate_leg":
		s.mu.Lock()
		if contextResetEligibleLocked(rs) {
			rs.contextResetPending = true
		}
		s.mu.Unlock()
	case "stop":
		_, _ = s.stopAgentLoop(rs.id)
	}
}

// maybeOfferContextResetAtAdmissionLocked treats a provider-compacted
// (context-degraded) leg like the ask tier at the next admission boundary:
// it offers the rotate_leg decision card. Once per leg — a user who chose
// continue is not re-asked on every subsequent admission (the card dedupe
// inside emitContextPressureCardLocked still covers pending/resolving).
// Caller must hold s.mu.
func (s *InteractiveService) maybeOfferContextResetAtAdmissionLocked(rs *interactiveRun) {
	if s == nil || rs == nil || !contextPressureEnabled() {
		return
	}
	if !contextResetEligibleLocked(rs) {
		return
	}
	legID := rs.providerSessionID
	if !rs.contextDegradedLegs[legID] {
		return
	}
	if rs.contextResetOfferedLegs == nil {
		rs.contextResetOfferedLegs = map[string]bool{}
	}
	if rs.contextResetOfferedLegs[legID] {
		return
	}
	rs.contextResetOfferedLegs[legID] = true
	window := int64(0)
	if w := modelContextWindowFor(rs.providerKey, rs.modelName); w != nil {
		window = *w
	}
	s.emitContextPressureCardLocked(rs, rs.lastUsageTotalTokens, window)
}

// consumePendingContextReset performs the committed leg reset at an
// admission boundary: a fresh provider session on the SAME
// provider+account+model binding, reseeded by the chat-switch handoff
// machinery (durable transcript — FlowPilot chooses what survives, not the
// provider's compaction). Returns the new leg's run id, "" when nothing was
// consumed (no intent, terminal run, ineligible, headroom insufficient, or
// rotation failed — the intent stays for a later admission).
func (s *InteractiveService) consumePendingContextReset(ctx context.Context, runID string) string {
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil || !rs.contextResetPending {
		s.mu.Unlock()
		return ""
	}
	// Intent expiry: a run that reached a terminal state first never replays
	// for context reasons.
	if worktreeTerminal(rs.status) {
		rs.contextResetPending = false
		s.mu.Unlock()
		return ""
	}
	if !contextResetEligibleLocked(rs) {
		rs.contextResetPending = false
		s.mu.Unlock()
		return ""
	}
	if rs.contextResetCount >= maxContextLegResets {
		rs.contextResetPending = false
		s.mu.Unlock()
		return "" // bounded — escalate via routing gate when CP-87 lands
	}
	chatID := rs.chatID
	headroom := s.contextResetHeadroomOK
	s.mu.Unlock()

	// Reseed costs tokens — the pinned account must afford the handoff. On
	// failure this becomes a QUOTA problem: escalate to the CP-87 gate.
	if headroom != nil && !headroom(rs) {
		if s.usageRouter != nil {
			_ = s.usageRouter.RotateUsageBudgetRun(runID)
		}
		return ""
	}

	resp, aerr := s.rotateChatLegForContext(ctx, chatID, runID)
	if aerr != nil || resp.Handle.RunID == "" {
		return ""
	}
	s.mu.Lock()
	if cur := s.runs[runID]; cur != nil {
		cur.contextResetPending = false
		cur.contextResetCount++
	}
	s.mu.Unlock()
	return resp.Handle.RunID
}

// rotateChatLegForContext resets the leg on the SAME provider+account+model
// binding — it reuses the provider-switch machinery with the same-provider
// rejection lifted (a context reset is not routing).
func (s *InteractiveService) rotateChatLegForContext(ctx context.Context, chatID, srcRunID string) (chatSwitchResponse, *apiErr) {
	s.mu.Lock()
	src := s.runs[srcRunID]
	if src == nil {
		s.mu.Unlock()
		return chatSwitchResponse{}, newAPIErr(404, "run_not_found", "workflow run not found")
	}
	req := chatSwitchRequest{
		TargetProviderKey: src.providerKey,
		Model:             src.modelName,
		ReasoningEffort:   src.reasoningEffort,
	}
	s.mu.Unlock()
	return s.switchChatLeg(ctx, chatID, req, true)
}
