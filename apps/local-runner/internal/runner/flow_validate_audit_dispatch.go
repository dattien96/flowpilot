package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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

// flowNodeInlineDispatchable reports whether a node's behavior can be dispatched
// in-process by tryAdvanceFlowThroughInline (validate/audit/notify/freeze/
// context.produce). A writer/delegate node (agent.code/agent.delegate) is NOT —
// Continue must retry that node's child run instead of a no-op inline dispatch
// (BUG-327, run-221516 scope-drift park on implement). Kept in lock-step with
// tryAdvanceFlowThroughInline's own switch so the two cannot drift apart.
func flowNodeInlineDispatchable(node agentpack.FlowNode) bool {
	canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior)
	if !ok {
		return false
	}
	switch canonical {
	case "command.validate", "artifact.audit_draft", "telegram.notify", "hub.notify", "hub.inline", "contract.freeze", "context.produce":
		return true
	}
	return false
}

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
// command.validate/artifact.audit_draft/telegram.notify/hub.notify/
// contract.freeze/context.produce (CA-732 added context.produce so a
// task-harness scout -> context -> plan_writer hop dispatches in-process
// instead of falling back to note+reinvoke-hub; context.render still has no
// entry here) so an unrecognized shape falls back to the pre-existing
// note+reinvoke-hub behavior unchanged.
func (s *InteractiveService) tryAdvanceFlowThroughInline(parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, node agentpack.FlowNode, resultMessage string) bool {
	canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior)
	if !ok {
		return false
	}
	// BUG-327 lock-step guard: Continue routes a non-inline escalated node to
	// child reinvoke via flowNodeInlineDispatchable — tryAdvance must never
	// dispatch a behavior that helper excludes, or a writer/delegate node would
	// no-op here and hang the flow waiting on nothing. If a new inline behavior
	// is added to the switch below, it MUST also be added to the helper.
	if !flowNodeInlineDispatchable(node) {
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
	case "hub.inline":
		s.dispatchHubNotifyNode(parentRunID, node)
		return true
	case "contract.freeze":
		// CP-55 P-3: freeze the read-only planner's proposal, then advance the
		// bounded freeze -> [context.produce ...] -> writer chain.
		return s.runContractFreezeNode(ctx, parentRunID, edges, nodes, node, resultMessage)
	case "context.produce":
		// run-198699: task-harness's scout -> context -> plan_writer hop. The
		// scout completes with a forward "done" edge into this inline node;
		// dispatch it in-process (like the freeze-chain context hops) and then
		// spawn its single forward target so the flow never falls back to the
		// note+reinvoke-hub path (whose prose turn then hub_stalled forever).
		return s.runContextProduceNode(ctx, parentRunID, edges, nodes, node, resultMessage)
	default:
		return false
	}
}

// runContextProduceNode dispatches a mid-flow context.produce node reached as
// the forward "done" target of a completed delegate (task-harness:
// preflight_contract_plan -> context -> plan_writer). It builds + stores the
// FlowContextPackage (mirroring advanceFlowThroughFreezeChain's
// buildAndStorePackage), stamps the context node DONE, then advances to the
// node's single forward target — an agent.delegate spawned with the rendered
// package in its prompt (like startInlineEntryChain), or an agent.code spawned
// as a frozen-contract writer. Any other shape returns false so the caller
// falls back to the pre-existing note+reinvoke-hub behavior unchanged.
//
// run-198699: without this the scout's completion reinvoked the hub, which
// prose-answered without submit_review_outcome and parked hub_stalled after
// 2m; Retry continue then no-op'd on a never-spawned plan_writer and stalled
// again (the S2 retry loop).
func (s *InteractiveService) runContextProduceNode(ctx context.Context, parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, node agentpack.FlowNode, resultMessage string) bool {
	// Match runContractFreezeNode's guard shape: a terminal run is claimed as
	// handled (never fall back to the note+reinvoke-hub path after Stop); a
	// loop that is not advancing is not ours to advance this tick.
	if s.flowRunTerminalLocked(parentRunID) {
		return true
	}
	if !s.loopIsAdvancing(parentRunID) {
		s.flowDiagLog(parentRunID, "flow_advance_context_skipped_loop_blocked", "skipping context.produce because loop is not advancing", "node_id", node.ID)
		return false
	}
	// Any produce/spawn/shape failure escalates to the operator (Retry/Stop
	// card) instead of returning false into the note+reinvoke-hub fallback —
	// that fallback is the original run-198699 hub_stalled class (the hub
	// prose-answers without submit_review_outcome and parks after 2m).
	escalate := func(reason string) bool {
		s.flowDiagLog(parentRunID, "flow_advance_context_blocked", reason, "node_id", node.ID)
		if s.isFlowEngineDriven(parentRunID) {
			s.stampLastEscalatedInlineNode(parentRunID, node.ID)
			s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusWaitingUserApr)
		}
		if _, err := s.applyFlowControl(parentRunID, FlowControlInput{
			Status:  "escalate",
			Summary: "Context production blocked: " + reason,
		}); err != nil {
			log.Printf("[flow-executor] context.produce escalate failed: %v", err)
		}
		return true
	}

	workspace := s.workspaceCwdFor(parentRunID)
	hints := FlowContextHints{
		WorkflowRunID: parentRunID,
		PlanStepRunID: node.ID,
		UserPrompt:    resultMessage,
	}
	if workspace != "" {
		hints.ExplicitSourcePaths = extractPromptSourcePaths(resultMessage)
		hints.ChangedPaths = uncommittedChangedPaths(workspace)
	}
	built, err := BuildFlowContextPackageWithSources(ctx, workspace, hints, node.ContextSources)
	if err != nil {
		return escalate("context production failed: " + err.Error())
	}
	pkg := &built
	// Stash + emit + persist exactly like advanceFlowThroughFreezeChain /
	// startInlineEntryChain so later nodes (validate/audit/retry prompts) can
	// read the package back.
	s.mu.Lock()
	if rs := s.runs[parentRunID]; rs != nil {
		rs.planContextPackage = pkg
		s.emitLocked(rs, ProviderEvent{
			Type:               EventFlowContextPackage,
			WorkflowRunID:      parentRunID,
			WorkflowStepRunID:  node.ID,
			FlowContextPackage: pkg,
		})
	}
	s.mu.Unlock()
	if store := s.persistenceStore(); store != nil {
		if err := PersistFlowContextPackage(ctx, store, *pkg); err != nil {
			s.flowDiagLog(parentRunID, "flow_advance_context_persist_failed", "persist context package failed mid-flow",
				"node_id", node.ID, "error", err.Error(),
			)
		}
	}

	targets := forwardDoneTargets(edges, node.ID)
	if len(targets) != 1 {
		return escalate(fmt.Sprintf("context node must have exactly one forward done target, got %d", len(targets)))
	}
	target, ok := findFlowNode(nodes, targets[0])
	if !ok {
		return escalate("forward target " + targets[0] + " missing from flow nodes")
	}
	canonical, ok := agentpack.NormalizeBehaviorID(target.Behavior)
	if !ok || (canonical != "agent.delegate" && canonical != "agent.code") {
		return escalate("forward target " + target.ID + " is not a spawnable delegate/writer (behavior " + target.Behavior + ")")
	}
	agentName := flowNodeAgentName(target)
	if agentName == "" {
		return escalate("forward target " + target.ID + " has no resolvable agent")
	}
	if canonical == "agent.code" {
		store, storeErr := changecontract.NewFrozenStore(workspace)
		if storeErr != nil {
			return escalate("writer " + target.ID + " frozen store: " + storeErr.Error())
		}
		rec, frozenOK, _ := store.GetFrozenForStep(parentRunID, target.ID)
		if !frozenOK {
			return escalate("writer " + target.ID + " has no frozen contract")
		}
		if err := s.spawnFrozenWriterChild(ctx, parentRunID, target, rec); err != nil {
			return escalate("writer " + target.ID + " spawn failed: " + err.Error())
		}
	} else {
		prompt := renderFlowContextPromptWithSecret(ctx, *pkg, resultMessage, s.markerSecret)
		prompt = composeFlowNodeAgentPrompt(workspace, prompt, target)
		prompt = appendChangeContractIfAnyWithSecret(workspace, parentRunID, prompt, s.markerSecret)
		fcpProvenanceRunID := pkg.WorkflowRunID
		if fcpProvenanceRunID == "" {
			fcpProvenanceRunID = pkg.PackageID
		}
		agentDef, _ := resolvePackAgentDefinition(agentName)
		if _, err := s.spawnChildRun(ctx, parentRunID, SpawnAgentInput{
			Agent:                    agentName,
			Prompt:                   prompt,
			Wait:                     false,
			Label:                    target.ID,
			AutoOrchestrate:          true,
			AgentDefOverride:         agentDef,
			Model:                    s.delegateSpawnModel(ctx, parentRunID, target),
			FCPMarkerProvenanceRunID: fcpProvenanceRunID,
		}); err != nil {
			return escalate("target " + target.ID + " spawn failed: " + err.Error())
		}
	}
	// V9-11: stamp the source DONE only after the spawn succeeded, so a spawn
	// failure escalates and Retry can re-dispatch context.produce instead of
	// leaving context DONE with nothing behind it. spawnFrozenWriterChild
	// already stamped the agent.code target RUNNING; stamp the delegate target
	// here.
	if s.isFlowEngineDriven(parentRunID) {
		s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusDone)
		if canonical != "agent.code" {
			s.setFlowStepStatus(ctx, parentRunID, target.ID, StepStatusRunning)
			s.stampFlowNodePosture(ctx, parentRunID, target)
		}
	}
	s.flowDiagLog(parentRunID, "flow_advance_context_spawned", "auto-advanced context target node spawned",
		"node_id", node.ID, "target_node_id", target.ID, "agent_name", agentName,
	)
	return true
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
	case RunStatusFailed, RunStatusCancelled:
		return true
	case RunStatusCompleted:
		if s.loopIsAdvancing(parentRunID) {
			return false
		}
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
	case RunStatusFailed, RunStatusCancelled:
		return alreadyCancelledContext()
	case RunStatusCompleted:
		if s.loopIsAdvancing(parentRunID) {
			break
		}
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
	// BUG-289 M4/F-8: if we previously escalated skipped_no_command but a
	// command is now configured, rebuild state so validate can actually run.
	if state.Status == "skipped_no_command" && strings.TrimSpace(command) != "" {
		planPackageID := state.OriginalPlanPackageID
		if planPackageID == "" && hasPkg {
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
		s.stampLastEscalatedInlineNode(parentRunID, node.ID)
		if _, err := s.applyFlowControl(parentRunID, FlowControlInput{
			Status:  "escalate",
			Summary: "Validation skipped: no test command configured in baseline. Configure .flowpilot/guard/test_baseline.json or Continue after manual verification.",
		}); err != nil {
			log.Printf("[flow-executor] validate: escalate after skipped_no_command failed: %v", err)
			return false
		}
		return true
	}

	// Make validate visible as RUNNING before the potentially long baseline +
	// command execution, so F2 and Thinking accurately reflect the inline step
	// (rag-harness F1: validate/audit looked pending while go test ran).
	if s.isFlowEngineDriven(parentRunID) {
		s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusRunning)
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
		// Task-293: rag-harness's validate forward-done edge now targets the
		// review cohort (reviewer), not only inline audit — route through the
		// full forward-done advance so a delegate/writer target can spawn.
		// Preserve the terminal (done/ask_user) settle for flows whose
		// validate edges straight to a terminal.
		if targetID, ok := edgeTargetFrom(edges, node.ID, "done", "forward"); ok && (targetID == "done" || targetID == "ask_user") {
			return s.advanceToNextInlineOrDelegate(ctx, parentRunID, edges, nodes, node.ID, "done", resultMessage)
		}
		return s.tryAdvanceFlowFromNode(parentRunID, node.ID, resultMessage)

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
		var fcpProvenanceRunID string
		if hasPkg {
			// BUG-288 R19-4: per-service marker secret on validation retry compose.
			prompt = ComposeRetryPromptWithSecret(pkg, state, s.markerSecret)
			// CP-51 Task-252: same trustID selection ComposeFlowCodingPrompt(WithSecret)
			// uses to mint the embedded "flowpilot-fcp" marker.
			fcpProvenanceRunID = pkg.WorkflowRunID
			if fcpProvenanceRunID == "" {
				fcpProvenanceRunID = pkg.PackageID
			}
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
		// BUG-318: the validate/audit retry target is a single-node retry (its
		// spawn fallback below carries no FlowCohortID), so pass cohortID="" — no
		// cohort re-registration, unchanged BUG-279 reuse behavior.
		if flowNodeReusesChild(targetNode) && s.reinvokeExistingFlowChild(parentRunID, targetNode.ID, prompt, "", 0) {
			if s.isFlowEngineDriven(parentRunID) {
				s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusDone)
				s.setFlowStepStatus(ctx, parentRunID, targetNode.ID, StepStatusRunning)
				s.stampFlowNodePosture(ctx, parentRunID, targetNode)
			}
			return true
		}
		agentDef, _ := resolvePackAgentDefinition(agentName)
		if _, err := s.spawnChildRun(ctx, parentRunID, SpawnAgentInput{
			Agent:                    agentName,
			Prompt:                   prompt,
			Wait:                     false,
			Label:                    targetNode.ID,
			AutoOrchestrate:          true,
			AgentDefOverride:         agentDef,
			Model:                    s.delegateSpawnModel(ctx, parentRunID, targetNode),
			FCPMarkerProvenanceRunID: fcpProvenanceRunID,
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
		// BUG-362: stamp THIS node WAITING, not setFlowStepAwaitingUser's
		// first-hub fallback — on dual-hub flows that stamps plan_synthesis
		// (run-210188: dead plan hub beside live validate). applyFlowControl
		// below re-stamps this same node via lastEscalated; idempotent.
		s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusWaitingUserApr)
		s.stampLastEscalatedInlineNode(parentRunID, node.ID)
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

// stampLastEscalatedInlineNode records the node for hub-less Continue (BUG-289 A5).
func (s *InteractiveService) stampLastEscalatedInlineNode(parentRunID, nodeID string) {
	if strings.TrimSpace(nodeID) == "" {
		return
	}
	s.mu.Lock()
	if rs := s.runs[parentRunID]; rs != nil {
		rs.lastEscalatedInlineNodeID = nodeID
	}
	s.mu.Unlock()
}

// maxInlineAdvanceDepth caps synchronous inline chains (BUG-289 L2/F-9).
// A mis-authored forward-done cycle among validate/audit nodes used to recurse
// until stack overflow; we escalate instead.
const maxInlineAdvanceDepth = 16

type inlineAdvanceDepthKey struct{}

// advanceToNextInlineOrDelegate follows the (fromNodeID, when, "forward")
// edge and dispatches whatever it finds: chains into another inline node
// (validate -> audit, when validate passes immediately), spawns a delegate
// node, or settles the flow via applyFlowControl for a terminal ("done" /
// "ask_user") target â€” reusing the exact same finalization applyFlowControl
// already performs for review-loop's synthesis node, since "an inline node
// reached its done/escalate edge" has identical semantics regardless of
// which node emitted it.
func (s *InteractiveService) advanceToNextInlineOrDelegate(ctx context.Context, parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, fromNodeID, when, resultMessage string) bool {
	depth := 0
	if v := ctx.Value(inlineAdvanceDepthKey{}); v != nil {
		if d, ok := v.(int); ok {
			depth = d
		}
	}
	if depth >= maxInlineAdvanceDepth {
		s.flowDiagLog(parentRunID, "flow_inline_cycle_cap", "inline advance depth cap hit; escalating",
			"from_node_id", fromNodeID, "depth", depth)
		if _, err := s.applyFlowControl(parentRunID, FlowControlInput{
			Status:  "escalate",
			Summary: fmt.Sprintf("Inline flow cycle or excessive chain detected at node %q (depth %d). Check forward-done edges among validate/audit nodes.", fromNodeID, depth),
		}); err != nil {
			log.Printf("[flow-executor] escalate after inline cycle cap failed: %v", err)
		}
		return true
	}
	ctx = context.WithValue(ctx, inlineAdvanceDepthKey{}, depth+1)

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
			Model:            s.delegateSpawnModel(ctx, parentRunID, nextNode),
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
		// run-201295: this switch had drifted from tryAdvanceFlowThroughInline —
		// contract.freeze / context.produce were dispatchable there (CA-732) but
		// silently returned false here, so a hub's approved "done" successor
		// (plan_synthesis -> preflight_contract_freeze) never ran: the flow idled
		// RUNNING for 2 minutes and parked hub_stalled (plan_synthesis
		// WAITING_USER_APPROVAL, no freeze, no card). Route through the shared
		// inline dispatcher; BUG-327's lock-step guard (flowNodeInlineDispatchable)
		// keeps the behavior lists in sync.
		if s.tryAdvanceFlowThroughInline(parentRunID, edges, nodes, nextNode, resultMessage) {
			markSourceDone()
			return true
		}
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
	pkg, hasPkg := s.loadPlanContextPackage(ctx, parentRunID)

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

	// Make audit visible as RUNNING before observation + BuildAuditDraft, so
	// F2 does not look pending while the audit gate runs (rag-harness F1).
	if s.isFlowEngineDriven(parentRunID) {
		s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusRunning)
	}

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

	// BUG-356: slice-only docs flows (cp-harness) declare no command.validate
	// node by design, so no validation state can ever exist — without this,
	// audit parks blocked_validation_failed with no forward path (Retry
	// re-runs the same empty state). When the flow by design cannot validate
	// AND the aggregate diff verifies docs-only, record a passed
	// slice-outputs verification instead of blocking forever. Flows WITH a
	// validate node keep the existing fail-closed behavior untouched, and the
	// tier-3 doc-rule pass below still runs as defense-in-depth.
	if state.Status == "" && !flowHasValidateNode(nodes) && len(changedFiles) > 0 && verifySliceOnlyOutputs(changedFiles) {
		planPackageID := ""
		if hasPkg {
			planPackageID = pkg.PackageID
		}
		state = NewFlowValidationRetryState(planPackageID, sliceOutputsCheckCommand)
		state.Status = "passed"
		s.mu.Lock()
		if rs := s.runs[parentRunID]; rs != nil {
			rs.flowValidationRetryState = &state
		}
		s.mu.Unlock()
		if store := s.persistenceStore(); store != nil {
			if err := PersistRetryState(ctx, store, parentRunID, node.ID, state); err != nil {
				log.Printf("[flow-executor] audit: persist slice-outputs state failed: %v", err)
			}
		}
		s.flowDiagLog(parentRunID, "flow_audit_slice_outputs_verified", "docs-only slice verified; validation passed",
			"node_id", node.ID, "artifacts", len(changedFiles),
		)
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
			contractDeclared := s.frozenContractDeclaredForRun(workspace, parentRunID)
			if !contractDeclared {
				if store, err := changecontract.OpenStoreReadOnly(workspace); err == nil && store != nil {
					if c, ok := store.GetLatestForRun(parentRunID); ok {
						contractDeclared = c.Confidence == changecontract.ConfidenceDeclared
					}
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
				// run-202550: stamp the audit node (not the plan hub) as the
				// escalated node — without this, applyFlowControl's escalate
				// settles plan_synthesis WAITING via setFlowStepAwaitingUser's
				// first-hub fallback, and Retry reinvokes the plan hub instead
				// of remediating the audit block (e.g. a missing CA note).
				// Settle audit WAITING directly (freeze-escalate shape) so the
				// plan hub is left alone — exactly one WAITING node.
				s.stampLastEscalatedInlineNode(parentRunID, node.ID)
				if s.isFlowEngineDriven(parentRunID) {
					s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusWaitingUserApr)
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

	// run-127174: package.FeatureKey was "grok" (catalog noise from .grok/skills), while the coder already declared "calc-format" in the contract store. Prefer the declared contract when it is registered, so a valid implementation does not escalate as blocked_missing_feature_key.
	if workspace != "" {
		if store, err := changecontract.OpenStoreReadOnly(workspace); err == nil && store != nil {
			if c, ok := store.GetLatestForRun(parentRunID); ok && strings.TrimSpace(c.FeatureKey) != "" && c.Confidence == changecontract.ConfidenceDeclared {
				if featureKeyRegistered(workspace, c.FeatureKey) {
					pkg.FeatureKey = c.FeatureKey
					pkg.FeatureConfidence = ConfidenceVerified
				}
			}
		}
		// run-147126: same as above but the declared contract lives in the
		// FrozenStore (CP-55 P-8 skipSave:true) — prefer it so the audit draft
		// does not resolve a catalog-noise feature key.
		if c, ok := s.frozenContractForRun(workspace, parentRunID); ok && strings.TrimSpace(c.FeatureKey) != "" {
			if featureKeyRegistered(workspace, c.FeatureKey) {
				pkg.FeatureKey = c.FeatureKey
				pkg.FeatureConfidence = ConfidenceVerified
			}
		}
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

// flowInlineChainHopLimit bounds the number of edge traversals
// resolveFreezeWriterTarget will follow from a contract.freeze node before
// giving up. Each loop iteration consumes exactly one edge, including the
// final edge that lands on the writer — so a chain of N intermediate
// context.produce hops before the writer requires N+1 iterations, meaning
// this limit admits at most flowInlineChainHopLimit-1 intermediate hops (5,
// at the current value of 6), not flowInlineChainHopLimit itself. A
// pathological or cyclic graph must fail closed instead of looping forever;
// no built-in flow needs more than a couple of hops here, so this is
// deliberately generous rather than tight (CP-55 P-3).
const flowInlineChainHopLimit = 6

// resolveFreezeWriterTarget walks forward from a contract.freeze node through
// zero or more intermediate "context.produce" hops — the only intermediate
// shape this chain knows how to actually execute (advanceFlowThroughFreezeChain
// dispatches it for real; see CA-426 C-3) — to the first forward target whose
// behavior is agent.code: the writer this contract binds to. Bounded by
// hopLimit hops and a visited-node-id set, so a cycle or a chain longer than
// the limit fails closed (ok=false) rather than looping forever.
//
// Any other shape also fails closed rather than being silently accepted:
// more than one forward-done edge at any point, a node id that doesn't
// resolve, an intermediate behavior other than context.produce (a safety
// node like command.validate/artifact.audit_draft must never be silently
// skipped — see CA-426 C-3), or a terminal target that isn't agent.code
// (binding a contract to, say, a plain agent.delegate reviewer step would be
// wrong — CA-426 M-5). Pure graph-structure logic — no dispatch, no I/O — so
// it is directly unit-testable against hand-built fixtures.
func resolveFreezeWriterTarget(edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, fromNodeID string, hopLimit int) (writer agentpack.FlowNode, path []agentpack.FlowNode, ok bool) {
	visited := map[string]bool{fromNodeID: true}
	current := fromNodeID
	for hop := 0; hop < hopLimit; hop++ {
		targets := forwardDoneTargets(edges, current)
		if len(targets) != 1 {
			return agentpack.FlowNode{}, nil, false
		}
		targetID := targets[0]
		if visited[targetID] {
			return agentpack.FlowNode{}, nil, false // cycle
		}
		visited[targetID] = true
		target, found := findFlowNode(nodes, targetID)
		if !found {
			return agentpack.FlowNode{}, nil, false
		}
		canonical, ok := agentpack.NormalizeBehaviorID(target.Behavior)
		if !ok {
			return agentpack.FlowNode{}, nil, false
		}
		switch canonical {
		case "context.produce":
			path = append(path, target)
			current = target.ID
			continue
		case "agent.code":
			return target, path, true
		default:
			return agentpack.FlowNode{}, nil, false
		}
	}
	return agentpack.FlowNode{}, nil, false // hop limit exceeded
}

// worktreeMutatedSincePaths returns the paths in current that are either
// absent from baseline or present with a different content hash — i.e.
// genuinely mutated since baseline, not merely "still dirty from before."
// A plain "is anything in current dirty" check cannot make this distinction
// (CA-426 C-1): a workspace with pre-existing uncommitted work at flow start
// would otherwise always read as "the planner mutated something."
func worktreeMutatedSincePaths(baseline, current map[string]string) []string {
	var mutated []string
	for p, hash := range current {
		// CA-638 / run-243681: FlowPilot's own runtime metadata (.flowpilot/**)
		// and the GitNexus index (.gitnexus/**) are written by the runner itself
		// every turn — never attribute them to the read-only planner.
		if isFlowPlannerExcludedPath(p) {
			continue
		}
		if baseline[p] != hash {
			mutated = append(mutated, p)
		}
	}
	sort.Strings(mutated)
	return mutated
}

// isFlowPlannerExcludedPath reports whether a worktree path is FlowPilot's own
// runtime metadata or a tool-owned surface — written by the runner
// (`.flowpilot/**` ledger/manifest/canonical/contracts/gate-metrics), by the
// GitNexus auto-indexer (`.gitnexus/**`, CA-639), or by skillpack/desktop
// skill sync (`.claude/**`, `.agents/**`, `.grok/**` agent skill dirs plus the
// root `AGENTS.md`/`CLAUDE.md`/`.gitignore` scaffold — CA-645), never by the
// contract planner. The planner-mutation guard in runContractFreezeNode must
// never flag these (run-243681: every turn rewrites chat_summary.ndjson +
// manifest.json; run-151954: gitnexus skillpack installs 7 SKILL.md +
// AGENTS.md + CLAUDE.md + .gitignore mid-flow — both permanently
// false-blocked contract.freeze → WAITING_USER_APPROVAL park).
func isFlowPlannerExcludedPath(path string) bool {
	p := filepath.ToSlash(strings.TrimSpace(path))
	if p == ".flowpilot" || strings.HasPrefix(p, ".flowpilot/") ||
		p == ".gitnexus" || strings.HasPrefix(p, ".gitnexus/") {
		return true
	}
	// run-201704: task-harness freezes AFTER plan_writer, so the plan phase's
	// own Task md (requirements/08-Task/todo/Task-*.md) lands between the
	// flow-start baseline and the freeze check — a legitimate plan artifact,
	// never the read-only planner touching code. Doc/audit surfaces
	// (requirements/**, change-audit/**, *.md) are never planner mutations;
	// the gate scope path already treats them the same way
	// (flowgate.IsDocOrAuditFile). A planner-written *.go still blocks.
	if flowgate.IsDocOrAuditFile(p) {
		return true
	}
	return changecontract.IsToolOwnedScaffoldPath(p)
}

// runContractFreezeNodeLocks path-keys an in-process mutex per
// (parentRunID, freezeNodeID) so two concurrent deliveries of the SAME
// freeze-node completion (e.g. an async resume redelivery racing the
// synchronous completion path) serialize entirely, instead of each opening
// its own FrozenStore instance and racing on "does this already exist" —
// FrozenStore's own path-keyed mutex only serializes the physical NDJSON
// append, not this runner-level check-then-decide sequence (CA-426 I-1).
var (
	runContractFreezeNodeLocksMu sync.Mutex
	runContractFreezeNodeLocks   = map[string]*sync.Mutex{}
)

func runContractFreezeNodeLockFor(key string) *sync.Mutex {
	runContractFreezeNodeLocksMu.Lock()
	defer runContractFreezeNodeLocksMu.Unlock()
	if mu, ok := runContractFreezeNodeLocks[key]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	runContractFreezeNodeLocks[key] = mu
	return mu
}

// baselineWorktreeFingerprint returns a deterministic content fingerprint for
// every currently-dirty path in workspace (CP-55 P-2's
// FrozenContractRecord.BaselineWorktree): a Flow can freeze a contract against
// an uncommitted tree, so BaseSHA alone cannot distinguish two freezes that
// share the same HEAD but different working-tree state. Best-effort — an
// unreadable file yields an empty-string fingerprint entry rather than
// failing the whole freeze; a workspace with nothing dirty yields nil.
//
// Fix run-144900: use the full dirty snapshot (flowgate.ObserveGitDiff) so
// leftover untracked skill dirs (e.g. .agents/skills/.../SKILL.md) are
// captured in the baseline. The previous impl used uncommittedChangedPaths
// which excludes docs/*.md and caps at 20 paths and therefore missed the
// leftover that later tripped a false scope-drift park on test_signatures.
func baselineWorktreeFingerprint(workspace string) map[string]string {
	if strings.TrimSpace(workspace) == "" {
		return nil
	}
	files, err := flowgate.ObserveGitDiff(workspace)
	if err != nil || len(files) == 0 {
		return nil
	}
	out := make(map[string]string, len(files))
	for _, f := range files {
		p := filepath.ToSlash(f.Path)
		if p == "" || isFlowPlannerExcludedPath(p) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(p)))
		if err != nil {
			out[p] = ""
			continue
		}
		sum := sha256.Sum256(data)
		out[p] = hex.EncodeToString(sum[:])
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// runContractFreezeNode is CP-55 P-3's contract.freeze dispatch: it enforces
// that the read-only planner produced no project mutations, strictly parses
// and validates its proposal, resolves the writer this contract will bind to,
// freezes the contract durably (reload-verified before advancing — spec step
// 7), and then advances the bounded freeze -> [context.produce hops] ->
// writer chain. Any failure (invalid draft, planner mutation, unresolvable
// writer, storage error) escalates via applyFlowControl and returns true
// (handled — the legacy note+reinvoke-hub fallback must never see an unbound
// writer dispatch for this behavior).
//
// Fixed after Claude-agent review (2026-07-31, see CA-426 C-1/C-3/I-1/I-2/I-4):
// the planner-mutation check now diffs against the flow-start worktree
// fingerprint instead of treating any dirty file as the planner's doing; the
// loop-advancing status is re-checked (BUG-234 class); duplicate-delivery
// resolution is serialized in-process per (run, freeze-node); and the
// mint-vs-reuse version number is derived from the store instead of
// hardcoded, so a future superseded/abandoned version cannot permanently
// collide with a fresh one.
// findPlannerResultForFreeze retrieves the planner output from the predecessor child run
// when plannerResult is empty or unparseable (e.g. user submitted "/continue" feedback).
func (s *InteractiveService) findPlannerResultForFreeze(parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, freezeNodeID string) string {
	var fromNodeID string
	for _, e := range edges {
		if e.To == freezeNodeID {
			fromNodeID = e.From
			break
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, child := range s.runs {
		if child.parentRunID != parentRunID {
			continue
		}
		if fromNodeID != "" && (child.label == fromNodeID || child.stepID == fromNodeID) {
			for i := len(child.events) - 1; i >= 0; i-- {
				if child.events[i].Type == EventTurnCompleted && child.events[i].FinalMessage != "" {
					return child.events[i].FinalMessage
				}
			}
		}
	}
	for _, child := range s.runs {
		if child.parentRunID != parentRunID {
			continue
		}
		for i := len(child.events) - 1; i >= 0; i-- {
			if child.events[i].Type == EventTurnCompleted && child.events[i].FinalMessage != "" {
				if _, err := changecontract.ParsePreflightDraft(child.events[i].FinalMessage); err == nil {
					return child.events[i].FinalMessage
				}
			}
		}
	}
	// BUG-360: post-restart the transient scout child is gone — fall back to
	// the draft cached on the parent at scout completion (durable via session
	// snapshot + runtime blob). Parse-gated like every other source here, so
	// a stale/corrupt cache can never satisfy the freeze.
	if rs := s.runs[parentRunID]; rs != nil {
		if cached := strings.TrimSpace(rs.preflightDraftResult); cached != "" {
			if _, err := changecontract.ParsePreflightDraft(cached); err == nil {
				return cached
			}
		}
	}
	return ""
}

func (s *InteractiveService) runContractFreezeNode(ctx context.Context, parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, node agentpack.FlowNode, plannerResult string) bool {
	if s.flowRunTerminalLocked(parentRunID) {
		return true
	}
	if !s.loopIsAdvancing(parentRunID) {
		s.flowDiagLog(parentRunID, "flow_contract_freeze_skipped_loop_blocked", "skipping freeze because loop is not advancing", "node_id", node.ID)
		return false
	}

	s.mu.Lock()
	rs := s.runs[parentRunID]
	var workspace, baseSHA string
	var startFingerprint map[string]string
	if rs != nil {
		workspace = rs.workspaceCwd
		baseSHA = rs.flowStartGitHead
		startFingerprint = rs.flowStartWorktreeFingerprint
	}
	s.mu.Unlock()

	escalate := func(reason string) bool {
		s.flowDiagLog(parentRunID, "flow_contract_freeze_blocked", reason, "node_id", node.ID)
		if s.isFlowEngineDriven(parentRunID) {
			s.stampLastEscalatedInlineNode(parentRunID, node.ID)
			s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusWaitingUserApr)
		}
		if _, err := s.applyFlowControl(parentRunID, FlowControlInput{
			Status:  "escalate",
			Summary: "Contract freeze blocked: " + reason,
		}); err != nil {
			log.Printf("[flow-executor] contract.freeze escalate failed: %v", err)
		}
		return true
	}

	// Enforce the planner produced no project mutations (it is read-only by
	// contract). Diffing against a plain "is the tree dirty" check would fire
	// on any pre-existing uncommitted work the user already had when the flow
	// started — instead, diff a fresh fingerprint against the one captured
	// once at flow start (rs.flowStartWorktreeFingerprint, before the planner
	// — the flow's entry node — ever ran): a path new or changed-hash
	// relative to that baseline is the planner's own mutation; a path dirty
	// in both with the same hash is pre-existing and not the planner's doing.
	if workspace != "" {
		currentFingerprint := baselineWorktreeFingerprint(workspace)
		if mutated := worktreeMutatedSincePaths(startFingerprint, currentFingerprint); len(mutated) > 0 {
			return escalate(fmt.Sprintf("planner changed %d file(s) (%s); the contract planner must be read-only", len(mutated), strings.Join(mutated, ", ")))
		}
	}

	draft, err := changecontract.ParsePreflightDraft(plannerResult)
	if err != nil {
		if fallback := s.findPlannerResultForFreeze(parentRunID, edges, nodes, node.ID); fallback != "" && fallback != plannerResult {
			if d2, err2 := changecontract.ParsePreflightDraft(fallback); err2 == nil {
				draft = d2
				err = nil
			}
		}
	}
	if err != nil {
		return escalate("invalid planner proposal: " + err.Error())
	}
	// CP-55 P-3 deliberately does not allowlist feature_key against the
	// catalog here (knownFeatureKeys=nil skips that check in
	// ValidatePreflightDraft) — the planner's feature_key is trusted for now;
	// catalog-backed validation can be added later without an API change.
	draft, err = changecontract.ValidatePreflightDraft(draft, nil)
	if err != nil {
		return escalate("invalid planner proposal: " + err.Error())
	}
	writerNode, path, direct, ok := freezeWriterBinding(edges, nodes, node.ID)
	if !ok {
		return escalate("no reachable agent.code writer target for this contract.freeze node")
	}

	// Serialize concurrent deliveries of THIS SAME freeze-node completion
	// (e.g. an async resume redelivery racing the synchronous completion
	// path) — held only across the check-then-freeze-then-save decision
	// below, not across the chain-advance/spawn that follows.
	lock := runContractFreezeNodeLockFor(parentRunID + "\x00" + node.ID)
	lock.Lock()

	store, err := changecontract.NewFrozenStore(workspace)
	if err != nil {
		lock.Unlock()
		return escalate("cannot open frozen contract store: " + err.Error())
	}

	// Duplicate delivery / recovery: a prior process may already have frozen
	// this exact (runID, coderStepID) — reuse it rather than minting a second
	// version (the version bump/Supersedes chain is reserved for genuine
	// amendments, CP-55 P-4).
	if existing, ok, _ := store.GetFrozenForStep(parentRunID, writerNode.ID); ok {
		lock.Unlock()
		s.flowDiagLog(parentRunID, "flow_contract_freeze_reused", "reusing already-frozen contract for this step",
			"node_id", node.ID, "coder_node_id", writerNode.ID, "contract_id", existing.ContractID, "version", existing.Version,
		)
		if s.isFlowEngineDriven(parentRunID) {
			s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusDone)
		}
		// Task-293: recovery/repeat delivery must also (re)bind any sibling
		// agent.code writer still missing a contract, from the already-frozen
		// record's own draft fields.
		existingDraft := changecontract.PreflightContractDraft{
			FeatureKey:    existing.FeatureKey,
			Intent:        existing.Intent,
			DeclaredPaths: existing.DeclaredPaths,
			SourceDocID:   existing.SourceDocID,
		}
		if err := s.bindFrozenContractToSiblingWriters(store, workspace, parentRunID, node.ID, existingDraft, nodes, writerNode.ID, existing.BaseSHA, existing.BaselineWorktree); err != nil {
			return escalate("could not bind sibling writer contracts: " + err.Error())
		}
		return s.advanceAfterContractFreeze(ctx, parentRunID, edges, nodes, node, writerNode, existing, path, direct, plannerResult)
	}

	// Derive the next version from what's actually on disk (not hardcoded 1):
	// once P-4/P-5 land amendment/abandon transitions, GetFrozenForStep can
	// report "no active version" even though prior (superseded/abandoned)
	// versions exist — recomputing version=1 in that case would collide with
	// an old version's ContractID (ComputeContractID does not include
	// DeclaredAt) and SaveFrozen would permanently reject it as a payload
	// mismatch.
	versions, _ := store.ListVersionsForStep(parentRunID, writerNode.ID)
	version := len(versions) + 1

	baseline := baselineWorktreeFingerprint(workspace)
	rec, err := changecontract.FreezeContract(workspace, parentRunID, node.ID, writerNode.ID, draft, baseSHA, baseline, "", version, time.Now().UTC())
	if err != nil {
		lock.Unlock()
		return escalate("could not normalize declared scope: " + err.Error())
	}
	if err := store.SaveFrozen(rec); err != nil {
		lock.Unlock()
		return escalate("could not persist frozen contract: " + err.Error())
	}
	// Task-293: the freeze node's contract governs EVERY agent.code writer in
	// the flow (rag-harness: test_signatures AND implement), so each writer's
	// own gate pass finds a frozen contract bound to its node id.
	if err := s.bindFrozenContractToSiblingWriters(store, workspace, parentRunID, node.ID, draft, nodes, writerNode.ID, baseSHA, baseline); err != nil {
		lock.Unlock()
		return escalate("could not bind sibling writer contracts: " + err.Error())
	}
	lock.Unlock()

	// Reload-verify durability before advancing (spec step 7): open a fresh
	// store instance and confirm the record survives, rather than trusting
	// the in-memory instance that just wrote it.
	reloaded, err := changecontract.NewFrozenStore(workspace)
	if err != nil {
		return escalate("could not reload frozen contract store: " + err.Error())
	}
	// Reload-verify every bound writer survives (first writer + siblings).
	verifyIDs := []string{writerNode.ID}
	for _, w := range flowAgentCodeWriterNodes(nodes) {
		if w.ID != writerNode.ID {
			verifyIDs = append(verifyIDs, w.ID)
		}
	}
	for _, id := range verifyIDs {
		if _, ok, _ := reloaded.GetFrozenForStep(parentRunID, id); !ok {
			return escalate(fmt.Sprintf("frozen contract for writer %q did not survive reload", id))
		}
	}

	if s.isFlowEngineDriven(parentRunID) {
		s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusDone)
	}
	// BUG-362: freeze DONE closes the plan phase — settle a stale
	// plan_synthesis WAITING (consumed CA-749 park) so the step timeline
	// tracks the code phase. No-op unless freeze reads DONE.
	s.settlePlanSynthesisAfterFreezeDone(parentRunID)
	s.flowDiagLog(parentRunID, "flow_contract_frozen", "froze preflight contract before writer dispatch",
		"node_id", node.ID, "coder_node_id", writerNode.ID, "contract_id", rec.ContractID, "version", rec.Version,
	)

	return s.advanceAfterContractFreeze(ctx, parentRunID, edges, nodes, node, writerNode, rec, path, direct, plannerResult)
}

// advanceFlowThroughFreezeChain dispatches each intermediate context.produce
// hop in path (every entry is guaranteed to be context.produce by
// resolveFreezeWriterTarget — no other inline behavior is ever admitted into
// path), then spawns writerNode as a child agent run bound to rec. Any
// inline-hop failure, or a spawn failure, escalates and stops before/without
// the writer ever running (CP-55 P-3's own "no writer dispatch after
// freeze/chain failure" invariant — fixed after Claude-agent review, CA-426
// C-2: the spawn-failure path previously returned false/unescalated, which
// could leak an unbound-writer situation to the legacy hub fallback).
//
// CORRECTION (CP-55 P-8, found while fixing test fallout from the built-in
// flow migration): when path is empty — review-loop's own freeze -> coder
// edge is direct, with no intermediate context.produce node — this used to
// mean NO FlowContextPackage was ever built for the writer at all, and the
// writer's prompt was just rec.Intent wrapped in its system prompt: the
// source-excerpt/feature-history machinery P-6/P-7 built had no path to the
// coder's actual prompt. A package is now always built (from the last
// path hop's package if one exists, else freshly for the writer's own
// declared context sources) and rendered into the writer's prompt via the
// same context.render behavior startInlineEntryChain already uses — path
// having zero context.produce hops is no longer a silent "skip context
// entirely" case.
func (s *InteractiveService) advanceFlowThroughFreezeChain(ctx context.Context, parentRunID string, freezeNode, writerNode agentpack.FlowNode, rec changecontract.FrozenContractRecord, path []agentpack.FlowNode) bool {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	var workspace string
	if rs != nil {
		workspace = rs.workspaceCwd
	}
	s.mu.Unlock()

	escalate := func(nodeID, reason string) bool {
		s.flowDiagLog(parentRunID, "flow_contract_freeze_chain_failed", reason, "node_id", nodeID)
		if s.isFlowEngineDriven(parentRunID) {
			if strings.TrimSpace(nodeID) != "" {
				s.stampLastEscalatedInlineNode(parentRunID, nodeID)
				s.setFlowStepStatus(ctx, parentRunID, nodeID, StepStatusWaitingUserApr)
			} else {
				s.setFlowStepAwaitingUser(ctx, parentRunID)
			}
		}
		if _, err := s.applyFlowControl(parentRunID, FlowControlInput{
			Status:  "escalate",
			Summary: "Contract-freeze chain blocked: " + reason,
		}); err != nil {
			log.Printf("[flow-executor] contract-freeze-chain escalate failed: %v", err)
		}
		return true
	}

	var pkg *FlowContextPackage
	buildAndStorePackage := func(nodeID string, sourceIDs []string) error {
		hints := FlowContextHints{
			WorkflowRunID:      parentRunID,
			PlanStepRunID:      nodeID,
			UserPrompt:         rec.Intent,
			SourceDocID:        rec.SourceDocID,
			ResolvedFeatureKey: rec.FeatureKey,
		}
		if workspace != "" {
			hints.ExplicitSourcePaths = rec.DeclaredPaths
			hints.ChangedPaths = uncommittedChangedPaths(workspace)
		}
		built, err := BuildFlowContextPackageWithSources(ctx, workspace, hints, sourceIDs)
		if err != nil {
			return err
		}
		// Always store/emit this contract-scoped package, even when an
		// earlier plan-stage package already exists — it is the authoritative
		// one for this frozen contract (seeded with rec.DeclaredPaths), not a
		// fallback to skip if something else got there first.
		pkg = &built
		s.mu.Lock()
		if rs != nil {
			rs.planContextPackage = pkg
			s.emitLocked(rs, ProviderEvent{
				Type:               EventFlowContextPackage,
				WorkflowRunID:      parentRunID,
				WorkflowStepRunID:  nodeID,
				FlowContextPackage: pkg,
			})
		}
		s.mu.Unlock()
		return nil
	}

	for _, mid := range path {
		if err := buildAndStorePackage(mid.ID, mid.ContextSources); err != nil {
			return escalate(mid.ID, "context production failed: "+err.Error())
		}
		if s.isFlowEngineDriven(parentRunID) {
			s.setFlowStepStatus(ctx, parentRunID, mid.ID, StepStatusDone)
		}
	}

	// CP-55 P-8 fix: a flow whose freeze node edges directly to its writer
	// (review-loop's own shape — no intermediate context.produce hop in path
	// at all) must still give the writer a rendered FlowContextPackage, not
	// just rec.Intent's bare sentence. Build one scoped to the writer's own
	// declared context sources when the loop above never ran.
	if pkg == nil {
		if err := buildAndStorePackage(writerNode.ID, writerNode.ContextSources); err != nil {
			return escalate(writerNode.ID, "context production failed: "+err.Error())
		}
	}

	// Re-check right before spawning: the chain above can take real time
	// (context production runs the full source registry) — a Stop or a loop
	// settle landing during it must not still result in a spawn.
	if s.flowRunTerminalLocked(parentRunID) || !s.loopIsAdvancing(parentRunID) {
		s.flowDiagLog(parentRunID, "flow_contract_freeze_chain_aborted", "run settled during chain advance; not spawning writer", "node_id", writerNode.ID)
		return true
	}

	if err := s.spawnFrozenWriterChild(ctx, parentRunID, writerNode, rec); err != nil {
		return escalate(writerNode.ID, "failed to spawn writer: "+err.Error())
	}
	return true
}

// spawnFrozenWriterChild spawns writerNode as a child agent run bound to rec
// (a frozen preflight contract). Composes the writer prompt from the rendered
// context package (when one exists) + the node's own static promptTemplate +
// the frozen change contract. Shared by advanceFlowThroughFreezeChain (first
// writer after freeze) and tryAdvanceFlowFromNode (subsequent agent.code
// writers, e.g. rag-harness's test_signatures -> implement) so both paths use
// the same writer prompt and never the review handoff.
func (s *InteractiveService) spawnFrozenWriterChild(ctx context.Context, parentRunID string, writerNode agentpack.FlowNode, rec changecontract.FrozenContractRecord) error {
	workspace := s.workspaceCwdFor(parentRunID)
	pkg, hasPkg := s.loadPlanContextPackage(ctx, parentRunID)
	writerPrompt := rec.Intent
	if hasPkg {
		writerPrompt = renderFlowContextPromptWithSecret(ctx, pkg, rec.Intent, s.markerSecret)
	}
	prompt := composeFlowNodeAgentPrompt(workspace, writerPrompt, writerNode)
	prompt = appendChangeContractIfAnyWithSecret(workspace, parentRunID, prompt, s.markerSecret)
	agentName := flowNodeAgentName(writerNode)
	if agentName == "" {
		return fmt.Errorf("writer node %q has no resolvable agent", writerNode.ID)
	}
	agentDef, _ := resolvePackAgentDefinition(agentName)
	if _, err := s.spawnChildRun(ctx, parentRunID, SpawnAgentInput{
		Agent:            agentName,
		Prompt:           prompt,
		Wait:             false,
		Label:            writerNode.ID,
		AgentDefOverride: agentDef,
		Model:            s.delegateSpawnModel(ctx, parentRunID, writerNode),
	}); err != nil {
		return fmt.Errorf("failed to spawn writer node %q (agent %q): %w", writerNode.ID, agentName, err)
	}
	if s.isFlowEngineDriven(parentRunID) {
		s.setFlowStepStatus(ctx, parentRunID, writerNode.ID, StepStatusRunning)
		s.stampFlowNodePosture(ctx, parentRunID, writerNode)
	}
	s.flowDiagLog(parentRunID, "flow_contract_freeze_writer_spawned", "spawned frozen-contract writer",
		"node_id", writerNode.ID, "agent_name", agentName, "contract_id", rec.ContractID,
	)
	return nil
}

// freezeWriterBinding picks the agent.code writer a contract.freeze node binds
// to. The CP-55 direct chain (freeze → [context.produce]* → agent.code) still
// owns writer spawn. vibe-sprint (SD-24) puts TDD as agent.delegate between
// context and coder — that hop is not a freeze-chain skip (CA-426 C-3). Bind
// the first agent.code in the graph and follow the freeze done-edge normally
// so TDD still runs before coder.
func freezeWriterBinding(edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, freezeNodeID string) (writer agentpack.FlowNode, path []agentpack.FlowNode, direct bool, ok bool) {
	if w, p, resolved := resolveFreezeWriterTarget(edges, nodes, freezeNodeID, flowInlineChainHopLimit); resolved {
		return w, p, true, true
	}
	writers := flowAgentCodeWriterNodes(nodes)
	if len(writers) == 0 {
		return agentpack.FlowNode{}, nil, false, false
	}
	return writers[0], nil, false, true
}

func (s *InteractiveService) advanceAfterContractFreeze(ctx context.Context, parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, freezeNode, writerNode agentpack.FlowNode, rec changecontract.FrozenContractRecord, path []agentpack.FlowNode, direct bool, resultMessage string) bool {
	if direct {
		return s.advanceFlowThroughFreezeChain(ctx, parentRunID, freezeNode, writerNode, rec, path)
	}
	return s.advanceToNextInlineOrDelegate(ctx, parentRunID, edges, nodes, freezeNode.ID, "done", resultMessage)
}
// flowAgentCodeWriterNodes returns every agent.code writer node in nodes — the
// nodes a frozen preflight contract must be bound to (Task-293: a freeze node
// governs the whole coding chain, e.g. rag-harness's test_signatures AND
// implement, not only the first writer resolveFreezeWriterTarget finds).
func flowAgentCodeWriterNodes(nodes []agentpack.FlowNode) []agentpack.FlowNode {
	var out []agentpack.FlowNode
	for _, n := range nodes {
		canonical, ok := agentpack.NormalizeBehaviorID(n.Behavior)
		if !ok || canonical != "agent.code" {
			continue
		}
		out = append(out, n)
	}
	return out
}

// frozenContractForRun returns an active frozen preflight contract for the
// parent run — the first agent.code writer node in the live topology that has
// a frozen (not superseded/abandoned) record. A frozen contract IS a declared
// Change Contract; the coder is deliberately skipSave (CP-55 P-8) so the
// legacy Store never sees it, which previously made audit tier-3 fire
// r-contract on a valid run (run-147126).
func (s *InteractiveService) frozenContractForRun(workspace, parentRunID string) (changecontract.FrozenContractRecord, bool) {
	if strings.TrimSpace(workspace) == "" || parentRunID == "" {
		return changecontract.FrozenContractRecord{}, false
	}
	frozenStore, err := changecontract.NewFrozenStore(workspace)
	if err != nil {
		return changecontract.FrozenContractRecord{}, false
	}
	nodes := s.activeFlowNodesFor(parentRunID)
	for _, w := range flowAgentCodeWriterNodes(nodes) {
		if rec, ok, _ := frozenStore.GetFrozenForStep(parentRunID, w.ID); ok {
			return rec, true
		}
	}
	// Fallback: restart may not have live topology — scan any record bound to
	// this run that is still active, regardless of step id.
	if recs, err := frozenStore.ListForRun(parentRunID); err == nil {
		for _, rec := range recs {
			if rec.CoderStepID != "" {
				if _, ok, _ := frozenStore.GetFrozenForStep(parentRunID, rec.CoderStepID); ok {
					return rec, true
				}
			}
		}
	}
	return changecontract.FrozenContractRecord{}, false
}

// frozenContractDeclaredForRun reports whether any active frozen contract
// exists for the run (see frozenContractForRun).
func (s *InteractiveService) frozenContractDeclaredForRun(workspace, parentRunID string) bool {
	_, ok := s.frozenContractForRun(workspace, parentRunID)
	return ok
}

// bindFrozenContractToSiblingWriters freezes the same validated draft for
// every agent.code writer in nodes other than firstWriterID, so each writer's
// own gate pass (GetFrozenForStep keyed by its node id) finds a frozen
// contract. Idempotent: a step that already has an active frozen record is
// skipped (duplicate delivery / recovery reuse path).
func (s *InteractiveService) bindFrozenContractToSiblingWriters(store *changecontract.FrozenStore, workspace, runID, plannerStepID string, draft changecontract.PreflightContractDraft, nodes []agentpack.FlowNode, firstWriterID, baseSHA string, baseline map[string]string) error {
	for _, w := range flowAgentCodeWriterNodes(nodes) {
		if w.ID == firstWriterID {
			continue
		}
		if _, ok, _ := store.GetFrozenForStep(runID, w.ID); ok {
			continue
		}
		versions, _ := store.ListVersionsForStep(runID, w.ID)
		rec, err := changecontract.FreezeContract(workspace, runID, plannerStepID, w.ID, draft, baseSHA, baseline, "", len(versions)+1, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("writer %q: %w", w.ID, err)
		}
		if err := store.SaveFrozen(rec); err != nil {
			return fmt.Errorf("writer %q: %w", w.ID, err)
		}
	}
	return nil
}
