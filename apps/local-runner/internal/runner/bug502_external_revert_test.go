package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// BUG-502 (live, b501): durable rows for second-project chat runs vanished
// between turn completion and restart. Root cause found: an out-of-band
// `git reset --hard` in the live bed rewound the GIT-TRACKED
// .flowpilot/chats/{sessions,approvals}.ndjson files to their committed
// snapshot (reflog HEAD@{19:30:50} == file birth times; the committed blob is
// a byte-identical prefix of the current file). The live runner kept
// appending onto the rewound file without noticing — memory diverged from
// disk, and after the next restart the affected runs/chat/approvals were
// "not found" even though the write path itself was never broken.
//
// These tests pin the fix: shared durable files must (a) detect out-of-band
// modification and resync memory to the on-disk truth instead of silently
// diverging, and (b) serialize writes across runner processes via a sibling
// lock file — the dispatch.lock precedent — so a sibling append cannot be
// lost between DeleteProviderSession's read and rename.

func bug502Store(t *testing.T) (*localFileSessionStore, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(dir, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	return store, dir
}

func bug502FileLines(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out []string
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func TestBUG502_SessionsFileExternallyRewoundResyncs(t *testing.T) {
	store, dir := bug502Store(t)
	ctx := context.Background()
	sessionsPath := filepath.Join(dir, ".flowpilot", "chats", "sessions.ndjson")

	if err := store.UpsertProviderSession(ctx, ProviderSessionState{RunID: "run-a", ProjectID: "p1", Status: "running", ChatID: "c1"}); err != nil {
		t.Fatalf("upsert A: %v", err)
	}
	snapshot, err := os.ReadFile(sessionsPath)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := store.UpsertProviderSession(ctx, ProviderSessionState{RunID: "run-b", ProjectID: "p2", Status: "running", ChatID: "c2"}); err != nil {
		t.Fatalf("upsert B: %v", err)
	}

	// Out-of-band revert (the observed `git reset --hard` shape): the file is
	// replaced with an older snapshot that lacks run-b's row.
	if err := os.WriteFile(sessionsPath, snapshot, 0o644); err != nil {
		t.Fatalf("revert: %v", err)
	}
	// Force a different mtime so detection does not depend on fs granularity.
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(sessionsPath, old, old)

	if err := store.UpsertProviderSession(ctx, ProviderSessionState{RunID: "run-c", ProjectID: "p1", Status: "running", ChatID: "c3"}); err != nil {
		t.Fatalf("upsert C: %v", err)
	}

	if _, found, _ := store.GetProviderSession(ctx, "run-b"); found {
		t.Fatalf("run-b still present in memory after external rewind: in-memory view diverged from disk (BUG-502 silent loss)")
	}
	if _, found, _ := store.GetProviderSession(ctx, "run-a"); !found {
		t.Fatalf("run-a must survive the resync — it is still on disk")
	}
	if _, found, _ := store.GetProviderSession(ctx, "run-c"); !found {
		t.Fatalf("run-c missing after upsert")
	}
	for _, l := range bug502FileLines(t, sessionsPath) {
		if strings.Contains(l, `"run-b"`) {
			t.Fatalf("run-b row must not reappear on disk after resync")
		}
	}
}

func TestBUG502_ApprovalsFileExternallyRewoundResyncs(t *testing.T) {
	store, dir := bug502Store(t)
	ctx := context.Background()
	approvalsPath := filepath.Join(dir, ".flowpilot", "chats", "approvals.ndjson")

	if err := store.UpsertApproval(ctx, ProviderApprovalState{ApprovalID: "appr-a", RunID: "run-a", Status: "pending"}); err != nil {
		t.Fatalf("upsert appr-a: %v", err)
	}
	snapshot, err := os.ReadFile(approvalsPath)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := store.UpsertApproval(ctx, ProviderApprovalState{ApprovalID: "appr-b", RunID: "run-b", Status: "pending"}); err != nil {
		t.Fatalf("upsert appr-b: %v", err)
	}
	if err := os.WriteFile(approvalsPath, snapshot, 0o644); err != nil {
		t.Fatalf("revert: %v", err)
	}
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(approvalsPath, old, old)

	if err := store.UpsertApproval(ctx, ProviderApprovalState{ApprovalID: "appr-c", RunID: "run-c", Status: "pending"}); err != nil {
		t.Fatalf("upsert appr-c: %v", err)
	}
	if got, _ := store.ListApprovalsByRun(ctx, "run-b"); len(got) != 0 {
		t.Fatalf("appr-b still visible after external rewind: memory diverged from disk")
	}
	if got, _ := store.ListApprovalsByRun(ctx, "run-c"); len(got) != 1 {
		t.Fatalf("appr-c missing after upsert")
	}
}

func TestBUG502_QuestionsFileExternallyRewoundResyncs(t *testing.T) {
	store, dir := bug502Store(t)
	ctx := context.Background()
	questionsPath := filepath.Join(dir, ".flowpilot", "chats", "questions.ndjson")

	if err := store.UpsertQuestion(ctx, ProviderQuestionState{QuestionID: "q-a", RunID: "run-a", Status: "pending"}); err != nil {
		t.Fatalf("upsert q-a: %v", err)
	}
	snapshot, err := os.ReadFile(questionsPath)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := store.UpsertQuestion(ctx, ProviderQuestionState{QuestionID: "q-b", RunID: "run-b", Status: "pending"}); err != nil {
		t.Fatalf("upsert q-b: %v", err)
	}
	if err := os.WriteFile(questionsPath, snapshot, 0o644); err != nil {
		t.Fatalf("revert: %v", err)
	}
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(questionsPath, old, old)

	if err := store.UpsertQuestion(ctx, ProviderQuestionState{QuestionID: "q-c", RunID: "run-c", Status: "pending"}); err != nil {
		t.Fatalf("upsert q-c: %v", err)
	}
	if got, _ := store.ListQuestionsByRun(ctx, "run-b"); len(got) != 0 {
		t.Fatalf("q-b still visible after external rewind: memory diverged from disk")
	}
	if got, _ := store.ListQuestionsByRun(ctx, "run-c"); len(got) != 1 {
		t.Fatalf("q-c missing after upsert")
	}
}

// A sibling runner process appending to the shared file must not trigger a
// false "revert": the resync merges foreign rows into memory (disk is the
// authority) rather than dropping them.
func TestBUG502_SiblingProcessAppendIsResyncedNotLost(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	chatsDir := filepath.Join(dir, ".flowpilot", "chats")
	storeA, err := NewLocalFileSessionStore(chatsDir)
	if err != nil {
		t.Fatalf("storeA: %v", err)
	}
	if err := storeA.UpsertProviderSession(ctx, ProviderSessionState{RunID: "run-a", Status: "running"}); err != nil {
		t.Fatalf("upsert A: %v", err)
	}
	// Second store instance == a second runner process sharing the file.
	storeB, err := NewLocalFileSessionStore(chatsDir)
	if err != nil {
		t.Fatalf("storeB: %v", err)
	}
	if err := storeB.UpsertProviderSession(ctx, ProviderSessionState{RunID: "run-b", Status: "running"}); err != nil {
		t.Fatalf("sibling upsert B: %v", err)
	}
	// storeA's next write must notice storeB's append and merge it — not
	// rewrite or ignore it.
	if err := storeA.UpsertProviderSession(ctx, ProviderSessionState{RunID: "run-c", Status: "running"}); err != nil {
		t.Fatalf("upsert C: %v", err)
	}
	if _, found, _ := storeA.GetProviderSession(ctx, "run-b"); !found {
		t.Fatalf("sibling row run-b not merged into storeA view after resync")
	}
	// Sibling delete rewrite must not discard storeA's later append either:
	// serialize via the shared lock — storeB deletes run-b while holding the
	// same lock storeA uses for appends.
	if err := storeB.DeleteProviderSession(ctx, "run-b"); err != nil {
		t.Fatalf("sibling delete: %v", err)
	}
	if err := storeA.UpsertProviderSession(ctx, ProviderSessionState{RunID: "run-d", Status: "running"}); err != nil {
		t.Fatalf("upsert D: %v", err)
	}
	lines := bug502FileLines(t, filepath.Join(chatsDir, "sessions.ndjson"))
	var seenA, seenD, seenB bool
	for _, l := range lines {
		seenA = seenA || strings.Contains(l, `"run-a"`)
		seenB = seenB || strings.Contains(l, `"run-b"`)
		seenD = seenD || strings.Contains(l, `"run-d"`)
	}
	if !seenA || !seenD {
		t.Fatalf("rows lost across sibling delete+append: a=%v b=%v d=%v", seenA, seenB, seenD)
	}
	if seenB {
		t.Fatalf("deleted run-b row still on disk")
	}
}

// The per-operation lock must actually engage: holding sessions.ndjson.lock
// exclusively (what a sibling process's writer holds) must block our append
// until released — the dispatch.lock precedent, applied per-op.
func TestBUG502_SessionWriterBlocksOnHeldDurableLock(t *testing.T) {
	store, dir := bug502Store(t)
	lockPath := filepath.Join(dir, ".flowpilot", "chats", "sessions.ndjson.lock")
	lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open lock: %v", err)
	}
	defer lf.Close()
	if err := flockExclusive(lf); err != nil {
		t.Fatalf("test flock: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- store.UpsertProviderSession(context.Background(), ProviderSessionState{RunID: "run-x", Status: "running"})
	}()
	select {
	case err := <-done:
		_ = funlock(lf)
		t.Fatalf("append completed while durable lock held by sibling (err=%v): writes are not mutually excluded", err)
	case <-time.After(400 * time.Millisecond):
	}
	if err := funlock(lf); err != nil {
		t.Fatalf("funlock: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("append after lock release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("append never completed after lock release")
	}
}

// The durable dir must not silently stay git-revertable: on store init the
// runner ensures .flowpilot/ is in .git/info/exclude so out-of-band `git add
// -A` / resets do not start tracking (or keep tracking) state files —
// the exact mechanism that erased rows live (BUG-502).
func TestBUG502_StoreInitAddsFlowpilotToGitInfoExclude(t *testing.T) {
	dir := t.TempDir()
	infoDir := filepath.Join(dir, ".git", "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	excludePath := filepath.Join(infoDir, "exclude")
	if err := os.WriteFile(excludePath, []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewLocalFileSessionStore(filepath.Join(dir, ".flowpilot", "chats")); err != nil {
		t.Fatalf("store: %v", err)
	}
	raw, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("exclude read: %v", err)
	}
	found := false
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(l) == ".flowpilot/" {
			found = true
		}
	}
	if !found {
		t.Fatalf(".flowpilot/ not added to .git/info/exclude: durable files remain git-revertable (BUG-502)")
	}
	// Idempotent: second init must not append a duplicate line.
	if _, err := NewLocalFileSessionStore(filepath.Join(dir, ".flowpilot", "chats")); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(excludePath)
	if strings.Count(string(raw), ".flowpilot/") != 1 {
		t.Fatalf("exclude line duplicated")
	}
}
