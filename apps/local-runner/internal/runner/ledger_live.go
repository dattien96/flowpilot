package runner

import (
	"path/filepath"

	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
)

// rebuildLedgerIfDirty checks for the post-commit sentinel dropped by
// .git/hooks/post-commit. When present it runs an incremental ledger build
// (parse → enrich → upsert) and then rebuilds the feature catalog so the AI's
// next turn always sees commits made during the current session, not just commits
// that existed at session-start (engine init). Non-fatal: any error is silently
// swallowed so a ledger hiccup never blocks the turn.
func (s *InteractiveService) rebuildLedgerIfDirty(repoDir string) {
	if repoDir == "" {
		return
	}
	dotFlowpilotDir := filepath.Join(repoDir, ".flowpilot")
	rebuilt, err := changeledger.CheckAndRebuild(repoDir, dotFlowpilotDir)
	if err != nil || !rebuilt {
		return
	}
	ledger, err := changeledger.New(dotFlowpilotDir)
	if err != nil {
		return
	}
	_, _ = featurecatalog.Build(repoDir, ledger, dotFlowpilotDir)
}
