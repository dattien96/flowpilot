package runner

import (
	"context"
	"fmt"
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
		return s.runAuditNode(ctx, parentRunID, edges, nodes, nextNode, resultMessage)
	case "command.validate":
		return s.runValidateNode(ctx, parentRunID, edges, nodes, nextNode, resultMessage)
	case "telegram.notify":
		return s.runTelegramNotifyNode(ctx, parentRunID, edges, nodes, nextNode, resultMessage)
	case "hub.notify":
		s.dispatchHubNotifyNode(parentRunID, nextNode)
		return true
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
func (s *InteractiveService) runAuditNode(ctx context.Context, parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, node agentpack.FlowNode, resultMessage string) bool {
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
	// General post-node chaining: if this audit node's forward "done" edge
	// targets a real successor node (not the terminal "done"/"ask_user"),
	// dispatch it instead of settling the whole flow here — this is what lets a
	// user append e.g. a telegram.notify node after audit. rag-harness's
	// built-in "audit --done--> done" resolves to the terminal and falls
	// through to the unchanged settle below.
	if target, ok := edgeTargetFrom(edges, node.ID, "done", "forward"); ok && target != "done" && target != "ask_user" {
		return s.advanceToNextInlineOrDelegate(ctx, parentRunID, edges, nodes, node.ID, "done", RenderAuditDraftText(draft))
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

// resolveTelegramMessage picks the text a telegram.notify node sends for one
// bound target: the artifact's configured messageTemplate verbatim when set,
// otherwise a short default summarizing the prior node's result. v1 keeps this
// deterministic and does not substitute template variables (e.g. {{status}}) —
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
// — which resolves the connected bot credential under the runner's own HOME,
// honors the integration's auto-approve gate, and verifies a real message_id.
// This mirrors runValidateNode/runAuditNode: a deterministic Go inline node
// reached mid-flow that then follows its forward "done" edge. Because Go both
// performs and verifies the send, no flow-gate write-contract (r-artifact-
// telegram-sent) is needed for this path — that gate covers the AI-driven
// agent.delegate + MCP send, not this deterministic one.
func (s *InteractiveService) runTelegramNotifyNode(ctx context.Context, parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, node agentpack.FlowNode, resultMessage string) bool {
	targets := requiredTelegramOutputTargets(node)
	if len(targets) == 0 {
		// No telegram OUTPUT binding → nothing to send. Pass through on the
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
			// out, so escalate to ask_user rather than settling the flow done —
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
// one more turn to compose and send the notification itself — no child spawn,
// no Go-only fixed text — so it can freely fill a telegram.v1 OUTPUT artifact's
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
	prompt := fmt.Sprintf("[flow-engine] This flow has reached its %q step, which finishes as part of this same turn — no separate agent is spawned for it.", node.ID)
	prompt = appendTelegramOutputPrompt(prompt, node)
	// BUG-286 follow-up: appendTelegramOutputPrompt shows the Message Template
	// verbatim but never says what to DO with it — a receiving model can (and
	// was observed to) just echo the template's own placeholder text back
	// unfilled. This turn already has the coder/reviewer/synthesis results in
	// its own visible context (it is the SAME hub session, not a fresh one —
	// that is the whole point of hub.notify over agent.delegate), so it is the
	// only node type that CAN legitimately fill a template from what actually
	// happened, rather than fabricating detail it never saw.
	prompt += "\n\nIf a message template above contains placeholders (e.g. {{status}}) or generic section labels, " +
		"replace them with the ACTUAL outcome of this run — what was worked on, what changed (or that nothing needed " +
		"to change), and the current state (tests, verdict, anything still open) — using the real results already in " +
		"this conversation. Do not send the template's placeholder text unfilled, and do not invent detail this " +
		"conversation does not actually show."
	// BUG-287: the ONLY flow-control tool actually exposed to this session for
	// every flow that can reach a hub.notify node today (review-loop.yaml /
	// context-coding-review-synthesis.yaml, both declaring
	// tools/submit-review-outcome.yaml — the sole path into hub.notify, since
	// it requires being chained after a hub.inline node) is
	// `submit_review_outcome`, whose `status` enum is strictly
	// `approved | changes_requested | blocked` (claude_mcp_server.go
	// sharedReviewOutcomeSchema) — there is no literal "done" value, and the
	// tool's own description explicitly says "This is the only flow-control
	// tool — do not use flow_control directly." Instructing the model to call
	// "the flow's control tool with status=\"done\"" asked it to do something
	// its actual tool schema cannot express; observed live, the model then
	// re-submitted an unrelated earlier verdict (status=blocked, stale
	// feedback text) instead, re-escalating a flow that had already sent its
	// notification. `approved` is the one enum value that maps to `done`
	// (statusMap in submit-review-outcome.yaml) and is named explicitly here
	// so the model has an actual, callable instruction — with a clarifying
	// note that it is a formality, not a real code-review judgment, so a
	// notify-only turn does not feel pressured to relate it to reviewing code.
	// Scoped fix, not fully generic: a hypothetical future flow using a
	// DIFFERENT declared control-tool name/schema for its hub.inline node
	// would need this resolved dynamically from that flow's own tool
	// declaration rather than hardcoded here — out of scope until such a flow
	// exists.
	prompt += "\n\nOnce that is done (or if this step has nothing to send), call `submit_review_outcome` with " +
		"status=\"approved\" to finish this step. This is a technical formality that lets the flow advance past this " +
		"step — it does not mean you are approving or judging a code change."
	return prompt
}
