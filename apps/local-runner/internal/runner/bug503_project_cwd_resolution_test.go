package runner

// BUG-503 (live run-15708): an API-launched run carrying a bound projectId but
// no explicit cwd silently bound the runner process's own workspace
// (/tmp/fp-live-2026) instead of the project's registered root
// (/Users/tiendat/fp-beds/full). Everything downstream ran against the wrong
// tree: preflight_contract_freeze minted a contract with no base_sha (non-git
// dir), the context package reported every declared path not_found, and the
// reproducer child wrote strutil2/ into the runner's home — polluting it.
//
// Fix: when a project is bound and cwd is empty, resolve the project's
// registered Path via lookupProject before falling back to the runner
// workspace. The BUG-455 runner-workspace default still applies when no
// project resolves (unknown id / catalog unavailable) — those tests pin the
// fallback, this file pins the preference order.

import "testing"

// TestBug503_ProjectBoundRunResolvesProjectPath asserts the preference order:
// a bound project whose registered Path resolves must win over the runner
// workspace default.
func TestBug503_ProjectBoundRunResolvesProjectPath(t *testing.T) {
	projectDir := t.TempDir()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), &stubCatalogStore{
		projects: []Project{{ID: "proj-bed", Name: "Bed", Path: projectDir}},
	}, newFakeWorkflowStore())
	svc.runner = &Runner{workspace: t.TempDir()}

	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj-bed", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	got := svc.runs[h.RunID].workspaceCwd
	svc.mu.Unlock()
	if got != projectDir {
		t.Fatalf("project-bound run must bind project root %q, got runner workspace %q", projectDir, got)
	}
}

// TestBug503_UnknownProjectKeepsRunnerDefault pins the BUG-455 fallback for
// the other half: when the bound project does not resolve (unknown id or the
// catalog cannot list it) the runner workspace default still applies — a
// catalog outage must not break run creation.
func TestBug503_UnknownProjectKeepsRunnerDefault(t *testing.T) {
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), &stubCatalogStore{
		projects: []Project{{ID: "proj-bed", Name: "Bed", Path: t.TempDir()}},
	}, newFakeWorkflowStore())
	ws := t.TempDir()
	svc.runner = &Runner{workspace: ws}

	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj-unknown", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	got := svc.runs[h.RunID].workspaceCwd
	svc.mu.Unlock()
	if got != ws {
		t.Fatalf("unresolvable project must keep runner workspace %q, got %q", ws, got)
	}
}

// TestBug503_ExplicitCwdBeatsProjectPath pins that a caller-supplied cwd stays
// authoritative even when the project resolves — the desktop client's explicit
// workspace choice wins over the project registry.
func TestBug503_ExplicitCwdBeatsProjectPath(t *testing.T) {
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), &stubCatalogStore{
		projects: []Project{{ID: "proj-bed", Name: "Bed", Path: t.TempDir()}},
	}, newFakeWorkflowStore())
	svc.runner = &Runner{workspace: t.TempDir()}
	custom := t.TempDir()

	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj-bed", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui", Cwd: custom,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	got := svc.runs[h.RunID].workspaceCwd
	svc.mu.Unlock()
	if got != custom {
		t.Fatalf("explicit cwd must stay authoritative, got %q want %q", got, custom)
	}
}

// TestBug503_NonexistentProjectPathKeepsRunnerDefault pins the stale-
// registration guard: a bound project whose registered Path does not exist on
// disk must NOT be bound — the post-turn gate's turn-scoped git diff fails
// closed on a non-directory cwd and would block every turn forever (the same
// failure class the bug itself reported). The runner workspace default wins.
func TestBug503_NonexistentProjectPathKeepsRunnerDefault(t *testing.T) {
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), &stubCatalogStore{
		projects: []Project{{ID: "proj-bed", Name: "Bed", Path: "/nonexistent/proj-bed-path"}},
	}, newFakeWorkflowStore())
	ws := t.TempDir()
	svc.runner = &Runner{workspace: ws}

	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj-bed", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	got := svc.runs[h.RunID].workspaceCwd
	svc.mu.Unlock()
	if got != ws {
		t.Fatalf("nonexistent project path must keep runner workspace %q, got %q", ws, got)
	}
}
