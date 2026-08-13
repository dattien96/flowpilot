package app

import "fmt"

// Task-290 / Desktop Timeline parity: window long chats by user-prompt groups.

const chatPromptPageSize = 6

func countUserPrompts(msgs []ChatMessage) int {
	n := 0
	for _, msg := range msgs {
		if msg.Role == "user" {
			n++
		}
	}
	return n
}

func sliceMessagesFromPrompt(msgs []ChatMessage, visiblePromptCount int) []ChatMessage {
	total := countUserPrompts(msgs)
	if visiblePromptCount >= total {
		return msgs
	}
	promptsToSkip := total - visiblePromptCount
	promptsSeen := 0
	for i, msg := range msgs {
		if msg.Role != "user" {
			continue
		}
		promptsSeen++
		if promptsSeen > promptsToSkip {
			return msgs[i:]
		}
	}
	return msgs
}

func ensureVisiblePromptCount(current, total, pageSize int) int {
	if total <= pageSize {
		return total
	}
	if current <= 0 || current < pageSize {
		return pageSize
	}
	if current > total {
		return total
	}
	return current
}

func expandVisiblePromptCount(current, total, pageSize int) int {
	if total <= pageSize {
		return total
	}
	next := current
	if next <= 0 {
		next = pageSize
	}
	next += pageSize
	if next > total {
		return total
	}
	return next
}

func findPromptGroupStart(msgs []ChatMessage, idx int) int {
	if idx < 0 || idx >= len(msgs) {
		return 0
	}
	for i := idx; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return i
		}
	}
	return 0
}

func (m *AppModel) syncVisiblePromptCount() {
	total := countUserPrompts(m.messages)
	m.visiblePromptCount = ensureVisiblePromptCount(m.visiblePromptCount, total, chatPromptPageSize)
}

func (m *AppModel) windowStartIndex() int {
	m.syncVisiblePromptCount()
	total := countUserPrompts(m.messages)
	start := 0
	if m.visiblePromptCount < total {
		sliced := sliceMessagesFromPrompt(m.messages, m.visiblePromptCount)
		start = len(m.messages) - len(sliced)
	}
	if min := m.minVisibleIndexForPending(); min >= 0 && min < start {
		start = min
	}
	return start
}

func (m *AppModel) minVisibleIndexForPending() int {
	if idx := m.thinkingIndex(); idx >= 0 {
		return findPromptGroupStart(m.messages, idx)
	}
	if m.approval != nil || m.question != nil || m.gate != nil {
		for i := len(m.messages) - 1; i >= 0; i-- {
			switch m.messages[i].FormatHint {
			case "approval", "question", "gate":
				return findPromptGroupStart(m.messages, i)
			}
		}
	}
	return -1
}

func (m *AppModel) hiddenPromptCountBeforeWindow() int {
	start := m.windowStartIndex()
	if start <= 0 {
		return 0
	}
	return countUserPrompts(m.messages[:start])
}

func (m *AppModel) loadEarlierPrompts() {
	total := countUserPrompts(m.messages)
	m.visiblePromptCount = expandVisiblePromptCount(m.visiblePromptCount, total, chatPromptPageSize)
	m.rowCache = nil
	m.rowCacheSig = 0
	c := m.tuiChrome()
	lines := len(m.renderMessages())
	m.clampViewport(lines, c.messagesHeight)
	maxOff := lines - c.messagesHeight
	if maxOff < 0 {
		maxOff = 0
	}
	m.viewport.offset = maxOff
}

func loadEarlierPromptLabel(hidden int) string {
	if hidden < 1 {
		return ""
	}
	return fmtLoadEarlierLabel(hidden)
}

func fmtLoadEarlierLabel(hidden int) string {
	return fmt.Sprintf("↑ Load earlier prompts (%d)", hidden)
}
