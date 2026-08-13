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
	Err                   string
}

// HistoryChunkMsg carries an older SSE chunk prepended on Load earlier (Task-290 Q-1).
type HistoryChunkMsg struct {
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
	var sb strings.Builder
	sb.WriteString("Recent chats (Desktop history parity):\n")
	if len(items) == 0 {
		sb.WriteString("  (none for this project)\n")
		sb.WriteString("Usage: /history|/open|/resume  (then ↑↓ Tab Enter)")
		return sb.String()
	}
	// Dump stays short; the live picker (command + space) scrolls through ALL items.
	limit := 20
	if len(items) < limit {
		limit = len(items)
	}
	for i := 0; i < limit; i++ {
		it := items[i]
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
		sb.WriteString(fmt.Sprintf("  %2d  %s  [%s] %s  %s · %s\n", i+1, shortID(it.RunID), kind, it.Status, when, title))
		sb.WriteString(fmt.Sprintf("      id %s  %s\n", it.RunID, it.ProviderKey))
	}
	if len(items) > limit {
		sb.WriteString(fmt.Sprintf("  … %d more in dump — type /history  and ↑↓ to reach every chat (%d total)\n", len(items)-limit, len(items)))
	}
	sb.WriteString("Pick: /history|/open|/resume  then ↑↓ · Tab · Enter (picker scrolls past this dump)")
	return sb.String()
}

func collapseWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// resolveChatOpenTarget maps /history|/open|/resume args to a run id using the last list.
func resolveChatOpenTarget(args []string, listed []client.RunHistoryItem) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: /history|/open|/resume <n|runId> — type the command + space for the picker")
	}
	token := strings.TrimSpace(args[0])
	if n, err := strconv.Atoi(token); err == nil {
		if n < 1 || n > len(listed) {
			return "", fmt.Errorf("chat index %d out of range (1-%d) — type /history  for the picker", n, len(listed))
		}
		return listed[n-1].RunID, nil
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
	for _, ev := range evs {
		switch ev.Type {
		case "turn_started":
			if p := strings.TrimSpace(ev.Prompt); p != "" {
				out = append(out, ChatMessage{Role: "user", Content: p})
			}
		case "message_delta":
			if ev.Text == "" {
				continue
			}
			if len(out) > 0 && out[len(out)-1].Role == "assistant" {
				out[len(out)-1].Content += ev.Text
			} else {
				out = append(out, ChatMessage{Role: "assistant", Content: ev.Text})
			}
		case "message_completed":
			if isStepCompleteStub(ev.Text) {
				continue
			}
			if ev.Text == "" {
				continue
			}
			if len(out) > 0 && out[len(out)-1].Role == "assistant" {
				if strings.TrimSpace(out[len(out)-1].Content) == "" {
					out[len(out)-1].Content = ev.Text
				}
			} else {
				out = append(out, ChatMessage{Role: "assistant", Content: ev.Text})
			}
		case "turn_completed":
			if isStepCompleteStub(ev.FinalMessage) {
				continue
			}
			if strings.TrimSpace(ev.FinalMessage) == "" {
				continue
			}
			// Only fill empty assistant bubble; never replace streamed text with stub.
			if len(out) == 0 || out[len(out)-1].Role != "assistant" {
				out = append(out, ChatMessage{Role: "assistant", Content: ev.FinalMessage})
			} else if strings.TrimSpace(out[len(out)-1].Content) == "" {
				out[len(out)-1].Content = ev.FinalMessage
			}
		}
	}
	return out
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
		items, err := cl.ListRunHistory(context.Background(), projectID)
		if err != nil {
			return ChatListMsg{Err: err.Error(), Silent: silent}
		}
		return ChatListMsg{Items: filterParentHistory(items), Silent: silent}
	}
}

func (m *AppModel) cmdMaybePrefetchHistory() tea.Cmd {
	trimmed := strings.TrimSpace(m.inputValue)
	_, _, argOK := parseChatOpenArgPrefix(m.inputValue)
	bare := false
	for _, cmd := range chatOpenSlashCommands {
		if strings.EqualFold(trimmed, cmd) {
			bare = true
			break
		}
	}
	if !argOK && !bare {
		return nil
	}
	if len(m.chatList) > 0 {
		return nil
	}
	return m.cmdPrefetchChats()
}

func (m *AppModel) cmdOpenChat(runID string) tea.Cmd {
	runnerURL := m.runnerURL
	chatProvider := ""
	for _, it := range m.chatList {
		if it.RunID == runID {
			chatProvider = it.ProviderKey
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
			collected = collectReplayEvents(cl, ctx, runID, after, until, chatReplayMaxEvents)
		}
		msgs := replayHistoryMessages(trimEventsFromTurnStart(collected))
		return ChatOpenedMsg{
			Handle:                handle,
			Messages:              msgs,
			HistoryLoadedAfterSeq: after,
		}
	}
}
