package app

import (
	"strings"
	"testing"
)

// run-202550: on a missing change-audit-note park, [Retry] continues by
// re-running the writer to write the note — the chip copy must say exactly
// that instead of "run again with old scope". [Allow] stays drift-only.
// New file; no pre-existing test is modified.

func TestBlockedBar_MissingCARetryCopy(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk+"_missing_ca_retry_copy", func(t *testing.T) {
			m := driftBlockedModel(pk, "Audit gate (aggregate): Flow gate: code changed but no change-audit note found.")
			view := stripANSI(m.View())
			if !strings.Contains(view, "[Retry]") {
				t.Fatalf("%s: missing-CA view must contain [Retry]", pk)
			}
			if !strings.Contains(view, "[Stop]") {
				t.Fatalf("%s: missing-CA view must contain [Stop]", pk)
			}
			if strings.Contains(view, "[Allow]") {
				t.Fatalf("%s: missing-CA (non-drift) must NOT have [Allow]", pk)
			}
			if !strings.Contains(view, "missing change-audit note") {
				t.Fatalf("%s: Retry copy must name the missing change-audit note: %q", pk, view)
			}
			if strings.Contains(view, "run again with old scope") {
				t.Fatalf("%s: missing-CA Retry copy must not say old scope: %q", pk, view)
			}
		})
		t.Run(pk+"_generic_escalate_keeps_old_copy", func(t *testing.T) {
			m := driftBlockedModel(pk, "Reviewer requested escalate: missing clamp")
			view := stripANSI(m.View())
			if !strings.Contains(view, "run again with old scope") {
				t.Fatalf("%s: generic escalate must keep the established Retry copy", pk)
			}
			if strings.Contains(view, "missing change-audit note") {
				t.Fatalf("%s: generic escalate must not show the missing-CA copy", pk)
			}
		})
	}
}

func TestIsMissingChangeAuditNoteGate(t *testing.T) {
	for _, yes := range []string{
		"Audit gate (aggregate): Flow gate: code changed but no change-audit note found.",
		"CODE CHANGED BUT NO CHANGE-AUDIT NOTE FOUND",
	} {
		if !isMissingChangeAuditNoteGate(yes) {
			t.Errorf("gate %q must match as missing-CA", yes)
		}
	}
	for _, no := range []string{
		"",
		"flow scope drift: wrote outside the frozen contract's declared paths: a.go",
		"cap 3 reached",
	} {
		if isMissingChangeAuditNoteGate(no) {
			t.Errorf("gate %q must NOT match as missing-CA", no)
		}
	}
}
