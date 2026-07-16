package runner

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/flowgate"
)

// BUG-243 F-0/F-1/F-2: gives the flow executor a path to actually run
// rag-harness's `validate` (command.validate) and `audit` (artifact.audit_draft)
// nodes, and wires their behavior handlers to the real, already-tested
// Task-170/171 implementations (RunValidationCommand/AdvanceRetryState/
// ComposeRetryPrompt, BuildAuditDraft/PersistAuditDraft) instead of the stub
// handlers that only inspected RawArgs. Reuse, not rewrite (K-2): every
// function called from here already exists and is unit-tested; this file is
// the live dispatch path that was missing.

// edgeTargetFrom finds the single edge matching (from, when, kind) and
// returns its target, mirroring resolveContinueBackEdgeTarget but scoped to
// one specific source node â€” needed here because validate/audit each have
// exactly one edge per (when, kind) triple, unlike the flow-wide unscoped
// lookup that function does for review-loop's single back-edge.
func edgeTargetFrom(edges []agentpack.FlowEdge, from, when, kind string) (string, bool) {
	for _, e := range edges {
		if strings.EqualFold(strings.TrimSpace(e.From), from) &&
			strings.EqualFold(strings.TrimSpace(e.When), when) &&
			strings.EqualFold(strings.TrimSpace(e.Kind), kind) &&
			e.To != "" {
			return e.To, true
		}
	}
	return "", false
}

// tryAdvanceFlowThroughInline dispatches a single inline-scope target node
// (validate/audit) in-process via the behavior registry, mirroring
// startInlineEntryChain's shape but for a node reached mid-flow instead of
// at flow entry. Returns false for any inline behavior other than
// command.validate/artifact.audit_draft (context.produce/context.render
// have their own dedicated dispatch paths and are never reached this way in
// either built-in flow) so an unrecognized shape falls back to the
// pre-existing note+reinvoke-hub behavior unchanged.
func (s *InteractiveService) tryAdvanceFlowThroughInline(parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, node agentpack.FlowNode, resultMessage string) bool {
	canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior)
	if !ok {
		return false
	}
	// BUG-288 P1-12: recheck the run's terminal/stop status right here, before
	// any dispatch work, not only when acquiring the context below. A
	// late/queued inline dispatch callback (validate/audit/telegram/delegate)
	// can reach this function after Stop already cancelled/cleared
	// flowInlineCtx; without this guard it would still call
	// runValidateNode/runAuditNode/runTelegramNotifyNode and advance flow
	// state past Stop. Claim the callback as handled (return true) instead of
	// falling back to the note+reinvoke-hub path, which would also advance.
	if s.flowRunTerminalLocked(parentRunID) {
		s.flowDiagLog(parentRunID, "flow_inline_dispatch_skipped_terminal",
			"run terminal/stopped; skipping inline dispatch",
			"node_id", node.ID,
			"behavior", canonical,
		)
		return true
	}
	// Live context so Stop cancels in-process validate suites (not Background).
	ctx := s.flowInlineContext(parentRunID)
	switch canonical {
	case "command.validate":
		return s.runValidateNode(ctx, parentRunID, edges, nodes, node, resultMessage)
	case "artifact.audit_draft":
		return s.runAuditNode(ctx, parentRunID, edges, nodes, node, resultMessage)
	case "telegram.notify":
		return s.runTelegramNotifyNode(ctx, parentRunID, edges, nodes, node, resultMessage)
	case "hub.notify":
		s.dispatchHubNotifyNode(parentRunID, node)
		return true
	default:
		return false
	}
}

// flowRunTerminalLocked reports whether parentRunID's run is missing or has
// reached a terminal status (Completed/Failed/Cancelled). stopAgentLoop
// unconditionally moves every non-terminal root to Cancelled before clearing
// flowInlineCtx/flowInlineCancel, so a terminal status here reliably means
// "Stop happened (or the flow already finished) â€” do not resurrect inline
// work", without needing a separate stop-generation tombstone field
// (BUG-288 P1-12).
func (s *InteractiveService) flowRunTerminalLocked(parentRunID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[parentRunID]
	if rs == nil {
		return true
	}
	switch rs.status {
	case RunStatusCompleted, RunStatusFailed, RunStatusCancelled:
		return true
	default:
		return false
	}
}

// alreadyCancelledContext returns a context.Context whose Done() is already
// closed and Err() is context.Canceled â€” used by flowInlineContext so a late
// inline-dispatch callback observes an unambiguously-cancelled context
// instead of a fresh, uncancelled context.Background() (BUG-288 P1-12).
func alreadyCancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// flowInlineContext returns a cancelable context for in-process inline nodes
// (validate/audit). stopAgentLoop cancels it so suites abort with the flow.
// BUG-288 P1-12: once the run is terminal (stopped/done), this never mints a
// fresh non-cancelled context â€” that was the bypass a late/queued inline
// callback could exploit to keep running validate/audit/telegram/delegate
// work after Stop cleared flowInlineCtx/flowInlineCancel.
func (s *InteractiveService) flowInlineContext(parentRunID string) context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[parentRunID]
	if rs == nil {
		return alreadyCancelledContext()
	}
	switch rs.status {
	case RunStatusCompleted, RunStatusFailed, RunStatusCancelled:
		return alreadyCancelledContext()
	}
	if rs.flowInlineCtx != nil {
		return rs.flowInlineCtx
	}
	ctx, cancel := context.WithCancel(context.Background())
	rs.flowInlineCtx = ctx
	rs.flowInlineCancel = cancel
	return ctx
}

// loadValidateCommand resolves the validate node's shell command from
// .flowpilot/guard/test_baseline.json's test_command field (BUG-243 Q-1's
// own suggested resolution â€” the CP-35 gate/regression-guard baseline
// already captures a working test command per-project; reusing it avoids
// inventing a second config surface). Returns "" when no baseline exists or
// it declares no command â€” NewFlowValidationRetryState already treats an
// empty command as "skipped_no_command", not a fabricated default (K-4).
// The returned cwd honors Baseline.TestDir for monorepos with a nested
// go.mod, same as the CP-35 gate itself.
func loadValidateCommand(workspaceCwd string) (command, cwd string) {
	if workspaceCwd == "" {
		return "", ""
	}
	baseline, err := flowgate.LoadBaseline(filepath.Join(workspaceCwd, ".flowpilot"))
	if err != nil || baseline == nil {
		return "", workspaceCwd
	}
	cwd = workspaceCwd
	if baseline.TestDir != "" {
		cwd = filepath.Join(workspaceCwd, baseline.TestDir)
	}
	return strings.TrimSpace(baseline.TestCmd), cwd
}

// loadPlanContextPackage retrieves the flow's context.produce output for
// audit/retry-prompt assembly. startInlineEntryChain stashes it on the
// parent run's planContextPackage field when the context node completes
// (BUG-243 F-0 addition â€” it previously only rendered the package into the
// one prompt handoff and discarded it, leaving nothing for validate/audit to
// read later in the same run). Falls back to scanning rs.events directly
// (in-memory, no store round-trip â€” mirrors interactive_resume.go's own
// restore-on-reconnect scan) for a run resumed after this field was reset.
func (s *InteractiveService) loadPlanContextPackage(_ context.Context, parentRunID string) (FlowContextPackage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[parentRunID]
	if rs == nil {
		return FlowContextPackage{}, false
	}
	if rs.planContextPackage != nil {
		return *rs.planContextPackage, true
	}
	for i := len(rs.events) - 1; i >= 0; i-- {
		if rs.events[i].Type == EventFlowContextPackage && rs.events[i].FlowContextPackage != nil {
			return *rs.events[i].FlowContextPackage, true
		}
	}
	return FlowContextPackage{}, false
}

// changedFilesSince observes changed paths in cwd since baseSHA, reusing
// the exact same CP-35 gate observation (flowgate.ObserveGitDiffSince).
// Returns an error when observation fails so callers (especially tier-3 audit)
// can block/escalate instead of treating failure as a clean empty diff (V10R4).
func changedFilesSince(cwd, baseSHA string) ([]string, error) {
	if cwd == "" {
		return nil, nil
	}
	diff, err := flowgate.ObserveGitDiffSince(cwd, baseSHA)
	if err != nil {
		return nil, err
	}
	return changedPathsFromDiff(diff), nil
}

// runValidateWithOracleIfPossible runs the validation suite via flowgate.RunOracle
// when a baseline exists (Task-242 D-4), mapping regression vs new-fail into
// ValidationResult for AdvanceRetryState. Falls back to RunValidationCommand
// when there is no baseline or the oracle is disabled.
//
// workspaceRoot is the flow workspace (where .flowpilot/ lives). workDir may be
// a nested TestDir monorepo package; baseline/overrides must always load from
// workspaceRoot, and RunOracle takes the workspace root so it can apply TestDir
// itself (same as the CP-35 gate).
//
// Mapping:
//   - oracle.SuitePassed â†’ ExitCode 0 (passed)
//   - oracle.HasRegression â†’ ExitCode 1 + REGRESSION-labeled stderr (retry path)
//   - suite fail without regression â†’ ExitCode 1 + new-failure label
func runValidateWithOracleIfPossible(ctx context.Context, command, workspaceRoot, workDir, baseSHA string, changedFiles []string) ValidationResult {
	if workspaceRoot == "" {
		workspaceRoot = workDir
	}
	dotFP := filepath.Join(workspaceRoot, ".flowpilot")
	baseline, blErr := flowgate.LoadBaseline(dotFP)
	if blErr != nil {
		// BUG-288 R13-13: corrupt baseline must not soft-skip oracle into a bare
		// command path that can look "green" without regression context.
		log.Printf("[flow-executor] LoadBaseline failed: %v (fail-closed env error)", blErr)
		now := time.Now().UTC().Format(time.RFC3339)
		return ValidationResult{
			EnvError:   "baseline unreadable: " + blErr.Error(),
			StartedAt:  now,
			FinishedAt: now,
		}
	}
	if baseline == nil || baseline.TestCmd == "" {
		return RunValidationCommand(ctx, command, workDir)
	}
	overrides, _ := flowgate.LoadOverrides(dotFP)
	// Observe from workspace root (git repo + baseline paths), not the nested TestDir.
	diff, _ := flowgate.ObserveGitDiffSince(workspaceRoot, baseSHA)
	// Prefer caller-provided changed path list when Observe fails empty but we know paths.
	if len(diff) == 0 && len(changedFiles) > 0 {
		for _, p := range changedFiles {
			diff = append(diff, flowgate.ChangedFile{Path: p, Status: "M"})
		}
	}
	// RunOracleContext propagates turn ctx; applies baseline.TestDir internally.
	oracle := flowgate.RunOracleContext(ctx, workspaceRoot, baseline, diff, overrides)
	if oracle.Disabled {
		return RunValidationCommand(ctx, command, workDir)
	}
	cmd := command
	if strings.TrimSpace(cmd) == "" {
		cmd = baseline.TestCmd
	}
	// Command-start failures (missing binary, permission) map to EnvError so
	// AdvanceRetryState takes skipped_env_error â€” never regression retry.
	if oracle.EnvError != "" {
		return ValidationResult{
			Command:  cmd,
			WorkDir:  workDir,
			ExitCode: 0,
			EnvError: oracle.EnvError,
			// Prefer real suite/pipe output when present; fall back to message.
			Stderr: firstNonEmpty(oracle.Output, oracle.Message),
		}
	}
	exit := 0
	if !oracle.SuitePassed {
		exit = 1
	}
	// Human-readable label for gate/log; raw suite log goes to Stderr so
	// SummarizeValidationFailure can extract compile/test failure lines.
	summary := oracle.Message
	if oracle.HasRegression {
		if summary == "" {
			summary = "Regression: previously-passing tests now fail: " + strings.Join(oracle.Regressed, ", ")
		} else if !strings.Contains(strings.ToLower(summary), "regress") {
			summary = "REGRESSION: " + summary
		}
	} else if exit != 0 && len(oracle.Failed) > 0 && summary == "" {
		summary = "Validation failed (new or non-baseline failures): " + strings.Join(oracle.Failed, ", ")
	}
	// Stderr preferred by SummarizeValidationFailure: put suite output there.
	// Prefix with summary when both exist so unstructured logs still carry context.
	stderr := oracle.Output
	if strings.TrimSpace(stderr) == "" {
		stderr = summary
	} else if summary != "" && !strings.Contains(stderr, summary) {
		stderr = summary + "\n" + stderr
	}
	return ValidationResult{
		Command:  cmd,
		WorkDir:  workDir,
		ExitCode: exit,
		Stdout:   "", // combined suite log lives in Stderr (matches CombinedOutput)
		Stderr:   stderr,
	}
}

// codingChildTurnMetaLocked returns turnStartGitHead and lastTurnID from the
// most recently active coding child. V9-20: any non-reviewer delegate with a
// captured turn head (custom labels), not only coder/implement names.
// Caller holds s.mu.
func codingChildTurnMetaLocked(s *InteractiveService, parentRunID string) (baseSHA, turnID string) {
	var best *interactiveRun
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		c := s.runs[childID]
		if c == nil {
			continue
		}
		// Prefer explicit coder/implement, but also any non-reviewer with a turn head.
		label := strings.ToLower(c.label)
		role := strings.ToLower(c.role)
		name := strings.ToLower(c.agentName)
		isReviewer := role == "reviewer" || role == "review" || name == "reviewer"
		isCoderNamed := role == "coder" || name == "coder" || label == "coder" || label == "implement" ||
			strings.HasPrefix(label, "implement")
		if isReviewer {
			continue
		}
		if !isCoderNamed && strings.TrimSpace(c.turnStartGitHead) == "" {
			continue
		}
		if best == nil || c.updatedAt > best.updatedAt {
			best = c
		}
	}
	if best == nil {
		return "", ""
	}
	return best.turnStartGitHead, best.lastTurnID
}

// validateResultIsCancelled reports user/flow Stop (parent context canceled),
// not oracle-owned deadline (BUG-288 #11) or missing-binary EnvError.
func validateResultIsCancelled(ctx context.Context, result ValidationResult) bool {
	// BUG-288 R11 #1: check the caller's ctx directly and unconditionally first â€”
	// Stop can land between the suite finishing (a clean, non-EnvError result)
	// and this call, which the EnvError-only heuristic below would miss and let
	// a "passed" result advance validate â†’ audit/done after Stop.
	if ctx != nil && ctx.Err() == context.Canceled {
		return true
	}
	// Parent/flow ctx canceled by Stop â€” but only when the EnvError also looks
	// like cancel (oracle may finish with a real result while parent later cancels).
	err := strings.ToLower(strings.TrimSpace(result.EnvError))
	if err == "" {
		return false
	}
	// Internal oracle timeout surfaces as "context deadline exceeded" on the
	// *suite* env error while the caller's flow ctx may still be open â€” treat
	// as validation failure (caller maps EnvErrorâ‰ cancel), not flow cancel.
	if strings.Contains(err, "context deadline exceeded") {
		// Only treat as cancel if the *caller* ctx is also canceled with cancel (not deadline).
		if ctx != nil && ctx.Err() == context.Canceled {
			return true
		}
		return false
	}
	if strings.Contains(err, "context canceled") {
		return true
	}
	return ctx != nil && ctx.Err() == context.Canceled
}

// runValidateNode implements F-1: wires command.validate to a real command
// execution + the bounded (max 3) retry loop, replacing the stub that only
// read a never-populated RawArgs["exitCode"].
func (s *InteractiveService) runValidateNode(ctx context.Context, parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, node agentpack.FlowNode, resultMessage string) bool {
	cwd := s.workspaceCwdFor(parentRunID)
	pkg, hasPkg := s.loadPlanContextPackage(ctx, parentRunID)

	s.mu.Lock()
	rs := s.runs[parentRunID]
	var state FlowValidationRetryState
	var baseSHA, prevTurnID string
	if rs != nil {
		// BUG-288 #17: prefer coding child's turn head / last turn id when present
		// (multi-round / custom flows), not only hub hub's last turn.
		baseSHA, prevTurnID = codingChildTurnMetaLocked(s, parentRunID)
		if baseSHA == "" {
			baseSHA = rs.turnStartGitHead
		}
		if prevTurnID == "" {
			prevTurnID = rs.lastTurnID
		}
		if rs.flowValidationRetryState != nil {
			state = *rs.flowValidationRetryState
		}
	}
	s.mu.Unlock()

	command, workDir := loadValidateCommand(cwd)
	if state.ValidationCommand == "" && state.Status == "" {
		planPackageID := ""
		if hasPkg {
			planPackageID = pkg.PackageID
		}
		state = NewFlowValidationRetryState(planPackageID, command)
	}

	if state.Status == "skipped_no_command" {
		s.mu.Lock()
		if rs := s.runs[parentRunID]; rs != nil {
			rs.flowValidationRetryState = &state
		}
		s.mu.Unlock()
		if store := s.persistenceStore(); store != nil {
			if err := PersistRetryState(ctx, store, parentRunID, node.ID, state); err != nil {
				log.Printf("[flow-executor] validate: persist skipped_no_command state failed: %v", err)
			}
		}
		// V9-01: no verified suite â†’ do not advance auditâ†’done. Escalate so the
		// human can configure a command or continue explicitly.
		s.flowDiagLog(parentRunID, "flow_validate_skipped_no_command", "no validate command; blocking audit/done", "node_id", node.ID)
		if _, err := s.applyFlowControl(parentRunID, FlowControlInput{
			Status:  "escalate",
			Summary: "Validation skipped: no test command configured in baseline. Configure .flowpilot/guard/test_baseline.json or Continue after manual verification.",
		}); err != nil {
			log.Printf("[flow-executor] validate: escalate after skipped_no_command failed: %v", err)
			return false
		}
		return true
	}

	// BUG-288 #3: wait for baseline singleflight so first validate is not skipped.
	// BUG-288 P2-04: thread runValidateNode's own ctx (already cancelled by Stop
	// via flowInlineContext) so a first-time baseline capture is cancellable too.
	s.ensureBaselineReadyContext(ctx, cwd)

	// Task-242 D-4 / T-2: when a flowgate baseline exists, use RunOracle so we
	// distinguish regression (baseline green â†’ red) from ordinary new failures.
	// No baseline â†’ keep RunValidationCommand. Never double-run the suite.
	// Observe + baseline load from workspace root (cwd), not nested TestDir workDir.
	// Validate tolerates observation failure for retry-state paths; audit fails closed.
	changedFiles, _ := changedFilesSince(cwd, baseSHA)
	result := runValidateWithOracleIfPossible(ctx, command, cwd, workDir, baseSHA, changedFiles)

	// Stop/cancel must not advance validate â†’ audit/done as skipped_env_error.
	if validateResultIsCancelled(ctx, result) {
		s.flowDiagLog(parentRunID, "flow_validate_cancelled", "validate suite cancelled; not advancing",
			"node_id", node.ID, "env_error", result.EnvError,
		)
		return false
	}

	// BUG-288 R13-17 / R13-20: AdvanceRetryState mutates attempt counters â€”
	// only commit to in-memory rs after durable persist succeeds so a flake
	// cannot mark "passed" (audit fail-open) or burn two budget slots on re-enter.
	AdvanceRetryState(&state, result, changedFiles, prevTurnID)

	// BUG-288 P1-13: PersistValidationResult/PersistRetryState are the durable
	// audit trail for this validate attempt. Advancing (retry/spawn/done) on a
	// persist failure would let the flow move past this attempt with no
	// authoritative record â€” a crash right after would then either replay the
	// suite (burning retry budget on a passed attempt) or lose the failure
	// detail entirely. Fail-closed: escalate and stop instead of falling
	// through to the status switch below.
	var persistErr error
	if store := s.persistenceStore(); store != nil {
		if err := PersistValidationResult(ctx, store, parentRunID, node.ID, result); err != nil {
			log.Printf("[flow-executor] validate: persist result failed: %v", err)
			persistErr = err
		}
		if err := PersistRetryState(ctx, store, parentRunID, node.ID, state); err != nil {
			log.Printf("[flow-executor] validate: persist retry state failed: %v", err)
			if persistErr == nil {
				persistErr = err
			}
		}
	}
	if persistErr != nil {
		s.flowDiagLog(parentRunID, "flow_validate_persist_failed", "validation result/retry state did not persist; blocking advance",
			"node_id", node.ID, "status", state.Status, "error", persistErr.Error(),
		)
		// Do NOT assign rs.flowValidationRetryState (R13-17). Surface awaiting-user.
		s.setFlowStepAwaitingUser(ctx, parentRunID)
		if _, err := s.applyFlowControl(parentRunID, FlowControlInput{
			Status:  "escalate",
			Summary: "Validation ran but its result could not be durably recorded (" + persistErr.Error() + "). Not advancing until this is resolved â€” retry Continue once storage is available.",
		}); err != nil {
			// BUG-288 R13-18: escalate failure must still leave an actionable surface.
			log.Printf("[flow-executor] validate: escalate after persist failure failed: %v", err)
			s.flowDiagLog(parentRunID, "flow_validate_persist_escalate_failed",
				"persist failed and escalate also failed; flow left awaiting user",
				"node_id", node.ID, "error", err.Error())
		}
		return true
	}
	s.mu.Lock()
	if rs := s.runs[parentRunID]; rs != nil {
		rs.flowValidationRetryState = &state
	}
	s.mu.Unlock()
	s.flowDiagLog(parentRunID, "flow_validate_ran", "ran validate command",
		"node_id", node.ID, "status", state.Status, "attempt", state.RetryAttempt, "exit_code", result.ExitCode,
	)

	switch state.Status {
	case "passed":
		return s.advanceToNextInlineOrDelegate(ctx, parentRunID, edges, nodes, node.ID, "done", resultMessage)

	case "skipped_env_error":
		// V9-01: env/timeout/oracle setup failure is not a green suite â€” escalate
		// instead of advancing auditâ†’done (BuildAuditDraft would also block).
		// BUG-288 R13-18: unify with persist-fail path â€” always return true so the
		// caller does not fall into note+reinvoke-hub fallback (which can advance).
		summary := "Validation suite could not run (environment/timeout)."
		if strings.TrimSpace(result.EnvError) != "" {
			summary = "Validation suite could not run: " + result.EnvError
		}
		s.setFlowStepAwaitingUser(ctx, parentRunID)
		if _, err := s.applyFlowControl(parentRunID, FlowControlInput{Status: "escalate", Summary: summary}); err != nil {
			log.Printf("[flow-executor] validate: escalate after skipped_env_error failed: %v", err)
			s.flowDiagLog(parentRunID, "flow_validate_escalate_failed", "escalate failed after skipped_env_error",
				"node_id", node.ID, "error", err.Error())
		}
		return true

	case "retrying":
		targetID, ok := edgeTargetFrom(edges, node.ID, "continue", "back")
		if !ok {
			return false
		}
		targetNode, ok := findFlowNode(nodes, targetID)
		if !ok {
			return false
		}
		prompt := resultMessage
		if hasPkg {
			prompt = ComposeRetryPrompt(pkg, state)
		}
		// Task-247 / CP-50 P-4: re-entry after validate must still see declared scope.
		prompt = composeFlowNodeAgentPrompt(cwd, prompt, targetNode)
		prompt = appendChangeContractIfAnyWithSecret(cwd, parentRunID, prompt, s.markerSecret)
		agentName := flowNodeAgentName(targetNode)
		if agentName == "" {
			return false
		}
		// V10 / V9-11: mark validate DONE only after reinvoke/spawn of the retry
		// target succeeds â€” not before (spawn fail left DONE with no successor).
		// BUG-279: a target node declared lifecycle: reinvoke must reuse its
		// existing child run/provider session for the retry turn, same as the
		// forward-edge auto-advance path (flowNodeReusesChild check before
		// reinvokeExistingFlowChild) â€” spawning a brand new child here silently
		// dropped the coder's own working memory of the prior attempt every retry.
		if flowNodeReusesChild(targetNode) && s.reinvokeExistingFlowChild(parentRunID, targetNode.ID, prompt) {
			if s.isFlowEngineDriven(parentRunID) {
				s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusDone)
				s.setFlowStepStatus(ctx, parentRunID, targetNode.ID, StepStatusRunning)
				s.stampFlowNodePosture(ctx, parentRunID, targetNode)
			}
			return true
		}
		agentDef, _ := resolvePackAgentDefinition(agentName)
		if _, err := s.spawnChildRun(ctx, parentRunID, SpawnAgentInput{
			Agent:            agentName,
			Prompt:           prompt,
			Wait:             false,
			Label:            targetNode.ID,
			AutoOrchestrate:  true,
			AgentDefOverride: agentDef,
			Model:            s.resolveFlowNodeModel(ctx, targetNode),
		}); err != nil {
			log.Printf("[flow-executor] validate: retry spawn of %q failed: %v", targetNode.ID, err)
			return false
		}
		if s.isFlowEngineDriven(parentRunID) {
			s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusDone)
			s.setFlowStepStatus(ctx, parentRunID, targetNode.ID, StepStatusRunning)
			s.stampFlowNodePosture(ctx, parentRunID, targetNode)
		}
		return true

	case "failed_validation_max_retries":
		summary := "Validation failed after the maximum number of retries."
		if state.FailureSummary != nil && len(state.FailureSummary.FailureLines) > 0 {
			summary += " Last failure:\n" + strings.Join(state.FailureSummary.FailureLines, "\n")
		}
		// BUG-288 R13-18: always return true (handled) even if escalate fails.
		s.setFlowStepAwaitingUser(ctx, parentRunID)
		if _, err := s.applyFlowControl(parentRunID, FlowControlInput{Status: "escalate", Summary: summary}); err != nil {
			log.Printf("[flow-executor] validate: escalate after max retries failed: %v", err)
			s.flowDiagLog(parentRunID, "flow_validate_escalate_failed", "escalate failed after max retries",
				"node_id", node.ID, "error", err.Error())
		}
		return true

	default:
		return false
	}
}

// advanceToNextInlineOrDelegate follows the (fromNodeID, when, "forward")
// edge and dispatches whatever it finds: chains into another inline node
// (validate -> audit, when validate passes immediately), spawns a delegate
// node, or settles the flow via applyFlowControl for a terminal ("done" /
// "ask_user") target â€” reusing the exact same finalization applyFlowControl
// already performs for review-loop's synthesis node, since "an inline node
// reached its done/escalate edge" has identical semantics regardless of
// which node emitted it.
func (s *InteractiveService) advanceToNextInlineOrDelegate(ctx context.Context, parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, fromNodeID, when, resultMessage string) bool {
	targetID, ok := edgeTargetFrom(edges, fromNodeID, when, "forward")
	if !ok {
		return false
	}
	// V9-11: stamp source DONE only after successful terminal action / spawn â€”
	// not before target validation (missing target used to leave DONE with no successor).
	markSourceDone := func() {
		if s.isFlowEngineDriven(parentRunID) {
			s.setFlowStepStatus(ctx, parentRunID, fromNodeID, StepStatusDone)
		}
	}
	switch targetID {
	case "done":
		_, err := s.applyFlowControl(parentRunID, FlowControlInput{Status: "done", Summary: resultMessage})
		if err == nil {
			markSourceDone()
		}
		return err == nil
	case "ask_user":
		_, err := s.applyFlowControl(parentRunID, FlowControlInput{Status: "escalate", Summary: resultMessage})
		if err == nil {
			markSourceDone()
		}
		return err == nil
	}
	nextNode, ok := findFlowNode(nodes, targetID)
	if !ok {
		return false
	}
	canonical, ok := agentpack.NormalizeBehaviorID(nextNode.Behavior)
	if !ok {
		return false
	}
	switch canonical {
	case "artifact.audit_draft":
		ok := s.runAuditNode(ctx, parentRunID, edges, nodes, nextNode, resultMessage)
		if ok {
			markSourceDone()
		}
		return ok
	case "command.validate":
		ok := s.runValidateNode(ctx, parentRunID, edges, nodes, nextNode, resultMessage)
		if ok {
			markSourceDone()
		}
		return ok
	case "telegram.notify":
		ok := s.runTelegramNotifyNode(ctx, parentRunID, edges, nodes, nextNode, resultMessage)
		if ok {
			markSourceDone()
		}
		return ok
	case "hub.notify":
		s.dispatchHubNotifyNode(parentRunID, nextNode)
		markSourceDone()
		return true
	case "agent.delegate":
		agentName := flowNodeAgentName(nextNode)
		if agentName == "" {
			return false
		}
		// Task-247 / CP-50 P-4: inline-dispatch delegate must carry change.contract.
		cwd := s.workspaceCwdFor(parentRunID)
		prompt := composeFlowNodeAgentPrompt(cwd, resultMessage, nextNode)
		prompt = appendChangeContractIfAnyWithSecret(cwd, parentRunID, prompt, s.markerSecret)
		agentDef, _ := resolvePackAgentDefinition(agentName)
		if _, err := s.spawnChildRun(ctx, parentRunID, SpawnAgentInput{
			Agent:            agentName,
			Prompt:           prompt,
			Wait:             false,
			Label:            nextNode.ID,
			AutoOrchestrate:  true,
			AgentDefOverride: agentDef,
			Model:            s.resolveFlowNodeModel(ctx, nextNode),
		}); err != nil {
			log.Printf("[flow-executor] advance: spawn %q failed: %v", nextNode.ID, err)
			return false
		}
		markSourceDone()
		if s.isFlowEngineDriven(parentRunID) {
			s.setFlowStepStatus(ctx, parentRunID, nextNode.ID, StepStatusRunning)
			s.stampFlowNodePosture(ctx, parentRunID, nextNode)
		}
		return true
	default:
		return false
	}
}

// runAuditNode implements F-2: wires artifact.audit_draft to the real
// BuildAuditDraft (change-ledger block + commit-message suggestion),
// replacing the stub that only echoed RawArgs["summary"]. Persists the
// result via PersistAuditDraft (EventFlowAuditDraft) so it is inspectable
// before any write/commit (Task-171's own non-write invariant â€” this
// function never writes a file or creates a commit), then settles the whole
// flow as done via applyFlowControl.
func (s *InteractiveService) runAuditNode(ctx context.Context, parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, node agentpack.FlowNode, resultMessage string) bool {
	if auditCtxCancelled(ctx, parentRunID, node.ID, "entry") {
		return false
	}
	pkg, _ := s.loadPlanContextPackage(ctx, parentRunID)

	s.mu.Lock()
	rs := s.runs[parentRunID]
	var state FlowValidationRetryState
	var workspace, changeType, baseSHA string
	if rs != nil {
		if rs.flowValidationRetryState != nil {
			state = *rs.flowValidationRetryState
		}
		workspace = rs.workspaceCwd
		changeType = rs.changeType
		// Task-242 tier-3: prefer flow-start HEAD so audit sees the whole-flow
		// aggregate diff, not only the hub's last turn.
		baseSHA = rs.flowStartGitHead
		if baseSHA == "" {
			baseSHA = rs.turnStartGitHead
		}
	}
	s.mu.Unlock()

	// BuildAuditDraft derives SourceDocID from pkg.SourceDocIDs[0] itself
	// (flow_audit_draft.go) â€” no separate source needed here.
	// V10R4 P1: observation failure must NOT look like a clean empty diff â€”
	// tier-3 audit is defense-in-depth and must block/escalate when unverifiable.
	changedFiles, observeErr := changedFilesSince(workspace, baseSHA)
	if observeErr != nil && workspace != "" {
		if auditCtxCancelled(ctx, parentRunID, node.ID, "observe_fail") {
			return false
		}
		msg := "Audit gate: cannot observe aggregate git diff: " + observeErr.Error()
		log.Printf("[flow-executor] audit tier-3 observation failed: %v", observeErr)
		s.flowDiagLog(parentRunID, "flow_audit_observe_fail", "audit aggregate git observation failed",
			"node_id", node.ID, "error", observeErr.Error(),
		)
		if s.isFlowEngineDriven(parentRunID) {
			s.setFlowStepAwaitingUser(ctx, parentRunID)
		}
		if auditCtxCancelled(ctx, parentRunID, node.ID, "observe_fail_before_escalate") {
			return false
		}
		if _, err := s.applyFlowControl(parentRunID, FlowControlInput{
			Status:  "escalate",
			Summary: msg,
		}); err != nil {
			log.Printf("[flow-executor] audit observe-fail escalate failed: %v", err)
		}
		return true
	}

	// Task-242 tier-3: re-evaluate doc-family rules on the aggregate flow diff
	// before settling done. Missing required docs â†’ escalate with remediation
	// instead of finalizing (defense-in-depth; tier-1 should have caught earlier).
	if workspace != "" && len(changedFiles) > 0 {
		diff, diffErr := flowgate.ObserveGitDiffSince(workspace, baseSHA)
		if diffErr != nil {
			if auditCtxCancelled(ctx, parentRunID, node.ID, "reobserve_fail") {
				return false
			}
			msg := "Audit gate: cannot re-observe aggregate git diff: " + diffErr.Error()
			log.Printf("[flow-executor] audit tier-3 re-observe failed: %v", diffErr)
			if s.isFlowEngineDriven(parentRunID) {
				s.setFlowStepAwaitingUser(ctx, parentRunID)
			}
			if auditCtxCancelled(ctx, parentRunID, node.ID, "reobserve_fail_before_escalate") {
				return false
			}
			if _, err := s.applyFlowControl(parentRunID, FlowControlInput{
				Status:  "escalate",
				Summary: msg,
			}); err != nil {
				log.Printf("[flow-executor] audit re-observe escalate failed: %v", err)
			}
			return true
		}
		if len(diff) > 0 {
			// BUG-288 #2: honor on-disk rules + ContractDeclared from parent-run store
			// (coder already declared; do not always fire r-contract).
			rules := flowgate.MergeDefaultRules(nil)
			if loaded, err := flowgate.LoadRules(filepath.Join(workspace, ".flowpilot", "settings")); err == nil {
				rules = flowgate.MergeDefaultRules(loaded)
			}
			var docRules []flowgate.Rule
			for _, r := range rules {
				if r.Enabled && flowgate.IsDocScopeRule(r.ID) {
					docRules = append(docRules, r)
				}
			}
			contractDeclared := false
			if store, err := changecontract.OpenStoreReadOnly(workspace); err == nil && store != nil {
				if c, ok := store.GetLatestForRun(parentRunID); ok {
					contractDeclared = c.Confidence == changecontract.ConfidenceDeclared
				}
			}
			tr := flowgate.TurnResult{
				FinalMessage: resultMessage,
				GitDiff:      diff,
				ChangedPaths: changedFiles,
				// Aggregate flow delta for r-ca/etc.; r-contract only fires when
				// WrittenPaths non-empty AND !ContractDeclared (BUG-288 #2).
				WrittenPaths:     changedFiles,
				ChangeType:       changeType,
				WorkspaceCwd:     workspace,
				ContractDeclared: contractDeclared,
			}
			if vios := flowgate.Evaluate(tr, docRules); len(vios) > 0 {
				result := flowgate.Enforce(vios, loadGateMode(filepath.Join(workspace, ".flowpilot")))
				if result.Action == "reprompt" || result.Action == "block" {
					if auditCtxCancelled(ctx, parentRunID, node.ID, "tier3_before_escalate") {
						return false
					}
					log.Printf("[flow-executor] audit tier-3 gate: %s (tier-1 should have caught earlier)", result.Message)
					s.flowDiagLog(parentRunID, "flow_audit_tier3_block", "audit aggregate gate blocked done",
						"node_id", node.ID, "message", result.Message,
					)
					if s.isFlowEngineDriven(parentRunID) {
						s.setFlowStepAwaitingUser(ctx, parentRunID)
					}
					if auditCtxCancelled(ctx, parentRunID, node.ID, "tier3_immediate_before_escalate") {
						return false
					}
					if _, err := s.applyFlowControl(parentRunID, FlowControlInput{
						Status:  "escalate",
						Summary: "Audit gate (aggregate): " + result.Message,
					}); err != nil {
						log.Printf("[flow-executor] audit tier-3 escalate failed: %v", err)
					}
					return true
				}
			}
		}
	}

	if auditCtxCancelled(ctx, parentRunID, node.ID, "before_draft") {
		return false
	}

	draft := BuildAuditDraft(AuditDraftInput{
		WorkflowRunID:   parentRunID,
		AuditStepID:     node.ID,
		ContextPackage:  pkg,
		ValidationState: state,
		ChangedFiles:    changedFiles,
		WhatChanged:     resultMessage,
		ChangeType:      changeType,
		Workspace:       workspace,
	})

	// BUG-288 P1-13: a ready audit draft is the durable audit-trail record that
	// the whole validate->audit chain exists to produce. Advancing to a
	// successor node or settling the flow "done" without this persisting
	// would let the flow finish (or chain onward) with no durable audit
	// artifact â€” fail-closed instead: escalate and stop.
	if store := s.persistenceStore(); store != nil {
		if err := PersistAuditDraft(ctx, store, parentRunID, node.ID, draft); err != nil {
			log.Printf("[flow-executor] audit: persist draft failed: %v", err)
			s.flowDiagLog(parentRunID, "flow_audit_persist_failed", "audit draft did not persist; blocking advance",
				"node_id", node.ID, "status", draft.Status, "error", err.Error(),
			)
			if _, ferr := s.applyFlowControl(parentRunID, FlowControlInput{
				Status:  "escalate",
				Summary: "Audit draft was built but could not be durably recorded (" + err.Error() + "). Not advancing until this is resolved â€” retry Continue once storage is available.",
			}); ferr != nil {
				log.Printf("[flow-executor] audit: escalate after persist failure failed: %v", ferr)
			}
			return true
		}
	}
	s.flowDiagLog(parentRunID, "flow_audit_draft_built", "built audit draft",
		"node_id", node.ID, "status", draft.Status, "feature_key", draft.FeatureKey,
	)

	// V9-01: never stamp audit DONE / flow done when validation was not positively
	// verified (blocked_validation_failed / blocked_missing_feature_key).
	if draft.Status != "ready" {
		if auditCtxCancelled(ctx, parentRunID, node.ID, "draft_not_ready") {
			return false
		}
		summary := "Audit blocked: validation was not positively verified (status=" + draft.Status + ", validation=" + draft.ValidationResult + ")."
		if draft.Status == "blocked_missing_feature_key" {
			summary = "Audit blocked: feature key missing or unverified; cannot finalize."
		}
		s.flowDiagLog(parentRunID, "flow_audit_blocked_not_ready", "audit draft not ready; escalating",
			"node_id", node.ID, "status", draft.Status, "validation", draft.ValidationResult,
		)
		if s.isFlowEngineDriven(parentRunID) {
			s.setFlowStepAwaitingUser(ctx, parentRunID)
		}
		if auditCtxCancelled(ctx, parentRunID, node.ID, "draft_not_ready_before_escalate") {
			return false
		}
		if _, err := s.applyFlowControl(parentRunID, FlowControlInput{Status: "escalate", Summary: summary}); err != nil {
			log.Printf("[flow-executor] audit: escalate for non-ready draft failed: %v", err)
			return false
		}
		return true
	}

	if auditCtxCancelled(ctx, parentRunID, node.ID, "before_terminal") {
		return false
	}

	// V10 / V9-11: mark audit DONE only after successor dispatch or flow-done
	// settles successfully â€” not before (failed successor left DONE false).
	// General post-node chaining: if this audit node's forward "done" edge
	// targets a real successor node (not the terminal "done"/"ask_user"),
	// dispatch it instead of settling the whole flow here â€” this is what lets a
	// user append e.g. a telegram.notify node after audit. rag-harness's
	// built-in "audit --done--> done" resolves to the terminal and falls
	// through to the unchanged settle below.
	if target, ok := edgeTargetFrom(edges, node.ID, "done", "forward"); ok && target != "done" && target != "ask_user" {
		if auditCtxCancelled(ctx, parentRunID, node.ID, "before_successor") {
			return false
		}
		okAdv := s.advanceToNextInlineOrDelegate(ctx, parentRunID, edges, nodes, node.ID, "done", RenderAuditDraftText(draft))
		// advanceToNextInlineOrDelegate marks source DONE on success already.
		return okAdv
	}
	if auditCtxCancelled(ctx, parentRunID, node.ID, "before_flow_done") {
		return false
	}
	if _, err := s.applyFlowControl(parentRunID, FlowControlInput{
		Status:  "done",
		Summary: RenderAuditDraftText(draft),
		Payload: map[string]any{"auditDraft": draft},
	}); err != nil {
		log.Printf("[flow-executor] audit: flow-done settle failed: %v", err)
		return false
	}
	if s.isFlowEngineDriven(parentRunID) {
		s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusDone)
	}
	return true
}

// auditCtxCancelled is true when Stop cancelled the inline flow context â€”
// callers must not escalate/dispatch/done after a stop race.
func auditCtxCancelled(ctx context.Context, parentRunID, nodeID, where string) bool {
	if ctx == nil || ctx.Err() == nil {
		return false
	}
	log.Printf("[flow-executor] audit cancelled at %s parent=%q node=%q: %v", where, parentRunID, nodeID, ctx.Err())
	return true
}

// resolveTelegramMessage picks the text a telegram.notify node sends for one
// bound target: the artifact's configured messageTemplate verbatim when set,
// otherwise a short default summarizing the prior node's result. v1 keeps this
// deterministic and does not substitute template variables (e.g. {{status}}) â€”
// that is a follow-up once a real per-run status value exists to fill them.
func resolveTelegramMessage(t telegramOutputTarget, resultMessage string) string {
	if strings.TrimSpace(t.messageTemplate) != "" {
		return t.messageTemplate
	}
	summary := strings.TrimSpace(resultMessage)
	if summary == "" {
		return "[FlowPilot] Run finished."
	}
	return "[FlowPilot] Run finished.\n\n" + truncateDisplayField(summary, 1500)
}

// runTelegramNotifyNode implements the telegram.notify inline behavior: it
// sends the node's bound telegram.v1 OUTPUT message(s) directly from the
// runner (no child agent spawn, no AI turn), reusing executeTelegramLoopbackSend
// â€” which resolves the connected bot credential under the runner's own HOME,
// honors the integration's auto-approve gate, and verifies a real message_id.
// This mirrors runValidateNode/runAuditNode: a deterministic Go inline node
// reached mid-flow that then follows its forward "done" edge. Because Go both
// performs and verifies the send, no flow-gate write-contract (r-artifact-
// telegram-sent) is needed for this path â€” that gate covers the AI-driven
// agent.delegate + MCP send, not this deterministic one.
func (s *InteractiveService) runTelegramNotifyNode(ctx context.Context, parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, node agentpack.FlowNode, resultMessage string) bool {
	targets := requiredTelegramOutputTargets(node)
	if len(targets) == 0 {
		// No telegram OUTPUT binding â†’ nothing to send. Pass through on the
		// forward "done" edge, same as a validate node skipped for no command.
		s.flowDiagLog(parentRunID, "flow_telegram_no_binding", "telegram.notify node has no bound OUTPUT target; passing through", "node_id", node.ID)
		return s.advanceToNextInlineOrDelegate(ctx, parentRunID, edges, nodes, node.ID, "done", resultMessage)
	}
	if s.runner == nil {
		if _, err := s.applyFlowControl(parentRunID, FlowControlInput{Status: "escalate", Summary: "Telegram notify unavailable: runner not configured."}); err != nil {
			log.Printf("[flow-executor] telegram.notify: escalate (no runner) failed: %v", err)
			return false
		}
		return true
	}
	for _, t := range targets {
		text := resolveTelegramMessage(t, resultMessage)
		resp, status := s.runner.executeTelegramLoopbackSend(ctx, telegramLoopbackSendRequest{ChatID: t.chatID, Text: text})
		if !resp.OK {
			// autoApprove OFF surfaces MCP_TOOL_APPROVAL_REQUIRED; a real send
			// failure surfaces its own code. Either way the message did not go
			// out, so escalate to ask_user rather than settling the flow done â€”
			// mirrors validate's failed_validation_max_retries escalate.
			summary := strings.TrimSpace(resp.Error)
			if summary == "" {
				summary = strings.TrimSpace(resp.ErrorCode)
			}
			if summary == "" {
				summary = "Telegram send did not complete."
			}
			s.flowDiagLog(parentRunID, "flow_telegram_send_blocked", "telegram.notify send did not complete",
				"node_id", node.ID, "chat_id", t.chatID, "error_code", resp.ErrorCode, "http_status", fmt.Sprint(status))
			if _, err := s.applyFlowControl(parentRunID, FlowControlInput{Status: "escalate", Summary: "Telegram notify not completed: " + summary}); err != nil {
				log.Printf("[flow-executor] telegram.notify: escalate failed: %v", err)
				return false
			}
			return true
		}
		s.flowDiagLog(parentRunID, "flow_telegram_sent", "sent telegram notification",
			"node_id", node.ID, "chat_id", t.chatID, "message_id", fmt.Sprint(resp.MessageID))
	}
	return s.advanceToNextInlineOrDelegate(ctx, parentRunID, edges, nodes, node.ID, "done", resultMessage)
}

// composeHubNotifyPrompt (Task-235) builds the reinvoke prompt for a
// hub.notify node: the SAME hub session that ran coder/reviewer/synthesis gets
// one more turn to compose and send the notification itself â€” no child spawn,
// no Go-only fixed text â€” so it can freely fill a telegram.v1 OUTPUT artifact's
// Message Template (e.g. "{{status}}" placeholders) from the flow's own
// context, which the Go-only telegram.notify path cannot do. Reuses
// appendTelegramOutputPrompt (already softened this session to read as a plain
// task rather than a coerced "you MUST" instruction, reducing the chance a
// receiving model mistakes its own flow's task for injected content) rather
// than composing a second, divergent Telegram write-contract wording.
// appendTelegramOutputPrompt no-ops if node has no telegram OUTPUT binding, so
// this is safe to call for a hub.notify node used for some other future
// non-Telegram write-contract shape too.
func composeHubNotifyPrompt(node agentpack.FlowNode) string {
	prompt := fmt.Sprintf("[flow-engine] This flow has reached its %q step, which finishes as part of this same turn â€” no separate agent is spawned for it.", node.ID)
	prompt = appendTelegramOutputPrompt(prompt, node)
	// BUG-286 follow-up: appendTelegramOutputPrompt shows the Message Template
	// verbatim but never says what to DO with it â€” a receiving model can (and
	// was observed to) just echo the template's own placeholder text back
	// unfilled. This turn already has the coder/reviewer/synthesis results in
	// its own visible context (it is the SAME hub session, not a fresh one â€”
	// that is the whole point of hub.notify over agent.delegate), so it is the
	// only node type that CAN legitimately fill a template from what actually
	// happened, rather than fabricating detail it never saw.
	prompt += "\n\nIf a message template above contains placeholders (e.g. {{status}}) or generic section labels, " +
		"replace them with the ACTUAL outcome of this run â€” what was worked on, what changed (or that nothing needed " +
		"to change), and the current state (tests, verdict, anything still open) â€” using the real results already in " +
		"this conversation. Do not send the template's placeholder text unfilled, and do not invent detail this " +
		"conversation does not actually show."
	// BUG-287: the ONLY flow-control tool actually exposed to this session for
	// every flow that can reach a hub.notify node today (review-loop.yaml /
	// context-coding-review-synthesis.yaml, both declaring
	// tools/submit-review-outcome.yaml â€” the sole path into hub.notify, since
	// it requires being chained after a hub.inline node) is
	// `submit_review_outcome`, whose `status` enum is strictly
	// `approved | changes_requested | blocked` (claude_mcp_server.go
	// sharedReviewOutcomeSchema) â€” there is no literal "done" value, and the
	// tool's own description explicitly says "This is the only flow-control
	// tool â€” do not use flow_control directly." Instructing the model to call
	// "the flow's control tool with status=\"done\"" asked it to do something
	// its actual tool schema cannot express; observed live, the model then
	// re-submitted an unrelated earlier verdict (status=blocked, stale
	// feedback text) instead, re-escalating a flow that had already sent its
	// notification. `approved` is the one enum value that maps to `done`
	// (statusMap in submit-review-outcome.yaml) and is named explicitly here
	// so the model has an actual, callable instruction â€” with a clarifying
	// note that it is a formality, not a real code-review judgment, so a
	// notify-only turn does not feel pressured to relate it to reviewing code.
	// Scoped fix, not fully generic: a hypothetical future flow using a
	// DIFFERENT declared control-tool name/schema for its hub.inline node
	// would need this resolved dynamically from that flow's own tool
	// declaration rather than hardcoded here â€” out of scope until such a flow
	// exists.
	prompt += "\n\nOnce that is done (or if this step has nothing to send), call `submit_review_outcome` with " +
		"status=\"approved\" to finish this step. This is a technical formality that lets the flow advance past this " +
		"step â€” it does not mean you are approving or judging a code change."
	return prompt
}
