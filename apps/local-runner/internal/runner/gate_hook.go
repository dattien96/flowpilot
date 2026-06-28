package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
	"flowpilot-runner/internal/flowgate"
)

const maxFlowGateReprompts = 2

// gateBlockInfo carries r-reg details for the decision handler (Task-155).
type gateBlockInfo struct {
	regressedTests []string
	stepID         string
}

// runFlowGate is the post-turn enforcement hook (CP-35 P-4/P-5). It is called
// after turnInFlight=false and before finalizer.Finalize on a clean completion.
//
// It observes the git diff and test outcomes, evaluates the flow rules, and
// either allows the turn to finalise, reprompts the AI with the missing
// requirement, or blocks the step from completing.
//
// All errors inside this function are non-fatal: if the gate cannot observe or
// evaluate it returns false (degraded = safe, turn completes normally).
func (s *InteractiveService) runFlowGate(
	ctx context.Context, rs *interactiveRun, turnID string, fin finalizeInput,
) (block bool) {
	cwd := rs.workspaceCwd
	if cwd == "" {
		return false
	}
	dotFP := filepath.Join(cwd, ".flowpilot")

	// 1. Observe git diff — include commits made during the turn (CP-35).
	s.mu.Lock()
	baseSHA := rs.turnStartGitHead
	s.mu.Unlock()
	diff, _ := flowgate.ObserveGitDiffSince(cwd, baseSHA)
	log.Printf("[gate] cwd=%q baseSHA=%q diffLen=%d diff=%+v", cwd, baseSHA, len(diff), diff)
	changedPaths := changedPathsFromDiff(diff)
	commitSubjects := collectCommitSubjectsSince(cwd, baseSHA)
	knownFeatureKeys := loadKnownFeatureKeys(cwd)
	suggestedFeatureKeys := suggestFeatureKeys(dotFP, changedPaths, strings.Join(commitSubjects, "\n"))

	// 2. Load test baseline — non-fatal.
	baseline, _ := flowgate.LoadBaseline(dotFP)

	// 3. Load per-test overrides (Task-155): agreed-changed tests are not re-counted.
	overrides, _ := flowgate.LoadOverrides(dotFP)

	if blocked, _ := s.maybeRunFlowValidation(ctx, rs, fin, dotFP); blocked {
		return true
	}

	// 4. Run regression oracle with override awareness.
	oracle := flowgate.RunOracle(cwd, baseline, diff, overrides)

	// 5. Clear overrides for tests that are now green (sticky-until-green, DOD-07).
	// oracle.Passed is populated by RunOracle from the same suite run — no second execution needed.
	if !oracle.HasRegression && len(oracle.Passed) > 0 {
		_ = flowgate.ClearOverrideIfGreen(dotFP, oracle.Passed)
	}

	// 6. Build TurnResult for the evaluator.
	var failedTests []string
	if oracle.HasRegression {
		failedTests = oracle.Regressed
	}
	tr := flowgate.TurnResult{
		RunID:                rs.id,
		StepID:               rs.stepID,
		FinalMessage:         fin.FinalMessage,
		SourceDocID:          rs.sourceDocID,
		GitDiff:              diff,
		CommitSubjects:       commitSubjects,
		ChangedPaths:         changedPaths,
		KnownFeatureKeys:     knownFeatureKeys,
		SuggestedFeatureKeys: suggestedFeatureKeys,
		WrittenPaths:         fin.ChangedFiles, // files actually written by AI tool calls this turn
		Tests: flowgate.TestOutcome{
			Ran:    baseline != nil,
			Failed: failedTests,
		},
		ChangeType: rs.changeType,
	}

	// 7. Load rules; fall back to defaults when flow-rules.json is absent.
	rules := flowgate.DefaultRules()
	if loaded, err := flowgate.LoadRules(filepath.Join(dotFP, "settings")); err == nil {
		rules = loaded
	}

	// 8. Evaluate rule set.
	violations := flowgate.Evaluate(tr, rules)
	hasCA := flowgate.HasChangeAuditNote(diff)
	hasCode := flowgate.HasCodeChanges(diff)
	log.Printf("[gate] violations=%d gateMode=%q hasCode=%v hasCA=%v", len(violations), loadGateMode(dotFP), hasCode, hasCA)

	// 8a. Proposal-turn exemption (Task-155 opt-2): the AI just proposed a requirement
	// change and has not fixed code yet; tests are expected to still fail. Suppress
	// r-reg and r-tests for this single turn so the modal does not re-appear.
	s.mu.Lock()
	proposalTurn := rs.proposalTurnPending
	rs.proposalTurnPending = false
	s.mu.Unlock()
	if proposalTurn {
		filtered := violations[:0]
		for _, v := range violations {
			if v.Rule.ID != "r-reg" && v.Rule.ID != "r-tests" {
				filtered = append(filtered, v)
			}
		}
		violations = filtered
		log.Printf("[gate] proposal turn: r-reg/r-tests suppressed")
	}

	// 9. Surface oracle-detected tampering as an additional warn violation.
	if oracle.HasTampering {
		violations = append(violations, flowgate.Violation{
			Rule: flowgate.Rule{
				ID:      "r-tamper",
				Scope:   "step",
				Trigger: "oracle_tamper",
				Action:  "warn",
				Enabled: true,
			},
			Detail: "pre-existing test file modified: " + strings.Join(oracle.Tampered, ", "),
		})
	}

	if len(violations) == 0 {
		return false
	}

	// 10. Enforce — read gate_mode from .flowpilot/settings/gate-config.json; default warn.
	result := flowgate.Enforce(violations, loadGateMode(dotFP))

	// 11. Extract r-reg options for the decision card (Task-155).
	var gateOptions []string
	var gateRegressedTests []string
	for _, v := range violations {
		if v.Rule.ID == "r-reg" && len(v.Options) > 0 {
			gateOptions = v.Options
			gateRegressedTests = v.RegressedTests
			break
		}
	}

	// 12. Emit the violation event so the desktop can surface it inline.
	s.mu.Lock()
	if result.Action == "block" && len(gateOptions) > 0 {
		rs.pendingGateBlock = &gateBlockInfo{
			regressedTests: gateRegressedTests,
			stepID:         rs.lastTurnStepID,
		}
	}
	s.emitLocked(rs, ProviderEvent{
		Type:               EventFlowGateViolation,
		ProviderTurnID:     turnID,
		Error:              result.Message,
		Status:             result.Action,
		GateOptions:        gateOptions,
		GateRegressedTests: gateRegressedTests,
	})
	s.mu.Unlock()

	switch result.Action {
	case "block":
		return true

	case "reprompt":
		s.mu.Lock()
		attempts := rs.repromptAttempts
		rs.repromptAttempts++
		stepID := rs.lastTurnStepID
		s.mu.Unlock()
		log.Printf("[gate] reprompt attempt=%d stepID=%q", attempts, stepID)
		if attempts < maxFlowGateReprompts {
			go func(runID, stepID, prompt string) {
				_, _ = s.startTurn(runID, TurnInput{StepID: stepID, Prompt: prompt}, "", "")
			}(rs.id, stepID, flowgate.RepromptPrompt(result))
		}
		return true
	}

	// "warn" or "approve": log only, let the turn complete normally.
	return false
}

func (s *InteractiveService) maybeRunFlowValidation(ctx context.Context, rs *interactiveRun, fin finalizeInput, dotFP string) (blocked bool, handled bool) {
	if rs == nil {
		return false, false
	}
	step, ok := s.workflowStepByID(rs.id, rs.stepID)
	if !ok || !strings.EqualFold(step.StepType, "testing") {
		return false, false
	}
	cfg, err := loadFlowModeConfig(dotFP)
	if err != nil {
		log.Printf("[flow-validation] config load failed run=%q step=%q err=%v", rs.id, rs.stepID, err)
		return false, true
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.ValidationCommand == "" {
		result := FlowValidationResult{
			WorkflowRunID:        rs.id,
			TestingStepRunID:     rs.stepID,
			CodingStepRunID:      "",
			ValidationCommand:    "",
			ValidationWorkingDir: cfg.ValidationWorkingDirectory,
			Status:               "skipped_no_command",
			ExitCode:             0,
			FailureSummary:       "Testing validation command is not configured.",
		}
		if s.runner != nil {
			_, _ = s.runner.SaveFlowValidationArtifact(rs.projectID, result)
		}
		return false, true
	}
	wrkDir := cfg.ValidationWorkingDirectory
	if wrkDir == "" {
		wrkDir = rs.workspaceCwd
	}
	stdout, stderr, exitCode, runErr := runValidationCommand(ctx, wrkDir, cfg.ValidationCommand)
	passed := runErr == nil && exitCode == 0
	failureSummary := ""
	if !passed {
		failureSummary = summarizeFlowValidationOutput(cfg.ValidationCommand, stdout, stderr, exitCode)
	}
	codingStep, hasCoding := s.latestCompletedStepByType(rs.id, "coding")
	testingStepRetryCount := 0
	if currentStep, ok := s.workflowStepByID(rs.id, rs.stepID); ok {
		testingStepRetryCount = currentStep.RetryCount
	}
	result := FlowValidationResult{
		WorkflowRunID:        rs.id,
		TestingStepRunID:     rs.stepID,
		CodingStepRunID:      codingStep.ID,
		ValidationCommand:    cfg.ValidationCommand,
		ValidationWorkingDir: wrkDir,
		Status:               "passed",
		ExitCode:             exitCode,
		StdoutSummary:        stdout,
		StderrSummary:        stderr,
		FailureSummary:       failureSummary,
		MaxRetries:           cfg.MaxRetries,
		PreviousCodingTurnID: rs.lastTurnID,
	}
	if !passed {
		result.Status = "failed"
	}
	if s.runner != nil {
		if _, saveErr := s.runner.SaveFlowValidationArtifact(rs.projectID, result); saveErr != nil {
			log.Printf("[flow-validation] artifact save failed run=%q step=%q err=%v", rs.id, rs.stepID, saveErr)
		}
	}
	if passed || !hasCoding {
		if passed && s.runner != nil {
			if auditStep, ok := s.firstStepByType(rs.id, "audit"); ok {
				pkg, pkgOK := s.loadLatestFlowContextPackage(rs.id)
				if pkgOK {
					draft := BuildFlowAuditDraft(rs.workspaceCwd, pkg, codingStep.ID, rs.stepID, auditStep.ID, s.loadPersistedCodingChangedFiles(rs.id, rs.lastTurnID), []string{cfg.ValidationCommand}, nil, "Validation passed.", "feature", truncateDisplayField(fin.FinalMessage, 200))
					_, _ = s.runner.SaveFlowAuditDraftArtifact(rs.projectID, draft, auditStep.ID)
				}
			}
		}
		return false, true
	}
	if isValidationEnvironmentFailure(exitCode, stderr, runErr) {
		result.Status = "environment_error"
		if s.runner != nil {
			_, _ = s.runner.SaveFlowValidationArtifact(rs.projectID, result)
		}
		return false, true
	}
	if testingStepRetryCount >= cfg.MaxRetries {
		result.Status = "failed_validation_max_retries"
		if s.runner != nil {
			_, _ = s.runner.SaveFlowValidationArtifact(rs.projectID, result)
		}
		return false, true
	}

	planPkg, pkgOK := s.loadLatestFlowContextPackage(rs.id)
	if !pkgOK {
		planPkg = FlowContextPackage{WorkflowRunID: rs.id, PlanStepRunID: codingStep.ID, FeatureKey: "", FeatureConfidence: FlowContextConfidenceMissing, Warnings: []string{"missing original plan package"}}
	}
	result.RetryAttempt = testingStepRetryCount + 1
	result.OriginalPlanPackageID = planPkg.PackageID
	if s.runner != nil {
		_, _ = s.runner.SaveFlowValidationArtifact(rs.projectID, result)
	}
	prompt := buildValidationRetryPrompt(planPkg, result)
	transition := PlanRejectedStepRetry(rs.stepID, testingStepRetryCount, time.Now().UTC().Format(time.RFC3339Nano), result.FailureSummary)
	if err := s.workflowStore.ApplyStepTransition(ctx, rs.id, transition); err != nil {
		log.Printf("[flow-validation] retry transition failed run=%q testingStep=%q err=%v", rs.id, rs.stepID, err)
		return false, true
	}
	go func(runID, stepID, prompt string) {
		_, _ = s.startTurn(runID, TurnInput{StepID: stepID, Prompt: prompt}, "", "")
	}(rs.id, rs.stepID, prompt)
	return true, true
}

func (s *InteractiveService) loadPersistedCodingChangedFiles(runID, turnID string) []string {
	if s.finalizer != nil && strings.TrimSpace(turnID) != "" {
		if state, ok := s.finalizer.state(runID, turnID); ok {
			for _, artifact := range state.artifacts {
				if artifact.Kind != "rag_document" || strings.TrimSpace(artifact.Preview) == "" {
					continue
				}
				var doc struct {
					ChangedFiles []string `json:"changedFiles"`
				}
				if json.Unmarshal([]byte(artifact.Preview), &doc) == nil && len(doc.ChangedFiles) > 0 {
					return uniqueSorted(doc.ChangedFiles)
				}
			}
		}
	}
	return nil
}

func isValidationEnvironmentFailure(exitCode int, stderr string, runErr error) bool {
	if exitCode == 126 || exitCode == 127 {
		return true
	}
	if runErr == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(runErr.Error() + " " + stderr))
	switch {
	case strings.Contains(msg, "permission denied"):
		return true
	case strings.Contains(msg, "not found"):
		return true
	case strings.Contains(msg, "no such file"):
		return true
	default:
		return false
	}
}

// SubmitGateDecision handles the user's r-reg decision card choice (Task-155).
// It fires a startTurn reprompt tailored to the chosen option and clears the pending gate block.
func (s *InteractiveService) SubmitGateDecision(runID, option, customText string) *apiErr {
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		return newAPIErr(404, "run_not_found", "workflow run not found")
	}
	info := rs.pendingGateBlock
	rs.pendingGateBlock = nil
	stepID := rs.lastTurnStepID
	cwd := rs.workspaceCwd
	s.mu.Unlock()

	var prompt string
	switch option {
	case "keep-test-fix-code":
		tests := describeTests(info)
		prompt = fmt.Sprintf(
			"The flow gate detected a regression in %s. The user chose: keep test + requirement → fix the code.\n\n"+
				"Fix the code in the source files so that %s passes again without modifying any pre-existing test file. "+
				"The test is the source of truth — do not edit, delete, or weaken it.",
			tests, tests)
	case "suggest-requirement-change":
		prompt = buildSuggestRequirementPrompt(info, cwd)
		// Mark the upcoming turn as a proposal turn so runFlowGate does not
		// re-block on r-reg/r-tests — the AI is proposing, not fixing code yet.
		s.mu.Lock()
		if rs2 := s.runs[runID]; rs2 != nil {
			rs2.proposalTurnPending = true
		}
		s.mu.Unlock()
	case "custom":
		if strings.TrimSpace(customText) == "" {
			return newAPIErr(400, "invalid_request", "customText is required for option 'custom'")
		}
		prompt = customText
	default:
		return newAPIErr(400, "invalid_option", "option must be: keep-test-fix-code | suggest-requirement-change | custom")
	}

	log.Printf("[gate-decision] runID=%q option=%q", runID, option)

	go func() {
		_, _ = s.startTurn(runID, TurnInput{StepID: stepID, Prompt: prompt}, "", "")
	}()
	return nil
}

// RecordGateAgreement records that the user agreed to the AI's opt-2 requirement
// proposal and writes the per-test override. Called from handleGateAgreement. (Task-155)
func (s *InteractiveService) RecordGateAgreement(runID string, testNames []string) *apiErr {
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		return newAPIErr(404, "run_not_found", "workflow run not found")
	}
	dotFP := filepath.Join(rs.workspaceCwd, ".flowpilot")
	stepID := rs.lastTurnStepID
	s.mu.Unlock()

	// Write human-confirmed override for each agreed test name.
	for _, name := range testNames {
		if name == "" {
			continue
		}
		_ = flowgate.SaveOverride(dotFP, flowgate.Override{TestName: name, HumanConfirm: true})
	}

	tests := strings.Join(testNames, ", ")
	if tests == "" {
		tests = "the agreed-upon tests"
	}
	prompt := fmt.Sprintf(
		"The user has explicitly agreed to the proposed requirement change. Override recorded for: %s.\n\n"+
			"Now perform both steps in order:\n"+
			"1. Update the governing requirement in `requirements/05-System-Specs/` to match the agreed behavior "+
			"(create the file if none exists — `SS-<N>-<short-title>.md`). "+
			"User intent is authoritative — follow it even if the requirement seems unusual, but flag any concern briefly.\n"+
			"2. After the spec update is committed, align the test so it matches the approved requirement.",
		tests)

	go func() {
		_, _ = s.startTurn(runID, TurnInput{StepID: stepID, Prompt: prompt}, "", "")
	}()
	return nil
}

// describeTests returns a human-readable summary of regressed test names.
func describeTests(info *gateBlockInfo) string {
	if info != nil && len(info.regressedTests) > 0 {
		filtered := make([]string, 0, len(info.regressedTests))
		for _, t := range info.regressedTests {
			if t != "suite_regressed" {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) > 0 {
			return strings.Join(filtered, ", ")
		}
	}
	return "the failing tests"
}

// buildSuggestRequirementPrompt builds the opt-2 propose-phase prompt.
// It tries to resolve a governing spec file; if none exists, it uses the empty-spec path.
func buildSuggestRequirementPrompt(info *gateBlockInfo, cwd string) string {
	tests := describeTests(info)
	specPath := findGoverningSpec(cwd)

	if specPath != "" {
		return fmt.Sprintf(
			"The flow gate detected a regression in %s. The user chose: suggest requirement changes.\n\n"+
				"Open the governing requirement file at `%s` and PROPOSE (do not edit yet) what change "+
				"to that requirement would justify the failing test behavior. Present your proposal clearly "+
				"for user review. Do not modify the spec or the test until the user explicitly agrees.\n\n"+
				"The requirement in `requirements/05-System-Specs/` is the source of truth — user intent "+
				"is authoritative. If the user wants this behavior even if it seems incorrect, follow it "+
				"and only flag a concern. Once the user agrees, call the gate-agreement endpoint or indicate "+
				"agreement in your reply so the override can be recorded.",
			tests, specPath)
	}

	// Empty-spec path (T-6): no governing spec in 05-System-Specs — degrade to suggest-or-input.
	return fmt.Sprintf(
		"The flow gate detected a regression in %s. The user chose: suggest requirement changes.\n\n"+
			"No governing requirement file was found in `requirements/05-System-Specs/`. "+
			"Please PROPOSE a requirement that would justify the failing test behavior, "+
			"or present the user with the option to type their own. Do not create or edit "+
			"any spec file without explicit user approval.\n\n"+
			"Once the user agrees to a requirement, you will create "+
			"`requirements/05-System-Specs/SS-<N>-<short-title>.md` with the approved "+
			"requirement, then align the test to match it.",
		tests)
}

// findGoverningSpec looks for the first real SS-*.md under requirements/05-System-Specs/
// that is not a FORMAT-REFERENCE file. Returns a repo-relative path or "" if none found.
func findGoverningSpec(cwd string) string {
	specDir := filepath.Join(cwd, "requirements", "05-System-Specs")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasPrefix(name, "SS-") && strings.HasSuffix(name, ".md") &&
			!strings.Contains(name, "FORMAT-REFERENCE") {
			return filepath.ToSlash(filepath.Join("requirements", "05-System-Specs", name))
		}
	}
	return ""
}

// loadGateMode delegates to the shared readGateMode helper (engine_gate_config.go).
func loadGateMode(dotFP string) string {
	return readGateMode(dotFP)
}

func changedPathsFromDiff(diff []flowgate.ChangedFile) []string {
	if len(diff) == 0 {
		return nil
	}
	out := make([]string, 0, len(diff))
	for _, file := range diff {
		if strings.TrimSpace(file.Path) != "" {
			out = append(out, filepath.ToSlash(strings.TrimSpace(file.Path)))
		}
	}
	sort.Strings(out)
	return out
}

func collectCommitSubjectsSince(cwd, baseSHA string) []string {
	args := []string{"-C", cwd, "log", "--no-merges", "--format=%s"}
	if strings.TrimSpace(baseSHA) != "" {
		args = append(args, baseSHA+"..HEAD")
	}
	out, err := exec.Command("git", args...).Output()
	if err != nil || len(out) == 0 {
		return nil
	}
	var subjects []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			subjects = append(subjects, line)
		}
	}
	return subjects
}

func loadKnownFeatureKeys(cwd string) []string {
	known := changeledger.LoadKnownKeys(filepath.Join(cwd, "change-audit", "FEATURE-KEYS.md"))
	if len(known) == 0 {
		return nil
	}
	keys := make([]string, 0, len(known))
	for key := range known {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func suggestFeatureKeys(dotFP string, changedPaths []string, message string) []string {
	catalog, err := featurecatalog.LoadCatalog(dotFP)
	if err != nil {
		return nil
	}
	candidates := featurecatalog.SuggestKey(changedPaths, message, catalog)
	if len(candidates) == 0 {
		return nil
	}
	limit := 3
	if len(candidates) < limit {
		limit = len(candidates)
	}
	keys := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		keys = append(keys, candidates[i].Key)
	}
	return keys
}

// captureGitHead returns the current HEAD SHA in repoDir, trimmed of whitespace.
func captureGitHead(repoDir string) (string, error) {
	if repoDir == "" {
		return "", nil
	}
	out, err := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ensureBaseline captures or refreshes the test baseline for cwd asynchronously.
// It compares the stored HEAD SHA against the current HEAD and only re-captures
// when something changed, making the per-chat cadence cheap. (Task-156)
func (s *InteractiveService) ensureBaseline(cwd string) {
	if cwd == "" {
		return
	}
	dotFP := filepath.Join(cwd, ".flowpilot")
	go func() {
		if _, err := flowgate.RefreshBaselineIfStale(cwd, dotFP); err != nil {
			log.Printf("[gate] ensureBaseline refresh error: %v", err)
		}
	}()
}
