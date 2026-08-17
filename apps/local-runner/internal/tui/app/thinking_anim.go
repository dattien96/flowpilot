package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Opencode/Grok-style thinking animation for the chat "Thinking" placeholder.
//
// The thinking placeholder message keeps its stored content ("thinking…") so
// message-state logic and legacy tests are untouched; only rendering is
// animated. `thinkingFrame` (advanced by a dedicated 90ms tick while a thinking
// row is live) drives the spinner glyph, the label shimmer window, and the
// elapsed time — everything is derived from the frame so the row-cache
// signature only needs the frame to invalidate.

const (
	// thinkingTickInterval is the spinner step period. 90ms is smooth without
	// hogging the render loop (opencode-style spinner cadence).
	thinkingTickInterval = 90 * time.Millisecond
	thinkingLabel        = "Thinking"
	thinkingShimmerWidth = 3
)

var (
	thinkingSpinnerFrames      = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	thinkingSpinnerFramesASCII = []string{"|", "/", "-", "\\"}

	styleThinkingSpin   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent))
	styleThinkingActive = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent))
)

// thinkingSpinner returns the spinner glyph for the given frame.
func thinkingSpinner(frame int, ascii bool) string {
	frames := thinkingSpinnerFrames
	if ascii {
		frames = thinkingSpinnerFramesASCII
	}
	return frames[frame%len(frames)]
}

// formatThinkingElapsed renders the elapsed thinking time opencode/Grok style:
// "0s" → "1.4s" → "12s" → "1m 04s".
func formatThinkingElapsed(d time.Duration) string {
	secs := d.Seconds()
	switch {
	case secs < 1:
		return "0s"
	case secs < 10:
		return fmt.Sprintf("%.1fs", secs)
	case secs < 60:
		return fmt.Sprintf("%ds", int(secs))
	default:
		return fmt.Sprintf("%dm %02ds", int(secs)/60, int(secs)%60)
	}
}

// thinkingLabelText is the plain (unstyled) animated label — used by the status
// line so it picks up the status color, and as the base for the shimmered row.
func thinkingLabelText(frame int, ascii bool) string {
	elapsed := formatThinkingElapsed(time.Duration(frame) * thinkingTickInterval)
	return thinkingSpinner(frame, ascii) + " " + thinkingLabel + "  " + elapsed
}

// renderThinkingLine is the chat-row thinking placeholder: spinner + a bright
// window sweeping left→right across the label + elapsed seconds. The label
// chars outside the window keep the dim italic thinking style, chars inside the
// window flip to the accent — an opencode-style light sweep.
func renderThinkingLine(frame int, ascii bool) string {
	var sb strings.Builder
	sb.WriteString(styleThinkingSpin.Render(thinkingSpinner(frame, ascii)))
	sb.WriteString(" ")
	runes := []rune(thinkingLabel)
	// Window start sweeps across and past the label (including an off-label
	// gap on each side) so the highlight re-enters naturally from the left.
	start := frame % (len(runes) + thinkingShimmerWidth - 1)
	for i, r := range runes {
		if i >= start && i < start+thinkingShimmerWidth {
			sb.WriteString(styleThinkingActive.Render(string(r)))
		} else {
			sb.WriteString(styleThinking.Render(string(r)))
		}
	}
	elapsed := formatThinkingElapsed(time.Duration(frame) * thinkingTickInterval)
	sb.WriteString(styleThinking.Render("  " + elapsed))
	return sb.String()
}
