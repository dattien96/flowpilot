package runner

// Run-scoped timeline (BUG-355 F2): transcript read model for chat-less
// workflow runs, persisted under the run id (see recordChatTranscript).
// Same record shape as the chat timeline; legs carry just this run.

import (
	"context"
	"net/http"
	"strconv"
	"strings"
)

func (s *InteractiveService) handleGetRunTimeline(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("runId"))
	if runID == "" {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "runId is required"))
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
	if rs := s.runs[runID]; rs != nil {
		legs[rs.id] = chatLegView{
			RunID:           rs.id,
			ProviderKey:     string(rs.providerKey),
			LegSeq:          rs.legSeq,
			LegState:        rs.legState,
			LegClosedReason: rs.legClosedReason,
			Status:          string(rs.status),
		}
		degraded = s.chatRuns != nil && s.chatRuns.isDegraded(rs.id)
	}
	s.mu.Unlock()
	if len(legs) == 0 {
		// Not resident (e.g. /open after a restart): fall back to the
		// persisted session row for leg identity. Transcript records are
		// keyed by run id on disk, so they survive without residency.
		if reader, ok := s.workflowStore.(SessionHistoryReader); ok {
			if sess, found, err := reader.GetProviderSession(context.Background(), runID); err == nil && found {
				legs[sess.RunID] = chatLegView{
					RunID:           sess.RunID,
					ProviderKey:     string(sess.ProviderKey),
					LegSeq:          sess.LegSeq,
					LegState:        sess.LegState,
					LegClosedReason: sess.LegClosedReason,
					Status:          string(sess.Status),
				}
			}
		}
	}
	if len(legs) == 0 {
		writeInteractiveError(w, newAPIErr(http.StatusNotFound, "run_not_found", "no run found for id"))
		return
	}
	ordered := make([]chatLegView, 0, len(legs))
	for _, leg := range legs {
		ordered = append(ordered, leg)
	}

	// Same one-shot legacy synthesis as the chat timeline: a run that went
	// through turns before this fix (or with recording disabled) gets its
	// in-memory events synthesized once, marker-guarded, under the run key.
	s.backfillLegacyChatTranscript(r.Context(), runID, ordered)

	records := []ChatTranscriptRecord{}
	var nextSeq int64
	truncated := false
	if writer := s.ensureChatTranscriptWriter(); writer != nil {
		recs, err := writer.store.ReadChatRecords(r.Context(), runID, afterSeq, limit)
		if err != nil {
			writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "run_timeline_unavailable", err.Error()))
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
		ChatID:    "",
		Legs:      ordered,
		Records:   records,
		NextSeq:   nextSeq,
		Truncated: truncated,
		Degraded:  degraded,
	})
}
