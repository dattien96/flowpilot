package runner

import (
	"log"
	"os"
	"strings"
	"time"
)

// cohort_diag_log.go is a targeted, opt-in diagnostic log for BUG-236: does a
// review cohort's join genuinely wait for every expected member before the hub
// is reinvoked, and does every child's completion status reach the desktop as
// an agent_graph_updated snapshot? Live testing showed a reviewer's inline card
// still reading "running" in the Agents panel after the flow had already
// completed and approved — this traces the full backend-side truth (cohort
// registration → each member's append → the complete/drain check → the note
// handed to the hub → every agent_graph_updated snapshot's per-run statuses) so
// a captured run's log can be diffed against what the desktop showed, without
// guessing whether the gap is a hub decision made too early or a status update
// that never reached the client.
//
// Off by default — enable with FLOWPILOT_LOG_COHORT_DIAG=1 to capture a repro,
// then share the runner's stdout/log file. Microsecond timestamps are included
// on every line (independent of the global logger's own timestamp prefix) so
// near-simultaneous events (e.g. two reviewers finishing within the same
// second) can still be ordered.

func cohortDiagEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWPILOT_LOG_COHORT_DIAG"))) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func cohortDiagLog(format string, args ...any) {
	if !cohortDiagEnabled() {
		return
	}
	log.Printf("[BUG-236-diag %s] "+format, append([]any{time.Now().Format("15:04:05.000000")}, args...)...)
}
