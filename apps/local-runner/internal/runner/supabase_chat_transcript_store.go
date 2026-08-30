package runner

// Supabase chat transcript store (CP-59 / SD-26 §5.3, Task-313 T-4): the
// workflow_chat_events backing for ChatTranscriptStore when the runner is
// Supabase-backed. Append is idempotent on unique(chat_id, chat_seq) via
// PostgREST resolution=ignore-duplicates — restore replays act as upserts.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// providerSessionSelectChat lists the columns for chat-leg discovery. Kept
// separate from providerSessionSelectRecovery so recovery reads never require
// the chat columns to exist (pre-migration deployments stay untouched).
const providerSessionSelectChat = "run_id,project_id,provider_key,provider_session_id,status,started_at,updated_at,run_kind,chat_id,leg_seq,leg_state,leg_closed_reason,switch_from_run_id"

// AppendChatRecords implements ChatTranscriptStore. Idempotent on
// (chat_id, chat_seq): duplicates are ignored server-side by the unique key.
func (s *SupabaseWorkflowStore) AppendChatRecords(ctx context.Context, recs []ChatTranscriptRecord) error {
	if len(recs) == 0 {
		return nil
	}
	body, err := json.Marshal(recs)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/workflow_chat_events?on_conflict=chat_id,chat_seq", s.restURL)
	code, respBody, err := httpRequestFn(ctx, http.MethodPost, endpoint, s.headers("resolution=ignore-duplicates"), body)
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("supabase append chat records failed: status %d: %s", code, string(respBody))
	}
	return nil
}

// ReadChatRecords returns the chat's records after afterSeq in ascending
// chatSeq order (limit 0 = unbounded).
func (s *SupabaseWorkflowStore) ReadChatRecords(ctx context.Context, chatID string, afterSeq int64, limit int) ([]ChatTranscriptRecord, error) {
	endpoint := fmt.Sprintf(
		"%s/workflow_chat_events?select=chat_id,chat_seq,leg_run_id,type,payload&chat_id=eq.%s&chat_seq=gt.%d&order=chat_seq.asc",
		s.restURL, chatID, afterSeq,
	)
	if limit > 0 {
		endpoint += fmt.Sprintf("&limit=%d", limit)
	}
	code, body, err := httpRequestFn(ctx, http.MethodGet, endpoint, s.headers(""), nil)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("supabase read chat records failed: status %d: %s", code, string(body))
	}
	out := make([]ChatTranscriptRecord, 0, 32)
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("supabase read chat records decode: %w", err)
	}
	return out, nil
}

// LatestChatSeq returns the chat's max chatSeq (0 when the chat is unknown).
func (s *SupabaseWorkflowStore) LatestChatSeq(ctx context.Context, chatID string) (int64, error) {
	endpoint := fmt.Sprintf(
		"%s/workflow_chat_events?select=chat_seq&chat_id=eq.%s&order=chat_seq.desc&limit=1",
		s.restURL, chatID,
	)
	code, body, err := httpRequestFn(ctx, http.MethodGet, endpoint, s.headers(""), nil)
	if err != nil {
		return 0, err
	}
	if code < 200 || code >= 300 {
		return 0, fmt.Errorf("supabase latest chat seq failed: status %d: %s", code, string(body))
	}
	var rows []struct {
		ChatSeq int64 `json:"chat_seq"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return 0, fmt.Errorf("supabase latest chat seq decode: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].ChatSeq, nil
}

// ListProviderSessionsByChat implements ChatSessionReader (SD-26 §6.2):
// persisted leg discovery after a restart. Rows missing the chat columns
// (pre-migration) simply never match chat_id — legacy chats self-tag in memory.
func (s *SupabaseWorkflowStore) ListProviderSessionsByChat(ctx context.Context, chatID string) ([]ProviderSessionState, error) {
	endpoint := fmt.Sprintf(
		"%s/workflow_provider_sessions?select=%s&chat_id=eq.%s&order=leg_seq.asc",
		s.restURL, providerSessionSelectChat, chatID,
	)
	code, body, err := httpRequestFn(ctx, http.MethodGet, endpoint, s.headers(""), nil)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("supabase list sessions by chat failed: status %d: %s", code, string(body))
	}
	var rows []dbProviderSessionRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("supabase list sessions by chat decode: %w", err)
	}
	out := make([]ProviderSessionState, 0, len(rows))
	for _, r := range rows {
		out = append(out, providerSessionFromDBRow(r))
	}
	return out, nil
}
