package runner

// Chat timeline read model (CP-59 / SD-26 §6.2, Task-313 T-6): joins a chat's
// legs (resident runs + persisted session rows) with its transcript records
// into one ordered view. Flag-gated: with FLOWPILOT_CHAT_SSOT off the route
// answers typed 404 and nothing else in the service changes.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ChatSessionReader (SD-26 §6.2): optional store capability to discover a
// chat's persisted legs after a restart. fakeWorkflowStore implements it
// in-memory (localFileSessionStore inherits the embedding); the Supabase store
// implements it with a chat_id query. Absent → only resident legs are listed.
type ChatSessionReader interface {
	ListProviderSessionsByChat(ctx context.Context, chatID string) ([]ProviderSessionState, error)
}

type chatLegView struct {
	RunID           string `json:"runId"`
	ProviderKey     string `json:"providerKey"`
	LegSeq          int    `json:"legSeq"`
	LegState        string `json:"legState"`
	LegClosedReason string `json:"legClosedReason,omitempty"`
	Status          string `json:"status,omitempty"`
}

type chatTimelineResponse struct {
	ChatID    string                 `json:"chatId"`
	Legs      []chatLegView          `json:"legs"`
	Records   []ChatTranscriptRecord `json:"records"`
	NextSeq   int64                  `json:"nextSeq"`
	Truncated bool                   `json:"truncated"`
	Degraded  bool                   `json:"degraded"`
}

// ensureChatTranscriptWriter lazily builds the capture stack when
// FLOWPILOT_CHAT_SSOT is on (SD-26 Key Decision D-10). Returns nil when the
// flag is off — every caller treats nil as "capture disabled". The store
// directory defaults to ~/.flowpilot/chat-transcripts and is overridable via
// FLOWPILOT_CHAT_STORE_DIR (tests / per-worktree isolation).
func (s *InteractiveService) ensureChatTranscriptWriter() *chatTranscriptWriter {
	if !chatSSOTEnabled() {
		return nil
	}
	s.chatOnce.Do(func() {
		s.chatRuns = newChatRunRegistry()
		dir := strings.TrimSpace(os.Getenv("FLOWPILOT_CHAT_STORE_DIR"))
		if dir == "" {
			if home, err := os.UserHomeDir(); err == nil {
				dir = filepath.Join(home, ".flowpilot", "chat-transcripts")
			}
		}
		if dir == "" {
			return
		}
		s.chatTranscripts = newChatTranscriptWriter(newLocalFileChatTranscriptStore(dir))
	})
	return s.chatTranscripts
}

func (s *InteractiveService) handleChatTimeline(w http.ResponseWriter, r *http.Request) {
	if !chatSSOTEnabled() {
		writeInteractiveError(w, newAPIErr(http.StatusNotFound, "chat_ssot_disabled", "chat SSOT is disabled (FLOWPILOT_CHAT_SSOT)"))
		return
	}
	chatID := strings.TrimSpace(r.PathValue("chatId"))
	if chatID == "" {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "chatId is required"))
		return
	}
	var afterSeq int64
	if v := strings.TrimSpace(r.URL.Query().Get("afterSeq")); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil && parsed >= 0 {
			afterSeq = parsed
		}
	}
	limit := 0
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	legs := map[string]chatLegView{}
	degraded := false
	s.mu.Lock()
	for _, rs := range s.runs {
		// Chat-kind gate is absolute (SD-26): a workflow run never joins a
		// chat timeline even if it somehow carries the chatId.
		if rs.runKind != "chat" {
			continue
		}
		if rs.chatID != chatID && rs.id != chatID {
			continue
		}
		ensureChatTagging(rs)
		if rs.chatID != chatID {
			continue
		}
		if s.chatRuns != nil {
			s.chatRuns.register(rs.id, rs.chatID)
		}
		legs[rs.id] = chatLegView{
			RunID:           rs.id,
			ProviderKey:     string(rs.providerKey),
			LegSeq:          rs.legSeq,
			LegState:        rs.legState,
			LegClosedReason: rs.legClosedReason,
			Status:          string(rs.status),
		}
		degraded = degraded || (s.chatRuns != nil && s.chatRuns.isDegraded(rs.id))
	}
	s.mu.Unlock()
	if reader, ok := s.workflowStore.(ChatSessionReader); ok {
		// Persisted rows may satisfy ChatSessionReader without the registry
		// existing (flag just turned on); guard anyway.
		if s.chatRuns != nil || len(legs) > 0 {
			if rows, err := reader.ListProviderSessionsByChat(r.Context(), chatID); err == nil {
				for _, row := range rows {
					if _, have := legs[row.RunID]; have {
						continue
					}
					state := row.LegState
					if state == "" {
						state = LegStateActive
					}
					legs[row.RunID] = chatLegView{
						RunID:           row.RunID,
						ProviderKey:     string(row.ProviderKey),
						LegSeq:          row.LegSeq,
						LegState:        state,
						LegClosedReason: row.LegClosedReason,
						Status:          string(row.Status),
					}
				}
			}
		}
	}
	if len(legs) == 0 {
		writeInteractiveError(w, newAPIErr(http.StatusNotFound, "chat_not_found", "no legs found for chat"))
		return
	}
	ordered := make([]chatLegView, 0, len(legs))
	for _, leg := range legs {
		ordered = append(ordered, leg)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].LegSeq < ordered[j].LegSeq })

	records := []ChatTranscriptRecord{}
	var nextSeq int64
	truncated := false
	if writer := s.ensureChatTranscriptWriter(); writer != nil {
		recs, err := writer.store.ReadChatRecords(r.Context(), chatID, afterSeq, limit)
		if err != nil {
			writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "chat_timeline_unavailable", err.Error()))
			return
		}
		records = collapseRepeatedFinals(recs)
		if len(records) > 0 {
			nextSeq = records[len(records)-1].ChatSeq
		} else {
			nextSeq = afterSeq
		}
		truncated = limit > 0 && len(recs) == limit
	}
	writeInteractiveJSON(w, http.StatusOK, chatTimelineResponse{
		ChatID:    chatID,
		Legs:      ordered,
		Records:   records,
		NextSeq:   nextSeq,
		Truncated: truncated,
		Degraded:  degraded,
	})
}

// collapseRepeatedFinals drops a message_completed record whose text equals the
// immediately preceding message_completed text on the same leg: the capture
// mapper records both EventMessageCompleted and EventTurnCompleted, and the
// echo would duplicate the final text. Raw records stay on disk; the read model
// collapses (SD-26 §6.1 note).
func collapseRepeatedFinals(recs []ChatTranscriptRecord) []ChatTranscriptRecord {
	out := make([]ChatTranscriptRecord, 0, len(recs))
	lastTextByLeg := map[string]string{}
	for _, rec := range recs {
		if rec.Type == EventTypeChatMessageCompleted {
			var p struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(rec.Payload, &p) == nil && p.Text != "" {
				if lastTextByLeg[rec.LegRunID] == p.Text {
					continue
				}
				lastTextByLeg[rec.LegRunID] = p.Text
			}
		}
		out = append(out, rec)
	}
	return out
}
