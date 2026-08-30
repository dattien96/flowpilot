package runner

// Chat transcript store tests (Task-313 T-3): local NDJSON round-trip,
// idempotent duplicate-seq append (restore upsert semantics), corrupt-tail
// skip, monotonic chatSeq under concurrency, and the path-traversal guard.
// New file — additive only.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func chatRec(chatID string, seq int64, typ string) ChatTranscriptRecord {
	payload, _ := json.Marshal(map[string]any{"text": "payload"})
	return ChatTranscriptRecord{ChatID: chatID, ChatSeq: seq, LegRunID: "run-1", Type: typ, Payload: payload}
}

func TestLocalChatTranscriptAppendReadRoundTrip(t *testing.T) {
	store := newLocalFileChatTranscriptStore(t.TempDir())
	ctx := context.Background()
	recs := []ChatTranscriptRecord{
		chatRec("cht_a", 1, EventTypeChatTurnStarted),
		chatRec("cht_a", 2, EventTypeChatMessageCompleted),
		chatRec("cht_a", 3, EventTypeChatToolStarted),
	}
	if err := store.AppendChatRecords(ctx, recs); err != nil {
		t.Fatalf("append: %v", err)
	}
	got, err := store.ReadChatRecords(ctx, "cht_a", 0, 100)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("read %d records, want 3", len(got))
	}
	for i, rec := range got {
		if rec.ChatSeq != int64(i+1) || rec.Type != recs[i].Type {
			t.Fatalf("record[%d] = {seq:%d type:%s}", i, rec.ChatSeq, rec.Type)
		}
	}
	// afterSeq pagination.
	got, err = store.ReadChatRecords(ctx, "cht_a", 2, 100)
	if err != nil || len(got) != 1 || got[0].ChatSeq != 3 {
		t.Fatalf("afterSeq=2 read = %v (err %v)", got, err)
	}
	latest, err := store.LatestChatSeq(ctx, "cht_a")
	if err != nil || latest != 3 {
		t.Fatalf("LatestChatSeq = %d (err %v)", latest, err)
	}
	// Unknown chat reads as empty, not error.
	got, err = store.ReadChatRecords(ctx, "cht_missing", 0, 10)
	if err != nil || len(got) != 0 {
		t.Fatalf("missing chat read = %v (err %v)", got, err)
	}
}

func TestLocalChatTranscriptAppendIdempotentOnDuplicateSeq(t *testing.T) {
	store := newLocalFileChatTranscriptStore(t.TempDir())
	ctx := context.Background()
	if err := store.AppendChatRecords(ctx, []ChatTranscriptRecord{chatRec("cht_a", 1, EventTypeChatTurnStarted)}); err != nil {
		t.Fatal(err)
	}
	// Same (chatId, chatSeq) again — restore upsert semantics: dropped.
	if err := store.AppendChatRecords(ctx, []ChatTranscriptRecord{chatRec("cht_a", 1, EventTypeChatTurnStarted)}); err != nil {
		t.Fatal(err)
	}
	got, err := store.ReadChatRecords(ctx, "cht_a", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("duplicate seq appended: %d records, want 1", len(got))
	}
}

func TestLocalChatTranscriptCorruptTailSkipped(t *testing.T) {
	dir := t.TempDir()
	store := newLocalFileChatTranscriptStore(dir)
	ctx := context.Background()
	if err := store.AppendChatRecords(ctx, []ChatTranscriptRecord{chatRec("cht_a", 1, EventTypeChatTurnStarted)}); err != nil {
		t.Fatal(err)
	}
	// Append garbage bytes directly (simulated crash mid-write).
	path := filepath.Join(dir, "chats", "cht_a", "transcript.ndjson")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{\"chatId\": \"cht_a\", \"trunca"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	got, err := store.ReadChatRecords(ctx, "cht_a", 0, 100)
	if err != nil {
		t.Fatalf("corrupt tail must not error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("corrupt tail read = %d records, want the 1 valid record", len(got))
	}
}

func TestChatSeqMonotonicUnderConcurrency(t *testing.T) {
	store := newLocalFileChatTranscriptStore(t.TempDir())
	writer := newChatTranscriptWriter(store)
	ctx := context.Background()
	const goroutines = 10
	var wg sync.WaitGroup
	var mu sync.Mutex
	seqs := map[int64]struct{}{}
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := writer.append(ctx, chatRec("cht_a", 0, EventTypeChatTokenUsage)); err != nil {
				t.Errorf("append: %v", err)
				return
			}
			got, err := store.ReadChatRecords(ctx, "cht_a", 0, 1000)
			if err != nil {
				t.Errorf("read: %v", err)
				return
			}
			mu.Lock()
			for _, rec := range got {
				seqs[rec.ChatSeq] = struct{}{}
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(seqs) < goroutines {
		t.Fatalf("concurrent appends lost seqs: %d distinct, want >= %d", len(seqs), goroutines)
	}
	prev := int64(0)
	ordered, err := store.ReadChatRecords(ctx, "cht_a", 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range ordered {
		if rec.ChatSeq <= prev {
			t.Fatalf("seqs not strictly increasing: %d after %d", rec.ChatSeq, prev)
		}
		prev = rec.ChatSeq
	}
}

// TestChatTranscriptWriterSeedsFromLatestSeq pins the restart-continuation
// rule: a fresh writer continues the on-disk sequence, never restarts at 1.
func TestChatTranscriptWriterSeedsFromLatestSeq(t *testing.T) {
	store := newLocalFileChatTranscriptStore(t.TempDir())
	ctx := context.Background()
	if err := store.AppendChatRecords(ctx, []ChatTranscriptRecord{chatRec("cht_a", 7, EventTypeChatTurnStarted)}); err != nil {
		t.Fatal(err)
	}
	writer := newChatTranscriptWriter(store)
	if err := writer.append(ctx, chatRec("cht_a", 0, EventTypeChatMessageCompleted)); err != nil {
		t.Fatal(err)
	}
	got, err := store.ReadChatRecords(ctx, "cht_a", 7, 10)
	if err != nil || len(got) != 1 || got[0].ChatSeq != 8 {
		t.Fatalf("writer restart continuation = %+v (err %v), want seq 8", got, err)
	}
}

func TestSanitizeChatIDPathTraversal(t *testing.T) {
	if got := sanitizeChatID("../../etc"); got != "etc" {
		t.Fatalf("sanitizeChatID traversal = %q", got)
	}
	if got := sanitizeChatID("cht_9f2a71c04b8d"); got != "cht_9f2a71c04b8d" {
		t.Fatalf("sanitizeChatID mangled a real id: %q", got)
	}
}
