package app

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

// G2/G3 Drive chat-session sync surface (Desktop Navigator parity, CA-549/550).
// Push = /sync (upload a chat to the project's Drive folder); pull = /restore
// (bring a Drive-backed chat back into this machine). OAuth/folder binding is
// owned by the runner + Desktop /settings — the TUI only calls the three
// /client endpoints.

// driveSyncState tracks an in-flight /sync batch. Single syncs use the same
// machinery with a one-item queue so progress/badge updates share one path.
type driveSyncState struct {
	queue     []string
	projectID string
	total     int
	done      int
	failed    int
}

// DriveSyncBatchMsg reports one completed chat-session upload inside a batch.
type DriveSyncBatchMsg struct {
	RunID  string
	Result *client.ChatSessionSyncResult // nil when the upload failed
	Err    error
}

// hasAgentPromptPrefix mirrors Desktop navigatorHistory.hasBuiltInAgentPromptPrefix.
func hasAgentPromptPrefix(prompt string) bool {
	p := strings.ToLower(strings.TrimSpace(prompt))
	return strings.HasPrefix(p, "you are the coder sub-agent.") ||
		strings.HasPrefix(p, "you are the reviewer sub-agent.") ||
		strings.HasPrefix(p, "you are the tester sub-agent.")
}

// isAgentHistoryItem mirrors Desktop navigatorHistory.isAgentHistoryItem: a
// child agent run is never a sync/restore target at the top level.
func isAgentHistoryItem(it client.RunHistoryItem) bool {
	return strings.TrimSpace(it.ParentRunID) != "" || hasAgentPromptPrefix(it.LastPrompt)
}

// isSyncableRun mirrors Desktop navigatorHistory.isSyncableRun: kind must be a
// chat-like run, the row must not be an agent child / unavailable, and the
// confirmed Drive index (when loaded) overrides a stale local syncStatus.
func isSyncableRun(it client.RunHistoryItem, remote []client.RemoteChatSessionSummary) bool {
	kind := strings.TrimSpace(it.RunKind)
	if kind != "chat" && kind != "" && kind != "workflow" {
		return false
	}
	if isAgentHistoryItem(it) || strings.TrimSpace(it.UnavailableReason) != "" {
		return false
	}
	switch strings.TrimSpace(it.SyncStatus) {
	case "synced", "unsyncable":
		return false
	}
	if strings.TrimSpace(it.SourceMachineID) != "" && strings.TrimSpace(it.SourceRunID) != "" {
		for _, r := range remote {
			if r.SourceMachineID == it.SourceMachineID && r.SourceRunID == it.SourceRunID {
				return false
			}
		}
	}
	return true
}

// syncableChats returns the /sync target list for the current project: every
// isSyncableRun row in the cached history (already filtered to parents).
func (m *AppModel) syncableChats() []client.RunHistoryItem {
	out := make([]client.RunHistoryItem, 0, len(m.chatList))
	for _, it := range m.chatList {
		if isSyncableRun(it, m.remoteChatList) {
			out = append(out, it)
		}
	}
	return out
}

// cmdSyncRun uploads one chat to the project Drive folder.
func (m *AppModel) cmdSyncRun(runID, projectID string) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		res, err := cl.SyncChatRun(context.Background(), runID, client.ChatSessionSyncRequest{
			GoogleDriveProjectID: projectID,
		})
		if err != nil {
			return DriveSyncBatchMsg{RunID: runID, Err: err}
		}
		return DriveSyncBatchMsg{RunID: runID, Result: &res}
	}
}

// startSyncBatch arms the sequential batch and processes the first item.
func (m *AppModel) startSyncBatch(runIDs []string, projectID string) tea.Cmd {
	if len(runIDs) == 0 {
		m.addMessage("system", "Nothing to sync — every chat is already in Drive.", "")
		return nil
	}
	m.driveSync = &driveSyncState{
		queue:     runIDs[1:],
		projectID: projectID,
		total:     len(runIDs),
	}
	return m.cmdSyncRun(runIDs[0], projectID)
}

// markChatSyncStatus writes the sync result (or failure) back into the cached
// /history rows so the picker badge reflects it immediately.
func (m *AppModel) markChatSyncStatus(runID string, err error, res *client.ChatSessionSyncResult) {
	status := "failed"
	reason := ""
	if err != nil {
		if ae, ok := err.(*client.APIError); ok {
			if ae.Code == "unsyncable" {
				status = "unsyncable"
			}
			reason = ae.Message
		}
	} else if res != nil && strings.TrimSpace(res.SyncStatus) != "" {
		status = res.SyncStatus
	}
	for i := range m.chatList {
		if m.chatList[i].RunID == runID {
			m.chatList[i].SyncStatus = status
			if status == "failed" {
				m.chatList[i].UnavailableReason = reason
			}
			return
		}
	}
}

// formatDriveSyncErr maps runner sync errors to a TUI hint.
func formatDriveSyncErr(runID string, err error) string {
	if ae, ok := err.(*client.APIError); ok {
		switch ae.Code {
		case "google_drive_not_connected":
			return fmt.Sprintf("Drive is not connected for this project — run /settings to connect, then retry /sync %s", runID)
		case "unsyncable":
			return fmt.Sprintf("%s can never be synced (no resumable session on this machine): %s", runID, ae.Message)
		}
	}
	return fmt.Sprintf("Sync %s failed: %v", runID, err)
}

func formatDriveSyncSummary(st *driveSyncState) string {
	ok := st.total - st.failed
	if st.failed == 0 {
		return fmt.Sprintf("Synced %d/%d chats to Drive.", ok, st.total)
	}
	return fmt.Sprintf("Synced %d/%d chats to Drive · %d failed.", ok, st.total, st.failed)
}

// runSyncDispatch resolves the /sync args and starts the batch.
func (m *AppModel) runSyncDispatch(args []string) (tea.Model, tea.Cmd) {
	if m.project == nil || strings.TrimSpace(m.project.ID) == "" {
		m.addMessage("system", "No project bound — wait for session load or set --project before /sync.", "error")
		return m, nil
	}
	var targets []string
	switch {
	case len(args) == 0:
		if m.runHandle == nil || strings.TrimSpace(m.runHandle.RunID) == "" {
			m.addMessage("system", "No open chat — type /history then /sync <n|runId> or /sync all.", "")
			return m, nil
		}
		targets = []string{m.runHandle.RunID}
	case strings.EqualFold(args[0], "all"):
		for _, it := range m.syncableChats() {
			targets = append(targets, it.RunID)
		}
	default:
		runID, err := resolveChatOpenTarget(args, m.chatList)
		if err != nil {
			m.addMessage("system", err.Error()+" — type /sync  then ↑↓ Tab Enter", "error")
			return m, nil
		}
		targets = []string{runID}
	}
	if len(targets) == 0 {
		m.addMessage("system", "Nothing to sync — every chat is already in Drive.", "")
		return m, nil
	}
	return m, m.startSyncBatch(targets, m.project.ID)
}

// filterSyncSuggestions returns the /sync picker rows: an "all" bulk row plus
// every syncable chat matching the query. Returns nil for non-/sync input so
// the generic pickers still win. The "all" row uses kind "sync" so accepting it
// yields `/sync all` (Desktop Navigator "Sync all" parity).
func filterSyncSuggestions(input string, syncable []client.RunHistoryItem) []suggestItem {
	ok, query := parseSlashArgPrefix(input, "/sync")
	if !ok {
		return nil
	}
	if len(syncable) == 0 {
		return nil
	}
	q := strings.ToLower(query)
	out := make([]suggestItem, 0, len(syncable)+1)
	out = append(out, suggestItem{
		value:  "all",
		detail: fmt.Sprintf("Sync all %d syncable chats to Drive", len(syncable)),
		kind:   "sync",
		slash:  "/sync",
	})
	for _, it := range syncable {
		title := strings.TrimSpace(it.LastPrompt)
		if title == "" {
			title = strings.TrimSpace(it.LastMessage)
		}
		if title == "" {
			title = "(no prompt)"
		}
		hay := strings.ToLower(it.RunID + " " + title + " " + it.Status + " " + it.ProviderKey)
		if q != "" && !strings.Contains(hay, q) {
			continue
		}
		detail := title
		if badge := formatSyncBadge(it); badge != "" {
			detail += " · " + badge
		}
		out = append(out, suggestItem{value: it.RunID, detail: detail, kind: "sync", slash: "/sync"})
	}
	return out
}
