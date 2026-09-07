package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
)

// BUG-362 (run-210188): TUI showed Now: plan_synthesis while the live gate
// was a validate escalate, and Retry ("run again with old scope") looped the
// same card on a spec/test conflict. F-2: activeStepName prefers the last
// RUNNING, else the last WAITING. F-3: validate-exhausted parks keep Retry
// for flakes but say old scope fails again, and Enter defaults to Revise.
// New file; no pre-existing test is modified.

func bug362Steps(pairs ...string) []client.WorkflowStepRuntime {
	var out []client.WorkflowStepRuntime
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, client.WorkflowStepRuntime{NodeID: pairs[i], Status: pairs[i+1]})
	}
	return out
}

func TestBug362ActiveStepNamePrefersLiveStep(t *testing.T) {
	for _, tc := range []struct {
		name  string
		steps []client.WorkflowStepRuntime
		want  string
	}{
		// The live shape: dead plan hub WAITING beside live validate WAITING.
		{"dual waiting picks later", bug362Steps("plan_synthesis", "WAITING_USER_APPROVAL", "validate", "WAITING_USER_APPROVAL"), "validate"},
		// RUNNING beats an earlier WAITING.
		{"running beats waiting", bug362Steps("plan_synthesis", "WAITING_USER_APPROVAL", "implement", "RUNNING"), "implement"},
		// RUNNING beats a later WAITING.
		{"running beats later waiting", bug362Steps("validate", "RUNNING", "plan_synthesis", "WAITING_USER_APPROVAL"), "validate"},
		// Last RUNNING wins among several.
		{"last running wins", bug362Steps("implement", "RUNNING", "validate", "RUNNING"), "validate"},
		// Singletons unchanged (legacy behavior).
		{"single running", bug362Steps("my-reviewer", "RUNNING"), "my-reviewer"},
		{"single waiting", bug362Steps("my-reviewer", "WAITING_USER_APPROVAL"), "my-reviewer"},
		{"all done", bug362Steps("plan_synthesis", "DONE", "preflight_contract_freeze", "DONE"), ""},
		{"empty", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := activeStepName(tc.steps); got != tc.want {
				t.Fatalf("activeStepName = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsValidateExhaustedGate(t *testing.T) {
	for _, yes := range []string{
		"Validation failed after the maximum number of retries.",
		"Validation failed after the maximum number of retries.\nLast failure:\n--- FAIL: TestDivide (0.00s)\npanic: division by zero",
		"VALIDATION FAILED AFTER THE MAXIMUM NUMBER OF RETRIES",
	} {
		if !isValidateExhaustedGate(yes) {
			t.Errorf("gate %q must match as validate-exhausted", yes)
		}
	}
	for _, no := range []string{
		"",
		"Reviewer requested escalate: missing clamp",
		"flow scope drift: wrote outside the frozen contract's declared paths: a.go",
		"Plan revised after review (writer round 1)",
		"Audit gate (aggregate): Flow gate: code changed but no change-audit note found.",
	} {
		if isValidateExhaustedGate(no) {
			t.Errorf("gate %q must NOT match as validate-exhausted", no)
		}
	}
}

const bug362ValidateExhaustedReason = "Validation failed after the maximum number of retries.\nLast failure:\n--- FAIL: TestDivide (0.00s)\npanic: division by zero [recovered, repanicked]"

func TestBlockedBar_ValidateExhaustedCopyAndReviseDefault(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk+"_copy", func(t *testing.T) {
			m := driftBlockedModel(pk, bug362ValidateExhaustedReason)
			view := stripANSI(m.View())
			if !strings.Contains(view, "[Retry]") {
				t.Fatalf("%s: validate-exhausted must keep [Retry] for flakes", pk)
			}
			if !strings.Contains(view, "[Revise]") {
				t.Fatalf("%s: validate-exhausted must keep [Revise]", pk)
			}
			if !strings.Contains(view, "re-run unchanged") {
				t.Fatalf("%s: Retry copy must warn old scope fails again: %q", pk, view)
			}
			if strings.Contains(view, "run again with old scope") {
				t.Fatalf("%s: validate-exhausted must not claim old scope: %q", pk, view)
			}
		})
		t.Run(pk+"_revise_default", func(t *testing.T) {
			m := driftBlockedModel(pk, bug362ValidateExhaustedReason)
			m.syncActionRingCard()
			items := m.actionRingItems()
			if len(items) != 3 {
				t.Fatalf("%s: ring items = %d, want 3 (Retry Stop Revise, no drift Allow)", pk, len(items))
			}
			if items[2].label != "[Revise]" {
				t.Fatalf("%s: ring[2] = %q, want [Revise]", pk, items[2].label)
			}
			if m.actionRingIdx != 2 {
				t.Fatalf("%s: actionRingIdx = %d, want 2 (Enter defaults to Revise)", pk, m.actionRingIdx)
			}
		})
		t.Run(pk+"_generic_escalate_unchanged", func(t *testing.T) {
			m := driftBlockedModel(pk, "Reviewer requested escalate: missing clamp")
			view := stripANSI(m.View())
			if !strings.Contains(view, "run again with old scope") {
				t.Fatalf("%s: generic escalate must keep the established Retry copy", pk)
			}
			m.syncActionRingCard()
			if m.actionRingIdx != 0 {
				t.Fatalf("%s: generic escalate Enter must stay on Retry (idx 0), got %d", pk, m.actionRingIdx)
			}
		})
	}
}
