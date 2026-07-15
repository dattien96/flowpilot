package runner

import (
	"testing"

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
