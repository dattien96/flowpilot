package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

// CA-527 — expanding a tool-call group must push the transcript DOWN (b shifts
// to a lower row), not shove the list UP off-screen. The viewport is
// bottom-anchored (offset = rows from the bottom), so toggleToolGroup shifts the
// offset by ±N (N = tool-line count) to keep the summary row — and everything
// above it — pinned while the group expands downward.

func toolGroupKey3() string {
	return toolGroupKey([]ChatMessage{
		{Role: "tool", Content: "→ read_file"},
		{Role: "tool", Content: "→ grep"},
		{Role: "tool", Content: "→ write"},
	})
}

// TestToolGroupExpand_OffsetShiftsByN: at the bottom anchor (offset=0) the offset
// increases by the number of tool rows on expand and returns on collapse.
func TestToolGroupExpand_OffsetShiftsByN(t *testing.T) {
	m := toolGroupModel("codex")
	m.addMessage("user", strings.Repeat("padding line ", 300), "")
	m.addMessage("tool", "→ read_file", "tool")
	m.addMessage("tool", "→ grep", "tool")
	m.addMessage("tool", "→ write", "tool")
	key := toolGroupKey3()
	_ = m.View()
	c := m.tuiChrome()
	total := len(m.renderMessages())
	m.clampViewport(total, c.messagesHeight)
	if total <= c.messagesHeight {
		t.Skip("transcript must be scrollable for this test")
	}
	m.viewport.offset = 0 // follow live (bottom anchor)
	m.toggleToolGroup(key)
	if m.viewport.offset != 3 {
		t.Fatalf("expand must raise offset by 3 (tool rows), got %d", m.viewport.offset)
	}
	m.toggleToolGroup(key)
	if m.viewport.offset != 0 {
		t.Fatalf("collapse must restore offset, got %d", m.viewport.offset)
	}
}

// TestToolGroupExpand_KeepsSummaryPinnedAndShiftsB: on a scrollable transcript at
// the bottom, expanding keeps the "tool calls" summary on the same screen row
// and moves the following "b" row DOWN (b's screen Y increases), with the tool
// lines now visible without scrolling up.
func TestToolGroupExpand_KeepsSummaryPinnedAndShiftsB(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := toolGroupModel(pk)
			m.height = 40
			m.addMessage("user", strings.Repeat("padding line ", 60), "")
			m.addMessage("user", strings.Repeat("trailing line ", 60), "")
			m.addMessage("user", "AAA", "")
			m.addMessage("tool", "→ read_file", "tool")
			m.addMessage("tool", "→ grep", "tool")
			m.addMessage("tool", "→ write", "tool")
			m.addMessage("assistant", "ZZZ_after", "")
			m.addMessage("user", strings.Repeat("end line ", 60), "")
			key := toolGroupKey3()
			_ = m.View()

			yc := viewLineIndex(m, "tool calls")
			yb0 := viewLineIndex(m, "ZZZ_after")
			if yc < 0 || yb0 < 0 {
				t.Fatalf("%s: summary/b not found (yc=%d yb=%d)", pk, yc, yb0)
			}

			m.toggleToolGroup(key)
			view := stripANSI(m.View())
			ye := viewLineIndex(m, "tool calls")
			yb1 := viewLineIndex(m, "ZZZ_after")

			if ye != yc {
				t.Fatalf("%s: summary must stay pinned, moved %d→%d:\n%s", pk, yc, ye, view)
			}
			if yb1 <= yb0 {
				t.Fatalf("%s: following row must shift DOWN, %d→%d:\n%s", pk, yb0, yb1, view)
			}
			if !strings.Contains(view, "→ read_file") {
				t.Fatalf("%s: expanded tool lines must be visible without scrolling up:\n%s", pk, view)
			}

			// Collapse: b returns to its original screen row.
			m.toggleToolGroup(key)
			view = stripANSI(m.View())
			if got := viewLineIndex(m, "ZZZ_after"); got != yb0 {
				t.Fatalf("%s: b must return to row %d after collapse, got %d:\n%s", pk, yb0, got, view)
			}
			if strings.Contains(view, "→ read_file") {
				t.Fatalf("%s: collapse must hide tool lines:\n%s", pk, view)
			}
		})
	}
}

// TestToolGroupExpand_TopAnchored: scrolled to the top (offset=maxOff), expanding
// keeps the first content row in place and shows the tool lines just under the
// summary.
func TestToolGroupExpand_TopAnchored(t *testing.T) {
	m := toolGroupModel("codex")
	m.addMessage("user", "AAA_TOP", "")
	m.addMessage("tool", "→ read_file", "tool")
	m.addMessage("tool", "→ grep", "tool")
	m.addMessage("tool", "→ write", "tool")
	m.addMessage("user", strings.Repeat("padding line ", 300), "")
	key := toolGroupKey3()
	_ = m.View()
	c := m.tuiChrome()
	total := len(m.renderMessages())
	m.clampViewport(total, c.messagesHeight)
	if total <= c.messagesHeight {
		t.Skip("transcript must be scrollable for this test")
	}
	m.viewport.offset = total - c.messagesHeight // top of history
	y0 := viewLineIndex(m, "AAA_TOP")
	if y0 < 0 {
		t.Fatalf("top row not found in view")
	}
	m.toggleToolGroup(key)
	view := stripANSI(m.View())
	if y1 := viewLineIndex(m, "AAA_TOP"); y1 != y0 {
		t.Fatalf("top content must stay put on expand, %d→%d:\n%s", y0, y1, view)
	}
	if !strings.Contains(view, "→ read_file") {
		t.Fatalf("expanded tool lines must be visible at the top anchor:\n%s", view)
	}
}

// TestToolGroupExpand_ClickCollapseSameY: after an expand click, the summary stays
// on the same screen row and a second click at that same row collapses it again
// (the row must not slide under the cursor).
func TestToolGroupExpand_ClickCollapseSameY(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := toolGroupModel(pk)
			m.height = 40
			m.addMessage("user", strings.Repeat("padding line ", 60), "")
			m.addMessage("tool", "→ read_file", "tool")
			m.addMessage("tool", "→ grep", "tool")
			m.addMessage("tool", "→ write", "tool")
			m.addMessage("user", strings.Repeat("end line ", 60), "")
			key := toolGroupKey3()
			x, y, ok := findClickTarget(m, "tool-group:"+key)
			if !ok {
				t.Fatalf("%s: summary must be clickable", pk)
			}
			m2, _ := m.Update(clickLeft(x, y))
			am := m2.(*AppModel)
			// After expanding, the summary is still hit-tested at the same screen row.
			if got := am.clickTargetAt(x, y); got != "tool-group:"+key {
				t.Fatalf("%s: summary must stay clickable at same y=%d, got %q", pk, y, got)
			}
			m3, _ := am.Update(clickLeft(x, y))
			if view := stripANSI(m3.(*AppModel).View()); strings.Contains(view, "→ read_file") {
				t.Fatalf("%s: second click at the same row must collapse the group:\n%s", pk, view)
			}
		})
	}
}

// TestToolGroupExpand_StillWorksWithNew (helper sanity): New() model used by all
// CA-527 tests uses a chat-mode session panel (no sidebar) so chatWidth == width.
func TestToolGroupExpand_ModelBuilderSanity(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = true
	if m.useRightSidebar() {
		t.Fatal("no session content → no right sidebar")
	}
	if m.chatWidth() != 80 {
		t.Fatalf("chatWidth=%d want 80", m.chatWidth())
	}
}
