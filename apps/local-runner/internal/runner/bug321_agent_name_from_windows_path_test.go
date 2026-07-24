package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-321: agentNameFromRef (and its wrapper flowNodeAgentName) used the
// POSIX-only `path` package, so a flow node whose `agent:` ref is a Windows
// absolute path (backslash separators, e.g. a provider CLI's agent file at
// C:\Users\me\.codex\agents\coder-agent.toml) was NOT reduced to its
// basename: path.Base finds no "/" and returns the whole string, so the
// derived catalog name became the full path minus only the extension.
//
// That broke every consumer of the derived name for such nodes — most
// visibly matchFlowNodeForSession's AgentName/Role fallback (resume timeline)
// and isFlowReviewerChild (gate tier) — whenever the primary Label match was
// unavailable. These tests are RED before the fix and GREEN after.

// TestAgentNameFromRefStripsWindowsAndMixedSeparators is the leaf unit proof:
// the reported repro plus mixed/UNC/relative-backslash shapes across all three
// provider agent directories (.codex/.claude/.grok — the function is
// provider-agnostic string handling, so one table covers R2 parity), and the
// pre-existing POSIX shapes that must stay byte-identical (no regression to
// TestFlowNodeAgentNameDerivesFromFilePath).
func TestAgentNameFromRefStripsWindowsAndMixedSeparators(t *testing.T) {
	cases := []struct {
		name string
		ref  string
		want string
	}{
		// Reported repro (Codex agent file, Windows absolute, backslashes).
		{"codex-windows-abs", `C:\Users\dat.nguyen\.codex\agents\coder-agent.toml`, "coder-agent"},
		// Cross-provider parity — same shape for Claude and Grok agent dirs.
		{"claude-windows-abs", `C:\Users\me\.claude\agents\reviewer-agent.md`, "reviewer-agent"},
		{"grok-windows-abs", `C:\Users\me\.grok\agents\synthesizer.toml`, "synthesizer"},
		// A bare "reviewer" basename from a Windows path (drives isFlowReviewerChild below).
		{"claude-windows-reviewer", `C:\Users\me\.claude\agents\reviewer.md`, "reviewer"},
		// Mixed separators (a config that concatenated a Windows base with a "/" tail).
		{"mixed-separators", `C:\Users\me/.claude\agents/reviewer.md`, "reviewer"},
		// UNC path.
		{"unc-path", `\\server\share\agents\coder.md`, "coder"},
		// Relative backslash ref.
		{"relative-backslash", `agents\coder.md`, "coder"},
		// Backslash with no extension.
		{"windows-no-ext", `C:\Users\me\.codex\agents\coder-agent`, "coder-agent"},
		// Pre-existing POSIX shapes — must remain byte-identical (regression guard).
		{"posix-relative", "agents/coder.md", "coder"},
		{"posix-abs", "/home/me/.claude/agents/reviewer.md", "reviewer"},
		{"bare-name", "coder.md", "coder"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := agentNameFromRef(tc.ref); got != tc.want {
				t.Errorf("agentNameFromRef(%q) = %q, want %q", tc.ref, got, tc.want)
			}
		})
	}
}

// TestMatchFlowNodeForSessionResolvesWindowsPathAgentByFallback reproduces the
// live symptom directly (the HIGH-risk resume-match process): a child session
// with NO Label (e.g. persisted before Label preservation, or any legacy row)
// falls back to matching by AgentName against flowNodeAgentName(node). When the
// node's agent ref is a Windows absolute path, that derivation was wrong, so no
// node matched and the step timeline lost the child's real status. Covered for
// all three providers' agent-dir path shapes.
func TestMatchFlowNodeForSessionResolvesWindowsPathAgentByFallback(t *testing.T) {
	nodes := []agentpack.FlowNode{
		{ID: "my-coder", Behavior: "agent.delegate", Agent: `C:\Users\dat.nguyen\.codex\agents\coder-agent.toml`},
		{ID: "my-reviewer-claude", Behavior: "agent.delegate", Agent: `C:\Users\dat.nguyen\.claude\agents\reviewer.md`, DependsOn: []string{"my-coder"}},
	}
	cases := []struct {
		name      string
		agentName string
		role      string
		wantNode  string
	}{
		{"codex-coder-by-agentname", "coder-agent", "", "my-coder"},
		{"claude-reviewer-by-agentname", "reviewer", "", "my-reviewer-claude"},
		{"reviewer-by-role", "", "reviewer", "my-reviewer-claude"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Empty Label forces the AgentName/Role fallback — the broken path.
			session := ProviderSessionState{AgentName: tc.agentName, Role: tc.role}
			if got := matchFlowNodeForSession(nodes, session); got != tc.wantNode {
				t.Errorf("matchFlowNodeForSession(agentName=%q, role=%q) = %q, want %q",
					tc.agentName, tc.role, got, tc.wantNode)
			}
		})
	}
}

// TestIsFlowReviewerChildDetectsWindowsPathReviewerNode covers the second
// HIGH-risk downstream process (runFlowGateAtEpoch → isFlowReviewerChild):
// a reviewer node whose agent ref is a Windows absolute path must still be
// recognized as a reviewer for gate-tier purposes even when the run's own
// role/agentName are unset (so detection relies solely on the node's derived
// agent name). Before the fix, flowNodeAgentName returned the full path and the
// node was mis-classified as a non-reviewer (wrong gate tier).
func TestIsFlowReviewerChildDetectsWindowsPathReviewerNode(t *testing.T) {
	reviewerNode := agentpack.FlowNode{
		ID:       "my-reviewer-claude",
		Behavior: "agent.delegate",
		Agent:    `C:\Users\dat.nguyen\.claude\agents\reviewer.md`,
	}
	coderNode := agentpack.FlowNode{
		ID:       "my-coder",
		Behavior: "agent.delegate",
		Agent:    `C:\Users\dat.nguyen\.codex\agents\coder-agent.toml`,
	}
	// role/agentName intentionally empty so detection falls through to the
	// node-derived agent name — the exact branch the bug broke.
	rs := &interactiveRun{}

	if !isFlowReviewerChild(rs, reviewerNode, true) {
		t.Errorf("isFlowReviewerChild(windows-path reviewer node) = false, want true")
	}
	if isFlowReviewerChild(rs, coderNode, true) {
		t.Errorf("isFlowReviewerChild(windows-path coder node) = true, want false")
	}
}
