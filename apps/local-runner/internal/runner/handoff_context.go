package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
	"flowpilot-runner/internal/promptblock"
)

const handoffMaxBytes = 64 * 1024

// handoffPromptPrefix is the leading marker of every handoff envelope (see
// renderHandoffPrompt). The runner matches it to recognise a handoff turn so
// feature resolution does NOT resolve on the envelope text — which embeds the
// prior conversation and gate-reprompt lines that name feature keys and would
// otherwise mis-resolve the target run's first turn.
const handoffPromptPrefix = "[FlowPilot cross-provider chat handoff]"

type handoffContextRequest struct {
	TargetProviderKey ProviderKey `json:"targetProviderKey"`
	MaxBytes          int         `json:"maxBytes,omitempty"`
}

type handoffContextResponse struct {
	SourceRunID       string      `json:"sourceRunId"`
	SourceProviderKey ProviderKey `json:"sourceProviderKey"`
	TargetProviderKey ProviderKey `json:"targetProviderKey"`
	Prompt            string      `json:"prompt"`
	IncludedTurnCount int         `json:"includedTurnCount"`
	OmittedTurnCount  int         `json:"omittedTurnCount"`
	Truncated         bool        `json:"truncated"`
	HandoffMode       string      `json:"handoffMode"`
}

type transcriptTurn struct {
	User      string
	Assistant string
}

func (s *InteractiveService) handleHandoffContext(w http.ResponseWriter, r *http.Request) {
	var body handoffContextRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	result, apiErr := s.buildHandoffContext(r.Context(), r.PathValue("runId"), body)
	if apiErr != nil {
		writeInteractiveError(w, apiErr)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, result)
}

func (s *InteractiveService) buildHandoffContext(_ context.Context, runID string, body handoffContextRequest) (handoffContextResponse, *apiErr) {
	rs, apiErr := s.loadHandoffSourceRun(runID)
	if apiErr != nil {
		return handoffContextResponse{}, apiErr
	}
	if body.TargetProviderKey == "" {
		return handoffContextResponse{}, newAPIErr(http.StatusBadRequest, "invalid_request", "targetProviderKey is required")
	}
	if rs.providerKey == body.TargetProviderKey {
		return handoffContextResponse{}, newAPIErr(http.StatusConflict, "handoff_same_provider", "same-provider continuity should use the existing resume path")
	}
	if rs.runKind != "chat" {
		return handoffContextResponse{}, newAPIErr(http.StatusConflict, "handoff_run_kind_unsupported", "only chat runs can be handed off")
	}
	if rs.turnInFlight || rs.pendingApprovalID != "" || rs.pendingQuestionID != "" {
		return handoffContextResponse{}, newAPIErr(http.StatusConflict, "handoff_run_busy", "cannot hand off while the source run is active")
	}
	if !supportsHandoffSource(rs.providerKey) {
		return handoffContextResponse{}, newAPIErr(http.StatusUnprocessableEntity, "handoff_source_provider_unsupported", "source provider has no transcript extractor")
	}

	turns := transcriptTurnsFromRun(rs)
	if len(turns) == 0 {
		return handoffContextResponse{}, newAPIErr(http.StatusUnprocessableEntity, "handoff_context_unavailable", "no usable user/assistant content could be reconstructed")
	}

	maxBytes := body.MaxBytes
	if maxBytes <= 0 || maxBytes > handoffMaxBytes {
		maxBytes = handoffMaxBytes
	}
	summaryText := strings.TrimSpace(s.loadHandoffSummary(rs, turns))
	conversation, included, omitted, truncated := packConversationTurns(turns, maxBytes)

	// Fallback ladder (Task-162 T-1): hybrid when a state-matched cached summary
	// exists; otherwise target_summary when the raw floor had to drop or truncate
	// history (the target benefits from self-summarizing what it received and
	// minding the gaps); otherwise raw, since the whole conversation is present
	// and there is nothing left to summarize. Raw is always the underlying floor.
	mode := "raw"
	switch {
	case summaryText != "":
		mode = "hybrid"
	case omitted > 0 || truncated:
		mode = "target_summary"
	}

	prompt := renderHandoffPrompt(rs.providerKey, rs.id, summaryText, mode, conversation)
	// Prepend the SOURCE feature's history (resolved from the clean source transcript,
	// which skips system/gate-reprompt turns — not the envelope text, which names
	// feature keys and would mis-resolve). This keeps the target's first turn in
	// Task-157/161 context (D-1 / V-162-05), correctly attributed. The per-turn
	// injection seam skips handoff prompts so this block is never re-resolved.
	if block := s.handoffFeatureBlocks(rs, turns); block != "" {
		prompt = block + "\n\n---\n\n" + prompt
	}
	return handoffContextResponse{
		SourceRunID:       rs.id,
		SourceProviderKey: rs.providerKey,
		TargetProviderKey: body.TargetProviderKey,
		Prompt:            prompt,
		IncludedTurnCount: included,
		OmittedTurnCount:  omitted,
		Truncated:         truncated || omitted > 0,
		HandoffMode:       mode,
	}, nil
}

func (s *InteractiveService) loadHandoffSourceRun(runID string) (*interactiveRun, *apiErr) {
	s.mu.Lock()
	rs := s.runs[runID]
	s.mu.Unlock()
	if rs != nil {
		return rs, nil
	}

	rebuilt, err := s.loadPersistedRun(runID)
	if err != nil {
		return nil, err
	}
	s.seedTranscriptFromDisk(rebuilt)
	return rebuilt, nil
}

func supportsHandoffSource(key ProviderKey) bool {
	switch key {
	case ProviderKeyClaude, ProviderKeyCodex:
		return true
	// ProviderKeyGrok deliberately falls to default (CP-46 Task-212 T-5,
	// Open Question Q-7): a ~/.grok/sessions SQLite transcript extractor was
	// not attempted in this pass, so Grok stays handoff-TARGET-only, never a
	// source, until one is built and proven. Do not flip this without adding
	// the extractor + a passing test.
	default:
		return false
	}
}

func transcriptTurnsFromRun(rs *interactiveRun) []transcriptTurn {
	if rs == nil {
		return nil
	}
	var turns []transcriptTurn
	var current *transcriptTurn

	flush := func() {
		if current == nil {
			return
		}
		if strings.TrimSpace(current.User) != "" || strings.TrimSpace(current.Assistant) != "" {
			turns = append(turns, *current)
		}
		current = nil
	}

	for _, ev := range rs.events {
		switch ev.Type {
		case EventTurnStarted:
			if strings.TrimSpace(ev.Prompt) == "" {
				continue
			}
			flush()
			current = &transcriptTurn{User: ev.Prompt}
		case EventMessageCompleted:
			if current == nil {
				continue
			}
			text := strings.TrimSpace(ev.Text)
			if text == "" {
				continue
			}
			current.Assistant = appendTranscriptLine(current.Assistant, text)
		case EventTurnCompleted:
			if current == nil {
				continue
			}
			text := strings.TrimSpace(ev.FinalMessage)
			if text == "" {
				continue
			}
			current.Assistant = appendTranscriptLine(current.Assistant, text)
		case EventTurnFailed:
			flush()
		}
	}
	flush()
	return turns
}

func appendTranscriptLine(existing, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	if next == "" {
		return existing
	}
	if existing == "" {
		return next
	}
	if strings.Contains(existing, next) {
		return existing
	}
	return existing + "\n" + next
}

// handoffFeatureBlocks resolves the source conversation's feature from its clean
// transcript (resolveTurnsFeature skips system/gate-reprompt turns) and returns
// that feature's history block, or "" if none resolves. Used to seed the handoff
// target's first turn with the right feature context.
func (s *InteractiveService) handoffFeatureBlocks(rs *interactiveRun, turns []transcriptTurn) string {
	if rs == nil || rs.workspaceCwd == "" {
		return ""
	}
	dotFP := filepath.Join(rs.workspaceCwd, ".flowpilot")
	catalog, err := featurecatalog.LoadCatalog(dotFP)
	if err != nil {
		return ""
	}
	top, ok := resolveTurnsFeature(turns, catalog)
	if !ok {
		return ""
	}
	return composeFeatureBlocks(dotFP, top.Key)
}

func (s *InteractiveService) loadHandoffSummary(rs *interactiveRun, turns []transcriptTurn) string {
	if rs == nil || rs.workspaceCwd == "" {
		return ""
	}
	dotFP := filepath.Join(rs.workspaceCwd, ".flowpilot")
	catalog, err := featurecatalog.LoadCatalog(dotFP)
	if err != nil {
		return ""
	}
	top, ok := resolveTurnsFeature(turns, catalog)
	if !ok {
		return ""
	}
	ledger, err := changeledger.NewChatSummaryLedger(dotFP)
	if err != nil {
		return ""
	}
	summaries, err := ledger.GetFeatureSummariesForRun(top.Key, rs.id)
	if err == nil && len(summaries) > 0 {
		// The recorder hashed the feature-bucketed turns, not the whole transcript,
		// so match that set here — otherwise the state_key never agrees and hybrid
		// silently degrades to raw for any chat with off-feature turns.
		expectedStateKey := transcriptStateKey(rs.id, featureBucketTurns(turns, catalog, top.Key))
		for i := len(summaries) - 1; i >= 0; i-- {
			if summaries[i].StateKey != expectedStateKey {
				continue
			}
			if summary := strings.TrimSpace(summaries[i].Summary); summary != "" {
				return summary
			}
		}
	}
	return ""
}

// renderHandoffPrompt wraps an already-packed raw conversation in the stable
// handoff envelope. The summary block (hybrid) or the self-summarize instruction
// (target_summary) is layered on top of the raw floor per handoffMode; raw mode
// adds neither.
func renderHandoffPrompt(sourceProvider ProviderKey, sourceRunID string, summaryText string, handoffMode string, conversation string) string {
	var sb strings.Builder
	sb.WriteString(handoffPromptPrefix + "\n\n")
	sb.WriteString("This is conversation history from a different AI provider and a different\n")
	sb.WriteString("provider session. Treat it as background context only. Do not claim that you\n")
	sb.WriteString("performed the previous assistant's actions. Historical assistant messages may\n")
	sb.WriteString("be incomplete or incorrect.\n\n")
	sb.WriteString("Source provider: ")
	sb.WriteString(string(sourceProvider))
	sb.WriteString("\n")
	sb.WriteString("Source run: ")
	sb.WriteString(sourceRunID)
	sb.WriteString("\n\n")
	if strings.TrimSpace(summaryText) != "" {
		sb.WriteString("<conversation_summary>\n")
		sb.WriteString(promptblock.EscapeClosingTag(strings.TrimSpace(summaryText), "conversation_summary"))
		sb.WriteString("\n</conversation_summary>\n\n")
	} else if handoffMode == "target_summary" {
		sb.WriteString("No cached FlowPilot summary was available, and older turns were dropped to fit the handoff size limit. First summarize the previous conversation for yourself, then continue from the latest user intent.\n\n")
	}
	sb.WriteString("<previous_conversation>\n")
	sb.WriteString(conversation)
	sb.WriteString("\n</previous_conversation>")
	return sb.String()
}

func packConversationTurns(turns []transcriptTurn, maxBytes int) (string, int, int, bool) {
	if len(turns) == 0 {
		return "", 0, 0, false
	}
	omitMarker := "[Earlier conversation omitted due to handoff size limit]"
	turnMarker := "[turn truncated due to handoff size limit]"
	effectiveBudget := maxBytes - len(omitMarker) - 4
	if effectiveBudget <= 0 {
		effectiveBudget = maxBytes
	}

	blocks := make([]string, 0, len(turns))
	used := 0
	included := 0
	omitted := 0
	truncated := false

	for i := len(turns) - 1; i >= 0; i-- {
		remaining := effectiveBudget - used
		if remaining <= 0 {
			omitted = i + 1
			break
		}
		block, blockTruncated := renderConversationTurn(turns[i], remaining, turnMarker)
		if len(block) <= remaining {
			blocks = append([]string{block}, blocks...)
			used += len(block)
			included++
			truncated = truncated || blockTruncated
			continue
		}
		if len(blocks) == 0 {
			blocks = []string{block}
			included = 1
			truncated = true
		}
		omitted = i
		break
	}

	body := strings.Join(blocks, "\n\n")
	if omitted > 0 {
		if body != "" {
			body = omitMarker + "\n\n" + body
		} else {
			body = omitMarker
		}
	}
	return body, included, omitted, truncated
}

func renderConversationTurn(turn transcriptTurn, maxBytes int, truncatedMarker string) (string, bool) {
	if maxBytes <= 0 {
		return truncatedMarker, true
	}
	user := promptblock.EscapeClosingTag(strings.TrimSpace(turn.User), "previous_conversation")
	assistant := promptblock.EscapeClosingTag(strings.TrimSpace(turn.Assistant), "previous_conversation")

	var sb strings.Builder
	sb.WriteString("User:\n")
	remaining := maxBytes - sb.Len()
	if remaining <= 0 {
		return truncatedMarker, true
	}
	userCap := remaining
	if assistant != "" {
		userCap -= len("\n\nAssistant:\n")
	}
	if userCap < 0 {
		userCap = 0
	}
	userText, userTruncated := promptblock.TruncateUTF8(user, userCap)
	sb.WriteString(userText)
	if userTruncated {
		return appendTurnMarker(sb.String(), truncatedMarker, maxBytes), true
	}
	if assistant == "" {
		return appendTurnMarker(sb.String(), truncatedMarker, maxBytes), false
	}

	sb.WriteString("\n\nAssistant:\n")
	remaining = maxBytes - sb.Len()
	if remaining <= 0 {
		return appendTurnMarker(sb.String(), truncatedMarker, maxBytes), true
	}
	assistantText, assistantTruncated := promptblock.TruncateUTF8(assistant, remaining)
	sb.WriteString(assistantText)
	if assistantTruncated {
		return appendTurnMarker(sb.String(), truncatedMarker, maxBytes), true
	}
	return sb.String(), false
}

func appendTurnMarker(text, marker string, maxBytes int) string {
	text = strings.TrimSpace(text)
	if marker == "" {
		return text
	}
	candidate := text + "\n" + marker
	if len(candidate) <= maxBytes {
		return candidate
	}
	if len(text) >= maxBytes {
		truncated, _ := promptblock.TruncateUTF8(text, maxBytes)
		return truncated
	}
	remaining := maxBytes - len(text) - 1
	if remaining <= 0 {
		return text
	}
	if len(marker) <= remaining {
		return text + "\n" + marker
	}
	return text
}
