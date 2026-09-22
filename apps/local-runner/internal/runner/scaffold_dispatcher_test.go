package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"flowpilot-runner/internal/skillpack"
)

// recordingScaffoldExecutor is the dispatcher's LLM seam: it records every turn
// request and never invokes a real provider, so the whole Task-384/386 matrix can
// be exercised deterministically (including the self-healing loop).
type recordingScaffoldExecutor struct {
	mu     sync.Mutex
	turns  []PromptExecutionRequest
	onCall func(call int, req PromptExecutionRequest) (PromptExecutionResult, error)
}

func (e *recordingScaffoldExecutor) ExecutePrompt(_ context.Context, req PromptExecutionRequest) (PromptExecutionResult, error) {
	e.mu.Lock()
	e.turns = append(e.turns, req)
	call := len(e.turns)
	handler := e.onCall
	e.mu.Unlock()

	if handler != nil {
		result, err := handler(call, req)
		if result.ProviderKey == "" {
			result.ProviderKey = req.ProviderKey
		}
		return result, err
	}
	return PromptExecutionResult{
		Status:      "success",
		RunID:       fmt.Sprintf("scaffold-run-%d", call),
		ProviderKey: req.ProviderKey,
		ExitCode:    0,
	}, nil
}

func (e *recordingScaffoldExecutor) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.turns)
}

func (e *recordingScaffoldExecutor) turn(index int) PromptExecutionRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.turns[index]
}

// realScaffoldRecipe returns the shipped react-native recipe with its gate
// command swapped for a cheap one, so tests exercise the REAL manifest, REAL
// skill set and REAL prompt template without shelling out to pnpm.
func realScaffoldRecipe(t *testing.T, gateCommand string) *skillpack.ScaffoldRecipe {
	t.Helper()
	recipe, ok, err := skillpack.LoadScaffoldRecipe("react-native")
	if err != nil || !ok || recipe == nil {
		t.Fatalf("load react-native recipe: ok=%v err=%v", ok, err)
	}
	copied := *recipe
	copied.ScaffoldSkills = append([]string(nil), recipe.ScaffoldSkills...)
	copied.VerificationGate.Command = gateCommand
	return &copied
}

// dispatcherForRecipe wires the dispatcher with the real embedded prompt template
// and a fixed recipe (nil ⇒ "no verified recipe").
func dispatcherForRecipe(executor ScaffoldPromptExecutor, recipe *skillpack.ScaffoldRecipe) *ScaffoldDispatcher {
	return &ScaffoldDispatcher{
		executor: executor,
		loadRecipe: func(string) (*skillpack.ScaffoldRecipe, bool, error) {
			if recipe == nil {
				return nil, false, nil
			}
			return recipe, true, nil
		},
		loadPrompt: loadBuiltinPromptText,
	}
}

func newScaffoldWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// A real init already installed the skills into .agents/skills; reproduce that
	// so the Context Profile resolution path is exercised for real.
	for _, name := range reactNativeScaffoldSkillNames() {
		skillDir := filepath.Join(dir, ".agents", "skills", name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		contents := "---\nname: " + name + "\ndescription: blueprint skill for " + name + "\n---\n\n# " + name + "\n"
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func reactNativeScaffoldSkillNames() []string {
	recipe, ok, err := skillpack.LoadScaffoldRecipe("react-native")
	if err != nil || !ok || recipe == nil {
		return nil
	}
	return recipe.ScaffoldSkills
}

func TestScaffoldDispatcher_CapablePlatformDispatchesTurnWithBlueprintSkills(t *testing.T) {
	executor := &recordingScaffoldExecutor{}
	recipe := realScaffoldRecipe(t, "true")
	dispatcher := dispatcherForRecipe(executor, recipe)
	workspace := newScaffoldWorkspace(t)

	result, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		ProjectID:    "proj-rn",
		WorkspaceDir: workspace,
		Platform:     "react-native",
		ProviderKey:  "claude",
		ModelName:    "claude-sonnet-4-5",
		Trigger:      "init_all",
	})
	if err != nil {
		t.Fatalf("Dispatch() error = %v, want nil", err)
	}
	if result.Status != ScaffoldStatusDone {
		t.Fatalf("Status = %q (%s), want done", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "compiler gate PASS") {
		t.Fatalf("Message = %q, want a gate PASS note", result.Message)
	}
	if len(result.SkillsAttached) != 4 {
		t.Fatalf("SkillsAttached = %v, want the 4 declared blueprint skills", result.SkillsAttached)
	}
	if result.Attempts != 1 {
		t.Fatalf("Attempts = %d, want 1 (clean first pass)", result.Attempts)
	}
	if result.CompilerGate == nil || !result.CompilerGate.Passed {
		t.Fatalf("CompilerGate = %+v, want Passed", result.CompilerGate)
	}
	if executor.count() != 1 {
		t.Fatalf("executor calls = %d, want exactly 1 scaffold turn", executor.count())
	}

	turn := executor.turn(0)
	if !turn.AllowWrite {
		t.Fatal("scaffold turn must be write-enabled")
	}
	if !turn.YoloMode {
		t.Fatal("scaffold turn must run in YOLO posture")
	}
	if turn.WorkingDirectory != workspace {
		t.Fatalf("WorkingDirectory = %q, want %q", turn.WorkingDirectory, workspace)
	}
	if turn.TimeoutMs != scaffoldTurnTimeoutMs {
		t.Fatalf("TimeoutMs = %d, want %d", turn.TimeoutMs, scaffoldTurnTimeoutMs)
	}
	if turn.ProviderKey != "claude" || turn.ModelName != "claude-sonnet-4-5" {
		t.Fatalf("provider/model = %q/%q, want claude/claude-sonnet-4-5", turn.ProviderKey, turn.ModelName)
	}
	if strings.Join(turn.SkillIds, ",") != strings.Join(recipe.ScaffoldSkills, ",") {
		t.Fatalf("SkillIds = %v, want %v", turn.SkillIds, recipe.ScaffoldSkills)
	}
	if strings.Contains(turn.Prompt, "{{") {
		t.Fatalf("prompt still contains unrendered template braces:\n%s", turn.Prompt)
	}
	for _, want := range append([]string{"react-native", recipe.VerificationGate.Command}, recipe.ScaffoldSkills...) {
		if !strings.Contains(turn.Prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, turn.Prompt)
		}
	}

	statusFile, ok := LoadScaffoldStatusFile(workspace)
	if !ok {
		t.Fatalf("expected %s to be written", result.StatusPath)
	}
	if statusFile.Status != ScaffoldStatusDone || statusFile.Platform != "react-native" {
		t.Fatalf("status file = %+v, want done/react-native", statusFile)
	}
	if statusFile.ProjectID != "proj-rn" || statusFile.Attempts != 1 {
		t.Fatalf("status file = %+v, want projectId=proj-rn attempts=1", statusFile)
	}
	if statusFile.VerificationCommand != recipe.VerificationGate.Command {
		t.Fatalf("status file verification command = %q, want %q", statusFile.VerificationCommand, recipe.VerificationGate.Command)
	}
}

func TestScaffoldDispatcher_ContextProfileResolvesInstalledSkills(t *testing.T) {
	// Proves the "Context Profile" claim end-to-end at the prompt level: the
	// SkillIds the dispatcher passes resolve to the skill files init installed.
	workspace := newScaffoldWorkspace(t)
	executor := &recordingScaffoldExecutor{}
	dispatcher := dispatcherForRecipe(executor, realScaffoldRecipe(t, "true"))

	if _, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		WorkspaceDir: workspace,
		Platform:     "react-native",
		ProviderKey:  "codex",
	}); err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}

	runner := &Runner{workspace: workspace}
	profile := runner.injectSkillContent(workspace, "BASE PROMPT", reactNativeScaffoldSkillNames())
	if !strings.Contains(profile, "## Selected Skills") {
		t.Fatalf("Context Profile has no skill block:\n%s", profile)
	}
	for _, name := range reactNativeScaffoldSkillNames() {
		if !strings.Contains(profile, "/"+name) {
			t.Fatalf("Context Profile missing %q:\n%s", name, profile)
		}
		expectedPath := filepath.Join(workspace, ".agents", "skills", name, "SKILL.md")
		if !strings.Contains(profile, expectedPath) {
			t.Fatalf("Context Profile missing resolved path %q:\n%s", expectedPath, profile)
		}
	}
	if !strings.Contains(profile, "BASE PROMPT") {
		t.Fatal("Context Profile dropped the original prompt")
	}
}
func TestScaffoldDispatcher_PromptTemplateRendersRealRecipeGate(t *testing.T) {
	// The production template must be registered in the pack manifest (otherwise
	// LoadBuiltinPrompt can never return it) and render the real pnpm gate.
	tmpl, found, err := loadBuiltinPromptText(scaffoldPromptRelPath)
	if err != nil || !found || strings.TrimSpace(tmpl) == "" {
		t.Fatalf("prompt %s not registered in the pack manifest: found=%v len=%d err=%v", scaffoldPromptRelPath, found, len(tmpl), err)
	}

	dispatcher := &ScaffoldDispatcher{loadPrompt: loadBuiltinPromptText}
	recipe, ok, err := skillpack.LoadScaffoldRecipe("react-native")
	if err != nil || !ok || recipe == nil {
		t.Fatalf("load real recipe: ok=%v err=%v", ok, err)
	}
	rendered, err := dispatcher.scaffoldPrompt(recipe, "/tmp/workspace")
	if err != nil {
		t.Fatalf("scaffoldPrompt() error = %v", err)
	}
	for _, want := range []string{"pnpm install && pnpm tsc --noEmit", "react-native", "/tmp/workspace"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered prompt missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "{{") {
		t.Fatalf("rendered prompt still has template braces:\n%s", rendered)
	}
}

func TestScaffoldDispatcher_SkipsUnsupportedPlatformWithZeroAICalls(t *testing.T) {
	// Uses the PRODUCTION recipe loader: vuejs ships no scaffold.yaml.
	executor := &recordingScaffoldExecutor{}
	dispatcher := &ScaffoldDispatcher{
		executor:   executor,
		loadRecipe: skillpack.LoadScaffoldRecipe,
		loadPrompt: loadBuiltinPromptText,
	}

	for _, platform := range []string{"vuejs", "ruby", "android", ""} {
		result, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
			WorkspaceDir: t.TempDir(),
			Platform:     platform,
			ProviderKey:  "codex",
		})
		if err != nil {
			t.Fatalf("Dispatch(%q) error = %v, want nil (graceful ignore)", platform, err)
		}
		if result.Status != ScaffoldStatusSkipped {
			t.Fatalf("Dispatch(%q) Status = %q, want skipped", platform, result.Status)
		}
		if result.Message != scaffoldSkippedNoRecipe {
			t.Fatalf("Dispatch(%q) Message = %q, want %q", platform, result.Message, scaffoldSkippedNoRecipe)
		}
	}
	if executor.count() != 0 {
		t.Fatalf("executor calls = %d, want zero AI calls for unsupported platforms", executor.count())
	}
}

func TestScaffoldDispatcher_SkipsDisabledRecipeAndMissingSkills(t *testing.T) {
	executor := &recordingScaffoldExecutor{}
	disabled := realScaffoldRecipe(t, "true")
	disabled.Enabled = false
	dispatcher := dispatcherForRecipe(executor, disabled)

	result, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		WorkspaceDir: t.TempDir(), Platform: "react-native", ProviderKey: "codex",
	})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if result.Status != ScaffoldStatusSkipped || !strings.Contains(result.Message, "recipe disabled") {
		t.Fatalf("disabled recipe result = %+v, want skipped/disabled", result)
	}

	// Near-miss: enabled recipe whose declared skill set is incomplete.
	incomplete := realScaffoldRecipe(t, "true")
	incomplete.ScaffoldSkills = append(incomplete.ScaffoldSkills, "react-native-not-in-pack")
	dispatcher = dispatcherForRecipe(executor, incomplete)

	result, err = dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		WorkspaceDir: t.TempDir(), Platform: "react-native", ProviderKey: "codex",
	})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if result.Status != ScaffoldStatusSkipped {
		t.Fatalf("Status = %q, want skipped when a declared skill is missing", result.Status)
	}
	if !strings.Contains(result.Message, "react-native-not-in-pack") {
		t.Fatalf("Message = %q, want the missing skill named", result.Message)
	}
	if executor.count() != 0 {
		t.Fatalf("executor calls = %d, want zero AI calls", executor.count())
	}
}

func TestScaffoldDispatcher_ReportsTurnFailureWithoutStatusFile(t *testing.T) {
	executor := &recordingScaffoldExecutor{
		onCall: func(_ int, _ PromptExecutionRequest) (PromptExecutionResult, error) {
			return PromptExecutionResult{Status: "failed", ExitCode: 1, ErrorMessage: "provider crashed"}, nil
		},
	}
	workspace := newScaffoldWorkspace(t)
	dispatcher := dispatcherForRecipe(executor, realScaffoldRecipe(t, "true"))

	result, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		WorkspaceDir: workspace, Platform: "react-native", ProviderKey: "codex",
	})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if result.Status != ScaffoldStatusError {
		t.Fatalf("Status = %q, want error", result.Status)
	}
	if !strings.Contains(result.Message, "AI scaffold turn failed") || !strings.Contains(result.Message, "provider crashed") {
		t.Fatalf("Message = %q, want the turn failure surfaced", result.Message)
	}
	if _, ok := LoadScaffoldStatusFile(workspace); ok {
		t.Fatal("a failed turn must not write scaffold-status.json")
	}
}

func TestScaffoldDispatcher_NilExecutorIsReportedNotPanicked(t *testing.T) {
	dispatcher := dispatcherForRecipe(nil, realScaffoldRecipe(t, "true"))
	result, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		WorkspaceDir: t.TempDir(), Platform: "react-native",
	})
	if err != nil {
		t.Fatalf("Dispatch() error = %v, want nil", err)
	}
	if result.Status != ScaffoldStatusError || !strings.Contains(result.Message, "runner is unavailable") {
		t.Fatalf("result = %+v, want error/runner unavailable", result)
	}

	// A missing recipe stays a graceful skip even with no runner at all.
	dispatcher = &ScaffoldDispatcher{loadRecipe: skillpack.LoadScaffoldRecipe, loadPrompt: loadBuiltinPromptText}
	result, err = dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		WorkspaceDir: t.TempDir(), Platform: "vuejs",
	})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if result.Status != ScaffoldStatusSkipped || result.Message != scaffoldSkippedNoRecipe {
		t.Fatalf("result = %+v, want graceful skip", result)
	}
}
func TestScaffoldDispatcher_HealsAfterCompilerGateFailure(t *testing.T) {
	executor := &recordingScaffoldExecutor{}
	dispatcher := dispatcherForRecipe(executor, realScaffoldRecipe(t, "test -f fixed.marker"))
	workspace := newScaffoldWorkspace(t)

	// The "AI" fixes the build on its second turn by creating the marker file the
	// gate is waiting for — exactly the self-healing contract (Task-386 T-2).
	executor.onCall = func(call int, _ PromptExecutionRequest) (PromptExecutionResult, error) {
		if call == 2 {
			if err := os.WriteFile(filepath.Join(workspace, "fixed.marker"), []byte("fixed"), 0o644); err != nil {
				t.Fatalf("write marker: %v", err)
			}
		}
		return PromptExecutionResult{Status: "success", RunID: fmt.Sprintf("run-%d", call), ExitCode: 0}, nil
	}

	result, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		WorkspaceDir: workspace, Platform: "react-native", ProviderKey: "grok",
	})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if result.Status != ScaffoldStatusDone {
		t.Fatalf("Status = %q (%s), want done after healing", result.Status, result.Message)
	}
	if result.Attempts != 2 {
		t.Fatalf("Attempts = %d, want 2 (failed then healed)", result.Attempts)
	}
	if executor.count() != 2 {
		t.Fatalf("executor calls = %d, want 2 (scaffold + one repair turn)", executor.count())
	}

	repair := executor.turn(1)
	if !strings.Contains(repair.Prompt, "[Compiler Gate Thất Bại]") {
		t.Fatalf("repair prompt missing the mandated header:\n%s", repair.Prompt)
	}
	if !strings.Contains(repair.Prompt, "test -f fixed.marker") {
		t.Fatalf("repair prompt must name the failing gate command:\n%s", repair.Prompt)
	}
	if !strings.Contains(repair.Prompt, "sửa mã nguồn") {
		t.Fatalf("repair prompt missing the fix instruction:\n%s", repair.Prompt)
	}
	if strings.Join(repair.SkillIds, ",") != strings.Join(reactNativeScaffoldSkillNames(), ",") {
		t.Fatalf("repair turn SkillIds = %v, want the same blueprint skills", repair.SkillIds)
	}
	if !repair.AllowWrite || !repair.YoloMode {
		t.Fatal("repair turn must keep write access")
	}
	if _, ok := LoadScaffoldStatusFile(workspace); !ok {
		t.Fatal("a healed scaffold must write scaffold-status.json")
	}
}

func TestScaffoldDispatcher_StopsAfterCapExceeded(t *testing.T) {
	executor := &recordingScaffoldExecutor{}
	recipe := realScaffoldRecipe(t, "echo 'src/x.ts(1,1): error TS2304: Cannot find name foo' >&2 && exit 7")
	recipe.VerificationGate.Cap = 3
	dispatcher := dispatcherForRecipe(executor, recipe)
	workspace := newScaffoldWorkspace(t)

	result, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		WorkspaceDir: workspace, Platform: "react-native", ProviderKey: "opencode",
	})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if result.Status != ScaffoldStatusError {
		t.Fatalf("Status = %q, want error after cap", result.Status)
	}
	if result.Attempts != 3 {
		t.Fatalf("Attempts = %d, want the recipe cap 3", result.Attempts)
	}
	if executor.count() != 3 {
		t.Fatalf("executor calls = %d, want 3 (1 scaffold + 2 repairs; no wasted turn after the last failure)", executor.count())
	}
	if !strings.Contains(result.Message, "FAILED after 3 attempt(s)") {
		t.Fatalf("Message = %q, want the cap-exceeded report", result.Message)
	}
	if !strings.Contains(result.Message, "TS2304") {
		t.Fatalf("Message = %q, want the full parsed log handed to the user", result.Message)
	}
	if result.CompilerGate == nil || result.CompilerGate.ExitCode != 7 {
		t.Fatalf("CompilerGate = %+v, want exit code 7", result.CompilerGate)
	}
	if _, ok := LoadScaffoldStatusFile(workspace); ok {
		t.Fatal("a failed scaffold must not write scaffold-status.json")
	}
}
func TestScaffoldDispatcher_GateTimeoutIsNotHealed(t *testing.T) {
	executor := &recordingScaffoldExecutor{}
	recipe := realScaffoldRecipe(t, "sleep 5")
	recipe.VerificationGate.TimeoutSeconds = 1
	dispatcher := dispatcherForRecipe(executor, recipe)
	workspace := newScaffoldWorkspace(t)

	result, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		WorkspaceDir: workspace, Platform: "react-native", ProviderKey: "codex",
	})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if result.Status != ScaffoldStatusError {
		t.Fatalf("Status = %q, want error on gate timeout", result.Status)
	}
	if result.CompilerGate == nil || !result.CompilerGate.TimedOut {
		t.Fatalf("CompilerGate = %+v, want TimedOut", result.CompilerGate)
	}
	if !strings.Contains(result.Message, "could not complete") {
		t.Fatalf("Message = %q, want the non-healable report", result.Message)
	}
	if executor.count() != 1 {
		t.Fatalf("executor calls = %d, want 1 — a stalled install must not burn repair turns", executor.count())
	}
}

func TestScaffoldDispatcher_ReplayGuardSkipsCompletedScaffold(t *testing.T) {
	executor := &recordingScaffoldExecutor{}
	dispatcher := dispatcherForRecipe(executor, realScaffoldRecipe(t, "true"))
	workspace := newScaffoldWorkspace(t)

	first, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		WorkspaceDir: workspace, Platform: "react-native", ProviderKey: "codex",
	})
	if err != nil || first.Status != ScaffoldStatusDone {
		t.Fatalf("first dispatch = %+v err=%v, want done", first, err)
	}

	second, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		WorkspaceDir: workspace, Platform: "react-native", ProviderKey: "codex",
	})
	if err != nil {
		t.Fatalf("second Dispatch() error = %v", err)
	}
	if second.Status != ScaffoldStatusSkipped || !strings.Contains(second.Message, "already scaffolded") {
		t.Fatalf("second dispatch = %+v, want skipped/already scaffolded", second)
	}
	if executor.count() != 1 {
		t.Fatalf("executor calls = %d, want 1 (replay guard must not re-run AI)", executor.count())
	}

	forced, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		WorkspaceDir: workspace, Platform: "react-native", ProviderKey: "codex", Force: true,
	})
	if err != nil {
		t.Fatalf("forced Dispatch() error = %v", err)
	}
	if forced.Status != ScaffoldStatusDone {
		t.Fatalf("forced dispatch = %+v, want done", forced)
	}
	if executor.count() != 2 {
		t.Fatalf("executor calls = %d, want 2 after Force", executor.count())
	}
}

func TestScaffoldDispatcher_NoGateCommandIsDoneWithoutCompiler(t *testing.T) {
	executor := &recordingScaffoldExecutor{}
	dispatcher := dispatcherForRecipe(executor, realScaffoldRecipe(t, ""))
	workspace := newScaffoldWorkspace(t)

	result, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		WorkspaceDir: workspace, Platform: "react-native", ProviderKey: "codex",
	})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if result.Status != ScaffoldStatusDone || !strings.Contains(result.Message, "no verification gate configured") {
		t.Fatalf("result = %+v, want done/no gate", result)
	}
	if result.CompilerGate != nil {
		t.Fatalf("CompilerGate = %+v, want nil when no gate command exists", result.CompilerGate)
	}
}

func TestScaffoldDispatcher_RequiresWorkingDirectory(t *testing.T) {
	dispatcher := dispatcherForRecipe(&recordingScaffoldExecutor{}, realScaffoldRecipe(t, "true"))
	result, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{Platform: "react-native"})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if result.Status != ScaffoldStatusError || !strings.Contains(result.Message, "working directory is required") {
		t.Fatalf("result = %+v, want error/working directory required", result)
	}
}

func TestScaffoldDispatcher_MissingPromptTemplateIsAnError(t *testing.T) {
	// A verified recipe whose prompt template is absent is a pack defect: surface
	// it as an error rather than silently dispatching an empty prompt.
	dispatcher := &ScaffoldDispatcher{
		executor: &recordingScaffoldExecutor{},
		loadRecipe: func(string) (*skillpack.ScaffoldRecipe, bool, error) {
			return realScaffoldRecipe(t, "true"), true, nil
		},
		loadPrompt: func(string) (string, bool, error) { return "", false, nil },
	}
	result, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		WorkspaceDir: t.TempDir(), Platform: "react-native", ProviderKey: "codex",
	})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if result.Status != ScaffoldStatusError || !strings.Contains(result.Message, "missing from the pack") {
		t.Fatalf("result = %+v, want error/missing template", result)
	}
}

func TestNewScaffoldDispatcher_ToleratesNilRunner(t *testing.T) {
	dispatcher := NewScaffoldDispatcher(nil)
	if dispatcher == nil {
		t.Fatal("NewScaffoldDispatcher(nil) = nil, want a usable dispatcher")
	}
	if dispatcher.executor != nil {
		t.Fatalf("executor = %v, want nil for a nil runner", dispatcher.executor)
	}
	result, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		WorkspaceDir: t.TempDir(), Platform: "react-native", ProviderKey: "codex",
	})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if result.Status != ScaffoldStatusError || !strings.Contains(result.Message, "runner is unavailable") {
		t.Fatalf("result = %+v, want error/runner unavailable", result)
	}
}
func TestScaffoldDispatcher_RefusesScaffoldOutsideWorkspaceBoundary(t *testing.T) {
	// Path-traversal guard: a workspace that escapes the runner's visible tree via
	// ".." must be rejected before any prompt or AI call — a recipe name can never
	// authorize writing somewhere the caller did not explicitly point at.
	executor := &recordingScaffoldExecutor{}
	dispatcher := dispatcherForRecipe(executor, realScaffoldRecipe(t, "true"))

	result, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		ProjectID:    "proj-rn",
		WorkspaceDir: filepath.Join("..", "..", "outside-runner-root"),
		Platform:     "react-native",
		ProviderKey:  "claude",
	})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if result.Status != ScaffoldStatusError {
		t.Fatalf("Status = %q, want error for an escaping workspace", result.Status)
	}
	if !strings.Contains(result.Message, "outside the runner workspace boundary") {
		t.Fatalf("Message = %q, want the boundary rejection explained", result.Message)
	}
	if executor.count() != 0 {
		t.Fatalf("executor calls = %d, want 0 — reject before any AI call", executor.count())
	}
}

func TestScaffoldDispatcher_AllowsWorkspaceBoundaryAndSubdirectories(t *testing.T) {
	// Near-miss guard against an over-strict boundary: the runner root itself and
	// any directory beneath an explicit visible root must still scaffold.
	for _, tc := range []struct {
		name       string
		workspace  func(root string) string
		useTempDir bool
	}{
		{name: "root itself", workspace: func(root string) string { return root }},
		{name: "nested child", workspace: func(root string) string {
			return filepath.Join(root, "apps", "mobile")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executor := &recordingScaffoldExecutor{}
			dispatcher := dispatcherForRecipe(executor, realScaffoldRecipe(t, "true"))
			root := t.TempDir()
			workspace := tc.workspace(root)
			if err := os.MkdirAll(workspace, 0o755); err != nil {
				t.Fatal(err)
			}
			result, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
				ProjectID:    "proj-rn",
				WorkspaceDir: workspace,
				Platform:     "react-native",
				ProviderKey:  "claude",
			})
			if err != nil {
				t.Fatalf("Dispatch() error = %v", err)
			}
			if result.Status != ScaffoldStatusDone {
				t.Fatalf("Status = %q (%s), want done inside the boundary", result.Status, result.Message)
			}
		})
	}
}

func TestScaffoldDispatcher_RelativeTraversalIsScopedToRequestDir(t *testing.T) {
	// Relative workspaces are resolved against the caller's cwd (no chdir games
	// survive filepath.Abs), and a gate that points at a nonexistent directory
	// fails closed as an environment error instead of a healable compile error.
	executor := &recordingScaffoldExecutor{}
	dispatcher := dispatcherForRecipe(executor, realScaffoldRecipe(t, "true"))

	result, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		ProjectID:    "proj-rn",
		WorkspaceDir: filepath.Join("somewhere", "..", "else"),
		Platform:     "react-native",
		ProviderKey:  "claude",
	})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	// "somewhere/../else" cleans to a plain sub-path of cwd, so the dispatch may
	// proceed; the gate must then fail closed on the missing directory.
	if result.Status == ScaffoldStatusDone {
		t.Fatalf("result = %+v, want no success for a nonexistent workspace", result)
	}
	if result.CompilerGate != nil && !result.CompilerGate.Passed && result.CompilerGate.EnvError == "" {
		t.Fatalf("CompilerGate = %+v, want the chdir failure marked as an environment error", result.CompilerGate)
	}
}

func TestScaffoldDispatcher_RejectsNonexistentAbsoluteWorkspace(t *testing.T) {
	// CP-68 scaffold review (S2): a workspace that does not exist (or is not a
	// directory) is rejected after path resolution and BEFORE any recipe/replay
	// check or AI call — a missing dir can never be scaffolded.
	executor := &recordingScaffoldExecutor{}
	dispatcher := dispatcherForRecipe(executor, realScaffoldRecipe(t, "true"))
	workspace := filepath.Join(t.TempDir(), "missing-workspace")

	result, err := dispatcher.Dispatch(context.Background(), ScaffoldRequest{
		ProjectID:    "proj-rn",
		WorkspaceDir: workspace,
		Platform:     "react-native",
		ProviderKey:  "claude",
	})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if result.Status != ScaffoldStatusError {
		t.Fatalf("Status = %q (%s), want error for a nonexistent workspace", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "not a directory") || !strings.Contains(result.Message, workspace) {
		t.Fatalf("Message = %q, want the not-a-directory rejection naming %q", result.Message, workspace)
	}
	if executor.count() != 0 {
		t.Fatalf("executor calls = %d, want 0 — reject before any AI call", executor.count())
	}
}
