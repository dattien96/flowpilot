package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

// Dispatch operator attention (CP-51 Task-256). Desktop DispatchAttentionCard
// parity: uncertain turns / open repairs that need an operator decision are
// surfaced as clickable chips above the composer, and automated dispatch stays
// blocked until every item is resolved. TUI-only — the runner routes already
// exist (dispatch_operator.go); this file is the TUI surface + commands.

// hasUnresolvedAttention reports whether the current run has any operator
// attention item still needing a decision (uncertain / repair_required /
// cancel_required — NOT passive settle_pending). While true, the TUI treats the
// flow as parked (not live-running) so it neither arms [stop] nor allows a new
// turn.
func (m *AppModel) hasUnresolvedAttention() bool {
	if !m.isFlowChrome() || m.runHandle == nil {
		return false
	}
	for _, it := range m.attention {
		if strings.TrimSpace(it.RunID) != m.runHandle.RunID {
			continue
		}
		switch it.Kind {
		case "uncertain", "repair_required", "cancel_required":
			return true
		}
	}
	return false
}

// attentionKey builds a stable lookup key from a run/turn pair.
func attentionKey(runID, turnID string) string {
	return runID + "/" + turnID
}

// applyAttention stores the latest attention set for the current run, drops the
// inspect cache for items no longer present, and surfaces a warn banner when a
// decision-worthy item (uncertain / repair_required) newly appears.
func (m *AppModel) applyAttention(items []client.DispatchAttentionItem) {
	prevHadDecision := m.hasDecisionAttention()
	m.attention = items

	if m.attentionInspect != nil {
		keep := map[string]*client.DispatchInspectResult{}
		for _, it := range items {
			key := attentionKey(it.RunID, it.TurnID)
			if v, ok := m.attentionInspect[key]; ok {
				keep[key] = v
			}
		}
		m.attentionInspect = keep
	}
	m.attentionErr = ""

	if m.hasDecisionAttention() && !prevHadDecision {
		m.showAttentionBanner(items)
	}
}

// hasDecisionAttention reports whether any attention item requires an operator
// decision (uncertain or repair_required), as opposed to settle_pending which
// only offers passive "view details".
func (m *AppModel) hasDecisionAttention() bool {
	for _, it := range m.attention {
		switch it.Kind {
		case "uncertain", "repair_required", "cancel_required":
			return true
		}
	}
	return false
}

// showAttentionBanner surfaces the parked dispatch surface (Desktop
// DispatchAttentionCard parity) so the user knows automated dispatch is blocked
// on their decision.
func (m *AppModel) showAttentionBanner(items []client.DispatchAttentionItem) {
	n := len(items)
	line := fmt.Sprintf("Dispatch attention: %d item(s) need an operator decision (resolve below).", n)
	m.addMessage("system", line, "warn")
	m.connStatus = ConnWaiting
	m.statusMsg = "awaiting your dispatch decision"
}

// cmdHydrateDispatchAttention is a one-shot GET /dispatch-attention fetch used on
// flow open / graph refresh so the TUI surfaces operator attention (uncertain /
// repair_required) without waiting for a live event. Desktop poll parity.
func (m *AppModel) cmdHydrateDispatchAttention(runID string) tea.Cmd {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil
	}
	if m.attentionInFlight {
		return nil
	}
	m.attentionInFlight = true
	runnerURL := m.runnerURL
	return func() tea.Msg {
		defer func() { m.attentionInFlight = false }()
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		items, err := cl.ListDispatchAttention(ctx, runID)
		if err != nil {
			return AttentionLoadedMsg{RunID: runID, Err: err}
		}
		return AttentionLoadedMsg{RunID: runID, Items: items}
	}
}

// cmdInspectAttention fetches the full record for one turn so the user can see
// state/settle/cancelRequested before deciding (Desktop Inspect).
func (m *AppModel) cmdInspectAttention(runID, turnID string) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		res, err := cl.InspectDispatch(ctx, runID, turnID)
		return AttentionInspectedMsg{RunID: runID, TurnID: turnID, Result: res, Err: err}
	}
}

// cmdResolveAttention resolves an uncertain turn (mark_completed | mark_failed |
// confirm_cancelled | abandon). Desktop DispatchResolveAction parity.
func (m *AppModel) cmdResolveAttention(runID, turnID, action string) tea.Cmd {
	runnerURL := m.runnerURL
	resolutionID := fmt.Sprintf("tui-%d", time.Now().UnixNano())
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		var expectedRev int64
		if info := m.attentionInspect[attentionKey(runID, turnID)]; info != nil {
			expectedRev = info.Revision
		}
		_, err := cl.ResolveDispatchUncertain(ctx, runID, turnID, expectedRev, resolutionID, action, "")
		return AttentionResolvedMsg{RunID: runID, TurnID: turnID, Kind: "uncertain", Outcome: action, Err: err}
	}
}

// cmdRetryAttention retries an uncertain turn as new. On success it refreshes
// attention. The caller enforces the cancel-bias double-confirm before invoking.
func (m *AppModel) cmdRetryAttention(runID, turnID string) tea.Cmd {
	runnerURL := m.runnerURL
	resolutionID := fmt.Sprintf("tui-%d-retry", time.Now().UnixNano())
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		var info *client.DispatchInspectResult
		if v := m.attentionInspect[attentionKey(runID, turnID)]; v != nil {
			info = v
		}
		input := client.RetryAsNewInput{
			ResolutionID:         resolutionID,
			NewTurnID:            fmt.Sprintf("t%x", time.Now().UnixNano()),
			ExpectedEnvelopeHash: "",
		}
		if info != nil {
			input.ExpectedRev = info.Revision
			input.ExpectedIntentGen = info.OuterIntentGen
			input.ExpectedEnvelopeHash = info.EnvelopeHash
		}
		_, err := cl.RetryDispatchAsNew(ctx, runID, turnID, input)
		return AttentionResolvedMsg{RunID: runID, TurnID: turnID, Kind: "uncertain", Outcome: "retry-as-new", Err: err}
	}
}

// cmdResolveRepair resolves an open repair (retry_load | abandon).
func (m *AppModel) cmdResolveRepair(runID, action string) tea.Cmd {
	runnerURL := m.runnerURL
	resolutionID := fmt.Sprintf("tui-%d-repair", time.Now().UnixNano())
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		var expectedRepairRev int64
		for _, it := range m.attention {
			if it.Kind == "repair_required" && strings.TrimSpace(it.RunID) == runID {
				expectedRepairRev = 1
				break
			}
		}
		_, err := cl.ResolveDispatchRepair(ctx, runID, expectedRepairRev, resolutionID, action)
		return AttentionResolvedMsg{RunID: runID, Kind: "repair_required", Outcome: action, Err: err}
	}
}

// renderAttentionBar renders the attention surface above the composer (Desktop
// DispatchAttentionCard parity). settle_pending items only offer [details];
// uncertain items offer Inspect + Confirm cancelled / Mark completed / Mark
// failed / Retry as new / Abandon; repair_required offers Inspect + Retry load /
// Abandon repair. Chips are clickable via hitAttentionChrome.
func (m *AppModel) renderAttentionBar() string {
	if len(m.attention) == 0 {
		return ""
	}
	left := styleInputStroke.Render("┃")
	mid := styleInputStroke.Render("│")
	var lines []string
	if m.attentionErr != "" {
		lines = append(lines, left+" "+styleError.Render("dispatch: "+m.attentionErr))
	}
	for _, it := range m.attention {
		if strings.TrimSpace(it.RunID) != m.runHandle.RunID {
			continue
		}
		head := styleGate.Render("attention") + " " + styleLink.Render("["+it.Kind+"]") + " " +
			styleSystem.Render("run "+it.RunID)
		if it.TurnID != "" {
			head += styleSystem.Render(" turn " + it.TurnID)
		}
		if it.Reason != "" {
			head += styleSystem.Render(" — " + it.Reason)
		}
		lines = append(lines, left+" "+head)
		var chips []string
		switch it.Kind {
		case "settle_pending":
			chips = append(chips, styleLink.Render("[details]"))
		case "uncertain", "cancel_required":
			chips = append(chips,
				styleLink.Render("[inspect]"),
				styleLink.Render("[confirm-cancelled]"),
				styleLink.Render("[mark-completed]"),
				styleLink.Render("[mark-failed]"),
			)
			if m.attentionRetryConfirm[attentionKey(it.RunID, it.TurnID)] {
				chips = append(chips, styleLink.Render("[confirm-retry]"))
			} else {
				chips = append(chips, styleLink.Render("[retry-as-new]"))
			}
			chips = append(chips, styleLink.Render("[abandon]"))
		case "repair_required":
			chips = append(chips,
				styleLink.Render("[inspect]"),
				styleLink.Render("[retry-load]"),
				styleLink.Render("[abandon-repair]"),
			)
		}
		if info := m.attentionInspect[attentionKey(it.RunID, it.TurnID)]; info != nil {
			lines = append(lines, left+" "+styleSystem.Render(fmt.Sprintf(
				"  state=%s settle=%s cancelRequested=%v",
				info.State, info.SettlePhase, info.CancelRequested)))
		}
		lines = append(lines, left+" "+mid+" "+strings.Join(chips, "  "))
	}
	lines = append(lines, styleSystem.Render("automated dispatch is blocked until resolved"))
	return strings.Join(lines, "\n")
}

// hitAttentionChip maps a click position in the rendered attention bar to a
// command tag. It mirrors hitApprovalChrome: the click is resolved against the
// actual input-block row so x/y map to the real rendered chip positions.
func (m *AppModel) hitAttentionChip(x, y int) string {
	if m.runHandle == nil || len(m.attention) == 0 {
		return ""
	}
	c := m.tuiChrome()
	if c.inputH <= 0 || y < c.inputY || y >= c.inputY+c.inputH {
		return ""
	}
	lines := strings.Split(c.inputBlock, "\n")
	rel := y - c.inputY
	if rel < 0 || rel >= len(lines) {
		return ""
	}
	stripped := stripANSI(lines[rel])
	for _, it := range m.attention {
		if strings.TrimSpace(it.RunID) != m.runHandle.RunID {
			continue
		}
		key := attentionKey(it.RunID, it.TurnID)
		var tokens, tags []string
		switch it.Kind {
		case "settle_pending":
			tokens = append(tokens, "[details]")
			tags = append(tags, "attention-details:"+key)
		case "uncertain", "cancel_required":
			tokens = append(tokens, "[inspect]")
			tags = append(tags, "attention-inspect:"+key)
			tokens = append(tokens, "[confirm-cancelled]")
			tags = append(tags, "attention-resolve:"+key+":confirm_cancelled")
			tokens = append(tokens, "[mark-completed]")
			tags = append(tags, "attention-resolve:"+key+":mark_completed")
			tokens = append(tokens, "[mark-failed]")
			tags = append(tags, "attention-resolve:"+key+":mark_failed")
			if m.attentionRetryConfirm[key] {
				tokens = append(tokens, "[confirm-retry]")
				tags = append(tags, "attention-retry:"+key)
			} else {
				tokens = append(tokens, "[retry-as-new]")
				tags = append(tags, "attention-retry:"+key)
			}
			tokens = append(tokens, "[abandon]")
			tags = append(tags, "attention-resolve:"+key+":abandon")
		case "repair_required":
			tokens = append(tokens, "[inspect]")
			tags = append(tags, "attention-inspect:"+key)
			tokens = append(tokens, "[retry-load]")
			tags = append(tags, "attention-repair:"+it.RunID+":retry_load")
			tokens = append(tokens, "[abandon-repair]")
			tags = append(tags, "attention-repair:"+it.RunID+":abandon")
		}
		for i, token := range tokens {
			if hitToken(stripped, token, x) {
				return tags[i]
			}
		}
	}
	return ""
}
