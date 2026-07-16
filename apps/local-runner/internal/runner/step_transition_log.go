package runner

import (
	"context"
	"strings"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// stepTransitionLine is one NDJSON line in the per-run step-transition sidecar
// (Task-239 / charter T-10). Appended by setFlowStepStatus / setFlowStepPosture
// so resume can replay settled node status instead of guessing from session
// evidence alone (I-17).
//
// Status empty = posture-only line (provider/model stamp; BUG-228). Replay must
// not overwrite Status when Status is empty (ApplyStepTransition empty-status
// semantics).
type stepTransitionLine struct {
	RunID    string `json:"run_id"`
	NodeID   string `json:"node_id"`
	Status   string `json:"status,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	TS       string `json:"ts"` // RFC3339Nano
}

// StepTransitionLogStore persists per-node step transitions for flow-engine
// runs so reconstructRun can replay them after a process restart (Task-239).
// Implemented by localFileSessionStore; absent on fakeWorkflowStore so unit
// tests without a disk store keep the legacy evidence-walk path (I-14 fallback).
type StepTransitionLogStore interface {
	AppendStepTransition(ctx context.Context, runID string, line stepTransitionLine) error
	LoadStepTransitions(ctx context.Context, runID string) ([]stepTransitionLine, error)
	DeleteStepTransitions(ctx context.Context, runID string) error
}

// applyStepTransitionReplay merges transition-log evidence into evidence-walk
// rows (Task-239 B3 merge rule):
//
//   - Nodes that appear in the log: last non-empty Status wins; posture fields
//     (Provider/Model) apply from any line that carries them.
//   - Nodes never logged: keep the evidence-walk result (legacy no-label members).
//   - In-flight statuses are normalized: RUNNING → CANCELED; WAITING_USER_APPROVAL
//     stays only when keepWaitingNodeIDs says a pending gate still exists.
//   - FAILED/CANCELED are never promoted to DONE (I-3) — last log status is final.
func applyStepTransitionReplay(rows []RuntimeWorkflowStep, lines []stepTransitionLine, keepWaitingNodeIDs map[string]bool) []RuntimeWorkflowStep {
	if len(lines) == 0 || len(rows) == 0 {
		return rows
	}
	byID := make(map[string]*RuntimeWorkflowStep, len(rows))
	for i := range rows {
		byID[rows[i].ID] = &rows[i]
	}
	type last struct {
		status   string
		provider string
		model    string
		ts       string
		hasAny   bool
	}
	lastByNode := make(map[string]*last, len(rows))
	for _, line := range lines {
		nodeID := strings.TrimSpace(line.NodeID)
		if nodeID == "" {
			continue
		}
		cur := lastByNode[nodeID]
		if cur == nil {
			cur = &last{}
			lastByNode[nodeID] = cur
		}
		cur.hasAny = true
		if st := strings.TrimSpace(line.Status); st != "" {
			// V9-13 / BUG-288 #21: full terminal monotonic — never resurrect
			// DONE/FAILED/CANCELED into non-terminal or promote FAILED/CANCELED→DONE.
			prev := RuntimeWorkflowStepStatus(cur.status)
			next := RuntimeWorkflowStepStatus(st)
			if isTerminalStepStatus(prev) {
				// Terminal stays terminal; only allow same terminal overwrite of posture.
				if next == prev {
					cur.status = st
				}
				// else ignore DONE→RUNNING, FAILED→PENDING, FAILED→DONE, etc.
			} else if (prev == StepStatusFailed || prev == StepStatusCanceled) && next == StepStatusDone {
				// Ignore late DONE after terminal failure/cancel.
			} else {
				cur.status = st
			}
		}
		if p := strings.TrimSpace(line.Provider); p != "" {
			cur.provider = p
		}
		if m := strings.TrimSpace(line.Model); m != "" {
			cur.model = m
		}
		if ts := strings.TrimSpace(line.TS); ts != "" {
			cur.ts = ts
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for nodeID, cur := range lastByNode {
		row := byID[nodeID]
		if row == nil || !cur.hasAny {
			continue
		}
		if cur.provider != "" {
			row.Provider = cur.provider
		}
		if cur.model != "" {
			row.Model = cur.model
		}
		if cur.status == "" {
			continue
		}
		status := RuntimeWorkflowStepStatus(cur.status)
		// Normalize in-flight after kill (I-17).
		switch status {
		case StepStatusRunning:
			status = StepStatusCanceled
		case StepStatusWaitingUserApr:
			if keepWaitingNodeIDs == nil || !keepWaitingNodeIDs[nodeID] {
				status = StepStatusCanceled
			}
		}
		row.Status = status
		// V9-13: prefer log timestamps instead of inventing "now".
		stamp := cur.ts
		if stamp == "" {
			stamp = now
		}
		switch status {
		case StepStatusDone:
			if row.StartedAt == "" {
				row.StartedAt = stamp
			}
			row.FinishedAt = stamp
		case StepStatusFailed, StepStatusCanceled, StepStatusSkipped:
			row.FinishedAt = stamp
		case StepStatusRunning, StepStatusWaitingUserApr:
			if row.StartedAt == "" {
				row.StartedAt = stamp
			}
			row.FinishedAt = ""
		case StepStatusPending:
			row.StartedAt = ""
			row.FinishedAt = ""
		}
	}
	return rows
}

func isTerminalStepStatus(st RuntimeWorkflowStepStatus) bool {
	switch st {
	case StepStatusDone, StepStatusFailed, StepStatusCanceled, StepStatusSkipped:
		return true
	default:
		return false
	}
}

// keepWaitingNodeIDsForResume returns node ids that should keep
// WAITING_USER_APPROVAL after replay because a pending question/approval still
// exists (BUG-271 re-derive). Approvals/questions are run-scoped; when any
// pending gate exists on the parent we keep the hub inline node waiting.
// LoopState.ActiveNode is also preserved when the loop is blocked.
//
// extraNodeIDs carries child-label nodes that still have a pending gate on the
// child's own run id (BUG-288 #22) — approvals are keyed by child RunID, so the
// parent ListApprovalsByRun alone cannot discover them.
func keepWaitingNodeIDsForResume(st ProviderSessionState, nodes []agentpack.FlowNode, pendingApprovals []ProviderApprovalState, pendingQuestions []ProviderQuestionState, extraNodeIDs ...string) map[string]bool {
	out := make(map[string]bool)
	hubID := hubInlineNodeID(nodes)
	hasPending := hasPendingGate(pendingApprovals, pendingQuestions)
	if hasPending && hubID != "" {
		out[hubID] = true
	}
	if active := strings.TrimSpace(st.LoopState.ActiveNode); active != "" &&
		(strings.TrimSpace(st.LoopState.Status) == "blocked" || hasPending) {
		out[active] = true
	}
	for _, id := range extraNodeIDs {
		if id = strings.TrimSpace(id); id != "" {
			out[id] = true
		}
	}
	return out
}

func hasPendingGate(pendingApprovals []ProviderApprovalState, pendingQuestions []ProviderQuestionState) bool {
	for _, a := range pendingApprovals {
		if strings.EqualFold(strings.TrimSpace(a.Status), "pending") {
			return true
		}
	}
	for _, q := range pendingQuestions {
		if strings.EqualFold(strings.TrimSpace(q.Status), "pending") {
			return true
		}
	}
	return false
}
