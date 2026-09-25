package runner

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

// BUG-502: live drill on b501 — a chat run created under a SECOND registered
// project (projectId != runner workspace project) completed a real provider
// turn and parked on a gate approval, yet left ZERO rows in
// sessions.ndjson / approvals.ndjson. After restart the run was
// run_not_found, the chat timeline chat_not_found, and its pending cards
// were unrecoverable — while an identical workspace-project run persisted
// normally. Durable state is the source of truth (local-runner invariant):
// every project's run must persist the same session/card rows.
//
// This test exercises the verified-good write path as a regression guard —
// it PASSES on the current build (both projects persist), so the live loss
// mechanism is not in the persist callsite itself (see the BUG-502 doc for
// the remaining suspects: cross-process rewrite / torn-tail ordering).

func TestBug502SessionRowPersistsForSecondProjectRun(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	_, srv := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)

	runA := startProjectRun(t, srv.URL, "proj-workspace", "")
	runB := startProjectRun(t, srv.URL, "proj-second", "")

	if st, _ := sendTurn(t, srv.URL, runA, "", nil); st != http.StatusOK && st != http.StatusAccepted {
		t.Fatalf("turn A status=%d", st)
	}
	if st, _ := sendTurn(t, srv.URL, runB, "", nil); st != http.StatusOK && st != http.StatusAccepted {
		t.Fatalf("turn B status=%d", st)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		sessions, _ := store.ListAllProviderSessions(context.Background())
		foundA, foundB := false, false
		for _, sess := range sessions {
			if sess.RunID == runA {
				foundA = true
			}
			if sess.RunID == runB {
				foundB = true
			}
		}
		if foundA && foundB {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	sessions, _ := store.ListAllProviderSessions(context.Background())
	gotA, gotB := false, false
	for _, sess := range sessions {
		if sess.RunID == runA {
			gotA = true
		}
		if sess.RunID == runB {
			gotB = true
		}
	}
	if !gotA {
		t.Fatalf("session row missing for workspace run %s", runA)
	}
	if !gotB {
		t.Fatalf("session row missing for second-project run %s (BUG-502: rows only persist for the workspace project)", runB)
	}
}
