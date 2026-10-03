package runner

import (
	"strings"

	"flowpilot-runner/internal/changecontract"
)

// BUG-627 (live run-174243): the step-scoped contract-write guards
// (decideReproduceTestLock, decideScaffoldPreExistingLock) bind a child run's
// label/stepID — the hub run has neither (parentRunID is empty, no step
// label), so its writes slid past every contract check. A post-debate
// reprompt then turned the hub into the "materializing node" and it edited a
// file the tdd contract had frozen read-only, which the planner-purity
// fingerprint flagged — the restore↔remediate loop that wedged the sprint.
//
// The contract record is keyed on the flow run id — for a hub that IS the
// run's own id (same insight as decideFrozenContractStateGuard). While any
// active frozen contract exists for the run, a hub write/edit or a mutating
// shell command aimed at a path ANY of those contracts owns (read-only,
// declared, or allowed-extra — every subset reserves the path for its writer
// leg) is silent-denied here: same provider-neutral choke point, likewise
// BEFORE YOLO. The hub orchestrates; it is never a materializing leg.
// Non-contract paths stay writable — docs/audit/notes are the hub's
// legitimate surface. Reads fall through unhandled.
func (s *InteractiveService) decideHubContractScopeLock(rs *interactiveRun, details ApprovalDetails) (decision, reason string, handled bool) {
	if s == nil || rs == nil {
		return "", "", false
	}
	// Hub runs only: children carry parentRunID and are bound by the
	// step-scoped guards upstream.
	if strings.TrimSpace(rs.parentRunID) != "" {
		return "", "", false
	}
	runID := strings.TrimSpace(rs.id)
	if runID == "" {
		return "", "", false
	}
	cwd := strings.TrimSpace(rs.workspaceCwd)
	if cwd == "" {
		return "", "", false
	}
	store, err := changecontract.NewFrozenStore(cwd)
	if err != nil {
		return "", "", false
	}
	recs, err := store.ListActiveForRun(runID)
	if err != nil || len(recs) == 0 {
		return "", "", false
	}

	switch details.Kind {
	case "exec":
		if isReadOnlyCommand(details.Command) {
			return "", "", false
		}
		var owned []string
		for _, rec := range recs {
			owned = append(owned, rec.ReadOnlyPaths...)
			owned = append(owned, rec.DeclaredPaths...)
			owned = append(owned, rec.AllowedExtraPaths...)
		}
		if commandNamesLockedPath(details.Command, owned) {
			return "deny", "hub_write_contract_owned", true
		}
	case "file":
		if isReadOnlyToolName(details.Reason) {
			return "", "", false
		}
		candidate := normalizeReproduceLockCandidate(cwd, details.Command)
		if candidate == "" {
			return "", "", false
		}
		for _, rec := range recs {
			if changecontract.IsOwnedScopePathUnder(rec, candidate, cwd) {
				return "deny", "hub_write_contract_owned", true
			}
		}
	}
	return "", "", false
}
