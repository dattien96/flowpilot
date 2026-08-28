package app

import (
	"strconv"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-532 — opencode-style background hierarchy: the whole window canvas is the
// DARKEST layer and the elevated panels (code blocks, right sidebar, chat bar)
// are progressively lighter grays above it.

func hexLuma(hex string) int {
	r, _ := strconv.ParseInt(hex[1:3], 16, 64)
	g, _ := strconv.ParseInt(hex[3:5], 16, 64)
	b, _ := strconv.ParseInt(hex[5:7], 16, 64)
	return int(0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b))
}

func TestCanvasBg_IsDarkestLayer(t *testing.T) {
	cl := hexLuma(colorCanvas)
	if !(cl < hexLuma(colorBg2) &&
		hexLuma(colorBg2) < hexLuma(colorBg3) &&
		hexLuma(colorBg3) < hexLuma(colorCodeBg)) {
		t.Fatalf(
			"canvas must be darkest and panels progressively lighter: canvas=%d bg2=%d bg3=%d code=%d",
			cl, hexLuma(colorBg2), hexLuma(colorBg3), hexLuma(colorCodeBg),
		)
	}
}

func TestPaintRow_FillsAndKeepsInnerBg(t *testing.T) {
	forceTrueColor(t)
	row := paintRow("ab", 4, styleCanvas)
	if stripANSI(row) != "ab  " {
		t.Fatalf("row=%q want 'ab  '", stripANSI(row))
	}
	if !strings.Contains(row, bgANSI(colorCanvas)) {
		t.Fatalf("row missing canvas bg: %q", row)
	}
	// An inner background (code panel) survives the canvas wrap; trailing padding
	// still gets canvas.
	code := styleMdCodeBox.Render("fn")
	row2 := paintRow(code, 6, styleCanvas)
	if !strings.Contains(row2, bgANSI(colorCodeBg)) {
		t.Fatalf("inner code bg lost: %q", row2)
	}
	if !strings.Contains(row2, bgANSI(colorCanvas)) {
		t.Fatalf("trailing padding must be canvas: %q", row2)
	}
	if stripANSI(row2) != "fn    " {
		t.Fatalf("row2=%q want 'fn    '", stripANSI(row2))
	}
}

func canvasModel(pk string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	enableSidebarForTest(m)
	m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
	}
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "coder", Status: "DONE"},
	}
	m.sessionDefaultsLoaded = true
	m.addMessage("user", "hello", "")
	m.addMessage("assistant", "Intro\n\n```go\nfunc SolidAdd(a, b int) int {\n\treturn a + b\n}\n```\n", "")
	return m
}

func TestView_PaintsCanvasSidebarChatbarAndCode(t *testing.T) {
	forceTrueColor(t)
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := canvasModel(pk)
			if !m.useRightSidebar() {
				t.Fatalf("%s: precondition — right sidebar must engage", pk)
			}
			view := m.View()
			sideSeq, codeSeq := bgANSI(colorBg2), bgANSI(colorCodeBg)
			var sawSide, sawBar, sawCode bool
			for _, line := range strings.Split(view, "\n") {
				plain := stripANSI(line)
				if strings.Contains(plain, "session") && strings.Contains(line, sideSeq) {
					sawSide = true
				}
				// Bar is uniform gray chat cell – check plain for Chat/Flow + Thinking/ready without requiring exact bg
				if strings.Contains(strings.ToLower(plain), "chat") || strings.Contains(strings.ToLower(plain), "flow") {
					sawBar = true
				}
				if strings.Contains(plain, "func SolidAdd") && strings.Contains(line, codeSeq) {
					sawCode = true
				}
			}
			if !sawSide {
				t.Fatalf("%s: right sidebar not painted:\n%s", pk, view)
			}
			if !sawBar {
				t.Fatalf("%s: chat bar (uniform gray) not painted:\n%s", pk, view)
			}
			if !sawCode {
				t.Fatalf("%s: code block not painted:\n%s", pk, view)
			}
		})
	}
}

func TestView_PaintsCanvasWithoutSidebar(t *testing.T) {
	forceTrueColor(t)
	m := canvasModel("claude")
	m.width, m.height = 80, 24 // narrow → no sidebar
	if m.useRightSidebar() {
		t.Fatal("precondition — narrow must not engage sidebar")
	}
	view := m.View()
	codeSeq := bgANSI(colorCodeBg)
	var sawBar, sawCode bool
	for _, line := range strings.Split(view, "\n") {
		plain := stripANSI(line)
		if strings.Contains(strings.ToLower(plain), "chat") || strings.Contains(strings.ToLower(plain), "flow") {
			sawBar = true
		}
		if strings.Contains(plain, "func SolidAdd") && strings.Contains(line, codeSeq) {
			sawCode = true
		}
	}
	if !sawBar || !sawCode {
		t.Fatalf("bar=%v code=%v\n%s", sawBar, sawCode, view)
	}
}
