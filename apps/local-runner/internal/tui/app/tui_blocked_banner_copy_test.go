package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// run-198699 UX: the blocked banner hardcoded "click Retry / Stop / Allow
// above" but Allow only renders for frozen-contract scope drift. A hub_stalled
// / cap / member_stalled park shows only [Retry] [Stop], so the banner must
// not advertise a chip that is not there. New file; no pre-existing test is
// modified.

func blockedBannerModel(pk, reason, gate string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: pk + "-flow"}
	m.runHandle = &client.RunHandle{RunID: "run-1", Status: "running"}
	m.showBlockedBanner(client.AgentLoopState{Status: "blocked", BlockReason: reason, GateReason: gate})
	return m
}

func lastSystemMessage(m *AppModel) string {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Role == "system" {
			return m.messages[i].Content
		}
	}
	return ""
}

func TestBlockedBannerCopyListsOnlyShownChips(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk+"_hub_stalled_no_allow", func(t *testing.T) {
			m := blockedBannerModel(pk, "hub_stalled", "hub has made no progress for 2m0s (no turn, gate, or reinvoke in flight)")
			msg := lastSystemMessage(m)
			if !strings.Contains(msg, "click Retry / Stop above") {
				t.Fatalf("%s: hub_stalled banner = %q, want 'click Retry / Stop above'", pk, msg)
			}
			if strings.Contains(msg, "Allow") {
				t.Fatalf("%s: hub_stalled banner must not advertise Allow: %q", pk, msg)
			}
		})
		t.Run(pk+"_cap_no_allow", func(t *testing.T) {
			m := blockedBannerModel(pk, "cap", "cap 3 reached with 2 open issue(s)")
			msg := lastSystemMessage(m)
			if !strings.Contains(msg, "click Retry / Stop above") {
				t.Fatalf("%s: cap banner = %q, want 'click Retry / Stop above'", pk, msg)
			}
			if strings.Contains(msg, "Allow") {
				t.Fatalf("%s: cap banner must not advertise Allow: %q", pk, msg)
			}
		})
		t.Run(pk+"_drift_allow", func(t *testing.T) {
			m := blockedBannerModel(pk, "escalate", "flow scope drift: wrote outside the frozen contract's declared paths: calc_test.go")
			msg := lastSystemMessage(m)
			if !strings.Contains(msg, "click Retry / Stop / Allow above") {
				t.Fatalf("%s: drift banner = %q, want 'click Retry / Stop / Allow above'", pk, msg)
			}
		})
		t.Run(pk+"_member_stalled_no_allow", func(t *testing.T) {
			m := blockedBannerModel(pk, "member_stalled", "flow scope drift: wrote outside the frozen contract's declared paths: a.go")
			msg := lastSystemMessage(m)
			if strings.Contains(msg, "Allow") {
				t.Fatalf("%s: member_stalled banner must not advertise Allow: %q", pk, msg)
			}
			if !strings.Contains(msg, "click Retry / Stop above") {
				t.Fatalf("%s: member_stalled banner = %q, want 'click Retry / Stop above'", pk, msg)
			}
		})
	}
}