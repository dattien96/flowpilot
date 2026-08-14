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

	// Plain content still carries labels + values (no regression for click/parse).
	if !strings.Contains(plain, "grok-4.5") {
		t.Fatalf("missing model:\n%s", plain)
	}
	if !strings.Contains(plain, "reasoning: high") {
		t.Fatalf("missing reasoning label+value:\n%s", plain)
	}
	if !strings.Contains(plain, "YOLO:ON") {
		t.Fatalf("missing YOLO:\n%s", plain)
	}
	if !strings.Contains(plain, "skills:1") {
		t.Fatalf("missing skills chip:\n%s", plain)
	}
	if !strings.Contains(plain, "7d:87%") {
		t.Fatalf("missing 7d:\n%s", plain)
	}

	// Highlighted segments must use ANSI (styleStatusHi); dim labels alone are not enough.
	if !strings.Contains(got, "\x1b[") {
		t.Fatal("status line should include ANSI highlight codes")
	}
	// model line is built with accent on values — full line must not be plain-only equal.
	modelLine := ""
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(stripANSI(line), "reasoning:") {
			modelLine = line
			break
		}
	}
	if modelLine == "" {
		t.Fatal("model/reasoning row missing")
	}
	if modelLine == stripANSI(modelLine) {
		t.Fatalf("model row should be styled, got plain-only %q", modelLine)
	}
	// "reasoning: " label should remain present in plain; value "high" present.
	if !strings.Contains(stripANSI(modelLine), "reasoning: high") {
		t.Fatalf("model row plain: %q", stripANSI(modelLine))
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
