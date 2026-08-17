package app

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/tui/config"
)

// CA-533 — opencode/Grok-style animated "Thinking" placeholder in the chat
// row + status line. Provider-agnostic (Case 1): render/tick logic reads only
// message FormatHint + thinkingFrame, never providerKey — parameterized over
// claude/codex/grok anyway so a future provider-specific branch trips here.

func thinkingModel(pk string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = false
	m.sessionLoading = false
	return m
}

func thinkingRowText(m *AppModel) string {
	for _, r := range m.chatRows() {
		if strings.Contains(stripANSI(r.Text), "Thinking") {
			return r.Text
		}
	}
	return ""
}

// fgANSI returns the truecolor foreground escape marker lipgloss emits for hex.
func fgANSI(hex string) string {
	r, _ := strconv.ParseInt(hex[1:3], 16, 64)
	g, _ := strconv.ParseInt(hex[3:5], 16, 64)
	b, _ := strconv.ParseInt(hex[5:7], 16, 64)
	return fmt.Sprintf("38;2;%d;%d;%d", r, g, b)
}

func TestFormatThinkingElapsed(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{500 * time.Millisecond, "0s"},
		{900 * time.Millisecond, "0s"},
		{1400 * time.Millisecond, "1.4s"},
		{9900 * time.Millisecond, "9.9s"},
		{10 * time.Second, "10s"},
		{12 * time.Second, "12s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m 00s"},
		{64 * time.Second, "1m 04s"},
		{125 * time.Second, "2m 05s"},
	}
	for _, c := range cases {
		if got := formatThinkingElapsed(c.d); got != c.want {
			t.Errorf("formatThinkingElapsed(%v)=%q want %q", c.d, got, c.want)
		}
	}
}

func TestThinkingLabelText_SpinnerAdvancesAndWraps(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			braille := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			for i, want := range braille {
				got := thinkingLabelText(i, false)
				if !strings.HasPrefix(got, want+" ") {
					t.Fatalf("frame %d spinner=%q want prefix %q; line=%q", i, got[:3], want, got)
				}
			}
			if got := thinkingLabelText(len(braille), false); !strings.HasPrefix(got, braille[0]+" ") {
				t.Fatalf("spinner must wrap to frame 0: %q", got)
			}
		})
	}
}

func TestThinkingLabelText_AsciiSpinner(t *testing.T) {
	for i, want := range []string{"|", "/", "-", "\\"} {
		if got := thinkingLabelText(i, true); !strings.HasPrefix(got, want+" ") {
			t.Fatalf("ascii frame %d spinner=%q want %q; line=%q", i, got[:2], want, got)
		}
	}
	if got := thinkingLabelText(4, true); !strings.HasPrefix(got, "| ") {
		t.Fatalf("ascii spinner must wrap: %q", got)
	}
}

func TestThinkingLabelText_ElapsedRisesWithFrame(t *testing.T) {
	// frame * 90ms: frame 12 ≈ 1.1s, frame 134 ≈ 12s
	cases := []struct {
		frame int
		want  string
	}{
		{0, "0s"},
		{12, "1.1s"},
		{134, "12s"},
	}
	for _, c := range cases {
		if got := thinkingLabelText(c.frame, false); !strings.Contains(got, c.want) {
			t.Fatalf("frame %d should show %q: %q", c.frame, c.want, got)
		}
	}
}

func TestThinkingShimmer_WindowSweeps(t *testing.T) {
	forceTrueColor(t)
	accent := fgANSI(colorAccent)
	count := func(s string) int { return strings.Count(s, accent) }
	// frame 0: window fully inside label (3 chars) + spinner = 4 accent segments
	if got := count(renderThinkingLine(0, false)); got != 4 {
		t.Fatalf("frame 0 accent segments=%d want 4 (spinner + 3 label chars)", got)
	}
	// frame 9: window fully past the label → only the spinner is accent
	if got := count(renderThinkingLine(9, false)); got != 1 {
		t.Fatalf("frame 9 accent segments=%d want 1 (spinner only)", got)
	}
	// visible text (label + elapsed) must stay identical while the spinner
	// rotates and only the highlight moves
	labelOf := func(s string) string {
		parts := strings.SplitN(stripANSI(s), " ", 2)
		if len(parts) < 2 {
			return ""
		}
		return parts[1]
	}
	if labelOf(renderThinkingLine(0, false)) != labelOf(renderThinkingLine(9, false)) {
		t.Fatalf("shimmer must not change the visible label/elapsed")
	}
}

func TestThinkingRow_RendersAnimatedInView(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := thinkingModel(pk)
			m.addMessage("user", "fix the bug", "")
			m.addMessage("assistant", "thinking…", "thinking")
			m.thinkingFrame = 0
			view := stripANSI(m.View())
			if !strings.Contains(view, "Thinking") {
				t.Fatalf("%s: thinking row must render the animated label:\n%s", pk, view)
			}
			if !strings.Contains(view, "⠋") {
				t.Fatalf("%s: thinking row must show the braille spinner:\n%s", pk, view)
			}
			if !strings.Contains(view, "0s") {
				t.Fatalf("%s: thinking row must show elapsed seconds:\n%s", pk, view)
			}
		})
	}
}

func TestThinkingRow_OnlyAnimatedForThinkingHint(t *testing.T) {
	m := thinkingModel("grok")
	m.addMessage("user", "u", "")
	m.addMessage("assistant", "a normal answer", "")
	view := stripANSI(m.View())
	if strings.Contains(view, "Thinking") || strings.Contains(view, "⠋") {
		t.Fatalf("normal assistant message must not be animated:\n%s", view)
	}
}

func TestThinkingTick_AdvancesAndSelfStops(t *testing.T) {
	m := thinkingModel("codex")
	m.addMessage("user", "u", "")
	m.addMessage("assistant", "thinking…", "thinking")
	m.thinkingFrame = 0
	m.thinkingTickerActive = true

	m2, cmd := m.Update(thinkingTickMsg{})
	am := m2.(*AppModel)
	if am.thinkingFrame != 1 {
		t.Fatalf("thinkingFrame=%d want 1 after tick", am.thinkingFrame)
	}
	if cmd == nil {
		t.Fatal("tick must reschedule while a thinking row is live")
	}
	if !am.thinkingTickerActive {
		t.Fatal("ticker must stay active while a thinking row is live")
	}

	// The thinking placeholder is replaced by a real answer → ticker must stop.
	am.replaceThinkingAt(am.thinkingIndex(), "answer")
	m3, cmd2 := am.Update(thinkingTickMsg{})
	am2 := m3.(*AppModel)
	if am2.thinkingFrame != 2 {
		t.Fatalf("thinkingFrame=%d want 2 (tick still advances once more)", am2.thinkingFrame)
	}
	if cmd2 != nil {
		t.Fatal("tick must not reschedule after the thinking row is gone")
	}
	if am2.thinkingTickerActive {
		t.Fatal("ticker must be marked inactive after the thinking row is gone")
	}
}

func TestThinkingTick_ResetsOnFreshPlaceholder(t *testing.T) {
	m := thinkingModel("claude")
	m.addMessage("user", "u", "")
	m.addMessage("assistant", "thinking…", "thinking")
	m.thinkingFrame = 40
	// A new thinking placeholder (follow-up turn) resets the elapsed spinner.
	m.addMessage("assistant", "thinking…", "thinking")
	if m.thinkingFrame != 0 {
		t.Fatalf("thinkingFrame=%d want 0 after fresh placeholder", m.thinkingFrame)
	}
}

func TestChatRowsSig_ThinkingFrameInvalidates(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := thinkingModel(pk)
			m.addMessage("user", "u", "")
			m.addMessage("assistant", "thinking…", "thinking")
			m.thinkingFrame = 0
			_ = m.chatRows()
			sig0 := m.rowCacheSig
			text0 := thinkingRowText(m)
			if text0 == "" {
				t.Fatalf("%s: no thinking row rendered", pk)
			}
			m.thinkingFrame = 5
			text1 := thinkingRowText(m)
			if text1 == "" {
				t.Fatalf("%s: no thinking row rendered after frame bump", pk)
			}
			if text0 == text1 {
				t.Fatalf("%s: cache must re-render when thinking frame advances", pk)
			}
			if m.rowCacheSig == sig0 {
				t.Fatalf("%s: row-cache signature must change with thinking frame", pk)
			}
		})
	}
}

func TestChatRowsSig_StableWithoutThinkingRow(t *testing.T) {
	m := thinkingModel("codex")
	m.addMessage("user", "u", "")
	m.addMessage("assistant", "answer", "")
	m.thinkingFrame = 0
	_ = m.chatRows()
	sig0 := m.rowCacheSig
	m.thinkingFrame = 99
	_ = m.chatRows()
	if m.rowCacheSig != sig0 {
		t.Fatal("row-cache signature must stay stable when no thinking row is live")
	}
}

func TestStatusReadyLabel_ShowsAnimatedThinking(t *testing.T) {
	m := thinkingModel("claude")
	m.statusMsg = "thinking…"
	if got := m.statusReadyLabel(); got != "thinking…" {
		t.Fatalf("without a thinking row statusReadyLabel=%q want static thinking…", got)
	}
	m.addMessage("user", "u", "")
	m.addMessage("assistant", "thinking…", "thinking")
	m.thinkingFrame = 0
	got := m.statusReadyLabel()
	if !strings.HasPrefix(got, "⠋ ") {
		t.Fatalf("status must lead with the spinner: %q", got)
	}
	if !strings.Contains(got, "Thinking") || !strings.Contains(got, "0s") {
		t.Fatalf("status must show label + elapsed: %q", got)
	}
}
