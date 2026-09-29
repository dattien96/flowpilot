package runner

// Chat provider switch (CP-59 / SD-26 §7.1, Task-314): the three-phase
// linearized cross-provider switch for normal_chat chats.
//
// Lock rule (SD26-S-2): phase A validates under s.mu and stamps the durable
// intent (persisted BEFORE unlock); phase B — chat-store I/O, createRun (which
// takes s.mu itself), seed dispatch — runs with NO lock held; phase C closes
// the old leg and appends the switch record under s.mu. Ordering is
// start-new-first, close-old-second so every crash window heals
// (healChatLegsLocked, matrix in SD-26 §8).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// chatSwitchHardCapBytes caps the context-derived envelope budget (SD-26 D-9).
const chatSwitchHardCapBytes = 512 * 1024

type chatSwitchRequest struct {
	TargetProviderKey ProviderKey `json:"targetProviderKey"`
	Model             string      `json:"model,omitempty"`
	ReasoningEffort   string      `json:"reasoningEffort,omitempty"`
	YoloMode          *bool       `json:"yoloMode,omitempty"` // nil = inherit current leg
	// ProviderAccountID pins the new leg to a specific connected account —
	// set by the quota gate's committed rotation (Task-449); empty keeps the
	// legacy resolve-active-account behavior for user-initiated switches.
	ProviderAccountID string `json:"providerAccountId,omitempty"`
}

type chatSwitchHandoffStats struct {
	Mode              string `json:"handoffMode"` // raw | target_summary | fresh_start
	IncludedTurnCount int    `json:"includedTurnCount"`
	OmittedTurnCount  int    `json:"omittedTurnCount"`
	Truncated         bool   `json:"truncated"`
	ActionsDigest     bool   `json:"actionsDigestIncluded"`
}

type chatEnvelope struct {
	Prompt string
	Stats  chatSwitchHandoffStats
}

type chatSwitchResponse struct {
	Handle  RunHandle              `json:"handle"`
	ChatID  string                 `json:"chatId"`
	LegSeq  int                    `json:"legSeq"`
	Model   string                 `json:"model"` // target model — clients set it on adopt (review I-R6)
	Handoff chatSwitchHandoffStats `json:"handoff"`
}

func (s *InteractiveService) handleChatSwitchProvider(w http.ResponseWriter, r *http.Request) {
	var req chatSwitchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	if strings.TrimSpace(string(req.TargetProviderKey)) == "" {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "targetProviderKey is required"))
		return
	}
	chatID := strings.TrimSpace(r.PathValue("chatId"))
	resp, apiErr := s.switchChatProvider(r.Context(), chatID, req)
	if apiErr != nil {
		writeInteractiveError(w, apiErr)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, resp)
}

// activeChatLegsLocked collects a chat's legs (resident runs only; legacy
// untagged runs self-tag). Caller holds s.mu.
func (s *InteractiveService) activeChatLegsLocked(chatID string) []*interactiveRun {
	var legs []*interactiveRun
	for _, rs := range s.runs {
		if rs.runKind != "chat" {
			continue // chat-kind gate absolute (CS-12)
		}
		if rs.chatID == chatID {
			legs = append(legs, rs)
			continue
		}
		if rs.id == chatID {
			ensureChatTagging(rs)
			if s.chatRuns != nil {
				s.chatRuns.register(rs.id, rs.chatID)
			}
			if rs.chatID == chatID {
				legs = append(legs, rs)
			}
		}
	}
	return legs
}

// healChatLegsLocked closes the SD26-S-2 crash windows observable on leg
// records: two-active (discriminator: keep `switchFromRunID != own id`, close
// `switchFromRunID == own id` + append the missing E-9 record), closed-no-
// record (old already closed, new active, E-9 missing → append), and orphan
// intent (single active leg stamped with its own id and no partner → clear).
// Caller holds s.mu. Best effort: record appends are non-fatal.
func (s *InteractiveService) healChatLegsLocked(chatID string) {
	legs := s.activeChatLegsLocked(chatID)
	var newLeg, oldLeg *interactiveRun
	var closedSrc *interactiveRun
	for _, rs := range legs {
		switch {
		case rs.legState == LegStateActive && rs.switchFromRunID != "" && rs.switchFromRunID != rs.id:
			newLeg = rs
		case rs.legState == LegStateActive && rs.switchFromRunID == rs.id:
			oldLeg = rs
		case rs.legState == LegStateClosed && (rs.legClosedReason == LegClosedReasonProviderSwitch || rs.legClosedReason == LegClosedReasonContextReset):
			closedSrc = rs
		}
	}
	switch {
	case newLeg != nil && oldLeg != nil:
		// Two-active window: keep the new leg (it points at the source), close
		// the source, append the missing record.
		oldLeg.legState = LegStateClosed
		oldLeg.legClosedReason = LegClosedReasonProviderSwitch
		clearClosedLegPendingLocked(oldLeg)
		s.appendChatSwitchRecordOnce(chatID, oldLeg, newLeg)
	case newLeg != nil && closedSrc != nil && closedSrc.id == newLeg.switchFromRunID:
		// Closed-no-record window (review C-3): phase C's record write was
		// lost — append it idempotently.
		s.appendChatSwitchRecordOnce(chatID, closedSrc, newLeg)
	default:
		// Orphan intent: a lone active leg carrying its own id means a switch
		// was interrupted before createRun — clear so it can be retried.
		if newLeg == nil && oldLeg != nil && oldLeg.legState == LegStateActive {
			oldLeg.switchFromRunID = ""
		}
	}
}

// appendChatSwitchRecordOnce emits the SD26-E-9 record only when the pair has
// none yet (heal re-entry safety); the direct switch path appends without the
// scan because the in-flight guard already serializes it.
func (s *InteractiveService) appendChatSwitchRecordOnce(chatID string, from, to *interactiveRun, stats ...chatSwitchHandoffStats) {
	if s.chatTranscripts == nil {
		return
	}
	if s.hasSwitchRecord(context.Background(), chatID, to.id) {
		return
	}
	st := chatSwitchHandoffStats{Mode: "raw"}
	if len(stats) > 0 {
		st = stats[0]
	}
	s.appendChatSwitchRecord(chatID, from, to, st)
}

// hasSwitchRecord reports whether an E-9 record for toRunID already exists.
func (s *InteractiveService) hasSwitchRecord(ctx context.Context, chatID, toRunID string) bool {
	if s.chatTranscripts == nil {
		return false
	}
	records, err := s.chatTranscripts.store.ReadChatRecords(ctx, chatID, 0, 0)
	if err != nil {
		return false
	}
	for _, rec := range records {
		if rec.Type != EventTypeChatProviderSwitch {
			continue
		}
		var p struct {
			ToRunID string `json:"toRunId"`
		}
		if json.Unmarshal(rec.Payload, &p) == nil && p.ToRunID == toRunID {
			return true
		}
	}
	return false
}

// persistSwitchLegCloseLocked commits the old leg's closed state durably.
// BUG-489: the upsert used to be fire-and-forget (`_ =`) — a failure left the
// leg `active` on disk while memory said closed, resurrecting a dual-active
// leg pair on restart. One retry absorbs transient IO; a final failure is
// logged and marks the leg degraded so the operator/timeline can see the
// close never committed. Caller holds s.mu.
func (s *InteractiveService) persistSwitchLegCloseLocked(src *interactiveRun) {
	if err := s.persistProviderSession(sessionStateOf(src)); err != nil {
		if retryErr := s.persistProviderSession(sessionStateOf(src)); retryErr != nil {
			fmt.Printf("[chat-switch] leg-close persist failed run=%s: %v — leg may resurrect as active on restart\n", src.id, retryErr)
			if s.chatRuns != nil {
				s.chatRuns.setDegraded(src.id, true)
			}
		}
	}
}

// isProvisioningSeedError classifies a seed-turn admission error: provisioning
// failures mean the leg can never serve a turn (abort-worthy for a committed
// routing switch), while any other failure is content/runtime-class and stays
// non-fatal under SD26-X-7.
func isProvisioningSeedError(err error) bool {
	var ae *apiErr
	if !errors.As(err, &ae) {
		return false
	}
	switch ae.code {
	case "session_unavailable", "account_unavailable", "account_not_signed_in",
		"provider_unavailable", "leg_closed":
		return true
	}
	return false
}

// markSwitchSeedFailed records the SD26-X-7 state as a queryable chat record
// (type switch_seed_failed) instead of log-only: the timeline consumer and
// operators can see the committed switch whose seed turn failed, and the chat
// stays continuable.
func (s *InteractiveService) markSwitchSeedFailed(chatID, runID string, cause error) {
	if s.chatTranscripts == nil {
		return
	}
	payload, _ := json.Marshal(map[string]any{"runId": runID, "error": cause.Error()})
	_ = s.chatTranscripts.append(context.Background(), ChatTranscriptRecord{
		ChatID: chatID, LegRunID: runID, Type: EventTypeChatSeedFailed, Payload: payload,
	})
}

// appendChatSwitchRecord emits the SD26-E-9 record exactly once per
// (fromRunId, toRunId) pair. Best effort: a store failure logs via the
// degraded path and never fails the switch.
func (s *InteractiveService) appendChatSwitchRecord(chatID string, from, to *interactiveRun, stats chatSwitchHandoffStats) {
	writer := s.chatTranscripts
	if writer == nil {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"fromProvider": string(from.providerKey), "fromModel": from.modelName,
		"toProvider": string(to.providerKey), "toModel": to.modelName,
		"fromRunId": from.id, "toRunId": to.id, "legSeq": to.legSeq,
		"handoffMode": stats.Mode, "includedTurnCount": stats.IncludedTurnCount,
		"omittedTurnCount": stats.OmittedTurnCount, "truncated": stats.Truncated,
	})
	if err != nil {
		return
	}
	_ = writer.append(context.Background(), ChatTranscriptRecord{
		ChatID: chatID, LegRunID: to.id, Type: EventTypeChatProviderSwitch, Payload: payload,
	})
}

func (s *InteractiveService) switchChatProvider(ctx context.Context, chatID string, req chatSwitchRequest) (chatSwitchResponse, *apiErr) {
	return s.switchChatLeg(ctx, chatID, req, false)
}

// switchLegFlowRefFallback decides whether the source leg's chatFlowRef rides
// the new leg as FlowRefFallback. The ref is only still needed while the flow
// could legitimately launch: a pending latch needs it as the forward target,
// and an immediate mount that never ran keeps its intent. Once
// flowEngineDriven is set the flow already launched on the source leg
// (immediate first-turn or a committed forward — both set it), and the new leg
// restarts at turnCount=0: a carried ref would resolve as a first-turn mount
// in resolveWorkflowFlowRef and re-run the whole flow on the first post-switch
// turn whose seed failed or was never sent (CP-89 review R8-1).
func switchLegFlowRefFallback(src *interactiveRun) string {
	if src.flowEngineDriven {
		return ""
	}
	return src.chatFlowRef
}

// switchChatLeg mints a new leg for the chat from the handoff machinery.
// allowSameProvider=false keeps the provider-switch contract (same-provider
// continuity uses the in-place model-change path); true is the Task-443
// context reset — a fresh provider session on the SAME binding, reseeded
// from the durable transcript (leg lifecycle, not account routing).
func (s *InteractiveService) switchChatLeg(ctx context.Context, chatID string, req chatSwitchRequest, allowSameProvider bool) (chatSwitchResponse, *apiErr) {
	if chatID == "" {
		return chatSwitchResponse{}, newAPIErr(http.StatusBadRequest, "invalid_request", "chatId is required")
	}
	// BUG-466: the transcript stack is lazy (chatOnce) — restored legs never
	// run createRun, so on a fresh process the first switch can be the first
	// transcript-touching call. Without this, buildChatHandoffContext saw a
	// nil writer and silently degraded to fresh_start: no seed turn, zero
	// context on the new leg, and an E-9 record durably logging the wrong
	// stats. Initialize before Phase A so heal paths can append too.
	s.ensureChatTranscriptWriter()

	// ---- Phase A (s.mu): heal, guards, durable intent, release -------------
	s.mu.Lock()
	if s.chatSwitchInFlight == nil {
		s.chatSwitchInFlight = map[string]bool{}
	}
	s.healChatLegsLocked(chatID)
	legs := s.activeChatLegsLocked(chatID)
	if len(legs) == 0 {
		s.mu.Unlock()
		return chatSwitchResponse{}, newAPIErr(http.StatusNotFound, "chat_not_found", "no legs found for chat")
	}
	var src *interactiveRun
	anyActive := false
	for _, rs := range legs {
		if rs.legState == LegStateActive {
			anyActive = true
			if src == nil || rs.legSeq > src.legSeq {
				src = rs
			}
		}
	}
	if !anyActive || src == nil {
		// Detached (restored) chat: reattach is a direct createRun — never the
		// switch endpoint (SD26 §10 / review I-R3).
		s.mu.Unlock()
		return chatSwitchResponse{}, newAPIErr(http.StatusConflict, "chat_no_active_leg", "chat has no active leg — reattach via startRun with chatId+switchFromRunId")
	}
	if src.turnInFlight || src.pendingApprovalID != "" || src.pendingQuestionID != "" {
		s.mu.Unlock()
		return chatSwitchResponse{}, newAPIErr(http.StatusConflict, "handoff_run_busy", "cannot switch while the chat leg is active (turn/approval/question pending)")
	}
	if s.chatSwitchInFlight[chatID] {
		s.mu.Unlock()
		return chatSwitchResponse{}, newAPIErr(http.StatusConflict, "handoff_run_busy", "a provider switch is already in flight")
	}
	if strings.EqualFold(string(src.providerKey), string(req.TargetProviderKey)) && !allowSameProvider {
		s.mu.Unlock()
		return chatSwitchResponse{}, newAPIErr(http.StatusConflict, "handoff_same_provider", "same-provider continuity uses the in-place model-change path")
	}
	if _, err := s.registry.Selectable(req.TargetProviderKey); err != nil {
		s.mu.Unlock()
		return chatSwitchResponse{}, newAPIErr(http.StatusUnprocessableEntity, "provider_unavailable", err.Error())
	}
	// BUG-541 residual: a committed route pins the target account — resolve it
	// before stamping the durable intent so an unprovisionable binding never
	// mints a leg (close-before-provision zombie, live run-19946).
	if err := s.pinnedAccountResolvable(req.TargetProviderKey, req.ProviderAccountID); err != nil {
		s.mu.Unlock()
		return chatSwitchResponse{}, newAPIErr(http.StatusConflict, "account_unavailable", err.Error())
	}
	// Durable intent, persisted BEFORE unlock (review I-R5): a crash after
	// this point leaves the marker on disk for healChatLegsLocked.
	src.switchFromRunID = src.id
	if err := s.persistProviderSession(sessionStateOf(src)); err != nil {
		src.switchFromRunID = ""
		s.mu.Unlock()
		return chatSwitchResponse{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	s.chatSwitchInFlight[chatID] = true
	s.mu.Unlock()

	// ---- Phase B (no lock): envelope, createRun, seed ----------------------
	env := s.buildChatHandoffContext(ctx, chatID, src, req)
	seedPrompt := env.Prompt
	// The latch pair (flowArm + flowEngineDriven, via the fallback helper)
	// must be read atomically: a forward committing between the Phase-A
	// turnInFlight check and this read could otherwise hand the new leg a
	// torn pending+ref pair and double-launch the flow on forward.
	s.mu.Lock()
	srcFlowArm := src.flowArm
	srcFlowRefFallback := switchLegFlowRefFallback(src)
	s.mu.Unlock()
	createInput := StartRunInput{
		ProjectID:       src.projectID,
		ChatMode:        "normal_chat",
		ProviderKey:     req.TargetProviderKey,
		Model:           req.Model,
		ReasoningEffort: req.ReasoningEffort,
		Cwd:             src.workspaceCwd,
		ChatID:          chatID,
		SwitchFromRunID: src.id,
		LegSeq:          src.legSeq + 1,
		// Task-449: a quota-committed switch carries the target account pin so
		// the new leg never re-resolves the machine-global active account.
		ProviderAccountID: req.ProviderAccountID,
		// CP-89 Task-451: flowArm + the pin are run-scoped — the new leg
		// inherits them so a pending latch survives a provider switch. The pin
		// rides FlowRefFallback (internal carrier, never a user mount) so vibe
		// markers stay governed by the latch, and the parent's working mode
		// travels with it (a vibe pending run must stay vibe on the new leg).
		FlowArm:         string(srcFlowArm),
		FlowRefFallback: srcFlowRefFallback,
		WorkingMode:     src.workingMode,
		// CP-89 review R4-3: the create-time CP source pin is run-scoped like
		// the latch — a pending vibe-cp-ingest leg that loses it wedges its
		// forward on invalid_cp_source with no way to recover the pin.
		SourceDocID: src.sourceDocID,
		// The leg is engine-minted, not a user mount — the client and
		// start-family gates were validated on the source leg's own create.
		// (Without this the propagated vibe mode would hit CheckClient's
		// user-facing "vibe is Desktop/TUI only" gate with no client header.)
		SpawnedInternally: true,
	}
	if req.YoloMode != nil {
		createInput.YoloMode = *req.YoloMode
	} else {
		createInput.YoloMode = src.yolo
	}
	newHandle, aerr := s.createRun(createInput)
	if aerr != nil {
		s.mu.Lock()
		src.switchFromRunID = "" // heal row 2 also clears this on load
		delete(s.chatSwitchInFlight, chatID)
		s.mu.Unlock()
		return chatSwitchResponse{}, aerr
	}
	// Seed fire-and-return (SD-26 D-6): stats ride the seed turn; a seed
	// failure surfaces in the stream, the leg stays active and continuable
	// (SD26-X-7).
	if seedPrompt != "" {
		if _, seedErr := s.startTurn(newHandle.RunID, TurnInput{StepID: newHandle.StepID, Prompt: seedPrompt}, "chat_switch_seed", "chat-switch-seed-"+chatID+"-"+newHandle.RunID); seedErr != nil {
			// BUG-541 residual / BUG-534 chat-leg variant (live run-19946): a
			// routing-commit switch pins ProviderAccountID — if the new leg
			// cannot even provision a session (account home unresolvable,
			// provider gone), leaving it active strands the chat on a dead
			// binding forever (close-before-provision zombie). Abort the
			// switch: fail the new leg terminal, keep the source leg open.
			// Content-level seed failures stay non-fatal (SD26-X-7).
			if req.ProviderAccountID != "" && isProvisioningSeedError(seedErr) {
				s.mu.Lock()
				if nl := s.runs[newHandle.RunID]; nl != nil {
					nl.status = RunStatusFailed
					nl.agentStatus = string(RunStatusFailed)
					nl.legState = LegStateClosed
					nl.legClosedReason = LegClosedReasonDispatchFailed
					clearClosedLegPendingLocked(nl)
				}
				src.switchFromRunID = ""
				delete(s.chatSwitchInFlight, chatID)
				snap := sessionStateOf(src)
				var newSnap *ProviderSessionState
				if nl := s.runs[newHandle.RunID]; nl != nil {
					st := sessionStateOf(nl)
					newSnap = &st
				}
				s.mu.Unlock()
				_ = s.persistProviderSession(snap)
				if newSnap != nil {
					_ = s.persistProviderSession(*newSnap)
				}
				return chatSwitchResponse{}, newAPIErr(http.StatusConflict, "leg_provision_failed",
					"new leg cannot provision a session: "+seedErr.msg)
			}
			// SD26-X-7: queryable record + log; the leg stays active and the
			// chat continuable (resend or re-switch).
			s.markSwitchSeedFailed(chatID, newHandle.RunID, seedErr)
			fmt.Printf("[chat-ssot] switch seed failed chat=%s leg=%s: %v (switch_seed_failed)\n", chatID, newHandle.RunID, seedErr)
		}
	}

	// ---- Phase C (s.mu): close old, append record, release -----------------
	s.mu.Lock()
	var newLeg *interactiveRun
	if rs := s.runs[newHandle.RunID]; rs != nil {
		newLeg = rs
	}
	src.legState = LegStateClosed
	// Same-binding close = context reset (Task-443), not a routing decision —
	// keep the ledger honest about why the leg ended.
	src.legClosedReason = LegClosedReasonProviderSwitch
	if allowSameProvider {
		src.legClosedReason = LegClosedReasonContextReset
	}
	// A closed leg can never flush armed intents or answer a parked card —
	// drop the transient pending state so the durable row stops reading
	// actionable (live run-945 residue).
	clearClosedLegPendingLocked(src)
	s.persistSwitchLegCloseLocked(src)
	if newLeg != nil {
		s.appendChatSwitchRecord(chatID, src, newLeg, env.Stats)
	}
	delete(s.chatSwitchInFlight, chatID)
	s.mu.Unlock()

	// BUG-452: report the leg's RESOLVED model, not the raw request — an empty
	// request model must not round-trip as "" and push clients onto a
	// catalog-index-0 guess that can differ from the session's actual model.
	// modelName holds the request verbatim for chat legs; empty falls back to
	// the provider default a turn would resolve to.
	resolvedModel := req.Model
	if newLeg != nil {
		resolvedModel = newLeg.modelName
		if resolvedModel == "" {
			resolvedModel = defaultModelForProvider(newLeg.providerKey)
		}
	}
	return chatSwitchResponse{Handle: newHandle, ChatID: chatID, LegSeq: newHandle.LegSeq, Model: resolvedModel, Handoff: env.Stats}, nil
}

// buildChatHandoffContext assembles the chat-scoped envelope (SD-26 D-9):
// turns from the transcript store (all legs), `<previous_conversation>` via
// Task-078's packer, optional `<actions_summary>` from tool/file/approval
// records, prefix handoffPromptPrefix preserved. Empty transcript → fresh_start
// with an empty prompt (no envelope, no error — CS-07).
func (s *InteractiveService) buildChatHandoffContext(ctx context.Context, chatID string, src *interactiveRun, req chatSwitchRequest) chatEnvelope {
	if s.chatTranscripts == nil {
		return chatEnvelope{Stats: chatSwitchHandoffStats{Mode: "fresh_start"}}
	}
	records, err := s.chatTranscripts.store.ReadChatRecords(ctx, chatID, 0, 0)
	if err != nil {
		return chatEnvelope{Stats: chatSwitchHandoffStats{Mode: "fresh_start"}}
	}
	turns, actionRecords := chatTurnsAndActions(records)
	if len(turns) == 0 {
		return chatEnvelope{Stats: chatSwitchHandoffStats{Mode: "fresh_start"}}
	}
	budget := chatHandoffBudget(s.targetContextWindow(req.Model), targetModel(req))
	body, included, omitted, truncated := packConversationTurns(turns, budget)
	mode := "raw"
	if truncated || omitted > 0 {
		mode = "target_summary" // self-summarize instruction (Task-078 ladder; cached hybrid lands with the summary-ledger wiring)
	}
	prompt := renderHandoffPrompt(src.providerKey, chatID, "", mode, body)
	digest := renderActionsDigest(actionRecords, 8*1024)
	actionsIncluded := false
	if digest != "" {
		prompt += "\n\n<actions_summary>\n" + digest + "\n</actions_summary>"
		actionsIncluded = true
	}
	return chatEnvelope{Prompt: prompt, Stats: chatSwitchHandoffStats{
		Mode: mode, IncludedTurnCount: included, OmittedTurnCount: omitted,
		Truncated: truncated || omitted > 0, ActionsDigest: actionsIncluded,
	}}
}

func targetModel(req chatSwitchRequest) string { return req.Model }

// targetContextWindow returns the target model's context window for the
// envelope budget (SD-26 D-9). The model-metadata source (catalog/registry)
// lands with Task-315/316 wiring; until then 0 → the 64 KiB Task-078 floor
// applies (documented in CA-692).
func (s *InteractiveService) targetContextWindow(model string) int64 { return 0 }

// chatHandoffBudget derives the envelope budget from the target model's
// context window (~3 chars/token, conservative) with a 512 KiB hard cap; an
// unknown context falls back to the 64 KiB Task-078 floor (SD-26 D-9). A known
// small context uses its derived budget as-is — an 8k-context model must not
// receive a 64 KiB envelope. The context-lookup source lands with Task-315/316
// model metadata wiring; until then 0 → floor (documented in CA-692).
func chatHandoffBudget(contextTokens int64, _ string) int {
	if contextTokens <= 0 {
		return handoffMaxBytes
	}
	derived := contextTokens * 3
	if derived > chatSwitchHardCapBytes {
		return chatSwitchHardCapBytes
	}
	return int(derived)
}

// chatTurnsAndActions splits transcript records into conversation turns and
// action records (tools/files/approvals) for the digest.
func chatTurnsAndActions(records []ChatTranscriptRecord) ([]transcriptTurn, []ChatTranscriptRecord) {
	var turns []transcriptTurn
	var actions []ChatTranscriptRecord
	var current *transcriptTurn
	flush := func() {
		if current != nil && (strings.TrimSpace(current.User) != "" || strings.TrimSpace(current.Assistant) != "") {
			turns = append(turns, *current)
		}
		current = nil
	}
	for _, rec := range records {
		switch rec.Type {
		case EventTypeChatTurnStarted:
			var p struct {
				Prompt string `json:"prompt"`
			}
			if json.Unmarshal(rec.Payload, &p) == nil && strings.TrimSpace(p.Prompt) != "" {
				flush()
				current = &transcriptTurn{User: p.Prompt}
			}
		case EventTypeChatMessageCompleted:
			var p struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(rec.Payload, &p) != nil || strings.TrimSpace(p.Text) == "" {
				continue
			}
			if current == nil {
				current = &transcriptTurn{}
			}
			current.Assistant = appendTranscriptLine(current.Assistant, p.Text)
		case EventTypeChatProviderSwitch, EventTypeChatBackfillMarker:
			// switch boundaries stay out of the conversation body
			flush()
		default:
			actions = append(actions, rec)
		}
	}
	flush()
	return turns, actions
}

// renderActionsDigest compiles the `<actions_summary>` body from tool/file/
// approval records (SD-26 D-9; review I-3): what previous legs DID, not just
// what they said. Capped at maxBytes.
func renderActionsDigest(records []ChatTranscriptRecord, maxBytes int) string {
	if len(records) == 0 {
		return ""
	}
	var lines []string
	for _, rec := range records {
		var line string
		switch rec.Type {
		case EventTypeChatToolStarted:
			var p struct {
				Tool string `json:"tool"`
			}
			if json.Unmarshal(rec.Payload, &p) == nil && p.Tool != "" {
				line = "tool " + p.Tool + " started"
			}
		case EventTypeChatToolCompleted:
			var p struct {
				Tool   string `json:"tool"`
				Status string `json:"status"`
			}
			if json.Unmarshal(rec.Payload, &p) == nil && p.Tool != "" {
				line = "tool " + p.Tool + " -> " + p.Status
			}
		case EventTypeChatFileChanged:
			var p struct {
				Path string `json:"path"`
			}
			if json.Unmarshal(rec.Payload, &p) == nil && p.Path != "" {
				line = "file " + p.Path
			}
		case EventTypeChatApprovalRequested:
			var p struct {
				ID       string `json:"id"`
				Kind     string `json:"kind"`
				Command  string `json:"command"`
				Decision string `json:"decision"`
			}
			if json.Unmarshal(rec.Payload, &p) == nil && p.ID != "" {
				line = "approval " + p.ID
				if p.Decision != "" {
					line += " decision=" + p.Decision
				}
			}
		}
		if line != "" {
			lines = append(lines, "- "+line)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	out := strings.Join(lines, "\n")
	if len(out) > maxBytes {
		out = out[:maxBytes] + "\n- [earlier actions omitted]"
	}
	return out
}
