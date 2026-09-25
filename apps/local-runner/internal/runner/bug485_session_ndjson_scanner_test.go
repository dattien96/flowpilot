package runner

// BUG-485: durable NDJSON readers in local_file_session_store use
// bufio.Scanner with the default 64KB token cap and never check sc.Err().
// The first oversized line aborts the scan silently — every record after it
// is invisible to session restore, to the boot worktree GC's binding
// authority (gcOrphan → live bindings pruned), and to the delete-rewrite
// path (partial `kept` slice destroys records permanently). Live-verified
// on the test bed: 567 lines >64KB in sessions.ndjson; boot GC pruned two
// worktree dirs whose sessions still said worktree_state=active.
// Companion defect: loadFromDisk also drops records with an empty
// project_id, which discards every normal-chat session record (chats have
// no project) — including their worktree bindings.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bug485FatRecord returns a valid session-record line >64KB — the shape a
// real record takes when a long transcript/last_prompt is embedded.
func bug485FatRecord(t *testing.T, runID string, size int) []byte {
	t.Helper()
	rec := map[string]any{
		"run_id":      runID,
		"project_id":  "p-fat",
		"provider_key": "codex",
		"status":      "terminal_completed",
		"last_prompt": strings.Repeat("x", size),
		"updated_at":  "2026-09-25T03:00:00Z",
	}
	line, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if len(line) < size {
		t.Fatalf("fat record only %d bytes, wanted >%d", len(line), size)
	}
	return append(line, '\n')
}

func bug485BoundRecord(t *testing.T, runID, chatID, state string) []byte {
	t.Helper()
	rec := map[string]any{
		"run_id":             runID,
		"project_id":         "p-1",
		"provider_key":       "codex",
		"status":             "cancelled",
		"run_kind":           "chat",
		"chat_id":            chatID,
		"worktree_owner_id":  chatID,
		"worktree_path":      "/tmp/wt/" + chatID,
		"worktree_state":     state,
		"worktree_enabled":   true,
		"updated_at":         "2026-09-25T03:30:00Z",
	}
	line, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	return append(line, '\n')
}

func bug485WriteSessions(t *testing.T, dir string, lines ...[]byte) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var buf []byte
	for _, l := range lines {
		buf = append(buf, l...)
	}
	if err := os.WriteFile(filepath.Join(dir, "sessions.ndjson"), buf, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Records after a >64KB line must still load — the binding authority for the
// boot GC. Pre-fix: bufio.Scanner aborts at the fat line and the bound
// record is invisible (the live prune observed on the bed).
func TestBUG485_SessionLoadSurvivesOversizedLine(t *testing.T) {
	dir := t.TempDir()
	bug485WriteSessions(t, dir,
		bug485FatRecord(t, "run-fat", 80*1024),
		bug485BoundRecord(t, "run-bound", "cht-bound485", "active"),
	)

	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	sess, found, err := store.GetProviderSession(context.Background(), "run-bound")
	if err != nil {
		t.Fatalf("GetProviderSession: %v", err)
	}
	if !found {
		t.Fatal("worktree-bound session record after a >64KB line was not loaded — scanner truncated the file")
	}
	if sess.WorktreeOwnerID != "cht-bound485" || sess.WorktreeState != "active" {
		t.Fatalf("bound record loaded without worktree fields: %#v", sess)
	}
	legs, err := store.ListProviderSessionsByChat(context.Background(), "cht-bound485")
	if err != nil {
		t.Fatalf("ListProviderSessionsByChat: %v", err)
	}
	if len(legs) == 0 {
		t.Fatal("chat leg listing missed the post-fat-line record")
	}
}

// When the session file genuinely cannot be fully read, the GC's binding
// authority must surface the failure — never degrade to gcOrphan.
func TestBUG485_TruncatedSessionLoadDefersGC(t *testing.T) {
	dir := t.TempDir()
	bug485WriteSessions(t, dir,
		bug485FatRecord(t, "run-fat", 80*1024),
		bug485BoundRecord(t, "run-bound2", "cht-bound485b", "active"),
	)
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store)
	svc.AttachRunner(&Runner{workspace: dir})
	verdict := svc.worktreeGCVerdictFor(context.Background(), "cht-bound485b")
	if verdict != gcBound {
		t.Fatalf("verdict = %v — a bound owner must never reach gcOrphan (records exist and are readable); the load must either surface the record or fail closed to gcUnknown", verdict)
	}
}

// Chat runs legitimately persist project_id:"" — dropping them at load
// discards every chat session record, including their worktree bindings.
func TestBUG485_ChatRunRecordWithEmptyProjectLoads(t *testing.T) {
	dir := t.TempDir()
	rec := map[string]any{
		"run_id":            "run-chat485",
		"project_id":        "",
		"provider_key":      "codex",
		"status":            "cancelled",
		"run_kind":          "chat",
		"chat_id":           "cht-chat485",
		"worktree_owner_id": "cht-chat485",
		"worktree_state":    "active",
		"worktree_enabled":  true,
		"updated_at":        "2026-09-25T03:30:00Z",
	}
	line, _ := json.Marshal(rec)
	bug485WriteSessions(t, dir, append(line, '\n'))

	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	legs, err := store.ListProviderSessionsByChat(context.Background(), "cht-chat485")
	if err != nil {
		t.Fatalf("ListProviderSessionsByChat: %v", err)
	}
	if len(legs) == 0 {
		t.Fatal("chat-run session with empty project_id dropped at load — worktree binding invisible to GC")
	}
	if legs[0].WorktreeState != "active" {
		t.Fatalf("worktree_state = %q, want active", legs[0].WorktreeState)
	}
}

// The delete-rewrite path must not destroy records after an oversized line:
// a partial `kept` scan renamed over the file is permanent data loss.
func TestBUG485_DeleteRewritePreservesOversizedLines(t *testing.T) {
	dir := t.TempDir()
	bug485WriteSessions(t, dir,
		bug485BoundRecord(t, "run-del", "cht-del", "active"),
		bug485FatRecord(t, "run-fat2", 80*1024),
		bug485BoundRecord(t, "run-keep", "cht-keep", "merge_pending"),
	)
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	if err := store.DeleteProviderSession(context.Background(), "run-del"); err != nil {
		t.Fatalf("DeleteProviderSession: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "sessions.ndjson"))
	if err != nil {
		t.Fatalf("read sessions.ndjson: %v", err)
	}
	for _, want := range []string{"run-fat2", "run-keep", "cht-keep"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("sessions.ndjson lost %q after delete-rewrite — oversized line truncated the kept set", want)
		}
	}
	if strings.Contains(string(raw), "run-del") {
		t.Fatal("deleted run still present")
	}
}

// questions.ndjson / approvals.ndjson carry pending gates across restart —
// same scanner, same silent truncation.
func TestBUG485_QuestionsAndApprovalsSurviveOversizedLine(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fat := strings.Repeat("y", 80*1024)
	qline := `{"QuestionID":"q-keep","RunID":"run-q","Prompt":"pick","Status":"pending"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "questions.ndjson"),
		[]byte(`{"QuestionID":"q-fat","RunID":"run-q","Prompt":"`+fat+`","Status":"pending"}`+"\n"+qline), 0o644); err != nil {
		t.Fatal(err)
	}
	aline := `{"ApprovalID":"a-keep","RunID":"run-a","Prompt":"ok","Status":"pending"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "approvals.ndjson"),
		[]byte(`{"ApprovalID":"a-fat","RunID":"run-a","Prompt":"`+fat+`","Status":"pending"}`+"\n"+aline), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	qs, err := store.ListQuestionsByRun(context.Background(), "run-q")
	if err != nil {
		t.Fatalf("ListQuestionsByRun: %v", err)
	}
	foundQ := false
	for _, q := range qs {
		if q.QuestionID == "q-keep" {
			foundQ = true
		}
	}
	if !foundQ {
		t.Fatalf("pending question after oversized line not replayed: %#v", qs)
	}
	as, err := store.ListApprovalsByRun(context.Background(), "run-a")
	if err != nil {
		t.Fatalf("ListApprovalsByRun: %v", err)
	}
	foundA := false
	for _, a := range as {
		if a.ApprovalID == "a-keep" {
			foundA = true
		}
	}
	if !foundA {
		t.Fatalf("pending approval after oversized line not replayed: %#v", as)
	}
}

// TestBUG485_TornTailRecordFailsClosed covers the crash-mid-append case: the
// file ends with an unterminated, unparseable line (a torn write of what may
// have been a worktree-bound record). Silently dropping it recreates the same
// data-loss class — the store must report itself incomplete so GC defers.
func TestBUG485_TornTailRecordFailsClosed(t *testing.T) {
	dir := t.TempDir()
	torn := []byte(`{"run_id":"run-torn","worktree_owner_id":"cht-torn485","worktree_state":"act`)
	bug485WriteSessions(t, dir,
		bug485BoundRecord(t, "run-thin", "cht-thin485", "active"),
		torn, // no trailing newline — torn append
	)

	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store)
	svc.AttachRunner(&Runner{workspace: dir})
	verdict := svc.worktreeGCVerdictFor(context.Background(), "cht-torn485")
	if verdict == gcOrphan {
		t.Fatal("torn tail record silently dropped → owner classified gcOrphan; the store must fail closed (gcUnknown) on an unterminated tail line")
	}
}
