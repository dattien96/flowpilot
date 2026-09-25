package runner

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Task-442 (CP-86 P-3): per-node REAL usage budget. maxEstPromptTokens caps
// the estimated prompt input (bytes/4 heuristic); maxUsageTokens caps what
// the node actually burned, measured on provider-reported
// TokenUsage.Total.TotalTokens — the only number comparable post-run.
//
// Exceeding is a decision point, not a bug: the in-flight turn always runs
// to completion (mid-turn kills are banned), then the engine emits a
// usage_budget_exceeded card — extend / rotate / stop. `extend` mutates the
// user's declared budget so it is never auto-selectable (CP-87 auto mode).
// `rotate` delegates to the CP-87 routing gate through usageRouter — while
// that seam is unwired the card degrades to extend/stop (typed degradation).

// usageBudgetQuestionKind marks engine-emitted usage-budget cards so
// AnswerQuestion can route the resolved choice to applyUsageBudgetAnswer.
const usageBudgetQuestionKind = "usage_budget_exceeded"

// usageBudgetRouter is the CP-87 seam: the quota/routing gate plugs in here.
// nil until CP-87 lands — rotate is then simply not offered.
type usageBudgetRouter interface {
	RotateUsageBudgetRun(runID string) error
}

// nodeUsageTokensLocked returns the node's real consumed tokens: the latest
// Total.TotalTokens per provider session, SUMMED across sessions. A new leg
// (rotation) restarts the provider's cumulative counter, so per-leg latest
// values are added — usage accounting carries across legs of the same node
// run and rotation never resets it. Caller must hold s.mu.
func (s *InteractiveService) nodeUsageTokensLocked(rs *interactiveRun) int64 {
	latest := map[string]int64{}
	for _, ev := range rs.events {
		if ev.Type == EventTokenUsageUpdated && ev.TokenUsage != nil && ev.TokenUsage.Total != nil {
			latest[ev.ProviderSessionID] = ev.TokenUsage.Total.TotalTokens
		}
	}
	var sum int64
	for _, v := range latest {
		sum += v
	}
	return sum
}

// usageBudgetPendingQuestionLocked reports whether a usage-budget card is
// already awaiting an answer for this run — one card per exceed, never a
// flood. Caller must hold s.mu.
func (s *InteractiveService) usageBudgetPendingQuestionLocked(runID string) bool {
	for _, rec := range s.questions {
		if rec == nil || rec.runID != runID || rec.kind != usageBudgetQuestionKind {
			continue
		}
		if rec.status == "pending" || rec.status == "resolving" {
			return true
		}
	}
	return false
}

// usageBudgetExtensionsLocked counts resolved usage-budget cards answered
// with extend OR rotate — both raise the cap by one profile value (rotation
// implicitly applies the same extension or the check re-fires instantly).
// The count derives from the durable question store, so it survives restart.
// Caller must hold s.mu.
func (s *InteractiveService) usageBudgetExtensionsLocked(runID string) int64 {
	var n int64
	for _, rec := range s.questions {
		if rec == nil || rec.runID != runID || rec.kind != usageBudgetQuestionKind {
			continue
		}
		if rec.status != "resolved" || len(rec.choice) == 0 {
			continue
		}
		if rec.choice[0] == "extend" || rec.choice[0] == "rotate" {
			n++
		}
	}
	return n
}

// nodeUsageBudgetFor returns the effective usage cap for the node's profile:
// maxUsageTokens + one increment per resolved extend/rotate answer.
// 0 = uncapped (no profile / no cap declared).
func (s *InteractiveService) nodeUsageBudgetFor(rs *interactiveRun) int64 {
	profile, ok := s.flowNodeProfileFor(rs)
	if !ok || profile.MaxUsageTokens <= 0 {
		return 0
	}
	base := int64(profile.MaxUsageTokens)
	s.mu.Lock()
	extensions := s.usageBudgetExtensionsLocked(rs.id)
	s.mu.Unlock()
	return base * (1 + extensions)
}

// checkUsageBudgetPostTurn is the post-turn seam (same class as
// recordDriftTelemetry): evaluated only after the turn's terminal event,
// never mid-turn. Appends the est-vs-actual audit line for every turn of a
// profiled run, then emits usage_budget_exceeded when usage > cap and no
// card is already pending.
func (s *InteractiveService) checkUsageBudgetPostTurn(rs *interactiveRun, turnID string) {
	if s == nil || rs == nil {
		return
	}
	profile, ok := s.flowNodeProfileFor(rs)
	if !ok {
		return // unprofiled node: zero behavior change
	}
	s.mu.Lock()
	used := s.nodeUsageTokensLocked(rs)
	budget := int64(profile.MaxUsageTokens) * (1 + s.usageBudgetExtensionsLocked(rs.id))
	est := int64(len(rs.lastPrompt)) / 4
	workspace, projectID, runID := "", rs.projectID, rs.id
	if s.runner != nil {
		workspace = s.runner.workspace
	}
	s.mu.Unlock()

	appendUsageAuditLine(workspace, projectID, runID, turnID, used, est, budget)

	if budget <= 0 || used <= budget {
		return
	}
	s.mu.Lock()
	if s.usageBudgetPendingQuestionLocked(rs.id) {
		s.mu.Unlock()
		return
	}
	expiresAt := time.Now().UTC().Add(s.questionTTL).Format(time.RFC3339Nano)
	options := []QuestionOption{
		{Label: "extend", Description: fmt.Sprintf("raise the usage budget by %d tokens once", profile.MaxUsageTokens)},
	}
	if s.usageRouter != nil {
		options = append(options, QuestionOption{Label: "rotate", Description: "route the node to another provider/account via the routing gate"})
	}
	options = append(options, QuestionOption{Label: "stop", Description: "escalate the node out through the flow's escalate edge"})
	rec := &questionRecord{
		id:        s.nextID("q"),
		runID:     rs.id,
		prompt:    fmt.Sprintf("usage_budget_exceeded: node burned %d real tokens, budget %d", used, budget),
		options:   options,
		status:    "pending",
		resolve:   make(chan questionResolveResult, 1),
		expiresAt: expiresAt,
		revision:  1,
		createdAt: time.Now().UTC().Format(time.RFC3339Nano),
		kind:      usageBudgetQuestionKind,
	}
	s.questions[rec.id] = rec
	rs.pendingQuestionID = rec.id
	questionID := rec.id
	snapshot := questionStateFromRecord(rec, turnID, expiresAt)
	s.mu.Unlock()

	// Persist pending BEFORE emit (BUG-288 P1-07 pattern) so restart never
	// resurrects a card that was never durably asked.
	if err := s.persistQuestion(snapshot); err != nil {
		s.mu.Lock()
		delete(s.questions, questionID)
		if rs.pendingQuestionID == questionID {
			rs.pendingQuestionID = ""
		}
		s.mu.Unlock()
		return
	}

	s.mu.Lock()
	s.emitLocked(rs, ProviderEvent{
		Type:           EventUserQuestionRequired,
		ProviderTurnID: turnID,
		QuestionID:     questionID,
		Prompt:         rec.prompt,
		Options:        options,
	})
	// CA-642: clients subscribe to the root stream — mirror the card up so a
	// child node's budget decision is answerable from the main timeline.
	if rootID := s.flowRootIDLocked(rs); rootID != "" {
		if root := s.runs[rootID]; root != nil {
			s.emitLocked(root, ProviderEvent{
				Type:       EventUserQuestionRequired,
				QuestionID: questionID,
				Prompt:     rec.prompt,
				Options:    options,
			})
		}
	}
	s.mu.Unlock()
}

// applyUsageBudgetAnswer applies a resolved usage-budget choice. Called from
// AnswerQuestion after the durable resolved-commit. `extend` needs no extra
// action — the raised cap derives from the durable choice itself. `rotate`
// delegates to the CP-87 gate. `stop` escalates the node through the
// existing flow_control path (the flow's `when: escalate` edge routes it).
func (s *InteractiveService) applyUsageBudgetAnswer(rs *interactiveRun, optionID string) {
	if s == nil || rs == nil {
		return
	}
	switch strings.TrimSpace(optionID) {
	case "rotate":
		if s.usageRouter != nil {
			if err := s.usageRouter.RotateUsageBudgetRun(rs.id); err != nil {
				log.Printf("[usage-budget] rotate delegation failed run=%s err=%v", rs.id, err)
			}
		}
	case "stop":
		if rs.parentRunID != "" {
			if _, err := s.applyFlowControl(rs.parentRunID, FlowControlInput{
				Status:  "escalate",
				Summary: "usage_budget_exceeded: user chose stop on the usage budget card",
			}); err != nil {
				log.Printf("[usage-budget] stop escalation failed run=%s err=%v", rs.id, err)
			}
		}
	}
}

// usageBudgetAutoSelectable filters the options a CP-87 auto policy may pick
// WITHOUT the user: never `extend` (it mutates the user's declared budget).
func usageBudgetAutoSelectable(options []QuestionOption) []QuestionOption {
	out := make([]QuestionOption, 0, len(options))
	for _, o := range options {
		if o.Label == "extend" {
			continue
		}
		out = append(out, o)
	}
	return out
}

// usageAuditLine is the post-turn est-vs-actual record appended to the
// per-turn prompt-context-audit jsonl (Task-442 T-4 — input for future
// est-heuristic calibration; no auto-calibration here).
type usageAuditLine struct {
	Kind              string `json:"kind"` // "usage"
	TurnID            string `json:"turn_id"`
	ActualUsageTokens *int64 `json:"actual_usage_tokens,omitempty"`
	EstPromptTokens   int64  `json:"est_prompt_tokens"`
	UsageBudget       int64  `json:"usage_budget"` // 0 = uncapped
	UsageStatus       string `json:"usage_status,omitempty"`
}

// appendUsageAuditLine writes the usage record as a second jsonl line next
// to the packer's audit report. Best-effort, same contract as
// writePromptContextAudit: no workspace → skip the file, never block a run.
func appendUsageAuditLine(toolWorkspace, projectID, runID, turnID string, actual, est, budget int64) {
	if strings.TrimSpace(toolWorkspace) == "" || strings.TrimSpace(turnID) == "" {
		return
	}
	if strings.TrimSpace(projectID) == "" {
		projectID = "unknown-project"
	}
	runDir := filepath.Join(toolWorkspace, ".flowpilot", "runs", projectID, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(runDir, "prompt-context-audit-"+turnID+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	line := usageAuditLine{
		Kind:            "usage",
		TurnID:          turnID,
		EstPromptTokens: est,
		UsageBudget:     budget,
	}
	if actual > 0 {
		line.ActualUsageTokens = &actual
	} else {
		line.UsageStatus = "no usage data"
	}
	_ = json.NewEncoder(f).Encode(line)
}
