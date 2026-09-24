package runner

// CP-71 gap-fill (KR-005): HTTP-level coverage for the merge-resolve modes the
// original E2E file never exercised — worktree_unavailable on non-git cwd,
// keep_branch, the discard requiresConfirm two-step, archive/recreate_empty on
// a lost binding, apply_patch/keep_branch on lost, worktree_not_lost, and
// invalid_mode. Additive file — cp71_worktree_e2e_test.go untouched.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// resolveWorktreeConfirmHTTP is resolveWorktreeHTTP plus the confirm flag the
// discard-with-untracked path requires (run_worktree_merge.go).
func resolveWorktreeConfirmHTTP(t *testing.T, srv *httptest.Server, runID, mode string, confirm bool) (int, []byte) {
	t.Helper()
	return doJSON(t, http.MethodPost,
		srv.URL+"/client/workflow-runs/"+runID+"/worktree/resolve",
		map[string]any{"mode": mode, "confirm": confirm}, nil)
}

// gitBranchExists reports whether `name` is a local branch in dir.
func gitBranchExists(t *testing.T, dir, name string) bool {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "branch", "--list", name).Output()
	if err != nil {
		t.Fatalf("git branch --list: %v", err)
	}
	return strings.TrimSpace(string(out)) != ""
}

// loseWorktreeExternally replicates the E-8 setup: remove the worktree under
// the service's feet, restart the service over the same session store, and
// resume so the binding validates as lost (returns the second server).
func loseWorktreeExternally(t *testing.T, srv *httptest.Server, repo, runID string) *httptest.Server {
	t.Helper()
	wtPath := runWorktreePath(t, srv, runID)
	if err := exec.Command("git", "-C", repo, "worktree", "remove", "--force", wtPath).Run(); err != nil {
		t.Fatalf("external worktree remove: %v", err)
	}
	_ = os.RemoveAll(wtPath)

	_, srv2 := worktreeHTTPServer(t, repo)
	status, raw := doJSON(t, http.MethodPost,
		srv2.URL+"/client/workflow-runs/"+runID+"/resume", nil, nil)
	if status != http.StatusOK && status != http.StatusAccepted {
		t.Fatalf("resume: %d %s", status, raw)
	}
	if st := runWorktreeState(t, srv2, runID); st != "lost" {
		t.Fatalf("state=%s want lost", st)
	}
	return srv2
}

// TestE2EWorktree_StartOnNonGitRepoReturnsUnavailable: worktree:true against a
// cwd that is not a git repository must fail 400 worktree_unavailable — never
// silently degrade to a plain run (SS-23/P-2).
func TestE2EWorktree_StartOnNonGitRepoReturnsUnavailable(t *testing.T) {
	plain := t.TempDir() // deliberately NOT a git repo
	_, srv := worktreeHTTPServer(t, plain)

	status, raw := doJSON(t, http.MethodPost, srv.URL+"/client/workflow-runs",
		map[string]any{"projectId": "p-1", "chatMode": "normal_chat",
			"providerKey": "codex", "cwd": plain, "worktree": true},
		map[string]string{"X-Client": "desktop"})
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400, body=%s", status, raw)
	}
	if !strings.Contains(string(raw), "worktree_unavailable") {
		t.Fatalf("expected worktree_unavailable, got %s", raw)
	}
}

// TestE2EWorktree_KeepBranchHTTP: keep_branch removes the worktree dir but
// keeps the run/<slug> branch with all committed work reachable.
func TestE2EWorktree_KeepBranchHTTP(t *testing.T) {
	repo := initWorktreeRepo(t)
	svc, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, map[string]any{"providerKey": "claude"})
	wtPath := runWorktreePath(t, srv, runID)

	markRunTerminal(t, svc, runID)
	status, raw := resolveWorktreeHTTP(t, srv, runID, "keep_branch")
	if status != http.StatusOK {
		t.Fatalf("keep_branch: %d %s", status, raw)
	}
	var out struct {
		WorktreeState string `json:"worktreeState"`
		Branch        string `json:"branch"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.WorktreeState != "kept_branch" || out.Branch == "" {
		t.Fatalf("unexpected response: %+v", out)
	}
	if !gitBranchExists(t, repo, out.Branch) {
		t.Fatalf("branch %q must survive keep_branch", out.Branch)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree dir must be pruned after keep_branch, stat err=%v", err)
	}
	if st := runWorktreeState(t, srv, runID); st != "kept_branch" {
		t.Fatalf("persisted state=%s want kept_branch", st)
	}
}

// TestE2EWorktree_DiscardWithUntrackedRequiresConfirm: discard on a worktree
// containing untracked artifacts must 409 with requiresConfirm + the artifact
// list; resending with confirm=true proceeds (SD-27 Q-2 data-loss guard).
func TestE2EWorktree_DiscardWithUntrackedRequiresConfirm(t *testing.T) {
	repo := initWorktreeRepo(t)
	svc, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, nil)
	wtPath := runWorktreePath(t, srv, runID)

	// Provider left an artifact it never committed.
	if err := os.WriteFile(filepath.Join(wtPath, "untracked-artifact.log"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	markRunTerminal(t, svc, runID)

	status, raw := resolveWorktreeConfirmHTTP(t, srv, runID, "discard", false)
	if status != http.StatusConflict {
		t.Fatalf("discard without confirm: %d %s", status, raw)
	}
	var conflict struct {
		RequiresConfirm bool     `json:"requiresConfirm"`
		Untracked       []string `json:"untracked"`
		WorktreeState   string   `json:"worktreeState"`
	}
	if err := json.Unmarshal(raw, &conflict); err != nil {
		t.Fatalf("decode conflict payload: %v (%s)", err, raw)
	}
	if !conflict.RequiresConfirm {
		t.Fatalf("expected requiresConfirm, got %+v", conflict)
	}
	found := false
	for _, u := range conflict.Untracked {
		if strings.Contains(filepath.ToSlash(u), "untracked-artifact.log") {
			found = true
		}
	}
	if !found {
		t.Fatalf("untracked list missing artifact: %+v", conflict.Untracked)
	}

	// Second call with confirm=true discards for real.
	status, raw = resolveWorktreeConfirmHTTP(t, srv, runID, "discard", true)
	if status != http.StatusOK {
		t.Fatalf("discard with confirm: %d %s", status, raw)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree dir must be gone after discard, stat err=%v", err)
	}
	if st := runWorktreeState(t, srv, runID); st != "discarded" {
		t.Fatalf("state=%s want discarded", st)
	}
}

// TestE2EWorktree_LostBindingResolutionsHTTP: the two lost-state resolutions —
// archive (keep run, mark discarded, no git ops) and recreate_empty (new clean
// worktree at the recorded base commit) — plus the fail-closed guards on the
// live-state modes against a lost binding.
func TestE2EWorktree_LostBindingResolutionsHTTP(t *testing.T) {
	repo := initWorktreeRepo(t)
	_, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, map[string]any{"providerKey": "claude"})
	srv2 := loseWorktreeExternally(t, srv, repo, runID)

	// apply_patch / keep_branch on a lost binding must fail closed.
	for _, mode := range []string{"apply_patch", "keep_branch"} {
		status, raw := resolveWorktreeHTTP(t, srv2, runID, mode)
		if status != http.StatusConflict || !strings.Contains(string(raw), "worktree_lost") {
			t.Fatalf("%s on lost: %d %s, want 409 worktree_lost", mode, status, raw)
		}
	}

	// recreate_empty on a NON-lost binding must refuse.
	runID2 := startWorktreeChatRun(t, srv2, repo, map[string]any{"providerKey": "codex"})
	status, raw := resolveWorktreeHTTP(t, srv2, runID2, "recreate_empty")
	if status != http.StatusConflict || !strings.Contains(string(raw), "worktree_not_lost") {
		t.Fatalf("recreate_empty on live binding: %d %s, want 409 worktree_not_lost", status, raw)
	}

	// archive: run kept, binding marked discarded, no git ops needed.
	status, raw = resolveWorktreeHTTP(t, srv2, runID, "archive")
	if status != http.StatusOK {
		t.Fatalf("archive: %d %s", status, raw)
	}
	var archOut struct {
		WorktreeState string `json:"worktreeState"`
		Archived      bool   `json:"archived"`
	}
	if err := json.Unmarshal(raw, &archOut); err != nil {
		t.Fatalf("decode archive: %v", err)
	}
	if !archOut.Archived || archOut.WorktreeState != "discarded" {
		t.Fatalf("archive response malformed: %+v", archOut)
	}

	// recreate_empty: on a second lost run — new worktree at the ORIGINAL base
	// commit, binding active again, priorChangesLost flagged.
	runID3 := startWorktreeChatRun(t, srv2, repo, map[string]any{"providerKey": "claude"})
	srv3 := loseWorktreeExternally(t, srv2, repo, runID3)
	status, raw = resolveWorktreeHTTP(t, srv3, runID3, "recreate_empty")
	if status != http.StatusOK {
		t.Fatalf("recreate_empty: %d %s", status, raw)
	}
	var recOut struct {
		WorktreeState    string `json:"worktreeState"`
		Path             string `json:"path"`
		Recreated        bool   `json:"recreated"`
		PriorChangesLost bool   `json:"priorChangesLost"`
	}
	if err := json.Unmarshal(raw, &recOut); err != nil {
		t.Fatalf("decode recreate_empty: %v", err)
	}
	if !recOut.Recreated || !recOut.PriorChangesLost || recOut.WorktreeState != "active" {
		t.Fatalf("recreate_empty response malformed: %+v", recOut)
	}
	if recOut.Path == "" {
		t.Fatal("recreate_empty must return the new worktree path")
	}
	if _, err := os.Stat(recOut.Path); err != nil {
		t.Fatalf("recreated worktree missing on disk: %v", err)
	}
	if st := runWorktreeState(t, srv3, runID3); st != "active" {
		t.Fatalf("post-recreate state=%s want active", st)
	}
}

// TestE2EWorktree_InvalidModeRejectedHTTP: an unknown mode must fail 400
// invalid_mode and leave the binding untouched.
func TestE2EWorktree_InvalidModeRejectedHTTP(t *testing.T) {
	repo := initWorktreeRepo(t)
	_, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, nil)

	status, raw := resolveWorktreeHTTP(t, srv, runID, "nuke_everything")
	if status != http.StatusBadRequest || !strings.Contains(string(raw), "invalid_mode") {
		t.Fatalf("invalid mode: %d %s, want 400 invalid_mode", status, raw)
	}
	if st := runWorktreeState(t, srv, runID); st != "active" {
		t.Fatalf("binding state changed by invalid mode: %s", st)
	}
}

// TestE2EWorktree_KeepBranchWithUncommittedRequiresConfirm: live-found gap —
// keep_branch removes the worktree but keeps only the branch pointer; any
// uncommitted delta (untracked OR modified tracked files) is silently lost
// because it was never on the branch. Same data-loss guard as discard: 409
// requiresConfirm + the uncommitted path list; confirm=true proceeds.
func TestE2EWorktree_KeepBranchWithUncommittedRequiresConfirm(t *testing.T) {
	repo := initWorktreeRepo(t)
	svc, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, map[string]any{"providerKey": "claude"})
	wtPath := runWorktreePath(t, srv, runID)

	// Provider left an uncommitted artifact AND modified a tracked file.
	if err := os.WriteFile(filepath.Join(wtPath, "uncommitted.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	markRunTerminal(t, svc, runID)

	status, raw := resolveWorktreeConfirmHTTP(t, srv, runID, "keep_branch", false)
	if status != http.StatusConflict {
		t.Fatalf("keep_branch without confirm: %d %s — uncommitted work must not be silently dropped", status, raw)
	}
	var conflict struct {
		RequiresConfirm bool     `json:"requiresConfirm"`
		Uncommitted     []string `json:"uncommitted"`
	}
	if err := json.Unmarshal(raw, &conflict); err != nil {
		t.Fatalf("decode conflict payload: %v (%s)", err, raw)
	}
	if !conflict.RequiresConfirm {
		t.Fatalf("expected requiresConfirm, got %+v", conflict)
	}
	found := false
	for _, u := range conflict.Uncommitted {
		if strings.Contains(filepath.ToSlash(u), "uncommitted.txt") {
			found = true
		}
	}
	if !found {
		t.Fatalf("uncommitted list missing artifact: %+v", conflict.Uncommitted)
	}
	// Worktree must still be there — the user has not confirmed the loss.
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatal("worktree must survive an unconfirmed keep_branch")
	}

	status, raw = resolveWorktreeConfirmHTTP(t, srv, runID, "keep_branch", true)
	if status != http.StatusOK {
		t.Fatalf("keep_branch with confirm: %d %s", status, raw)
	}
	if st := runWorktreeState(t, srv, runID); st != "kept_branch" {
		t.Fatalf("state=%s want kept_branch", st)
	}
}
