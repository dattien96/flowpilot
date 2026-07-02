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
	s.mu.Lock()
	store := s.flowDefinitionStore
	s.mu.Unlock()

	resolver := NewFlowDefinitionResolver(store)
	record, err := resolver.ResolveFlowRef(ctx, flowRef)
	if err != nil {
		log.Printf("[flow-executor] resolve flowRef %q for run %q failed: %v", flowRef, parentRunID, err)
		return
	}

	entryNodes := entryDelegateNodes(record.Definition)
	if len(entryNodes) == 0 {
		// BUG-NOTE-CP42 #9: a flow whose entry node is an inline behavior
		// (e.g. rag-harness's "context" -> context.produce) has no
		// agent.delegate entry node at all, so it would otherwise resolve
		// successfully and then spawn nothing.
		if s.startInlineEntryChain(ctx, parentRunID, flowRef, record, userPrompt) {
			return
		}
		log.Printf("[flow-executor] flow %q has no entry agent.delegate node; nothing to start", flowRef)
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

	spawnedAny := false
	for _, node := range entryNodes {
		agentName := flowNodeAgentName(node)
		if agentName == "" {
			log.Printf("[flow-executor] flow %q entry node %q has no resolvable agent; skipped", flowRef, node.ID)
			continue
		}
		agentDef, _ := resolvePackAgentDefinition(agentName)
		if _, err := s.spawnChildRun(ctx, parentRunID, SpawnAgentInput{
			Agent:            agentName,
			Prompt:           userPrompt,
			Wait:             false,
			Label:            node.ID,
			AutoOrchestrate:  true,
			AgentDefOverride: agentDef,
		}); err != nil {
			log.Printf("[flow-executor] spawn entry node %q (agent %q) for flow %q on run %q failed: %v",
				node.ID, agentName, flowRef, parentRunID, err)
			continue
		}
		spawnedAny = true
		if s.isFlowEngineDriven(parentRunID) {
			s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusRunning)
		}
	}

	if spawnedAny {
		s.notifyHubFlowStarted(parentRunID)
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
	}); err != nil {
		log.Printf("[flow-executor] spawn inline-chain delegate node %q (agent %q) for flow %q on run %q failed: %v",
			delegateTarget.ID, agentName, flowRef, parentRunID, err)
		return false
	}
	if s.isFlowEngineDriven(parentRunID) {
		s.setFlowStepStatus(ctx, parentRunID, delegateTarget.ID, StepStatusRunning)
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
	s.mu.Lock()
	parent := s.runs[parentRunID]
	if parent == nil || len(parent.activeFlowEdges) == 0 || len(parent.activeFlowNodes) == 0 {
		s.mu.Unlock()
		return false
	}
	edges := parent.activeFlowEdges
	nodes := parent.activeFlowNodes
	s.mu.Unlock()
	round := s.agentOrchestrator.loopStateFor(parentRunID).Round

	targetIDs := forwardDoneTargets(edges, completedNodeID)
	if len(targetIDs) == 0 {
		return false
	}
	targetNodes := make([]agentpack.FlowNode, 0, len(targetIDs))
	for _, id := range targetIDs {
		node, ok := findFlowNode(nodes, id)
		if !ok {
			return false
		}
		canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior)
		if !ok || canonical != "agent.delegate" {
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
		agentName := flowNodeAgentName(node)
		if agentName == "" {
			log.Printf("[flow-executor] auto-advance: node %q has no resolvable agent; skipped", node.ID)
			continue
		}
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
		}); err != nil {
			log.Printf("[flow-executor] auto-advance: spawn node %q (agent %q) failed: %v", node.ID, agentName, err)
			continue
		}
		spawnedAny = true
		if flowDriven {
			s.setFlowStepStatus(context.Background(), parentRunID, node.ID, StepStatusRunning)
		}
	}
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
	agent := strings.TrimSpace(node.Agent)
	if agent == "" {
		return ""
	}
	base := path.Base(agent)
	return strings.TrimSuffix(base, path.Ext(base))
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
