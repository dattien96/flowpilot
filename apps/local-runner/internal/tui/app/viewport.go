package app

// sliceViewport returns the visible window of transcript lines.
// offset is the number of lines scrolled up from the bottom (0 = follow live).
func sliceViewport(lines []string, height, offset int) []string {
	if height < 1 {
		height = 1
	}
	if len(lines) <= height {
		return lines
	}
	maxOff := len(lines) - height
	if offset < 0 {
		offset = 0
	}
	if offset > maxOff {
		offset = maxOff
	}
	end := len(lines) - offset
	start := end - height
	return lines[start:end]
}

func sliceChatRows(rows []chatRow, height, offset int) []chatRow {
	if height < 1 {
		height = 1
	}
	if len(rows) <= height {
		return rows
	}
	maxOff := len(rows) - height
	if offset < 0 {
		offset = 0
	}
	if offset > maxOff {
		offset = maxOff
	}
	end := len(rows) - offset
	start := end - height
	return rows[start:end]
}

func (m *AppModel) clampViewport(total, height int) {
	maxOff := total - height
	if maxOff < 0 {
		maxOff = 0
	}
	if m.viewport.offset > maxOff {
		m.viewport.offset = maxOff
	}
	if m.viewport.offset < 0 {
		m.viewport.offset = 0
	}
}

func (m *AppModel) scrollTranscript(delta int) {
	m.viewport.offset += delta
	if m.viewport.offset < 0 {
		m.viewport.offset = 0
	}
}

func (m *AppModel) pageScrollAmount() int {
	n := m.tuiChrome().messagesHeight
	if n < 1 {
		return 1
	}
	return n
}
