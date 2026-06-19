package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type captureResumeBridge struct {
	events []ProviderEvent
}

func (b *captureResumeBridge) Emit(ev ProviderEvent) { b.events = append(b.events, ev) }
func (b *captureResumeBridge) RequestApproval(ApprovalDetails) (string, error) { return "", nil }
func (b *captureResumeBridge) AskQuestion(string, []QuestionOption, bool) ([]string, error) {
	return nil, nil
}
func (b *captureResumeBridge) SpawnAgent(_ SpawnAgentInput) (SpawnAgentResult, error) {
	return SpawnAgentResult{}, nil
}

func TestCodexFreshRunUsesAppServerPath(t *testing.T) {
	var adapterCalls atomic.Int32
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyCodex,
		DisplayName:  "Codex",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, Resume: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, bridge TurnBridge) error {
				adapterCalls.Add(1)
				bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := NewInteractiveServiceWithStore(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	handle, apiErr := svc.createRun(StartRunInput{
		ProjectID:   "project-1",
		ProviderKey: ProviderKeyCodex,
		ChatMode:    "normal_chat",
		Cwd:         t.TempDir(),
	})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	if _, apiErr := svc.startTurn(handle.RunID, TurnInput{StepID: handle.StepID, Prompt: "hello"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %v", apiErr)
	}
	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[handle.RunID].turnInFlight
	}, "fresh codex turn to finish")
	if adapterCalls.Load() != 1 {
		t.Fatalf("adapterCalls = %d, want 1", adapterCalls.Load())
	}
}

func TestCodexResumeCommandUsesYoloDerivedSandboxAndApproval(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	for _, tc := range []struct {
		name string
		yolo bool
	}{
		{name: "yolo-off", yolo: false},
		{name: "yolo-on", yolo: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotArgs []string
			originalCmd := commandContextFn
			commandContextFn = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
				gotArgs = append([]string{}, args...)
				script := `out=""; prev=""; for a in "$@"; do if [ "$prev" = "-o" ]; then out="$a"; fi; prev="$a"; done; [ -n "$out" ] && printf "done\n" > "$out"`
				cmdArgs := append([]string{"-c", script, "sh"}, args...)
				return exec.CommandContext(ctx, "sh", cmdArgs...)
			}
			defer func() { commandContextFn = originalCmd }()

			adapter := newCodexResumeAdapter(home, nil)
			err := adapter.SendTurn(context.Background(), TurnRequest{
				RunID:             "run-1",
				ProviderSessionID: "rollout-abc",
				Prompt:            "continue",
				YoloMode:          tc.yolo,
				Cwd:               workspace,
			}, &captureResumeBridge{})
			if err != nil {
				t.Fatalf("SendTurn: %v", err)
			}
			sandbox, approvalMode := codexYoloDerive(tc.yolo)
			joined := strings.Join(gotArgs, " ")
			if !strings.Contains(joined, `sandbox_mode="`+sandbox+`"`) {
				t.Fatalf("args missing sandbox_mode=%q: %v", sandbox, gotArgs)
			}
			if !strings.Contains(joined, `approval_policy="`+approvalMode+`"`) {
				t.Fatalf("args missing approval_policy=%q: %v", approvalMode, gotArgs)
			}
		})
	}
}

func TestCodexResumeStdoutMapsFinalAssistantMessage(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	originalCmd := commandContextFn
	commandContextFn = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		script := `out=""; prev=""; for a in "$@"; do if [ "$prev" = "-o" ]; then out="$a"; fi; prev="$a"; done; printf "assistant final text\n"; [ -n "$out" ] && printf "assistant final text\n" > "$out"`
		cmdArgs := append([]string{"-c", script, "sh"}, args...)
		return exec.CommandContext(ctx, "sh", cmdArgs...)
	}
	defer func() { commandContextFn = originalCmd }()

	bridge := &captureResumeBridge{}
	adapter := newCodexResumeAdapter(home, nil)
	if err := adapter.SendTurn(context.Background(), TurnRequest{
		RunID:             "run-1",
		ProviderSessionID: "rollout-abc",
		Prompt:            "continue",
		Cwd:               workspace,
	}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if len(bridge.events) != 2 {
		t.Fatalf("events = %+v, want 2", bridge.events)
	}
	if bridge.events[0].Type != EventMessageCompleted || bridge.events[0].Text != "assistant final text" {
		t.Fatalf("unexpected first event: %+v", bridge.events[0])
	}
	if bridge.events[1].Type != EventTurnCompleted {
		t.Fatalf("unexpected second event: %+v", bridge.events[1])
	}
}

func TestCodexResumeCommandFailureEmitsNoAssistantCompletedEvent(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	originalCmd := commandContextFn
	commandContextFn = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		script := `echo "auth failed" 1>&2; exit 7`
		cmdArgs := append([]string{"-c", script, "sh"}, args...)
		return exec.CommandContext(ctx, "sh", cmdArgs...)
	}
	defer func() { commandContextFn = originalCmd }()

	bridge := &captureResumeBridge{}
	adapter := newCodexResumeAdapter(home, nil)
	err := adapter.SendTurn(context.Background(), TurnRequest{
		RunID:             "run-1",
		ProviderSessionID: "rollout-abc",
		Prompt:            "continue",
		Cwd:               workspace,
	}, bridge)
	if err == nil || !strings.Contains(err.Error(), "auth failed") {
		t.Fatalf("SendTurn error = %v, want auth failed", err)
	}
	for _, ev := range bridge.events {
		if ev.Type == EventMessageCompleted {
			t.Fatalf("unexpected assistant completed event: %+v", bridge.events)
		}
	}
}

func TestCodexResumeCommandUsesActiveAccountHomeAndCwd(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	envCapture := filepath.Join(t.TempDir(), "env.txt")
	cwdCapture := filepath.Join(t.TempDir(), "cwd.txt")
	originalCmd := commandContextFn
	commandContextFn = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		script := `printf "%s" "$CODEX_HOME" > ` + testShellQuote(envCapture) + `; pwd > ` + testShellQuote(cwdCapture) + `; out=""; prev=""; for a in "$@"; do if [ "$prev" = "-o" ]; then out="$a"; fi; prev="$a"; done; [ -n "$out" ] && printf "done\n" > "$out"`
		cmdArgs := append([]string{"-c", script, "sh"}, args...)
		return exec.CommandContext(ctx, "sh", cmdArgs...)
	}
	defer func() { commandContextFn = originalCmd }()

	adapter := newCodexResumeAdapter(home, nil)
	if err := adapter.SendTurn(context.Background(), TurnRequest{
		RunID:             "run-1",
		ProviderSessionID: "rollout-abc",
		Prompt:            "continue",
		Cwd:               workspace,
	}, &captureResumeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	rawHome, _ := os.ReadFile(envCapture)
	if strings.TrimSpace(string(rawHome)) != home {
		t.Fatalf("CODEX_HOME = %q, want %q", strings.TrimSpace(string(rawHome)), home)
	}
	rawCwd, _ := os.ReadFile(cwdCapture)
	gotCwd := strings.TrimSpace(string(rawCwd))
	wantCwd, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatalf("EvalSymlinks(workspace): %v", err)
	}
	if gotCwd != wantCwd {
		t.Fatalf("cwd = %q, want %q", gotCwd, wantCwd)
	}
}

func testShellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

func TestCodexRestoredRunUsesCLIResumePathDirectly(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	originalCmd := commandContextFn
	var gotArgs []string
	commandContextFn = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		gotArgs = append([]string{}, args...)
		script := `out=""; prev=""; for a in "$@"; do if [ "$prev" = "-o" ]; then out="$a"; fi; prev="$a"; done; [ -n "$out" ] && printf "done\n" > "$out"`
		cmdArgs := append([]string{"-c", script, "sh"}, args...)
		return exec.CommandContext(ctx, "sh", cmdArgs...)
	}
	defer func() { commandContextFn = originalCmd }()
	adapter := newCodexResumeAdapter(home, nil)
	if err := adapter.SendTurn(context.Background(), TurnRequest{
		RunID:             "run-1",
		ProviderSessionID: "rollout-abc",
		Prompt:            "continue",
		Cwd:               workspace,
	}, &captureResumeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if got := strings.Join(gotArgs[:3], " "); got != "exec resume rollout-abc" {
		t.Fatalf("unexpected args: %v", gotArgs)
	}
}

func TestCodexResumeAdapterOutputFileFallbackIsStable(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	originalCmd := commandContextFn
	commandContextFn = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		script := `sleep 0.01`
		cmdArgs := append([]string{"-c", script, "sh"}, args...)
		return exec.CommandContext(ctx, "sh", cmdArgs...)
	}
	defer func() { commandContextFn = originalCmd }()
	adapter := newCodexResumeAdapter(home, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := adapter.SendTurn(ctx, TurnRequest{
		RunID:             "run-1",
		ProviderSessionID: "rollout-abc",
		Prompt:            "",
		Cwd:               workspace,
	}, &captureResumeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
}
