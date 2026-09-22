package runner

// E2E tests for CP-71 (run/chat worktree isolation, SS-23 / SD-27).
// These tests drive the feature through the real HTTP API surface
// (httptest + RegisterInteractiveRoutes + doJSON) on real git repos
// (t.TempDir + git init), sharing a real localFileSessionStore across
// service instances for restart coverage — the same convention as
// amend_flow_endpoint_test.go / interactive_service_e2e_test.go.
//
// Scenarios covered:
//   1. Full lifecycle over HTTP: start with worktree:true → binding persisted,
//      cwd inside worktree → terminal → worktree_merge_requested in event log →
//      resolve apply_patch → file lands in main workspace → worktree pruned.
//   2. Provider-switch leg (switchFromRunId) shares the chat worktree — same
//      worktreePath, no second `git worktree add`, leg-1 files visible.
//   3. Conflict path: main drifts → apply_patch → 409 + evidence payload;
//      state stays merge_pending; fix drift → retry apply_patch → merged.
//   4. Recovery: restart service over the same sessions.ndjson → resume
//      validates the binding (D-7) → run keeps the worktree cwd.
//   5. External deletion → resume → worktreeState=lost + worktree_lost event;
//      new legs in the lost chat are blocked (E-8); never silently recreated.
//   6. Client gate: worktree:true without X-Client desktop|tui → 403.
//   7. Boot GC prunes orphaned worktrees only — live bindings survive.
//   8. Toggle-off byte parity: no worktree → no worktree* fields anywhere.
//
// Provider-agnosticism evidence: the feature path contains zero providerKey
// branching (Case-1); tests exercise codex/claude labels interchangeably.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// initWorktreeRepo creates a real git repo in t.TempDir with one committed
// file, returning the repo dir (Task-369 convention: real git, no mocks).
func initWorktreeRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@flowpilot")
	run("config", "user.name", "flowpilot-test")
	run("config", "core.autocrlf", "false")
	if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "seed")
	return dir
}

func gitHeadAtDir(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// worktreeHTTPServer boots an InteractiveService bound to the repo via a real
// localFileSessionStore under <repo>/.flowpilot — a second service over the
// same repo therefore sees the same persisted sessions (restart simulation).
// The gitnexus auto-index and knowledge bootstrap are pre-marked so tests
// never spawn the gitnexus CLI inside temp repos.
func worktreeHTTPServer(t *testing.T, repoDir string) (*InteractiveService, *httptest.Server) {
	t.Helper()
	store, err := NewLocalFileSessionStore(filepath.Join(repoDir, ".flowpilot"))
	if err != nil {
		t.Fatalf("session store: %v", err)
	}
	reg := newProviderRegistry()
	for _, key := range []ProviderKey{ProviderKeyCodex, ProviderKeyClaude} {
		reg.register(ProviderRegistration{
			Key:          key,
			Status:       ProviderStatusAvailable,
			Capabilities: ProviderCapabilities{Streaming: true},
			newAdapter: func() ProviderRuntimeAdapter {
				return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, _ TurnBridge) error { return nil })
			},
		})
	}
	svc := newInteractiveService(reg, newInteractiveCatalog(), store)
	svc.mu.Lock()
	svc.gitnexusAnalyzeOnce[repoDir] = true
	svc.mu.Unlock()
	knowledgeBootstrapOnce.Store(repoDir, true)
	svc.AttachRunner(&Runner{workspace: repoDir})
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return svc, srv
}

// startWorktreeChatRun starts a chat run with worktree:true over HTTP.
func startWorktreeChatRun(t *testing.T, srv *httptest.Server, repo string, extra map[string]any) string {
	t.Helper()
	body := map[string]any{
		"projectId": "p-1", "chatMode": "normal_chat",
		"providerKey": "codex", "cwd": repo, "worktree": true,
	}
	for k, v := range extra {
		body[k] = v
	}
	status, raw := doJSON(t, http.MethodPost, srv.URL+"/client/workflow-runs",
		body, map[string]string{"X-Client": "desktop"})
	if status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("start run: status=%d body=%s", status, raw)
	}
	var resp struct {
		RunID string `json:"runId"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || resp.RunID == "" {
		t.Fatalf("decode runId: %v body=%s", err, raw)
	}
	return resp.RunID
}

// startWorktreeFlowRun starts a workflow (flow) run with worktree:true.
func startWorktreeFlowRun(t *testing.T, srv *httptest.Server, repo string) string {
	t.Helper()
	status, raw := doJSON(t, http.MethodPost, srv.URL+"/client/workflow-runs",
		map[string]any{
			"projectId": "p-1", "workflowId": "wf-x", "stepId": "step-plan",
			"providerKey": "codex", "cwd": repo, "worktree": true,
		}, map[string]string{"X-Client": "desktop"})
	if status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("start flow run: status=%d body=%s", status, raw)
	}
	var resp struct {
		RunID string `json:"runId"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || resp.RunID == "" {
		t.Fatalf("decode runId: %v body=%s", err, raw)
	}
	return resp.RunID
}

// runEventLog fetches the persisted event log for a run and returns event
// types in order (live HTTP surface, not internal state).
func runEventLog(t *testing.T, srv *httptest.Server, runID string) []string {
	t.Helper()
	status, raw := doJSON(t, http.MethodGet,
		srv.URL+"/admin/workflow-runs/"+runID+"/events", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("events: status=%d body=%s", status, raw)
	}
	var events []struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatalf("decode events: %v body=%s", err, raw)
	}
	types := make([]string, 0, len(events))
	for _, e := range events {
		types = append(types, e.Type)
	}
	return types
}

func logHasEvent(types []string, want string) bool {
	for _, ty := range types {
		if ty == want {
			return true
		}
	}
	return false
}

func runWorktreePath(t *testing.T, srv *httptest.Server, runID string) string {
	t.Helper()
	status, raw := doJSON(t, http.MethodGet,
		srv.URL+"/client/workflow-runs/"+runID, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("get run: %d", status)
	}
	var snap struct {
		Worktree *struct {
			Path string `json:"path"`
		} `json:"worktree"`
	}
	if err := json.Unmarshal(raw, &snap); err != nil || snap.Worktree == nil {
		t.Fatalf("no worktree block: %s", raw)
	}
	return snap.Worktree.Path
}

func runWorktreeState(t *testing.T, srv *httptest.Server, runID string) string {
	t.Helper()
	_, raw := doJSON(t, http.MethodGet,
		srv.URL+"/client/workflow-runs/"+runID, nil, nil)
	var snap struct {
		Worktree *struct {
			State string `json:"state"`
		} `json:"worktree"`
	}
	if err := json.Unmarshal(raw, &snap); err != nil || snap.Worktree == nil {
		return ""
	}
	return snap.Worktree.State
}

// markRunTerminal drives a run to the terminal status the merge-card hook
// keys on (same state the flow executor sets).
func markRunTerminal(t *testing.T, svc *InteractiveService, runID string) {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs == nil {
		t.Fatalf("run %s not resident", runID)
	}
	rs.status = RunStatusCompleted
	svc.maybeEmitWorktreeMergeRequest(rs)
}

func resolveWorktreeHTTP(t *testing.T, srv *httptest.Server, runID, mode string) (int, []byte) {
	t.Helper()
	return doJSON(t, http.MethodPost,
		srv.URL+"/client/workflow-runs/"+runID+"/worktree/resolve",
		map[string]any{"mode": mode}, nil)
}

// ── 1. Full lifecycle over HTTP (flow run → terminal merge trigger) ─────────

func TestE2EWorktree_FullLifecycleHTTP(t *testing.T) {
	repo := initWorktreeRepo(t)
	base := gitHeadAtDir(t, repo)
	svc, srv := worktreeHTTPServer(t, repo)

	runID := startWorktreeFlowRun(t, srv, repo)

	status, raw := doJSON(t, http.MethodGet,
		srv.URL+"/client/workflow-runs/"+runID, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("get run: %d %s", status, raw)
	}
	var snap struct {
		Worktree *struct {
			OwnerID, Path, Branch, BaseCommit, State string
		} `json:"worktree"`
		WorkingDirectory string `json:"workingDirectory"`
	}
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if snap.Worktree == nil || snap.Worktree.State != "active" {
		t.Fatalf("expected active worktree binding, got %+v", snap.Worktree)
	}
	if snap.Worktree.OwnerID != runID {
		t.Fatalf("flow owner=%s want self runId %s", snap.Worktree.OwnerID, runID)
	}
	if snap.Worktree.BaseCommit != base {
		t.Fatalf("baseCommit=%s want %s", snap.Worktree.BaseCommit, base)
	}
	if snap.WorkingDirectory != snap.Worktree.Path {
		t.Fatalf("cwd=%s want worktree %s", snap.WorkingDirectory, snap.Worktree.Path)
	}
	if _, err := os.Stat(snap.Worktree.Path); err != nil {
		t.Fatalf("worktree dir missing: %v", err)
	}
	if !strings.Contains(filepath.ToSlash(snap.Worktree.Path), ".flowpilot/worktrees/") {
		t.Fatalf("worktree path outside convention: %s", snap.Worktree.Path)
	}

	newFile := filepath.Join(snap.Worktree.Path, "feature.txt")
	if err := os.WriteFile(newFile, []byte("isolated work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	markRunTerminal(t, svc, runID)

	waitLoop(t, "worktree_merge_requested", 5*time.Second, func() bool {
		return logHasEvent(runEventLog(t, srv, runID), "worktree_merge_requested")
	})

	status, raw = resolveWorktreeHTTP(t, srv, runID, "apply_patch")
	if status != http.StatusOK {
		t.Fatalf("resolve: %d %s", status, raw)
	}
	got, err := os.ReadFile(filepath.Join(repo, "feature.txt"))
	if err != nil || string(got) != "isolated work\n" {
		t.Fatalf("patch not applied to main: %v %q", err, got)
	}
	out, _ := exec.Command("git", "-C", repo, "worktree", "list", "--porcelain").Output()
	if strings.Contains(string(out), ".flowpilot") {
		t.Fatalf("worktree not cleaned: %s", out)
	}
	if st := runWorktreeState(t, srv, runID); st != "merged" {
		t.Fatalf("state=%s want merged", st)
	}
}

// ── 2. Provider-switch leg shares the chat worktree ─────────────────────────

func TestE2EWorktree_LegSwitchSharesWorktreeHTTP(t *testing.T) {
	repo := initWorktreeRepo(t)
	_, srv := worktreeHTTPServer(t, repo)

	leg1 := startWorktreeChatRun(t, srv, repo, map[string]any{"providerKey": "claude"})
	path1 := runWorktreePath(t, srv, leg1)
	if err := os.WriteFile(filepath.Join(path1, "leg1.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	leg2 := startWorktreeChatRun(t, srv, repo, map[string]any{"switchFromRunId": leg1})
	path2 := runWorktreePath(t, srv, leg2)
	if path2 != path1 {
		t.Fatalf("leg2 worktree=%s want shared %s", path2, path1)
	}
	if _, err := os.Stat(filepath.Join(path2, "leg1.txt")); err != nil {
		t.Fatalf("leg1 file not visible to leg2: %v", err)
	}
	out, _ := exec.Command("git", "-C", repo, "worktree", "list").Output()
	if strings.Count(string(out), ".flowpilot") != 1 {
		t.Fatalf("expected 1 worktree, got:\n%s", out)
	}
}

// ── 3. Conflict evidence + retry over HTTP ───────────────────────────────────

func TestE2EWorktree_ConflictEvidenceAndRetryHTTP(t *testing.T) {
	repo := initWorktreeRepo(t)
	svc, srv := worktreeHTTPServer(t, repo)

	runID := startWorktreeFlowRun(t, srv, repo)
	wtPath := runWorktreePath(t, srv, runID)

	if err := os.WriteFile(filepath.Join(wtPath, "seed.txt"), []byte("wt version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Drift main: commit a different version of the same file.
	if err := os.WriteFile(filepath.Join(repo, "seed.txt"), []byte("main version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	drift := exec.Command("git", "-C", repo, "add", ".")
	_ = drift.Run()
	drift = exec.Command("git", "-C", repo, "commit", "-m", "drift")
	_ = drift.Run()

	markRunTerminal(t, svc, runID)
	waitLoop(t, "merge card", 5*time.Second, func() bool {
		return logHasEvent(runEventLog(t, srv, runID), "worktree_merge_requested")
	})

	status, raw := resolveWorktreeHTTP(t, srv, runID, "apply_patch")
	if status != http.StatusConflict {
		t.Fatalf("conflict apply: status=%d body=%s", status, raw)
	}
	var conflict struct {
		ConflictPaths []string `json:"conflictPaths"`
		PatchRef      string   `json:"patchArtifactRef"`
	}
	if err := json.Unmarshal(raw, &conflict); err != nil ||
		len(conflict.ConflictPaths) == 0 || conflict.PatchRef == "" {
		t.Fatalf("conflict evidence missing: %s", raw)
	}
	if st := runWorktreeState(t, srv, runID); st != "merge_pending" {
		t.Fatalf("state=%s want merge_pending", st)
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("worktree must survive conflict: %v", err)
	}

	// Fix the drift (revert main to the worktree base content), retry same mode.
	fix := exec.Command("git", "-C", repo, "checkout", "HEAD~1", "--", "seed.txt")
	_ = fix.Run()
	fix = exec.Command("git", "-C", repo, "commit", "-m", "revert drift")
	_ = fix.Run()
	status, raw = resolveWorktreeHTTP(t, srv, runID, "apply_patch")
	if status != http.StatusOK {
		t.Fatalf("retry apply: %d %s", status, raw)
	}
	if st := runWorktreeState(t, srv, runID); st != "merged" {
		t.Fatalf("state=%s want merged", st)
	}
}

// ── 4+5. Recovery & lost over HTTP ───────────────────────────────────────────

func TestE2EWorktree_ResumeAfterRestartHTTP(t *testing.T) {
	repo := initWorktreeRepo(t)
	_, srv := worktreeHTTPServer(t, repo)
	// Claude + synthetic thread-* session id → skipsResumeSessionValidation
	// skips the provider session-file check, so resume reaches the worktree
	// binding validation directly.
	runID := startWorktreeChatRun(t, srv, repo, map[string]any{"providerKey": "claude"})
	wtPath := runWorktreePath(t, srv, runID)

	// New service over the same sessions.ndjson = process restart.
	_, srv2 := worktreeHTTPServer(t, repo)
	status, raw := doJSON(t, http.MethodPost,
		srv2.URL+"/client/workflow-runs/"+runID+"/resume", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("resume: %d %s", status, raw)
	}
	if got := runWorktreePath(t, srv2, runID); got != wtPath {
		t.Fatalf("resumed cwd=%s want %s", got, wtPath)
	}
	if st := runWorktreeState(t, srv2, runID); st != "active" {
		t.Fatalf("state=%s want active", st)
	}
}

func TestE2EWorktree_ExternalDeleteMarksLostHTTP(t *testing.T) {
	repo := initWorktreeRepo(t)
	_, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, map[string]any{"providerKey": "claude"})
	wtPath := runWorktreePath(t, srv, runID)

	// User deletes the worktree externally.
	_ = exec.Command("git", "-C", repo, "worktree", "remove", "--force", wtPath).Run()
	_ = os.RemoveAll(wtPath)

	_, srv2 := worktreeHTTPServer(t, repo)
	_, _ = doJSON(t, http.MethodPost,
		srv2.URL+"/client/workflow-runs/"+runID+"/resume", nil, nil)

	if st := runWorktreeState(t, srv2, runID); st != "lost" {
		t.Fatalf("state=%s want lost", st)
	}
	waitLoop(t, "worktree_lost", 5*time.Second, func() bool {
		return logHasEvent(runEventLog(t, srv2, runID), "worktree_lost")
	})
	if _, err := os.Stat(wtPath); err == nil {
		t.Fatal("worktree must NOT be silently recreated")
	}
	// New leg in the lost chat is blocked behind the notice (E-8).
	status, raw := doJSON(t, http.MethodPost, srv2.URL+"/client/workflow-runs",
		map[string]any{"projectId": "p-1", "chatMode": "normal_chat",
			"providerKey": "codex", "cwd": repo,
			"switchFromRunId": runID, "worktree": true},
		map[string]string{"X-Client": "desktop"})
	if status == http.StatusOK || status == http.StatusCreated {
		t.Fatalf("leg in lost chat must not start clean: %s", raw)
	}
}

// ── 6+7+8. Gate, GC, parity ──────────────────────────────────────────────────

func TestE2EWorktree_ClientGateHTTP(t *testing.T) {
	repo := initWorktreeRepo(t)
	_, srv := worktreeHTTPServer(t, repo)

	for _, hdr := range []map[string]string{
		nil, {"X-Client": "admin"}, {"X-Client": "mcp"},
	} {
		status, raw := doJSON(t, http.MethodPost, srv.URL+"/client/workflow-runs",
			map[string]any{"projectId": "p-1", "chatMode": "normal_chat",
				"providerKey": "codex", "cwd": repo, "worktree": true}, hdr)
		if status != http.StatusForbidden {
			t.Fatalf("hdr=%v status=%d want 403 body=%s", hdr, status, raw)
		}
	}
}

func TestE2EWorktree_BootGCPrunesOnlyOrphansHTTP(t *testing.T) {
	repo := initWorktreeRepo(t)
	_, srv := worktreeHTTPServer(t, repo)

	live := startWorktreeChatRun(t, srv, repo, nil)
	livePath := runWorktreePath(t, srv, live)
	orphan := filepath.Join(repo, ".flowpilot", "worktrees", "ghost-123")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}

	// Second service over the same store: boot sweep must prune only the
	// orphan — the live chat's worktree is bound in sessions.ndjson.
	_, _ = worktreeHTTPServer(t, repo)
	waitLoop(t, "orphan prune", 5*time.Second, func() bool {
		_, err := os.Stat(orphan)
		return os.IsNotExist(err)
	})
	if _, err := os.Stat(livePath); err != nil {
		t.Fatal("live worktree must survive boot GC")
	}
}

// CP-81 live-run finding: a status write through sessionStateOf lands after
// the binding row and — under last-wins — erases the persisted worktree
// fields. The next boot's GC then prunes the worktree (unmerged work lost)
// and resolve 404s instead of surfacing the merge card. Reproduced here:
// a terminal run + a plain session write must NOT drop the binding.
func TestE2EWorktree_PostBindingSessionWriteKeepsBindingHTTP(t *testing.T) {
	repo := initWorktreeRepo(t)
	svc, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, nil)
	wtPath := runWorktreePath(t, srv, runID)
	if err := os.WriteFile(filepath.Join(wtPath, "kept.txt"), []byte("unmerged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	markRunTerminal(t, svc, runID)

	// Every status write path (running/completed/cancelled, drain stop-all)
	// persists sessionStateOf — emulate one landing after the binding row.
	svc.mu.Lock()
	rs := svc.runs[runID]
	svc.mu.Unlock()
	if rs == nil {
		t.Fatal("run not resident")
	}
	if err := svc.persistProviderSession(sessionStateOf(rs)); err != nil {
		t.Fatalf("persist sessionStateOf: %v", err)
	}

	// Restart: fresh service over the same store runs boot GC. A sentinel
	// orphan proves the sweep actually ran before we assert survival.
	orphan := filepath.Join(repo, ".flowpilot", "worktrees", "ghost-terminal")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}
	_, srv2 := worktreeHTTPServer(t, repo)
	waitLoop(t, "orphan prune", 5*time.Second, func() bool {
		_, err := os.Stat(orphan)
		return os.IsNotExist(err)
	})
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("terminal run's bound worktree was pruned: %v", err)
	}
	// The merge decision must still be resolvable post-restart.
	status, raw := resolveWorktreeHTTP(t, srv2, runID, "apply_patch")
	if status != http.StatusOK {
		t.Fatalf("resolve after restart: %d %s", status, raw)
	}
	if got, err := os.ReadFile(filepath.Join(repo, "kept.txt")); err != nil || string(got) != "unmerged\n" {
		t.Fatalf("patch not applied: %v %q", err, got)
	}
}

func TestE2EWorktree_OffByteParityHTTP(t *testing.T) {
	repo := initWorktreeRepo(t)
	_, srv := worktreeHTTPServer(t, repo)

	status, raw := doJSON(t, http.MethodPost, srv.URL+"/client/workflow-runs",
		map[string]any{"projectId": "p-1", "chatMode": "normal_chat",
			"providerKey": "codex", "cwd": repo},
		map[string]string{"X-Client": "desktop"})
	if status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("start: %d %s", status, raw)
	}
	var resp struct {
		RunID string `json:"runId"`
	}
	_ = json.Unmarshal(raw, &resp)
	_, raw = doJSON(t, http.MethodGet,
		srv.URL+"/client/workflow-runs/"+resp.RunID, nil, nil)
	if strings.Contains(string(raw), `"worktree"`) {
		t.Fatalf("toggle-off run must carry no worktree fields: %s", raw)
	}
}
