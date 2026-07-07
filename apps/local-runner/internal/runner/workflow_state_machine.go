package runner

import "fmt"

// Phase 5 (04-05): the workflow run/step state machine, ported from the TS
// orchestration that lives in the Admin Web server tier + Supabase edge functions
// today (supabase/functions/_shared/workflow-engine-state-machine.ts). This is a
// faithful, golden-tested port: given the same steps/yolo/now it produces the same
// transitions as planWorkflowProgress in TS, so the runner becomes the single
// orchestration backend without behavior drift.
//
// Pure logic: no I/O, no locking. The orchestrator applies the returned plan
// (persisting transitions to Supabase + emitting events) under a per-run lock.

// RuntimeWorkflowStepStatus mirrors the TS union.
type RuntimeWorkflowStepStatus string

const (
	StepStatusPending        RuntimeWorkflowStepStatus = "PENDING"
	StepStatusRunning        RuntimeWorkflowStepStatus = "RUNNING"
	StepStatusWaitingUserApr RuntimeWorkflowStepStatus = "WAITING_USER_APPROVAL"
	StepStatusDone           RuntimeWorkflowStepStatus = "DONE"
	StepStatusFailed         RuntimeWorkflowStepStatus = "FAILED"
	StepStatusSkipped        RuntimeWorkflowStepStatus = "SKIPPED"
)

// WorkflowRunStatus is the run-level status the plan resolves to. (The full DB set
// includes PENDING/FAILED/CANCELED/WAITING_USER_APPROVAL; the planner only returns
// RUNNING or DONE — a pause keeps the run RUNNING with a pausedStepId.)
type WorkflowRunStatus string

const (
	RunStatusEngineRunning WorkflowRunStatus = "RUNNING"
	RunStatusEngineDone    WorkflowRunStatus = "DONE"
)

// RuntimeWorkflowStep is the planner's view of a step.
type RuntimeWorkflowStep struct {
	ID               string
	StepType         string
	Status           RuntimeWorkflowStepStatus
	RequiresApproval bool
	StartedAt        string // "" == null
	FinishedAt       string // "" == null
	RetryCount       int
	RejectionNote    string // "" == null
	// BehaviorID is the CP-42 canonical behavior id (agent.delegate/
	// context.produce/...) declared on the joined step definition, when set.
	// "" for a step whose definition predates CP-42 or never
	// set one — callers fall back to classifying by StepType in that case
	// (BUG-NOTE-CP42 #7).
	BehaviorID string
	// NodeID is the flow-graph node id (e.g. "coder", "reviewer_correctness")
	// from the joined step definition. Distinct from StepType,
	// which for a CP-42 flow-engine node is a shared generic dispatch category
	// (e.g. "flow-agent-delegate") and therefore identical across every node
	// running the same behavior — NodeID is what the UI must show as the
	// per-step name instead (BUG-155).
	NodeID string
	// AgentRef is the step definition's agent_ref value: the agent definition
	// file this node delegates to (e.g. "coder"), when the step declares one.
	// "" when the step has no agent binding of its own (e.g. inline/control
	// behaviors).
	AgentRef string
	// Provider/Model reflect the step's actually-resolved posture. For the
	// classic (non-flow-engine) planner these are derived from the step
	// type's own catalog default at seed time (step_definitions.model, with
	// Provider derived from that model via providerKeyFromModel) — BUG-164
	// removed workflow_steps.provider_override/model_override entirely; a
	// step type has exactly one configured model, not a per-workflow-instance
	// override. For a flow-engine node these start empty at seed time
	// (flowStepRowsFromNodes) and are patched in once the node is actually
	// spawned, via stampFlowNodePosture/setFlowStepPosture (BUG-228) — either
	// the node's own role's step_definitions row, or the run's own baseline
	// posture when the role has no such row.
	Provider string
	Model    string
	// YoloMode is the step_type's step_definitions.yolo_mode default.
	YoloMode bool
}

// WorkflowStepPatch is the mutation to apply to a step. Pointer fields distinguish
// "set to null" (non-nil pointer to "") from "leave unchanged" (nil), matching the
// optional-field semantics of the TS patch.
type WorkflowStepPatch struct {
	Status        RuntimeWorkflowStepStatus
	StartedAt     *string
	FinishedAt    *string
	RejectionNote *string
	RetryCount    *int
	// Provider/Model stamp the node's OWN actually-resolved posture (BUG-228
	// display follow-up) so the step-timeline UI shows what that node really
	// ran on instead of always mirroring the run's single baseline posture.
	Provider *string
	Model    *string
}

// WorkflowLogLevel mirrors the TS log levels.
type WorkflowLogLevel string

const (
	LogInfo  WorkflowLogLevel = "info"
	LogWarn  WorkflowLogLevel = "warn"
	LogError WorkflowLogLevel = "error"
	LogDebug WorkflowLogLevel = "debug"
)

type WorkflowLog struct {
	LogLevel WorkflowLogLevel
	Message  string
}

// WorkflowStepTransition is one step's patch + logs.
type WorkflowStepTransition struct {
	StepID string
	Patch  WorkflowStepPatch
	Logs   []WorkflowLog
}

// WorkflowProgressPlan is the planner output: the resolved run status, the optional
// finish time, the paused step (if any), and the ordered step transitions.
type WorkflowProgressPlan struct {
	RunStatus       WorkflowRunStatus
	RunFinishedAt   string // "" == null
	PausedStepID    string // "" == null
	StepTransitions []WorkflowStepTransition
}

func strptr(s string) *string { return &s }
func intptr(i int) *int       { return &i }

// startedOrNow returns the step's existing startedAt, or now if unset (the TS
// `step.startedAt ?? now`).
func startedOrNow(startedAt, now string) string {
	if startedAt != "" {
		return startedAt
	}
	return now
}

// PlanWorkflowProgress walks the steps in execution order and resolves the next
// transitions. It is a line-for-line port of planWorkflowProgress (TS):
//
//   - terminal steps (DONE/SKIPPED/FAILED) are skipped;
//   - a WAITING_USER_APPROVAL step pauses the run unless YOLO auto-approves it;
//   - a PENDING/RUNNING step that requires approval (and not YOLO) transitions to
//     WAITING_USER_APPROVAL and pauses; otherwise it completes;
//   - if no step pauses, the run is DONE.
func PlanWorkflowProgress(steps []RuntimeWorkflowStep, yoloMode bool, now string) WorkflowProgressPlan {
	transitions := []WorkflowStepTransition{}

	for _, step := range steps {
		switch step.Status {
		case StepStatusDone, StepStatusSkipped, StepStatusFailed:
			continue

		case StepStatusWaitingUserApr:
			if !yoloMode {
				return WorkflowProgressPlan{
					RunStatus:       RunStatusEngineRunning,
					PausedStepID:    step.ID,
					StepTransitions: transitions,
				}
			}
			transitions = append(transitions, WorkflowStepTransition{
				StepID: step.ID,
				Patch: WorkflowStepPatch{
					Status:        StepStatusDone,
					StartedAt:     strptr(startedOrNow(step.StartedAt, now)),
					FinishedAt:    strptr(now),
					RejectionNote: strptr(""),
				},
				Logs: []WorkflowLog{{LogInfo, fmt.Sprintf("YOLO mode auto-approved step %s.", step.StepType)}},
			})
			continue

		case StepStatusPending, StepStatusRunning:
			if step.RequiresApproval && !yoloMode {
				transitions = append(transitions, WorkflowStepTransition{
					StepID: step.ID,
					Patch: WorkflowStepPatch{
						Status:     StepStatusWaitingUserApr,
						StartedAt:  strptr(startedOrNow(step.StartedAt, now)),
						FinishedAt: strptr(""),
					},
					Logs: []WorkflowLog{{LogInfo, fmt.Sprintf("Generation complete. Paused on approval gate for %s.", step.StepType)}},
				})
				return WorkflowProgressPlan{
					RunStatus:       RunStatusEngineRunning,
					PausedStepID:    step.ID,
					StepTransitions: transitions,
				}
			}

			msg := fmt.Sprintf("Completed step %s.", step.StepType)
			if yoloMode && step.RequiresApproval {
				msg = fmt.Sprintf("YOLO mode auto-approved step %s.", step.StepType)
			}
			transitions = append(transitions, WorkflowStepTransition{
				StepID: step.ID,
				Patch: WorkflowStepPatch{
					Status:        StepStatusDone,
					StartedAt:     strptr(startedOrNow(step.StartedAt, now)),
					FinishedAt:    strptr(now),
					RejectionNote: strptr(""),
				},
				Logs: []WorkflowLog{{LogInfo, msg}},
			})
		}
	}

	return WorkflowProgressPlan{
		RunStatus:       RunStatusEngineDone,
		RunFinishedAt:   now,
		StepTransitions: transitions,
	}
}

// PlanRejectedStepRetry resets a rejected step to PENDING with an incremented retry
// count (port of planRejectedStepRetry).
func PlanRejectedStepRetry(stepID string, retryCount int, now, comment string) WorkflowStepTransition {
	return WorkflowStepTransition{
		StepID: stepID,
		Patch: WorkflowStepPatch{
			Status:        StepStatusPending,
			StartedAt:     strptr(""),
			FinishedAt:    strptr(""),
			RejectionNote: strptr(comment),
			RetryCount:    intptr(retryCount + 1),
		},
		Logs: []WorkflowLog{{LogWarn, fmt.Sprintf("Rejection received at %s: %q. Resetting step for another pass.", now, comment)}},
	}
}
