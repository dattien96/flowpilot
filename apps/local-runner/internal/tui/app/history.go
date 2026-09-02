package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

// ChatListMsg carries /history listing results.
type ChatListMsg struct {
	Items  []client.RunHistoryItem
	Err    string
	Silent bool // cache only — used while typing `/history `
}

// ChatOpenedMsg carries a resumed chat with replayed transcript.
type ChatOpenedMsg struct {
	Handle                client.RunHandle
	Messages              []ChatMessage
	HistoryLoadedAfterSeq int64 // events with seq <= this are not yet loaded; 0 = full history
	Snapshot              client.RunSnapshot
	// HistoryMeta is the list row for this run when known (kind/workflow/flowRef).
	HistoryMeta client.RunHistoryItem
	// TokenUsage is the last token_usage_updated snapshot replayed for this run
	// (nil when the run never emitted one). Seeds the per-run ctx/token status
	// line so /open of chat B never shows chat A's usage (CA-540).
	TokenUsage *client.TokenUsageSnapshot
	Err        string
}

// applyOpenedRunFlowChrome restores ModeFlow/launch + clears stale steps when
// opening a workflow/catalog flow run (CA-502). Chat opens stay ModeChat.
func (m *AppModel) applyOpenedRunFlowChrome(handle client.RunHandle, meta client.RunHistoryItem) {
	runKind := strings.TrimSpace(handle.RunKind)
	if runKind == "" {
		runKind = strings.TrimSpace(meta.RunKind)
	}
	workflowID := strings.TrimSpace(handle.WorkflowID)
	if workflowID == "" {
		workflowID = strings.TrimSpace(meta.WorkflowID)
	}
	flowRef := strings.TrimSpace(handle.FlowRef)
	if flowRef == "" {
		flowRef = strings.TrimSpace(meta.FlowRef)
	}
	isFlow := strings.EqualFold(runKind, "workflow") || workflowID != "" || flowRef != ""
	if !isFlow {
		m.mode = ModeChat
		m.launch = LaunchArm{}
		m.flowSteps = nil
		m.flowStepsActive = ""
		m.agentRuns = nil
		m.focusedAgentIdx = 0
		return
	}
	m.mode = ModeFlow
	label := m.resolveFlowDisplayName(workflowID, flowRef)
	if label == "" {
		label = "flow"
	}
	m.launch = LaunchArm{
		Mode:       ModeFlow,
		WorkflowID: workflowID,
		FlowRef:    flowRef,
		Label:      label,
	}
	// Steps / agents refilled after open (cmdRefreshStepsRuntime + cmdHydrateAgentRuns).
	m.flowSteps = nil
	m.flowStepsActive = ""
	m.agentRuns = nil
	m.focusedAgentIdx = 0
	// Do not persist here — /open is run chrome for this session only.
	// Mode/flow prefs are owned by /flow and /chat (and explicit settings).
}

// resolveFlowDisplayName prefers catalog/builtin human names over raw UUIDs.
func (m *AppModel) resolveFlowDisplayName(workflowID, flowRef string) string {
	wid := strings.TrimSpace(workflowID)
	ref := strings.TrimSpace(flowRef)
	for _, wf := range m.flowWorkflows {
		if (wid != "" && wf.ID == wid) || (ref != "" && wf.ID == ref) {
			if n := strings.TrimSpace(wf.Name); n != "" {
				return n
			}
		}
	}
	for _, b := range m.flowBuiltins {
		if ref != "" && strings.EqualFold(strings.TrimSpace(b.FlowRef), ref) {
			if n := strings.TrimSpace(b.Label); n != "" {
				return n
			}
		}
	}
	// Prefer non-UUID-looking tokens for status (never show bare 8-char id alone).
	if ref != "" && !looksLikeUUID(ref) {
		return ref
	}
	if wid != "" && !looksLikeUUID(wid) {
		return wid
	}
	return ""
}

func looksLikeUUID(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 32 {
		return false
	}
	hyphen := 0
	for _, r := range s {
		if r == '-' {
			hyphen++
			continue
		}
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return hyphen >= 4 || len(s) >= 32
}

// runSnapshotMsg is GET /client/workflow-runs/{id} used to hydrate a live
// pending approval/question after the user turn stream closes (run-97624).
type runSnapshotMsg struct {
	Snap client.RunSnapshot
	Err  string
}

// HistoryChunkMsg carries an older SSE chunk prepended on Load earlier (Task-290 Q-1).
type HistoryChunkMsg struct {
	RunID             string
	Messages          []ChatMessage
	NewLoadedAfterSeq int64
	Err               string
}

// filterParentHistory keeps top-level runs (no parent) for the switcher list.
func filterParentHistory(items []client.RunHistoryItem) []client.RunHistoryItem {
	out := make([]client.RunHistoryItem, 0, len(items))
	for _, it := range items {
		if strings.TrimSpace(it.ParentRunID) != "" {
			continue
		}
		out = append(out, it)
	}
	return out
}

func formatChatList(items []client.RunHistoryItem) string {
	return formatChatListWithRemote(items, nil)
}

// formatChatListWithRemote is formatChatList with Drive-index reconciliation
// (CA-552): a row that only matches the confirmed Drive index still gets its
// (synced) badge even when the runner's local syncStatus is stale/empty.
func formatChatListWithRemote(items []client.RunHistoryItem, remote []client.RemoteChatSessionSummary) string {
	var sb strings.Builder
	sb.WriteString("Recent chats (Desktop history parity):\n")
	if len(items) == 0 {
		sb.WriteString("  (none for this project)\n")
		sb.WriteString("Usage: /history|/open|/resume  (then ↑↓ Tab Enter)")
		return sb.String()
	}
	rows := groupRunsByChatId(items)
	// Dump stays short; the live picker (command + space) scrolls through ALL items.
	limit := 20
	if len(rows) < limit {
		limit = len(rows)
	}
	for i := 0; i < limit; i++ {
		row := rows[i]
		it := row.item
		title := strings.TrimSpace(it.LastPrompt)
		if title == "" {
			title = strings.TrimSpace(it.LastMessage)
		}
		if title == "" {
			title = "(no prompt)"
		}
		title = collapseWS(title)
		if len([]rune(title)) > 56 {
			r := []rune(title)
			title = string(r[:53]) + "…"
		}
		kind := it.RunKind
		if kind == "" {
			if it.WorkflowID != "" {
				kind = "workflow"
			} else {
				kind = "chat"
			}
		}
		when := formatHistoryChangedAt(it)
		if when == "" {
			when = "—"
		}
		line := fmt.Sprintf("  %2d  %s  [%s] %s", i+1, shortID(it.RunID), kind, it.Status)
		if row.group != nil && len(row.group.legs) > 1 {
			line += fmt.Sprintf(" · %d legs", len(row.group.legs))
		}
		if badge := syncBadgeWithRemote(it, remote); badge != "" {
			line += " (" + badge + ")"
		}
		line += "  " + when + " · " + title
		sb.WriteString(line + "\n")
		if row.group != nil && len(row.group.legs) > 1 && strings.TrimSpace(it.ChatID) != "" {
			sb.WriteString(fmt.Sprintf("      id %s  %s  chat %s\n", it.RunID, it.ProviderKey, it.ChatID))
		} else {
			sb.WriteString(fmt.Sprintf("      id %s  %s\n", it.RunID, it.ProviderKey))
		}
	}
	if len(rows) > limit {
		sb.WriteString(fmt.Sprintf("  … %d more in dump — type /history  and ↑↓ to reach every chat (%d total)\n", len(rows)-limit, len(rows)))
	}
	sb.WriteString("Pick: /history|/open|/resume  then ↑↓ · Tab · Enter (picker scrolls past this dump)")
	return sb.String()
}

func collapseWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// formatSyncBadge renders the Drive chat-session sync marker for a history row
// (Desktop Navigator syncStatus parity, CA-548). Empty when the run was never
// marked, so local-first rows stay visually unchanged.
func formatSyncBadge(it client.RunHistoryItem) string {
	switch strings.TrimSpace(it.SyncStatus) {
	case "synced":
		return "synced"
	case "failed":
		return "failed"
	case "unsyncable":
		return "unsyncable"
	case "syncing":
		return "syncing"
	default:
		return ""
	}
}

// syncBadgeWithRemote is formatSyncBadge plus Drive-index reconciliation
// (CA-552). The runner's syncStatus is written to its local store asynchronously
// and can lag (or be lost across a restart), so a row that already appears in
// the confirmed Drive index is treated as synced. A local marker always wins
// (failed stays failed until the next successful sync).
func syncBadgeWithRemote(it client.RunHistoryItem, remote []client.RemoteChatSessionSummary) string {
	if badge := formatSyncBadge(it); badge != "" {
		return badge
	}
	if len(remote) == 0 {
		return ""
	}
	if strings.TrimSpace(it.SourceMachineID) != "" && strings.TrimSpace(it.SourceRunID) != "" {
		for _, r := range remote {
			if strings.TrimSpace(r.SourceMachineID) == strings.TrimSpace(it.SourceMachineID) &&
				strings.TrimSpace(r.SourceRunID) == strings.TrimSpace(it.SourceRunID) {
				return "synced"
			}
		}
		return ""
	}
	runID := strings.TrimSpace(it.RunID)
	if runID == "" {
		return ""
	}
	for _, r := range remote {
		if strings.TrimSpace(r.SourceRunID) == runID {
			return "synced"
		}
	}
	return ""
}

// mergeChatListSyncStatus carries known local sync markers forward across a
// fresh runner fetch (Desktop parity, navigatorHistory.ts:19-24): the runner's
// store write is async, so a prefetch/poll can race it and return syncStatus=""
// right after a sync. A fresh non-empty status always wins.
func mergeChatListSyncStatus(cached, fresh []client.RunHistoryItem) []client.RunHistoryItem {
	known := make(map[string]string, len(cached))
	for _, it := range cached {
		if s := strings.TrimSpace(it.SyncStatus); s != "" {
			known[it.RunID] = s
		}
	}
	if len(known) == 0 {
		return fresh
	}
	out := make([]client.RunHistoryItem, len(fresh))
	for i, it := range fresh {
		out[i] = it
		if strings.TrimSpace(it.SyncStatus) == "" {
			if s, ok := known[it.RunID]; ok {
				out[i].SyncStatus = s
			}
		}
	}
	return out
}

// resolveChatOpenTarget maps /history|/open|/resume args to a run id using the last list.
// Grouped chats (CP-59 Q-4): numeric index refers to the grouped chat row
// (latest leg is the face), and a chatId or any leg's runId resolves to the
// head runId so the reopen always lands on the latest leg.
func resolveChatOpenTarget(args []string, listed []client.RunHistoryItem) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: /history|/open|/resume <n|runId> — type the command + space for the picker")
	}
	token := strings.TrimSpace(args[0])
	rows := groupRunsByChatId(listed)
	if n, err := strconv.Atoi(token); err == nil {
		if n < 1 || n > len(rows) {
			return "", fmt.Errorf("chat index %d out of range (1-%d) — type /history  for the picker", n, len(rows))
		}
		return rows[n-1].item.RunID, nil
	}
	lower := strings.ToLower(token)
	for _, row := range rows {
		headID := strings.TrimSpace(row.item.RunID)
		if strings.EqualFold(headID, token) || strings.EqualFold(strings.TrimSpace(row.item.ChatID), token) {
			return headID, nil
		}
		if row.group != nil {
			// ChatId match already checked on head; also allow any leg's runId
			// to resolve to the head (stable reopen-by-chat).
			for _, leg := range row.group.legs {
				if strings.EqualFold(strings.TrimSpace(leg.RunID), token) {
					return headID, nil
				}
			}
			// Also allow chatId match via lower (already head) but keep for legs
			// chatId is same across legs.
			if strings.TrimSpace(row.item.ChatID) != "" && strings.ToLower(strings.TrimSpace(row.item.ChatID)) == lower {
				return headID, nil
			}
		}
		// Legacy: direct runId match for ungrouped rows already handled via head check.
	}
	return token, nil
}

// formatOpenChatErr explains runner resume failures (Desktop openHistoryRun parity).
func formatOpenChatErr(err error) string {
	return formatOpenChatErrDetailed(err, "", "")
}

func formatOpenChatErrDetailed(err error, chatProvider, activeAccountLabel string) string {
	var api *client.APIError
	if errors.As(err, &api) {
		switch api.Code {
		case "session_unavailable":
			var sb strings.Builder
			sb.WriteString("Open failed (runner session_unavailable): " + api.Message)
			sb.WriteString("\nThis is a runner/session issue — not a TUI bug. Provider session files for this run are missing on this machine (or the wrong account is active). Same limit as Desktop history open.")
			if chatProvider != "" {
				sb.WriteString("\nChat provider: " + chatProvider)
			}
			if activeAccountLabel != "" {
				sb.WriteString(fmt.Sprintf("\nActive %s account now: %s", orDash(chatProvider), activeAccountLabel))
			}
			sb.WriteString("\nNew chats can still work on the current account. Try Desktop → Settings → AI Providers → activate the account that owned this chat, then reopen — or open it on the machine where it was created.")
			return sb.String()
		case "account_not_signed_in":
			return "Open failed (runner account_not_signed_in): " + api.Message +
				"\nSign in to the provider account that owns this chat, then retry."
		case "account_unavailable":
			return "Open failed (runner account_unavailable): " + api.Message
		default:
			return fmt.Sprintf("Open failed (runner %s): %s", api.Code, api.Message)
		}
	}
	return "Open chat failed: " + err.Error()
}

func replayHistoryMessages(evs []client.ProviderEvent) []ChatMessage {
	var out []ChatMessage
	skipNextAssistant := false
	for _, ev := range evs {
		switch ev.Type {
		case "turn_started":
			if p := strings.TrimSpace(ev.Prompt); p != "" {
				// CP-59 I3: handoff seed envelope (current-leg first turn after a
				// switch) must not render as a raw user bubble — its divider
				// comes from the E-9 chat_provider_switch record. Drop the seed
				// prompt and its immediate assistant reply; live switches collapse
				// via addMessage's lastSwitchStats path.
				if strings.HasPrefix(p, client.HandoffPromptPrefix) {
					skipNextAssistant = true
					continue
				}
				out = append(out, ChatMessage{Role: "user", Content: p})
				skipNextAssistant = false // a real prompt resets the seed skip
			} else {
				// BUG-347: the switch seed's turn_started records with an empty
				// prompt (the envelope attaches asynchronously) — treat it as a
				// seed and drop its envelope reply too.
				skipNextAssistant = true
				continue
			}
		case "message_delta":
			if skipNextAssistant {
				continue
			}
			if ev.Text == "" {
				continue
			}
			if len(out) > 0 && out[len(out)-1].Role == "assistant" {
				out[len(out)-1].Content += ev.Text
			} else {
				out = append(out, ChatMessage{Role: "assistant", Content: ev.Text})
			}
		case "message_completed":
			if skipNextAssistant {
				skipNextAssistant = false
				continue
			}
			out = applyReplayAssistantText(out, ev.Text)
		case "turn_completed":
			if skipNextAssistant {
				skipNextAssistant = false
				continue
			}
			out = applyReplayAssistantText(out, ev.FinalMessage)
		}
	}
	return out
}

// applyReplayAssistantText merges a later assistant frame into the last bubble.
// Grok (and Claude/Codex JSONL seed) emit one message_completed before a tool
// and another after — run-97624 dropped "Đã tạo file…" on /open because the
// second completed was ignored once the first bubble was non-empty. Never
// replace richer text with a step-complete stub (CA-475 / chooseAssistantFinal).
func applyReplayAssistantText(out []ChatMessage, next string) []ChatMessage {
	if isStepCompleteStub(next) || strings.TrimSpace(next) == "" {
		return out
	}
	if len(out) == 0 || out[len(out)-1].Role != "assistant" {
		return append(out, ChatMessage{Role: "assistant", Content: next})
	}
	out[len(out)-1].Content = mergeReplayAssistant(out[len(out)-1].Content, next)
	return out
}

func mergeReplayAssistant(cur, next string) string {
	c := strings.TrimSpace(cur)
	n := strings.TrimSpace(next)
	if n == "" || isStepCompleteStub(n) {
		return cur
	}
	if c == "" {
		return next
	}
	if strings.Contains(c, n) {
		return cur
	}
	if strings.Contains(n, c) {
		return next
	}
	return strings.TrimRight(cur, "\n") + "\n" + next
}

// lastReplayTokenUsage returns the last token_usage_updated snapshot from the
// replayed events (the runner persists these for all providers), or nil when the
// run never emitted one. Seeding the status line from this on /open makes ctx/
// token usage per-run instead of leaking the previously-opened chat's numbers
// (CA-540).
func lastReplayTokenUsage(evs []client.ProviderEvent) *client.TokenUsageSnapshot {
	var last *client.TokenUsageSnapshot
	for _, ev := range evs {
		if ev.Type == "token_usage_updated" && ev.TokenUsage != nil {
			last = ev.TokenUsage
		}
	}
	return last
}

func (m *AppModel) cmdListChats() tea.Cmd {
	return m.cmdFetchChats(false)
}

func (m *AppModel) cmdPrefetchChats() tea.Cmd {
	return m.cmdFetchChats(true)
}

func (m *AppModel) cmdFetchChats(silent bool) tea.Cmd {
	runnerURL := m.runnerURL
	projectID := ""
	if m.project != nil {
		projectID = m.project.ID
	}
	return func() tea.Msg {
		if projectID == "" {
			return ChatListMsg{Err: "project_id required — wait for session load or set --project", Silent: silent}
		}
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		items, err := cl.ListRunHistory(ctx, projectID)
		if err != nil {
			return ChatListMsg{Err: err.Error(), Silent: silent}
		}
		return ChatListMsg{Items: filterParentHistory(items), Silent: silent}
	}
}

func (m *AppModel) cmdMaybePrefetchHistory() tea.Cmd {
	line := m.slashSuggestLine()
	trimmed := strings.TrimSpace(line)
	_, _, argOK := parseChatOpenArgPrefix(line)
	bare := false
	for _, cmd := range chatOpenSlashCommands {
		if strings.EqualFold(trimmed, cmd) {
			bare = true
			break
		}
	}
	syncBare := strings.EqualFold(trimmed, "/sync") || strings.EqualFold(trimmed, "/sync all")
	syncArg, _ := parseSlashArgPrefix(line, "/sync")
	restoreBare := strings.EqualFold(trimmed, "/restore") || strings.EqualFold(trimmed, "/restore all")
	restoreArg, _ := parseSlashArgPrefix(line, "/restore")
	deleteBare := strings.EqualFold(trimmed, "/delete")
	deleteArg, _ := parseDeleteArgPrefix(line)
	if !argOK && !bare && !syncBare && !syncArg && !restoreBare && !restoreArg && !deleteBare && !deleteArg {
		return nil
	}
	// Reconcile badges against the confirmed Drive index (CA-552), so the remote
	// index is fresh when the picker first opens; the batch /restore and /sync
	// completion paths also refresh it afterwards.
	var cmds []tea.Cmd
	if len(m.chatList) == 0 {
		// First open of any history/sync/restore picker: fetch both the local
		// chat list and the remote index (CA-552 reconcile needs the remote list).
		cmds = append(cmds, m.cmdPrefetchChats(), m.cmdPrefetchRemoteChats())
	} else if (restoreBare || restoreArg) && len(m.remoteChatList) == 0 {
		// CA-554: the /restore picker lists Drive-backed chats one at a time, so
		// the remote index must prefetch for a restore command even when the local
		// chatList is already cached (the old single gate returned nil as soon as
		// chatList was populated, which made the /restore Tab picker never appear).
		cmds = append(cmds, m.cmdPrefetchRemoteChats())
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m *AppModel) cmdOpenChat(runID string) tea.Cmd {
	runnerURL := m.runnerURL
	chatProvider := ""
	var historyMeta client.RunHistoryItem
	for _, it := range m.chatList {
		if it.RunID == runID {
			chatProvider = it.ProviderKey
			historyMeta = it
			break
		}
	}
	activeLabel := ""
	if chatProvider != "" {
		for _, a := range m.providerAccounts {
			if strings.EqualFold(a.ProviderKey, chatProvider) && a.IsActive {
				activeLabel = strings.TrimSpace(a.DisplayLabel)
				break
			}
		}
	} else {
		activeLabel = m.activeProviderAccountLabel()
	}
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		handle, err := cl.ResumeRun(ctx, runID)
		if err != nil {
			return ChatOpenedMsg{Err: formatOpenChatErrDetailed(err, chatProvider, activeLabel)}
		}
		// Tail replay through lastEventSeq (Task-290 Q-1); older chunks on Load earlier.
		until := handle.LastEventSeq
		after := chatReplayTailAfterSeq(until)
		var collected []client.ProviderEvent
		if until > 0 {
			collected = collectReplayEvents(ctx, cl, runID, after, until, chatReplayMaxEvents)
		}
		trimmed := trimEventsFromTurnStart(collected)
		msgs := replayHistoryMessages(trimmed)
		// Server snapshot is ground truth for a still-pending gate (CA-089 twin:
		// do not infer live Approve from replayed permission_required events).
		snap, _ := cl.GetRun(ctx, runID)
		return ChatOpenedMsg{
			Handle:                handle,
			Messages:              msgs,
			HistoryLoadedAfterSeq: historyCursorAfterReplay(after, collected, trimmed),
			Snapshot:              snap,
			HistoryMeta:           historyMeta,
			TokenUsage:            lastReplayTokenUsage(collected),
		}
	}
}

func snapshotStatusWaiting(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "waiting_approval", "waiting_question":
		return true
	default:
		return false
	}
}

// applyPendingFromSnapshot mounts a live approval/question only when the
// runner still reports waiting_* plus a pending card. Completed runs with
// historical permission_required events stay read-only (CA-089).
func (m *AppModel) applyPendingFromSnapshot(snap client.RunSnapshot) tea.Cmd {
	if !snapshotStatusWaiting(snap.Status) {
		return nil
	}
	runID := strings.TrimSpace(snap.RunID)
	if runID == "" && m.runHandle != nil {
		runID = m.runHandle.RunID
	}
	if snap.PendingApproval != nil && strings.TrimSpace(snap.PendingApproval.ID) != "" {
		id := strings.TrimSpace(snap.PendingApproval.ID)
		if m.effectiveYolo() {
			m.connStatus = ConnRunning
			m.statusMsg = "auto-approved"
			return m.cmdAutoApprove(id, runID)
		}
		if m.approval != nil && m.approval.ID == id {
			m.connStatus = ConnWaiting
			m.statusMsg = "approval required"
			return nil
		}
		if m.pushApproval(approvalStateFromInfo(id, runID, snap.PendingApproval)) {
			m.connStatus = ConnWaiting
			m.statusMsg = "approval required"
			m.addMessage("system", formatApprovalWaitingLineDetailed(id, m.asciiMode, *m.approval), "approval")
		}
		return nil
	}
	if snap.PendingQuestion != nil && strings.TrimSpace(snap.PendingQuestion.ID) != "" {
		id := strings.TrimSpace(snap.PendingQuestion.ID)
		if m.question != nil && m.question.ID == id {
			m.connStatus = ConnWaiting
			m.statusMsg = "question"
			return nil
		}
		if m.pushQuestion(QuestionState{
			ID:          id,
			Prompt:      snap.PendingQuestion.Prompt,
			Options:     snap.PendingQuestion.Options,
			MultiSelect: snap.PendingQuestion.MultiSelect,
			RunID:       runID,
		}) {
			m.connStatus = ConnWaiting
			m.statusMsg = "question"
			m.addMessage("system", formatQuestionMessage(snap.PendingQuestion.Prompt, snap.PendingQuestion.Options, snap.PendingQuestion.MultiSelect), "question")
		}
	}
	return nil
}

func (m *AppModel) cmdHydratePendingFromSnapshot() tea.Cmd {
	if m.runHandle == nil {
		return nil
	}
	runID := m.runHandle.RunID
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		snap, err := cl.GetRun(ctx, runID)
		if err != nil {
			return runSnapshotMsg{Err: err.Error()}
		}
		return runSnapshotMsg{Snap: snap}
	}
}

// ChatDeletedMsg carries the result of DELETE /client/workflow-runs/{runId} (Task-318).
type ChatDeletedMsg struct {
	RunID string
	Err   string
}

func (m *AppModel) cmdDeleteChat(runID string) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := cl.DeleteRun(ctx, runID); err != nil {
			return ChatDeletedMsg{RunID: runID, Err: err.Error()}
		}
		return ChatDeletedMsg{RunID: runID}
	}
}

func (m *AppModel) handleChatDeleted(msg ChatDeletedMsg) (tea.Model, tea.Cmd) {
	runID := strings.TrimSpace(msg.RunID)
	// Clear pending confirm regardless of outcome.
	m.deletePendingRunID = ""
	m.deletePendingIDs = nil
	m.deletePendingLabel = ""
	if strings.TrimSpace(msg.Err) != "" {
		m.addMessage("system", fmt.Sprintf("Delete failed for %s: %s", orDash(runID), msg.Err), "error")
		// Continue batch on error — dispatch next if queued
		if len(m.deleteBatchQueue) > 0 {
			next := m.deleteBatchQueue[0]
			m.deleteBatchQueue = m.deleteBatchQueue[1:]
			return m, m.cmdDeleteChat(next)
		}
		if m.deleteBatchTotal > 1 && len(m.deleteBatchQueue) == 0 {
			m.addMessage("system", fmt.Sprintf("Delete batch finished (%d total, with errors).", m.deleteBatchTotal), "")
			m.deleteBatchQueue = nil
			m.deleteBatchTotal = 0
			m.deleteSelected = nil
		}
		return m, nil
	}
	// Optimistically drop from local list.
	if len(m.chatList) > 0 {
		kept := make([]client.RunHistoryItem, 0, len(m.chatList))
		for _, it := range m.chatList {
			if strings.TrimSpace(it.RunID) != runID {
				kept = append(kept, it)
			}
		}
		m.chatList = kept
	}
	// Remove from tick set if present
	if m.deleteSelected != nil {
		delete(m.deleteSelected, runID)
	}
	openID := ""
	if m.runHandle != nil {
		openID = strings.TrimSpace(m.runHandle.RunID)
	}
	isBatch := m.deleteBatchTotal > 1 || len(m.deleteBatchQueue) > 0
	if runID != "" && runID == openID {
		// Current chat deleted — reset like /new / Desktop BUG-258.
		m.stopOrchestrationStream()
		m.turnStream = nil
		m.runHandle = nil
		m.stepID = ""
		m.pendingPrompt = ""
		m.messages = nil
		m.visiblePromptCount = 0
		m.historyLoadedAfterSeq = 0
		m.mainHistoryLoadedAfterSeq = 0
		m.historyChunkInFlight = false
		m.viewport.offset = 0
		m.connStatus = ConnIdle
		m.statusMsg = "ready"
		m.lastEventSeq = 0
		m.lastTurnError = ""
		m.lastTokens = nil
		m.gate = nil
		m.clearPendingDecisions()
		m.refreshSessionPanel()
		if isBatch {
			m.addMessage("system", fmt.Sprintf("Deleted current chat %s (batch).", orDash(runID)), "")
		} else {
			m.addMessage("system", fmt.Sprintf("Deleted current chat %s — started new conversation.", orDash(runID)), "")
		}
	} else {
		if !isBatch {
			m.addMessage("system", fmt.Sprintf("Deleted chat %s.", orDash(runID)), "")
		}
	}
	// Batch: dispatch next or finish
	if len(m.deleteBatchQueue) > 0 {
		next := m.deleteBatchQueue[0]
		m.deleteBatchQueue = m.deleteBatchQueue[1:]
		return m, m.cmdDeleteChat(next)
	}
	if m.deleteBatchTotal > 1 {
		m.addMessage("system", fmt.Sprintf("Deleted %d chats.", m.deleteBatchTotal), "")
		m.deleteBatchTotal = 0
		m.deleteBatchQueue = nil
		m.deleteSelected = nil
	} else if m.deleteBatchTotal == 1 {
		m.deleteBatchTotal = 0
		m.deleteBatchQueue = nil
	}
	if len(m.deleteBatchQueue) == 0 && m.deleteSelected != nil && len(m.deleteSelected) == 0 {
		// keep nil for clean state; no-op
	}
	return m, nil
}

// toggleDeleteSelection toggles tick for /delete picker (skill-like).
func (m *AppModel) toggleDeleteSelection(runID string) {
	if strings.EqualFold(strings.TrimSpace(runID), "all") {
		// Toggle all
		allTicked := true
		if m.deleteSelected == nil {
			allTicked = false
		} else {
			for _, ch := range m.chatList {
				if !m.deleteSelected[strings.TrimSpace(ch.RunID)] {
					allTicked = false
					break
				}
			}
		}
		if allTicked {
			m.deleteSelected = nil
		} else {
			if m.deleteSelected == nil {
				m.deleteSelected = make(map[string]bool)
			}
			for _, ch := range m.chatList {
				if id := strings.TrimSpace(ch.RunID); id != "" {
					m.deleteSelected[id] = true
				}
			}
		}
		return
	}
	id := strings.TrimSpace(runID)
	if id == "" {
		return
	}
	if m.deleteSelected == nil {
		m.deleteSelected = make(map[string]bool)
	}
	if m.deleteSelected[id] {
		delete(m.deleteSelected, id)
		if len(m.deleteSelected) == 0 {
			m.deleteSelected = nil
		}
	} else {
		m.deleteSelected[id] = true
	}
}

func (m *AppModel) retargetDeleteSuggestion(value string) {
	items := m.collectSuggestions()
	for i, it := range items {
		if it.kind == "delete" && strings.EqualFold(it.value, value) {
			m.suggIdx = i
			return
		}
	}
}

func chatLabelForRunID(items []client.RunHistoryItem, runID string) string {
	for _, it := range items {
		if strings.TrimSpace(it.RunID) == strings.TrimSpace(runID) {
			t := strings.TrimSpace(it.LastPrompt)
			if t == "" {
				t = strings.TrimSpace(it.LastMessage)
			}
			if t == "" {
				t = it.RunID
			}
			t = collapseWS(t)
			if len([]rune(t)) > 48 {
				r := []rune(t)
				t = string(r[:45]) + "…"
			}
			return t
		}
	}
	return runID
}
