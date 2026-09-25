package changecontract

// BUG-497: loadFromDisk treated any os.Open failure as "first run" and
// never checked scanner.Err(). An unreadable or truncated contracts file
// produced a silently empty/partial store → Get reported "no contract"
// for runs that had one → declared-path enforcement fell open to
// inferred scope. Contract: missing file = empty store; unreadable file
// = error; truncated load = error.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBUG497_OpenStoreReadOnlyUnreadableFileErrors(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, ".flowpilot", "contracts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A directory where the file should be: stat succeeds, open fails with
	// a non-ENOENT error → must surface, not masquerade as empty store.
	if err := os.Mkdir(filepath.Join(dir, "contracts.ndjson"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStoreReadOnly(ws)
	if err == nil {
		t.Fatalf("unreadable contracts store must error, got store=%+v", store)
	}
}

func TestBUG497_OpenStoreReadOnlyMissingFileStillEmpty(t *testing.T) {
	ws := t.TempDir()
	store, err := OpenStoreReadOnly(ws)
	if err != nil {
		t.Fatalf("missing file is a legitimate empty store: %v", err)
	}
	if store == nil {
		t.Fatal("missing file must still return a usable empty store")
	}
}

func TestBUG497_LoadDoesNotTruncateSilently(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, ".flowpilot", "contracts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A >4MiB contract line (pathological payload) followed by a valid one —
	// scanner cap would stop the load and drop the trailing contract.
	fat := `{"run_id":"fat","step_id":"s","declared_paths":["` + strings.Repeat("p", 5*1024*1024) + `"]}`
	valid := `{"run_id":"run-tail","step_id":"s","declared_paths":["x.go"],"declared_at":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(dir, "contracts.ndjson"), []byte(fat+"\n"+valid+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStoreReadOnly(ws)
	if err != nil {
		t.Fatalf("oversized line should not fail the whole load silently: %v", err)
	}
	if _, ok := store.Get("run-tail", "s"); !ok {
		t.Fatal("contract after an oversized line was silently dropped")
	}
}
