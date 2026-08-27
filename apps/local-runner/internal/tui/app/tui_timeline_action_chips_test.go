package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

func TestTimelineActionChips_ApprovalRenderAndClick(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width = 100
			m.height = 30
			m.approval = &ApprovalState{
				ID:      "app-123",
				Command: "go test ./...",
				Reason:  "running verification",
			}

			// 1. Verify approval chips render in chat messages timeline
			rows := m.chatRows()
			foundApprove := false
			foundDeny := false
			for _, r := range rows {
				text := stripANSI(r.Text)
				if strings.Contains(text, "[Approve]") || strings.Contains(text, "Approve") {
					foundApprove = true
				}
				if strings.Contains(text, "[Deny]") || strings.Contains(text, "Deny") {
					foundDeny = true
				}
			}
			if !foundApprove || !foundDeny {
				t.Fatalf("%s: expected Approve and Deny chips in chat timeline", pk)
			}

			// 2. Verify composer input bar does not contain approval chips
			c := m.tuiChrome()
			if strings.Contains(c.inputBlock, "Approve") || strings.Contains(c.inputBlock, "Deny") {
				t.Fatalf("%s: input block must not contain approval chips", pk)
			}

			// 3. Verify click targeting finds approve and deny in timeline
			xApprove, yApprove, okApprove := findClickTarget(m, "approve")
			if !okApprove {
				t.Fatalf("%s: approve chip must be clickable in timeline", pk)
			}
			if target := m.clickTargetAt(xApprove, yApprove); target != "approve" {
				t.Fatalf("%s: expected target approve, got %q", pk, target)
			}

			xDeny, yDeny, okDeny := findClickTarget(m, "deny")
			if !okDeny {
				t.Fatalf("%s: deny chip must be clickable in timeline", pk)
			}
			if target := m.clickTargetAt(xDeny, yDeny); target != "deny" {
				t.Fatalf("%s: expected target deny, got %q", pk, target)
			}
		})
	}
}

func TestTimelineActionChips_BlockedContinueStopRenderAndClick(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := blockedChipModel(pk)
			m.width = 100
			m.height = 30

			// 1. Verify blocked chips render in chat messages timeline
			rows := m.chatRows()
			foundContinue := false
			foundStop := false
			for _, r := range rows {
				text := stripANSI(r.Text)
				if strings.Contains(text, "[Retry]") {
					foundContinue = true
				}
				if strings.Contains(text, "[Stop]") {
					foundStop = true
				}
			}
			if !foundContinue || !foundStop {
				t.Fatalf("%s: expected [Continue] and [Stop] chips in chat timeline", pk)
			}

			// 2. Verify composer input bar does not contain blocked chips
			c := m.tuiChrome()
			if strings.Contains(c.inputBlock, "[Retry]") || strings.Contains(c.inputBlock, "[Stop]") {
				t.Fatalf("%s: input block must not contain blocked chips", pk)
			}

			// 3. Verify click targeting finds retry and stop in timeline
			xCont, yCont, okCont := findClickTarget(m, "retry")
			if !okCont {
				t.Fatalf("%s: retry chip must be clickable in timeline", pk)
			}
			if target := m.clickTargetAt(xCont, yCont); target != "retry" {
				t.Fatalf("%s: expected target retry, got %q", pk, target)
			}

			xStop, yStop, okStop := findClickTarget(m, "stop")
			if !okStop {
				t.Fatalf("%s: stop chip must be clickable in timeline", pk)
			}
			if target := m.clickTargetAt(xStop, yStop); target != "stop" {
				t.Fatalf("%s: expected target stop, got %q", pk, target)
			}
		})
	}
}

func TestTimelineActionChips_QuestionOptionsRenderAndClick(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width = 100
			m.height = 30
			m.question = &QuestionState{
				ID:     "q-1",
				Prompt: "Select an option:",
				Options: []map[string]string{
					{"label": "First choice"},
					{"label": "Second choice"},
				},
			}

			// 1. Verify question options render in chat messages timeline
			rows := m.chatRows()
			foundOpt0 := false
			foundOpt1 := false
			for _, r := range rows {
				text := stripANSI(r.Text)
				if strings.Contains(text, "First choice") || strings.Contains(text, "1)") {
					foundOpt0 = true
				}
				if strings.Contains(text, "Second choice") || strings.Contains(text, "2)") {
					foundOpt1 = true
				}
			}
			if !foundOpt0 || !foundOpt1 {
				t.Fatalf("%s: expected options in chat timeline", pk)
			}

			// 2. Verify composer input bar does not contain question options
			c := m.tuiChrome()
			if strings.Contains(c.inputBlock, "First choice") {
				t.Fatalf("%s: input block must not contain question options", pk)
			}

			// 3. Verify click targeting finds option 0 and option 1 in timeline
			xOpt0, yOpt0, okOpt0 := findClickTarget(m, "qopt:0")
			if !okOpt0 {
				t.Fatalf("%s: option 0 must be clickable in timeline", pk)
			}
			if target := m.clickTargetAt(xOpt0, yOpt0); target != "qopt:0" {
				t.Fatalf("%s: expected target qopt:0, got %q", pk, target)
			}
		})
	}
}
