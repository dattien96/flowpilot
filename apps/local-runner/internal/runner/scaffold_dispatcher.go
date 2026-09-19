package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"flowpilot-runner/internal/skillpack"
)

// Scaffold Turn status values (Task-384 T-3).
const (
	ScaffoldStatusDone    = "done"
	ScaffoldStatusSkipped = "skipped"
	ScaffoldStatusError   = "error"
)

// scaffoldSkippedNoRecipe is the exact log/UI line the operator specified for the
// graceful-ignore branch (CP-68 P-3 T-4): not capable ⇒ static init still
// succeeds and the scaffold branch is silently dropped.
const scaffoldSkippedNoRecipe = "scaffold: skipped (no verified recipe)"

const (
	// scaffoldPromptRelPath is the embedded Zero-Prompt Injection template.
	scaffoldPromptRelPath = "prompts/scaffold-step0-bootstrap.md"
	// scaffoldTurnTimeoutMs is the budget for one scaffold AI turn. Deliberately
	// longer than the TUI client's 5-minute engine-init timeout: generating a
	// monorepo skeleton takes far longer than copying skills.
	scaffoldTurnTimeoutMs = 30 * 60 * 1000
	// scaffoldDefaultGateCap mirrors the CP-68 recipe default (cap: 3).
	scaffoldDefaultGateCap = 3
	// maxCompilerFeedbackBytes caps the raw log echoed into the healing prompt.
	maxCompilerFeedbackBytes = 8000
	// scaffoldStatusFileName is the CP-68 §6 replay guard.
	scaffoldStatusFileName = "scaffold-status.json"
)

// ScaffoldPromptExecutor is the seam between the dispatcher and the AI runtime.
// *Runner satisfies it via ExecutePrompt (one-shot, synchronous, provider-agnostic
// across claude/codex/gemini/grok/opencode). Tests inject a recording fake so no
// real LLM is ever invoked.
type ScaffoldPromptExecutor interface {
	ExecutePrompt(ctx context.Context, request PromptExecutionRequest) (PromptExecutionResult, error)
}

// ScaffoldRequest is one "build Step 0 for this project" request.
type ScaffoldRequest struct {
	ProjectID       string
	WorkspaceDir    string
	Platform        string
	ProviderKey     string
	ModelName       string
	ReasoningEffort string
	// YoloMode is accepted for API symmetry and logged, but the scaffold turn is
	// always write-enabled regardless (see Dispatch).
	YoloMode bool
	// Force re-runs the scaffold even when a completed scaffold-status.json
	// already exists for this platform.
	Force bool
	// Trigger records who asked ("manual" | "init_all" | "create_project").
	Trigger string
}

// ScaffoldDispatchResult is the API/UI-facing outcome of a dispatch.
type ScaffoldDispatchResult struct {
	Status         string              `json:"status"`
	Platform       string              `json:"platform"`
	SkillsAttached []string            `json:"skillsAttached"`
	Message        string              `json:"message"`
	RunID          string              `json:"runId,omitempty"`
	ProviderKey    string              `json:"providerKey,omitempty"`
	Attempts       int                 `json:"attempts,omitempty"`
	CompilerGate   *CompilerGateResult `json:"compilerGate,omitempty"`
	StatusPath     string              `json:"statusPath,omitempty"`
}

// ScaffoldStatusFile is the on-disk replay guard (CP-68 §6): once a scaffold has
// passed the compiler gate it is never re-run for the same platform unless the
// caller forces it.
type ScaffoldStatusFile struct {
	Status              string   `json:"status"`
	Platform            string   `json:"platform"`
	ProjectID           string   `json:"projectId,omitempty"`
	CompletedAt         string   `json:"completedAt"`
	SkillsAttached      []string `json:"skillsAttached"`
	VerificationCommand string   `json:"verificationCommand,omitempty"`
	Attempts            int      `json:"attempts,omitempty"`
}

// ScaffoldDispatcher orchestrates the AI Scaffold Turn: recipe discovery →
// context profile → AI turn → Compiler Verification Gate self-healing loop.
//
// Every collaborator is injectable so unit tests exercise the full matrix
// (capable / not capable / heal / cap exceeded) without touching a real provider.
type ScaffoldDispatcher struct {
	executor   ScaffoldPromptExecutor
	loadRecipe func(platform string) (*skillpack.ScaffoldRecipe, bool, error)
	loadPrompt func(relPath string) (string, bool, error)
}

// NewScaffoldDispatcher returns the production dispatcher backed by the live
// runner. A nil runner is tolerated: Dispatch degrades to an error result rather
// than panicking.
func NewScaffoldDispatcher(r *Runner) *ScaffoldDispatcher {
	var executor ScaffoldPromptExecutor
	if r != nil {
		executor = r
	}
	return &ScaffoldDispatcher{
		executor:   executor,
		loadRecipe: skillpack.LoadScaffoldRecipe,
		loadPrompt: loadBuiltinPromptText,
	}
}

// Dispatch runs the full scaffold orchestration. It never returns a transport
// error for a skipped or failed scaffold: the outcome is always encoded in the
// returned result's Status/Message so TUI, Desktop and HTTP callers render the
// same truth. The error return is reserved for future invalid-argument cases.
func (d *ScaffoldDispatcher) Dispatch(ctx context.Context, req ScaffoldRequest) (*ScaffoldDispatchResult, error) {
	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	result := &ScaffoldDispatchResult{
		Status:         ScaffoldStatusSkipped,
		Platform:       platform,
		SkillsAttached: []string{},
	}

	workspace := strings.TrimSpace(req.WorkspaceDir)
	if workspace == "" {
		result.Status = ScaffoldStatusError
		result.Message = "scaffold: working directory is required"
		return result, nil
	}
	resolvedWorkspace, absErr := filepath.Abs(workspace)
	if absErr != nil {
		result.Status = ScaffoldStatusError
		result.Message = fmt.Sprintf("scaffold: resolve working directory: %v", absErr)
		return result, nil
	}
	workspace = resolvedWorkspace
	if !filepath.IsAbs(strings.TrimSpace(req.WorkspaceDir)) && escapesProcessCwd(workspace) {
		result.Status = ScaffoldStatusError
		result.Message = "scaffold: working directory is outside the runner workspace boundary"
		return result, nil
	}
	// Existence check: a workspace that does not exist (or is a file) can never
	// be scaffolded — reject before the recipe/replay checks and any AI call.
	if info, statErr := os.Stat(workspace); statErr != nil || !info.IsDir() {
		result.Status = ScaffoldStatusError
		result.Message = fmt.Sprintf("scaffold: working directory is not a directory: %s", workspace)
		return result, nil
	}

	// Tier 1 + Tier 2 (Task-383): recipe discovery and skill integrity. Anything
	// short of "fully verified recipe" means graceful ignore — zero AI calls, and
	// the static CP-34 init that already ran stays untouched.
	recipe, found, _ := d.loadRecipe(platform)
	if !found || recipe == nil {
		result.Message = scaffoldSkippedNoRecipe
		return result, nil
	}
	if !recipe.Enabled {
		result.Message = "scaffold: skipped (recipe disabled)"
		return result, nil
	}
	if recipePlatform := strings.TrimSpace(recipe.Platform); recipePlatform != "" {
		result.Platform = recipePlatform
	}
	missing, skillsOK := skillpack.VerifyRecipeSkills(recipe)
	if !skillsOK {
		result.Message = fmt.Sprintf("scaffold: skipped (recipe skills missing: %s)", strings.Join(missing, ", "))
		return result, nil
	}
	result.SkillsAttached = append([]string(nil), recipe.ScaffoldSkills...)

	// CP-68 §6 replay guard: a previously completed scaffold for the same platform
	// is not re-run (the generated files are the user's now, not ours).
	if !req.Force {
		if existing, ok := LoadScaffoldStatusFile(workspace); ok &&
			existing.Status == ScaffoldStatusDone &&
			strings.EqualFold(existing.Platform, result.Platform) {
			result.Message = "scaffold: skipped (already scaffolded)"
			result.StatusPath = ScaffoldStatusPath(workspace)
			return result, nil
		}
	}

	gateCommand := strings.TrimSpace(recipe.VerificationGate.Command)
	prompt, promptErr := d.scaffoldPrompt(recipe, workspace)
	if promptErr != nil {
		result.Status = ScaffoldStatusError
		result.Message = fmt.Sprintf("scaffold: %v", promptErr)
		return result, nil
	}

	if d.executor == nil {
		result.Status = ScaffoldStatusError
		result.Message = "scaffold: runner is unavailable"
		return result, nil
	}

	turn, turnErr := d.executeTurn(ctx, req, workspace, recipe, prompt)
	result.RunID = turn.RunID
	if providerKey := strings.TrimSpace(turn.ProviderKey); providerKey != "" {
		result.ProviderKey = providerKey
	} else {
		result.ProviderKey = strings.TrimSpace(req.ProviderKey)
	}
	if failure := scaffoldTurnFailure(turn, turnErr); failure != nil {
		result.Status = ScaffoldStatusError
		result.Message = fmt.Sprintf("scaffold: AI scaffold turn failed: %v", failure)
		return result, nil
	}

	if gateCommand == "" {
		// No verified gate configured for this platform: honour the scaffold but
		// say out loud that no compiler verified it.
		result.Status = ScaffoldStatusDone
		result.Message = "scaffold: done (no verification gate configured)"
		result.Attempts = 1
		d.persistStatus(workspace, req, recipe, result)
		return result, nil
	}

	return d.runCompilerGateLoop(ctx, req, workspace, recipe, gateCommand, result)
}

// runCompilerGateLoop is the Task-386 T-2 self-healing loop: run the gate, and on
// a *fixable* failure hand the parsed diagnostics back to the AI (same provider,
// same attached skills) for at most recipe.VerificationGate.Cap attempts.
//
// Fail-closed: the result only reaches "done" when the gate exits 0. A timeout or
// a gate that cannot start is reported immediately instead of burning AI turns on
// something the model cannot repair.
func (d *ScaffoldDispatcher) runCompilerGateLoop(
	ctx context.Context,
	req ScaffoldRequest,
	workspace string,
	recipe *skillpack.ScaffoldRecipe,
	gateCommand string,
	result *ScaffoldDispatchResult,
) (*ScaffoldDispatchResult, error) {
	attemptCap := recipe.VerificationGate.Cap
	if attemptCap <= 0 {
		attemptCap = scaffoldDefaultGateCap
	}
	gateTimeout := time.Duration(recipe.VerificationGate.TimeoutSeconds) * time.Second
	gate := NewCompilerGate(workspace, gateCommand, gateTimeout)

	for attempt := 1; ; attempt++ {
		gateResult, gateErr := gate.Run(ctx)
		if gateErr != nil {
			result.Status = ScaffoldStatusError
			result.Message = fmt.Sprintf("scaffold: compiler gate unavailable: %v", gateErr)
			return result, nil
		}
		result.CompilerGate = gateResult
		result.Attempts = attempt

		if gateResult.Passed {
			result.Status = ScaffoldStatusDone
			result.Message = fmt.Sprintf("scaffold: done — compiler gate PASS (%s)", gateCommand)
			d.persistStatus(workspace, req, recipe, result)
			return result, nil
		}

		if gateResult.TimedOut || gateResult.EnvError != "" {
			result.Status = ScaffoldStatusError
			result.Message = fmt.Sprintf(
				"scaffold: compiler gate could not complete — %s\nFull log:\n%s",
				firstCompilerError(gateResult),
				truncateRunes(gateResult.RawOutput, maxCompilerFeedbackBytes),
			)
			return result, nil
		}

		if attempt >= attemptCap {
			result.Status = ScaffoldStatusError
			result.Message = fmt.Sprintf(
				"scaffold: compiler gate FAILED after %d attempt(s) — stopping for human intervention.\nFull log:\n%s",
				attempt,
				truncateRunes(gateResult.RawOutput, maxCompilerFeedbackBytes),
			)
			return result, nil
		}

		// Heal: hand the structured diagnostics back to the same provider.
		feedback := buildCompilerFeedbackPrompt(gateCommand, gateResult)
		repairTurn, repairErr := d.executeTurn(ctx, req, workspace, recipe, feedback)
		if repairTurn.RunID != "" {
			result.RunID = repairTurn.RunID
		}
		if failure := scaffoldTurnFailure(repairTurn, repairErr); failure != nil {
			result.Status = ScaffoldStatusError
			result.Message = fmt.Sprintf(
				"scaffold: AI repair turn failed on attempt %d: %v\nCompiler log:\n%s",
				attempt, failure, truncateRunes(gateResult.RawOutput, maxCompilerFeedbackBytes),
			)
			return result, nil
		}
	}
}

// executeTurn sends one scaffold-side turn. Scaffold turns are always
// write-enabled (generating Step 0 IS writing code) and YOLO: the provider's own
// permission layer is bypassed via resolveYoloPosture(true), because the runner
// has already gated the turn behind a verified recipe.
//
// SkillIds carries the recipe's blueprint skills; ExecutePrompt turns those into
// the "Context Profile" block the model must obey (injectSkillContent → the
// skills installed into <workspace>/.agents/skills by the init step).
func (d *ScaffoldDispatcher) executeTurn(
	ctx context.Context,
	req ScaffoldRequest,
	workspace string,
	recipe *skillpack.ScaffoldRecipe,
	prompt string,
) (PromptExecutionResult, error) {
	return d.executor.ExecutePrompt(ctx, PromptExecutionRequest{
		ProviderKey:      strings.TrimSpace(req.ProviderKey),
		ModelName:        strings.TrimSpace(req.ModelName),
		ReasoningEffort:  strings.TrimSpace(req.ReasoningEffort),
		Prompt:           prompt,
		SkillIds:         append([]string(nil), recipe.ScaffoldSkills...),
		WorkingDirectory: workspace,
		AllowWrite:       true,
		YoloMode:         true,
		TimeoutMs:        scaffoldTurnTimeoutMs,
	})
}

// scaffoldTurnFailure collapses the many ways a one-shot AI turn can fail into a
// single non-nil error, or nil when the turn genuinely succeeded.
func scaffoldTurnFailure(result PromptExecutionResult, err error) error {
	if err != nil {
		return fmt.Errorf("%w (provider=%s)", err, result.ProviderKey)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("exit code %d: %s", result.ExitCode, firstNonEmptyLineOf(result.StderrSummary, result.ErrorMessage, result.StdoutSummary))
	}
	if msg := strings.TrimSpace(result.ErrorMessage); msg != "" {
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// buildCompilerFeedbackPrompt is Task-386 T-2's structured feedback: the header
// the operator specified plus the parsed file:line diagnostics, with the raw log
// only as a bounded fallback.
func buildCompilerFeedbackPrompt(gateCommand string, gateResult *CompilerGateResult) string {
	var sb strings.Builder
	sb.WriteString("[Compiler Gate Thất Bại] Trình biên dịch trả về lỗi sau. Hãy sửa mã nguồn để thỏa mãn compiler:\n\n")
	fmt.Fprintf(&sb, "Lệnh kiểm định: %s\nExit code: %d\n\n", gateCommand, gateResult.ExitCode)
	sb.WriteString("--- Lỗi đã trích xuất ---\n")
	if len(gateResult.ParsedErrors) > 0 {
		for _, line := range gateResult.ParsedErrors {
			sb.WriteString("- ")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("(không trích xuất được dòng lỗi có cấu trúc — xem log thô bên dưới)\n")
	}
	sb.WriteString("\n--- Log thô (rút gọn) ---\n")
	sb.WriteString(truncateRunes(gateResult.RawOutput, maxCompilerFeedbackBytes))
	sb.WriteString("\n\nChỉ sửa những gì compiler báo. Không refactor ngoài phạm vi lỗi. Khi xong, dừng lại.")
	return sb.String()
}

// firstCompilerError returns the single most useful diagnostic line.
func firstCompilerError(gateResult *CompilerGateResult) string {
	if gateResult == nil {
		return "no compiler output"
	}
	if len(gateResult.ParsedErrors) > 0 {
		return gateResult.ParsedErrors[0]
	}
	return fmt.Sprintf("exit code %d with no diagnostics", gateResult.ExitCode)
}

// firstNonEmptyLineOf returns the first non-empty trimmed candidate.
func firstNonEmptyLineOf(candidates ...string) string {
	for _, candidate := range candidates {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
		}
	}
	return "no output"
}

// scaffoldPromptData is the Zero-Prompt Injection view model: the operator-supplied
// header plus everything the model needs to bootstrap Step 0 without a single
// keystroke from the user.
type scaffoldPromptData struct {
	Platform            string
	Workspace           string
	Skills              []string
	SkillsCSV           string
	SkillsBullets       string
	VerificationCommand string
}

// scaffoldPrompt loads the embedded template and renders it for this workspace.
// A pack without the template is an error (not a silent skip): the capability
// check already promised a verified recipe, so failing to build its prompt is a
// real defect worth surfacing.
func (d *ScaffoldDispatcher) scaffoldPrompt(recipe *skillpack.ScaffoldRecipe, workspace string) (string, error) {
	loader := d.loadPrompt
	if loader == nil {
		loader = loadBuiltinPromptText
	}
	tmpl, found, err := loader(scaffoldPromptRelPath)
	if err != nil {
		return "", fmt.Errorf("load scaffold prompt template: %w", err)
	}
	if !found || strings.TrimSpace(tmpl) == "" {
		return "", fmt.Errorf("scaffold prompt template %s is missing from the pack", scaffoldPromptRelPath)
	}

	data := scaffoldPromptData{
		Platform:            strings.TrimSpace(recipe.Platform),
		Workspace:           workspace,
		Skills:              append([]string(nil), recipe.ScaffoldSkills...),
		SkillsCSV:           strings.Join(recipe.ScaffoldSkills, ", "),
		SkillsBullets:       "- " + strings.Join(recipe.ScaffoldSkills, "\n- "),
		VerificationCommand: strings.TrimSpace(recipe.VerificationGate.Command),
	}

	parsed, err := template.New("scaffold-step0-bootstrap").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse scaffold prompt template: %w", err)
	}
	var buf bytes.Buffer
	if err := parsed.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render scaffold prompt template: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// ScaffoldStatusPath is the absolute path of the replay-guard file for a workspace.
func ScaffoldStatusPath(workspace string) string {
	return filepath.Join(strings.TrimSpace(workspace), ".flowpilot", scaffoldStatusFileName)
}

// LoadScaffoldStatusFile reads .flowpilot/scaffold-status.json. A missing or
// unreadable file is reported as not-found, never as an error: the file is an
// optimization, not a contract.
func LoadScaffoldStatusFile(workspace string) (*ScaffoldStatusFile, bool) {
	if strings.TrimSpace(workspace) == "" {
		return nil, false
	}
	raw, err := os.ReadFile(ScaffoldStatusPath(workspace))
	if err != nil {
		return nil, false
	}
	var status ScaffoldStatusFile
	if err := json.Unmarshal(raw, &status); err != nil {
		return nil, false
	}
	return &status, true
}

// persistStatus writes the replay guard after a passing gate. Best-effort: a
// write failure is appended to the message but never downgrades a PASS.
func (d *ScaffoldDispatcher) persistStatus(
	workspace string,
	req ScaffoldRequest,
	recipe *skillpack.ScaffoldRecipe,
	result *ScaffoldDispatchResult,
) {
	path := ScaffoldStatusPath(workspace)
	result.StatusPath = path

	status := ScaffoldStatusFile{
		Status:              ScaffoldStatusDone,
		Platform:            strings.TrimSpace(recipe.Platform),
		ProjectID:           strings.TrimSpace(req.ProjectID),
		CompletedAt:         time.Now().UTC().Format(time.RFC3339),
		SkillsAttached:      append([]string(nil), recipe.ScaffoldSkills...),
		VerificationCommand: strings.TrimSpace(recipe.VerificationGate.Command),
		Attempts:            result.Attempts,
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		result.Message += fmt.Sprintf(" (warning: cannot create status dir: %v)", err)
		return
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		result.Message += fmt.Sprintf(" (warning: cannot encode scaffold status: %v)", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		result.Message += fmt.Sprintf(" (warning: cannot write %s: %v)", path, err)
	}
}
