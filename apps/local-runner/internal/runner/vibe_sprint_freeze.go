package runner

import (
	"strings"
	"time"

	"flowpilot-runner/internal/changecontract"
)

// abandonActiveFrozenContractsForRun marks every currently-active frozen
// contract for runID abandoned so the next vibe-sprint freeze can mint a
// new record from the new Task's planner draft.
//
// Recovery/duplicate delivery of the SAME freeze still reuses the active
// record (TestRecoveredFlowReusesPersistedFrozenContract). That path never
// calls this helper. Sequential vibe-sprints on one parent run MUST call it
// before startResolvedFlow — otherwise GetFrozenForStep(run, "coder") keeps
// sprint-1's declared_paths and Task-911/912 cannot write their files
// (live run-654339 / BUG-368).
func abandonActiveFrozenContractsForRun(cwd, runID, reason string) {
	if strings.TrimSpace(cwd) == "" || strings.TrimSpace(runID) == "" {
		return
	}
	store, err := changecontract.NewFrozenStore(cwd)
	if err != nil {
		return
	}
	recs, err := store.ListForRun(runID)
	if err != nil || len(recs) == 0 {
		return
	}
	seenStep := make(map[string]bool, len(recs))
	now := time.Now().UTC()
	if strings.TrimSpace(reason) == "" {
		reason = "vibe-sprint next task"
	}
	for _, rec := range recs {
		step := rec.CoderStepID
		if seenStep[step] {
			continue
		}
		seenStep[step] = true
		active, ok, err := store.GetFrozenForStep(runID, step)
		if err != nil || !ok {
			continue
		}
		_ = store.AppendStatus(active.ContractID, changecontract.ContractStatusAbandoned, reason, now)
	}
}
