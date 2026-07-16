package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/flowgate"
)

// Task-242 B1: rule-family helpers are stable.
func TestFlowGateRuleFamilies(t *testing.T) {
	if len(flowgate.DocScopeRuleIDs()) == 0 {
		t.Fatal("DocScopeRuleIDs empty")
	}
	if !flowgate.IsDocScopeRule("r-ca") {
		t.Error("r-ca should be doc/scope")
	}
	if flowgate.IsDocScopeRule("r-tests") {
		t.Error("r-tests is tier-2, not doc/scope")
	}
	if !flowgate.IsArtifactRule("r-artifact-output") {
		t.Error("r-artifact-output should be artifact")
	}
	if len(flowgate.TestRuleIDs()) != 2 {
		t.Fatalf("TestRuleIDs = %v", flowgate.TestRuleIDs())
	}
}

// Task-242 D-7: git commit detection is token-aware.
func TestLooksLikeGitCommitCommand(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
	}{
		{"git commit -m 'x'", true},
		{"git  commit -am x", true},
		{`/usr/bin/git commit -m "hi"`, true},
		{"git -C . commit -m x", true},
		{"git -c user.name=a commit -m x", true},
		{"git --git-dir=.git commit -m x", true},
		{"git -C /tmp -c user.email=a@b commit -am msg", true},
		{`git -C "C:\work\repo with spaces" commit -m x`, true},
		{`git -C 'C:\work\repo with spaces' commit -m x`, true},
		{`git --git-dir="C:\work\repo with spaces\.git" commit -m x`, true},
		{"git status", false},
		{"git -C . status", false},
		{`git -C "C:\work\repo with spaces" status`, false},
		{"echo git commitment", false},
		{"npm run commit", false},
		{"", false},
		// BUG-288 #7 shell wrappers
		{`sh -c 'git commit -m x'`, true},
		{`bash -lc "git commit -m hi"`, true},
		{`sh -c 'git status'`, false},
	}
	for _, tc := range cases {
		if got := looksLikeGitCommitCommand(tc.cmd); got != tc.want {
			t.Errorf("looksLikeGitCommitCommand(%q) = %v, want %v", tc.cmd, got, tc.want)
		}
	}
}

// Task-242 D-7: coding child of flow-engine parent is denied git commit.
func TestIsFlowCodingCommitAttempt(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("child: %v", err)
	}
	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[child.RunID].parentRunID = parent.RunID
	svc.runs[child.RunID].label = "coder"
	svc.mu.Unlock()

	details := ApprovalDetails{Kind: "exec", Command: "git commit -m 'x'"}
	if !isFlowCodingCommitAttempt(svc, svc.runs[child.RunID], details) {
		// Need lock for runs access — re-fetch under lock path:
		svc.mu.Lock()
		rs := svc.runs[child.RunID]
		svc.mu.Unlock()
		if !isFlowCodingCommitAttempt(svc, rs, details) {
			t.Fatal("expected coding child git commit to be denied")
		}
	}
	// Hub itself is not a coding child.
	svc.mu.Lock()
	hub := svc.runs[parent.RunID]
	svc.mu.Unlock()
	if isFlowCodingCommitAttempt(svc, hub, details) {
		t.Fatal("hub must not match coding-commit deny")
	}
	// Non-commit exec is fine.
	svc.mu.Lock()
	rs := svc.runs[child.RunID]
	svc.mu.Unlock()
	if isFlowCodingCommitAttempt(svc, rs, ApprovalDetails{Kind: "exec", Command: "git status"}) {
		t.Fatal("git status must not match")
	}
}

// Task-242: RequestApproval denies commit even under YOLO.
func TestRequestApprovalDeniesFlowCodingCommitUnderYolo(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("child: %v", err)
	}
	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[child.RunID].parentRunID = parent.RunID
	svc.runs[child.RunID].label = "coder"
	rs := svc.runs[child.RunID]
	svc.mu.Unlock()

	bridge := &turnBridge{svc: svc, rs: rs, yolo: true, turnID: "t1"}
	decision, aerr := bridge.RequestApproval(ApprovalDetails{Kind: "exec", Command: "git commit -m hi"})
	if aerr != nil {
		t.Fatalf("RequestApproval: %v", aerr)
	}
	if decision != "deny" {
		t.Fatalf("decision = %q, want deny (even under YOLO)", decision)
	}
}

// Task-242 D-1: child gate captures contract so r-contract does not fire when
// the coder declared scope in the final message.
func TestChildGateCapturesContractDeclared(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "main.go")
	run("commit", "-m", "init")
	// Dirty tree so ObserveGitDiffSince is non-empty.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n// edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("child: %v", err)
	}
	nodes := reviewLoopTestNodes()
	// Enforce mode so undeclared would reprompt.
	settings := filepath.Join(dir, ".flowpilot", "settings")
	if err := os.MkdirAll(settings, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settings, "flow-rules.json"), []byte(`{"mode":"enforce","rules":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[child.RunID].parentRunID = parent.RunID
	svc.runs[child.RunID].label = "coder"
	svc.runs[child.RunID].workspaceCwd = dir
	svc.runs[child.RunID].turnStartGitHead = ""
	rs := svc.runs[child.RunID]
	svc.mu.Unlock()

	msg := `[Change Contract]
feature: demo
intent: edit main
files: main.go
`
	// With declaration present, tier-1 r-contract must not treat as undeclared.
	// (CA note rules may still fire; we only assert capture did not force r-contract.)
	blocked := svc.runChildArtifactOutputGate(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: msg,
		ChangedFiles: []string{"main.go"},
	})
	_ = blocked // may still block on r-ca etc.; contract capture is the invariant
	// Contract must be keyed by PARENT flow run id (downstream prompts use parentRunID).
	store, storeErr := changecontract.OpenStoreReadOnly(dir)
	var c changecontract.Contract
	var ok bool
	if storeErr != nil || store == nil {
		st2, err2 := changecontract.NewStore(dir)
		if err2 != nil {
			t.Fatalf("store: %v / %v", storeErr, err2)
		}
		c, ok = st2.GetLatestForRun(parent.RunID)
	} else {
		c, ok = store.GetLatestForRun(parent.RunID)
	}
	if !ok || c.Confidence != changecontract.ConfidenceDeclared {
		t.Fatalf("expected declared contract for parent run, ok=%v conf=%q", ok, c.Confidence)
	}
	if c.StepID != "coder" {
		t.Fatalf("step identity = %q, want coder (child node label)", c.StepID)
	}
	// Must NOT be stored only under the child run id.
	if store2, err := changecontract.NewStore(dir); err == nil {
		if _, childOnly := store2.GetLatestForRun(child.RunID); childOnly && c.RunID == child.RunID {
			t.Fatal("contract run_id must be parent, not child")
		}
	}
	// Downstream inject (Continue / validate-retry / inline delegate) must see it.
	prompt := appendChangeContractIfAny(dir, parent.RunID, "base prompt for continue")
	if !strings.Contains(prompt, "Change Contract") || !strings.Contains(prompt, "main.go") {
		t.Fatalf("appendChangeContractIfAny must inject declared scope by parentRunID, got %q", prompt)
	}
}

// Task-242 P0: reviewer on dirty worktree must not capture/overwrite coder contract.
func TestReviewerChildDoesNotOverwriteCoderContract(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "main.go")
	run("commit", "-m", "init")
	// Coder left uncommitted edits (dirty worktree reviewers observe).
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n// coder edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Seed declared contract as if coder already captured under parent run.
	store, storeErr := changecontract.NewStore(dir)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	svc, _ := newTestServer(t)
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if aerr != nil {
		t.Fatalf("createRun: %v", aerr)
	}
	declared := changecontract.Contract{
		RunID: parent.RunID, StepID: "coder", FeatureKey: "feat", Intent: "coder work",
		DeclaredPaths: []string{"main.go"}, Confidence: changecontract.ConfidenceDeclared,
		DeclaredAt: time.Now().UTC(),
	}
	if err := store.Save(declared); err != nil {
		t.Fatal(err)
	}

	reviewer, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if aerr != nil {
		t.Fatalf("reviewer: %v", aerr)
	}
	nodes := reviewLoopTestNodes()
	settings := filepath.Join(dir, ".flowpilot", "settings")
	if err := os.MkdirAll(settings, 0o755); err != nil {
		t.Fatal(err)
	}
	// Enforce would reprompt undeclared coding children — reviewers must not hit that path.
	if err := os.WriteFile(filepath.Join(settings, "flow-rules.json"), []byte(`{"mode":"enforce","rules":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Snapshot dirt at reviewer turn start so pre-existing coder dirt is not
	// attributed to this turn (BUG-288 #7 / worktree fingerprint).
	startWT := snapshotWorktreeFingerprints(dir)
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[reviewer.RunID].parentRunID = parent.RunID
	svc.runs[reviewer.RunID].label = "reviewer_correctness"
	svc.runs[reviewer.RunID].agentName = "reviewer"
	svc.runs[reviewer.RunID].role = "reviewer"
	svc.runs[reviewer.RunID].workspaceCwd = dir
	svc.runs[reviewer.RunID].turnStartGitHead = ""
	svc.runs[reviewer.RunID].turnStartWorktree = startWT
	rs := svc.runs[reviewer.RunID]
	svc.mu.Unlock()

	blocked := svc.runChildArtifactOutputGate(context.Background(), rs, "turn-rev", finalizeInput{
		FinalMessage: "lgtm",
		// No WrittenPaths — reviewer did not edit code.
	})
	if blocked {
		t.Fatal("reviewer lgtm on dirty worktree must not block/reprompt")
	}
	got, ok := store.GetLatestForRun(parent.RunID)
	if !ok {
		t.Fatal("coder contract missing after reviewer turn")
	}
	if got.Confidence != changecontract.ConfidenceDeclared {
		t.Fatalf("reviewer must not overwrite declared contract; conf=%q", got.Confidence)
	}
	if got.StepID != "coder" || got.Intent != "coder work" {
		t.Fatalf("contract overwritten: step=%q intent=%q", got.StepID, got.Intent)
	}
}

// Root gate reprompt must re-append contract captured under the same run id.
func TestRootGateRepromptIncludesChangeContract(t *testing.T) {
	dir := t.TempDir()
	store, storeErr := changecontract.NewStore(dir)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	svc, _ := newTestServer(t)
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if aerr != nil {
		t.Fatalf("createRun: %v", aerr)
	}
	if err := store.Save(changecontract.Contract{
		RunID: parent.RunID, StepID: "coding", FeatureKey: "f", Intent: "scope",
		DeclaredPaths: []string{"apps/x.go"}, Confidence: changecontract.ConfidenceDeclared,
		DeclaredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	// Mirror the root reprompt inject path without spinning a provider turn.
	prompt := appendChangeContractIfAny(dir, parent.RunID, "Please add a change-audit note.")
	if !strings.Contains(prompt, "apps/x.go") || !strings.Contains(prompt, changeContractPromptMarker) {
		t.Fatalf("root reprompt path must re-append contract: %q", prompt)
	}
}

func TestIsFlowCodeWritingChild(t *testing.T) {
	coderNode := agentpack.FlowNode{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"}
	reviewNode := agentpack.FlowNode{ID: "reviewer_correctness", Behavior: "agent.delegate", Agent: "agents/reviewer.md"}
	coder := &interactiveRun{label: "coder", agentName: "coder", role: "coder"}
	reviewer := &interactiveRun{label: "reviewer_correctness", agentName: "reviewer", role: "reviewer"}
	// Code-writing node whose name contains "review" must NOT be classified as reviewer.
	reviewAndFix := &interactiveRun{label: "review_and_fix", agentName: "review_and_fix", role: "coder"}
	reviewAndFixNode := agentpack.FlowNode{ID: "review_and_fix", Behavior: "agent.delegate", Agent: "agents/coder.md"}
	codeDiff := []flowgate.ChangedFile{{Path: "main.go", Status: "M"}}
	// WrittenPaths alone (no git delta) is allowed as fallback.
	if !isFlowCodeWritingChild(coder, coderNode, true, []string{"main.go"}, nil) {
		t.Fatal("coder with WrittenPaths should be code-writing")
	}
	// Task-242: non-empty git diff activates tier-1 even without EventFileChanged.
	if !isFlowCodeWritingChild(coder, coderNode, true, nil, codeDiff) {
		t.Fatal("coder with git code-diff should be code-writing without WrittenPaths")
	}
	if isFlowCodeWritingChild(coder, coderNode, true, nil, nil) {
		t.Fatal("coder with empty diff and no writes must not gate")
	}
	// Reviewer with code diff must still gate (BUG-288 #14).
	if !isFlowCodeWritingChild(reviewer, reviewNode, true, nil, codeDiff) {
		t.Fatal("reviewer who mutates code this turn must be gated")
	}
	// Reviewer with empty turn-diff stays zero-cost.
	if isFlowCodeWritingChild(reviewer, reviewNode, true, nil, nil) {
		t.Fatal("reviewer with no code delta must be zero-cost")
	}
	if !isFlowCodeWritingChild(reviewAndFix, reviewAndFixNode, true, nil, codeDiff) {
		t.Fatal("review_and_fix with role=coder must remain code-writing (no substring trap)")
	}
}

func TestObserveTurnScopedDiffIgnoresPreexistingDirt(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "main.go")
	run("commit", "-m", "init")
	// Coder dirt before next turn starts.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n// coder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	start := snapshotWorktreeFingerprints(dir)
	// Unchanged dirt during "delegate" turn → empty turn-scoped diff.
	got, err := observeTurnScopedDiff(dir, "", start)
	if err != nil {
		t.Fatalf("observeTurnScopedDiff: %v", err)
	}
	if flowgate.HasCodeChanges(got) {
		t.Fatalf("pre-existing unchanged dirt must not appear as turn diff: %+v", got)
	}
	// New edit this turn → appears.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n// coder\n// delegate edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got2, err := observeTurnScopedDiff(dir, "", start)
	if err != nil {
		t.Fatalf("observeTurnScopedDiff: %v", err)
	}
	if !flowgate.HasCodeChanges(got2) {
		t.Fatal("content change after snapshot must be turn-scoped")
	}
}

func TestIsFlowReviewerChildExactRoleOnly(t *testing.T) {
	reviewNode := agentpack.FlowNode{ID: "reviewer_correctness", Agent: "agents/reviewer.md"}
	if !isFlowReviewerChild(&interactiveRun{role: "reviewer", agentName: "reviewer"}, reviewNode, true) {
		t.Fatal("role=reviewer must match")
	}
	// Code-writing node whose id/name contains "review" but role/agent are not the pack reviewer.
	fixNode := agentpack.FlowNode{ID: "review_and_fix", Agent: "agents/coder.md"}
	if isFlowReviewerChild(&interactiveRun{role: "coder", agentName: "review_and_fix", label: "review_and_fix"}, fixNode, true) {
		t.Fatal("substring review in name must not classify as reviewer")
	}
}

// Diff-only coding turn (no EventFileChanged) still captures parent contract.
func TestChildGateActivatesOnGitDiffWithoutFileEvents(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "main.go")
	run("commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n// edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc, _ := newTestServer(t)
	parent, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if aerr != nil {
		t.Fatalf("parent: %v", aerr)
	}
	child, aerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if aerr != nil {
		t.Fatalf("child: %v", aerr)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[child.RunID].parentRunID = parent.RunID
	svc.runs[child.RunID].label = "coder"
	svc.runs[child.RunID].agentName = "coder"
	svc.runs[child.RunID].role = "coder"
	svc.runs[child.RunID].workspaceCwd = dir
	rs := svc.runs[child.RunID]
	svc.mu.Unlock()

	msg := "[Change Contract]\nfeature: demo\nintent: edit\nfiles: main.go\n"
	// Empty ChangedFiles — only git dirty tree proves the edit.
	_ = svc.runChildArtifactOutputGate(context.Background(), rs, "t1", finalizeInput{FinalMessage: msg})
	store, err := changecontract.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := store.GetLatestForRun(parent.RunID)
	if !ok || c.Confidence != changecontract.ConfidenceDeclared {
		t.Fatalf("expected parent contract from git-diff activation, ok=%v conf=%q", ok, c.Confidence)
	}
}

// Task-242: EventTurnCompleted for flow-engine child defers settle until gate pass.
func TestFlowChildDefersSettleUntilGate(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("child: %v", err)
	}
	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[child.RunID].parentRunID = parent.RunID
	svc.runs[child.RunID].label = "coder"
	svc.runs[child.RunID].flowCohortId = "cohort-1"
	rs := svc.runs[child.RunID]
	svc.mu.Unlock()
	svc.agentOrchestrator.registerCohortMember(parent.RunID, "cohort-1")
	svc.mu.Lock()
	svc.emitLocked(rs, ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done work"})
	if !rs.pendingFlowGateSettle {
		t.Fatal("expected pendingFlowGateSettle after EventTurnCompleted on flow child")
	}
	if rs.pendingFlowGateFinalMsg != "done work" {
		t.Fatalf("final msg = %q", rs.pendingFlowGateFinalMsg)
	}
	// Cohort must not have received the result yet (deferred until gate pass).
	if svc.agentOrchestrator.cohortComplete(parent.RunID, "cohort-1") {
		t.Fatal("cohort must not complete before gate settle")
	}
	// After gate pass, settle joins the cohort (still under lock, as runTurn does).
	msg := rs.pendingFlowGateFinalMsg
	at := rs.pendingFlowGateOccurredAt
	rs.pendingFlowGateSettle = false
	rs.pendingFlowGateFinalMsg = ""
	rs.pendingFlowGateOccurredAt = ""
	svc.settleFlowChildTurnCompletedLocked(rs, msg, ProviderEvent{FinalMessage: msg, OccurredAt: at})
	// drainCohort clears the buffer on join — assert join via retained note.
	parentRS := svc.runs[parent.RunID]
	if parentRS == nil || strings.TrimSpace(parentRS.lastCohortNote) == "" {
		svc.mu.Unlock()
		t.Fatal("expected cohort join (lastCohortNote) after deferred settle")
	}
	svc.mu.Unlock()
}

// Task-242 D-2: empty-diff child gate is zero-cost (no panic / no block).
func TestChildGateEmptyDiffNoOp(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("child: %v", err)
	}
	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[child.RunID].parentRunID = parent.RunID
	svc.runs[child.RunID].label = "reviewer_correctness"
	svc.runs[child.RunID].workspaceCwd = t.TempDir() // no git repo → empty diff
	rs := svc.runs[child.RunID]
	svc.mu.Unlock()

	blocked := svc.runChildArtifactOutputGate(nil, rs, "turn-1", finalizeInput{FinalMessage: "lgtm"})
	if blocked {
		t.Fatal("empty-diff reviewer must not block")
	}
}
