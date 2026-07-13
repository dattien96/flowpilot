package runner

import (
	"context"
	"log"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/agentpack"
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
// one specific source node — needed here because validate/audit each have
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
	ctx := context.Background()
	switch canonical {
	case "command.validate":
		return s.runValidateNode(ctx, parentRunID, edges, nodes, node, resultMessage)
	case "artifact.audit_draft":
		return s.runAuditNode(ctx, parentRunID, node, resultMessage)
	default:
		return false
	}
}

// loadValidateCommand resolves the validate node's shell command from
// .flowpilot/guard/test_baseline.json's test_command field (BUG-243 Q-1's
// own suggested resolution — the CP-35 gate/regression-guard baseline
// already captures a working test command per-project; reusing it avoids
// inventing a second config surface). Returns "" when no baseline exists or
// it declares no command — NewFlowValidationRetryState already treats an
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
// (BUG-243 F-0 addition — it previously only rendered the package into the
// one prompt handoff and discarded it, leaving nothing for validate/audit to
// read later in the same run). Falls back to scanning rs.events directly
// (in-memory, no store round-trip — mirrors interactive_resume.go's own
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
// the exact same CP-35 gate observation (flowgate.ObserveGitDiffSince) the
// bug doc points at (F-1: "same observation flowgate uses"). Non-fatal: an
// observation error yields an empty list rather than blocking validate/audit.
func changedFilesSince(cwd, baseSHA string) []string {
	if cwd == "" {
		return nil
	}
	diff, err := flowgate.ObserveGitDiffSince(cwd, baseSHA)
	if err != nil {
		return nil
	}
	return changedPathsFromDiff(diff)
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
		baseSHA = rs.turnStartGitHead
		prevTurnID = rs.lastTurnID
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
		s.flowDiagLog(parentRunID, "flow_validate_skipped_no_command", "no validate command configured; forwarding to audit", "node_id", node.ID)
		return s.advanceToNextInlineOrDelegate(ctx, parentRunID, edges, nodes, node.ID, "done", resultMessage)
	}

	result := RunValidationCommand(ctx, command, workDir)
	changedFiles := changedFilesSince(workDir, baseSHA)
	AdvanceRetryState(&state, result, changedFiles, prevTurnID)

	s.mu.Lock()
	if rs := s.runs[parentRunID]; rs != nil {
		rs.flowValidationRetryState = &state
	}
	s.mu.Unlock()

	if store := s.persistenceStore(); store != nil {
		if err := PersistValidationResult(ctx, store, parentRunID, node.ID, result); err != nil {
			log.Printf("[flow-executor] validate: persist result failed: %v", err)
		}
		if err := PersistRetryState(ctx, store, parentRunID, node.ID, state); err != nil {
			log.Printf("[flow-executor] validate: persist retry state failed: %v", err)
		}
	}
	s.flowDiagLog(parentRunID, "flow_validate_ran", "ran validate command",
		"node_id", node.ID, "status", state.Status, "attempt", state.RetryAttempt, "exit_code", result.ExitCode,
	)

	switch state.Status {
	case "passed", "skipped_env_error":
		return s.advanceToNextInlineOrDelegate(ctx, parentRunID, edges, nodes, node.ID, "done", resultMessage)

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
		agentName := flowNodeAgentName(targetNode)
		if agentName == "" {
			return false
		}
		if s.isFlowEngineDriven(parentRunID) {
			s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusDone)
		}
		// BUG-279: a target node declared lifecycle: reinvoke must reuse its
		// existing child run/provider session for the retry turn, same as the
		// forward-edge auto-advance path (flowNodeReusesChild check before
		// reinvokeExistingFlowChild) — spawning a brand new child here silently
		// dropped the coder's own working memory of the prior attempt every retry.
		if flowNodeReusesChild(targetNode) && s.reinvokeExistingFlowChild(parentRunID, targetNode.ID, prompt) {
			if s.isFlowEngineDriven(parentRunID) {
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
			s.setFlowStepStatus(ctx, parentRunID, targetNode.ID, StepStatusRunning)
			s.stampFlowNodePosture(ctx, parentRunID, targetNode)
		}
		return true

	case "failed_validation_max_retries":
		summary := "Validation failed after the maximum number of retries."
		if state.FailureSummary != nil && len(state.FailureSummary.FailureLines) > 0 {
			summary += " Last failure:\n" + strings.Join(state.FailureSummary.FailureLines, "\n")
		}
		if _, err := s.applyFlowControl(parentRunID, FlowControlInput{Status: "escalate", Summary: summary}); err != nil {
			log.Printf("[flow-executor] validate: escalate after max retries failed: %v", err)
			return false
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
// "ask_user") target — reusing the exact same finalization applyFlowControl
// already performs for review-loop's synthesis node, since "an inline node
// reached its done/escalate edge" has identical semantics regardless of
// which node emitted it.
func (s *InteractiveService) advanceToNextInlineOrDelegate(ctx context.Context, parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, fromNodeID, when, resultMessage string) bool {
	targetID, ok := edgeTargetFrom(edges, fromNodeID, when, "forward")
	if !ok {
		return false
	}
	if s.isFlowEngineDriven(parentRunID) {
		s.setFlowStepStatus(ctx, parentRunID, fromNodeID, StepStatusDone)
	}
	switch targetID {
	case "done":
		_, err := s.applyFlowControl(parentRunID, FlowControlInput{Status: "done", Summary: resultMessage})
		return err == nil
	case "ask_user":
		_, err := s.applyFlowControl(parentRunID, FlowControlInput{Status: "escalate", Summary: resultMessage})
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
		return s.runAuditNode(ctx, parentRunID, nextNode, resultMessage)
	case "command.validate":
		return s.runValidateNode(ctx, parentRunID, edges, nodes, nextNode, resultMessage)
	case "agent.delegate":
		agentName := flowNodeAgentName(nextNode)
		if agentName == "" {
			return false
		}
		agentDef, _ := resolvePackAgentDefinition(agentName)
		if _, err := s.spawnChildRun(ctx, parentRunID, SpawnAgentInput{
			Agent:            agentName,
			Prompt:           resultMessage,
			Wait:             false,
			Label:            nextNode.ID,
			AutoOrchestrate:  true,
			AgentDefOverride: agentDef,
			Model:            s.resolveFlowNodeModel(ctx, nextNode),
		}); err != nil {
			log.Printf("[flow-executor] advance: spawn %q failed: %v", nextNode.ID, err)
			return false
		}
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
// before any write/commit (Task-171's own non-write invariant — this
// function never writes a file or creates a commit), then settles the whole
// flow as done via applyFlowControl.
func (s *InteractiveService) runAuditNode(ctx context.Context, parentRunID string, node agentpack.FlowNode, resultMessage string) bool {
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
		baseSHA = rs.turnStartGitHead
	}
	s.mu.Unlock()

	// BuildAuditDraft derives SourceDocID from pkg.SourceDocIDs[0] itself
	// (flow_audit_draft.go) — no separate source needed here.
	changedFiles := changedFilesSince(workspace, baseSHA)

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

	if store := s.persistenceStore(); store != nil {
		if err := PersistAuditDraft(ctx, store, parentRunID, node.ID, draft); err != nil {
			log.Printf("[flow-executor] audit: persist draft failed: %v", err)
		}
	}
	s.flowDiagLog(parentRunID, "flow_audit_draft_built", "built audit draft",
		"node_id", node.ID, "status", draft.Status, "feature_key", draft.FeatureKey,
	)

	if s.isFlowEngineDriven(parentRunID) {
		s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusDone)
	}
	if _, err := s.applyFlowControl(parentRunID, FlowControlInput{
		Status:  "done",
		Summary: RenderAuditDraftText(draft),
		Payload: map[string]any{"auditDraft": draft},
	}); err != nil {
		log.Printf("[flow-executor] audit: flow-done settle failed: %v", err)
		return false
	}
	return true
}
