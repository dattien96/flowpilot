package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
	"flowpilot-runner/internal/flowgate"
	"flowpilot-runner/internal/structure"
	"flowpilot-runner/internal/tooling"
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

	// 1b. Task-184 (CP-43 P-1): capture this turn's Change Contract, declared
	// (parsed from the AI's own final message) or inferred from the diff when
	// absent. Task-185 (P-2): compute scope drift against it. Task-186 (P-3):
	// update the feature's Canonical Head. All non-fatal — a capture/scope/
	// head failure never blocks the turn (AC-9).
	contractDeclared, scopeOutOfScopePaths, scopeHighSeverity, headSpecDrifted, headCodeDrifted, headAttachSpecPending :=
		captureChangeContract(ctx, cwd, rs.id, rs.stepID, fin.FinalMessage, diff, suggestedFeatureKeys)

	// 2. Load test baseline — non-fatal.
	baseline, _ := flowgate.LoadBaseline(dotFP)

	// 3. Load per-test overrides (Task-155): agreed-changed tests are not re-counted.
	overrides, _ := flowgate.LoadOverrides(dotFP)

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
	// Task-223: resolve required file_artifact OUTPUT paths for this flow node
	// (child label == node id on the parent hub topology).
	requiredArtifactOutputs := requiredFileArtifactOutputsForRun(s, rs)
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
		ChangeType:                  rs.changeType,
		WorkspaceCwd:                cwd,
		RequiredFileArtifactOutputs: requiredArtifactOutputs,
		ContractDeclared:            contractDeclared,
		ScopeOutOfScopePaths:        scopeOutOfScopePaths,
		ScopeHighSeverity:           scopeHighSeverity,
		HeadSpecDrifted:             headSpecDrifted,
		HeadCodeDrifted:             headCodeDrifted,
		HeadAttachSpecPending:       headAttachSpecPending,
		HeadRetirePending:           detectRetirePending(cwd, knownFeatureKeys),
	}

	// 7. Load rules; fall back to defaults when flow-rules.json is absent.
	// LoadRules already merges missing DefaultRules by ID (Task-223).
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

// runChildArtifactOutputGate enforces Task-223 file_artifact OUTPUT write
// contracts on spawned flow children and Task-242 tier-1 doc/scope rules when
// the child is an agent.delegate with a non-empty turn diff. Reviewer/inline
// turns with empty diff remain zero-cost (BUG-152). Returns true when the turn
// should be blocked/reprompted.
func (s *InteractiveService) runChildArtifactOutputGate(
	_ context.Context, rs *interactiveRun, turnID string, fin finalizeInput,
) (block bool) {
	if rs == nil {
		return false
	}
	cwd := strings.TrimSpace(rs.workspaceCwd)
	if cwd == "" {
		return false
	}
	required := requiredFileArtifactOutputsForRun(s, rs)
	structured := requiredStructuredFileArtifactOutputsForRun(s, rs)
	telegramSends := requiredTelegramSendsForRun(s, rs)

	// Prefer defaults merged with any on-disk rules so r-artifact-output is
	// present even when an old flow-rules.json predates Task-223.
	rules := flowgate.MergeDefaultRules(nil)
	if loaded, err := flowgate.LoadRules(filepath.Join(cwd, ".flowpilot", "settings")); err == nil {
		rules = flowgate.MergeDefaultRules(loaded)
	}

	// Artifact family (Task-223+) — always considered when bindings exist.
	var only []flowgate.Rule
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if flowgate.IsArtifactRule(r.ID) {
			only = append(only, r)
		}
	}

	// Task-242 tier-1: doc/scope on coding delegate children with non-empty diff.
	s.mu.Lock()
	baseSHA := rs.turnStartGitHead
	parentID := rs.parentRunID
	changeType := rs.changeType
	if parentID != "" {
		if parent := s.runs[parentID]; parent != nil && parent.changeType != "" {
			changeType = parent.changeType
		}
	}
	s.mu.Unlock()
	diff, _ := flowgate.ObserveGitDiffSince(cwd, baseSHA)
	node, nodeOK := flowNodeForRun(s, rs)
	isDelegate := false
	if nodeOK {
		if canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior); ok && canonical == "agent.delegate" {
			isDelegate = true
		}
	}
	if isDelegate && len(diff) > 0 {
		for _, r := range rules {
			if !r.Enabled {
				continue
			}
			if flowgate.IsDocScopeRule(r.ID) {
				only = append(only, r)
			}
		}
		// Task-242 tier-2b: flows without command.validate also run test rules
		// on the coding child (review-loop). Flows with validate leave tests to
		// that node (rag-harness).
		if parentID != "" && !parentFlowHasValidateNode(s, parentID) {
			for _, r := range rules {
				if !r.Enabled {
					continue
				}
				for _, id := range flowgate.TestRuleIDs() {
					if r.ID == id {
						only = append(only, r)
					}
				}
			}
		}
	}

	if len(only) == 0 {
		return false
	}
	// Artifact-only path with no bindings and no doc rules selected → no-op.
	if len(required) == 0 && len(structured) == 0 && len(telegramSends) == 0 && !isDelegate {
		return false
	}
	if len(required) == 0 && len(structured) == 0 && len(telegramSends) == 0 && len(diff) == 0 {
		return false
	}

	tr := flowgate.TurnResult{
		RunID:                                 rs.id,
		StepID:                                rs.stepID,
		FinalMessage:                          fin.FinalMessage,
		WrittenPaths:                          fin.ChangedFiles,
		WorkspaceCwd:                          cwd,
		GitDiff:                               diff,
		ChangedPaths:                          changedPathsFromDiff(diff),
		ChangeType:                            changeType,
		RequiredFileArtifactOutputs:           required,
		RequiredStructuredFileArtifactOutputs: structured,
		RequiredTelegramSends:                 telegramSends,
	}
	// Optional oracle for tier-2b when test rules are in `only`.
	needsOracle := false
	for _, r := range only {
		for _, id := range flowgate.TestRuleIDs() {
			if r.ID == id {
				needsOracle = true
			}
		}
	}
	if needsOracle {
		dotFP := filepath.Join(cwd, ".flowpilot")
		baseline, _ := flowgate.LoadBaseline(dotFP)
		overrides, _ := flowgate.LoadOverrides(dotFP)
		oracle := flowgate.RunOracle(cwd, baseline, diff, overrides)
		var failedTests []string
		if oracle.HasRegression {
			failedTests = oracle.Regressed
		}
		tr.Tests = flowgate.TestOutcome{Ran: baseline != nil, Failed: failedTests}
	}

	violations := flowgate.Evaluate(tr, only)
	if len(violations) == 0 {
		return false
	}
	result := flowgate.Enforce(violations, loadGateMode(filepath.Join(cwd, ".flowpilot")))
	s.mu.Lock()
	s.emitLocked(rs, ProviderEvent{
		Type:           EventFlowGateViolation,
		ProviderTurnID: turnID,
		Error:          result.Message,
		Status:         result.Action,
	})
	s.mu.Unlock()
	switch result.Action {
	case "block":
		// Tier-2b always-block on child maps to parent escalate (actionable), not
		// an unanswerable child block (Task-242 T-9 / SD-20 Flow Mode mapping).
		if parentID != "" && needsOracle {
			_, _ = s.applyFlowControl(parentID, FlowControlInput{
				Status:  "escalate",
				Summary: "flow gate block on coding step: " + result.Message,
			})
		}
		return true
	case "reprompt":
		s.mu.Lock()
		attempts := rs.repromptAttempts
		rs.repromptAttempts++
		stepID := rs.lastTurnStepID
		runID := rs.id
		s.mu.Unlock()
		log.Printf("[gate] child gate reprompt attempt=%d run=%q stepID=%q",
			attempts, runID, stepID)
		if attempts < maxFlowGateReprompts {
			go func(runID, stepID, prompt string) {
				_, _ = s.startTurn(runID, TurnInput{StepID: stepID, Prompt: prompt}, "", "")
			}(runID, stepID, flowgate.RepromptPrompt(result))
		}
		return true
	}
	return false
}

// requiredFileArtifactOutputsForRun looks up the active flow node for this run
// (label matches node id on the parent hub) and returns required file_artifact
// OUTPUT paths (Task-223). Empty when not a flow child or no required outputs.
func requiredFileArtifactOutputsForRun(s *InteractiveService, rs *interactiveRun) []string {
	node, ok := flowNodeForRun(s, rs)
	if !ok {
		return nil
	}
	return requiredFileArtifactOutputPaths(node)
}

// requiredTelegramSendsForRun mirrors requiredFileArtifactOutputsForRun for
// telegram.v1 OUTPUT bindings (Task-233).
func requiredTelegramSendsForRun(s *InteractiveService, rs *interactiveRun) []string {
	node, ok := flowNodeForRun(s, rs)
	if !ok {
		return nil
	}
	targets := requiredTelegramOutputTargets(node)
	if len(targets) == 0 {
		return nil
	}
	chatIDs := make([]string, 0, len(targets))
	for _, t := range targets {
		chatIDs = append(chatIDs, t.chatID)
	}
	return chatIDs
}

// requiredStructuredFileArtifactOutputsForRun returns Task-225 structured
// required OUTPUT paths for the active flow node of this run.
func requiredStructuredFileArtifactOutputsForRun(s *InteractiveService, rs *interactiveRun) []flowgate.StructuredFileArtifactOutput {
	node, ok := flowNodeForRun(s, rs)
	if !ok {
		return nil
	}
	return requiredStructuredFileArtifactOutputs(node)
}

// isFlowCodingCommitAttempt reports whether this approval is a git commit from
// a flow-engine coding child (Task-242 T-4). Token-aware — does not match
// substrings like "git commitment".
func isFlowCodingCommitAttempt(s *InteractiveService, rs *interactiveRun, details ApprovalDetails) bool {
	if s == nil || rs == nil {
		return false
	}
	if details.Kind != "exec" && details.Kind != "" {
		// Only shell/exec approvals; empty Kind treated as possible shell on some providers.
	}
	if !looksLikeGitCommitCommand(details.Command) {
		return false
	}
	if strings.TrimSpace(rs.parentRunID) == "" {
		return false
	}
	if !s.isFlowEngineDriven(rs.parentRunID) {
		return false
	}
	node, ok := flowNodeForRun(s, rs)
	if !ok {
		return false
	}
	canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior)
	return ok && canonical == "agent.delegate"
}

// shellFields splits a command line with simple quote awareness so paths that
// contain spaces stay one token (Codex review Important: git -C "C:\repo with spaces").
// Supports "double" and 'single' quotes; quote characters are stripped.
func shellFields(cmd string) []string {
	var out []string
	var b strings.Builder
	var quote rune // 0 | '"' | '\''
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	for _, r := range cmd {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return out
}

// looksLikeGitCommitCommand detects git-commit invocations including forms that
// insert global options before the verb (Codex review Important #1):
//   git commit -m x
//   git -C . commit -m x
//   git -c user.name=a commit -m x
//   git -C "C:\repo with spaces" commit -m x
//   /usr/bin/git --git-dir=... commit
func looksLikeGitCommitCommand(cmd string) bool {
	fields := shellFields(cmd)
	for i := 0; i < len(fields); i++ {
		tok := fields[i]
		base := tok
		if j := strings.LastIndexAny(tok, `/\`); j >= 0 {
			base = tok[j+1:]
		}
		if base != "git" && base != "git.exe" {
			continue
		}
		// Walk remaining args: skip global options that take a value, then
		// the first non-option token is the git verb.
		for j := i + 1; j < len(fields); j++ {
			arg := fields[j]
			if arg == "" {
				continue
			}
			if arg == "commit" {
				return true
			}
			// Global options that consume the next token as a value.
			switch arg {
			case "-C", "-c", "--git-dir", "--work-tree", "--namespace", "--config-env",
				"--super-prefix", "--list-cmds":
				j++ // skip value (already one shell field even with spaces)
				continue
			}
			if strings.HasPrefix(arg, "-c") && strings.Contains(arg, "=") {
				continue
			}
			if strings.HasPrefix(arg, "--git-dir=") || strings.HasPrefix(arg, "--work-tree=") ||
				strings.HasPrefix(arg, "--namespace=") || strings.HasPrefix(arg, "--config-env=") {
				continue
			}
			if strings.HasPrefix(arg, "-") {
				continue
			}
			// First non-option token: the git subcommand.
			return arg == "commit"
		}
	}
	return false
}

// parentFlowHasValidateNode reports whether the parent flow topology includes a
// command.validate node (Task-242 tier-2 ownership).
func parentFlowHasValidateNode(s *InteractiveService, parentRunID string) bool {
	if s == nil || parentRunID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	parent := s.runs[parentRunID]
	if parent == nil {
		return false
	}
	for _, n := range parent.activeFlowNodes {
		if canonical, ok := agentpack.NormalizeBehaviorID(n.Behavior); ok && canonical == "command.validate" {
			return true
		}
	}
	return false
}

// flowNodeForRun resolves the active flow node for a child (or hub) run.
func flowNodeForRun(s *InteractiveService, rs *interactiveRun) (agentpack.FlowNode, bool) {
	if s == nil || rs == nil {
		return agentpack.FlowNode{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	label := strings.TrimSpace(rs.label)
	if label == "" {
		return agentpack.FlowNode{}, false
	}
	parentID := strings.TrimSpace(rs.parentRunID)
	if parentID == "" {
		// Hub itself rarely has file OUTPUT bindings; still check local topology.
		if node, ok := findFlowNode(rs.activeFlowNodes, label); ok {
			return node, true
		}
		return agentpack.FlowNode{}, false
	}
	parent := s.runs[parentID]
	if parent == nil {
		return agentpack.FlowNode{}, false
	}
	node, ok := findFlowNode(parent.activeFlowNodes, label)
	if !ok {
		return agentpack.FlowNode{}, false
	}
	return node, true
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

// captureChangeContract persists this turn's Change Contract (Task-184,
// CP-43 P-1) — declared if the AI's final message contains a
// `[Change Contract]` block (T-2), otherwise inferred from the observed diff
// (T-4) — computes scope drift against it (Task-185, P-2), and updates the
// feature's Canonical Head (Task-186, P-3). Entirely best-effort: every
// failure is logged and degrades toward "nothing to report" so a
// changecontract/scope/head problem can never block a turn (SS-14 AC-9).
func captureChangeContract(ctx context.Context, cwd, runID, stepID, finalMessage string, diff []flowgate.ChangedFile, suggestedFeatureKeys []string) (declared bool, outOfScopePaths []string, highSeverity, specDrifted, codeDrifted, attachSpecPending bool) {
	if cwd == "" {
		return false, nil, false, false, false, false
	}
	store, err := changecontract.NewStore(cwd)
	if err != nil {
		log.Printf("[changecontract] store open failed: %v", err)
		return false, nil, false, false, false, false
	}
	featureKey := ""
	if len(suggestedFeatureKeys) > 0 {
		featureKey = suggestedFeatureKeys[0]
	}

	c, declared := changecontract.ParseDeclaration(finalMessage)
	if declared {
		if c.FeatureKey == "" {
			c.FeatureKey = featureKey
		}
	} else {
		c = changecontract.InferFromDiff(featureKey, diff)
	}
	c.RunID = runID
	c.StepID = stepID

	if err := store.Save(c); err != nil {
		log.Printf("[changecontract] save failed: %v", err)
	}

	// Task-185: scope diff needs no structure.Provider (nil is safe — see
	// changecontract.ScopeDiff); only construct one, at the cost of a
	// gitnexus probe, if there is actually something out-of-scope to weigh.
	outOfScopePaths, _ = changecontract.ScopeDiff(c, diff, nil)
	if len(outOfScopePaths) > 0 {
		hasGitNexus := tooling.CheckTool("gitnexus", cwd).Status == "ok"
		sp := structure.New(cwd, hasGitNexus)
		highSeverity = changecontract.HighSeverity(ctx, sp, outOfScopePaths)
	}

	if c.FeatureKey != "" {
		specDrifted, codeDrifted, attachSpecPending = updateCanonicalHead(cwd, c, len(outOfScopePaths) > 0)
	}

	return declared, outOfScopePaths, highSeverity, specDrifted, codeDrifted, attachSpecPending
}

// updateCanonicalHead loads (or backfills/mints) c.FeatureKey's Canonical
// Head (Task-186), checks it for spec/code drift, and — only when neither
// drifted — folds this in-contract turn into the Head. A drifted turn is
// surfaced via the caller's TurnResult flags for a human to reconcile
// (BR-2); the Head itself is left untouched until that happens, so a
// drifted state is never silently overwritten.
func updateCanonicalHead(cwd string, c changecontract.Contract, hasOutOfContractChange bool) (specDrifted, codeDrifted, attachSpecPending bool) {
	dotFP := filepath.Join(cwd, ".flowpilot")
	catalog, _ := featurecatalog.LoadCatalog(dotFP)

	head, found, err := changecontract.LoadHead(cwd, c.FeatureKey)
	if err != nil {
		log.Printf("[changecontract] head load failed for %q: %v", c.FeatureKey, err)
		return false, false, false
	}
	if !found {
		ledger, _ := changeledger.New(dotFP)
		head = changecontract.BuildHead(cwd, c.FeatureKey, ledger, catalog, &c)
		if err := changecontract.SaveHead(cwd, head); err != nil {
			log.Printf("[changecontract] head save (birth/backfill) failed for %q: %v", c.FeatureKey, err)
		}
		return false, false, false // a just-minted Head cannot itself be drifted
	}

	// r-attach-spec: a spec_less Head whose feature has since gained a
	// governing doc in the catalog. Never auto-applied — surfaced for human
	// confirmation; RebaselineWithSpec runs only once that is given.
	if head.SpecConfidence == changecontract.SpecConfidenceSpecLess && catalog != nil {
		if feat, ok := catalog.Get(c.FeatureKey); ok && len(feat.DocRefs) > 0 {
			attachSpecPending = true
		}
	}

	specDrifted = changecontract.SpecDrifted(cwd, head)
	codeDrifted = changecontract.CodeDrifted(hasOutOfContractChange, specDrifted)

	if !specDrifted && !codeDrifted && !attachSpecPending {
		updated := changecontract.UpdateHead(head, c)
		if decisions, ok := foldCanonicalHeadDecisions(dotFP, c.FeatureKey); ok {
			updated.Decisions = decisions
			updated.IntentSignature = changecontract.ComputeSignature(updated)
		}
		if err := changecontract.SaveHead(cwd, updated); err != nil {
			log.Printf("[changecontract] head save (update) failed for %q: %v", c.FeatureKey, err)
		}
	}
	return specDrifted, codeDrifted, attachSpecPending
}

// foldCanonicalHeadDecisions folds negative knowledge (rejected chat_summary
// approaches + revert-type changeledger bugfix entries, Task-187) into a
// Head's Decisions on every gate-passing update. Best-effort: ok=false on any
// load/fold failure so the caller leaves the Head's existing Decisions
// untouched rather than silently clobbering them with an empty set (AC-9).
func foldCanonicalHeadDecisions(dotFP, featureKey string) ([]changecontract.Decision, bool) {
	chatLedger, err := changeledger.NewChatSummaryLedger(dotFP)
	if err != nil {
		log.Printf("[changecontract] chat summary ledger load failed for %q: %v", featureKey, err)
		return nil, false
	}
	ledger, err := changeledger.New(dotFP)
	if err != nil {
		log.Printf("[changecontract] ledger load failed for %q: %v", featureKey, err)
		return nil, false
	}
	decisions, err := changecontract.FoldDecisions(featureKey, chatLedger, ledger)
	if err != nil {
		log.Printf("[changecontract] FoldDecisions failed for %q: %v", featureKey, err)
		return nil, false
	}
	return decisions, true
}

// detectRetirePending reports whether any spec-backed Canonical Head exists for
// a feature key that is no longer among the workspace's known feature keys
// (change-audit/FEATURE-KEYS.md) — a signal the feature was renamed/merged/
// removed there but its Head has not yet been migrated (Task-187 r-retire).
// This is a deliberately conservative nudge, not auto-retire: it only flags
// spec_backed, not-yet-retired Heads (a born-spec_less Head may carry an
// inferred key that was never registered, so exempting them avoids false
// positives). The actual retire is always an explicit human action via
// handleRetireCanonicalHead (BR-2 — never automatic).
func detectRetirePending(cwd string, knownFeatureKeys []string) bool {
	matches, _ := filepath.Glob(filepath.Join(cwd, ".flowpilot", "canonical", "*.json"))
	if len(matches) == 0 {
		return false
	}
	known := make(map[string]bool, len(knownFeatureKeys))
	for _, key := range knownFeatureKeys {
		known[key] = true
	}
	for _, path := range matches {
		key := strings.TrimSuffix(filepath.Base(path), ".json")
		if key == "" || known[key] {
			continue
		}
		head, found, err := changecontract.LoadHead(cwd, key)
		if err != nil || !found {
			continue
		}
		if head.RetiredAt != nil {
			continue // already retired
		}
		if head.SpecConfidence != changecontract.SpecConfidenceSpecBacked {
			continue // exempt spec_less (possibly inferred, never-registered keys)
		}
		return true
	}
	return false
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
