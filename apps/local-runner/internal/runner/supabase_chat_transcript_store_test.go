package runner

// Live Supabase round-trip for the chat transcript store (Task-313 DOD-4).
// Creds-gated: requires SUPABASE_API_URL + SUPABASE_SERVICE_ROLE_KEY in the
// process env (e.g. sourced from .env.dev) AND the applied migration
// 20260830080000 (workflow_chat_events). Skips cleanly otherwise.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSupabaseChatTranscriptStoreRoundTrip(t *testing.T) {
	apiURL := strings.TrimSpace(os.Getenv("SUPABASE_API_URL"))
	key := strings.TrimSpace(os.Getenv("SUPABASE_SERVICE_ROLE_KEY"))
	if apiURL == "" || key == "" {
		t.Skip("SUPABASE_API_URL / SUPABASE_SERVICE_ROLE_KEY not set — source .env.dev to run the live round-trip")
	}
	store := NewSupabaseWorkflowStore(SupabaseWorkspaceConfig{APIURL: apiURL}, key)
	ctx := context.Background()
	chatID := fmt.Sprintf("cht_test_%d", time.Now().UnixNano())

	payload := func(text string) json.RawMessage { b, _ := json.Marshal(map[string]any{"text": text}); return b }
	recs := []ChatTranscriptRecord{
		{ChatID: chatID, ChatSeq: 1, LegRunID: "run-t", Type: EventTypeChatTurnStarted, Payload: payload("t1")},
		{ChatID: chatID, ChatSeq: 2, LegRunID: "run-t", Type: EventTypeChatMessageCompleted, Payload: payload("a1")},
		{ChatID: chatID, ChatSeq: 3, LegRunID: "run-t", Type: EventTypeChatTokenUsage, Payload: payload("u1")},
	}
	defer func() { // cleanup test rows
		endpoint := fmt.Sprintf("%s/rest/v1/workflow_chat_events?chat_id=eq.%s", strings.TrimRight(apiURL, "/"), chatID)
		_, _, _ = httpRequestFn(ctx, http.MethodDelete, endpoint, store.headers(""), nil)
	}()

	if err := store.AppendChatRecords(ctx, recs); err != nil {
		t.Fatalf("append: %v", err)
	}
	// Idempotency on the unique key: replaying seq 1..3 must not duplicate.
	if err := store.AppendChatRecords(ctx, recs); err != nil {
		t.Fatalf("idempotent append: %v", err)
	}
	got, err := store.ReadChatRecords(ctx, chatID, 0, 10)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("read %d records, want 3 (ignore-duplicates)", len(got))
	}
	for i, rec := range got {
		if rec.ChatSeq != int64(i+1) || rec.LegRunID != "run-t" || rec.ChatID != chatID {
			t.Fatalf("record[%d] = {seq:%d leg:%s chat:%s}", i, rec.ChatSeq, rec.LegRunID, rec.ChatID)
		}
	}
	// afterSeq pagination + latest seed.
	got, err = store.ReadChatRecords(ctx, chatID, 2, 10)
	if err != nil || len(got) != 1 || got[0].ChatSeq != 3 {
		t.Fatalf("afterSeq=2 read = %v (err %v)", got, err)
	}
	latest, err := store.LatestChatSeq(ctx, chatID)
	if err != nil || latest != 3 {
		t.Fatalf("LatestChatSeq = %d (err %v)", latest, err)
	}
}
