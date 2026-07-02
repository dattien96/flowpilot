package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mockGeminiCommand(t *testing.T, run func(ctx context.Context, name string, args ...string) *exec.Cmd) func() {
	t.Helper()
	original := commandContextFn
	commandContextFn = run
	return func() { commandContextFn = original }
}

func TestGeminiCLIArgsForReadOnlyAndYolo(t *testing.T) {
	readOnly := strings.Join(geminiCLIArgs("/tmp/work", "project-1", "gemini-3.5-flash-medium", false, false, true), " ")
	for _, want := range []string{"--project project-1", "--new-project", "--model gemini-3.5-flash-medium", "--add-dir /tmp/work", "--sandbox"} {
		if !strings.Contains(readOnly, want) {
			t.Fatalf("geminiCLIArgs(readOnly) = %q, missing %q", readOnly, want)
		}
	}
	for _, arg := range geminiCLIArgs("/tmp/work", "project-1", "gemini-3.5-flash-medium", false, false, true) {
		if arg == "--print" {
			t.Fatalf("geminiCLIArgs(readOnly) = %q, should leave --print until the prompt is appended", readOnly)
		}
	}

	yolo := strings.Join(geminiCLIArgs("/tmp/work", "project-1", "", true, true, false), " ")
	if !strings.Contains(yolo, "--continue") || !strings.Contains(yolo, "--dangerously-skip-permissions") {
		t.Fatalf("geminiCLIArgs(yolo) = %q, want continue + dangerously-skip-permissions", yolo)
	}
}

func TestGeminiAdapterRunsAgyPrintAndCompletesTurn(t *testing.T) {
	var gotName string
	var gotArgs []string
	defer mockGeminiCommand(t, func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string{}, args...)
		return testShellCommand(ctx, "printf 'Hello from agy\\n'")
	})()

	homePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(homePath, ".gemini", "config", "projects"), 0o755); err != nil {
		t.Fatalf("mkdir projects dir: %v", err)
	}
	a := newGeminiAdapter(".", "acct-gemini", map[string]string{"HOME": homePath})
	b := &fakeClaudeBridge{}
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{
		RunID:             "run-1",
		StepID:            "step-1",
		ProjectID:         "flowpilot",
		Cwd:               workspace,
		Prompt:            "hi",
		ModelName:         "gemini-flash",
		ProviderSessionID: "thread-1",
	}, b); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	if gotName != "agy" {
		t.Fatalf("binary = %q, want agy", gotName)
	}
	entries, err := os.ReadDir(filepath.Join(homePath, ".gemini", "config", "projects"))
	if err != nil {
		t.Fatalf("read projects dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("project configs = %d, want 1", len(entries))
	}
	raw, err := os.ReadFile(filepath.Join(homePath, ".gemini", "config", "projects", entries[0].Name()))
	if err != nil {
		t.Fatalf("read project config: %v", err)
	}
	var cfg geminiProjectConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("decode project config: %v", err)
	}
	args := strings.Join(gotArgs, " ")
	for _, want := range []string{"--print", "--project " + cfg.ID, "--model gemini-3.5-flash-medium", "--sandbox", "hi"} {
		if !strings.Contains(args, want) {
			t.Fatalf("agy args = %q, missing %q", args, want)
		}
	}
	if len(gotArgs) < 2 || gotArgs[len(gotArgs)-2] != "--print" || !strings.Contains(gotArgs[len(gotArgs)-1], "hi") {
		t.Fatalf("agy args = %q, want --print followed by actual prompt at the end", args)
	}
	if strings.Contains(args, "--project thread-1") {
		t.Fatalf("agy args = %q, should not use FlowPilot thread id as AGY project id", args)
	}
	if strings.Contains(args, "--continue") {
		t.Fatalf("agy args = %q, should not continue from an unbound FlowPilot thread id", args)
	}
	if strings.Contains(args, "--new-project") {
		t.Fatalf("agy args = %q, should not pass --new-project when project config already exists; --project UUID is sufficient", args)
	}
	done, ok := b.firstOf(EventTurnCompleted)
	if !ok || done.FinalMessage != "Hello from agy" {
		t.Fatalf("turn completed event = %+v, want final agy output", done)
	}
}

func TestGeminiAdapterRecoversEmptyAgyStdoutFromConversationDB(t *testing.T) {
	defer mockGeminiCommand(t, func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return testShellCommand(ctx, "")
	})()
	originalRecover := recoverGeminiAgyLatestMessageFn
	defer func() { recoverGeminiAgyLatestMessageFn = originalRecover }()

	homePath := t.TempDir()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(homePath, ".gemini", "config", "projects"), 0o755); err != nil {
		t.Fatalf("mkdir projects dir: %v", err)
	}
	recoverGeminiAgyLatestMessageFn = func(cwd string, env map[string]string) string {
		if cwd != workspace {
			t.Fatalf("recovery cwd = %q, want %q", cwd, workspace)
		}
		if env["HOME"] != homePath {
			t.Fatalf("recovery HOME = %q, want %q", env["HOME"], homePath)
		}
		return "Recovered AGY answer"
	}

	a := newGeminiAdapter(".", "acct-gemini", map[string]string{"HOME": homePath})
	b := &fakeClaudeBridge{}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{
		RunID:     "run-1",
		StepID:    "step-1",
		ProjectID: "flowpilot",
		Cwd:       workspace,
		Prompt:    "hi",
		ModelName: "gemini-flash",
	}, b); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	done, ok := b.firstOf(EventTurnCompleted)
	if !ok || done.FinalMessage != "Recovered AGY answer" {
		t.Fatalf("turn completed event = %+v, want recovered output", done)
	}
	completed, ok := b.firstOf(EventMessageCompleted)
	if !ok || completed.Text != "Recovered AGY answer" {
		t.Fatalf("message completed event = %+v, want recovered output", completed)
	}
}

func TestExtractGeminiAgyOutputFromPayload(t *testing.T) {
	message := "Hello from recovered AGY.\n\n- It includes Markdown output."
	payload := []byte{
		0x0a, 0x09,
	}
	payload = append(payload, []byte("sessionID")...)
	payload = append(payload, 0xa2, 0x01, byte(len(message)))
	payload = append(payload, []byte(message)...)

	if got := extractGeminiAgyOutputFromPayload(payload); got != message {
		t.Fatalf("extracted output = %q, want %q", got, message)
	}
}

func TestGeminiAdapterReusesMappedProjectOnResume(t *testing.T) {
	sessions := newGeminiSessionMap()
	sessions.setRealSession("acct-a", "thread-1", "project-77")

	var gotArgs []string
	defer mockGeminiCommand(t, func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotArgs = append([]string{}, args...)
		return testShellCommand(ctx, "printf 'continued\\n'")
	})()

	a := newGeminiAdapter(".", "acct-a", nil)
	a.sessions = sessions
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{
		RunID:             "run-1",
		StepID:            "step-1",
		Cwd:               workspace,
		Prompt:            "resume",
		ProviderSessionID: "thread-1",
	}, &fakeClaudeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	args := strings.Join(gotArgs, " ")
	if !strings.Contains(args, "--project project-77") || !strings.Contains(args, "--continue") {
		t.Fatalf("agy args = %q, want mapped project + continue", args)
	}
	if len(gotArgs) < 2 || gotArgs[len(gotArgs)-2] != "--print" || !strings.Contains(gotArgs[len(gotArgs)-1], "resume") {
		t.Fatalf("agy args = %q, want --print followed by actual prompt at the end", args)
	}
	if got := sessions.realSession("acct-a", "run-1"); got != "project-77" {
		t.Fatalf("run mapping = %q, want project-77", got)
	}
}

func TestGeminiAdapterUsesProjectIDBeforeRunFallback(t *testing.T) {
	var gotArgs []string
	defer mockGeminiCommand(t, func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotArgs = append([]string{}, args...)
		return testShellCommand(ctx, "printf 'project scoped\\n'")
	})()

	homePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(homePath, ".gemini", "config", "projects"), 0o755); err != nil {
		t.Fatalf("mkdir projects dir: %v", err)
	}
	a := newGeminiAdapter(".", "acct-gemini", map[string]string{"HOME": homePath})
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{
		RunID:     "run-99",
		StepID:    "step-1",
		ProjectID: "flowpilot",
		Cwd:       workspace,
		Prompt:    "resume in project",
	}, &fakeClaudeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	args := strings.Join(gotArgs, " ")
	if strings.Contains(args, "--project flowpilot-gemini-run-99") {
		t.Fatalf("agy args = %q, should not fall back to run-based project id when ProjectID is present", args)
	}
	if strings.Contains(args, "--project flowpilot") {
		t.Fatalf("agy args = %q, should prefer the workspace-bound AGY project id over the FlowPilot project id when bootstrapping", args)
	}
}

func TestGeminiAdapterUsesConfiguredAgyProjectForWorkspace(t *testing.T) {
	var gotArgs []string
	defer mockGeminiCommand(t, func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotArgs = append([]string{}, args...)
		return testShellCommand(ctx, "printf 'workspace project\\n'")
	})()

	homePath := t.TempDir()
	projectsDir := filepath.Join(homePath, ".gemini", "config", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatalf("mkdir projects dir: %v", err)
	}
	workspace := filepath.Join(t.TempDir(), "flowpilot")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	payload, err := buildGeminiProjectConfigPayload("d77f0d2e-2e78-4adf-bf19-93599129d476", "flowpilot", workspace)
	if err != nil {
		t.Fatalf("build project config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectsDir, "d77f0d2e-2e78-4adf-bf19-93599129d476.json"), payload, 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	a := newGeminiAdapter(".", "acct-gemini", map[string]string{"HOME": homePath})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{
		RunID:     "run-1",
		StepID:    "step-1",
		ProjectID: "flowpilot",
		Cwd:       workspace,
		Prompt:    "hi",
	}, &fakeClaudeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	args := strings.Join(gotArgs, " ")
	if !strings.Contains(args, "--project d77f0d2e-2e78-4adf-bf19-93599129d476") {
		t.Fatalf("agy args = %q, want configured AGY project id", args)
	}
	if strings.Contains(args, "--new-project") {
		t.Fatalf("agy args = %q, should not pass --new-project when project config already exists; --project UUID is sufficient", args)
	}
}

func TestGeminiAdapterBootstrapsAgyProjectForWorkspace(t *testing.T) {
	var gotArgs []string
	defer mockGeminiCommand(t, func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotArgs = append([]string{}, args...)
		return testShellCommand(ctx, "printf 'bootstrapped\\n'")
	})()

	homePath := t.TempDir()
	projectsDir := filepath.Join(homePath, ".gemini", "config", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatalf("mkdir projects dir: %v", err)
	}
	workspace := filepath.Join(t.TempDir(), "gate-sandbox")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	a := newGeminiAdapter(".", "acct-gemini", map[string]string{"HOME": homePath})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{
		RunID:     "run-1",
		StepID:    "step-1",
		ProjectID: "project-gate-sandbox",
		Cwd:       workspace,
		Prompt:    "hi",
	}, &fakeClaudeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		t.Fatalf("read projects dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("project configs = %d, want 1", len(entries))
	}
	configPath := filepath.Join(projectsDir, entries[0].Name())
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read project config: %v", err)
	}
	var cfg geminiProjectConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("decode project config: %v", err)
	}
	if cfg.Name != "gate-sandbox" {
		t.Fatalf("project config name = %q, want gate-sandbox", cfg.Name)
	}
	args := strings.Join(gotArgs, " ")
	if !strings.Contains(args, "--project "+cfg.ID) {
		t.Fatalf("agy args = %q, want bootstrapped project id %q", args, cfg.ID)
	}
	if strings.Contains(args, "--new-project") {
		t.Fatalf("agy args = %q, should not pass --new-project after bootstrapping config; --project UUID is sufficient", args)
	}
}

func TestGeminiAdapterRequiresWorkspacePath(t *testing.T) {
	a := newGeminiAdapter("/default/workspace", "acct-gemini", nil)
	err := a.SendTurn(context.Background(), TurnRequest{
		RunID:     "run-1",
		StepID:    "step-1",
		ProjectID: "project-gate-sandbox",
		Prompt:    "hi",
	}, &fakeClaudeBridge{})
	if !errors.Is(err, errGeminiWorkspaceRequired) {
		t.Fatalf("SendTurn error = %v, want errGeminiWorkspaceRequired", err)
	}
}

func TestGeminiAdapterScopesSessionMappingByAccount(t *testing.T) {
	sessions := newGeminiSessionMap()
	sessions.setRealSession("acct-a", "thread-1", "real-a")
	sessions.setRealSession("acct-b", "thread-1", "real-b")
	if got := sessions.realSession("acct-a", "thread-1"); got != "real-a" {
		t.Fatalf("acct-a realSession = %q, want real-a", got)
	}
	if got := sessions.realSession("acct-b", "thread-1"); got != "real-b" {
		t.Fatalf("acct-b realSession = %q, want real-b", got)
	}
	if got := sessions.realSession("acct-c", "thread-1"); got != "" {
		t.Fatalf("acct-c realSession = %q, want empty", got)
	}
}

func TestGeminiAdapterPromptPrepAndEnv(t *testing.T) {
	homePath := filepath.Join(t.TempDir(), "gemini-home")
	if err := os.MkdirAll(homePath, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	var sawPrepared bool
	defer mockGeminiCommand(t, func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if got := strings.Join(args, " "); strings.Contains(got, "prepared: hi") {
			sawPrepared = true
		}
		cmd := exec.CommandContext(ctx, "sh", "-c", "env | grep '^GEMINI_HOME=' && printf 'ok\\n'")
		cmd.Env = append(os.Environ(), "GEMINI_HOME="+homePath)
		return cmd
	})()

	a := newGeminiAdapter(".", "acct-a", map[string]string{"GEMINI_HOME": homePath})
	a.promptPrep = func(req TurnRequest) string { return "prepared: " + req.Prompt }
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{RunID: "r", StepID: "s", Cwd: workspace, Prompt: "hi"}, &fakeClaudeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if !sawPrepared {
		t.Fatal("expected prepared prompt to be passed to agy")
	}
}

func TestGeminiAdapterInjectsActiveProjectContextIntoPrompt(t *testing.T) {
	var gotArgs []string
	defer mockGeminiCommand(t, func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotArgs = append([]string{}, args...)
		return testShellCommand(ctx, "printf 'ok\\n'")
	})()

	workspace := t.TempDir()
	a := newGeminiAdapter("/default/workspace", "acct-a", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{
		RunID:     "run-1",
		StepID:    "step-1",
		ProjectID: "flowpilot",
		Cwd:       workspace,
		Prompt:    "do you know this project?",
	}, &fakeClaudeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	args := strings.Join(gotArgs, " ")
	for _, want := range []string{
		"Active project id: flowpilot",
		"Attached workspace cwd: " + workspace,
		"already attached to the workspace above",
		"do you know this project?",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("agy args = %q, missing %q", args, want)
		}
	}
}

func TestGeminiAdapterSurfacesProcessError(t *testing.T) {
	defer mockGeminiCommand(t, func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, "sh", "-c", "echo unsupported flags >&2; exit 2")
		return cmd
	})()

	a := newGeminiAdapter(".", "acct-a", nil)
	err := a.SendTurn(context.Background(), TurnRequest{RunID: "r", StepID: "s", Cwd: t.TempDir(), Prompt: "hi"}, &fakeClaudeBridge{})
	if err == nil || !strings.Contains(err.Error(), "unsupported flags") {
		t.Fatalf("SendTurn error = %v, want stderr surfaced", err)
	}
}

func TestGeminiRegistryGating(t *testing.T) {
	def := DefaultProviderRegistry()
	if reg, _ := def.Get(ProviderKeyGemini); reg.Status != ProviderStatusPlaceholder {
		t.Fatalf("default gemini status = %q, want placeholder", reg.Status)
	}
	if _, err := def.Selectable(ProviderKeyGemini); err == nil {
		t.Fatalf("placeholder gemini must not be selectable")
	}

	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("GEMINI_HOME", "")
	r, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	reg := ProviderRegistryFor(r)
	got, ok := reg.Get(ProviderKeyGemini)
	if !ok || got.Status != ProviderStatusAvailable {
		t.Fatalf("live gemini status = %q (ok=%v), want available", got.Status, ok)
	}
	if got.Capabilities.Streaming || !got.Capabilities.SkillSelection || !got.Capabilities.Interrupt {
		t.Fatalf("live gemini capabilities = %+v, want non-streaming skill-selection interrupt", got.Capabilities)
	}
	if got.Capabilities.ApprovalEvents || got.Capabilities.FileEvents || got.Capabilities.Mcp || got.Capabilities.Resume || got.Capabilities.Vision {
		t.Fatalf("live gemini must not advertise unsupported capabilities: %+v", got.Capabilities)
	}
}

func TestRefreshResumeHandleCapturesGeminiRealSession(t *testing.T) {
	svc := NewInteractiveService()
	sessions := newGeminiSessionMap()
	sessions.setRealSession("test-account", "thread-1", "real-gemini-session")
	a := newGeminiAdapter(".", "test-account", nil)
	a.sessions = sessions
	rs := &interactiveRun{providerKey: ProviderKeyGemini, providerSessionID: "thread-1"}

	if got := svc.refreshResumeHandleLocked(rs, a); got != "" {
		t.Fatalf("refreshResumeHandleLocked returned %q, want empty Gemini delta", got)
	}
	if rs.realProviderSessionID != "real-gemini-session" {
		t.Fatalf("realProviderSessionID = %q, want real-gemini-session", rs.realProviderSessionID)
	}
}

func TestRefreshResumeHandleCapturesGeminiRealSessionFromRunAccountScope(t *testing.T) {
	svc := NewInteractiveService()
	sessions := newGeminiSessionMap()
	sessions.setRealSession("stored-account", "thread-1", "real-gemini-session")
	a := newGeminiAdapter(".", "active-account", nil)
	a.sessions = sessions
	rs := &interactiveRun{
		providerKey:       ProviderKeyGemini,
		providerSessionID: "thread-1",
		providerAccountID: "stored-account",
	}

	if got := svc.refreshResumeHandleLocked(rs, a); got != "" {
		t.Fatalf("refreshResumeHandleLocked returned %q, want empty Gemini delta", got)
	}
	if rs.realProviderSessionID != "real-gemini-session" {
		t.Fatalf("realProviderSessionID = %q, want real-gemini-session", rs.realProviderSessionID)
	}
}

func TestGeminiAdapterRecordsSessionToStore(t *testing.T) {
	var stdout bytes.Buffer
	defer mockGeminiCommand(t, func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return testShellCommand(ctx, "printf 'stored\\n'")
	})()

	store := &fakeProviderSessionStore{ch: make(chan ProviderSessionRecord, 2)}
	homePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(homePath, ".gemini", "config", "projects"), 0o755); err != nil {
		t.Fatalf("mkdir projects dir: %v", err)
	}
	a := newGeminiAdapter(".", "acct-a", map[string]string{"HOME": homePath})
	a.sessionStore = store
	a.sessions = newGeminiSessionMap()
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := a.SendTurn(ctx, TurnRequest{RunID: "run-9", StepID: "step-2", Cwd: workspace, Prompt: "hi"}, &fakeClaudeBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	_ = stdout
	select {
	case rec := <-store.ch:
		if strings.TrimSpace(rec.ProviderSessionID) == "" || rec.ProviderThreadID != rec.ProviderSessionID {
			t.Fatalf("persisted record = %+v", rec)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for provider session record")
	}
}

func TestAgyFilteredEnvStripsClaudeCodeVars(t *testing.T) {
	base := []string{
		"PATH=/usr/bin",
		"HOME=/home/user",
		"CLAUDECODE=1",
		"AI_AGENT=claude-code_2-1-187_agent",
		"CLAUDE_CODE_ENTRYPOINT=cli",
		"CLAUDE_AGENT_FOO=bar",
		"TERM=xterm",
	}
	overrides := map[string]string{"HOME": "/custom/home"}

	got := agyFilteredEnv(base, overrides)

	for _, blockedKey := range []string{"CLAUDECODE", "AI_AGENT", "CLAUDE_CODE_ENTRYPOINT", "CLAUDE_AGENT_FOO"} {
		for _, entry := range got {
			if strings.HasPrefix(entry, blockedKey+"=") {
				t.Errorf("agyFilteredEnv kept blocked var: %q", entry)
			}
		}
	}
	wantKept := map[string]string{
		"PATH": "/usr/bin",
		"TERM": "xterm",
		"HOME": "/custom/home",
	}
	for k, wantVal := range wantKept {
		found := false
		for _, entry := range got {
			if entry == k+"="+wantVal {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("agyFilteredEnv missing %s=%s; got %v", k, wantVal, got)
		}
	}
}
