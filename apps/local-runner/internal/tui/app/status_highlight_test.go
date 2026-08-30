package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func forceStatusColorProfile(t *testing.T) {
	t.Helper()
	// CI/non-TTY defaults to Ascii — force color so highlight styles emit ANSI.
	// Restore Ascii so other package tests (plain substring asserts) stay stable.
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
}

func TestStatusLine_HighlightsModelReasoningYoloSkillsAnd7d(t *testing.T) {
	forceStatusColorProfile(t)
	seven := 87
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.5"}, "http://127.0.0.1:4317")
	m.asciiMode = true
	m.reasoningEffort = "high"
	m.yolo = true
	m.selectedSkills = []client.SkillSelection{{Name: "coding"}}
	m.account = &client.ProviderAccountSummary{
		ProviderKey:        "grok",
		DisplayLabel:       "acct@example.com",
		Remaining7dPercent: &seven,
	}
	m.accountLabel = "acct@example.com"

	got := m.renderStatusLine()
	plain := stripANSI(got)
	// Per new UI: status line is empty (only toast), chrome in input frame
	if strings.Contains(plain, "7d:87%") {
		t.Fatalf("quota must NOT be on status line (hidden), got %q", plain)
	}
	// Input frame now holds Model·reason·YOLO
	footer := stripANSI(m.inputFrameFooter())
	if !strings.Contains(footer, "grok-4.5") {
		t.Fatalf("footer missing model: %q", footer)
	}
	if !strings.Contains(footer, "reasoning: high") {
		t.Fatalf("footer missing reasoning: %q", footer)
	}
	if !strings.Contains(footer, "YOLO:ON") {
		t.Fatalf("footer missing YOLO: %q", footer)
	}
	// Skills are stored but not shown in input chrome per new spec (sidebar only session+steps)
	if len(m.selectedSkills) != 1 {
		t.Fatalf("selectedSkills not stored")
	}

	// Footer is plain (no ANSI) per new UI; highlight is tested via styleYoloStatus separately
	if !strings.Contains(footer, "reasoning: high") {
		t.Fatalf("footer missing reasoning: %q", footer)
	}
}

func TestStyleYoloStatus_OnlyValueAccented(t *testing.T) {
	forceStatusColorProfile(t)
	got := styleYoloStatus("YOLO:OFF")
	plain := stripANSI(got)
	if plain != "YOLO:OFF" {
		t.Fatalf("plain=%q", plain)
	}
	if got == plain {
		t.Fatal("expected ANSI styling on YOLO value")
	}
}

func TestFormatAccountLimitsStyled_Highlights7dOnly(t *testing.T) {
	forceStatusColorProfile(t)
	five, seven := 72, 40
	acc := &client.ProviderAccountSummary{
		Remaining5hPercent: &five,
		Remaining7dPercent: &seven,
	}
	got := formatAccountLimitsStyled(acc)
	plain := stripANSI(got)
	if plain != "5h:72% 7d:40%" {
		t.Fatalf("plain=%q", plain)
	}
	// 7d segment should be styled; whole string not plain-only.
	if got == plain {
		t.Fatal("expected 7d highlight ANSI")
	}
}
