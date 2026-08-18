package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

// RemoteChatListMsg carries the project's Drive-backed chat index.
type RemoteChatListMsg struct {
	Items  []client.RemoteChatSessionSummary
	Err    string
	Silent bool
}

// driveSyncTickMsg advances the Drive sync/restore spinner (CA-551) while a
// batch is in flight. The 90ms cadence matches the thinking spinner.
type driveSyncTickMsg struct{}

const driveSyncTickInterval = 90 * time.Millisecond

func cmdDriveSyncTick() tea.Cmd {
	return tea.Tick(driveSyncTickInterval, func(time.Time) tea.Msg {
		return driveSyncTickMsg{}
	})
}

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
	if len(runIDs) == 1 {
		m.addMessage("system", fmt.Sprintf("Syncing %s to Drive…", shortID(runIDs[0])), "")
	} else {
		m.addMessage("system", fmt.Sprintf("Syncing %d chats to Drive…", len(runIDs)), "")
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

// openChatDriveBadge returns the Drive sync badge for the currently open chat
// (CA-553). It resolves the open run against the cached /history rows plus the
// confirmed Drive index — the same reconciliation the picker/dump use — so the
// session panel's "Drive: <badge>" line under the Run line always matches.
func (m *AppModel) openChatDriveBadge() string {
	if m.runHandle == nil {
		return ""
	}
	runID := strings.TrimSpace(m.runHandle.RunID)
	if runID == "" {
		return ""
	}
	for _, it := range m.chatList {
		if strings.TrimSpace(it.RunID) == runID {
			if badge := syncBadgeWithRemote(it, m.remoteChatList); badge != "" {
				return badge
			}
		}
	}
	return ""
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
	return filterSyncSuggestionsWithRemote(input, syncable, nil)
}

// filterSyncSuggestionsWithRemote is filterSyncSuggestions plus Drive-index
// reconciliation (CA-552): a chat that only matches the remote index still gets
// its (synced) marker so the picker never re-targets an already-synced chat.
func filterSyncSuggestionsWithRemote(input string, syncable []client.RunHistoryItem, remote []client.RemoteChatSessionSummary) []suggestItem {
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
		if badge := syncBadgeWithRemote(it, remote); badge != "" {
			detail = badge + " · " + title
		}
		out = append(out, suggestItem{value: it.RunID, detail: detail, kind: "sync", slash: "/sync"})
	}
	return out
}

// ---- G3 /restore: pull a Drive-backed chat into this machine ----------------

// restoreState tracks an in-flight /restore batch. A single restore is a
// one-item batch with openAfter=true so the restored chat opens on success.
type restoreState struct {
	queue     []string
	projectID string
	cwd       string
	total     int
	done      int
	failed    int
	openAfter bool
}

// RestoreBatchMsg reports one completed chat-session restore inside a batch.
type RestoreBatchMsg struct {
	SourceKey string // sourceMachineId:sourceRunId
	Result    *client.ChatSessionRestoreResult
	Err       error
}

// remoteSourceKey is the picker value + queue key for one remote session.
func remoteSourceKey(r client.RemoteChatSessionSummary) string {
	return strings.TrimSpace(r.SourceMachineID) + ":" + strings.TrimSpace(r.SourceRunID)
}

// cmdFetchRemoteChats loads the project's Drive-backed chat index (silent for
// prefetch after a batch; loud for a bare /restore dump).
func (m *AppModel) cmdFetchRemoteChats(silent bool) tea.Cmd {
	runnerURL := m.runnerURL
	projectID := ""
	if m.project != nil {
		projectID = m.project.ID
	}
	return func() tea.Msg {
		if projectID == "" {
			return RemoteChatListMsg{Err: "project_id required — wait for session load or set --project", Silent: silent}
		}
		cl := client.New(runnerURL)
		items, err := cl.ListRemoteChatSessions(context.Background(), projectID)
		if err != nil {
			return RemoteChatListMsg{Err: err.Error(), Silent: silent}
		}
		return RemoteChatListMsg{Items: items, Silent: silent}
	}
}

func (m *AppModel) cmdPrefetchRemoteChats() tea.Cmd {
	return m.cmdFetchRemoteChats(true)
}

// cmdRestoreOne pulls one Drive-backed chat. On cwd_remap_required it retries
// once with the bound project path (Desktop remap parity); only the first
// attempt omits cwd.
func (m *AppModel) cmdRestoreOne(sourceKey, projectID, cwd string) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		parts := strings.SplitN(sourceKey, ":", 2)
		if len(parts) != 2 {
			return RestoreBatchMsg{SourceKey: sourceKey, Err: fmt.Errorf("invalid source %q (want sourceMachineId:sourceRunId)", sourceKey)}
		}
		cl := client.New(runnerURL)
		req := client.ChatSessionRestoreRequest{
			ProjectID:       projectID,
			SourceMachineID: parts[0],
			SourceRunID:     parts[1],
		}
		if cwd != "" {
			req.Cwd = cwd
		}
		res, err := cl.RestoreChatRun(context.Background(), req)
		if err != nil {
			if ae, ok := err.(*client.APIError); ok && ae.Code == "cwd_remap_required" && cwd == "" {
				req.Cwd = m.boundProjectPath()
				res2, err2 := cl.RestoreChatRun(context.Background(), req)
				if err2 == nil {
					return RestoreBatchMsg{SourceKey: sourceKey, Result: &res2}
				}
				return RestoreBatchMsg{SourceKey: sourceKey, Err: err2}
			}
			return RestoreBatchMsg{SourceKey: sourceKey, Err: err}
		}
		return RestoreBatchMsg{SourceKey: sourceKey, Result: &res}
	}
}

// projectPath returns the bound project path (may be empty before catalog binds).
func (m *AppModel) boundProjectPath() string {
	if m.project != nil {
		return m.project.Path
	}
	return ""
}

// startRestoreBatch arms the sequential restore queue and starts the first item.
func (m *AppModel) startRestoreBatch(keys []string, projectID, cwd string, openAfter bool) tea.Cmd {
	if len(keys) == 0 {
		m.addMessage("system", "Nothing to restore — no Drive-backed chats for this project.", "")
		return nil
	}
	if len(keys) == 1 {
		m.addMessage("system", fmt.Sprintf("Restoring %s from Drive…", keys[0]), "")
	} else {
		m.addMessage("system", fmt.Sprintf("Restoring %d chats from Drive…", len(keys)), "")
	}
	m.restoreBatch = &restoreState{
		queue:     keys[1:],
		projectID: projectID,
		cwd:       cwd,
		total:     len(keys),
		openAfter: openAfter,
	}
	return m.cmdRestoreOne(keys[0], projectID, cwd)
}

// driveIndicatorLine returns the right-side Drive sync/restore status line shown
// while a batch is in flight (CA-551): an animated spinner + progress. Empty
// when idle so the session panel stays unchanged.
func (m *AppModel) driveIndicatorLine() string {
	switch {
	case m.driveSync != nil:
		st := m.driveSync
		glyph := thinkingSpinner(m.driveSyncFrame, m.asciiMode)
		return fmt.Sprintf("Drive: %s Syncing %d/%d", glyph, st.done, st.total)
	case m.restoreBatch != nil:
		st := m.restoreBatch
		glyph := thinkingSpinner(m.driveSyncFrame, m.asciiMode)
		return fmt.Sprintf("Drive: %s Restoring %d/%d", glyph, st.done, st.total)
	default:
		return ""
	}
}

// formatRestoreErr maps runner restore errors to a TUI hint.
func formatRestoreErr(sourceKey string, err error) string {
	if ae, ok := err.(*client.APIError); ok {
		switch ae.Code {
		case "google_drive_not_connected":
			return fmt.Sprintf("Drive is not connected for this project — run /settings to connect, then retry /restore %s", sourceKey)
		case "session_unavailable":
			return fmt.Sprintf("%s can't be restored right now (not available on Drive): %s", sourceKey, ae.Message)
		case "sync_remote_not_found":
			return fmt.Sprintf("%s no longer exists on Drive: %s", sourceKey, ae.Message)
		case "sync_integrity_failed":
			return fmt.Sprintf("%s failed integrity check: %s", sourceKey, ae.Message)
		}
	}
	return fmt.Sprintf("Restore %s failed: %v", sourceKey, err)
}

func formatRestoreSummary(st *restoreState) string {
	ok := st.total - st.failed
	if st.failed == 0 {
		return fmt.Sprintf("Restored %d/%d chats from Drive.", ok, st.total)
	}
	return fmt.Sprintf("Restored %d/%d chats from Drive · %d failed.", ok, st.total, st.failed)
}

// formatRemoteChatList dumps the Drive-backed chat index for a bare /restore.
func formatRemoteChatList(items []client.RemoteChatSessionSummary) string {
	if len(items) == 0 {
		return "No Drive-backed chats found for this project."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Drive-backed chats (%d):", len(items))
	for i, it := range items {
		title := strings.TrimSpace(it.LastPrompt)
		if title == "" {
			title = strings.TrimSpace(it.LastMessage)
		}
		if title == "" {
			title = "(no prompt)"
		}
		title = collapseWS(title)
		if len([]rune(title)) > 42 {
			r := []rune(title)
			title = string(r[:39]) + "…"
		}
		fmt.Fprintf(&b, "\n  %d. %s [%s:%s] %s", i+1, title, it.SourceMachineID, it.SourceRunID, it.Status)
	}
	return b.String()
}

// filterRestoreSuggestions returns the /restore picker rows: an "all" bulk row
// plus every Drive-backed chat matching the query (Desktop "Restore all").
func filterRestoreSuggestions(input string, remote []client.RemoteChatSessionSummary) []suggestItem {
	ok, query := parseSlashArgPrefix(input, "/restore")
	if !ok {
		return nil
	}
	if len(remote) == 0 {
		return nil
	}
	q := strings.ToLower(query)
	out := make([]suggestItem, 0, len(remote)+1)
	out = append(out, suggestItem{
		value:  "all",
		detail: fmt.Sprintf("Restore all %d Drive-backed chats", len(remote)),
		kind:   "restore",
		slash:  "/restore",
	})
	for i, it := range remote {
		title := strings.TrimSpace(it.LastPrompt)
		if title == "" {
			title = strings.TrimSpace(it.LastMessage)
		}
		if title == "" {
			title = "(no prompt)"
		}
		hay := strings.ToLower(it.SourceMachineID + " " + it.SourceRunID + " " + it.RunID + " " + title + " " + it.Status)
		if q != "" && !strings.Contains(hay, q) {
			continue
		}
		// CA-554: one chat per row with the same #N · kind · status · date
		// detail as /open so the /restore Tab picker reads like the history
		// picker. The value stays machineId:runId (accept → /restore m1:r1).
		kind := strings.TrimSpace(it.RunKind)
		if kind == "" {
			if strings.TrimSpace(it.WorkflowID) != "" {
				kind = "workflow"
			} else {
				kind = "chat"
			}
		}
		when := ""
		if strings.TrimSpace(it.UpdatedAt) != "" {
			when = formatQuotaResetAt(it.UpdatedAt)
		}
		if when == "" {
			when = formatQuotaResetAt(it.StartedAt)
		}
		if when == "" {
			when = "—"
		}
		detail := fmt.Sprintf("#%d · %s · %s · %s", i+1, kind, it.Status, when)
		detail += " · " + title
		out = append(out, suggestItem{value: remoteSourceKey(it), detail: detail, kind: "restore", slash: "/restore"})
	}
	return out
}

// resolveRestoreTarget resolves /restore <n|runId|sourceKey> to a queue key.
func resolveRestoreTarget(args []string, remote []client.RemoteChatSessionSummary) (string, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return "", fmt.Errorf("no restore target")
	}
	arg := args[0]
	if n, err := strconv.Atoi(arg); err == nil {
		if n < 1 || n > len(remote) {
			return "", fmt.Errorf("index %d out of range", n)
		}
		return remoteSourceKey(remote[n-1]), nil
	}
	if strings.Contains(arg, ":") {
		return arg, nil
	}
	// runId match
	for _, it := range remote {
		if it.RunID == arg {
			return remoteSourceKey(it), nil
		}
	}
	return "", fmt.Errorf("no Drive-backed chat matches %q", arg)
}

// runRestoreDispatch resolves /restore args. A bare /restore dumps the remote
// index; /restore all batches every remote chat; /restore <n|runId|sourceKey>
// restores a single chat and opens it on success.
func (m *AppModel) runRestoreDispatch(args []string) (tea.Model, tea.Cmd) {
	if m.project == nil || strings.TrimSpace(m.project.ID) == "" {
		m.addMessage("system", "No project bound — wait for session load or set --project before /restore.", "error")
		return m, nil
	}
	if len(args) == 0 {
		if len(m.remoteChatList) == 0 {
			return m, m.cmdFetchRemoteChats(false)
		}
		m.addMessage("system", formatRemoteChatList(m.remoteChatList), "")
		return m, nil
	}
	if strings.EqualFold(args[0], "all") {
		keys := make([]string, 0, len(m.remoteChatList))
		for _, it := range m.remoteChatList {
			keys = append(keys, remoteSourceKey(it))
		}
		if len(keys) == 0 {
			m.addMessage("system", "No Drive-backed chats to restore.", "")
			return m, nil
		}
		return m, m.startRestoreBatch(keys, m.project.ID, m.boundProjectPath(), false)
	}
	if len(m.remoteChatList) == 0 {
		return m, m.cmdFetchRemoteChats(false)
	}
	key, err := resolveRestoreTarget(args, m.remoteChatList)
	if err != nil {
		m.addMessage("system", err.Error()+" — type /restore  then ↑↓ Tab Enter", "error")
		return m, nil
	}
	return m, m.startRestoreBatch([]string{key}, m.project.ID, m.boundProjectPath(), true)
}
