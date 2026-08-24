package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
	"flowpilot-runner/internal/flowgate"
	"flowpilot-runner/internal/structure"
	"flowpilot-runner/internal/tooling"
)

const maxFlowGateReprompts = 2

// maxGateFixCodeAutoReprompts caps silent re-attempts after the user already
// chose keep-test-fix-code. Without this, a model that only narrates the fix
// (or asks for write permission in chat without writing) re-fires the full
// regression decision modal every turn (CP-51 live run-11262).
const maxGateFixCodeAutoReprompts = 2

// gateBlockInfo carries r-reg details for the decision handler (Task-155).
type gateBlockInfo struct {
	regressedTests []string
	stepID         string
}

// gateEpochStillValid reports whether Stop has not invalidated the gate claim
// (epoch match) and the parent loop is not stopped/done (V10R4 P0-03 / P1-04).
func (s *InteractiveService) gateEpochStillValid(runID string, epoch int64) bool {
	if s == nil || runID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gateEpochStillValidLocked(runID, epoch)
}

// gateEpochStillValidLocked is the under-lock core of gateEpochStillValid.
// Caller holds s.mu.
func (s *InteractiveService) gateEpochStillValidLocked(runID string, epoch int64) bool {
	rs := s.runs[runID]
	if rs == nil || rs.gateEpoch != epoch {
		return false
	}
	if rs.status == RunStatusCancelled || rs.status == RunStatusFailed {
		return false
	}
	loopParent := runID
	if rs.parentRunID != "" {
		loopParent = rs.parentRunID
	}
	if st := s.agentOrchestrator.loopStateFor(loopParent).Status; st == "stopped" || st == "done" {
		return false
	}
	return true
}

// withGateEpochDurable holds s.mu across the entire durable side-effect so Stop
// (which must take s.mu to bump gateEpoch) cannot interleave mid-write
// (BUG-288 R16-P0). Prefer short writes only.
// fn may return an error (R17-P1 contract I/O) â€” treated as not-committed.
func (s *InteractiveService) withGateEpochDurable(runID string, epoch int64, fn func() error) bool {
	if s == nil || fn == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.gateEpochStillValidLocked(runID, epoch) {
		return false
	}
	if err := fn(); err != nil {
		log.Printf("[gate] durable side-effect failed run=%q: %v (treating as blocked)", runID, err)
		return false
	}
	return s.gateEpochStillValidLocked(runID, epoch)
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
	return s.runFlowGateAtEpoch(ctx, rs, turnID, fin, -1)
}

// runFlowGateAtEpoch is runFlowGate with a Stop-cancellation epoch. When epoch>=0,
// side effects (contract commit, emit, reprompt, escalate) are skipped if Stop
// bumped gateEpoch mid-evaluation (V10R4 P0-03).
func (s *InteractiveService) runFlowGateAtEpoch(
	ctx context.Context, rs *interactiveRun, turnID string, fin finalizeInput, epoch int64,
) (block bool) {
	cwd := rs.workspaceCwd
	if cwd == "" {
		return false
	}
	runID := rs.id
	dotFP := filepath.Join(cwd, ".flowpilot")

	// 1. Observe git diff â€” turn-scoped (V9-09): commits since turnStartGitHead +
	// dirty paths new/changed vs turnStartWorktree (not pre-existing dirt).
	s.mu.Lock()
	baseSHA := rs.turnStartGitHead
	startWT := rs.turnStartWorktree
	if epoch < 0 {
		epoch = rs.gateEpoch
	}
	s.mu.Unlock()
	if !s.gateEpochStillValid(runID, epoch) {
		return true // treat as blocked so outer path does not complete the turn
	}
	diff, diffErr := observeTurnScopedDiff(cwd, baseSHA, startWT)
	if diffErr != nil {
		// BUG-288 P1-17: an observation error is NOT "no changes" â€” evaluating
		// tier-1/tier-3 rules against a fabricated empty diff could let a real
		// change silently pass the gate. Fail closed: block instead of
		// evaluating, and surface it the same way other gate blocks are
		// surfaced so the user/desktop sees why.
		log.Printf("[gate] observe turn-scoped diff failed cwd=%q baseSHA=%q: %v (failing closed, blocking turn)", cwd, baseSHA, diffErr)
		if s.gateEpochStillValid(runID, epoch) {
			s.mu.Lock()
			if rs2 := s.runs[runID]; rs2 != nil && rs2.gateEpoch == epoch {
				s.emitLocked(rs2, ProviderEvent{
					Type:           EventFlowGateViolation,
					ProviderTurnID: turnID,
					Error:          "gate observation failed (workspace diff unreadable): " + diffErr.Error(),
					Status:         "block",
				})
			}
			s.mu.Unlock()
		}
		return true
	}
	log.Printf("[gate] cwd=%q baseSHA=%q diffLen=%d diff=%+v", cwd, baseSHA, len(diff), diff)
	changedPaths := changedPathsFromDiff(diff)
	commitSubjects := collectCommitSubjectsSince(cwd, baseSHA)
	knownFeatureKeys := loadKnownFeatureKeys(cwd)
	suggestedFeatureKeys := suggestFeatureKeys(dotFP, changedPaths, strings.Join(commitSubjects, "\n"))

	// 1b. Task-184/185: prepare Change Contract + scope for evaluation WITHOUT
	// saving/head update (V9-02/V9-04). Commit only after gate allows.
	// CP-43 P-1 Plan A: also consider the turn's prompt so a user-declared
	// [Change Contract] is not missed when the AI does not echo it.
	prepared := prepareChangeContract(ctx, cwd, rs.id, rs.stepID, rs.lastFullPrompt, fin.FinalMessage, diff, suggestedFeatureKeys)
	contractDeclared := prepared.declared
	scopeOutOfScopePaths := prepared.outOfScopePaths
	scopeHighSeverity := prepared.highSeverity
	headSpecDrifted := prepared.specDrifted
	headCodeDrifted := prepared.codeDrifted
	headAttachSpecPending := prepared.attachSpecPending

	// 2. Load test baseline â€” await singleflight so we do not race empty baseline (BUG-288 #3).
	// BUG-288 P2-04: thread this call's own cancellable ctx so Stop can cut a
	// first-time baseline capture short instead of always running under
	// context.Background().
	s.ensureBaselineReadyContext(ctx, cwd)
	baseline, blErr := flowgate.LoadBaseline(dotFP)
	if blErr != nil {
		// BUG-288 R13-13: corrupt/unreadable baseline must fail-closed (not
		// silently disable r-tests/r-reg). Missing file is (nil, nil).
		log.Printf("[gate] LoadBaseline failed cwd=%q: %v (failing closed)", cwd, blErr)
		msg := "gate baseline unreadable (test_baseline.json corrupt or unreadable): " + blErr.Error()
		if s.gateEpochStillValid(runID, epoch) {
			s.mu.Lock()
			if rs2 := s.runs[runID]; rs2 != nil && rs2.gateEpoch == epoch {
				s.emitLocked(rs2, ProviderEvent{
					Type:           EventFlowGateViolation,
					ProviderTurnID: turnID,
					Error:          msg,
					Status:         "block",
				})
			}
			s.mu.Unlock()
		}
		return true
	}

	// 3. Load per-test overrides (Task-155): agreed-changed tests are not re-counted.
	overrides, _ := flowgate.LoadOverrides(dotFP)

	// 4. Run regression oracle with turn context so cancel aborts the suite.
	oracle := flowgate.RunOracleContext(ctx, cwd, baseline, diff, overrides)

	// 5. Clear overrides for tests that are now green (sticky-until-green, DOD-07).
	// BUG-288 R15-P0 / R16-P0: durable write under s.mu so Stop cannot TOCTOU
	// between epoch check and file mutation.
	// BUG-288 R18-5: ClearOverrideIfGreen failure must fail-closed (block gate),
	// not continue with stale accepted-test overrides still on disk.
	if oracle.EnvError == "" && !oracle.HasRegression && len(oracle.Passed) > 0 {
		if !s.withGateEpochDurable(runID, epoch, func() error {
			return flowgate.ClearOverrideIfGreen(dotFP, oracle.Passed)
		}) {
			return true
		}
	}

	// CP-53 P-1: gate_blind when baseline missing / env error / red-at-capture.
	if s.gateBlindBlocksTurn(runID, turnID, epoch, rs, dotFP, baseline, oracle, diff) {
		return true
	}

	// 6. Build TurnResult for the evaluator.
	// V9-10/V9-27: ordinary Failed vs true Regressed (separate fields).
	// suite_failed only when baseline was green and suite is now red without
	// named regressed tests â€” not when baseline itself is already red.
	// BUG-289 M6/F-12: filter Failed by IsOverridden (same as regressed path)
	// so human-agreed test overrides do not keep blocking via r-tests.
	var failedTests, regressedTests []string
	if oracle.EnvError == "" {
		if oracle.HasRegression {
			regressedTests = append(regressedTests, oracle.Regressed...)
		}
		if !oracle.SuitePassed && !oracle.HasRegression {
			if len(oracle.Failed) > 0 {
				for _, t := range oracle.Failed {
					if !flowgate.IsOverridden(overrides, t) {
						failedTests = append(failedTests, t)
					}
				}
			} else if baseline != nil && baseline.SuitePassed {
				// Coarse-mode suite_failed cannot be per-test overridden.
				failedTests = []string{"suite_failed"}
			}
		}
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
			Ran:       baseline != nil && oracle.EnvError == "",
			Failed:    failedTests,
			Regressed: regressedTests,
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
		// V10R4 P0-03 / BUG-288 R16-P0: durable commit under s.mu (no TOCTOU).
		// V9-02: only persist contract + canonical head after gate allows.
		// CP-55 P-5: this is the root gate (Normal chat or a Flow's own hub
		// run, never a spawned coder child) — always immediate-write, never
		// staged; a Flow's coder children route through the child gate below.
		if !s.withGateEpochDurable(runID, epoch, func() error {
			return commitChangeContract(cwd, prepared, canonicalPendingRoute{}, s.markerSecret)
		}) {
			// BUG-289 H3/F-3: do not silent-block — emit + escalate like sibling
			// fail-closed sites (diff observe / baseline).
			msg := "gate change-contract/head commit failed (canonical head unreadable or unwritable); blocked fail-closed"
			if s.gateEpochStillValid(runID, epoch) {
				s.mu.Lock()
				if rs2 := s.runs[runID]; rs2 != nil && rs2.gateEpoch == epoch {
					s.emitLocked(rs2, ProviderEvent{
						Type:           EventFlowGateViolation,
						ProviderTurnID: turnID,
						Error:          msg,
						Status:         "block",
					})
				}
				s.mu.Unlock()
				if _, err := s.applyFlowControl(runID, FlowControlInput{
					Status:  "escalate",
					Summary: "flow gate block: " + msg,
				}); err != nil {
					log.Printf("[gate] escalate after commitChangeContract failure: %v", err)
				}
			}
			return true
		}
		return false
	}

	// 10. Enforce â€” read gate_mode from .flowpilot/settings/gate-config.json; default warn.
	gateMode := loadGateMode(dotFP)
	result := flowgate.Enforce(violations, gateMode)
	s.recordGateEnforceMetric(dotFP, rs, turnID, gateMode, result)

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
	// V10R4 P0-03: apply phase under epoch claim â€” no emit/mutate after Stop.
	if !s.gateEpochStillValid(runID, epoch) {
		return true
	}
	s.mu.Lock()
	if s.runs[runID] == nil || s.runs[runID].gateEpoch != epoch {
		s.mu.Unlock()
		return true
	}
	rs = s.runs[runID]
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
		if !s.gateEpochStillValid(runID, epoch) {
			return true
		}
		s.mu.Lock()
		rs = s.runs[runID]
		if rs == nil || rs.gateEpoch != epoch {
			s.mu.Unlock()
			return true
		}
		attempts := rs.repromptAttempts
		rs.repromptAttempts++
		stepID := rs.lastTurnStepID
		ws := rs.workspaceCwd
		s.mu.Unlock()
		log.Printf("[gate] reprompt attempt=%d stepID=%q", attempts, stepID)
		if attempts < maxFlowGateReprompts {
			// Root/chat gate captures contract under rs.id â€” re-append so retry
			// turns still see declared scope (mirror child-gate Task-247 inject).
			prompt := flowgate.RepromptPrompt(result)
			prompt = appendChangeContractIfAnyWithSecret(ws, runID, prompt, s.markerSecret)
			// V10R3 P0: queue reprompt; do NOT startTurn here â€” pendingFlowGateSettle
			// / postTurnGateCancel / turnInFlight are still set until the caller
			// cleans up, so a concurrent startTurn would 409 and be dropped.
			s.mu.Lock()
			if r := s.runs[runID]; r != nil && r.gateEpoch == epoch {
				r.pendingGateRepromptPrompt = prompt
				r.pendingGateRepromptStepID = stepID
				r.pendingGateRepromptGen++
			}
			s.mu.Unlock()
			return true
		}
		// V9-17: root max reprompts â€” escalate to durable awaiting-user like child path.
		if !s.gateEpochStillValid(runID, epoch) {
			return true
		}
		if _, err := s.applyFlowControl(runID, FlowControlInput{
			Status:  "escalate",
			Summary: "Gate reprompt exhausted: " + result.Message,
		}); err != nil {
			log.Printf("[gate] root escalate after max reprompts failed: %v", err)
		}
		s.recordGateEscalateMetric(dotFP, rs, turnID, gateMode, "reprompt_exhausted")
		return true
	}

	// "warn" or "approve": log only, let the turn complete normally — still commit
	// contract/head because the turn is allowed.
	// BUG-288 R19-3: must use withGateEpochDurable (epoch + fail-closed I/O),
	// same as the zero-violation allow path — direct commitChangeContract discarded
	// errors and raced Stop before contract/head write.
	// CP-55 P-5: root gate (Normal chat / Flow hub) — always immediate-write.
	if !s.withGateEpochDurable(runID, epoch, func() error {
		return commitChangeContract(cwd, prepared, canonicalPendingRoute{}, s.markerSecret)
	}) {
		return true
	}
	s.recordGateAcceptedMetric(dotFP, rs, turnID, gateMode)
	return false
}

// runChildArtifactOutputGate enforces Task-223 file_artifact OUTPUT write
// contracts on spawned flow children and Task-242 tier-1 doc/scope rules when
// the child is a genuine code-writing agent.delegate (not a reviewer) that
// wrote non-doc files this turn. Reviewers share agent.delegate + the coder's
// dirty worktree and must stay zero-cost for doc/scope/contract (BUG-152).
// Returns true when the turn should be blocked/reprompted.
func (s *InteractiveService) runChildArtifactOutputGate(
	ctx context.Context, rs *interactiveRun, turnID string, fin finalizeInput,
) (block bool) {
	return s.runChildArtifactOutputGateAtEpoch(ctx, rs, turnID, fin, -1)
}

func (s *InteractiveService) runChildArtifactOutputGateAtEpoch(
	ctx context.Context, rs *interactiveRun, turnID string, fin finalizeInput, epoch int64,
) (block bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if rs == nil {
		return false
	}
	cwd := strings.TrimSpace(rs.workspaceCwd)
	if cwd == "" {
		return false
	}
	runID := rs.id
	required := requiredFileArtifactOutputsForRun(s, rs)
	structured := requiredStructuredFileArtifactOutputsForRun(s, rs)
	telegramSends := requiredTelegramSendsForRun(s, rs)

	// Prefer defaults merged with any on-disk rules so r-artifact-output is
	// present even when an old flow-rules.json predates Task-223.
	rules := flowgate.MergeDefaultRules(nil)
	if loaded, err := flowgate.LoadRules(filepath.Join(cwd, ".flowpilot", "settings")); err == nil {
		rules = flowgate.MergeDefaultRules(loaded)
	}

	// Artifact family (Task-223+) â€” always considered when bindings exist.
	var only []flowgate.Rule
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if flowgate.IsArtifactRule(r.ID) {
			only = append(only, r)
		}
	}

	// Task-242 tier-1: doc/scope on genuine code-writing children only.
	s.mu.Lock()
	baseSHA := rs.turnStartGitHead
	startWT := rs.turnStartWorktree
	parentID := rs.parentRunID
	changeType := rs.changeType
	if epoch < 0 {
		epoch = rs.gateEpoch
	}
	s.mu.Unlock()
	if !s.gateEpochStillValid(runID, epoch) {
		return true
	}
	// Re-lock briefly was replaced by unlock above â€” re-acquire fields already copied.
	s.mu.Lock()
	// (parentID/changeType/base already set)
	if parentID != "" {
		if parent := s.runs[parentID]; parent != nil && parent.changeType != "" {
			changeType = parent.changeType
		}
	}
	s.mu.Unlock()
	// Per-turn diff: commits since turnStartGitHead + worktree paths that are
	// new/changed relative to turnStartWorktree (not pre-existing coder dirt).
	diff, diffErr := observeTurnScopedDiff(cwd, baseSHA, startWT)
	if diffErr != nil {
		// BUG-288 P1-17: same fail-closed contract as runFlowGateAtEpoch â€” an
		// observation error must not be silently treated as "no changes",
		// which could let Tier-1 pass with a fabricated empty diff.
		// BUG-288 R13-04: also escalate parent so the child is not hung without
		// an actionable hub surface (mirror Tier-1 block path).
		log.Printf("[gate] child artifact-output gate: observe turn-scoped diff failed cwd=%q baseSHA=%q: %v (failing closed, blocking turn)", cwd, baseSHA, diffErr)
		msg := "gate observation failed (workspace diff unreadable): " + diffErr.Error()
		if s.gateEpochStillValid(runID, epoch) {
			s.mu.Lock()
			if rs2 := s.runs[runID]; rs2 != nil && rs2.gateEpoch == epoch {
				s.emitLocked(rs2, ProviderEvent{
					Type:           EventFlowGateViolation,
					ProviderTurnID: turnID,
					Error:          msg,
					Status:         "block",
				})
			}
			s.mu.Unlock()
		}
		if parentID != "" && s.gateEpochStillValid(runID, epoch) {
			_, _ = s.applyFlowControl(parentID, FlowControlInput{
				Status:  "escalate",
				Summary: "flow gate block: " + msg,
			})
		}
		return true
	}
	node, nodeOK := flowNodeForRun(s, rs)
	isDelegate := false
	if nodeOK {
		if canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior); ok && canonical == "agent.delegate" {
			isDelegate = true
		}
	}
	// Reviewers (explicit role/agent) are excluded. Tier-1 activates on
	// non-empty per-turn code-diff OR WrittenPaths (EventFileChanged).
	// BUG-288 #8: pendingGateCodePaths forces re-check after a prior gate fail.
	s.mu.Lock()
	pendingPaths := append([]string(nil), rs.pendingGateCodePaths...)
	s.mu.Unlock()
	isCodingChild := isDelegate && isFlowCodeWritingChild(rs, node, nodeOK, fin.ChangedFiles, diff)
	if isDelegate && len(pendingPaths) > 0 {
		isCodingChild = true
	}

	// Task-242 tier-1: prepare Change Contract on coding children so r-contract
	// sees ContractDeclared/scope. Persist only after gate allows (V9-02).
	var (
		contractDeclared      bool
		scopeOutOfScopePaths  []string
		scopeHighSeverity     bool
		headSpecDrifted       bool
		headCodeDrifted       bool
		headAttachSpecPending bool
		prepared              preparedChangeContract
		hasPreparedContract   bool
	)
	if isCodingChild {
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
		changedPaths := changedPathsFromDiff(diff)
		dotFP := filepath.Join(cwd, ".flowpilot")
		suggested := suggestFeatureKeys(dotFP, changedPaths, fin.FinalMessage)
		// Key contract under parent flow run id so appendChangeContractIfAny /
		// change.contract Fetch (parentRunID) can see the coder's declaration.
		contractRunID := rs.id
		if parentID != "" {
			contractRunID = parentID
		}
		stepForContract := strings.TrimSpace(rs.label)
		if stepForContract == "" {
			stepForContract = rs.stepID
		}
		prepared = prepareChangeContract(ctx, cwd, contractRunID, stepForContract, rs.lastFullPrompt, fin.FinalMessage, diff, suggested)
		hasPreparedContract = true
		contractDeclared = prepared.declared
		scopeOutOfScopePaths = prepared.outOfScopePaths
		scopeHighSeverity = prepared.highSeverity
		headSpecDrifted = prepared.specDrifted
		headCodeDrifted = prepared.codeDrifted
		headAttachSpecPending = prepared.attachSpecPending
	}

	// CP-55 P-4: a Flow's agent.code writer is governed by its own frozen
	// preflight contract (Task-264/265), not the legacy declared/inferred
	// Contract path above — that block only ever activates for isDelegate
	// (agent.delegate) children, so an agent.code node never reaches
	// prepareChangeContract/commitChangeContract at all; a post-turn
	// declaration or inference from this node's own final message therefore
	// cannot satisfy Flow preflight (spec's explicit requirement). This is a
	// hardcoded Flow guarantee evaluated unconditionally, like the git-commit
	// check further below, rather than through the configurable flowgate
	// rules engine: Normal chat's r-scope is deliberately lenient (downgrades
	// a configured block to warn unless HighSeverity — flowgate/rules.go),
	// but a Flow's frozen scope must not inherit that leniency.
	isCodeWriterNode := false
	if nodeOK {
		if canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior); ok && canonical == "agent.code" {
			isCodeWriterNode = true
		}
	}
	// coderStepID identifies the step a frozen contract would be bound to.
	// Prefer rs.label (the flow-node id, matching how runContractFreezeNode
	// binds CoderStepID) with rs.stepID as a fallback, mirroring the same
	// fallback the legacy contract-prep block above uses.
	coderStepID := strings.TrimSpace(rs.label)
	if coderStepID == "" {
		coderStepID = rs.stepID
	}
	// Fixed after Claude-agent review (2026-07-31, CA-427 Finding 3): gating
	// enforcement on nodeOK/isCodeWriterNode ALONE meant a runner restart,
	// topology reload, or any other reason flowNodeForRun fails to resolve
	// the live node would silently skip frozen-scope enforcement entirely —
	// the exact "silently let an unbound write through" failure this check
	// exists to prevent. A frozen contract's mere existence, bound to this
	// exact (parentID, coderStepID), is itself durable proof this step is
	// governed — so it is also consulted directly, independent of whether
	// today's in-memory topology happens to resolve. A step that was NEVER
	// frozen (every existing agent.delegate flow, which never calls
	// runContractFreezeNode) still correctly skips enforcement: frozenOK
	// stays false and boundByFrozenContract stays false for it.
	var rec changecontract.FrozenContractRecord
	var frozenOK bool
	if parentID != "" && coderStepID != "" {
		if frozenStore, err := changecontract.NewFrozenStore(cwd); err == nil {
			rec, frozenOK, _ = frozenStore.GetFrozenForStep(parentID, coderStepID)
		}
	}
	if (isCodeWriterNode || frozenOK) && parentID != "" && coderStepID != "" {
		if !frozenOK {
			if !s.gateEpochStillValid(runID, epoch) {
				return true
			}
			msg := "no frozen contract found for this coding step; a Flow writer requires a contract frozen before it runs"
			s.mu.Lock()
			if r := s.runs[runID]; r != nil && r.gateEpoch == epoch {
				s.emitLocked(r, ProviderEvent{
					Type:           EventFlowGateViolation,
					ProviderTurnID: turnID,
					Error:          msg,
					Status:         "block",
				})
			}
			s.mu.Unlock()
			if s.gateEpochStillValid(runID, epoch) {
				_, _ = s.applyFlowControl(parentID, FlowControlInput{
					Status:  "escalate",
					Summary: "flow gate block: " + msg,
				})
			}
			return true
		}
		// Compare against a diff taken from the frozen contract's OWN baseline
		// SHA (not this turn's turnStartGitHead, which — while normally the
		// same commit — is a distinct value the frozen record does not carry
		// an assumption about matching).
		frozenDiff, frozenDiffErr := flowgate.ObserveGitDiffSince(cwd, rec.BaseSHA)
		if frozenDiffErr != nil {
			// Fixed after Claude-agent review (CA-427 Finding 1): this
			// previously degraded to fin.ChangedFiles (AI-tool-call-reported,
			// frequently empty) on an observation error, which let an
			// unverified turn pass with zero drift detected — failing OPEN on
			// exactly the ground truth this check exists to establish. Fail
			// CLOSED instead, matching this same function's own earlier
			// turn-scoped-diff-failure block (lines ~524-551).
			if !s.gateEpochStillValid(runID, epoch) {
				return true
			}
			msg := "gate observation failed against the frozen contract's baseline (workspace diff unreadable): " + frozenDiffErr.Error()
			s.mu.Lock()
			if r := s.runs[runID]; r != nil && r.gateEpoch == epoch {
				s.emitLocked(r, ProviderEvent{
					Type:           EventFlowGateViolation,
					ProviderTurnID: turnID,
					Error:          msg,
					Status:         "block",
				})
			}
			s.mu.Unlock()
			if s.gateEpochStillValid(runID, epoch) {
				_, _ = s.applyFlowControl(parentID, FlowControlInput{
					Status:  "escalate",
					Summary: "flow gate block: " + msg,
				})
			}
			return true
		}
		writtenAgainstFrozen := changedPathsFromDiff(frozenDiff)
		// Fixed after Claude-agent review (CA-427 Finding 2): the freeze
		// itself just wrote .flowpilot/contracts/*.ndjson into this same
		// workspace (Task-264's FrozenStore) — that bookkeeping artifact is
		// not something the coder wrote and must never itself trip drift.
		// The original fix used flowgate.IsDocOrAuditFile, which exempts the
		// ENTIRE .flowpilot/** tree and every *.md file — review correctly
		// flagged this as a real security hole: a writer could silently
		// rewrite .flowpilot/settings/flow-rules.json (disabling the very
		// gate judging it) or forge .flowpilot/contracts/frozen_contracts.ndjson
		// directly, with zero drift ever detected. Exclude ONLY the frozen
		// store's own two known bookkeeping files instead — everything else,
		// including the rest of .flowpilot/** and every doc file, remains
		// fully subject to scope enforcement.
		//
		// CP-55 P-8 finding: PendingCanonicalStore's own two files need the
		// identical exemption — a frozen writer's first gate pass stages a
		// pending Canonical Head update (P-5), and without this, that very
		// write showed up as drift on the SAME writer's next gate pass (any
		// validation-retry or review-loop retry), self-blocking every
		// migrated Flow the moment it looped more than once. This exemption
		// did not exist in P-4/P-5 because no coder had gone through more
		// than one gate pass against a frozen contract until P-8's migrated
		// flows made that a live path.
		codeOnlyWritten := writtenAgainstFrozen[:0:0]
		for _, p := range writtenAgainstFrozen {
			if changecontract.IsFrozenStoreBookkeepingPath(p) || changecontract.IsPendingCanonicalStoreBookkeepingPath(p) {
				continue
			}
			codeOnlyWritten = append(codeOnlyWritten, p)
		}
		drift := changecontract.FrozenContractScopeDrift(rec, codeOnlyWritten)
		if len(drift) > 0 {
			if !s.gateEpochStillValid(runID, epoch) {
				return true
			}
			msg := "flow scope drift: wrote outside the frozen contract's declared paths: " + strings.Join(drift, ", ")
			s.mu.Lock()
			if r := s.runs[runID]; r != nil && r.gateEpoch == epoch {
				s.emitLocked(r, ProviderEvent{
					Type:           EventFlowGateViolation,
					ProviderTurnID: turnID,
					Error:          msg,
					Status:         "block",
				})
			}
			s.mu.Unlock()
			if s.gateEpochStillValid(runID, epoch) {
				_, _ = s.applyFlowControl(parentID, FlowControlInput{
					Status:  "escalate",
					Summary: "flow gate block: " + msg,
				})
			}
			return true
		}
		// Within declared scope — this node does not go through the legacy
		// isCodingChild path at all (isDelegate is false for agent.code, so
		// that block above never ran for it either); fall through to the
		// artifact-output/telegram rule evaluation below exactly as any other
		// node would.
		//
		// CP-55 P-8: prepare this frozen writer's OWN Canonical Head effect
		// here, sourced from its FrozenContractRecord (rec) rather than from
		// prepareChangeContract's legacy declared/inferred parser. Without
		// this, a frozen agent.code writer's gate pass reached the artifact-
		// output check below with hasPreparedContract still false (only ever
		// set true inside the isCodingChild block above, which agent.code can
		// never enter) and commitChangeContract was never called at all for
		// it — no immediate write AND no P-5 staged write, i.e. this node's
		// Canonical Head effect silently did nothing, a gap found during
		// CP-55 P-8's own research. The legacy changecontract.Store is
		// deliberately NOT written here (skipSave: true) — a frozen writer's
		// record of truth is the FrozenContractRecord FrozenStore already
		// holds durably (Task-264/265), not the legacy Store/Contract this
		// synthetic value only exists to drive commitChangeContract's shared
		// Canonical Head plumbing (immediate write or P-5 staging, decided by
		// canonicalRoute below exactly as it is for an agent.delegate coder).
		hasPreparedContract = true
		// Also marks this gate pass as a coding child for every downstream
		// check keyed on isCodingChild (the artifact-output-only early
		// returns just below, the git-commit-during-coding-step check
		// further down, and canonicalRoute's own staging decision) — it was
		// only ever false for agent.code up to this point because the
		// isCodingChild-prep block above this whole frozen-writer branch
		// runs BEFORE this code and reads the OLD value; setting it now does
		// not retroactively re-run that earlier, agent.delegate-specific
		// block (doc-scope/test rules stay correctly un-added to `only` for
		// a frozen writer, since scope enforcement for it is the hardcoded
		// frozen-contract check above, not the configurable flowgate rules
		// engine — matching this file's own established P-4 design decision
		// to keep the two mechanisms separate).
		isCodingChild = true
		prepared = preparedChangeContract{
			ok:  true,
			cwd: cwd,
			contract: changecontract.Contract{
				RunID:         parentID,
				StepID:        coderStepID,
				FeatureKey:    rec.FeatureKey,
				Intent:        rec.Intent,
				DeclaredPaths: rec.DeclaredPaths,
				Confidence:    changecontract.ConfidenceDeclared,
			},
			declared: true,
			skipSave: true,
		}
	}

	// CORRECTION (CP-55 P-8 Claude-agent review, Important Finding 4): this
	// check originally had no `!isCodingChild` guard, unlike the two similar
	// no-op checks right below it — for a frozen agent.code writer, `only` is
	// populated ONLY by the unconditional artifact-rule scan above (the
	// doc-scope/test-rule block is deliberately never entered for it; see the
	// comment on `isCodingChild = true` above). If a workspace ever disables
	// every artifact rule (r-artifact-output/-structure/-telegram-sent — a
	// plausible cleanup for a flow with no artifact bindings, like
	// review-loop), `only` becomes empty and this would have discarded the
	// frozen writer's own prepared Canonical Head effect silently — exactly
	// the "this node's Canonical Head effect does nothing" bug this phase's
	// own fix (above) exists to close, just conditionally on rule
	// configuration this check had no defensive coupling to.
	if len(only) == 0 && !isCodingChild {
		return false
	}
	// Artifact-only path with no bindings and no coding/doc rules selected â†’ no-op.
	if len(required) == 0 && len(structured) == 0 && len(telegramSends) == 0 && !isCodingChild {
		return false
	}
	if len(required) == 0 && len(structured) == 0 && len(telegramSends) == 0 && len(diff) == 0 && !isCodingChild {
		return false
	}

	written := fin.ChangedFiles
	if len(written) == 0 && len(pendingPaths) > 0 {
		// Re-check prior coding paths until remediation (BUG-288 #8).
		written = pendingPaths
	}
	tr := flowgate.TurnResult{
		RunID:                                 rs.id,
		StepID:                                rs.stepID,
		FinalMessage:                          fin.FinalMessage,
		WrittenPaths:                          written,
		WorkspaceCwd:                          cwd,
		GitDiff:                               diff,
		ChangedPaths:                          changedPathsFromDiff(diff),
		ChangeType:                            changeType,
		RequiredFileArtifactOutputs:           required,
		RequiredStructuredFileArtifactOutputs: structured,
		RequiredTelegramSends:                 telegramSends,
		ContractDeclared:                      contractDeclared,
		ScopeOutOfScopePaths:                  scopeOutOfScopePaths,
		ScopeHighSeverity:                     scopeHighSeverity,
		HeadSpecDrifted:                       headSpecDrifted,
		HeadCodeDrifted:                       headCodeDrifted,
		HeadAttachSpecPending:                 headAttachSpecPending,
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
		// BUG-288 P2-04: cancellable via the caller's ctx (see runFlowGateAtEpoch).
		s.ensureBaselineReadyContext(ctx, cwd)
		dotFP := filepath.Join(cwd, ".flowpilot")
		baseline, blErr := flowgate.LoadBaseline(dotFP)
		if blErr != nil {
			// BUG-288 R13-13: fail-closed when suite rules need baseline truth.
			log.Printf("[gate] LoadBaseline (child) failed cwd=%q: %v (failing closed)", cwd, blErr)
			msg := "gate baseline unreadable (test_baseline.json corrupt or unreadable): " + blErr.Error()
			if s.gateEpochStillValid(runID, epoch) {
				s.mu.Lock()
				if rs2 := s.runs[runID]; rs2 != nil && rs2.gateEpoch == epoch {
					s.emitLocked(rs2, ProviderEvent{
						Type:           EventFlowGateViolation,
						ProviderTurnID: turnID,
						Error:          msg,
						Status:         "block",
					})
				}
				s.mu.Unlock()
			}
			if parentID != "" && s.gateEpochStillValid(runID, epoch) {
				_, _ = s.applyFlowControl(parentID, FlowControlInput{
					Status:  "escalate",
					Summary: "flow gate block: " + msg,
				})
			}
			return true
		}
		overrides, _ := flowgate.LoadOverrides(dotFP)
		oracle := flowgate.RunOracleContext(ctx, cwd, baseline, diff, overrides)
		if s.gateBlindBlocksTurn(runID, turnID, epoch, rs, dotFP, baseline, oracle, diff) {
			if parentID != "" && s.gateEpochStillValid(runID, epoch) {
				_, _ = s.applyFlowControl(parentID, FlowControlInput{
					Status:  "escalate",
					Summary: "flow gate block: gate_blind",
				})
			}
			return true
		}
		// V9-27: split ordinary Failed vs true Regressed for r-tests / r-reg.
		var failedTests, regressedTests []string
		if oracle.EnvError == "" {
			if oracle.HasRegression {
				regressedTests = append(regressedTests, oracle.Regressed...)
			}
			if !oracle.SuitePassed && !oracle.HasRegression {
				if len(oracle.Failed) > 0 {
					// BUG-289 M6/F-12: honor IsOverridden on child failedTests path.
					for _, t := range oracle.Failed {
						if !flowgate.IsOverridden(overrides, t) {
							failedTests = append(failedTests, t)
						}
					}
				} else if baseline != nil && baseline.SuitePassed {
					failedTests = []string{"suite_failed"}
				}
			}
		}
		tr.Tests = flowgate.TestOutcome{
			Ran:       baseline != nil && oracle.EnvError == "",
			Failed:    failedTests,
			Regressed: regressedTests,
		}
	}

	// V10 P0: Gemini (and any adapter without RequestApproval) may still create
	// commits under YOLO. Detect new commits since turn base and block coding
	// children â€” commit is reserved for audit/commit-prep (Task-242 D-7).
	if isCodingChild && len(collectCommitSubjectsSince(cwd, baseSHA)) > 0 {
		if !s.gateEpochStillValid(runID, epoch) {
			return true
		}
		msg := "flow coding step created a git commit; commits are reserved for the audit/commit-prep step"
		s.mu.Lock()
		if r := s.runs[runID]; r != nil && r.gateEpoch == epoch {
			s.emitLocked(r, ProviderEvent{
				Type:           EventFlowGateViolation,
				ProviderTurnID: turnID,
				Error:          msg,
				Status:         "block",
			})
		}
		s.mu.Unlock()
		if parentID != "" && s.gateEpochStillValid(runID, epoch) {
			_, _ = s.applyFlowControl(parentID, FlowControlInput{
				Status:  "escalate",
				Summary: "flow gate block: " + msg,
			})
		}
		return true
	}

	// CP-55 P-5: a genuine Flow-engine-driven coding child stages its
	// Canonical Head effect instead of writing it immediately — the real
	// Head file is only ever touched at Flow terminal acceptance
	// (finalizePendingCanonicalHeadsForRun, applyFlowControl's "done" case).
	// A coding child outside a Flow-engine-driven run (isCodingChild can be
	// true even without flowEngineDriven, e.g. an ad hoc spawn_agent target)
	// keeps today's immediate-write behavior. CP-55 P-8: a frozen agent.code
	// writer now also sets isCodingChild=true (above, once its own frozen
	// scope is confirmed clean) so it gets the identical staging decision.
	canonicalRoute := canonicalPendingRoute{}
	if isCodingChild && s.isFlowEngineDriven(parentID) {
		canonicalRoute = canonicalPendingRoute{ParentRunID: parentID, CoderStepID: coderStepID}
	}

	violations := flowgate.Evaluate(tr, only)
	if len(violations) == 0 {
		// BUG-288 R16-P0: durable commit under s.mu (no TOCTOU with Stop).
		if hasPreparedContract {
			if !s.withGateEpochDurable(runID, epoch, func() error {
				return commitChangeContract(cwd, prepared, canonicalRoute, s.markerSecret)
			}) {
				// BUG-289 H3/F-3: child path — emit + parent escalate (not silent).
				msg := "gate change-contract/head commit failed (canonical head unreadable or unwritable); blocked fail-closed"
				if s.gateEpochStillValid(runID, epoch) {
					s.mu.Lock()
					if r := s.runs[runID]; r != nil && r.gateEpoch == epoch {
						s.emitLocked(r, ProviderEvent{
							Type:           EventFlowGateViolation,
							ProviderTurnID: turnID,
							Error:          msg,
							Status:         "block",
						})
					}
					s.mu.Unlock()
					if parentID != "" {
						_, _ = s.applyFlowControl(parentID, FlowControlInput{
							Status:  "escalate",
							Summary: "flow gate block on coding step: " + msg,
						})
					}
				}
				return true
			}
		} else if !s.gateEpochStillValid(runID, epoch) {
			return true
		}
		s.mu.Lock()
		if r := s.runs[runID]; r != nil && r.gateEpoch == epoch {
			r.pendingGateCodePaths = nil
			r.gateFixCodeActive = false
			r.gateFixCodeAttempts = 0
		}
		s.mu.Unlock()
		return false
	}
	result := flowgate.Enforce(violations, loadGateMode(filepath.Join(cwd, ".flowpilot")))
	if !s.gateEpochStillValid(runID, epoch) {
		return true
	}
	// Remember code paths so empty-diff retry still re-checks (BUG-288 #8).
	pathsToHold := tr.WrittenPaths
	if len(pathsToHold) == 0 {
		pathsToHold = changedPathsFromDiff(diff)
	}
	// Extract r-reg options for child decision card (BUG-289 L3/F-12).
	var gateOptions []string
	var gateRegressedTests []string
	for _, v := range violations {
		if v.Rule.ID == "r-reg" && len(v.Options) > 0 {
			gateOptions = v.Options
			gateRegressedTests = v.RegressedTests
			break
		}
	}
	s.mu.Lock()
	rs = s.runs[runID]
	if rs == nil || rs.gateEpoch != epoch {
		s.mu.Unlock()
		return true
	}
	if len(pathsToHold) > 0 {
		rs.pendingGateCodePaths = append([]string(nil), pathsToHold...)
	}
	// CP-51 run-11262: user already chose keep-test-fix-code — do not re-open
	// the decision modal; auto-reprompt with a write-hard prompt a few times.
	autoFixPrompt := ""
	autoFixStep := ""
	emitOptions := gateOptions
	if result.Action == "block" && len(gateOptions) > 0 {
		emitOptions, autoFixPrompt, autoFixStep = applyGateFixCodeAutoRepromptLocked(rs, runID, gateOptions, gateRegressedTests)
	}
	// BUG-289 L3/F-12: populate pendingGateBlock/GateOptions on child regression
	// escalate so the user gets the same keep-test card as the root path.
	if result.Action == "block" && len(emitOptions) > 0 {
		rs.pendingGateBlock = &gateBlockInfo{
			regressedTests: gateRegressedTests,
			stepID:         rs.lastTurnStepID,
		}
	}
	// Auto fix-code path: emit as reprompt (not block) so desktop does not open
	// GateBlockModal / "Got it" overlay while we silently re-drive the coder.
	emitStatus := result.Action
	if autoFixPrompt != "" {
		emitStatus = "reprompt"
	}
	s.emitLocked(rs, ProviderEvent{
		Type:               EventFlowGateViolation,
		ProviderTurnID:     turnID,
		Error:              result.Message,
		Status:             emitStatus,
		GateOptions:        emitOptions,
		GateRegressedTests: gateRegressedTests,
	})
	if autoFixPrompt != "" {
		rs.pendingGateRepromptPrompt = autoFixPrompt
		rs.pendingGateRepromptStepID = autoFixStep
		rs.pendingGateRepromptGen++
	}
	s.mu.Unlock()
	switch result.Action {
	case "block":
		// Tier-2b always-block and tier-1 block → parent escalate (actionable),
		// not an unanswerable child hang (Task-242 T-9 / BUG-288 #9).
		//
		// Exception (CP-51 A1 live / dual-UI): when the child already has a
		// regression decision card (r-reg options), do NOT also escalate the
		// hub. Dual surfaces (GateBlockModal + FlowAwaitingUserCard) caused
		// operators to Continue the hub while the child gate was still open,
		// then hang on "post-turn gate still running". Child SubmitGateDecision
		// owns remediation; the next post-turn gate re-checks remaining rules.
		// Auto fix-code reprompt also suppresses hub escalate.
		if parentID != "" && s.gateEpochStillValid(runID, epoch) && len(emitOptions) == 0 && autoFixPrompt == "" {
			_, _ = s.applyFlowControl(parentID, FlowControlInput{
				Status:  "escalate",
				Summary: "flow gate block on coding step: " + result.Message,
			})
		}
		return true
	case "reprompt":
		if !s.gateEpochStillValid(runID, epoch) {
			return true
		}
		s.mu.Lock()
		rs = s.runs[runID]
		if rs == nil || rs.gateEpoch != epoch {
			s.mu.Unlock()
			return true
		}
		attempts := rs.repromptAttempts
		rs.repromptAttempts++
		stepID := rs.lastTurnStepID
		runID := rs.id
		parentRunID := rs.parentRunID
		ws := rs.workspaceCwd
		s.mu.Unlock()
		log.Printf("[gate] child gate reprompt attempt=%d run=%q stepID=%q",
			attempts, runID, stepID)
		if attempts < maxFlowGateReprompts {
			// Re-attach node OUTPUT contracts + parent-run Change Contract so the
			// retry still sees declared scope (Task-247 / CP-50 P-4).
			prompt := flowgate.RepromptPrompt(result)
			if nodeOK {
				prompt = composeFlowNodeAgentPrompt(ws, prompt, node)
			}
			if parentRunID != "" {
				prompt = appendChangeContractIfAnyWithSecret(ws, parentRunID, prompt, s.markerSecret)
			}
			// V10R3 P0: queue reprompt until settle/cancel/turnInFlight clear.
			s.mu.Lock()
			if r := s.runs[runID]; r != nil {
				r.pendingGateRepromptPrompt = prompt
				r.pendingGateRepromptStepID = stepID
				r.pendingGateRepromptGen++
			}
			s.mu.Unlock()
			return true
		}
		// BUG-288 #9: exhausted reprompt budget â€” escalate to human, do not wedge.
		if parentRunID != "" {
			_, _ = s.applyFlowControl(parentRunID, FlowControlInput{
				Status:  "escalate",
				Summary: "flow gate: max reprompts exceeded on coding step: " + result.Message,
			})
		}
		return true
	}
	// warn/approve: still commit prepared contract (turn allowed).
	// BUG-288 R16-P0: durable commit under s.mu.
	if hasPreparedContract {
		if !s.withGateEpochDurable(runID, epoch, func() error {
			return commitChangeContract(cwd, prepared, canonicalRoute, s.markerSecret)
		}) {
			return true
		}
	}
	return false
}

// isFlowCodeWritingChild reports whether a flow child should run tier-1 doc/scope
// rules and Change Contract capture (Task-242). Reviewers also use agent.delegate
// and share the coder's uncommitted diff â€” they must not capture/overwrite the
// parent-run contract or be reprompted for lacking a declaration.
//
// Activation (Task-242 / BUG-288 #14/#24):
//   - Reviewer with empty/non-code turn-diff: zero-cost (exclude).
//   - Reviewer who actually changes code this turn: still gate.
//   - Primary signal: non-empty per-turn git code-diff.
//   - WrittenPaths only when git observation yielded no code-diff (provider wrote
//     but net dirty snapshot empty / git failed) â€” avoids write-then-revert false +.
func isFlowCodeWritingChild(rs *interactiveRun, node agentpack.FlowNode, nodeOK bool, writtenPaths []string, diff []flowgate.ChangedFile) bool {
	if rs == nil {
		return false
	}
	hasDiff := flowgate.HasCodeChanges(diff)
	hasWrites := flowgate.HasCodeChangesInList(writtenPaths)
	if isFlowReviewerChild(rs, node, nodeOK) {
		// Zero-cost only when this turn did not mutate code.
		// V9-22: prefer git code-diff; WrittenPaths alone is weaker (write-revert).
		return hasDiff || (hasWrites && len(diff) == 0)
	}
	if hasDiff {
		return true
	}
	// V9-22: WrittenPaths fallback only when git observation produced no code
	// delta (parser empty / error), not when AI wrote then reverted to zero net.
	return hasWrites && len(diff) == 0
}

// isFlowReviewerChild detects genuine review agents via explicit pack/spawn
// role or agent catalog name â€” NOT substring matching on labels like
// "review_and_fix" (those remain code-writing and must run tier-1).
func isFlowReviewerChild(rs *interactiveRun, node agentpack.FlowNode, nodeOK bool) bool {
	if rs == nil {
		return false
	}
	// Exact role from agent definition / spawn (pack: role: reviewer).
	switch strings.ToLower(strings.TrimSpace(rs.role)) {
	case "reviewer", "review":
		return true
	}
	// Exact agent catalog name (agents/reviewer.md â†’ "reviewer").
	if strings.EqualFold(strings.TrimSpace(rs.agentName), "reviewer") {
		return true
	}
	if nodeOK {
		if strings.EqualFold(flowNodeAgentName(node), "reviewer") {
			return true
		}
	}
	return false
}

// snapshotWorktreeFingerprints records dirty path â†’ content fingerprint at turn
// start so the gate can attribute only this turn's worktree mutations.
func snapshotWorktreeFingerprints(cwd string) map[string]string {
	if strings.TrimSpace(cwd) == "" {
		return nil
	}
	files, err := flowgate.ObserveGitDiff(cwd)
	if err != nil || len(files) == 0 {
		return nil
	}
	out := make(map[string]string, len(files))
	for _, f := range files {
		// V10R3 P1: do not TrimSpace â€” whitespace is significant in git -z paths
		// (V10-07); trimming remaps " foo.go " â†’ "foo.go" and breaks attribution.
		p := filepath.ToSlash(f.Path)
		if p == "" {
			continue
		}
		out[p] = worktreeFileFingerprint(cwd, p)
	}
	return out
}

// isNotAGitRepoErr reports whether err is git's specific "not a git
// repository" failure (exit 128, stderr containing that phrase) rather than
// some other observation failure. cmd.Output() populates *exec.ExitError.Stderr
// when the command's own Stderr was left nil, which is the case for every
// git invocation in this package (BUG-288 P1-17 â€” see observeTurnScopedDiff).
func isNotAGitRepoErr(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	return strings.Contains(strings.ToLower(string(exitErr.Stderr)), "not a git repository")
}

// observeTurnScopedDiff returns commits since baseSHA plus uncommitted paths
// that are new or whose content fingerprint changed since turnStartWorktree.
// BUG-288 P1-17: the returned error MUST be checked by the caller and treated
// as fail-closed (block/escalate) â€” it means the workspace genuinely could
// not be read (e.g. git failure), which is NOT the same as "no changes".
// Returning the empty diff silently on error previously let Tier-1 gate
// evaluation see an empty diff and pass even though the real diff was
// unknown, masking real changes.
func observeTurnScopedDiff(cwd, baseSHA string, turnStartWorktree map[string]string) ([]flowgate.ChangedFile, error) {
	if strings.TrimSpace(cwd) == "" {
		return nil, nil
	}
	// Full view (committed since base + current dirty), then filter uncommitted
	// that were already dirty with the same content at turn start.
	full, err := flowgate.ObserveGitDiffSince(cwd, baseSHA)
	if err != nil {
		if isNotAGitRepoErr(err) {
			// Task-242 D-2 / BUG-288 R13-10: carve-out only when the turn never
			// observed a real repo (no base SHA and no worktree snapshot). If
			// turn start had a HEAD/worktree, ".git disappeared mid-turn" is a
			// real observation failure â€” fail closed, do not fabricate empty.
			if strings.TrimSpace(baseSHA) == "" && len(turnStartWorktree) == 0 {
				return nil, nil
			}
			return nil, fmt.Errorf("observe git diff since %q: workspace was a git repo at turn start but is no longer readable: %w", baseSHA, err)
		}
		return nil, fmt.Errorf("observe git diff since %q: %w", baseSHA, err)
	}
	if len(full) == 0 {
		return full, nil
	}
	if len(turnStartWorktree) == 0 {
		// No snapshot (legacy / empty) â€” fall back to full view.
		return full, nil
	}
	// Paths currently dirty (uncommitted only) for fingerprint compare.
	// BUG-288 R13-12: second observation must also fail-closed (not swallow).
	currentDirty, err2 := flowgate.ObserveGitDiff(cwd)
	if err2 != nil {
		return nil, fmt.Errorf("observe current dirty worktree: %w", err2)
	}
	dirtyNow := make(map[string]flowgate.ChangedFile, len(currentDirty))
	for _, f := range currentDirty {
		dirtyNow[filepath.ToSlash(f.Path)] = f
	}
	var out []flowgate.ChangedFile
	seen := map[string]bool{}
	for _, f := range full {
		p := filepath.ToSlash(f.Path)
		if p == "" || seen[p] {
			continue
		}
		if cur, isDirty := dirtyNow[p]; isDirty {
			// Uncommitted: only include if new or content changed this turn.
			prev, wasDirty := turnStartWorktree[p]
			fp := worktreeFileFingerprint(cwd, p)
			if wasDirty && prev == fp {
				// Pre-existing dirt unchanged â€” not this turn's edit.
				_ = cur
				continue
			}
		}
		// Committed-since-base (not in dirtyNow) or dirty that changed â†’ keep.
		seen[p] = true
		out = append(out, f)
	}
	return out, nil
}

func worktreeFileFingerprint(cwd, relPath string) string {
	full := filepath.Join(cwd, filepath.FromSlash(relPath))
	// Do not follow symlinks outside a normal open (O_NOFOLLOW best-effort via Lstat).
	fi, err := os.Lstat(full)
	if err != nil {
		return "missing:" + relPath
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, _ := os.Readlink(full)
		sum := sha256.Sum256([]byte("symlink:" + target))
		return hex.EncodeToString(sum[:16])
	}
	f, err := os.Open(full)
	if err != nil {
		return "missing:" + relPath
	}
	defer f.Close()
	h := sha256.New()
	// V9-14: stream entire file (bounded memory via io.Copy buffer). A 1MiB
	// prefix-only hash missed tail edits that kept the same size.
	_, _ = io.Copy(h, f)
	fmt.Fprintf(h, "|size=%d|mode=%v", fi.Size(), fi.Mode())
	return hex.EncodeToString(h.Sum(nil)[:16])
}

func appendUniqueStrings(dst []string, add ...string) []string {
	seen := make(map[string]bool, len(dst)+len(add))
	for _, s := range dst {
		seen[s] = true
	}
	for _, s := range add {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		dst = append(dst, s)
	}
	return dst
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
// a flow-engine coding child (Task-242 T-4). Token-aware â€” does not match
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
// insert global options before the verb (Codex review Important #1) and common
// shell wrappers (BUG-288 #7):
//
//	git commit -m x
//	git -C . commit -m x
//	sh -c 'git commit -m x'
//	bash -lc "git commit -m x"
//	/usr/bin/git --git-dir=... commit
func looksLikeGitCommitCommand(cmd string) bool {
	// Strip outer shell wrappers so nested git commit is still denied.
	for {
		fields := shellFields(cmd)
		if len(fields) < 2 {
			break
		}
		base := fields[0]
		if j := strings.LastIndexAny(base, `/\`); j >= 0 {
			base = base[j+1:]
		}
		// sh/bash -c/-lc 'payload'
		if base == "sh" || base == "bash" || base == "zsh" || base == "dash" {
			// find -c / -lc and take following payload as new cmd
			for i := 1; i < len(fields)-1; i++ {
				if fields[i] == "-c" || fields[i] == "-lc" {
					cmd = fields[i+1]
					goto reparse
				}
			}
		}
		break
	reparse:
		continue
	}
	fields := shellFields(cmd)
	// Also scan raw command for "git commit" after common wrappers when nested
	// parsing failed (e.g. env VAR=x git commit).
	if looksLikeGitCommitFields(fields) {
		return true
	}
	// Fallback: unwrap any token that embeds "git commit" after a shell -c payload
	// already expanded by shellFields.
	low := strings.ToLower(cmd)
	if strings.Contains(low, "git commit") || strings.Contains(low, "git.exe commit") {
		// Avoid "git commitment" false positive: require commit as its own token.
		return looksLikeGitCommitFields(shellFields(strings.ReplaceAll(strings.ReplaceAll(cmd, "&&", " "), ";", " ")))
	}
	return false
}

func looksLikeGitCommitFields(fields []string) bool {
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
		prompt = keepTestFixCodePrompt(tests, 0)
		s.mu.Lock()
		if rs2 := s.runs[runID]; rs2 != nil {
			// Arm silent auto-reprompt path so the decision modal is not re-shown
			// every turn when the model fails to write (CP-51 run-11262).
			rs2.gateFixCodeActive = true
			rs2.gateFixCodeAttempts = 0
		}
		s.mu.Unlock()
	case "suggest-requirement-change":
		prompt = buildSuggestRequirementPrompt(info, cwd)
		// Mark the upcoming turn as a proposal turn so runFlowGate does not
		// re-block on r-reg/r-tests — the AI is proposing, not fixing code yet.
		s.mu.Lock()
		if rs2 := s.runs[runID]; rs2 != nil {
			rs2.proposalTurnPending = true
			rs2.gateFixCodeActive = false
			rs2.gateFixCodeAttempts = 0
		}
		s.mu.Unlock()
	case "custom":
		if strings.TrimSpace(customText) == "" {
			return newAPIErr(400, "invalid_request", "customText is required for option 'custom'")
		}
		prompt = customText
		s.mu.Lock()
		if rs2 := s.runs[runID]; rs2 != nil {
			rs2.gateFixCodeActive = false
			rs2.gateFixCodeAttempts = 0
		}
		s.mu.Unlock()
	default:
		return newAPIErr(400, "invalid_option", "option must be: keep-test-fix-code | suggest-requirement-change | custom")
	}

	log.Printf("[gate-decision] runID=%q option=%q", runID, option)

	go func() {
		_, _ = s.startTurn(runID, TurnInput{StepID: stepID, Prompt: prompt}, "", "")
	}()
	return nil
}

// keepTestFixCodePrompt builds the remediation prompt after the user chooses
// keep-test-fix-code. attempt>0 adds a hard "you must write" nudge used by
// the silent auto-reprompt path when the previous turn produced no fix.
func keepTestFixCodePrompt(tests string, attempt int) string {
	base := fmt.Sprintf(
		"The flow gate detected a regression in %s. The user chose: keep test + requirement → fix the code.\n\n"+
			"You MUST apply the fix by calling the write/edit tool on the production source file(s) so that %s passes.\n"+
			"Do not end the turn only describing the fix or only asking for write permission in chat text.\n"+
			"If a write tool requires approval, request that tool permission and wait — do not claim the fix is done until the file on disk has changed.\n"+
			"Do not modify, delete, or weaken any pre-existing test file. The test is the source of truth.",
		tests, tests)
	if attempt > 0 {
		base += fmt.Sprintf(
			"\n\n[retry %d] Previous remediation turn did not clear the regression (suite still red, or no source write was recorded). "+
				"Open the failing production source now and apply the write tool immediately.",
			attempt)
	}
	return base
}

// applyGateFixCodeAutoRepromptLocked decides whether an r-reg block should
// suppress the decision modal and queue a silent fix-code reprompt.
// Caller holds s.mu. Returns (emitOptions, autoPrompt, autoStep).
func applyGateFixCodeAutoRepromptLocked(rs *interactiveRun, runID string, gateOptions, gateRegressedTests []string) (emitOptions []string, autoPrompt, autoStep string) {
	emitOptions = gateOptions
	if rs == nil || len(gateOptions) == 0 || !rs.gateFixCodeActive {
		return emitOptions, "", ""
	}
	if rs.gateFixCodeAttempts < maxGateFixCodeAutoReprompts {
		rs.gateFixCodeAttempts++
		attempt := rs.gateFixCodeAttempts
		tests := strings.Join(gateRegressedTests, ", ")
		if tests == "" {
			tests = "the regressed tests"
		}
		autoPrompt = keepTestFixCodePrompt(tests, attempt)
		autoStep = rs.lastTurnStepID
		emitOptions = nil
		log.Printf("[gate] auto fix-code reprompt attempt=%d run=%q (suppress decision modal)", attempt, runID)
		return emitOptions, autoPrompt, autoStep
	}
	rs.gateFixCodeActive = false
	rs.gateFixCodeAttempts = 0
	log.Printf("[gate] fix-code auto budget exhausted run=%q; re-showing decision card", runID)
	return emitOptions, "", ""
}

// RecordGateAgreement records that the user agreed to the AI's opt-2 requirement
// proposal and writes the per-test override. Called from handleGateAgreement. (Task-155)
func (s *InteractiveService) RecordGateAgreement(runID string, testNames []string, reason string) *apiErr {
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		return newAPIErr(404, "run_not_found", "workflow run not found")
	}
	dotFP := filepath.Join(rs.workspaceCwd, ".flowpilot")
	stepID := rs.lastTurnStepID
	s.mu.Unlock()

	if strings.TrimSpace(reason) == "" {
		return newAPIErr(400, "reason_required", "waiver reason is required when accepting a test override")
	}

	// Write human-confirmed override for each agreed test name.
	for _, name := range testNames {
		if name == "" {
			continue
		}
		if err := flowgate.SaveOverrideWithReason(dotFP, flowgate.Override{TestName: name, HumanConfirm: true}, reason, runID, "operator", flowgate.DefaultWaiverTTL); err != nil {
			return newAPIErr(422, "waiver_save_failed", err.Error())
		}
	}
	s.recordGateOverrideMetric(dotFP, rs, testNames)

	tests := strings.Join(testNames, ", ")
	if tests == "" {
		tests = "the agreed-upon tests"
	}
	prompt := fmt.Sprintf(
		"The user has explicitly agreed to the proposed requirement change. Override recorded for: %s.\n\n"+
			"Now perform both steps in order:\n"+
			"1. Update the governing requirement in `requirements/05-System-Specs/` to match the agreed behavior "+
			"(create the file if none exists â€” `SS-<N>-<short-title>.md`). "+
			"User intent is authoritative â€” follow it even if the requirement seems unusual, but flag any concern briefly.\n"+
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
				"The requirement in `requirements/05-System-Specs/` is the source of truth â€” user intent "+
				"is authoritative. If the user wants this behavior even if it seems incorrect, follow it "+
				"and only flag a concern. Once the user agrees, call the gate-agreement endpoint or indicate "+
				"agreement in your reply so the override can be recorded.",
			tests, specPath)
	}

	// Empty-spec path (T-6): no governing spec in 05-System-Specs â€” degrade to suggest-or-input.
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
		// V10R3 P1: preserve path bytes (no TrimSpace) so gate scope matches
		// the -z parser and worktree snapshot keys.
		if file.Path != "" {
			out = append(out, filepath.ToSlash(file.Path))
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

// preparedChangeContract holds contract/scope evaluation before durable side
// effects. commitChangeContract performs Save + Head update only after gate allow
// (V9-02). skipSave preserves an existing declared contract against hub overwrite (V9-04).
type preparedChangeContract struct {
	ok                bool
	cwd               string
	contract          changecontract.Contract
	declared          bool
	skipSave          bool
	outOfScopePaths   []string
	highSeverity      bool
	specDrifted       bool
	codeDrifted       bool
	attachSpecPending bool
}

// prepareChangeContract builds contract + scope flags WITHOUT Save/Head update.
//
// CP-43 P-1 Plan A: also accepts the turn's user prompt as a fallback source.
// D-1 originally parsed only finalMessage (assistant turn). When a user/operator
// pre-declares [Change Contract] in the prompt (C1), the gate must see it even
// if the AI does not echo the block. Try finalMessage first; if absent, try
// prompt before falling back to inferred. V9-04's "do not overwrite declared"
// guard also checks the prompt so a prompt-declared contract is not silently
// dropped.
func prepareChangeContract(ctx context.Context, cwd, runID, stepID, prompt, finalMessage string, diff []flowgate.ChangedFile, suggestedFeatureKeys []string) preparedChangeContract {
	out := preparedChangeContract{cwd: cwd}
	if cwd == "" {
		return out
	}
	store, err := changecontract.NewStore(cwd)
	if err != nil {
		log.Printf("[changecontract] store open failed: %v", err)
		return out
	}
	featureKey := ""
	if len(suggestedFeatureKeys) > 0 {
		featureKey = suggestedFeatureKeys[0]
	}

	// V9-04: do not let inferred hub/root contracts overwrite a declared coder contract.
	if existing, ok := store.GetLatestForRun(runID); ok && existing.Confidence == changecontract.ConfidenceDeclared {
		_, declaredNow := changecontract.ParseDeclaration(finalMessage)
		_, promptDeclared := changecontract.ParseDeclaration(prompt)
		if !declaredNow && !promptDeclared {
			out.ok = true
			out.contract = existing
			out.declared = true
			out.skipSave = true
			out.outOfScopePaths, _ = changecontract.ScopeDiff(existing, diff, nil)
			if len(out.outOfScopePaths) > 0 {
				hasGitNexus := tooling.CheckTool("gitnexus", cwd).Status == "ok"
				sp := structure.New(cwd, hasGitNexus)
				out.highSeverity = changecontract.HighSeverity(ctx, sp, out.outOfScopePaths)
			}
			fillHeadDriftFlags(cwd, existing, len(out.outOfScopePaths) > 0, &out)
			return out
		}
	}

	c, declared := changecontract.ParseDeclaration(finalMessage)
	if !declared && strings.TrimSpace(prompt) != "" {
		if pc, ok := changecontract.ParseDeclaration(prompt); ok {
			c = pc
			declared = true
		}
	}
	if declared {
		if c.FeatureKey == "" {
			c.FeatureKey = featureKey
		}
	} else {
		c = changecontract.InferFromDiff(featureKey, diff)
	}
	c.RunID = runID
	c.StepID = stepID
	out.ok = true
	out.contract = c
	out.declared = declared
	out.outOfScopePaths, _ = changecontract.ScopeDiff(c, diff, nil)
	if len(out.outOfScopePaths) > 0 {
		hasGitNexus := tooling.CheckTool("gitnexus", cwd).Status == "ok"
		sp := structure.New(cwd, hasGitNexus)
		out.highSeverity = changecontract.HighSeverity(ctx, sp, out.outOfScopePaths)
	}
	// Declared contracts may be saved immediately so reprompt/re-entry inject
	// still sees them (V9-02 blocks only Head + inferred save until allow).
	if declared {
		if err := store.Save(c); err != nil {
			log.Printf("[changecontract] save declared failed: %v", err)
		}
		out.skipSave = true // already saved; commit only updates Head
	}
	// V10 P0: compute Head drift flags for Evaluate WITHOUT updating Head.
	// updateCanonicalHead (which mutates Head) still runs only in commitChangeContract.
	fillHeadDriftFlags(cwd, out.contract, len(out.outOfScopePaths) > 0, &out)
	return out
}

// fillHeadDriftFlags sets spec/code/attach flags from the on-disk Head for gate
// evaluation (V10). Does not SaveHead â€” that remains post-allow only.
func fillHeadDriftFlags(cwd string, c changecontract.Contract, hasOutOfContract bool, out *preparedChangeContract) {
	if out == nil || strings.TrimSpace(c.FeatureKey) == "" {
		return
	}
	head, found, err := changecontract.LoadHead(cwd, c.FeatureKey)
	if err != nil || !found {
		return
	}
	dotFP := filepath.Join(cwd, ".flowpilot")
	catalog, _ := featurecatalog.LoadCatalog(dotFP)
	if head.SpecConfidence == changecontract.SpecConfidenceSpecLess && catalog != nil {
		if feat, ok := catalog.Get(c.FeatureKey); ok && len(feat.DocRefs) > 0 {
			out.attachSpecPending = true
		}
	}
	out.specDrifted = changecontract.SpecDrifted(cwd, head)
	out.codeDrifted = changecontract.CodeDrifted(hasOutOfContract, out.specDrifted)
}

// commitChangeContract persists Save (inferred) + Canonical Head only after gate allow (V9-02).
// BUG-288 R17-P1: returns error on I/O failure so the gate can fail-closed
// (block completion) instead of allowing a turn through with no durable contract.
//
// route.ParentRunID non-empty (CP-55 P-5) means this commit is a Flow coder
// child's gate pass: the Change Contract itself still saves immediately
// (the legacy Contract/Store is unaffected by P-5 — only Canonical Head
// timing changes), but the Canonical Head update is staged to
// PendingCanonicalStore instead of written to the real Head file, which is
// only ever touched at genuine Flow terminal acceptance
// (finalizePendingCanonicalHeadsForRun). An empty route (root gate, Normal
// chat, legacy captureChangeContract) keeps the original immediate-write
// behavior unchanged.
func commitChangeContract(cwd string, p preparedChangeContract, route canonicalPendingRoute, secret []byte) error {
	if !p.ok || cwd == "" {
		return nil
	}
	store, err := changecontract.NewStore(cwd)
	if err != nil {
		log.Printf("[changecontract] store open failed on commit: %v", err)
		return err
	}
	if !p.skipSave {
		if err := store.Save(p.contract); err != nil {
			log.Printf("[changecontract] save failed: %v", err)
			return err
		}
	}
	if p.contract.FeatureKey != "" {
		if route.ParentRunID != "" {
			if err := stagePendingCanonicalHead(cwd, route.ParentRunID, route.CoderStepID, p.contract, len(p.outOfScopePaths) > 0, secret); err != nil {
				return err
			}
		} else {
			// BUG-288 R18-6: head I/O must fail-closed with contract durability.
			if _, _, _, err := updateCanonicalHead(cwd, p.contract, len(p.outOfScopePaths) > 0); err != nil {
				return err
			}
		}
	}
	return nil
}

// canonicalPendingRoute tells commitChangeContract whether the Canonical
// Head effect of this commit must be staged pending Flow terminal acceptance
// (CP-55 P-5) instead of written immediately. A zero value (empty
// ParentRunID) means "write immediately" — root gate, Normal chat, and the
// legacy captureChangeContract path all pass this zero value, preserving
// their exact pre-P-5 behavior.
type canonicalPendingRoute struct {
	ParentRunID string
	CoderStepID string
}

// captureChangeContract is the legacy all-in-one path (tests / non-gate callers).
func captureChangeContract(ctx context.Context, cwd, runID, stepID, finalMessage string, diff []flowgate.ChangedFile, suggestedFeatureKeys []string) (declared bool, outOfScopePaths []string, highSeverity, specDrifted, codeDrifted, attachSpecPending bool) {
	p := prepareChangeContract(ctx, cwd, runID, stepID, "", finalMessage, diff, suggestedFeatureKeys)
	_ = commitChangeContract(cwd, p, canonicalPendingRoute{}, nil)
	return p.declared, p.outOfScopePaths, p.highSeverity, p.specDrifted, p.codeDrifted, p.attachSpecPending
}

// computeCanonicalHeadUpdate loads (or mints) c.FeatureKey's Canonical Head
// and, when neither spec- nor code-drifted, computes the value an in-contract
// turn would fold into it — WITHOUT writing anything. isNew=true means no
// Head existed yet (a fresh mint, not an update to fold). Pure computation,
// shared by updateCanonicalHead (immediate write — Normal chat / root) and
// stagePendingCanonicalHead (CP-55 P-5 — a Flow coder child stages this same
// value instead of writing it, so it is only ever durably written at genuine
// Flow terminal acceptance).
func computeCanonicalHeadUpdate(cwd string, c changecontract.Contract, hasOutOfContractChange bool) (updated changecontract.CanonicalHead, specDrifted, codeDrifted, attachSpecPending, isNew bool, err error) {
	dotFP := filepath.Join(cwd, ".flowpilot")
	catalog, _ := featurecatalog.LoadCatalog(dotFP)

	head, found, err := changecontract.LoadHead(cwd, c.FeatureKey)
	if err != nil {
		return changecontract.CanonicalHead{}, false, false, false, false, err
	}
	if !found {
		ledger, _ := changeledger.New(dotFP)
		head = changecontract.BuildHead(cwd, c.FeatureKey, ledger, catalog, &c)
		return head, false, false, false, true, nil // a just-minted Head cannot itself be drifted
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

	if specDrifted || codeDrifted || attachSpecPending {
		return changecontract.CanonicalHead{}, specDrifted, codeDrifted, attachSpecPending, false, nil
	}

	updated = changecontract.UpdateHead(head, c)
	if decisions, ok := foldCanonicalHeadDecisions(dotFP, c.FeatureKey); ok {
		updated.Decisions = decisions
		updated.IntentSignature = changecontract.ComputeSignature(updated)
	}
	return updated, false, false, false, false, nil
}

// updateCanonicalHead is computeCanonicalHeadUpdate plus the immediate write
// — unchanged behavior/return semantics from before CP-55 P-5's refactor,
// used by root/Normal-chat gate passes and the legacy captureChangeContract
// path, none of which stage a pending update.
// BUG-288 R18-6: returns err on I/O failure so commitChangeContract can fail-closed.
func updateCanonicalHead(cwd string, c changecontract.Contract, hasOutOfContractChange bool) (specDrifted, codeDrifted, attachSpecPending bool, err error) {
	updated, specDrifted, codeDrifted, attachSpecPending, isNew, err := computeCanonicalHeadUpdate(cwd, c, hasOutOfContractChange)
	if err != nil {
		log.Printf("[changecontract] head load failed for %q: %v", c.FeatureKey, err)
		return false, false, false, err
	}
	if isNew {
		if err := changecontract.SaveHead(cwd, updated); err != nil {
			log.Printf("[changecontract] head save (birth/backfill) failed for %q: %v", c.FeatureKey, err)
			return false, false, false, err
		}
		return false, false, false, nil
	}
	if specDrifted || codeDrifted || attachSpecPending {
		return specDrifted, codeDrifted, attachSpecPending, nil
	}
	if err := changecontract.SaveHead(cwd, updated); err != nil {
		log.Printf("[changecontract] head save (update) failed for %q: %v", c.FeatureKey, err)
		return specDrifted, codeDrifted, attachSpecPending, err
	}
	return specDrifted, codeDrifted, attachSpecPending, nil
}

// stagePendingCanonicalHead is computeCanonicalHeadUpdate plus a stage
// (CP-55 P-5): a Flow coder child's gate pass computes the same would-be
// Canonical Head update as the immediate-write path, but persists it to
// PendingCanonicalStore instead of .flowpilot/canonical/<feature_key>.json —
// the real file is only ever touched by finalizePendingCanonicalHeadsForRun,
// at genuine Flow terminal acceptance. A drifted/attach-pending turn stages
// nothing (mirrors updateCanonicalHead: surfaced via TurnResult for a human,
// never silently written).
func stagePendingCanonicalHead(cwd, runID, coderStepID string, c changecontract.Contract, hasOutOfContractChange bool, secret []byte) error {
	if c.FeatureKey == "" {
		return nil
	}
	updated, specDrifted, codeDrifted, attachSpecPending, _, err := computeCanonicalHeadUpdate(cwd, c, hasOutOfContractChange)
	if err != nil {
		log.Printf("[changecontract] pending head compute failed for %q: %v", c.FeatureKey, err)
		return err
	}
	if specDrifted || codeDrifted || attachSpecPending {
		return nil
	}
	store, err := changecontract.NewPendingCanonicalStoreWithSecret(cwd, secret)
	if err != nil {
		return err
	}
	return store.Stage(changecontract.PendingCanonicalRecord{
		RunID:       runID,
		FeatureKey:  c.FeatureKey,
		CoderStepID: coderStepID,
		Head:        updated,
	})
}

// finalizePendingCanonicalHeadsForRun durably writes every still-pending
// Canonical Head update staged for parentRunID (across every feature key a
// coder child staged one for during this Flow) and marks each finalized.
// Called only at genuine Flow terminal acceptance (applyFlowControl's "done"
// case), BEFORE the loop status flips to done — per CP-55 P-5's own
// requirement, a failure here must abort the "done" transition entirely, not
// silently continue with an un-finalized Canonical Head. Idempotent: a
// record already marked finalized is skipped (ListPendingForRunFresh only
// returns still-active entries), so retrying after a crash mid-finalize is
// safe and does not re-write/re-mark anything already done.
//
// Two-phase across the whole set (CP-55 P-5 review finding C-2): every
// feature's Head is staged to a tmp file FIRST; only once every stage in the
// batch has succeeded do any of them get committed (renamed into place) and
// marked finalized. Without this, a multi-feature Flow whose Nth feature
// failed to write would already have permanently mutated the real Head files
// of features 1..N-1 even though the "done" transition as a whole — and
// therefore this Flow's terminal acceptance of ALL of them — was refused.
func finalizePendingCanonicalHeadsForRun(cwd, parentRunID string, secret []byte) error {
	store, err := changecontract.NewPendingCanonicalStoreWithSecret(cwd, secret)
	if err != nil {
		return fmt.Errorf("open pending canonical store for run %q: %w", parentRunID, err)
	}
	// CP-55 P-5 review finding I-1: reload fresh under one lock hold
	// immediately before listing, so a Stage from a different in-process
	// PendingCanonicalStore instance (e.g. a coder gate pass whose gate
	// epoch was still valid the instant this Flow reached "done") that
	// landed after some earlier NewPendingCanonicalStore snapshot is not
	// silently invisible to this decision.
	pending, err := store.ListPendingForRunFresh(parentRunID)
	if err != nil {
		return fmt.Errorf("list pending canonical records for run %q: %w", parentRunID, err)
	}
	if len(pending) == 0 {
		return nil
	}

	type stagedWrite struct {
		rec                changecontract.PendingCanonicalRecord
		tmpPath, finalPath string
	}
	staged := make([]stagedWrite, 0, len(pending))
	for _, rec := range pending {
		tmp, final, err := changecontract.StageHeadWrite(cwd, rec.Head)
		if err != nil {
			for _, sw := range staged {
				changecontract.DiscardHeadWrite(sw.tmpPath)
			}
			return fmt.Errorf("stage canonical head for feature %q (run %q): %w", rec.FeatureKey, parentRunID, err)
		}
		staged = append(staged, stagedWrite{rec: rec, tmpPath: tmp, finalPath: final})
	}

	for _, sw := range staged {
		if err := changecontract.CommitHeadWrite(sw.tmpPath, sw.finalPath); err != nil {
			return fmt.Errorf("commit canonical head for feature %q (run %q): %w", sw.rec.FeatureKey, parentRunID, err)
		}
		if err := store.AppendStatus(sw.rec.RunID, sw.rec.FeatureKey, changecontract.PendingCanonicalStatusFinalized, "flow terminal acceptance", time.Now().UTC()); err != nil {
			return fmt.Errorf("mark canonical head finalized for feature %q (run %q): %w", sw.rec.FeatureKey, parentRunID, err)
		}
	}
	return nil
}

// abandonPendingCanonicalHeadsForRun marks every still-pending Canonical Head
// update staged for parentRunID as abandoned — the real Head file is never
// touched. Called on any non-"done" terminal Flow outcome (Stop/Cancel,
// failure) so a run that never reached genuine acceptance leaves no trace on
// the real Canonical Head. Best-effort: logs and continues past a store-open
// failure rather than blocking the (already-terminal) transition that calls
// it — there is no "block done" symmetry to preserve on the abandon path,
// unlike finalize.
func abandonPendingCanonicalHeadsForRun(cwd, parentRunID, reason string, secret []byte) {
	if strings.TrimSpace(cwd) == "" || strings.TrimSpace(parentRunID) == "" {
		return
	}
	store, err := changecontract.NewPendingCanonicalStoreWithSecret(cwd, secret)
	if err != nil {
		log.Printf("[changecontract] abandon pending canonical heads: open store failed for run %q: %v", parentRunID, err)
		return
	}
	// I-1 applies symmetrically here: a coder's Stage racing this abandon
	// must not be left permanently "pending" forever with no run left alive
	// to ever finalize or abandon it.
	pending, err := store.ListPendingForRunFresh(parentRunID)
	if err != nil {
		log.Printf("[changecontract] abandon pending canonical heads: list failed for run %q: %v", parentRunID, err)
		return
	}
	for _, rec := range pending {
		if err := store.AppendStatus(rec.RunID, rec.FeatureKey, changecontract.PendingCanonicalStatusAbandoned, reason, time.Now().UTC()); err != nil {
			log.Printf("[changecontract] abandon pending canonical head: mark abandoned failed for run %q feature %q: %v", parentRunID, rec.FeatureKey, err)
		}
	}
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
// (change-audit/FEATURE-KEYS.md) â€” a signal the feature was renamed/merged/
// removed there but its Head has not yet been migrated (Task-187 r-retire).
// This is a deliberately conservative nudge, not auto-retire: it only flags
// spec_backed, not-yet-retired Heads (a born-spec_less Head may carry an
// inferred key that was never registered, so exempting them avoids false
// positives). The actual retire is always an explicit human action via
// handleRetireCanonicalHead (BR-2 â€” never automatic).
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
	cmd := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// baselineSF coordinates per-workspace baseline init (BUG-288 #3).
var (
	baselineMu       sync.Mutex
	baselineInflight = map[string]chan struct{}{}
)

// ensureBaseline captures or refreshes the test baseline for cwd asynchronously
// with per-cwd singleflight (Task-156 / BUG-288 #3). BUG-288 R13-14: when the
// turn already has a cancellable ctx, prefer ensureBaselineReadyContext so the
// warm-up is cut short on Stop instead of always using Background.
func (s *InteractiveService) ensureBaseline(cwd string) {
	if cwd == "" {
		return
	}
	go s.ensureBaselineReady(cwd)
}

// ensureBaselineWithContext is ensureBaseline but threads a run-scoped ctx
// into the warm-up capture (BUG-288 R13-14).
func (s *InteractiveService) ensureBaselineWithContext(ctx context.Context, cwd string) {
	if cwd == "" {
		return
	}
	if ctx == nil {
		go s.ensureBaselineReady(cwd)
		return
	}
	go s.ensureBaselineReadyContext(ctx, cwd)
}

// ensureBaselineReady blocks until a RefreshBaselineIfStale attempt finishes
// for cwd, using context.Background() (no cancellation).
func (s *InteractiveService) ensureBaselineReady(cwd string) {
	s.ensureBaselineReadyContext(context.Background(), cwd)
}

// ensureBaselineReadyContext is ensureBaselineReady with an explicit context
// (BUG-288 P2-04, VÃ²ng 12): the first baseline capture runs the project's
// full test suite (can take up to ~5 minutes); previously this always used
// context.Background() regardless of caller, so Stop could not cut it short
// the way Stop already cancels other in-flight suite runs (flowInlineContext,
// postTurnGateCancel). Callers that already hold a run/loop-scoped cancellable
// ctx should call this instead of the context.Background() wrapper above.
func (s *InteractiveService) ensureBaselineReadyContext(ctx context.Context, cwd string) {
	if cwd == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	baselineMu.Lock()
	if ch, ok := baselineInflight[cwd]; ok {
		baselineMu.Unlock()
		// BUG-288 R13-14: waiters must honor ctx so Stop does not block ~5m.
		select {
		case <-ch:
		case <-ctx.Done():
		}
		return
	}
	ch := make(chan struct{})
	baselineInflight[cwd] = ch
	baselineMu.Unlock()

	dotFP := filepath.Join(cwd, ".flowpilot")
	if _, err := flowgate.RefreshBaselineIfStaleContext(ctx, cwd, dotFP); err != nil {
		log.Printf("[gate] ensureBaseline refresh error: %v", err)
	}

	baselineMu.Lock()
	delete(baselineInflight, cwd)
	close(ch)
	baselineMu.Unlock()
}
