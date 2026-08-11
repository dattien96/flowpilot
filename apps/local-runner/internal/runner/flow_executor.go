package runner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path"
	"strings"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// startResolvedFlow is the one place a CP-42 flowRef selection becomes a
// running flow instead of a validated-but-inert request field. It resolves
// flowRef (already validated by handleStartTurn against
// BuiltinOrchestrationOptions before this is ever called) and spawns the
// flow's entry node(s) as children of parentRunID through spawnChildRun â€”
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
	// policy_extend_by said in Supabase â€” a built-in with cap=3 in its own
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
	// Task-241 T-6: stall timeout from pack policy; default 10 minutes when 0.
	stallTimeout := 10 * time.Minute
	if record.Definition.Policy.StallTimeoutSec > 0 {
		stallTimeout = time.Duration(record.Definition.Policy.StallTimeoutSec) * time.Second
	}
	s.mu.Lock()
	if rs := s.runs[parentRunID]; rs != nil {
		rs.stallTimeout = stallTimeout
		// Task-242 tier-3: capture workspace HEAD once for aggregate audit diff.
		if head, err := captureGitHead(rs.workspaceCwd); err == nil {
			rs.flowStartGitHead = head
		}
		// CP-55 P-3: capture the pre-existing dirty-file fingerprint once, so
		// runContractFreezeNode can later tell "already dirty before this flow
		// started" apart from "the planner just touched this."
		rs.flowStartWorktreeFingerprint = baselineWorktreeFingerprint(rs.workspaceCwd)
	}
	s.mu.Unlock()
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
	// fast real one) could otherwise race ahead of this assignment â€” its
	// completion handler would then see no tracked flow data and silently
	// fall back to the legacy note+reinvoke-hub path instead of auto-spawning
	// the next node deterministically.
	s.mu.Lock()
	if rs := s.runs[parentRunID]; rs != nil {
		rs.activeFlowEdges = record.Definition.Edges
		rs.activeFlowNodes = record.Definition.Nodes
		rs.activeFlowAcceptanceNodes = append([]string(nil), record.Definition.AcceptanceNodes...)
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
	// H-B: track entry spawn outcomes so an all-fail start does not leave the
	// hub "active" with zero children (run-1618 class, but before any child run).
	var (
		entrySpawned   int
		failedLabels   []string
		lastSpawnErr   string
		skippedNoAgent int
	)
	for _, node := range entryNodes {
		agentName := flowNodeAgentName(node)
		if agentName == "" {
			log.Printf("[flow-executor] flow %q entry node %q has no resolvable agent; skipped", flowRef, node.ID)
			s.flowDiagLog(parentRunID, "flow_start_entry_skipped", "entry node has no resolvable agent",
				"flow_ref", flowRef,
				"node_id", node.ID,
			)
			skippedNoAgent++
			failedLabels = append(failedLabels, node.ID)
			if s.isFlowEngineDriven(parentRunID) {
				s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusFailed)
			}
			continue
		}
		s.flowDiagLog(parentRunID, "flow_start_entry_spawn_attempt", "spawning flow entry node",
			"flow_ref", flowRef,
			"node_id", node.ID,
			"agent_name", agentName,
		)
		agentDef, _ := resolvePackAgentDefinition(agentName)
		entryPrompt := composeFlowNodeAgentPrompt(s.workspaceCwdFor(parentRunID), userPrompt, node)
		entryPrompt = appendChangeContractIfAnyWithSecret(s.workspaceCwdFor(parentRunID), parentRunID, entryPrompt, s.markerSecret)
		if _, err := s.spawnChildRun(ctx, parentRunID, SpawnAgentInput{
			Agent:             agentName,
			Prompt:            entryPrompt,
			Wait:              false,
			Label:             node.ID,
			AutoOrchestrate:   true,
			AgentDefOverride:  agentDef,
			ParentContextNote: waitNotice,
			Model:             s.resolveFlowNodeModel(ctx, parentRunID, node),
		}); err != nil {
			log.Printf("[flow-executor] spawn entry node %q (agent %q) for flow %q on run %q failed: %v",
				node.ID, agentName, flowRef, parentRunID, err)
			s.flowDiagLog(parentRunID, "flow_start_entry_spawn_failed", "entry node spawn failed",
				"flow_ref", flowRef,
				"node_id", node.ID,
				"agent_name", agentName,
				"error", err.Error(),
			)
			failedLabels = append(failedLabels, node.ID)
			lastSpawnErr = err.Error()
			if s.isFlowEngineDriven(parentRunID) {
				s.setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusFailed)
			}
			continue
		}
		entrySpawned++
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
	if entrySpawned == 0 && len(entryNodes) > 0 {
		if lastSpawnErr == "" {
			if skippedNoAgent > 0 {
				lastSpawnErr = "entry node(s) had no resolvable agent"
			} else {
				lastSpawnErr = "entry spawn produced no child"
			}
		}
		s.notifyHubOfFlowEntrySpawnFailure(parentRunID, flowRef, failedLabels, lastSpawnErr)
	}
}

// resolveWorkflowFlowRef bridges a Flow-Mode workflow-picker launch to the flow
// executor (BUG-174). A workflow-picker run carries a workflowID but no flowRef
// (the desktop only sends flowRef for the chat "bug" sub-mode), so its selected
// workflow â€” even a built-in flow mirrored into the workflows table â€” never
// engaged the executor and the hub did all the work inline. This resolves the
// run's workflowID against the flow definition store (GetByRef accepts the
// mirror row's UUID and normalizes it back to the canonical packId/flowId
// flowRef) and, if it resolves to a flow with a spawnable entry node, returns
// that canonical flowRef so handleStartTurn can drive it through the exact same
// startResolvedFlow path an explicit flowRef uses.
//
// Returns ("", false) for a non-first turn, a run with no workflowID, no
// definition store, a resolution failure, or a plain workflow with no
// agent.delegate entry node (and no inline entry chain) â€” every one a safe bail
// that leaves the run on its existing behavior rather than forcing an executor
// it has nothing to run.
func (s *InteractiveService) resolveWorkflowFlowRef(ctx context.Context, runID string) (string, bool) {
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		log.Printf("[flow-ref-resolve] run %q: no such run", runID)
		return "", false
	}
	if rs.turnCount != 0 {
		turnCount := rs.turnCount
		s.mu.Unlock()
		log.Printf("[flow-ref-resolve] run %q: bailing, turnCount=%d (only the first turn resolves workflowId->flowRef)", runID, turnCount)
		return "", false
	}
	// BUG-315: a Drive-restored run keeps its workflowID, so on the first
	// post-restore turn (turnCount==0 when the manifest predates the TurnCount
	// carry) this would otherwise re-resolve the flowRef and let startTurn
	// re-spawn the whole flow. A restored run is a continuation, never a genuine
	// first-turn flow start -- bail so its follow-up reaches the hub instead.
	if strings.TrimSpace(rs.restoredFrom) != "" {
		restoredFrom := rs.restoredFrom
		s.mu.Unlock()
		log.Printf("[flow-ref-resolve] run %q: bailing, restored run (restoredFrom=%q) never re-starts its flow on a follow-up", runID, restoredFrom)
		return "", false
	}
	if strings.TrimSpace(rs.workflowID) == "" {
		s.mu.Unlock()
		log.Printf("[flow-ref-resolve] run %q: bailing, no workflowID set on this run", runID)
		return "", false
	}
	workflowID := rs.workflowID
	store := s.flowDefinitionStore
	s.mu.Unlock()

	resolver := NewFlowDefinitionResolver(store)
	record, err := resolver.ResolveFlowRef(ctx, workflowID)
	if err != nil {
		// BUG-270: a workflowID that resolves to an actual flow definition
		// which then fails validation is a genuine data problem, not "this
		// isn't a flow" â€” record it so handleStartTurn can surface it to the
		// user instead of silently falling through to a normal chat turn.
		var invalidErr *ErrFlowDefinitionInvalid
		if errors.As(err, &invalidErr) {
			s.mu.Lock()
			if rs := s.runs[runID]; rs != nil {
				rs.pendingFlowRefInvalidErr = err
			}
			s.mu.Unlock()
			log.Printf("[flow-ref-resolve] run %q: workflowID %q resolved to flow %q but failed validation: %v", runID, workflowID, invalidErr.FlowRef, err)
			return "", false
		}
		// Not a resolvable flow (a plain admin workflow, or no store): bail
		// quietly and let the run proceed on its existing path.
		log.Printf("[flow-ref-resolve] run %q: workflowID %q did not resolve to a flow definition: %v", runID, workflowID, err)
		return "", false
	}
	if len(entryDelegateNodes(record.Definition)) == 0 && len(entryNodesNoDeps(record.Definition)) == 0 {
		log.Printf("[flow-ref-resolve] run %q: workflowID %q resolved to flow %q but it has no spawnable entry node", runID, workflowID, record.FlowRef)
		return "", false
	}
	if strings.TrimSpace(record.FlowRef) == "" {
		log.Printf("[flow-ref-resolve] run %q: workflowID %q resolved but FlowRef is empty", runID, workflowID)
		return "", false
	}
	log.Printf("[flow-ref-resolve] run %q: workflowID %q resolved to flowRef %q â€” flow-engine-driven path will run", runID, workflowID, record.FlowRef)
	return record.FlowRef, true
}

// takePendingFlowRefInvalidErr reads and clears the validation error (if any)
// resolveWorkflowFlowRef stashed on runID (BUG-270) â€” a one-shot read so a
// stale error from an earlier turn is never re-surfaced for a later one.
func (s *InteractiveService) takePendingFlowRefInvalidErr(runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[runID]
	if rs == nil {
		return nil
	}
	err := rs.pendingFlowRefInvalidErr
	rs.pendingFlowRefInvalidErr = nil
	return err
}

// explicitFlowRefResolves synchronously confirms an explicit chat flowRef
// (Chat Mode's Bug sub-mode picker, CP-42/Task-177) actually resolves to a
// valid stored/embedded flow definition, before handleStartTurn ever hands it
// to startTurn.
//
// BUG-261: validateChatOrchestrationSelection only checks that flowRef is a
// known option for the sub-mode (BuiltinOrchestrationOptions) â€” it never
// confirms the underlying stored definition is still valid. Unlike the
// sibling Flow-Mode workflow-picker path (resolveWorkflowFlowRef, above),
// which already resolves synchronously before deciding to attach a flowRef,
// the explicit chat path used to hand its raw flowRef straight to startTurn,
// which unconditionally suppresses the hub's own turn (flowStartOnly=true)
// the moment flowRef is non-empty â€” before startResolvedFlow's own async
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
// Runs the single inline entry node synchronously in-process â€” matching
// BehaviorScopeInline's contract of "no provider call" â€” then follows its
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
	log.Printf("[inline-entry-chain] flow %q: entered for run %q", flowRef, parentRunID)
	def := record.Definition
	entries := entryNodesNoDeps(def)
	if len(entries) != 1 {
		log.Printf("[inline-entry-chain] flow %q: bailing, found %d no-dependency entry node(s), want exactly 1", flowRef, len(entries))
		return false
	}
	entry := entries[0]

	canonical, ok := agentpack.NormalizeBehaviorID(entry.Behavior)
	if !ok {
		log.Printf("[inline-entry-chain] flow %q: bailing, entry node %q behavior %q does not normalize", flowRef, entry.ID, entry.Behavior)
		return false
	}
	spec, err := DefaultBehaviorRegistry().Resolve(canonical)
	if err != nil || spec.Scope != BehaviorScopeInline {
		log.Printf("[inline-entry-chain] flow %q: bailing, entry node %q behavior %q is not inline-scoped", flowRef, entry.ID, canonical)
		return false
	}

	sourceIDs := resolveEnabledContextSourceIDs(def, entry)
	mcpDriverTarget, err := s.resolveMCPDriverTargetForRun(ctx, parentRunID, entry, sourceIDs)
	if err != nil {
		log.Printf("[flow-executor] flow %q inline entry node %q failed to resolve mcp.driver target: %v", flowRef, entry.ID, err)
		return false
	}
	jiraIssueTarget, err := s.resolveJiraIssueTargetForRun(ctx, parentRunID, entry, sourceIDs)
	if err != nil {
		log.Printf("[flow-executor] flow %q inline entry node %q failed to resolve jira.issue target: %v", flowRef, entry.ID, err)
		return false
	}
	jiraSprintTarget, err := s.resolveJiraSprintTargetForRun(ctx, parentRunID, entry, sourceIDs)
	if err != nil {
		log.Printf("[flow-executor] flow %q inline entry node %q failed to resolve jira.sprint target: %v", flowRef, entry.ID, err)
		return false
	}
	firebaseCrashTarget, err := s.resolveFirebaseCrashTargetForRun(ctx, parentRunID, entry, sourceIDs)
	if err != nil {
		log.Printf("[flow-executor] flow %q inline entry node %q failed to resolve firebase.crashlytics target: %v", flowRef, entry.ID, err)
		return false
	}
	contextSourceIDs := filterString(sourceIDs, string(ContextSourceMCPDriver))
	contextSourceIDs = filterString(contextSourceIDs, string(ContextSourceJiraIssue))
	contextSourceIDs = filterString(contextSourceIDs, string(ContextSourceJiraSprint))
	contextSourceIDs = filterString(contextSourceIDs, string(ContextSourceFirebaseCrashlytics))
	out, err := DefaultBehaviorRegistry().Dispatch(ctx, canonical, BehaviorInput{
		NodeID:           entry.ID,
		WorkflowRunID:    parentRunID,
		WorkspaceCwd:     s.workspaceCwdFor(parentRunID),
		Prompt:           userPrompt,
		ContextSourceIDs: contextSourceIDs,
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
	var fcpProvenanceRunID string
	if pkg, ok := out.Payload["package"].(FlowContextPackage); ok {
		// BUG-288 R19-4: per-service marker secret for inline FCP render.
		prompt = renderFlowContextPromptWithSecret(ctx, pkg, userPrompt, s.markerSecret)
		// CP-51 Task-252: this is the SAME trustID selection ComposeFlowCodingPrompt
		// uses to mint the "flowpilot-fcp" marker embedded in prompt above — record
		// it so the spawned child can verify against its recorded provenance rather
		// than trusting parentRunID by topology alone.
		fcpProvenanceRunID = pkg.WorkflowRunID
		if fcpProvenanceRunID == "" {
			fcpProvenanceRunID = pkg.PackageID
		}
		// BUG-243 F-0: stash the package on the run so a mid-flow node reached
		// later (rag-harness's validate/audit) can read it back â€” previously
		// it was only ever used for this one prompt render, then discarded.
		s.mu.Lock()
		if rs := s.runs[parentRunID]; rs != nil {
			pkgCopy := pkg
			rs.planContextPackage = &pkgCopy
		}
		s.mu.Unlock()
		if store := s.persistenceStore(); store != nil {
			if err := PersistFlowContextPackage(ctx, store, pkg); err != nil {
				log.Printf("[flow-executor] inline entry: persist context package failed: %v", err)
			}
		}
	}
	prompt = appendGoogleDriveTargetPrompt(prompt, mcpDriverTarget)
	prompt = appendJiraIssueTargetPrompt(prompt, jiraIssueTarget)
	prompt = appendJiraSprintTargetPrompt(prompt, jiraSprintTarget)
	prompt = appendFirebaseCrashTargetPrompt(prompt, firebaseCrashTarget)
	// Task-202/223: INPUT file artifacts (read) + OUTPUT file write contract
	// for the delegate target. context_artifact stays on the package path above.
	prompt = composeFlowNodeAgentPrompt(s.workspaceCwdFor(parentRunID), prompt, *delegateTarget)
	prompt = appendChangeContractIfAnyWithSecret(s.workspaceCwdFor(parentRunID), parentRunID, prompt, s.markerSecret)

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
		Agent:                    agentName,
		Prompt:                   prompt,
		Wait:                     false,
		Label:                    delegateTarget.ID,
		AutoOrchestrate:          true,
		AgentDefOverride:         agentDef,
		Model:                    s.resolveFlowNodeModel(ctx, parentRunID, *delegateTarget),
		FCPMarkerProvenanceRunID: fcpProvenanceRunID,
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

func (s *InteractiveService) resolveMCPDriverTargetForRun(ctx context.Context, parentRunID string, node agentpack.FlowNode, sourceIDs []string) (string, error) {
	legacyRef, _ := resolveArtifactBoundMCPDriverRef(node)
	legacyRef = strings.TrimSpace(legacyRef)
	if !containsString(sourceIDs, string(ContextSourceMCPDriver)) {
		return "", nil
	}
	if legacyRef != "" {
		return "file:" + legacyRef, nil
	}
	choice, apiErr := s.AskWorkflowQuestion(
		ctx,
		parentRunID,
		"Google Drive context is enabled for this run. Open the picker to choose a Google Drive file or folder for this run, or skip Drive context this time.",
		[]QuestionOption{
			{
				Label:       "Open Google Drive Picker",
				Value:       "__google_drive_picker__",
				Description: "Choose a Drive file or folder for this run.",
			},
			{
				Label:       "Skip Google Drive",
				Value:       "__skip__",
				Description: "Run this flow without Google Drive context this time.",
			},
		},
		false,
	)
	if apiErr != nil {
		if apiErr.code == "question_expired" || apiErr.code == "interrupted" {
			return "", nil
		}
		return "", apiErr
	}
	if len(choice) == 0 {
		return "", nil
	}
	return normalizeGoogleDriveTarget(choice[0]), nil
}

func normalizeGoogleDriveTarget(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "__skip__" {
		return ""
	}
	if strings.HasPrefix(raw, "file:") || strings.HasPrefix(raw, "folder:") {
		return raw
	}
	return "file:" + raw
}

// resolveJiraIssueTargetForRun mirrors resolveMCPDriverTargetForRun (Task-229,
// CP-05-06 P-6b): jira.issue is opt-in per-flow, and when enabled the run asks
// the user for an issue key rather than the runner guessing one. A skipped or
// expired/interrupted question degrades to "" (no target), not an error.
func (s *InteractiveService) resolveJiraIssueTargetForRun(ctx context.Context, parentRunID string, node agentpack.FlowNode, sourceIDs []string) (string, error) {
	if !containsString(sourceIDs, string(ContextSourceJiraIssue)) {
		return "", nil
	}
	choice, apiErr := s.AskWorkflowQuestion(
		ctx,
		parentRunID,
		"Jira issue context is enabled for this run. Enter the Jira issue key (e.g. SCRUM-123) to focus on, or skip Jira issue context this time.",
		[]QuestionOption{
			{
				Label:       "Skip Jira issue",
				Value:       "__skip__",
				Description: "Run this flow without Jira issue context this time.",
			},
		},
		false,
	)
	if apiErr != nil {
		if apiErr.code == "question_expired" || apiErr.code == "interrupted" {
			return "", nil
		}
		return "", apiErr
	}
	if len(choice) == 0 {
		return "", nil
	}
	return normalizeJiraIssueTarget(choice[0]), nil
}

func normalizeJiraIssueTarget(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "__skip__" {
		return ""
	}
	return raw
}

// resolveJiraSprintTargetForRun mirrors resolveJiraIssueTargetForRun for
// jira.sprint, defaulting the offered choice to the connected board's active
// sprint (the common case) while still allowing an explicit sprint id/name.
func (s *InteractiveService) resolveJiraSprintTargetForRun(ctx context.Context, parentRunID string, node agentpack.FlowNode, sourceIDs []string) (string, error) {
	if !containsString(sourceIDs, string(ContextSourceJiraSprint)) {
		return "", nil
	}
	choice, apiErr := s.AskWorkflowQuestion(
		ctx,
		parentRunID,
		"Jira sprint context is enabled for this run. Use the active sprint, enter a specific sprint id, or skip Jira sprint context this time.",
		[]QuestionOption{
			{
				Label:       "Use Active Sprint",
				Value:       "__active_sprint__",
				Description: "Use the connected board's current active sprint.",
			},
			{
				Label:       "Skip Jira Sprint",
				Value:       "__skip__",
				Description: "Run this flow without Jira sprint context this time.",
			},
		},
		false,
	)
	if apiErr != nil {
		if apiErr.code == "question_expired" || apiErr.code == "interrupted" {
			return "", nil
		}
		return "", apiErr
	}
	if len(choice) == 0 {
		return "", nil
	}
	return normalizeJiraSprintTarget(choice[0]), nil
}

func normalizeJiraSprintTarget(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "__skip__" {
		return ""
	}
	if raw == "__active_sprint__" {
		return "active"
	}
	return raw
}

// resolveFirebaseCrashTargetForRun mirrors resolveJiraIssueTargetForRun for
// firebase.crashlytics (Task-231, CP-05-04 P-3/P-4).
func (s *InteractiveService) resolveFirebaseCrashTargetForRun(ctx context.Context, parentRunID string, node agentpack.FlowNode, sourceIDs []string) (string, error) {
	if !containsString(sourceIDs, string(ContextSourceFirebaseCrashlytics)) {
		return "", nil
	}
	choice, apiErr := s.AskWorkflowQuestion(
		ctx,
		parentRunID,
		"Firebase Crashlytics context is enabled for this run. Enter the crash issue id to investigate, or skip Firebase context this time.",
		[]QuestionOption{
			{
				Label:       "Skip Firebase Crashlytics",
				Value:       "__skip__",
				Description: "Run this flow without Firebase Crashlytics context this time.",
			},
		},
		false,
	)
	if apiErr != nil {
		if apiErr.code == "question_expired" || apiErr.code == "interrupted" {
			return "", nil
		}
		return "", apiErr
	}
	if len(choice) == 0 {
		return "", nil
	}
	return normalizeFirebaseCrashTarget(choice[0]), nil
}

func normalizeFirebaseCrashTarget(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "__skip__" {
		return ""
	}
	return raw
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func filterString(items []string, target string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			continue
		}
		out = append(out, item)
	}
	return out
}

func appendGoogleDriveTargetPrompt(prompt, target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return prompt
	}
	kind := "file"
	id := target
	if strings.HasPrefix(target, "folder:") {
		kind = "folder"
		id = strings.TrimPrefix(target, "folder:")
	} else if strings.HasPrefix(target, "file:") {
		id = strings.TrimPrefix(target, "file:")
	}
	note := "\n\nGoogle Drive runtime target for this run:\n" +
		"- kind: " + kind + "\n" +
		"- id: " + id + "\n\n" +
		"Use Google Drive MCP tools to inspect only this selected target when Drive context is needed.\n"
	if kind == "folder" {
		note += "Start with `listFolder` on the selected folder, then read only relevant files inside it.\n"
	} else {
		note += "Read the selected file directly with the Google Drive document-read tools.\n"
	}
	note += "Do not search other Drive locations unless the user explicitly asks.\n"
	return prompt + note
}

// appendJiraIssueTargetPrompt mirrors appendGoogleDriveTargetPrompt for the
// jira.issue runtime target (Task-229, CP-05-06 P-4): a bounded note telling
// the AI which single issue it may read via the jira MCP server, not a dump
// of the issue's content â€” the live AI turn fetches it itself with real MCP
// tools, same division of labor as mcp.driver.
func appendJiraIssueTargetPrompt(prompt, issueKey string) string {
	issueKey = strings.TrimSpace(issueKey)
	if issueKey == "" {
		return prompt
	}
	note := "\n\nJira runtime target for this run:\n" +
		"- issue key: " + issueKey + "\n\n" +
		fmt.Sprintf("Use Jira MCP tools from the `%s` server to read only this selected issue when Jira context is needed. ", jiraMcpServerName) +
		"Do not search or read other Jira issues unless the user explicitly asks.\n"
	return prompt + note
}

// appendJiraSprintTargetPrompt mirrors appendJiraIssueTargetPrompt for the
// jira.sprint runtime target.
func appendJiraSprintTargetPrompt(prompt, sprintTarget string) string {
	sprintTarget = strings.TrimSpace(sprintTarget)
	if sprintTarget == "" {
		return prompt
	}
	scope := "the active sprint"
	if sprintTarget != "active" {
		scope = "sprint " + sprintTarget
	}
	note := "\n\nJira sprint runtime target for this run:\n" +
		"- sprint: " + sprintTarget + "\n\n" +
		fmt.Sprintf("Use Jira MCP tools from the `%s` server to list and read issues only within %s on the configured board. ", jiraMcpServerName, scope) +
		"Do not read issues outside this sprint unless the user explicitly asks.\n"
	return prompt + note
}

// appendFirebaseCrashTargetPrompt mirrors appendJiraIssueTargetPrompt for the
// firebase.crashlytics runtime target (Task-231, CP-05-04 P-4): a bounded
// note, not crash content â€” the AI reads the crash itself via the firebase
// MCP server's Crashlytics tools.
func appendFirebaseCrashTargetPrompt(prompt, crashRef string) string {
	crashRef = strings.TrimSpace(crashRef)
	if crashRef == "" {
		return prompt
	}
	note := "\n\nFirebase Crashlytics runtime target for this run:\n" +
		"- crash issue id: " + crashRef + "\n\n" +
		fmt.Sprintf("Use Firebase MCP tools from the `%s` server (crashlytics_get_issue, crashlytics_list_events) to read only this selected crash issue when crash context is needed. ", firebaseMcpServerName) +
		"Do not search or read other Crashlytics issues unless the user explicitly asks.\n"
	return prompt + note
}

// nodeHasIncomingForwardEdge reports whether any FORWARD edge targets
// nodeID. A back edge (e.g. review-loop's synthesis->coder "continue" loop)
// does NOT disqualify a node as an entry â€” only an incoming forward edge
// means "something else must complete before this node starts." Mirrors the
// exact edge-aware entry-detection rule
// WorkflowsSettings.tsx's validateFlowGraph already documents and applies
// client-side ("a node is an entry node iff no forward edge targets it").
func nodeHasIncomingForwardEdge(edges []agentpack.FlowEdge, nodeID string) bool {
	for _, e := range edges {
		if strings.EqualFold(strings.TrimSpace(e.Kind), "forward") && e.To == nodeID {
			return true
		}
	}
	return false
}

// forwardEdgeSources returns the node ids that must complete before nodeID can
// start: the `from` of every FORWARD edge targeting nodeID, in stable declared
// order, de-duplicated. Back edges (loop re-entry, e.g. review-loop's
// synthesis->coder "continue") are excluded, matching nodeHasIncomingForwardEdge
// and the client-side computeDependsOnByStepType (WorkflowsSettings.tsx).
//
// BUG-282: this is the per-flow, edge-derived replacement for
// step_definitions.depends_on_json. Topology is authoritative on
// workflows.edges_json (a flow-scoped column); the shared step_definitions
// catalog (keyed by step_type, reusable across flows) must not carry it. A
// Supabase-backed flow's nodes get their DependsOn from here at load time
// (supabase_workflow_flow_store.go), so reusing one step across flows resolves
// against each flow's own edges instead of dragging in another flow's node ids.
func forwardEdgeSources(edges []agentpack.FlowEdge, nodeID string) []string {
	if nodeID == "" {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	for _, e := range edges {
		if !strings.EqualFold(strings.TrimSpace(e.Kind), "forward") || e.To != nodeID {
			continue
		}
		if e.From == "" || seen[e.From] {
			continue
		}
		seen[e.From] = true
		out = append(out, e.From)
	}
	return out
}

// entryNodesNoDeps returns a flow's entry nodes â€” regardless of behavior, in
// declared order â€” those with no incoming dependency, from EITHER the node's
// own static dependsOn list OR a forward edge targeting it.
//
// BUG (found while diagnosing a live rag-harness test session, 2026-07-09):
// this used to check only node.DependsOn. rag-harness.yaml's "implement" node
// declares its dependency on "context" purely via the edges list
// (`{from: context, to: implement, when: done, kind: forward}`), not a
// dependsOn field â€” so entryDelegateNodes (below) wrongly classified
// "implement" itself as a zero-dependency entry node, and startResolvedFlow
// spawned the coder directly, skipping context.produce (and
// startInlineEntryChain's step-status wiring) entirely. Confirmed via
// .flowpilot/logs/features/agent-flow-engine: flow_start_resolved was
// immediately followed by flow_start_entry_spawn_attempt{node_id:"implement"}
// with no flow_start_inline_entry in between. Any flow whose entry
// dependency is edge-only (not dependsOn-only), not just rag-harness, has
// this exposure â€” including CP-45's context-coding-review-synthesis.yaml.
// Unlike entryDelegateNodes this does not filter by behavior, so it also
// matches an inline-behavior entry node (see startInlineEntryChain).
func entryNodesNoDeps(def agentpack.FlowDefinition) []agentpack.FlowNode {
	var out []agentpack.FlowNode
	for _, node := range def.Nodes {
		if len(node.DependsOn) > 0 {
			continue
		}
		if nodeHasIncomingForwardEdge(def.Edges, node.ID) {
			continue
		}
		out = append(out, node)
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

// forwardReachableNodeIDs (BUG-286) returns every node id reachable from
// startID by following only FORWARD edges (a back edge, e.g. synthesis's own
// re-entry edge, must not be walked here â€” that would make the loop's re-entry
// point "reachable from itself" and defeat the purpose of this scoping).
// startID itself is never included, so callers can freely re-mark it RUNNING
// while resetting only what actually needs to re-run. Terminal pseudo-nodes
// ("done", "ask_user") are never added, matching FLOW_EDGE_TERMINALS.
//
// Used by the "continue" (new review round) reset in applyFlowControl: only
// nodes forward-reachable from the back-edge's re-entry target (e.g. "coder")
// should reset to PENDING for the new round. A node that is NOT
// forward-reachable from there â€” e.g. rag-harness/context-coding-review-
// synthesis's own "context" entry node, which sits BEFORE the loop and only
// ever runs once (lifecycle: once) â€” must keep its prior DONE status instead
// of being wrongly reset to PENDING and then never revisited.
func forwardReachableNodeIDs(edges []agentpack.FlowEdge, startID string) map[string]bool {
	reachable := make(map[string]bool)
	if startID == "" {
		return reachable
	}
	queue := []string{startID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, e := range edges {
			if !strings.EqualFold(strings.TrimSpace(e.Kind), "forward") || e.From != cur {
				continue
			}
			to := strings.TrimSpace(e.To)
			if to == "" || to == "done" || to == "ask_user" || reachable[to] {
				continue
			}
			reachable[to] = true
			queue = append(queue, to)
		}
	}
	return reachable
}

// notifyHubFlowStarted tells the hub (parent run) an agent has already been
// spawned to do the actual work, so the hub's own reasoning doesn't
// redundantly try to also write the code itself. Delivered through the same
// pending-context mechanism the "coder completed" notes use â€” visible the
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
// `run: inline` â€” it is the hub's own reinvoke turn, not a spawned child, and
// that transition was already correctly Go-orchestrated via the existing
// cohort-join â†’ maybeAutoReinvokeHub path. Only the coder â†’ reviewer-cohort
// step needed a real fix.
//
// Returns false (a no-op) when the run has no tracked flow, the node has no
// outgoing forward ("done") edges, or any edge target isn't a spawnable
// agent.delegate node (e.g. a hub.inline node) â€” callers fall back to the
// existing note+reinvoke-hub behavior in that case, so a flow shape this
// function doesn't understand degrades to the pre-existing AI-driven
// behavior rather than silently doing nothing.
func (s *InteractiveService) tryAdvanceFlowFromNode(parentRunID, completedNodeID, resultMessage string) bool {
	// BUG-234: do not auto-advance once the loop has legitimately settled
	// (blocked/awaiting-user, done, stopped, paused). A child completion that
	// lands after the loop blocked would otherwise re-spawn the next nodes and,
	// via the cohort join, flip the hub node back to RUNNING â€” a runaway that
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

	// BUG-243 F-0: a single forward-done target whose behavior is a
	// registered INLINE-scope behavior (validate/audit, not a spawnable
	// agent.delegate) previously fell straight through to the bail below â€”
	// the executor had no path to dispatch an inline node reached mid-flow
	// (only the flow's own entry node, via startInlineEntryChain, was ever
	// dispatched in-process). Handling it here, before the delegate-only
	// validation loop, preserves every existing multi-target cohort-spawn
	// case (e.g. coder -> [reviewer_correctness, reviewer_security])
	// unchanged: this only fires for the single-target inline case.
	if len(targetIDs) == 1 {
		if target, ok := findFlowNode(nodes, targetIDs[0]); ok {
			if canonical, ok := agentpack.NormalizeBehaviorID(target.Behavior); ok && canonical != "agent.delegate" {
				if spec, err := DefaultBehaviorRegistry().Resolve(canonical); err == nil && spec.Scope == BehaviorScopeInline {
					// BUG-289 L5/F-10: mark the delegate source DONE before the
					// early return (the multi-target path does this at :1052-1053).
					if s.isFlowEngineDriven(parentRunID) {
						s.setFlowStepStatus(context.Background(), parentRunID, completedNodeID, StepStatusDone)
					}
					return s.tryAdvanceFlowThroughInline(parentRunID, edges, nodes, target, resultMessage)
				}
			}
		}
	}

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
	// "report your findings via the flow's declared control tool" â€” i.e. call
	// submit_review_outcome/flow_control itself. turnBridge.SubmitFlowControl
	// routes a child's flow_control call straight to its parent's
	// applyFlowControl, with no cohort-join gate at all, so a reviewer that
	// literally followed that instruction could advance/complete/block the
	// whole flow round before the other cohort member(s) even finished â€”
	// bypassing the join barrier maybeAutoReinvokeHub depends on. Every other
	// reviewer-spawn path in this codebase (see the manually-spawned E2E
	// tests) just asks the reviewer to review and report findings as its own
	// final message; only the hub's own synthesis turn (after the join
	// completes) is supposed to call the control tool. Match that convention.
	// BUG-174: the node that just completed is DONE on the step timeline; its
	// forward targets become RUNNING as they are spawned below. Gated to
	// flow-engine-driven runs so the AI-driven spawn_agent path is untouched.
	flowDriven := s.isFlowEngineDriven(parentRunID)
	if flowDriven {
		s.setFlowStepStatus(context.Background(), parentRunID, completedNodeID, StepStatusDone)
	}
	cwd := s.workspaceCwdFor(parentRunID)

	spawnedAny := false
	for i, node := range targetNodes {
		// BUG-234: re-check the loop status before EACH spawn, not just at entry.
		// The loop can transition to blocked/done between the entry gate and here
		// (e.g. a concurrent hub synthesis turn escalates while this advance
		// goroutine is resolving targets / building the prompt). Without this
		// re-check, a completion that entered while "running" would still spawn the
		// next round's reviewers into an already-settled loop â€” the runaway that
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
		// Task-224 / BUG-277: deliverable-centric review â€” when the target has
		// file_artifact INPUT paths, omit the full coder final message body
		// (path+tools is enough). Otherwise keep a truncated handoff body.
		baseReviewPrompt := buildFlowReviewHandoffPrompt(completedNodeID, resultMessage, node)
		// Task-223: each target node gets its own INPUT path inject + OUTPUT write contract.
		prompt := composeFlowNodeAgentPrompt(cwd, baseReviewPrompt, node)
		prompt = appendChangeContractIfAnyWithSecret(cwd, parentRunID, prompt, s.markerSecret)
		if flowNodeReusesChild(node) {
			// BUG-318: pass THIS round's cohort id + size so a reinvoke-lifecycle
			// cohort member re-joins a live cohort (see reinvokeExistingFlowChild).
			if reused := s.reinvokeExistingFlowChild(parentRunID, node.ID, prompt, cohortID, len(targetNodes)); reused {
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
			Model:            s.resolveFlowNodeModel(context.Background(), parentRunID, node),
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

// buildFlowReviewHandoffPrompt builds the tryAdvanceFlowFromNode brief for a
// downstream delegate. Task-224: when node has file_artifact INPUT paths,
// omit the full upstream final message (deliverable is the file path).
func buildFlowReviewHandoffPrompt(completedNodeID, resultMessage string, node agentpack.FlowNode) string {
	if nodeHasFileArtifactInput(node) {
		return fmt.Sprintf(
			"[flow-engine] Review this result from node %q and report your findings (approve or request changes, with specifics) as your final message.\n\n"+
				"The upstream deliverable is in the bound file artifact path(s) listed below â€” open them with your tools. "+
				"Coder final message body is omitted on purpose (path-first handoff).",
			completedNodeID,
		)
	}
	return fmt.Sprintf(
		"[flow-engine] Review this result from node %q and report your findings (approve or request changes, with specifics) as your final message:\n\n%s",
		completedNodeID, truncateDisplayField(resultMessage, 2000),
	)
}

// forwardDoneTargets returns the (deduplicated) targets of fromNodeID's
// forward edges gated on "done" â€” the condition a normal successful turn
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

func (s *InteractiveService) reinvokeExistingFlowChild(parentRunID, nodeID, prompt, cohortID string, cohortSize int) bool {
	if strings.TrimSpace(nodeID) == "" {
		return false
	}
	// BUG-318: a reinvoke-lifecycle cohort member (e.g. a single reviewer reused
	// across review-loop rounds) must RE-JOIN this round's cohort. Round 0 spawned
	// it into "flow-auto-<src>-round-0" and cohort-join drained that key (deleting
	// its cohortExpected entry) on completion. reinvokeMatchingFlowChild never
	// touches flowCohortId, so without this the reused child keeps the drained
	// round-0 cohort id: its next completion appends to a dead cohort
	// (cohortExpected==0), cohortComplete stays false, and the hub synthesis
	// reinvoke (maybeAutoReinvokeHubWithNote) is never scheduled -> the flow hangs.
	// Register THIS round's cohort and re-tag the reused child so its completion
	// joins a live barrier, mirroring the spawn path (spawnChildRun's FlowCohortID +
	// preRegisterCohort). A back-edge coder continue passes cohortID="" (no cohort).
	if strings.TrimSpace(cohortID) != "" {
		s.agentOrchestrator.preRegisterCohort(parentRunID, cohortID, cohortSize)
	}
	return s.reinvokeMatchingFlowChild(parentRunID, prompt, func(child *interactiveRun) bool {
		if child.label != nodeID {
			return false
		}
		if strings.TrimSpace(cohortID) != "" {
			// Caller (reinvokeMatchingFlowChild) holds s.mu while invoking this
			// predicate, so mutating the matched child here is lock-safe.
			child.flowCohortId = cohortID
		}
		return true
	})
}

// reinvokeMatchingFlowChild is the single implementation behind every
// "reactivate an existing child run for another turn" path: find the first
// non-terminal-turn child of parentRunID satisfying match, transition it back
// to running, and notify the desktop. BUG-242: this used to be duplicated â€”
// reinvokeExistingFlowChild (the forward-edge reuse-lifecycle path) had the
// BUG-Rnd2 fixes below, but maybeReinvokeCoderForContinue's back-edge
// "continue" reinvoke (the actual path a review-loop round-2+ coder re-entry
// takes) had its own older, un-fixed copy â€” so the coder reappearing for round
// 2 stayed stuck in the desktop's "Recently closed" section with no new
// main-chat card, while a forward-spawned reviewer behaved correctly.
//
//   - BUG-Rnd2 (Bug B): increments activationSeq so the desktop's monotonic
//     terminal-status guard (mergeAgentRunsById) recognizes a genuine
//     completedâ†’running transition instead of discarding it as a stale
//     snapshot â€” without this the run never leaves "completed" client-side and
//     is miscategorized as closed regardless of what the backend just did.
//   - BUG-Rnd2 (Bug C): emits EventAgentSpawnedByUser so the parent thread
//     renders a new agent card for this turn, matching the spawnChildRun path
//     (idempotent by event id, so replay never duplicates the row).
//
// Returns true if a matching child was found (whether or not a new turn was
// actually scheduled â€” a match with a turn already in flight still counts as
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
// nodes with no incoming dependency (neither a declared dependsOn NOR an
// incoming forward edge â€” see nodeHasIncomingForwardEdge), in declared
// order. These are the nodes a flow run starts from; every other node is
// reached through edges once its dependencies complete.
//
// The edge check is required, not optional: a flow whose entry dependency is
// expressed purely via `edges:` (rag-harness's "context -> implement", CP-45's
// "context -> coder" â€” no dependsOn field on the delegate node itself) would
// otherwise have its true entry node (an inline behavior like
// context.produce) silently bypassed in favor of the first delegate node,
// which this function would wrongly also call an "entry." See
// entryNodesNoDeps's doc comment for the live-test trace that found this.
func entryDelegateNodes(def agentpack.FlowDefinition) []agentpack.FlowNode {
	var out []agentpack.FlowNode
	for _, node := range def.Nodes {
		if len(node.DependsOn) > 0 {
			continue
		}
		if nodeHasIncomingForwardEdge(def.Edges, node.ID) {
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
	// Normalize Windows backslash separators to "/" before the POSIX path
	// package parses the ref. A node's agent ref can be an absolute path from a
	// provider CLI config (e.g. C:\Users\me\.codex\agents\coder-agent.toml), and
	// path.Base/path.Ext only understand "/", so on such a ref they would leave
	// the whole directory in place and return the full path minus only the
	// extension instead of the basename (BUG-321). This is an unconditional
	// string replace, not path/filepath, so the derivation is identical on
	// Windows and Linux/CI regardless of the host OS separator; it is a no-op for
	// the "/"-based and bare-name refs every built-in flow pack already uses.
	agent = strings.ReplaceAll(agent, "\\", "/")
	base := path.Base(agent)
	return strings.TrimSuffix(base, path.Ext(base))
}

// resolveFlowNodeModel resolves an agent.delegate flow node's OWN configured
// model, so it can run on a different model/provider than the flow's own
// resolved model instead of always inheriting it (BUG-228). After BUG-236,
// built-in mirror sync creates one node-specific step_definitions row per
// flow node. Lookup is scoped by the parent run's chatFlowRef when set so
// Chat Mode Review Loop does not pick another flow's row that shares
// node_id="coder" (Context Coding Coder=claude-haiku vs Review Loop
// Coder=grok-composer-2.5-fast — CA-358).
//
// Returns "" when no scoped/role row has a model, so callers pass empty
// SpawnAgentInput.Model and spawnChildRun falls back to inherit-from-parent.
// hub.inline never reaches this path.
func (s *InteractiveService) resolveFlowNodeModel(ctx context.Context, parentRunID string, node agentpack.FlowNode) string {
	if canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior); ok && canonical != "agent.delegate" {
		return ""
	}
	return s.resolveConfiguredModelForAgent(ctx, node.ID, flowNodeAgentName(node), s.flowRefForRun(parentRunID))
}

// flowRefForRun returns the active built-in/chat flowRef for model scoping.
func (s *InteractiveService) flowRefForRun(runID string) string {
	if s == nil || strings.TrimSpace(runID) == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if rs := s.runs[runID]; rs != nil {
		return strings.TrimSpace(rs.chatFlowRef)
	}
	return ""
}

// flowRefToMirrorStepPrefix maps "packId/flowId" to the sanitizeStepType prefix
// used by flowNodeStepType (without the trailing _nodeId).
// e.g. flowpilot-core-flow-pack/review-loop → flowpilot_core_flow_pack_review_loop
func flowRefToMirrorStepPrefix(flowRef string) string {
	flowRef = strings.TrimSpace(flowRef)
	if flowRef == "" {
		return ""
	}
	flowKey := flowRef
	if packID, flowID, ok := strings.Cut(flowRef, "/"); ok {
		flowKey = packID + "__" + flowID
	}
	return sanitizeStepType(flowKey)
}

// stepTypeBelongsToFlowMirror reports whether stepType is a per-node mirror row
// for the given flow prefix + node id.
func stepTypeBelongsToFlowMirror(stepType, flowPrefix, nodeID string) bool {
	st := strings.ToLower(strings.TrimSpace(stepType))
	pref := strings.ToLower(strings.TrimSpace(flowPrefix))
	if st == "" || pref == "" {
		return false
	}
	nodeKey := sanitizeStepType(nodeID)
	if nodeKey == "" {
		return st == pref || strings.HasPrefix(st, pref+"_") || strings.Contains(st, "_"+pref+"_")
	}
	want := pref + "_" + nodeKey
	if st == want || strings.HasSuffix(st, "_"+want) {
		return true
	}
	// UUID-prefixed or alternate separators: must contain flow+node token.
	if strings.Contains(st, want) || strings.Contains(st, pref+"__"+nodeKey) {
		return true
	}
	return false
}

// resolveConfiguredModelForAgent looks up step_definitions.model for a flow
// node / agent role.
//
// Order (CA-358 / cross-flow node_id collision):
//  1. flowRef-scoped per-node mirror (node_id + step_type belongs to this flow)
//  2. Unambiguous unscoped node_id match (exactly one non-generic row)
//  3. Role row "flow-agent-delegate-<agent>"
//
// BUG-241: skip generic flow-agent-delegate* in node_id matching.
// Never first-match among MULTIPLE node_id hits (Review Loop vs Context Coding).
func (s *InteractiveService) resolveConfiguredModelForAgent(ctx context.Context, nodeID, agentName, flowRef string) string {
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
	nodeID = strings.TrimSpace(nodeID)
	flowPrefix := flowRefToMirrorStepPrefix(flowRef)

	// Pass 1: flow-scoped node_id match.
	if flowPrefix != "" && nodeID != "" {
		for _, step := range steps {
			if isGenericFlowDispatchStepType(step.ID) {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(step.NodeID), nodeID) {
				continue
			}
			if strings.TrimSpace(step.Model) == "" {
				continue
			}
			if stepTypeBelongsToFlowMirror(step.ID, flowPrefix, nodeID) {
				return strings.TrimSpace(step.Model)
			}
		}
	}

	// Pass 2: unscoped node_id — only if unique among non-generic rows.
	if nodeID != "" {
		var hits []string
		for _, step := range steps {
			if isGenericFlowDispatchStepType(step.ID) {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(step.NodeID), nodeID) {
				continue
			}
			if m := strings.TrimSpace(step.Model); m != "" {
				hits = append(hits, m)
			}
		}
		if len(hits) == 1 {
			return hits[0]
		}
	}

	// Pass 3: purpose-named role row.
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
// "flow-hub-inline", "flow-agent-delegate-coder", â€¦). These are FK
// placeholders, not real node definitions, and must never win a node_id match
// (BUG-241). Real per-node mirror step_types never begin with the literal
// "flow-" prefix because sanitizeStepType rewrites "-" to "_".
func isGenericFlowDispatchStepType(stepType string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(stepType)), "flow-")
}

// resolveFlowNodeProviderModel resolves node's OWN actually-effective
// provider/model: its role's step_definitions row when one resolves, else the
// run's own baseline (the same fallback spawnChildRun applies once the node
// is actually spawned) â€” so callers get the node's true eventual posture
// whether or not it has been spawned yet.
func (s *InteractiveService) resolveFlowNodeProviderModel(ctx context.Context, parentRunID string, node agentpack.FlowNode) (provider, model string) {
	model = s.resolveFlowNodeModel(ctx, parentRunID, node)
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
// that name through the general-purpose catalog â€” which prefers a
// project-local .claude/agents/coder.md or .codex/agents/coder.md over the
// pack's own bundled agent of the same name. Any repo happening to define its
// own "coder"/"reviewer" agent for an unrelated purpose would silently swap
// out the built-in Review Loop's prompts, breaking the "pack-driven, not
// hardcoded" guarantee CP-42/CA-158 established. Every flow_executor.go spawn
// call site passes its result as SpawnAgentInput.AgentDefOverride so a
// pack-declared node's agent identity is never subject to catalog shadowing.
//
// Returns (nil, false) if the pack can't be loaded or has no agent by that
// name â€” callers fall back to the normal catalog-based resolution in that
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
