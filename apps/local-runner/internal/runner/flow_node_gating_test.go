package runner

import (
	"testing"
)

// Task-349 (CP-62 P-4 completion): a gated flow-node posture must force the
// same gated provider modes as the read-only chat postures — bypass modes
// never emit permission requests, which left read_only/verdict_only nodes
// structurally unenforced. Provider-agnostic: all three adapters resolve
// their modes through this one SSOT, so the table below IS the parity test.

func TestFlowNodeGating_PostureForcesGatedProviderModes(t *testing.T) {
	yolo := true // flow runs are product-locked YOLO=true (BUG-299)
	for _, posture := range []string{PostureReadOnly, PostureVerdictOnly} {
		p := resolveYoloPostureForFlowNode(yolo, false, "", posture)
		if p.ClaudePermissionMode != "default" {
			t.Fatalf("%s: ClaudePermissionMode = %q, want default", posture, p.ClaudePermissionMode)
		}
		if p.CodexApprovalMode != "untrusted" || p.CodexSandbox != "workspace-write" {
			t.Fatalf("%s: codex = %q/%q, want untrusted/workspace-write", posture, p.CodexApprovalMode, p.CodexSandbox)
		}
		if p.GrokPermissionMode != "" {
			t.Fatalf("%s: GrokPermissionMode = %q, want empty (gated)", posture, p.GrokPermissionMode)
		}
		if p.RunnerAutoApprove {
			t.Fatalf("%s: RunnerAutoApprove must be false so the posture matrix decides", posture)
		}
	}
}

func TestFlowNodeGating_StandardPostureKeepsFlowYOLO(t *testing.T) {
	p := resolveYoloPostureForFlowNode(true, false, "", "")
	if !p.RunnerAutoApprove || p.ClaudePermissionMode != "bypassPermissions" {
		t.Fatalf("standard flow node must keep YOLO bypass: %+v", p)
	}
	p = resolveYoloPostureForFlowNode(true, false, "", PostureStandard)
	if !p.RunnerAutoApprove || p.ClaudePermissionMode != "bypassPermissions" {
		t.Fatalf("standard posture must keep YOLO bypass: %+v", p)
	}
	if IsGatedFlowNodePosture(PostureStandard) || IsGatedFlowNodePosture("") || IsGatedFlowNodePosture("evil") {
		t.Fatalf("only read_only/verdict_only are gated")
	}
}

// Scenario: verdict face với tên MCP bị wrap vẫn được nhận diện (P1 fix)
func TestFlowNodeGating_VerdictToolCallWrappedNames(t *testing.T) {
	for _, name := range []string{
		"submit_review_outcome",
		"mcp__flowpilot__submit_review_outcome",
		"flowpilot__submit_review_outcome",
	} {
		if !isVerdictToolCall(ApprovalDetails{Kind: "other", Reason: name}) {
			t.Fatalf("verdict face %q must be recognized via Reason", name)
		}
		if !isVerdictToolCall(ApprovalDetails{Kind: "other", Command: name}) {
			t.Fatalf("verdict face %q must be recognized via Command", name)
		}
	}
	if isVerdictToolCall(ApprovalDetails{Kind: "other", Reason: "mcp__flowpilot__other_tool"}) {
		t.Fatalf("non-verdict MCP tool must not match")
	}
}

// Scenario: read_only reviewer phải nộp được verdict tool (mcp/other face)
func TestFlowNodeGating_ReadOnlyAllowsVerdictFace(t *testing.T) {
	if got := evaluateFlowNodeApproval(PostureReadOnly, ApprovalDetails{Kind: "other", Reason: "mcp__flowpilot__submit_review_outcome"}); got != "approve" {
		t.Fatalf("read_only must approve the wrapped verdict face, got %q", got)
	}
}

// Scenario: verdict_only owner — tool verdict được phép kể cả tên wrap, mọi exec vẫn chặn
func TestFlowNodeGating_VerdictOnlyMatrixWithWrappedNames(t *testing.T) {
	if got := evaluateFlowNodeApproval(PostureVerdictOnly, ApprovalDetails{Kind: "other", Reason: "mcp__flowpilot__submit_review_outcome"}); got != "approve" {
		t.Fatalf("verdict_only must approve the wrapped verdict face, got %q", got)
	}
	if got := evaluateFlowNodeApproval(PostureVerdictOnly, ApprovalDetails{Kind: "exec", Command: "git diff"}); got != "deny" {
		t.Fatalf("verdict_only must deny exec even read-only, got %q", got)
	}
}
