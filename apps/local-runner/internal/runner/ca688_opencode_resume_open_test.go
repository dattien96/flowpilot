package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// CA-688: opencode stores sessions in the shared
// ~/.local/share/opencode/opencode.db — there are no per-session files to
// LocateSessionFile, and ACP session/load works from any process on this
// machine (BUG-329 live probe). Opening a local opencode chat (TUI /history →
// /open, Desktop history) must therefore succeed with just a real ses_* id —
// the operator hit "Open failed (runner session_unavailable): session data not
// found on this machine" on the CP-57 test guide's smoke step.

func TestResumeRunSucceedsForOpencodeRealSessionWithoutFiles(t *testing.T) {
	// This test opens the REAL ~/.local/share/opencode/opencode.db — a live
	// probe by design (BUG-329). On machines without opencode, or where the DB
	// is locked by a running opencode process, it hangs/fails through no fault
	// of the code under test, so it is opt-in like the other FLOWPILOT_LIVE_*
	// gates.
	if os.Getenv("FLOWPILOT_LIVE_OPENCODE") == "" {
		t.Skip("set FLOWPILOT_LIVE_OPENCODE=1 to run the real opencode.db probe")
	}
	store := newFakeWorkflowStore()
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-oc-open-1",
		ProviderKey:       ProviderKeyOpencode,
		ProviderSessionID: "ses_fb312ff67ffel7Gvor1TPdecxq",
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		WorkingDirectory:  filepath.Join(t.TempDir(), "proj"),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	// No provider accounts configured and no session files exist on this
	// service at all — if the opencode bypass did not fire, ensureResumeReady
	// would fail on home resolution / LocateSessionFile before anything else.
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)

	handle, apiErr := svc.resumeRun("run-oc-open-1")
	if apiErr != nil {
		t.Fatalf("resumeRun: %v (CA-688: a real ses_* id must reopen without session files)", apiErr)
	}
	if handle.RunID != "run-oc-open-1" {
		t.Fatalf("handle.RunID = %q", handle.RunID)
	}
}
