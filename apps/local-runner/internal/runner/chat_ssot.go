package runner

// Chat SSOT substrate (CP-59 / SD-26): chat identity, leg lifecycle constants,
// the FLOWPILOT_CHAT_SSOT gate, and the transcript capture hook.
//
// Everything here is inert while the flag is off (default): recordChatTranscript
// no-ops, no chat fields are stamped beyond zero values, and the timeline route
// is only registered when the flag is on (Task-313 T-6/T-7).

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Leg lifecycle states (SD-26 §5.2, SD26-S-1).
const (
	LegStateActive = "active"
	LegStateClosed = "closed"
)

// Leg closure reasons (SD-26 §5.2).
const (
	LegClosedReasonProviderSwitch           = "provider_switch"
	LegClosedReasonChatEnded                = "chat_ended"
	LegClosedReasonRestored                 = "restored"
	LegClosedReasonProviderSwitchRolledBack = "provider_switch_rolled_back"
)

// Chat transcript record types (SD-26 §6.1, SD26-E-1..E-9). The record
// vocabulary mirrors the normalized ProviderEvent stream minus deltas; the
// approval/question records carry the request card, with the replay-only
// Decision/Answer fields populated when an event replays a resolved gate.
const (
	EventTypeChatTurnStarted       = "turn_started"
	EventTypeChatMessageCompleted  = "message_completed"
	EventTypeChatToolStarted       = "tool_started"
	EventTypeChatToolCompleted     = "tool_completed"
	EventTypeChatFileChanged       = "file_changed"
	EventTypeChatApprovalRequested = "approval_requested"
	EventTypeChatQuestionAsked     = "question_asked"
	EventTypeChatTokenUsage        = "token_usage"
	EventTypeChatProviderSwitch    = "chat_provider_switch"
	// EventTypeChatBackfillMarker gates the one-shot legacy backfill (Task-313
	// DOD-8): its presence means raw turns were already synthesized from the
	// pre-flag run events; not part of the SD26-E contract families.
	EventTypeChatBackfillMarker = "chat_backfilled"
	// EventTypeChatSeedFailed records SD26-X-7: a committed switch whose seed
	// turn failed. The leg stays active and the chat continuable.
	EventTypeChatSeedFailed = "switch_seed_failed"
)

// chatSSOTEnvFlag gates every chat-SSOT behavior (SD-26 Key Decision D-10).
// Default off: flag-off behavior is byte-identical to the pre-CP-59 runner.
const chatSSOTEnvFlag = "FLOWPILOT_CHAT_SSOT"

func chatSSOTEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(chatSSOTEnvFlag)))
	return v == "1" || v == "true" || v == "yes"
}

// newChatID mints `cht_<12-hex>` (SD-26 §5.1, SD26-D-0). Machine-local
// uniqueness is sufficient: Drive sync namespaces manifests by project+chatId.
func newChatID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("cht_%d", time.Now().UnixNano())
	}
	return "cht_" + hex.EncodeToString(b)
}

// ensureChatTagging normalizes a legacy chat-kind run to the SSOT identity
// (SD-26 §5.1): chatId=runId, legSeq=0, legState=active. Lazy and idempotent —
// called on load, timeline read, and switch entry. Caller holds s.mu.
func ensureChatTagging(rs *interactiveRun) {
	if rs == nil || rs.runKind != "chat" || rs.chatID != "" {
		return
	}
	rs.chatID = rs.id
	rs.legSeq = 0
	if rs.legState == "" {
		rs.legState = LegStateActive
	}
}

// resolveChatIdentity resolves the (chatId, legSeq, switchFromRunID) triple for
// a normal_chat StartRunInput BEFORE createRun takes s.mu — the same
// resolve-before-lock pattern as stampAccount (interactive_handlers.go:792),
// because adoption may read resident runs. Precedence (SD-26 §5.1):
//  1. explicit ChatID (+LegSeq when the caller computed it, e.g. Task-314's
//     phase-A lock) — trusted as-is;
//  2. explicit ChatID without LegSeq → next leg after the resident max;
//  3. SwitchFromRunID → adopt a resident source leg's chatId at legSeq+1;
//  4. otherwise a fresh chat mints.
//
// Caller must NOT hold s.mu.
func (s *InteractiveService) resolveChatIdentity(in StartRunInput) (string, int, string) {
	if in.ChatID != "" {
		if in.LegSeq > 0 {
			return in.ChatID, in.LegSeq, in.SwitchFromRunID
		}
		s.mu.Lock()
		maxSeq := 0
		for _, rs := range s.runs {
			if rs.chatID == in.ChatID && rs.legSeq > maxSeq {
				maxSeq = rs.legSeq
			}
		}
		s.mu.Unlock()
		return in.ChatID, maxSeq + 1, in.SwitchFromRunID
	}
	if in.SwitchFromRunID != "" {
		s.mu.Lock()
		src := s.runs[in.SwitchFromRunID]
		if src != nil && src.runKind == "chat" {
			ensureChatTagging(src)
			chatID, legSeq, switchFrom := src.chatID, src.legSeq+1, in.SwitchFromRunID
			s.mu.Unlock()
			return chatID, legSeq, switchFrom
		}
		s.mu.Unlock()
	}
	return newChatID(), 0, ""
}

// chatRunRegistry maps runID → chatID for the transcript capture path
// (SD-26 §5.3) and tracks the per-run degraded flag (SD26-X-6). It exists
// because persistEvent call sites hold varying lock state; the registry has
// its own mutex so capture never touches s.mu (deadlock-proof by design).
// Entries are written at createRun (mint/adopt), at ensureChatTagging call
// sites, and at createRun's persistence failure path; a run's chatID is
// immutable once registered.
type chatRunRegistry struct {
	mu       sync.RWMutex
	byRun    map[string]string
	degraded map[string]bool
}

func newChatRunRegistry() *chatRunRegistry {
	return &chatRunRegistry{byRun: map[string]string{}, degraded: map[string]bool{}}
}

func (r *chatRunRegistry) register(runID, chatID string) {
	if runID == "" || chatID == "" {
		return
	}
	r.mu.Lock()
	r.byRun[runID] = chatID
	r.mu.Unlock()
}

func (r *chatRunRegistry) chatIDFor(runID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byRun[runID]
}

// setDegraded flips the SD26-X-6 transcript-degraded flag and reports whether
// the flag changed (so the caller logs the one-time notice).
func (r *chatRunRegistry) setDegraded(runID string, degraded bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	prev := r.degraded[runID]
	if prev != degraded {
		r.degraded[runID] = degraded
	}
	return prev != degraded
}

func (r *chatRunRegistry) isDegraded(runID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.degraded[runID]
}

// recordChatTranscript mirrors persistEvent's skip policy exactly (SD-26 §5.3):
// same delta skip, same call point (persistEvent, interactive_service.go:3947).
// Flag-gated no-op otherwise. A transcript append failure never fails the turn
// (SD26-X-6): the run is flagged degraded in the registry and one log line is
// emitted; timeline responses carry degraded until a write succeeds again.
// Never touches s.mu — safe from any persistEvent call site.
func (s *InteractiveService) recordChatTranscript(event ProviderEvent) {
	writer := s.ensureChatTranscriptWriter()
	if writer == nil || s.chatRuns == nil {
		return
	}
	if event.Type == EventMessageDelta { // same skip as persistEvent
		return
	}
	runID := event.WorkflowRunID
	chatID := s.chatRuns.chatIDFor(runID)
	if chatID == "" {
		return
	}
	recs := chatRecordsFromProviderEvent(chatID, event)
	if len(recs) == 0 {
		return
	}
	if err := writer.append(context.Background(), recs...); err != nil {
		if s.chatRuns.setDegraded(runID, true) {
			fmt.Printf("[chat-ssot] transcript append failed run=%s: %v — marked degraded\n", runID, err)
		}
		return
	}
	s.chatRuns.setDegraded(runID, false)
}

// chatRecordsFromProviderEvent maps one normalized ProviderEvent to zero or
// more chat transcript records (SD-26 §6.1, SD26-E-1..E-9). Exhaustive over the
// non-delta event vocabulary; unknown types record nothing (forward compatible).
// EventTurnCompleted's FinalMessage is recorded even when it repeats the last
// message_completed text — the timeline read collapses consecutive identical
// texts for presentation, records stay raw.
func chatRecordsFromProviderEvent(chatID string, event ProviderEvent) []ChatTranscriptRecord {
	runID := event.WorkflowRunID
	mk := func(typ string, payload any) ChatTranscriptRecord {
		raw, err := json.Marshal(payload)
		if err != nil {
			raw = []byte("{}")
		}
		return ChatTranscriptRecord{ChatID: chatID, LegRunID: runID, Type: typ, Payload: raw}
	}
	switch event.Type {
	case EventTurnStarted:
		return []ChatTranscriptRecord{mk(EventTypeChatTurnStarted, map[string]any{"prompt": event.Prompt})}
	case EventMessageCompleted:
		return []ChatTranscriptRecord{mk(EventTypeChatMessageCompleted, map[string]any{"text": event.Text})}
	case EventTurnCompleted:
		if strings.TrimSpace(event.FinalMessage) == "" {
			return nil
		}
		return []ChatTranscriptRecord{mk(EventTypeChatMessageCompleted, map[string]any{"text": event.FinalMessage})}
	case EventToolStarted:
		return []ChatTranscriptRecord{mk(EventTypeChatToolStarted, map[string]any{"tool": event.ToolName})}
	case EventToolCompleted:
		return []ChatTranscriptRecord{mk(EventTypeChatToolCompleted, map[string]any{"tool": event.ToolName, "status": event.Status})}
	case EventFileChanged:
		return []ChatTranscriptRecord{mk(EventTypeChatFileChanged, map[string]any{"path": event.Path, "changeType": event.ChangeType})}
	case EventPermissionRequired:
		payload := map[string]any{"id": event.ApprovalID}
		if event.Details != nil {
			payload["kind"] = event.Details.Kind
			payload["command"] = event.Details.Command
		}
		if event.Decision != "" { // replay of an already-resolved gate
			payload["decision"] = event.Decision
		}
		return []ChatTranscriptRecord{mk(EventTypeChatApprovalRequested, payload)}
	case EventUserQuestionRequired:
		payload := map[string]any{"id": event.QuestionID, "prompt": event.Prompt}
		if len(event.Answer) > 0 { // replay of an already-answered question
			payload["answer"] = event.Answer
		}
		return []ChatTranscriptRecord{mk(EventTypeChatQuestionAsked, payload)}
	case EventTokenUsageUpdated:
		if event.TokenUsage == nil {
			return nil
		}
		return []ChatTranscriptRecord{mk(EventTypeChatTokenUsage, event.TokenUsage)}
	default:
		return nil
	}
}
