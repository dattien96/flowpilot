package runner

// Task-424 (CP-82 P-3): parallel same-project worktree safety. Proves the
// owner-key contract (chatId for chat runs / runId for flow runs), concurrent
// provisioning yields distinct trees, and chat legs inherit one binding —
// the user's only duty is opting in; collision safety is code-guaranteed.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func worktreeRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	run := func(args ...string) {
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

func TestWorktreeOwnerIDFor_ChatRunsUseChatID(t *testing.T) {
	if got := worktreeOwnerIDFor("chat-9", "run-1", "chat"); got != "chat-9" {
		t.Fatalf("chat run owner = %q, want chat-9", got)
	}
}

func TestWorktreeOwnerIDFor_FlowRunsUseRunID(t *testing.T) {
	for _, kind := range []string{"workflow", "step", ""} {
		if got := worktreeOwnerIDFor("chat-9", "run-2", kind); got != "run-2" {
			t.Fatalf("flow run kind %q owner = %q, want run-2", kind, got)
		}
	}
	// Even a chat-kind run with no chatId falls back to the runId — never "".
	if got := worktreeOwnerIDFor("", "run-3", "chat"); got != "run-3" {
		t.Fatalf("chat run without chatId owner = %q, want run-3", got)
	}
}

func TestWorktreeOwnerIDFor_ChatAndFlowNeverCollide(t *testing.T) {
	// Chat legs share the chat owner; flow runs are per-run. A chat leg and a
	// flow run in the same repo must resolve different owners unless the ids
	// themselves are equal (impossible — ids are unique).
	chatOwner := worktreeOwnerIDFor("chat-7", "run-leg-1", "chat")
	flowOwner := worktreeOwnerIDFor("chat-7", "run-flow-1", "workflow")
	if chatOwner == flowOwner {
		t.Fatalf("chat leg and flow run share owner %q", chatOwner)
	}
	// Two legs of one chat share the owner BY DESIGN (single worktree).
	if a, b := worktreeOwnerIDFor("chat-7", "r1", "chat"), worktreeOwnerIDFor("chat-7", "r2", "chat"); a != b {
		t.Fatalf("chat legs got different owners %q vs %q", a, b)
	}
}

func TestProvisionRunWorktree_ConcurrentDistinctOwners(t *testing.T) {
	repoDir := worktreeRepo(t)
	s := &InteractiveService{runs: map[string]*interactiveRun{}}

	const n = 6
	errs := make([]error, n)
	runs := make([]*interactiveRun, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rs := &interactiveRun{id: "run-par-" + string(rune('a'+i))}
			// Production serializes provisioning under s.mu — mirror that.
			s.mu.Lock()
			e := s.provisionRunWorktree(rs, repoDir, worktreeOwnerIDFor("", rs.id, "workflow"), nil)
			s.mu.Unlock()
			if e != nil {
				errs[i] = e
				return
			}
			runs[i] = rs
		}(i)
	}
	wg.Wait()

	paths := map[string]bool{}
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("provision %d failed: %v", i, errs[i])
		}
		p := runs[i].workspaceCwd
		if paths[p] {
			t.Fatalf("two runs got the same worktree path %q", p)
		}
		paths[p] = true
		if runs[i].worktree == nil || runs[i].worktree.State != "active" {
			t.Fatalf("run %d missing active binding", i)
		}
	}
	if len(paths) != n {
		t.Fatalf("expected %d distinct paths, got %d", n, len(paths))
	}
	// git's own view agrees: N managed worktrees registered.
	out, err := exec.Command("git", "-C", repoDir, "worktree", "list", "--porcelain").CombinedOutput()
	if err != nil {
		t.Fatalf("worktree list: %v", err)
	}
	if got := strings.Count(string(out), "worktree "); got != n+1 {
		t.Fatalf("expected %d registered worktrees, porcelain shows %d", n+1, got)
	}
}

func TestChatLegsInheritExistingWorktreeBinding(t *testing.T) {
	repoDir := worktreeRepo(t)
	s := &InteractiveService{runs: map[string]*interactiveRun{}}

	leg1 := &interactiveRun{id: "run-leg-1", chatID: "chat-42"}
	owner := worktreeOwnerIDFor(leg1.chatID, leg1.id, "chat")
	s.mu.Lock()
	if e := s.provisionRunWorktree(leg1, repoDir, owner, nil); e != nil {
		s.mu.Unlock()
		t.Fatalf("leg1 provision: %v", e)
	}
	s.runs[leg1.id] = leg1
	s.mu.Unlock()
	path1 := leg1.workspaceCwd

	// Second leg of the SAME chat: inherits via the resident-binding lookup —
	// no second Create, same path, validated binding.
	inherited := s.findChatWorktreeBindingLocked(leg1.chatID)
	if inherited == nil {
		t.Fatal("no binding found for second leg")
	}
	leg2 := &interactiveRun{id: "run-leg-2", chatID: "chat-42"}
	owner2 := worktreeOwnerIDFor(leg2.chatID, leg2.id, "chat")
	s.mu.Lock()
	e := s.provisionRunWorktree(leg2, repoDir, owner2, inherited)
	s.mu.Unlock()
	if e != nil {
		t.Fatalf("leg2 provision: %v", e)
	}
	if leg2.workspaceCwd != path1 {
		t.Fatalf("leg2 got %q, want inherited %q", leg2.workspaceCwd, path1)
	}
	if leg2.worktree == nil || leg2.worktree.OwnerID != "chat-42" {
		t.Fatalf("leg2 binding wrong: %+v", leg2.worktree)
	}
	// Still exactly one managed worktree on disk.
	out, err := exec.Command("git", "-C", repoDir, "worktree", "list", "--porcelain").CombinedOutput()
	if err != nil {
		t.Fatalf("worktree list: %v", err)
	}
	if got := strings.Count(string(out), "worktree "); got != 2 {
		t.Fatalf("leg inheritance created extra worktrees: %d", got)
	}
}
