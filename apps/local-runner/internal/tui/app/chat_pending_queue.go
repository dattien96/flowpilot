package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

// Pending approval/question cards can arrive in parallel: a single turn fans
// out several tool approvals and multiple agents can raise questions at once
// (BUG-157/BUG-158). Desktop keeps arrays (pendingApprovals/pendingQuestions);
// the TUI previously kept a single pointer and silently dropped every card but
// the last — the dropped card then blocked the run with no UI to resolve it.
//
// G1 (CA-195): approvals/questions are now queued. `approval`/`question`
// remain the HEAD (first unresolved) cards so every legacy pointer check keeps
// working unchanged; `approvals`/`questions` carry the full queue and the head
// pointer is re-derived after every mutation. Mutations must go through the
// helpers below — never set the pointers or slices directly.

// pushApproval appends a new approval card (dedup by ID) and points the head
// pointer at it when no card is pending yet. Returns true when the card is new.
func (m *AppModel) pushApproval(a ApprovalState) bool {
	for i := range m.approvals {
		if m.approvals[i].ID == a.ID {
			return false
		}
	}
	m.approvals = append(m.approvals, a)
	if m.approval == nil {
		h := a
		m.approval = &h
	}
	return true
}

// pushQuestion appends a new question card (dedup by ID) and points the head
// pointer at it when no card is pending yet. Returns true when the card is new.
func (m *AppModel) pushQuestion(q QuestionState) bool {
	for i := range m.questions {
		if m.questions[i].ID == q.ID {
			return false
		}
	}
	m.questions = append(m.questions, q)
	if m.question == nil {
		h := q
		m.question = &h
	}
	return true
}

// removeApproval drops the card with the given ID (all cards when ID is empty)
// and re-derives the head pointer.
func (m *AppModel) removeApproval(id string) {
	if id == "" {
		m.approvals = nil
		m.approval = nil
		return
	}
	for i := range m.approvals {
		if m.approvals[i].ID == id {
			m.approvals = append(m.approvals[:i], m.approvals[i+1:]...)
			break
		}
	}
	m.syncApprovalHead()
}

// removeQuestion drops the card with the given ID (all cards when ID is empty)
// and re-derives the head pointer.
func (m *AppModel) removeQuestion(id string) {
	if id == "" {
		m.questions = nil
		m.question = nil
		return
	}
	for i := range m.questions {
		if m.questions[i].ID == id {
			m.questions = append(m.questions[:i], m.questions[i+1:]...)
			break
		}
	}
	m.syncQuestionHead()
}

// syncApprovalHead re-derives the head pointer from the queue (a copy, so later
// slice mutations never corrupt it). Used after every queue mutation.
func (m *AppModel) syncApprovalHead() {
	if len(m.approvals) == 0 {
		m.approval = nil
		return
	}
	h := m.approvals[0]
	m.approval = &h
}

// syncQuestionHead re-derives the head pointer from the queue (a copy, so later
// slice mutations never corrupt it). Used after every queue mutation.
func (m *AppModel) syncQuestionHead() {
	if len(m.questions) == 0 {
		m.question = nil
		return
	}
	h := m.questions[0]
	m.question = &h
}

// pendingApprovalShown reports whether the given ID is the visible head card
// (empty ID means "any approval is pending").
func (m *AppModel) pendingApprovalShown(id string) bool {
	if m.approval == nil {
		return false
	}
	if id == "" {
		return true
	}
	return m.approval.ID == id
}

// pendingQuestionShown reports whether the given ID is the visible head card
// (empty ID means "any question is pending").
func (m *AppModel) pendingQuestionShown(id string) bool {
	if m.question == nil {
		return false
	}
	if id == "" {
		return true
	}
	return m.question.ID == id
}

// clearPendingDecisions drops every pending approval and question card.
func (m *AppModel) clearPendingDecisions() {
	m.approvals = nil
	m.approval = nil
	m.questions = nil
	m.question = nil
}

// resolveAllApprovals resolves every pending approval with the given decision
// (Desktop /approve-all /deny-all parity). Returns a batch cmd so each card is
// submitted and removed independently.
func (m *AppModel) resolveAllApprovals(decision string) (tea.Model, tea.Cmd) {
	if len(m.approvals) == 0 {
		m.addMessage("system", "No pending approval.", "")
		return m, nil
	}
	cmds := make([]tea.Cmd, 0, len(m.approvals))
	for _, a := range m.approvals {
		cmds = append(cmds, m.cmdApprove(a.ID, decision))
	}
	return m, tea.Batch(cmds...)
}

// approvalCommandOperators mirror the runner's approvalCommandOperators
// (BUG-246): a compound command can never be remembered.
var approvalCommandOperators = []string{"&&", "||", "|", ";", "&", ">", "<", "`", "$(", "(", ")", "{", "}", "\n", "\r"}

func isCompoundCommand(command string) bool {
	for _, op := range approvalCommandOperators {
		if strings.Contains(command, op) {
			return true
		}
	}
	return false
}

// approvalRememberable reports whether the "don't ask again" path may be
// offered for the head card: only "exec" shell commands that are single
// (non-compound) and that offer an approve decision (BUG-246 desktop parity —
// a deny is never persisted).
func approvalRememberable(a *ApprovalState) bool {
	if a == nil {
		return false
	}
	if a.Kind != "exec" || strings.TrimSpace(a.Command) == "" {
		return false
	}
	if isCompoundCommand(a.Command) {
		return false
	}
	if len(a.Decisions) > 0 {
		for _, d := range a.Decisions {
			if d.Value == "approve" {
				return true
			}
		}
		return false
	}
	return true
}

// approvalDecisionChips renders the decision buttons for the head approval.
// The runner's offered decisions win; the legacy Approve/Deny fallback covers
// cards without details.
func approvalDecisionChips(a *ApprovalState) string {
	var chips []string
	if a != nil && len(a.Decisions) > 0 {
		for _, d := range a.Decisions {
			chips = append(chips, styleLink.Render(d.Label))
		}
	} else {
		chips = append(chips, styleLink.Render("Approve"), styleLink.Render("Deny"))
	}
	if approvalRememberable(a) {
		chips = append(chips, styleLink.Render("Approve forever"))
	}
	return strings.Join(chips, "  ")
}

// approvalStateFromInfo builds a queued ApprovalState from a run-snapshot
// ApprovalInfo, lifting the typed BUG-246 detail fields out of the Details map
// so resume/reopen renders the same command/kind/decisions as a live card.
func approvalStateFromInfo(id, runID string, info *client.ApprovalInfo) ApprovalState {
	st := ApprovalState{ID: id, RunID: runID}
	if info == nil || info.Details == nil {
		return st
	}
	st.Details = info.Details
	if s, ok := info.Details["command"].(string); ok {
		st.Command = s
	}
	if s, ok := info.Details["cwd"].(string); ok {
		st.Cwd = s
	}
	if s, ok := info.Details["reason"].(string); ok {
		st.Reason = s
	}
	if s, ok := info.Details["kind"].(string); ok {
		st.Kind = s
	}
	if arr, ok := info.Details["decisions"].([]any); ok {
		for _, item := range arr {
			mm, ok := item.(map[string]any)
			if !ok {
				continue
			}
			v, _ := mm["value"].(string)
			l, _ := mm["label"].(string)
			st.Decisions = append(st.Decisions, client.ApprovalDecisionOption{Value: v, Label: l})
		}
	}
	return st
}
