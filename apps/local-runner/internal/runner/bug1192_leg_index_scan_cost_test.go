package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// BUG-1192: LegIndex rescans every chats/<id>/transcript.ndjson on every call.
// Live evidence: ~/.flowpilot/chat-transcripts held 69k chats / 285MB, so each
// /client/projects/{id}/workflow-runs call (stampMissingChatIdentity →
// transcriptLegIndex → LegIndex) blocked >90s and the desktop's 30s GET timeout
// fired — "loading project" never completed. The scan must be incremental:
// unchanged transcripts must not be re-read, appended/edited/deleted chats must
// still refresh.

func seedLegTranscript(t *testing.T, dir, chatID string, recs []ChatTranscriptRecord) {
	t.Helper()
	chatDir := filepath.Join(dir, "chats", chatID)
	if err := os.MkdirAll(chatDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f, err := os.Create(filepath.Join(chatDir, "transcript.ndjson"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, r := range recs {
		line, _ := json.Marshal(r)
		if _, err := f.Write(append(line, '\n')); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func legRec(chatID, legRunID string, seq int64) ChatTranscriptRecord {
	return ChatTranscriptRecord{ChatID: chatID, ChatSeq: seq, LegRunID: legRunID, Type: EventTypeChatTurnStarted, Payload: json.RawMessage(`{"prompt":"x"}`)}
}

func TestBug1192_LegIndexDoesNotRescanUnchangedChats(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		chatID := fmt.Sprintf("cht_%d", i)
		seedLegTranscript(t, dir, chatID, []ChatTranscriptRecord{legRec(chatID, fmt.Sprintf("run-leg-%d", i), 1)})
	}
	store := newLocalFileChatTranscriptStore(dir)

	first, err := store.LegIndex(context.Background())
	if err != nil {
		t.Fatalf("LegIndex: %v", err)
	}
	if len(first) != 5 {
		t.Fatalf("legs=%d want 5", len(first))
	}
	reads := store.legScanReads.Load()
	if reads != 5 {
		t.Fatalf("first scan should read 5 transcripts, got %d", reads)
	}

	// Second call with zero changes must not re-read a single transcript.
	second, err := store.LegIndex(context.Background())
	if err != nil {
		t.Fatalf("LegIndex 2: %v", err)
	}
	if got := store.legScanReads.Load(); got != reads {
		t.Fatalf("unchanged scan re-read transcripts: reads %d -> %d", reads, got)
	}
	if len(second) != 5 {
		t.Fatalf("second scan legs=%d want 5", len(second))
	}
}

func TestBug1192_LegIndexRescansOnlyChangedChat(t *testing.T) {
	dir := t.TempDir()
	seedLegTranscript(t, dir, "cht_a", []ChatTranscriptRecord{legRec("cht_a", "run-a0", 1)})
	seedLegTranscript(t, dir, "cht_b", []ChatTranscriptRecord{legRec("cht_b", "run-b0", 1)})
	store := newLocalFileChatTranscriptStore(dir)

	if _, err := store.LegIndex(context.Background()); err != nil {
		t.Fatalf("LegIndex: %v", err)
	}
	reads := store.legScanReads.Load()

	// Append a switch leg to cht_b only — next LegIndex must pick it up while
	// re-reading only that one transcript.
	if err := store.AppendChatRecords(context.Background(), []ChatTranscriptRecord{
		{ChatID: "cht_b", ChatSeq: 2, LegRunID: "run-b1", Type: EventTypeChatProviderSwitch, Payload: json.RawMessage(`{"fromRunId":"run-b0","toRunId":"run-b1","legSeq":1}`)},
		legRec("cht_b", "run-b1", 3),
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	idx, err := store.LegIndex(context.Background())
	if err != nil {
		t.Fatalf("LegIndex 2: %v", err)
	}
	if info, ok := idx["run-b1"]; !ok || info.ChatID != "cht_b" || info.LegSeq != 1 {
		t.Fatalf("new leg missing from index: %+v", idx["run-b1"])
	}
	if got := store.legScanReads.Load(); got != reads+1 {
		t.Fatalf("changed chat rescan should read exactly 1 transcript, delta=%d", got-reads)
	}
}

func TestBug1192_LegIndexDropsDeletedChat(t *testing.T) {
	dir := t.TempDir()
	seedLegTranscript(t, dir, "cht_a", []ChatTranscriptRecord{legRec("cht_a", "run-a0", 1)})
	seedLegTranscript(t, dir, "cht_b", []ChatTranscriptRecord{legRec("cht_b", "run-b0", 1)})
	store := newLocalFileChatTranscriptStore(dir)
	if idx, err := store.LegIndex(context.Background()); err != nil || len(idx) != 2 {
		t.Fatalf("LegIndex: %v len=%d", err, len(idx))
	}
	if err := os.RemoveAll(filepath.Join(dir, "chats", "cht_b")); err != nil {
		t.Fatalf("rm: %v", err)
	}
	idx, err := store.LegIndex(context.Background())
	if err != nil {
		t.Fatalf("LegIndex 2: %v", err)
	}
	if _, ok := idx["run-b0"]; ok {
		t.Fatalf("deleted chat's leg still indexed: %+v", idx)
	}
	if _, ok := idx["run-a0"]; !ok {
		t.Fatalf("surviving chat's leg dropped: %+v", idx)
	}
}

// countingLegStore lets stampMissingChatIdentity tests observe whether the
// transcript leg index was touched at all.
type countingLegStore struct {
	inner    *localFileChatTranscriptStore
	legCalls atomic.Int64
}

func (c *countingLegStore) AppendChatRecords(ctx context.Context, recs []ChatTranscriptRecord) error {
	return c.inner.AppendChatRecords(ctx, recs)
}
func (c *countingLegStore) ReadChatRecords(ctx context.Context, chatID string, afterSeq int64, limit int) ([]ChatTranscriptRecord, error) {
	return c.inner.ReadChatRecords(ctx, chatID, afterSeq, limit)
}
func (c *countingLegStore) LatestChatSeq(ctx context.Context, chatID string) (int64, error) {
	return c.inner.LatestChatSeq(ctx, chatID)
}
func (c *countingLegStore) LegIndex(ctx context.Context) (map[string]ChatLegInfo, error) {
	c.legCalls.Add(1)
	return c.inner.LegIndex(ctx)
}

func TestBug1192_StampSkipsLegIndexWhenNothingNeedsIt(t *testing.T) {
	dir := t.TempDir()
	counting := &countingLegStore{inner: newLocalFileChatTranscriptStore(dir)}
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	// Consume the lazy init so our counting store is not overwritten.
	svc.chatOnce.Do(func() {})
	svc.chatTranscripts = newChatTranscriptWriter(counting)

	items := []runHistoryItem{
		{RunID: "run-1", RunKind: "chat", ChatID: "cht_x", LegSeq: 0},
		{RunID: "run-wf", RunKind: "workflow", ChatID: ""},
	}
	out := svc.stampMissingChatIdentity(items)
	if got := counting.legCalls.Load(); got != 0 {
		t.Fatalf("LegIndex invoked %d times though no item needed stamping", got)
	}
	if out[0].ChatID != "cht_x" {
		t.Fatalf("chat identity lost: %+v", out[0])
	}
}

func TestBug1192_StampStillUsesLegIndexForLegacyRows(t *testing.T) {
	dir := t.TempDir()
	seedLegTranscript(t, dir, "cht_y", []ChatTranscriptRecord{legRec("cht_y", "run-old", 1)})
	counting := &countingLegStore{inner: newLocalFileChatTranscriptStore(dir)}
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.chatOnce.Do(func() {})
	svc.chatTranscripts = newChatTranscriptWriter(counting)

	items := []runHistoryItem{{RunID: "run-old", RunKind: "chat", ChatID: ""}}
	out := svc.stampMissingChatIdentity(items)
	if got := counting.legCalls.Load(); got != 1 {
		t.Fatalf("LegIndex should run exactly once for legacy rows, got %d", got)
	}
	if out[0].ChatID != "cht_y" {
		t.Fatalf("legacy row not stamped: %+v", out[0])
	}
}

// countingHistoryStore counts ListProviderSessionsByProject calls.
type countingHistoryStore struct {
	*fakeWorkflowStore
	historyCalls atomic.Int64
}

func (c *countingHistoryStore) ListProviderSessionsByProject(ctx context.Context, projectID string) ([]ProviderSessionState, error) {
	c.historyCalls.Add(1)
	return c.fakeWorkflowStore.ListProviderSessionsByProject(ctx, projectID)
}

func TestBug1192_ProjectRunHistoryReadsSessionsOnce(t *testing.T) {
	store := &countingHistoryStore{fakeWorkflowStore: newFakeWorkflowStore()}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-1", ProjectID: "proj-p", ProviderKey: ProviderKeyCodex,
		RunKind: "chat", Status: RunStatusCompleted,
		StartedAt: "2026-08-31T00:00:00Z", UpdatedAt: "2026-08-31T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.chatOnce.Do(func() {})
	svc.chatTranscripts = newChatTranscriptWriter(newLocalFileChatTranscriptStore(t.TempDir()))

	if _, err := svc.projectRunHistory("proj-p"); err != nil {
		t.Fatalf("projectRunHistory: %v", err)
	}
	if got := store.historyCalls.Load(); got != 1 {
		t.Fatalf("ListProviderSessionsByProject called %d times per history fetch, want 1", got)
	}
}
