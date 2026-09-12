package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// Task-340 (CP-62 P-4): per-node posture enforcement matrix. The enforcement
// lives at the provider-neutral approval bridge (turnBridge.RequestApproval),
// so parity across Claude/Codex/Grok is structural — the tests below pin the
// decision function and the run→node posture resolution.

func fileWriteDetails() ApprovalDetails {
	return ApprovalDetails{Kind: "file", Command: "stringutil/string.go", Reason: "Write"}
}

func execDetails(cmd string) ApprovalDetails {
	return ApprovalDetails{Kind: "exec", Command: cmd, Reason: "Bash"}
}

func mcpDetails(tool string) ApprovalDetails {
	return ApprovalDetails{Kind: "other", Command: tool, Reason: tool}
}

// Scenario: Reviewer gọi tool ghi file -> bị chặn silent-deny
func TestNodeIsolation_ReviewerWriteAttempt_SilentDenied(t *testing.T) {
	if got := evaluateFlowNodeApproval(PostureReadOnly, fileWriteDetails()); got != "deny" {
		t.Fatalf("read_only file write: got %q want deny", got)
	}
}

// Scenario: Reviewer gọi lệnh Bash đọc (git diff, git log, ls) -> được phép chạy.
// Lưu ý: `go vet`/`go build` là GHI theo phân loại BUG-344 (build cache) —
// chấp nhận bất biến đó thay vì nới classifier (oracle-rule).
func TestNodeIsolation_ReviewerBashReadOps_Allowed(t *testing.T) {
	for _, cmd := range []string{"git diff", "git log --oneline -5", "ls -la", "rg -n pattern ./..."} {
		if got := evaluateFlowNodeApproval(PostureReadOnly, execDetails(cmd)); got != "approve" {
			t.Fatalf("read_only exec %q: got %q want approve", cmd, got)
		}
	}
	// BUG-344 classification: go vet writes to the build cache — denied.
	if got := evaluateFlowNodeApproval(PostureReadOnly, execDetails("go vet ./...")); got != "deny" {
		t.Fatalf("read_only exec go vet: got %q want deny (BUG-344 classifies as write)", got)
	}
}

// Scenario: Reviewer gọi lệnh Bash ghi (rm, >, tee) -> bị chặn hoàn toàn
func TestNodeIsolation_ReviewerBashWriteOps_Denied(t *testing.T) {
	for _, cmd := range []string{"rm -rf /tmp/x", "echo abc > file.go", "tee out.txt", "git diff && rm x"} {
		if got := evaluateFlowNodeApproval(PostureReadOnly, execDetails(cmd)); got != "deny" {
			t.Fatalf("read_only exec %q: got %q want deny", cmd, got)
		}
	}
}

// Scenario: Node Owner trong vibe-owner-debate không có tool Bash (verdict_only)
func TestNodeIsolation_OwnerVerdictOnly_NoBash(t *testing.T) {
	if got := evaluateFlowNodeApproval(PostureVerdictOnly, execDetails("git diff")); got != "deny" {
		t.Fatalf("verdict_only exec: got %q want deny (Q-3: no Bash at all)", got)
	}
	if got := evaluateFlowNodeApproval(PostureVerdictOnly, fileWriteDetails()); got != "deny" {
		t.Fatalf("verdict_only file write: got %q want deny", got)
	}
	// The verdict face and read tools are the only approvals.
	if got := evaluateFlowNodeApproval(PostureVerdictOnly, mcpDetails("submit_review_outcome")); got != "approve" {
		t.Fatalf("verdict_only verdict tool: got %q want approve", got)
	}
	if got := evaluateFlowNodeApproval(PostureVerdictOnly, mcpDetails("Read")); got != "approve" {
		t.Fatalf("verdict_only read tool: got %q want approve", got)
	}
	if got := evaluateFlowNodeApproval(PostureVerdictOnly, mcpDetails("run_some_mutation_tool")); got != "deny" {
		t.Fatalf("verdict_only mutation mcp: got %q want deny", got)
	}
}

// Edge: standard/undeclared posture is never handled at the bridge (legacy
// path untouched); the matrix function returns approve for completeness.
func TestNodeIsolation_StandardPostureNotHandled(t *testing.T) {
	if got := evaluateFlowNodeApproval(PostureStandard, fileWriteDetails()); got != "approve" {
		t.Fatalf("standard posture matrix: got %q want approve (not consulted)", got)
	}
	svc, _ := newTestServer(t)
	child := &interactiveRun{id: "run-root", parentRunID: "", stepID: "reviewer"}
	if _, _, handled := svc.decideFlowNodePosture(child, fileWriteDetails()); handled {
		t.Fatalf("standard posture must not be handled at the bridge")
	}
}

// Scenario: posture resolution — child run đọc posture từ FlowNode của parent
func TestNodeIsolation_FlowNodePostureResolution(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: workingmode.Vibe, Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	parentRs := svc.runs[parent.RunID]
	parentRs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "reviewer", Behavior: "agent.delegate", Posture: PostureReadOnly},
		{ID: "owner_1", Behavior: "agent.delegate", Posture: PostureVerdictOnly},
		{ID: "implement", Behavior: "agent.code"},
	}
	svc.mu.Unlock()

	cases := []struct {
		name     string
		stepID   string
		posture  string
		decision string
		handled  bool
	}{
		{"reviewer-child-deny-write", "reviewer", PostureReadOnly, "deny", true},
		{"owner-child-no-bash", "owner_1", PostureVerdictOnly, "deny", true},
		{"coder-child-unhandled", "implement", PostureStandard, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			child := &interactiveRun{id: "run-child", parentRunID: parent.RunID, stepID: tc.stepID}
			decision, reason, handled := svc.decideFlowNodePosture(child, fileWriteDetails())
			if handled != tc.handled || decision != tc.decision {
				t.Fatalf("got decision=%q handled=%v reason=%q; want %q/%v", decision, handled, reason, tc.decision, tc.handled)
			}
		})
	}
	// No parent (hub/chat) is never handled.
	if _, _, handled := svc.decideFlowNodePosture(&interactiveRun{id: "run-root"}, fileWriteDetails()); handled {
		t.Fatalf("root run must not be posture-gated")
	}
}

// Scenario: Flow YAML pack parse giữ nguyên posture (data-level khai báo)
func TestNodeIsolation_PackFlowPosturesParsed(t *testing.T) {
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("load pack: %v", err)
	}
	var flow, debate *agentpack.FlowDefinition
	for i := range pack.Flows {
		switch pack.Flows[i].ID {
		case "task-harness":
			flow = &pack.Flows[i]
		case "vibe-owner-debate":
			debate = &pack.Flows[i]
		}
	}
	if flow == nil {
		t.Fatalf("task-harness flow missing")
	}
	want := map[string]string{
		"preflight_contract_plan": PostureReadOnly,
		"plan_reviewer":           PostureReadOnly,
		"reviewer":                PostureReadOnly,
		"implement":               PostureStandard,
	}
	for _, node := range flow.Nodes {
		expected, checked := want[node.ID]
		if !checked {
			continue
		}
		got := node.Posture
		if expected == PostureStandard {
			if got != "" && got != PostureStandard {
				t.Fatalf("node %q posture=%q want standard/empty", node.ID, got)
			}
			continue
		}
		if got != expected {
			t.Fatalf("node %q posture=%q want %q", node.ID, got, expected)
		}
	}
	if debate == nil {
		t.Fatalf("vibe-owner-debate flow missing")
	}
	owners := 0
	for _, node := range debate.Nodes {
		if node.ID == "owner_1" || node.ID == "owner_2" {
			owners++
			if node.Posture != PostureVerdictOnly {
				t.Fatalf("%s posture=%q want verdict_only", node.ID, node.Posture)
			}
		}
	}
	if owners != 2 {
		t.Fatalf("owner nodes = %d want 2", owners)
	}
}
