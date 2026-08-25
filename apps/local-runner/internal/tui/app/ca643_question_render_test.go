package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// CA-643: the question card timeline rendering used one flat --ask purple for
// [QUESTION], the option rows and the "Answered:" confirmation. Each part now
// renders with its own hue so the card is scannable. Additive — legacy
// question/approval tests untouched.

func TestRenderQuestionTextHeadAndBody(t *testing.T) {
	got := renderQuestionText("[QUESTION] Which file should callerSum live in?")
	plain := stripANSI(got)
	if plain != "[QUESTION] Which file should callerSum live in?" {
		t.Fatalf("plain text mutated: %q", plain)
	}
	if !strings.Contains(got, styleQuestionHead.Render("[QUESTION]")) {
		t.Fatalf("head must use styleQuestionHead: %q", got)
	}
	if !strings.Contains(got, styleQuestionBody.Render(" Which file should callerSum live in?")) {
		t.Fatalf("prompt body must use styleQuestionBody: %q", got)
	}
}

func TestRenderQuestionTextOptionRow(t *testing.T) {
	got := renderQuestionText("  1) Keep a thin Subtract compatibility shim — keeps old call sites")
	plain := stripANSI(got)
	want := "  1) Keep a thin Subtract compatibility shim — keeps old call sites"
	if plain != want {
		t.Fatalf("plain text mutated: %q", plain)
	}
	if !strings.Contains(got, styleQuestionHead.Render(" 1)")) {
		t.Fatalf("option index must use styleQuestionHead: %q", got)
	}
	if !strings.Contains(got, styleQuestionOpt.Render("Keep a thin Subtract compatibility shim")) {
		t.Fatalf("option label must use styleQuestionOpt: %q", got)
	}
	if !strings.Contains(got, styleQuestionDesc.Render(" — keeps old call sites")) {
		t.Fatalf("option description must use styleQuestionDesc: %q", got)
	}
}

func TestRenderQuestionTextOptionRowWithoutDescription(t *testing.T) {
	got := renderQuestionText("  2) Expand scope")
	if stripANSI(got) != "  2) Expand scope" {
		t.Fatalf("plain text mutated: %q", got)
	}
	if !strings.Contains(got, styleQuestionOpt.Render("Expand scope")) {
		t.Fatalf("bare option label must use styleQuestionOpt: %q", got)
	}
}

func TestRenderQuestionTextAnswered(t *testing.T) {
	got := renderQuestionText("Answered: Keep a thin Subtract compatibility shim")
	if stripANSI(got) != "Answered: Keep a thin Subtract compatibility shim" {
		t.Fatalf("plain text mutated: %q", got)
	}
	if !strings.Contains(got, styleAnswer.Render("Answered:")) {
		t.Fatalf("answered head must use styleAnswer (green): %q", got)
	}
	if !strings.Contains(got, styleQuestionBody.Render(" Keep a thin Subtract compatibility shim")) {
		t.Fatalf("answer text must use styleQuestionBody: %q", got)
	}
}

func TestRenderQuestionTextAlreadyAnsweredReplay(t *testing.T) {
	got := renderQuestionText("[QUESTION] q-1 already answered: shim")
	if !strings.Contains(got, styleQuestionHead.Render("[QUESTION]")) {
		t.Fatalf("replay head must be styled: %q", got)
	}
}

func TestRenderQuestionTextMultiSelectHint(t *testing.T) {
	got := renderQuestionText("  [multi-select] pick options (click or type 1/2/3), then /submit to send")
	if !strings.Contains(got, styleQuestionDesc.Render("  [multi-select] pick options (click or type 1/2/3), then /submit to send")) {
		t.Fatalf("multi-select hint must use styleQuestionDesc: %q", got)
	}
}

func TestRenderQuestionTextWrappedPromptContinuation(t *testing.T) {
	got := renderQuestionText("because the frozen contract only covers calc.go")
	if !strings.Contains(got, styleQuestionBody.Render("because the frozen contract only covers calc.go")) {
		t.Fatalf("continuation line must use styleQuestionBody: %q", got)
	}
}

func TestRenderQuestionTextKeepsNoBareAnsi(t *testing.T) {
	lines := []string{
		"[QUESTION] a?",
		"  1) one — desc",
		"  2) two",
		"Answered: one",
		"  [multi-select] hint",
		"continuation",
	}
	for _, l := range lines {
		out := renderQuestionText(l)
		if stripANSI(out) != l {
			t.Fatalf("line mutated: in=%q out=%q", l, stripANSI(out))
		}
		if strings.Contains(out, "\x1b[") && lipgloss.Width(out) != lipgloss.Width(l) {
			t.Fatalf("ansi styling changed visual width for %q", l)
		}
	}
}