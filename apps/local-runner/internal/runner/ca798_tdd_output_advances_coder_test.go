package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func ca798WriteSigMD(t *testing.T, cwd string) {
	t.Helper()
	p := filepath.Join(cwd, filepath.FromSlash(vibeTddSignaturesRel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("# tdd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func ca798Git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestCA798_SignatureOnlyTestsCountAsTddOutput(t *testing.T) {
	cwd := t.TempDir()
	if hasVibeTddOutput(cwd) {
		t.Fatal("empty cwd must not count")
	}
	ca798Git(t, cwd, "init", "-q")
	ca798Git(t, cwd, "config", "user.email", "t@t")
	ca798Git(t, cwd, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(cwd, "foo_test.go"), []byte("package p\nimport \"testing\"\nfunc TestFoo(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ca798Git(t, cwd, "add", "foo_test.go")
	ca798Git(t, cwd, "commit", "-qm", "t")
	if hasVibeTddOutput(cwd) {
		t.Fatal("tracked empty tests must not unlock coder")
	}
	dir := filepath.Join(cwd, "snake")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "package snake\nimport \"testing\"\nfunc TestNewGame_GridDimensions(t *testing.T) {}\n"
	if err := os.WriteFile(filepath.Join(dir, "snake_test.go"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if !hasVibeTddOutput(cwd) {
		t.Fatal("untracked empty TestX frames must count as tdd output")
	}
	if vibeCoderSpawnBlocked(workingmode.Vibe, "coder", hasVibeTddOutput(cwd)) {
		t.Fatal("coder must spawn when git-new signature tests exist")
	}
}

func TestCA798_CommentTFatalStillSignatureOnly(t *testing.T) {
	cwd := t.TempDir()
	ca798Git(t, cwd, "init", "-q")
	body := "package snake\nimport \"testing\"\n// do not add t.Fatal checks yet\nfunc TestA(t *testing.T) {}\n"
	if err := os.WriteFile(filepath.Join(cwd, "snake_test.go"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if !hasVibeTddOutput(cwd) {
		t.Fatal("t.Fatal in comments must not block signature-only")
	}
}

func TestCA798_FullBodyTestsDoNotCountAsTddOutput(t *testing.T) {
	cwd := t.TempDir()
	ca798Git(t, cwd, "init", "-q")
	if err := os.WriteFile(filepath.Join(cwd, "calc_test.go"), []byte("package p\nimport \"testing\"\nfunc TestSum(t *testing.T) { t.Fatal(\"x\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if hasVibeTddOutput(cwd) {
		t.Fatal("full-body tests must not unlock coder")
	}
	if !vibeCoderSpawnBlocked(workingmode.Vibe, "coder", hasVibeTddOutput(cwd)) {
		t.Fatal("coder must stay blocked")
	}
}

func TestCA798_ResumeAfterTddAdvancesCoder(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	cwd := t.TempDir()
	ca798WriteSigMD(t, cwd)
	edges := []agentpack.FlowEdge{
		{From: "tdd", To: "coder", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.code", Agent: "agents/tester.md"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.flowEngineDriven = true
	rs.workspaceCwd = cwd
	rs.vibeCheckpointNode = "tdd"
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	svc.mu.Unlock()
	svc.maybeResumeVibeCoderAfterTdd(parent.RunID)
	if countChildrenWithLabel(svc, parent.RunID, "ingest_reader") != 0 {
		t.Fatal("resume must not restart vibe-ingest")
	}
	if countChildrenWithLabel(svc, parent.RunID, "coder") != 0 {
		t.Fatal("coder must not spawn without frozen contract")
	}
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.Status != "blocked" || loop.BlockReason != "requirement" {
		t.Fatalf("want requirement park, got status=%q reason=%q gate=%q", loop.Status, loop.BlockReason, loop.GateReason)
	}
}

func TestCA798_ResumeWithoutTddDoneDoesNotAdvance(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	cwd := t.TempDir()
	ca798WriteSigMD(t, cwd)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.flowEngineDriven = true
	rs.workspaceCwd = cwd
	rs.vibeCheckpointNode = ""
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: "tdd", To: "coder", When: "done", Kind: "forward"}}
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.code", Agent: "agents/tester.md"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	svc.mu.Unlock()
	svc.maybeResumeVibeCoderAfterTdd(parent.RunID)
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.Status != "running" {
		t.Fatalf("stale tests must not resume coder, got %+v", loop)
	}
	if countChildrenWithLabel(svc, parent.RunID, "coder") != 0 {
		t.Fatal("coder must not spawn before tdd done")
	}
}

func TestCA798_ResumeWithoutSprintGraphParks(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	cwd := t.TempDir()
	ca798WriteSigMD(t, cwd)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.workspaceCwd = cwd
	rs.vibeCheckpointNode = "tdd"
	svc.mu.Unlock()
	svc.maybeResumeVibeCoderAfterTdd(parent.RunID)
	if countChildrenWithLabel(svc, parent.RunID, "ingest_reader") != 0 {
		t.Fatal("missing graph must not start ingest")
	}
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.Status != "blocked" || loop.BlockReason != "requirement" {
		t.Fatalf("want requirement park, got status=%q reason=%q", loop.Status, loop.BlockReason)
	}
}

func TestCA798_EmptySignaturesFileDoesNotCount(t *testing.T) {
	cwd := t.TempDir()
	p := filepath.Join(cwd, filepath.FromSlash(vibeTddSignaturesRel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if hasVibeTddSignatures(cwd) || hasVibeTddOutput(cwd) {
		t.Fatal("empty tdd-signatures.md must not unlock coder")
	}
}

func TestCA798_ResumeParksWhenTddDoneWithoutOutput(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.flowEngineDriven = true
	rs.workspaceCwd = t.TempDir()
	rs.vibeCheckpointNode = "tdd"
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: "tdd", To: "coder", When: "done", Kind: "forward"}}
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.code", Agent: "agents/tester.md"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	svc.mu.Unlock()
	svc.maybeResumeVibeCoderAfterTdd(parent.RunID)
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.Status != "blocked" || loop.BlockReason != "requirement" {
		t.Fatalf("want requirement park, got status=%q reason=%q", loop.Status, loop.BlockReason)
	}
	if !strings.Contains(loop.GateReason, "tdd artifact missing") {
		t.Fatalf("gate=%q want tdd artifact missing", loop.GateReason)
	}
}

func TestCA798_ResumeDoesNotDuplicatePersistedCoder(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	cwd := t.TempDir()
	ca798WriteSigMD(t, cwd)
	store, ok := svc.workflowStore.(InteractiveStateStore)
	if !ok {
		t.Fatal("workflowStore is not InteractiveStateStore")
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "orphan-coder", ParentRunID: parent.RunID, Label: "coder",
		Status: RunStatusRunning, ProviderKey: ProviderKeyCodex,
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.flowEngineDriven = true
	rs.workspaceCwd = cwd
	rs.vibeCheckpointNode = "tdd"
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: "tdd", To: "coder", When: "done", Kind: "forward"}}
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.code", Agent: "agents/tester.md"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	svc.mu.Unlock()
	svc.maybeResumeVibeCoderAfterTdd(parent.RunID)
	if countChildrenWithLabel(svc, parent.RunID, "coder") != 0 {
		t.Fatal("must not spawn a second coder over a persisted running coder")
	}
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.Status != "running" {
		t.Fatalf("must not park over existing coder, got %+v", loop)
	}
}
