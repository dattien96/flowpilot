package runner

import (
	"testing"
)

// CA-1212 (live run-262417): the commit gate blamed whatever leg completed
// its turn for ANY new workspace commit — the rework leg's premature commit
// eb27e87 landed inside the spec-aligner's turn window and got the
// read-only leg blocked + parked waiting_user_approval. Parallel siblings
// share cwd, so workspace-level commit detection cannot attribute
// authorship; only the leg's own tool telemetry can.
func TestCA1212_CommitGateAttribution(t *testing.T) {
	commits := []string{"eb27e87 coder commit"}

	cases := []struct {
		name          string
		isCodingChild bool
		commits       []string
		fin           finalizeInput
		wantBlock     bool
	}{
		{
			name:          "sibling commit + clean telemetry = innocent",
			isCodingChild: true,
			commits:       commits,
			fin: finalizeInput{
				ToolCalls:    3,
				ExecCommands: []string{"ls -la", "cat requirements/08-Task/done/Task-041.md"},
			},
			wantBlock: false,
		},
		{
			name:          "own commit command = blocked",
			isCodingChild: true,
			commits:       commits,
			fin: finalizeInput{
				ToolCalls:    4,
				ExecCommands: []string{"git add -A", "git commit -m feat"},
			},
			wantBlock: true,
		},
		{
			name:          "no telemetry + commit exists = fail-closed block",
			isCodingChild: true,
			commits:       commits,
			fin:           finalizeInput{ToolCalls: 0},
			wantBlock:     true,
		},
		{
			name:          "commit-shaped input inside a Write tool file content is not a command",
			isCodingChild: true,
			commits:       commits,
			fin: finalizeInput{
				ToolCalls:    2,
				ExecCommands: nil, // execCommandFromToolInput never yielded a command
			},
			wantBlock: false,
		},
		{
			name:          "no new commits = no block regardless",
			isCodingChild: true,
			commits:       nil,
			fin:           finalizeInput{ToolCalls: 0},
			wantBlock:     false,
		},
		{
			name:          "non-coding child never blocked",
			isCodingChild: false,
			commits:       commits,
			fin:           finalizeInput{ToolCalls: 0},
			wantBlock:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := commitGateBlocksLeg(tc.isCodingChild, tc.commits, tc.fin); got != tc.wantBlock {
				t.Fatalf("commitGateBlocksLeg=%v want %v", got, tc.wantBlock)
			}
		})
	}
}

func TestCA1212_ExecCommandFromToolInput(t *testing.T) {
	cases := []struct {
		name  string
		input any
		want  string
	}{
		{"bare string (codex exec input)", "git commit -m x", "git commit -m x"},
		{"claude Bash map", map[string]any{"command": "git commit -m x"}, "git commit -m x"},
		{"devin rawInput map", map[string]any{"command": "git add -A"}, "git add -A"},
		{"write tool (file_path+content)", map[string]any{"file_path": "/a", "content": "git commit"}, ""},
		{"nil input", nil, ""},
		{"argv array", map[string]any{"argv": []any{"git", "commit"}}, "git commit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := execCommandFromToolInput(tc.input); got != tc.want {
				t.Fatalf("execCommandFromToolInput(%v)=%q want %q", tc.input, got, tc.want)
			}
		})
	}
}

// finalizeInputLocked must collect exec commands + tool-call count from the
// turn's own events so the gate sees per-leg telemetry.
func TestCA1212_FinalizeInputCollectsTurnTelemetry(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.events = append(rs.events,
		ProviderEvent{Type: EventToolStarted, ProviderTurnID: "turn-1", ToolName: "Bash", Input: map[string]any{"command": "git commit -m x"}},
		ProviderEvent{Type: EventToolStarted, ProviderTurnID: "turn-1", ToolName: "Write", Input: map[string]any{"file_path": "/a", "content": "x"}},
		ProviderEvent{Type: EventToolStarted, ProviderTurnID: "turn-2", ToolName: "Bash", Input: map[string]any{"command": "git push"}},
	)
	in := svc.finalizeInputLocked(rs, "turn-1")
	svc.mu.Unlock()

	if in.ToolCalls != 2 {
		t.Fatalf("ToolCalls=%d want 2 (turn-1 only)", in.ToolCalls)
	}
	if len(in.ExecCommands) != 1 || in.ExecCommands[0] != "git commit -m x" {
		t.Fatalf("ExecCommands=%v want [git commit -m x]", in.ExecCommands)
	}
}
