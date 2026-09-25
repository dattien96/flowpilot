package runner

// CP-71 remaining LOW-gap fill (KR-005): 71-h worktree_merge_failed (apply
// error that is NOT a conflict), 71-i ensureGitignore side-effect over HTTP,
// 71-k provider-cwd parity (workspaceCwd == worktree path regardless of
// provider), 71-l markChatWorktreeState propagates to every leg + persisted
// session row. Additive file — no existing test touched.

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 71-h: a non-conflict apply failure (base sidecar deleted under the binding)
// must surface 500 worktree_merge_failed — NOT merge_pending, NOT conflict.
// The binding stays active so the user can still keep_branch/discard.
func TestE2EWorktree_MergeFailedOnMissingBaseSidecar(t *testing.T) {
	repo := initWorktreeRepo(t)
	svc, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, map[string]any{"providerKey": "claude"})
	wtPath := runWorktreePath(t, srv, runID)

	// Corrupt the binding's evidence: the base sidecar is what lets Apply
	// anchor the diff — deleting it is a non-conflict failure path.
	ownerID := filepath.Base(wtPath)
	sidecar := filepath.Join(repo, ".flowpilot", "worktrees", ownerID+".base")
	if err := os.Remove(sidecar); err != nil {
		t.Fatalf("remove base sidecar: %v", err)
	}
	// Dirty the worktree so Diff would have content if it could run.
	if err := os.WriteFile(filepath.Join(wtPath, "change.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	markRunTerminal(t, svc, runID)

	status, raw := resolveWorktreeConfirmHTTP(t, srv, runID, "apply_patch", true)
	if status != http.StatusInternalServerError {
		t.Fatalf("apply_patch on missing sidecar: %d %s, want 500 worktree_merge_failed", status, raw)
	}
	if !strings.Contains(string(raw), "worktree_merge_failed") {
		t.Fatalf("expected worktree_merge_failed code, got %s", raw)
	}
	if strings.Contains(string(raw), "conflict") {
		t.Fatalf("non-conflict failure must not masquerade as conflict: %s", raw)
	}
	// Fail-closed: state must NOT be merge_pending (nothing was parked).
	if st := runWorktreeState(t, srv, runID); st == "merge_pending" {
		t.Fatalf("state=%s — merge_failed must not park the binding", st)
	}
}

// 71-i: provisioning a worktree writes .flowpilot/worktrees/ into the repo's
// .gitignore — exactly once across multiple runs.
func TestE2EWorktree_GitignoreEntryWrittenOnce(t *testing.T) {
	repo := initWorktreeRepo(t)
	ignorePath := filepath.Join(repo, ".gitignore")
	if _, err := os.Stat(ignorePath); !os.IsNotExist(err) {
		t.Skip("repo fixture already has .gitignore — entry uniqueness untestable")
	}
	_, srv := worktreeHTTPServer(t, repo)
	startWorktreeChatRun(t, srv, repo, map[string]any{"providerKey": "claude"})
	startWorktreeChatRun(t, srv, repo, map[string]any{"providerKey": "codex"})

	raw, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatalf("worktree provisioning must write .gitignore: %v", err)
	}
	if n := strings.Count(string(raw), ".flowpilot/worktrees/"); n != 1 {
		t.Fatalf(".gitignore entry count=%d want 1 (two runs must not duplicate): %q", n, raw)
	}
}

// 71-k: workspaceCwd must point into the worktree for every provider — the
// binding is provider-agnostic, so verify both registered providers land the
// same contract (evidence for R2: provisionRunWorktree never touches the
// adapter; providerKey only selects which fake adapter the run would use).
func TestE2EWorktree_ProviderWorkspaceCwdParity(t *testing.T) {
	repo := initWorktreeRepo(t)
	svc, srv := worktreeHTTPServer(t, repo)
	for _, pk := range []string{"claude", "codex"} {
		runID := startWorktreeChatRun(t, srv, repo, map[string]any{"providerKey": pk})
		wtPath := runWorktreePath(t, srv, runID)
		svc.mu.Lock()
		rs := svc.runs[runID]
		var cwd string
		if rs != nil {
			cwd = rs.workspaceCwd
		}
		svc.mu.Unlock()
		if cwd == "" || cwd != wtPath {
			t.Fatalf("provider %s: workspaceCwd=%q want worktree path %q", pk, cwd, wtPath)
		}
	}
}

// 71-l: markChatWorktreeState must update every leg's in-memory binding AND
// every persisted session row for the chat — later legs read the latest
// lifecycle value on resume.
func TestMarkChatWorktreeState_PropagatesToLegsAndSessions(t *testing.T) {
	repo := initWorktreeRepo(t)
	svc, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, map[string]any{"providerKey": "claude"})

	// The run snapshot view does not expose chatId (field exists on the DTO
	// but runSnapshot never fills it) — read the binding owner from state.
	svc.mu.Lock()
	chatID := svc.runs[runID].chatID
	svc.mu.Unlock()
	if chatID == "" {
		t.Fatal("chat run must carry a minted chatID")
	}

	// Second leg on the same chat inherits the binding (D-8).
	runID2 := startWorktreeChatRun(t, srv, repo, map[string]any{
		"providerKey": "codex", "chatId": chatID,
	})

	// Persist session rows for both legs so the store half is observable.
	for _, id := range []string{runID, runID2} {
		if err := svc.persistProviderSession(ProviderSessionState{
			RunID: id, ChatID: chatID, WorktreeState: "active",
		}); err != nil {
			t.Fatalf("seed session %s: %v", id, err)
		}
	}

	_ = svc.markChatWorktreeState(chatID, "merge_pending")

	svc.mu.Lock()
	for _, id := range []string{runID, runID2} {
		rs := svc.runs[id]
		if rs == nil || rs.worktree == nil || rs.worktree.State != "merge_pending" {
			svc.mu.Unlock()
			t.Fatalf("run %s binding state not propagated", id)
		}
	}
	svc.mu.Unlock()

	reader, ok := svc.workflowStore.(ChatSessionReader)
	if !ok {
		t.Skip("session store lacks ChatSessionReader")
	}
	rows, err := reader.ListProviderSessionsByChat(context.Background(), chatID)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	states := map[string]string{}
	for _, row := range rows {
		states[row.RunID] = row.WorktreeState
	}
	for _, id := range []string{runID, runID2} {
		if states[id] != "merge_pending" {
			t.Fatalf("persisted session %s state=%q want merge_pending", id, states[id])
		}
	}
}
