package app

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// actionRingItem is one keyboard-selectable chip (same targets as mouse.go).
type actionRingItem struct {
	target string
	label  string
}

func (m *AppModel) hasActionRingCard() bool {
	if m.flowLoopBlocked() {
		return true
	}
	if m.approval != nil || m.question != nil || m.gate != nil {
		return true
	}
	return m.hasUnresolvedAttention()
}

func (m *AppModel) actionRingKeysActive() bool {
	if m.attachPanelOpen {
		return false
	}
	// BUG-333 (operator report: visible Approve/Deny gate, Tab/arrows dead): a
	// blocking card OWNS ←/→/Tab/Enter while the user is not typing. Passive
	// suggestion lists (history/flows/providers at empty input) must never veto
	// the gate ring — previously collectSuggestions()>0 disabled the whole ring
	// even with a card up. Typing still hands keys back to the composer so
	// "/approve"-style commands keep their pickers.
	if m.approval != nil || m.question != nil || m.gate != nil || m.hasUnresolvedAttention() {
		return strings.TrimSpace(m.inputValue) == ""
	}
	if len(m.collectSuggestions()) > 0 {
		return false
	}
	if m.actionRingFocus {
		return true
	}
	return strings.TrimSpace(m.inputValue) == "" && m.hasActionRingCard()
}

func (m *AppModel) actionRingItems() []actionRingItem {
	if m.hasUnresolvedAttention() && m.runHandle != nil {
		for _, it := range m.attention {
			if strings.TrimSpace(it.RunID) != m.runHandle.RunID {
				continue
			}
			key := attentionKey(it.RunID, it.TurnID)
			switch it.Kind {
			case "settle_pending":
				return []actionRingItem{{target: "attention-details:" + key, label: "[details]"}}
			case "uncertain", "cancel_required":
				items := []actionRingItem{
					{target: "attention-inspect:" + key, label: "[inspect]"},
					{target: "attention-resolve:" + key + ":confirm_cancelled", label: "[confirm-cancelled]"},
					{target: "attention-resolve:" + key + ":mark_completed", label: "[mark-completed]"},
					{target: "attention-resolve:" + key + ":mark_failed", label: "[mark-failed]"},
				}
				if m.attentionRetryConfirm[key] {
					items = append(items, actionRingItem{target: "attention-retry:" + key, label: "[confirm-retry]"})
				} else {
					items = append(items, actionRingItem{target: "attention-retry:" + key, label: "[retry-as-new]"})
				}
				items = append(items, actionRingItem{target: "attention-resolve:" + key + ":abandon", label: "[abandon]"})
				return items
			case "repair_required":
				return []actionRingItem{
					{target: "attention-inspect:" + key, label: "[inspect]"},
					{target: "attention-repair:" + it.RunID + ":retry_load", label: "[retry-load]"},
					{target: "attention-repair:" + it.RunID + ":abandon", label: "[abandon-repair]"},
				}
			}
		}
	}
	if m.gate != nil && len(m.gate.Options) > 0 {
		var items []actionRingItem
		for _, opt := range m.gate.Options {
			items = append(items, actionRingItem{target: "gopt:" + opt, label: gateOptionChip(opt)})
		}
		return items
	}
	if m.question != nil {
		var items []actionRingItem
		if m.question.MultiSelect {
			for i, o := range m.question.Options {
				items = append(items, actionRingItem{
					target: "qtoggle:" + strconv.Itoa(i),
					label:  questionOptionLabel(o),
				})
			}
			items = append(items, actionRingItem{target: "qsubmit", label: "[Submit]"})
			return items
		}
		for i, o := range m.question.Options {
			items = append(items, actionRingItem{
				target: "qopt:" + strconv.Itoa(i),
				label:  strconv.Itoa(i+1) + ") " + questionOptionLabel(o),
			})
		}
		return items
	}
	if m.approval != nil {
		var items []actionRingItem
		if len(m.approval.Decisions) > 0 {
			for _, d := range m.approval.Decisions {
				items = append(items, actionRingItem{target: "adec:" + d.Value, label: d.Label})
			}
		} else {
			items = append(items,
				actionRingItem{target: "approve", label: "Approve"},
				actionRingItem{target: "deny", label: "Deny"},
			)
		}
		if len(m.approvals) > 1 {
			items = append(items,
				actionRingItem{target: "approve-all", label: "Approve all"},
				actionRingItem{target: "deny-all", label: "Deny all"},
			)
		}
		if approvalRememberable(m.approval) {
			items = append(items, actionRingItem{target: "approve-forever", label: "Approve forever"})
		}
		return items
	}
	if m.flowLoopBlocked() {
		showAllow := m.blockedCardAllowShown()
		isPlanApproval := strings.EqualFold(strings.TrimSpace(m.flowBlockReason), "plan_approval")
		isVibeLock := strings.EqualFold(strings.TrimSpace(m.flowBlockReason), "vibe_lock")
		isSprintBoundary := strings.EqualFold(strings.TrimSpace(m.flowBlockReason), "vibe_sprint_boundary")
		retryLabel := "[Retry]"
		if isPlanApproval {
			retryLabel = "[Approve]"
		}
		if isVibeLock {
			retryLabel = "[Lock]"
		}
		if isSprintBoundary {
			retryLabel = "[Continue]"
		}
		items := []actionRingItem{
			{target: "retry", label: retryLabel},
			{target: "stop", label: "[Stop]"},
		}
		if showAllow {
			items = append(items, actionRingItem{target: "allow", label: "[Allow]"})
		}
		// Task-325 UX: [Revise] prefills "/continue " so feedback is
		// discoverable (appended last — Approve/Retry/Stop/Allow indices unchanged).
		items = append(items, actionRingItem{target: "revise", label: "[Revise]"})
		return items
	}
	return nil
}

func (m *AppModel) actionRingSig() string {
	var b strings.Builder
	if m.flowLoopBlocked() {
		b.WriteString("blocked:")
		b.WriteString(m.flowBlockReason)
	}
	if m.approval != nil {
		b.WriteString("|ap:" + m.approval.ID)
	}
	if m.question != nil {
		b.WriteString("|q:" + m.question.ID)
	}
	if m.gate != nil {
		b.WriteString("|g:" + m.gate.RunID + ":" + strings.Join(m.gate.Options, ","))
	}
	if m.hasUnresolvedAttention() {
		b.WriteString("|att")
	}
	return b.String()
}

func (m *AppModel) syncActionRingCard() {
	sig := m.actionRingSig()
	if sig == m.actionRingCardSig {
		return
	}
	m.actionRingCardSig = sig
	m.actionRingIdx = 0
	// BUG-362: a validate-exhausted Retry re-runs old scope, which cannot
	// fix a spec/test conflict — default Enter to Revise while the fresh
	// card is up (arrows still move; reset only fires on card change).
	if m.flowLoopBlocked() && isValidateExhaustedGate(m.blockedDecisionReason()) {
		m.actionRingIdx = m.blockedReviseRingIndex()
	}
	m.actionRingFocus = m.approval != nil || m.question != nil || m.gate != nil || m.hasUnresolvedAttention()
}

func (m *AppModel) clampActionRingIdx() {
	items := m.actionRingItems()
	if len(items) == 0 {
		m.actionRingIdx = 0
		return
	}
	if m.actionRingIdx < 0 {
		m.actionRingIdx = 0
	}
	if m.actionRingIdx >= len(items) {
		m.actionRingIdx = len(items) - 1
	}
}

func renderActionRingChip(label string, highlight bool) string {
	if highlight {
		// BUG-333 UX: selected action = filled chip (background + padding), not
		// another accent-colored text. Single choke point for every ring
		// surface: approval chips, gate options, question options/submit,
		// attention bar and flow-blocked Retry/Stop/Allow.
		return styleRingSelected.Render(" " + label + " ")
	}
	return styleLink.Render(label)
}

func (m *AppModel) ringHighlightFor(surface string) int {
	if !m.actionRingKeysActive() {
		return -1
	}
	switch surface {
	case "attention":
		if !m.hasUnresolvedAttention() || m.gate != nil || m.question != nil || m.approval != nil {
			return -1
		}
	case "gate":
		if m.gate == nil || len(m.gate.Options) == 0 || m.hasUnresolvedAttention() {
			return -1
		}
	case "question":
		if m.question == nil || m.gate != nil || m.hasUnresolvedAttention() {
			return -1
		}
	case "approval":
		if m.approval == nil || m.gate != nil || m.question != nil || m.hasUnresolvedAttention() {
			return -1
		}
	case "blocked":
		if !m.flowLoopBlocked() || m.approval != nil || m.question != nil || m.gate != nil || m.hasUnresolvedAttention() {
			return -1
		}
	default:
		return -1
	}
	return m.actionRingIdx
}

func (m *AppModel) actionRingHighlighted(surface string, idx int) bool {
	return idx == m.ringHighlightFor(surface)
}

func (m *AppModel) activateHighlightedAction() (tea.Model, tea.Cmd) {
	items := m.actionRingItems()
	if len(items) == 0 {
		return m, nil
	}
	m.clampActionRingIdx()
	return m.activateClickTarget(items[m.actionRingIdx].target)
}

func (m *AppModel) activateActionRingIndex(n1 int) (tea.Model, tea.Cmd) {
	items := m.actionRingItems()
	if n1 < 1 || n1 > len(items) {
		return m, nil
	}
	return m.activateClickTarget(items[n1-1].target)
}

func (m *AppModel) handleActionRingKey(msg tea.KeyMsg) (handled bool, model tea.Model, cmd tea.Cmd) {
	if !m.actionRingKeysActive() {
		return false, m, nil
	}
	items := m.actionRingItems()
	if len(items) == 0 {
		return false, m, nil
	}
	m.clampActionRingIdx()

	switch msg.Type {
	case tea.KeyLeft:
		m.actionRingFocus = true
		m.actionRingIdx = (m.actionRingIdx - 1 + len(items)) % len(items)
		return true, m, nil
	case tea.KeyRight:
		m.actionRingFocus = true
		m.actionRingIdx = (m.actionRingIdx + 1) % len(items)
		return true, m, nil
	case tea.KeySpace:
		if m.question != nil && m.question.MultiSelect && (m.actionRingFocus || strings.TrimSpace(m.inputValue) == "") {
			m.actionRingFocus = true
			item := items[m.actionRingIdx]
			if strings.HasPrefix(item.target, "qtoggle:") {
				model, cmd := m.activateClickTarget(item.target)
				return true, model, cmd
			}
		}
	case tea.KeyRunes:
		if len(msg.Runes) == 1 && strings.TrimSpace(m.inputValue) == "" {
			if d := msg.Runes[0]; d >= '1' && d <= '9' {
				model, cmd := m.activateActionRingIndex(int(d - '0'))
				return true, model, cmd
			}
		}
	}
	return false, m, nil
}

func (m *AppModel) actionRingEnterActivates() bool {
	if m.gate != nil && m.gate.AwaitingCustom {
		return false
	}
	if !m.hasActionRingCard() {
		return false
	}
	if m.approval != nil || m.question != nil || m.gate != nil || m.hasUnresolvedAttention() {
		return strings.TrimSpace(m.inputValue) == ""
	}
	if m.flowLoopBlocked() {
		// Live-found run-584646: Tab→Revise→Enter arms the composer, but
		// ring focus stays on — a second Enter with typed text re-fired
		// Revise (wiping the note + re-toasting) instead of submitting it.
		// Non-empty input always submits (parked plain text IS the feedback);
		// empty input keeps Enter-as-default-action. Mirrors the approval
		// pattern above.
		return strings.TrimSpace(m.inputValue) == ""
	}
	return false
}

// activateClickTarget runs the same commands as dispatchMouseClick without coordinates.
func (m *AppModel) activateClickTarget(target string) (tea.Model, tea.Cmd) {
	switch {
	case target == "session":
		// Task-311: no panel toggle — the sidebar follows terminal width.
		return m, nil
	case target == "sidebar-collapse":
		// Task-311: no collapse — width is the only switch.
		return m, nil
	case target == "skills":
		// Task-311: skills live in the sidebar, no expand toggle.
		return m, nil
	case target == "status-details":
		// Task-311: details live in the sidebar, no fold toggle.
		return m, nil
	case target == "stop":
		if m.turnIsActive() {
			return m, m.cmdStopTurn()
		}
		if m.flowLoopBlocked() {
			return m, m.cmdStopTurn()
		}
	case target == "retry", target == "continue":
		if m.flowLoopBlocked() && m.runHandle != nil {
			return m, m.cmdContinueFlow(m.runHandle.RunID)
		}
	case target == "revise":
		// Task-325 UX: parked plain text IS the feedback (no "/continue"
		// prefix needed), so [Revise] just clears the composer for the note
		// instead of prefilling a prefix — Tab here, type, Enter.
		if m.flowLoopBlocked() {
			m.inputValue = ""
			m.setInputCaret(0)
			m.mouseSel = mouseSelect{}
			return m, m.showFlashToast("type feedback note + Enter to revise the plan")
		}
	case target == "allow":
		if m.flowLoopBlocked() && m.runHandle != nil {
			isCap := strings.EqualFold(strings.TrimSpace(m.flowBlockReason), "cap")
			isStalled := strings.EqualFold(strings.TrimSpace(m.flowBlockReason), "member_stalled")
			if !isCap && !isStalled {
				if drifted := parseDriftedPaths(m.blockedDecisionReason()); len(drifted) > 0 {
					return m, m.cmdAmendFlow(m.runHandle.RunID, drifted)
				}
			}
		}
	case target == "approve":
		if m.approval != nil {
			return m.submitPendingApproval("approve")
		}
		if m.question != nil {
			return m.submitQuestionAnswer("approve")
		}
	case target == "deny":
		if m.approval != nil {
			return m.submitPendingApproval("deny")
		}
		if m.question != nil {
			return m.submitQuestionAnswer("deny")
		}
	case target == "approve-all", target == "deny-all":
		decision := "approve"
		if target == "deny-all" {
			decision = "deny"
		}
		return m.resolveAllApprovals(decision)
	case target == "approve-forever":
		if m.approval != nil {
			return m.submitPendingApprovalRemember("approve", approvalRememberable(m.approval))
		}
	case strings.HasPrefix(target, "adec:"):
		decision := strings.TrimPrefix(target, "adec:")
		if decision != "" && m.approval != nil {
			return m.submitPendingApprovalDecision(decision)
		}
	case target == "attach":
		if len(m.pendingAttach) > 0 {
			if m.attachPanelOpen {
				m.closeAttachPanel()
			} else {
				m.openAttachPanel()
			}
			return m, nil
		}
		return m, m.cmdClipboardPaste()
	case strings.HasPrefix(target, "attach-rm:"):
		idx, err := strconv.Atoi(strings.TrimPrefix(target, "attach-rm:"))
		if err == nil {
			if name, ok := m.removePendingAttachment(idx); ok {
				m.addMessage("system", fmt.Sprintf("Removed pending image: %s (%d left)", name, len(m.pendingAttach)), "")
			}
		}
		return m, nil
	case strings.HasPrefix(target, "attach-open:"):
		idx, err := strconv.Atoi(strings.TrimPrefix(target, "attach-open:"))
		if err == nil {
			return m, m.cmdOpenPendingAttachment(idx)
		}
		return m, nil
	case strings.HasPrefix(target, "qopt:"):
		idx, err := strconv.Atoi(strings.TrimPrefix(target, "qopt:"))
		if err == nil && m.question != nil && idx >= 0 && idx < len(m.question.Options) {
			return m.submitQuestionAnswer(questionOptionToken(m.question.Options[idx]))
		}
	case strings.HasPrefix(target, "gopt:"):
		opt := strings.TrimPrefix(target, "gopt:")
		if opt != "" && m.gate != nil {
			if opt == "custom" {
				return m.armGateCustom()
			}
			return m.handleGateInput(opt)
		}
	case strings.HasPrefix(target, "qtoggle:"):
		idx, err := strconv.Atoi(strings.TrimPrefix(target, "qtoggle:"))
		if err == nil && m.question != nil && idx >= 0 && idx < len(m.question.Options) {
			return m.toggleQuestionSelection(questionOptionToken(m.question.Options[idx])), nil
		}
	case target == "qsubmit":
		return m.submitQuestionSubmit()
	case strings.HasPrefix(target, "attention-inspect:"):
		runID, turnID, _ := parseAttentionKey(strings.TrimPrefix(target, "attention-inspect:"))
		if runID != "" && turnID != "" {
			return m, m.cmdInspectAttention(runID, turnID)
		}
	case strings.HasPrefix(target, "attention-resolve:"):
		rest := strings.TrimPrefix(target, "attention-resolve:")
		runID, turnID, action, _ := parseAttentionResolve(rest)
		if runID != "" && turnID != "" && action != "" {
			return m, m.cmdResolveAttention(runID, turnID, action)
		}
	case strings.HasPrefix(target, "attention-retry:"):
		runID, turnID, _ := parseAttentionKey(strings.TrimPrefix(target, "attention-retry:"))
		if runID != "" && turnID != "" {
			key := attentionKey(runID, turnID)
			if !m.attentionRetryConfirm[key] {
				m.attentionRetryConfirm = map[string]bool{key: true}
				m.addMessage("system", "Press Enter again on [confirm-retry] to retry this turn as new.", "warn")
				return m, nil
			}
			m.attentionRetryConfirm = map[string]bool{}
			return m, m.cmdRetryAttention(runID, turnID)
		}
	case strings.HasPrefix(target, "attention-repair:"):
		rest := strings.TrimPrefix(target, "attention-repair:")
		runID, action, _ := parseAttentionRepair(rest)
		if runID != "" && action != "" {
			return m, m.cmdResolveRepair(runID, action)
		}
	case strings.HasPrefix(target, "attention-details:"):
		runID, turnID, _ := parseAttentionKey(strings.TrimPrefix(target, "attention-details:"))
		if runID != "" && turnID != "" {
			return m, m.cmdInspectAttention(runID, turnID)
		}
	case strings.HasPrefix(target, "agent-open:"):
		runID := strings.TrimPrefix(target, "agent-open:")
		if runID != "" {
			return m, m.cmdFocusAgent(runID)
		}
	case target == "agent-back":
		return m, m.cmdFocusAgent(m.mainRunID())
	case strings.HasPrefix(target, "copyfence:"):
		msgIdx, fenceIdx, ok := parseCopyFenceTarget(target)
		if ok {
			return m, m.cmdCopyFence(msgIdx, fenceIdx)
		}
	case strings.HasPrefix(target, "tool-group:"):
		key := strings.TrimPrefix(target, "tool-group:")
		if key != "" {
			m.toggleToolGroup(key)
			return m, nil
		}
	case strings.HasPrefix(target, "user-prompt-expand:"):
		content := strings.TrimPrefix(target, "user-prompt-expand:")
		if content != "" {
			m.toggleUserPrompt(content)
			return m, nil
		}
	case strings.HasPrefix(target, "copy:"):
		idx, err := strconv.Atoi(strings.TrimPrefix(target, "copy:"))
		if err == nil {
			return m, m.cmdCopyMessage(idx)
		}
	case target == "load-earlier":
		return m, m.loadEarlierPrompts()
	}
	return m, nil
}
