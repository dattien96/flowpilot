package runner

import (
	"context"
	"fmt"
	"log"
	"path"
	"strings"

	"flowpilot-runner/internal/agentpack"
)

// startResolvedFlow is the one place a CP-42 flowRef selection becomes a
// running flow instead of a validated-but-inert request field. It resolves
// flowRef (already validated by handleStartTurn against
// BuiltinOrchestrationOptions before this is ever called) and spawns the
// flow's entry node(s) as children of parentRunID through spawnChildRun —
// the exact same path an AI-driven spawn_agent tool call with
// autoOrchestrate=true already uses. No cohort/join/cap/transition logic is
// duplicated here: once the entry node is spawned with AutoOrchestrate=true,
// the existing explicit-mode machinery (isCoderRun, maybeAutoReinvokeHub,
// flow_control handling) drives the rest exactly as it does today.
//
// Failure is logged only. A flow-start failure must never break the run's
// own first turn, which proceeds to its own provider normally regardless of
// whether the flow's entry node could be spawned.
func (s *InteractiveService) startResolvedFlow(ctx context.Context, parentRunID, flowRef, userPrompt string) {
	s.flowDiagLog(parentRunID, "flow_start_begin", "starting resolved flow",
		"flow_ref", flowRef,
		"user_prompt_len", len(userPrompt),
	)
	s.mu.Lock()
	store := s.flowDefinitionStore
	s.mu.Unlock()

	resolver := NewFlowDefinitionResolver(store)
	record, err := resolver.ResolveFlowRef(ctx, flowRef)
	if err != nil {
		log.Printf("[flow-executor] resolve flowRef %q for run %q failed: %v", flowRef, parentRunID, err)
		s.flowDiagLog(parentRunID, "flow_start_resolve_failed", "flow resolve failed",
			"flow_ref", flowRef,
			"error", err.Error(),
		)
		return
	}
	s.flowDiagLog(parentRunID, "flow_start_resolved", "flow resolved",
		"flow_ref", record.FlowRef,
		"node_count", len(record.Definition.Nodes),
		"edge_count", len(record.Definition.Edges),
	)

	// Apply the flow's own configured Cap/ExtendBy to the run's loop state.
	// Previously this never happened: effectiveCap()/the hardcoded
	// defaultExtendBy=2 in extendCap/resumeFlowWithFeedback meant EVERY flow
	// ran with cap=3, extendBy=2 regardless of what workflows.policy_cap/
	// policy_extend_by said in Supabase — a built-in with cap=3 in its own
	// YAML happened to match that hardcoded fallback, which is exactly what
	// masked the gap. A custom/cloned flow's own configured cap now actually
	// takes effect. Falls back to the same 3/2 defaults when a flow declares
	// no policy at all (a bare custom flow's Policy is a zero-value struct).
	cap := record.Definition.Policy.Cap
	if cap <= 0 {
		cap = 3
	}
	extendBy := record.Definition.Policy.ExtendBy
	if extendBy <= 0 {
		extendBy = 2
	}
	s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Cap = cap
		st.RoundCap = cap
		st.ExtendBy = extendBy
		return st
	})

	entryNodes := entryDelegateNodes(record.Definition)
	if len(entryNodes) == 0 {
		// BUG-NOTE-CP42 #9: a flow whose entry node is an inline behavior
		// (e.g. rag-harness's "context" -> context.produce) has no
		// agent.delegate entry node at all, so it would otherwise resolve
		// successfully and then spawn nothing.
		if s.startInlineEntryChain(ctx, parentRunID, flowRef, record, userPrompt) {
			s.flowDiagLog(parentRunID, "flow_start_inline_entry", "flow started via inline entry chain",
				"flow_ref", flowRef,
			)
			return
		}
		log.Printf("[flow-executor] flow %q has no entry agent.delegate node; nothing to start", flowRef)
		s.flowDiagLog(parentRunID, "flow_start_no_entry", "flow has no entry delegate node",
			"flow_ref", flowRef,
		)
		return
	}

	// Set BEFORE spawning any child, not after: spawnChildRun starts the
	// child's own first turn in its own goroutine as soon as it returns, and
	// a fast-completing turn (a test/fake adapter, or in principle a very
	// fast real one) could otherwise race ahead of this assignment — its
	// completion handler would then see no tracked flow data and silently
	// fall back to the legacy note+reinvoke-hub path instead of auto-spawning
	// the next node deterministically.
	s.mu.Lock()
	if rs := s.runs[parentRunID]; rs != nil {
		rs.activeFlowEdges = record.Definition.Edges
		rs.activeFlowNodes = record.Definition.Nodes
	}
	s.mu.Unlock()

	// BUG-174: for a flow-engine-driven run, replace the generic catalog steps
	// with one step per flow node so the step timeline tracks the flow's real
	// topology and the executor (not the bulk PlanWorkflowProgress planner,
	// which startTurn now skips for these runs) owns every transition.
	if s.isFlowEngineDriven(parentRunID) {
		s.reseedFlowStepRuntime(parentRunID, record.Definition.Nodes)
	}

	waitNotice, _, _ := loadBuiltinPromptText("prompts/flow-start-wait.md")
	if waitNotice == "" {
		waitNotice = "[flow-engine] An agent has already been spawned to work on this request. " +
			"Do not duplicate that work yourself. Wait for its result."
	}
	for _, node := range entryNodes {
		agentName := flowNodeAgentName(node)
		if agentName == "" {
			log.Printf("[flow-executor] flow %q entry node %q has no resolvable agent; skipped", flowRef, node.ID)
			s.flowDiagLog(parentRunID, "flow_start_entry_skipped", "entry node has no resolvable agent",
				"flow_ref", flowRef,
				"node_id", node.ID,
			)
			continue
		}
		s.flowDiagLog(parentRunID, "flow_start_entry_spawn_attempt", "spawning flow entry node",
			"flow_ref", flowRef,
			"node_id", node.ID,
			"agent_name", agentName,
		)
		agentDef, _ := resolvePackAgentDefinition(agentName)
		if _, err := s.spawnChildRun(ctx, parentRunID, SpawnAgentInput{
			Agent:             agentName,
			Prompt:            userPrompt,
			Wait:              false,
			Label:             node.ID,
			AutoOrchestrate:   true,
			AgentDefOverride:  agentDef,
			ParentContextNote: waitNotice,
			Model:             s.resolveFlowNodeModel(ctx, node),
		}); err != nil {
			log.Printf("[flow-executor] spawn entry node %q (agent %q) for flow %q on run %q failed: %v",
				node.ID, agentName, flowRef, parentRunID, err)
			s.flowDiagLog(parentRunID, "flow_start_entry_spawn_failed", "entry node spawn failed",
				"flow_ref", flowRef,
				"node_id", node.ID,
				"agent_name", agentName,
				"error", err.Error(),
			)
			continue
		}
		s.flowDiagLog(parentRunID, "flow_start_notified_hub", "hub notified that flow work started",
			"flow_ref", flowRef,
			"node_id", node.ID,
		)
		s.flowDiagLog(parentRunID, "flow_start_entry_spawned", "entry node spawned",
			"flow_ref", flowRef,
			"node_id", node.ID,
			"agent_name", agentName,
		)
		if s.isFlowEngineDriven(parentRunID) {
			s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusRunning)
			s.stampFlowNodePosture(ctx, parentRunID, node)
		}
	}
}

// resolveWorkflowFlowRef bridges a Flow-Mode workflow-picker launch to the flow
// executor (BUG-174). A workflow-picker run carries a workflowID but no flowRef
// (the desktop only sends flowRef for the chat "bug" sub-mode), so its selected
// workflow — even a built-in flow mirrored into the workflows table — never
// engaged the executor and the hub did all the work inline. This resolves the
// run's workflowID against the flow definition store (GetByRef accepts the
// mirror row's UUID and normalizes it back to the canonical packId/flowId
// flowRef) and, if it resolves to a flow with a spawnable entry node, returns
// that canonical flowRef so handleStartTurn can drive it through the exact same
// startResolvedFlow path an explicit flowRef uses.
//
// Returns ("", false) for a non-first turn, a run with no workflowID, no
// definition store, a resolution failure, or a plain workflow with no
// agent.delegate entry node (and no inline entry chain) — every one a safe bail
// that leaves the run on its existing behavior rather than forcing an executor
// it has nothing to run.
func (s *InteractiveService) resolveWorkflowFlowRef(ctx context.Context, runID string) (string, bool) {
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil || rs.turnCount != 0 || strings.TrimSpace(rs.workflowID) == "" {
		s.mu.Unlock()
		return "", false
	}
	workflowID := rs.workflowID
	store := s.flowDefinitionStore
	s.mu.Unlock()

	resolver := NewFlowDefinitionResolver(store)
	record, err := resolver.ResolveFlowRef(ctx, workflowID)
	if err != nil {
		// Not a resolvable flow (a plain admin workflow, or no store): bail
		// quietly and let the run proceed on its existing path.
		return "", false
	}
	if len(entryDelegateNodes(record.Definition)) == 0 && len(entryNodesNoDeps(record.Definition)) == 0 {
		return "", false
	}
	if strings.TrimSpace(record.FlowRef) == "" {
		return "", false
	}
	return record.FlowRef, true
}

// explicitFlowRefResolves synchronously confirms an explicit chat flowRef
// (Chat Mode's Bug sub-mode picker, CP-42/Task-177) actually resolves to a
// valid stored/embedded flow definition, before handleStartTurn ever hands it
// to startTurn.
//
// BUG-261: validateChatOrchestrationSelection only checks that flowRef is a
// known option for the sub-mode (BuiltinOrchestrationOptions) — it never
// confirms the underlying stored definition is still valid. Unlike the
// sibling Flow-Mode workflow-picker path (resolveWorkflowFlowRef, above),
// which already resolves synchronously before deciding to attach a flowRef,
// the explicit chat path used to hand its raw flowRef straight to startTurn,
// which unconditionally suppresses the hub's own turn (flowStartOnly=true)
// the moment flowRef is non-empty — before startResolvedFlow's own async
// resolve even runs. A corrupted/invalid stored definition (e.g. a
// BUG-249-style manually-tampered built-in mirror row) then failed
// resolution inside that goroutine with nothing to fall back to: the hub's
// turn was already suppressed and nothing was ever spawned to reinvoke it,
// so the run got stuck at "completed" forever with no assistant reply and no
// error surfaced anywhere. Resolving here first lets handleStartTurn clear
// an unresolvable flowRef before it ever reaches startTurn, matching the
// same safe-bail contract resolveWorkflowFlowRef already provides.
func (s *InteractiveService) explicitFlowRefResolves(ctx context.Context, flowRef string) bool {
	s.mu.Lock()
	store := s.flowDefinitionStore
	s.mu.Unlock()
	_, err := NewFlowDefinitionResolver(store).ResolveFlowRef(ctx, flowRef)
	return err == nil
}

// startInlineEntryChain handles flows whose entry node is an inline behavior
// (e.g. rag-harness's "context" node, behavior context.produce) rather than a
// spawnable agent.delegate node. entryDelegateNodes alone cannot start such a
// flow at all: it would resolve successfully and then spawn nothing, with
// only a log line to show for it (BUG-NOTE-CP42 #9).
//
// Runs the single inline entry node synchronously in-process — matching
// BehaviorScopeInline's contract of "no provider call" — then follows its
// forward "done" edge to the first agent.delegate node and spawns that node,
// carrying forward the inline node's output (a FlowContextPackage, for
// context.produce) rendered into the delegate's prompt exactly as the
// pre-existing step-based Flow Mode path does (see flow_context_handoff.go).
//
// Scoped to the one shape either built-in flow actually declares: exactly one
// no-dependency entry node, itself inline, with a single forward hop to an
// agent.delegate node. Anything else (multiple entry nodes, a longer inline
// chain, no reachable delegate target) returns false so the caller logs and
// bails rather than guessing at an unsupported topology.
func (s *InteractiveService) startInlineEntryChain(ctx context.Context, parentRunID, flowRef string, record FlowDefinitionRecord, userPrompt string) bool {
	def := record.Definition
	entries := entryNodesNoDeps(def)
	if len(entries) != 1 {
		return false
	}
	entry := entries[0]

	canonical, ok := agentpack.NormalizeBehaviorID(entry.Behavior)
	if !ok {
		return false
	}
	spec, err := DefaultBehaviorRegistry().Resolve(canonical)
	if err != nil || spec.Scope != BehaviorScopeInline {
		return false
	}

	out, err := DefaultBehaviorRegistry().Dispatch(ctx, canonical, BehaviorInput{
		NodeID:        entry.ID,
		WorkflowRunID: parentRunID,
		WorkspaceCwd:  s.workspaceCwdFor(parentRunID),
		Prompt:        userPrompt,
	})
	if err != nil {
		log.Printf("[flow-executor] flow %q inline entry node %q failed: %v", flowRef, entry.ID, err)
		return false
	}

	var delegateTarget *agentpack.FlowNode
	for _, id := range forwardDoneTargets(def.Edges, entry.ID) {
		node, ok := findFlowNode(def.Nodes, id)
		if !ok {
			continue
		}
		if nodeCanonical, ok := agentpack.NormalizeBehaviorID(node.Behavior); ok && nodeCanonical == "agent.delegate" {
			n := node
			delegateTarget = &n
			break
		}
	}
	if delegateTarget == nil {
		log.Printf("[flow-executor] flow %q inline entry node %q has no reachable agent.delegate target; unsupported chain shape", flowRef, entry.ID)
		return false
	}

	agentName := flowNodeAgentName(*delegateTarget)
	if agentName == "" {
		log.Printf("[flow-executor] flow %q delegate target %q has no resolvable agent; skipped", flowRef, delegateTarget.ID)
		return false
	}

	prompt := userPrompt
	if pkg, ok := out.Payload["package"].(FlowContextPackage); ok {
		prompt = renderFlowContextPrompt(ctx, pkg, userPrompt)
	}

	// Same ordering rationale as the delegate-entry path above: track the
	// flow's topology before spawning, not after, so a fast-completing child
	// can't race ahead of it.
	s.mu.Lock()
	if rs := s.runs[parentRunID]; rs != nil {
		rs.activeFlowEdges = def.Edges
		rs.activeFlowNodes = def.Nodes
	}
	s.mu.Unlock()

	// BUG-174: same step-timeline wiring as the delegate-entry path. The inline
	// entry node has already run synchronously above, so mark it DONE and the
	// spawned delegate target RUNNING.
	if s.isFlowEngineDriven(parentRunID) {
		s.reseedFlowStepRuntime(parentRunID, def.Nodes)
		s.setFlowStepStatus(ctx, parentRunID, entry.ID, StepStatusDone)
	}

	agentDef, _ := resolvePackAgentDefinition(agentName)
	if _, err := s.spawnChildRun(ctx, parentRunID, SpawnAgentInput{
		Agent:            agentName,
		Prompt:           prompt,
		Wait:             false,
		Label:            delegateTarget.ID,
		AutoOrchestrate:  true,
		AgentDefOverride: agentDef,
		Model:            s.resolveFlowNodeModel(ctx, *delegateTarget),
	}); err != nil {
		log.Printf("[flow-executor] spawn inline-chain delegate node %q (agent %q) for flow %q on run %q failed: %v",
			delegateTarget.ID, agentName, flowRef, parentRunID, err)
		return false
	}
	if s.isFlowEngineDriven(parentRunID) {
		s.setFlowStepStatus(ctx, parentRunID, delegateTarget.ID, StepStatusRunning)
		s.stampFlowNodePosture(ctx, parentRunID, *delegateTarget)
	}

	s.notifyHubFlowStarted(parentRunID)
	return true
}

// entryNodesNoDeps returns a flow's entry nodes — those declaring no
// dependsOn — regardless of behavior, in declared order. Unlike
// entryDelegateNodes this does not filter by behavior, so it also matches an
// inline-behavior entry node (see startInlineEntryChain).
func entryNodesNoDeps(def agentpack.FlowDefinition) []agentpack.FlowNode {
	var out []agentpack.FlowNode
	for _, node := range def.Nodes {
		if len(node.DependsOn) == 0 {
			out = append(out, node)
		}
	}
	return out
}

// workspaceCwdFor returns runID's workspace directory, or "" if the run is
// unknown. A tiny locked accessor so callers outside interactive_service.go
// don't need to reach into interactiveRun's fields directly.
func (s *InteractiveService) workspaceCwdFor(runID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rs := s.runs[runID]; rs != nil {
		return rs.workspaceCwd
	}
	return ""
}

// resolveContinueBackEdgeTarget finds the back-edge a "continue" flow_control
// signal should follow (Task-180: prefer edge data over role-name matching
// for runs started from a resolved flowRef). review-loop.yaml declares
// exactly one such edge (synthesis -> coder); a flow with more than one
// matching edge returns the first declared, since nothing in the current
// live path disambiguates by which node emitted the signal.
func resolveContinueBackEdgeTarget(edges []agentpack.FlowEdge) (string, bool) {
	for _, edge := range edges {
		if strings.EqualFold(strings.TrimSpace(edge.Kind), "back") && strings.EqualFold(strings.TrimSpace(edge.When), "continue") {
			if edge.To != "" {
				return edge.To, true
			}
		}
	}
	return "", false
}

// notifyHubFlowStarted tells the hub (parent run) an agent has already been
// spawned to do the actual work, so the hub's own reasoning doesn't
// redundantly try to also write the code itself. Delivered through the same
// pending-context mechanism the "coder completed" notes use — visible the
// next time the hub's context is assembled, not necessarily before its
// current in-flight turn. A perfectly race-free "before this exact turn"
// guarantee would need deeper synchronization with startTurn's own provider
// call, which this additive change does not attempt.
func (s *InteractiveService) notifyHubFlowStarted(parentRunID string) {
	notice, ok, err := loadBuiltinPromptText("prompts/flow-start-wait.md")
	if err != nil || !ok || notice == "" {
		notice = "[flow-engine] An agent has already been spawned to work on this request. " +
			"Do not duplicate that work yourself. Wait for its result."
	}
	s.mu.Lock()
	s.appendPendingAgentContextLocked(parentRunID, notice)
	s.mu.Unlock()
}

// tryAdvanceFlowFromNode auto-spawns the next node(s) in a tracked flow's
// topology when completedNodeID finishes, instead of leaving a bare
// completion note for the hub AI to infer the next step from.
//
// Investigating a user question about how the hub could possibly know to
// spawn a reviewer cohort revealed it couldn't: the "Coder completed" note
// was generic text with no topology information, spawn_agent's tool
// description doesn't enumerate flow nodes, and no existing E2E test actually
// exercised an AI deciding to spawn the cohort (they all spawn it manually to
// test the Go-side join/cap logic in isolation). This closes that gap for
// the one transition that needs it: review-loop.yaml's "synthesis" node has
// `run: inline` — it is the hub's own reinvoke turn, not a spawned child, and
// that transition was already correctly Go-orchestrated via the existing
// cohort-join → maybeAutoReinvokeHub path. Only the coder → reviewer-cohort
// step needed a real fix.
//
// Returns false (a no-op) when the run has no tracked flow, the node has no
// outgoing forward ("done") edges, or any edge target isn't a spawnable
// agent.delegate node (e.g. a hub.inline node) — callers fall back to the
// existing note+reinvoke-hub behavior in that case, so a flow shape this
// function doesn't understand degrades to the pre-existing AI-driven
// behavior rather than silently doing nothing.
func (s *InteractiveService) tryAdvanceFlowFromNode(parentRunID, completedNodeID, resultMessage string) bool {
	// BUG-234: do not auto-advance once the loop has legitimately settled
	// (blocked/awaiting-user, done, stopped, paused). A child completion that
	// lands after the loop blocked would otherwise re-spawn the next nodes and,
	// via the cohort join, flip the hub node back to RUNNING — a runaway that
	// never lets the settled state stick (the synthesis-step-spins-forever hang).
	if !s.loopIsAdvancing(parentRunID) {
		s.flowDiagLog(parentRunID, "flow_advance_skipped_loop_blocked", "skipping auto-advance because loop is not advancing",
			"completed_node_id", completedNodeID,
		)
		return false
	}
	s.mu.Lock()
	parent := s.runs[parentRunID]
	if parent == nil || len(parent.activeFlowEdges) == 0 || len(parent.activeFlowNodes) == 0 {
		s.mu.Unlock()
		s.flowDiagLog(parentRunID, "flow_advance_skipped_no_topology", "skipping auto-advance because no tracked flow topology is available",
			"completed_node_id", completedNodeID,
		)
		return false
	}
	edges := parent.activeFlowEdges
	nodes := parent.activeFlowNodes
	s.mu.Unlock()
	round := s.agentOrchestrator.loopStateFor(parentRunID).Round

	targetIDs := forwardDoneTargets(edges, completedNodeID)
	if len(targetIDs) == 0 {
		s.flowDiagLog(parentRunID, "flow_advance_no_targets", "completed node has no forward done targets",
			"completed_node_id", completedNodeID,
		)
		return false
	}
	s.flowDiagLog(parentRunID, "flow_advance_targets_resolved", "resolved forward targets for completed node",
		"completed_node_id", completedNodeID,
		"target_ids", strings.Join(targetIDs, ","),
		"result_len", len(resultMessage),
	)
	targetNodes := make([]agentpack.FlowNode, 0, len(targetIDs))
	for _, id := range targetIDs {
		node, ok := findFlowNode(nodes, id)
		if !ok {
			s.flowDiagLog(parentRunID, "flow_advance_target_missing", "target node missing from active flow nodes",
				"completed_node_id", completedNodeID,
				"target_node_id", id,
			)
			return false
		}
		canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior)
		if !ok || canonical != "agent.delegate" {
			s.flowDiagLog(parentRunID, "flow_advance_target_not_delegate", "target node is not a spawnable delegate",
				"completed_node_id", completedNodeID,
				"target_node_id", node.ID,
				"behavior", node.Behavior,
			)
			return false
		}
		targetNodes = append(targetNodes, node)
	}

	cohortID := fmt.Sprintf("flow-auto-%s-round-%d", completedNodeID, round)
	// BUG-NOTE-CP42 #13: this prompt used to tell the spawned reviewer to
	// "report your findings via the flow's declared control tool" — i.e. call
	// submit_review_outcome/flow_control itself. turnBridge.SubmitFlowControl
	// routes a child's flow_control call straight to its parent's
	// applyFlowControl, with no cohort-join gate at all, so a reviewer that
	// literally followed that instruction could advance/complete/block the
	// whole flow round before the other cohort member(s) even finished —
	// bypassing the join barrier maybeAutoReinvokeHub depends on. Every other
	// reviewer-spawn path in this codebase (see the manually-spawned E2E
	// tests) just asks the reviewer to review and report findings as its own
	// final message; only the hub's own synthesis turn (after the join
	// completes) is supposed to call the control tool. Match that convention.
	prompt := fmt.Sprintf(
		"[flow-engine] Review this result from node %q and report your findings (approve or request changes, with specifics) as your final message:\n\n%s",
		completedNodeID, truncateDisplayField(resultMessage, 2000),
	)
	// BUG-174: the node that just completed is DONE on the step timeline; its
	// forward targets become RUNNING as they are spawned below. Gated to
	// flow-engine-driven runs so the AI-driven spawn_agent path is untouched.
	flowDriven := s.isFlowEngineDriven(parentRunID)
	if flowDriven {
		s.setFlowStepStatus(context.Background(), parentRunID, completedNodeID, StepStatusDone)
	}

	spawnedAny := false
	for i, node := range targetNodes {
		// BUG-234: re-check the loop status before EACH spawn, not just at entry.
		// The loop can transition to blocked/done between the entry gate and here
		// (e.g. a concurrent hub synthesis turn escalates while this advance
		// goroutine is resolving targets / building the prompt). Without this
		// re-check, a completion that entered while "running" would still spawn the
		// next round's reviewers into an already-settled loop — the runaway that
		// kept the synthesis step flapping RUNNING after an escalate.
		if !s.loopIsAdvancing(parentRunID) {
			s.flowDiagLog(parentRunID, "flow_advance_aborted_mid_spawn", "stopped auto-advance because loop settled during spawn sequence",
				"completed_node_id", completedNodeID,
				"target_node_id", node.ID,
			)
			break
		}
		agentName := flowNodeAgentName(node)
		if agentName == "" {
			log.Printf("[flow-executor] auto-advance: node %q has no resolvable agent; skipped", node.ID)
			s.flowDiagLog(parentRunID, "flow_advance_target_skipped", "target node has no resolvable agent",
				"completed_node_id", completedNodeID,
				"target_node_id", node.ID,
			)
			continue
		}
		if flowNodeReusesChild(node) {
			if reused := s.reinvokeExistingFlowChild(parentRunID, node.ID, prompt); reused {
				spawnedAny = true
				s.flowDiagLog(parentRunID, "flow_advance_reinvoked_existing", "reinvoked existing target node child",
					"completed_node_id", completedNodeID,
					"target_node_id", node.ID,
					"agent_name", agentName,
				)
				if flowDriven {
					s.setFlowStepStatus(context.Background(), parentRunID, node.ID, StepStatusRunning)
					s.stampFlowNodePosture(context.Background(), parentRunID, node)
				}
				continue
			}
		}
		s.flowDiagLog(parentRunID, "flow_advance_spawn_attempt", "spawning auto-advanced target node",
			"completed_node_id", completedNodeID,
			"target_node_id", node.ID,
			"agent_name", agentName,
			"cohort_id", cohortID,
			"cohort_size", len(targetNodes),
		)
		agentDef, _ := resolvePackAgentDefinition(agentName)
		if _, err := s.spawnChildRun(context.Background(), parentRunID, SpawnAgentInput{
			Agent:            agentName,
			Prompt:           prompt,
			Wait:             false,
			Label:            node.ID,
			FlowCohortID:     cohortID,
			CohortSize:       len(targetNodes),
			AutoOrchestrate:  i == 0,
			AgentDefOverride: agentDef,
			Model:            s.resolveFlowNodeModel(context.Background(), node),
		}); err != nil {
			log.Printf("[flow-executor] auto-advance: spawn node %q (agent %q) failed: %v", node.ID, agentName, err)
			s.flowDiagLog(parentRunID, "flow_advance_spawn_failed", "auto-advanced target node spawn failed",
				"completed_node_id", completedNodeID,
				"target_node_id", node.ID,
				"agent_name", agentName,
				"cohort_id", cohortID,
				"error", err.Error(),
			)
			continue
		}
		spawnedAny = true
		s.flowDiagLog(parentRunID, "flow_advance_spawned", "auto-advanced target node spawned",
			"completed_node_id", completedNodeID,
			"target_node_id", node.ID,
			"agent_name", agentName,
			"cohort_id", cohortID,
		)
		if flowDriven {
			s.setFlowStepStatus(context.Background(), parentRunID, node.ID, StepStatusRunning)
			s.stampFlowNodePosture(context.Background(), parentRunID, node)
		}
	}
	s.flowDiagLog(parentRunID, "flow_advance_complete", "auto-advance processing finished",
		"completed_node_id", completedNodeID,
		"spawned_any", spawnedAny,
		"target_count", len(targetNodes),
	)
	return spawnedAny
}

// forwardDoneTargets returns the (deduplicated) targets of fromNodeID's
// forward edges gated on "done" — the condition a normal successful turn
// completion represents.
func forwardDoneTargets(edges []agentpack.FlowEdge, fromNodeID string) []string {
	var out []string
	seen := make(map[string]bool, len(edges))
	for _, edge := range edges {
		if edge.From != fromNodeID {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(edge.Kind), "forward") || !strings.EqualFold(strings.TrimSpace(edge.When), "done") {
			continue
		}
		if edge.To == "" || seen[edge.To] {
			continue
		}
		seen[edge.To] = true
		out = append(out, edge.To)
	}
	return out
}

func findFlowNode(nodes []agentpack.FlowNode, id string) (agentpack.FlowNode, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return agentpack.FlowNode{}, false
}

func flowNodeLifecycle(node agentpack.FlowNode) string {
	switch strings.ToLower(strings.TrimSpace(node.Lifecycle)) {
	case "spawn", "once":
		return strings.ToLower(strings.TrimSpace(node.Lifecycle))
	default:
		return "reinvoke"
	}
}

func flowNodeReusesChild(node agentpack.FlowNode) bool {
	return flowNodeLifecycle(node) == "reinvoke"
}

func (s *InteractiveService) reinvokeExistingFlowChild(parentRunID, nodeID, prompt string) bool {
	if strings.TrimSpace(nodeID) == "" {
		return false
	}
	return s.reinvokeMatchingFlowChild(parentRunID, prompt, func(child *interactiveRun) bool {
		return child.label == nodeID
	})
}

// reinvokeMatchingFlowChild is the single implementation behind every
// "reactivate an existing child run for another turn" path: find the first
// non-terminal-turn child of parentRunID satisfying match, transition it back
// to running, and notify the desktop. BUG-242: this used to be duplicated —
// reinvokeExistingFlowChild (the forward-edge reuse-lifecycle path) had the
// BUG-Rnd2 fixes below, but maybeReinvokeCoderForContinue's back-edge
// "continue" reinvoke (the actual path a review-loop round-2+ coder re-entry
// takes) had its own older, un-fixed copy — so the coder reappearing for round
// 2 stayed stuck in the desktop's "Recently closed" section with no new
// main-chat card, while a forward-spawned reviewer behaved correctly.
//
//   - BUG-Rnd2 (Bug B): increments activationSeq so the desktop's monotonic
//     terminal-status guard (mergeAgentRunsById) recognizes a genuine
//     completed→running transition instead of discarding it as a stale
//     snapshot — without this the run never leaves "completed" client-side and
//     is miscategorized as closed regardless of what the backend just did.
//   - BUG-Rnd2 (Bug C): emits EventAgentSpawnedByUser so the parent thread
//     renders a new agent card for this turn, matching the spawnChildRun path
//     (idempotent by event id, so replay never duplicates the row).
//
// Returns true if a matching child was found (whether or not a new turn was
// actually scheduled — a match with a turn already in flight still counts as
// "handled": its own completion will drive the next step).
func (s *InteractiveService) reinvokeMatchingFlowChild(parentRunID, prompt string, match func(*interactiveRun) bool) bool {
	if strings.TrimSpace(parentRunID) == "" || strings.TrimSpace(prompt) == "" || match == nil {
		return false
	}
	s.mu.Lock()
	var runID, stepID string
	var snap AgentGraphSnapshot
	var agentName string
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		child := s.runs[childID]
		if child == nil || !match(child) {
			continue
		}
		if child.turnInFlight {
			s.mu.Unlock()
			return true
		}
		child.activationSeq++
		child.status = RunStatusRunning
		child.agentStatus = string(RunStatusRunning)
		s.agentOrchestrator.upsertSummary(parentRunID, AgentRunSummary{
			RunID:         child.id,
			AgentName:     child.agentName,
			Label:         child.label,
			Role:          child.role,
			Status:        child.status,
			ParentRunID:   child.parentRunID,
			CreatedAt:     child.createdAt,
			DependsOn:     append([]string(nil), child.dependsOn...),
			AgentStatus:   child.agentStatus,
			ProviderKey:   string(child.providerKey),
			ModelName:     child.modelName,
			WaitForResult: child.waitForResult,
			ActivationSeq: child.activationSeq,
		})
		snap = s.agentOrchestrator.graphSnapshot(parentRunID)
		runID = child.id
		stepID = child.stepID
		agentName = child.agentName
		break
	}
	s.mu.Unlock()
	if runID == "" || stepID == "" {
		return false
	}
	s.emitAgentGraph(parentRunID, snap)
	s.emitOnParentRun(parentRunID, ProviderEvent{
		Type:       EventAgentSpawnedByUser,
		AgentName:  agentName,
		ChildRunID: runID,
	})
	s.scheduleChildTurn(runID, stepID, prompt)
	return true
}

// entryDelegateNodes returns a flow's entry nodes: agent.delegate-behavior
// nodes declaring no dependsOn, in declared order. These are the nodes a flow
// run starts from; every other node is reached through edges once its
// dependencies complete.
func entryDelegateNodes(def agentpack.FlowDefinition) []agentpack.FlowNode {
	var out []agentpack.FlowNode
	for _, node := range def.Nodes {
		if len(node.DependsOn) > 0 {
			continue
		}
		canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior)
		if !ok || canonical != "agent.delegate" {
			continue
		}
		out = append(out, node)
	}
	return out
}

// flowNodeAgentName derives the agent catalog name spawnChildRun expects from
// a node's agent file reference (e.g. "agents/coder.md" -> "coder"). This
// matches the default-name derivation agentpack.parseAgentSpec applies when
// an agent file has no explicit `name:` frontmatter, and pack agent file
// basenames are the catalog names Task-174 wired in (see
// agent_catalog_pack.go / flow-pack/agents/*.md).
func flowNodeAgentName(node agentpack.FlowNode) string {
	return agentNameFromRef(node.Agent)
}

// agentNameFromRef derives the agent catalog name from an agent file
// reference (e.g. "agents/coder.md" -> "coder"), the same basename
// derivation flowNodeAgentName applies to a live agentpack.FlowNode. Exposed
// standalone so callers holding only a persisted agent_ref string (e.g. the
// entry-step row read back from Supabase in createRun) can resolve the same
// role name without a FlowNode value.
func agentNameFromRef(agentRef string) string {
	agent := strings.TrimSpace(agentRef)
	if agent == "" {
		return ""
	}
	base := path.Base(agent)
	return strings.TrimSuffix(base, path.Ext(base))
}

// resolveFlowNodeModel resolves an agent.delegate flow node's OWN configured
// model, so it can run on a different model/provider than the flow's own
// resolved model instead of always inheriting it (BUG-228). After BUG-236,
// built-in mirror sync can create one node-specific step_definitions row per
// flow node, so prefer an exact node_id match. Fall back to the older
// purpose-named role row "flow-agent-delegate-<role>" — the same rows the
// manual workflow builder exposes as "Flow: Coder" / "Flow: Reviewer"
// (BUG-161/CA-230). This preserves existing reviewer/coder model settings
// while allowing the new node-specific definition model to take over.
//
// Returns "" when the role has no such row or it has no model configured, so
// callers pass an empty SpawnAgentInput.Model and spawnChildRun falls back to
// its pre-existing inherit-from-parent behavior unchanged. An inline
// (hub.inline / run: inline) node never reaches this: it executes as the
// parent run's own turn and never calls spawnChildRun, so it always uses the
// flow's already-resolved model — no lookup needed.
func (s *InteractiveService) resolveFlowNodeModel(ctx context.Context, node agentpack.FlowNode) string {
	if canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior); ok && canonical != "agent.delegate" {
		return ""
	}
	return s.resolveConfiguredModelForAgent(ctx, node.ID, flowNodeAgentName(node))
}

// resolveConfiguredModelForAgent is the shared two-tier lookup behind
// resolveFlowNodeModel: prefer the node's OWN per-node step_definitions row
// (matched by node_id), else fall back to the purpose-named role row
// "flow-agent-delegate-<agent>" (the same row WorkflowsSettings.tsx's manual
// builder writes to). Also used by createRun's entry-step model resolution.
//
// BUG-241: the node_id match MUST ignore the generic dispatch-category seed
// rows ("flow-agent-delegate", "flow-agent-delegate-coder", "flow-hub-inline",
// …). Those rows are supposed to carry no node identity (FLOW=workflow,
// NODE=step per BUG-236), but an early BUG-236 draft migration copied node_id
// onto them (e.g. flow-agent-delegate-reviewer.node_id="reviewer_correctness")
// and the BUG-239 repair never cleared it. Because ListSteps orders by
// name.asc, those "Flow: …"-named generic rows sort BEFORE the real per-node
// "<pack>: …" rows and were being returned first — so two graph nodes that
// share an agent file (reviewer_correctness / reviewer_security) resolved to
// DIFFERENT rows (one aliased onto the single contaminated generic row, the
// other fell through to its own per-node row), and the run's own entry model
// (main) disagreed with the coder child for the same node. Skipping generic
// rows here makes every path resolve the same authoritative per-node row.
//
// The prefix test is safe: per-node mirror step_types come from
// flowNodeStepType -> sanitizeStepType, which lowercases and replaces every
// non-alphanumeric rune (including "-") with "_", so a real per-node step_type
// can never begin with the literal "flow-" that the hand-seeded generic
// dispatch rows use. The role fallback below still matches those generic rows
// deliberately, by exact step_type — that path is unaffected.
func (s *InteractiveService) resolveConfiguredModelForAgent(ctx context.Context, nodeID, agentName string) string {
	if agentName == "" {
		return ""
	}
	s.mu.Lock()
	catalog := s.catalog
	s.mu.Unlock()
	if catalog == nil {
		return ""
	}
	steps, err := catalog.ListSteps(ctx)
	if err != nil {
		return ""
	}
	if strings.TrimSpace(nodeID) != "" {
		for _, step := range steps {
			if isGenericFlowDispatchStepType(step.ID) {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(step.NodeID), strings.TrimSpace(nodeID)) && strings.TrimSpace(step.Model) != "" {
				return strings.TrimSpace(step.Model)
			}
		}
	}
	stepType := "flow-agent-delegate-" + strings.ToLower(agentName)
	for _, step := range steps {
		if strings.EqualFold(step.ID, stepType) {
			return strings.TrimSpace(step.Model)
		}
	}
	return ""
}

// isGenericFlowDispatchStepType reports whether stepType is one of the
// hand-seeded generic flow dispatch categories ("flow-agent-delegate",
// "flow-hub-inline", "flow-agent-delegate-coder", …). These are FK
// placeholders, not real node definitions, and must never win a node_id match
// (BUG-241). Real per-node mirror step_types never begin with the literal
// "flow-" prefix because sanitizeStepType rewrites "-" to "_".
func isGenericFlowDispatchStepType(stepType string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(stepType)), "flow-")
}

// resolveFlowNodeProviderModel resolves node's OWN actually-effective
// provider/model: its role's step_definitions row when one resolves, else the
// run's own baseline (the same fallback spawnChildRun applies once the node
// is actually spawned) — so callers get the node's true eventual posture
// whether or not it has been spawned yet.
func (s *InteractiveService) resolveFlowNodeProviderModel(ctx context.Context, parentRunID string, node agentpack.FlowNode) (provider, model string) {
	model = s.resolveFlowNodeModel(ctx, node)
	if model != "" {
		if pk, ok := providerKeyFromModel(model); ok {
			provider = string(pk)
		}
	}
	if model == "" || provider == "" {
		s.mu.Lock()
		if parent := s.runs[parentRunID]; parent != nil {
			if model == "" {
				model = parent.modelName
			}
			if provider == "" {
				provider = string(parent.providerKey)
			}
		}
		s.mu.Unlock()
	}
	return provider, model
}

// stampFlowNodePosture patches node's resolved provider/model onto its
// step-timeline row via setFlowStepPosture, so the desktop step list shows
// what that node really ran on instead of always mirroring the run's single
// baseline posture (BUG-228 display follow-up). Best-effort: never blocks or
// fails the spawn it accompanies.
func (s *InteractiveService) stampFlowNodePosture(ctx context.Context, parentRunID string, node agentpack.FlowNode) {
	provider, model := s.resolveFlowNodeProviderModel(ctx, parentRunID, node)
	s.setFlowStepPosture(ctx, parentRunID, node.ID, provider, model)
}

// resolvePackAgentDefinition looks up agentName directly against the
// embedded pack's own bundled agent definitions, bypassing
// AgentCatalog.listAgents' project-local > provider-home > built-in
// precedence entirely.
//
// BUG-NOTE-CP42 #23: a flow node's `agent: agents/coder.md` reference reduces
// to the bare name "coder" via flowNodeAgentName, then spawnChildRun resolved
// that name through the general-purpose catalog — which prefers a
// project-local .claude/agents/coder.md or .codex/agents/coder.md over the
// pack's own bundled agent of the same name. Any repo happening to define its
// own "coder"/"reviewer" agent for an unrelated purpose would silently swap
// out the built-in Review Loop's prompts, breaking the "pack-driven, not
// hardcoded" guarantee CP-42/CA-158 established. Every flow_executor.go spawn
// call site passes its result as SpawnAgentInput.AgentDefOverride so a
// pack-declared node's agent identity is never subject to catalog shadowing.
//
// Returns (nil, false) if the pack can't be loaded or has no agent by that
// name — callers fall back to the normal catalog-based resolution in that
// case (the same safe-bail convention this file uses elsewhere), so a
// resolution failure here degrades to the pre-existing behavior rather than
// blocking the spawn outright.
func resolvePackAgentDefinition(agentName string) (*AgentDefinition, bool) {
	agentName = strings.TrimSpace(agentName)
	if agentName == "" {
		return nil, false
	}
	defs, err := loadBuiltinAgentDefinitionsFromPack()
	if err != nil {
		return nil, false
	}
	for i := range defs {
		if strings.EqualFold(defs[i].Name, agentName) {
			def := defs[i]
			return &def, true
		}
	}
	return nil, false
}
