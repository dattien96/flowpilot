package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-528 — [open] chip must appear on the first flow open, not after reopening.
// The hydrate (GET …/agents) can land late/empty/main-only while a faster
// agent_graph_updated already mapped children; a plain last-writer-wins replace
// hid the step [open] chips until the next poll or reopen.

func TestAdoptAgentRuns_MainOnlyListKeepsChildren(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.mode = ModeFlow
			m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
			m.flowSteps = []client.WorkflowStepRuntime{
				{StepID: "s1", NodeID: "my-reviewer", AgentRef: "my-reviewer", Status: "RUNNING"},
			}
			// agent_graph_updated already mapped a child run.
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
				{RunID: "run-rev", AgentName: "my-reviewer", Status: "running"},
			}
			// A slower list hydrate returns only the synthetic main row.
			next, _ := m.Update(agentRunsHydratedMsg{
				ParentRunID: "run-main",
				Runs:        []client.AgentRunSummary{{RunID: "run-main", AgentName: "main", Role: "main", Status: "completed"}},
			})
			am := next.(*AppModel)
			if !am.hasChildAgentRuns() {
				t.Fatalf("%s: main-only hydrate must not erase known children", pk)
			}
			if !strings.Contains(strings.Join(am.flowStepsPanelLines(), "\n"), "[open]") {
				t.Fatalf("%s: [open] must survive a main-only hydrate", pk)
			}
		})
	}
}

func TestHydrateRetry_RearmsWhenStepsMissingChip(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "coder", AgentRef: "coder", Status: "RUNNING"},
	}
	next, cmd := m.Update(agentRunsHydratedMsg{
		ParentRunID: "run-main",
		Runs:        []client.AgentRunSummary{{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"}},
	})
	am := next.(*AppModel)
	if cmd == nil {
		t.Fatal("step missing [open] chip must re-arm a bounded retry")
	}
	if am.agentHydrateRetries != 1 {
		t.Fatalf("want retries=1, got %d", am.agentHydrateRetries)
	}
	// The retry tick fires a fresh hydrate.
	next2, cmd2 := am.Update(hydrateAgentRunsIfNeededMsg{})
	am2 := next2.(*AppModel)
	if cmd2 == nil {
		t.Fatal("retry tick must fire a fresh hydrate")
	}
	if am2.agentHydrateRetries != 1 {
		t.Fatalf("tick must not double-count the retry (want 1, got %d)", am2.agentHydrateRetries)
	}
}

func TestHydrateRetry_CapsAtLimit(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "coder", AgentRef: "coder", Status: "RUNNING"},
	}
	m.agentHydrateRetries = agentHydrateRetryLimit
	if cmd := m.cmdHydrateAgentRunsIfNeeded(); cmd != nil {
		t.Fatal("retry ladder must cap at agentHydrateRetryLimit")
	}
}

func TestHydrateRetry_ResetsOnChildSuccess(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "coder", AgentRef: "coder", Status: "RUNNING"},
	}
	m.agentHydrateRetries = 2
	next, cmd := m.Update(agentRunsHydratedMsg{
		ParentRunID: "run-main",
		Runs: []client.AgentRunSummary{
			{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
			{RunID: "run-c", AgentName: "coder", Status: "running"},
		},
	})
	am := next.(*AppModel)
	if am.agentHydrateRetries != 0 {
		t.Fatalf("child success must reset retries (got %d)", am.agentHydrateRetries)
	}
	if cmd != nil {
		t.Fatal("chip mapped — no further retry needed")
	}
	if !strings.Contains(strings.Join(am.flowStepsPanelLines(), "\n"), "[open]") {
		t.Fatal("[open] must appear after child hydrate")
	}
}

func TestHydrateRetry_UnreachableErrorDoesNotRetry(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "coder", AgentRef: "coder", Status: "RUNNING"},
	}
	_, cmd := m.Update(agentRunsHydratedMsg{
		ParentRunID: "run-main",
		Err:         "dial tcp 127.0.0.1:4317: connection refused",
	})
	if cmd != nil {
		t.Fatal("dead runner must not re-arm retry (CA-514)")
	}
	if m.agentHydrateRetries != 0 {
		t.Fatal("unreachable error must not consume retry budget")
	}
}

func TestHydrateRetry_SoftErrorRearms(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "coder", AgentRef: "coder", Status: "RUNNING"},
	}
	_, cmd := m.Update(agentRunsHydratedMsg{ParentRunID: "run-main", Err: "internal error"})
	if cmd == nil {
		t.Fatal("soft hydrate error should re-arm the bounded retry")
	}
}

func TestTruncateStepLine_KeepsOpenChip(t *testing.T) {
	base := styleStepRunning.Render("[•] this-is-a-very-long-step-name-for-a-reviewer WAITING_USER_APPROVAL")
	out := truncateStepLine(base+"  "+styleStepAgentAction.Render("[open]"), 30)
	if !strings.Contains(out, "[open]") {
		t.Fatalf("chip must survive truncation: %q", out)
	}
	if w := lipgloss.Width(out); w > 30 {
		t.Fatalf("truncated width %d > 30: %q", w, out)
	}

	out2 := truncateStepLine(styleStatusHi.Render("[•] another-long-step-name-for-review RUNNING")+"  "+styleStepAgentAction.Render("[back]"), 26)
	if !strings.Contains(out2, "[back]") {
		t.Fatalf("back chip must survive truncation: %q", out2)
	}
	if w := lipgloss.Width(out2); w > 26 {
		t.Fatalf("truncated width %d > 26: %q", w, out2)
	}
}

func TestStepRowMatchesName_TruncatedRow(t *testing.T) {
	if !stepRowMatchesName("[•] reviewer-for-fix-super…", "reviewer-for-fix-super-long-name") {
		t.Fatal("meaningful prefix + ellipsis must match")
	}
	if !stepRowMatchesName("[•] my-reviewer RUNNING  [open]", "my-reviewer") {
		t.Fatal("full name must match")
	}
	if stepRowMatchesName("[•] coder RUNNING", "reviewer-for-fix-super-long-name") {
		t.Fatal("unrelated row must not match")
	}
}

func TestOpenRunIDFromPanelLine_TruncatedRowStillMaps(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-main"}
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "reviewer-for-fix-super-long-name", AgentRef: "reviewer-for-fix-super-long-name", Status: "RUNNING"},
	}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main"},
		{RunID: "run-r", AgentName: "reviewer-for-fix-super-long-name", Status: "running"},
	}
	if got := m.openRunIDFromPanelLine("[•] reviewer-for-fix-super…  [open]"); got != "run-r" {
		t.Fatalf("truncated row must map to run-r, got %q", got)
	}
}

func TestRightSidebar_LongStepKeepsOpenChip(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width, m.height = 120, 30
			m.asciiMode = true
			m.mode = ModeFlow
			m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
			enableSidebarForTest(m)
			m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
				{RunID: "run-r", AgentName: "reviewer-for-fix-super-long-name", Status: "running"},
			}
			m.flowSteps = []client.WorkflowStepRuntime{
				{StepID: "s1", NodeID: "reviewer-for-fix-super-long-name", AgentRef: "reviewer-for-fix-super-long-name", Status: "RUNNING"},
			}
			if !m.useRightSidebar() {
				t.Fatal("wide expanded must engage sidebar")
			}
			lines := m.renderRightSidebar(m.height)
			joined := strings.Join(lines, "\n")
			if !strings.Contains(joined, "[open]") {
				t.Fatalf("%s: long step must keep [open]:\n%s", pk, joined)
			}
			sideW := m.sideWidth()
			for _, l := range lines {
				if pl := stripANSI(l); strings.TrimSpace(pl) != "" && lipgloss.Width(pl) > sideW {
					t.Fatalf("%s: sidebar line wider than %d: %q", pk, sideW, pl)
				}
			}
			if _, _, ok := findClickTarget(m, "agent-open:run-r"); !ok {
				t.Fatalf("%s: truncated [open] must stay hittable:\n%s", pk, joined)
			}
		})
	}
}

func TestOverlay_LongStepKeepsOpenChip(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	enableSidebarForTest(m)
	m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
		{RunID: "run-r", AgentName: "reviewer-for-fix-super-long-name", Status: "running"},
	}
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "reviewer-for-fix-super-long-name", AgentRef: "reviewer-for-fix-super-long-name", Status: "RUNNING"},
	}
	if m.useRightSidebar() {
		t.Fatal("narrow must not show the sidebar")
	}
	// Task-311: no overlay — the [open] chip is hittable only in the wide
	// sidebar. At narrow width the step rows carry no hit-test surface.
	if _, _, ok := findClickTarget(m, "agent-open:run-r"); ok {
		t.Fatal("narrow terminal must not expose an overlay [open] (Task-311)")
	}
}
