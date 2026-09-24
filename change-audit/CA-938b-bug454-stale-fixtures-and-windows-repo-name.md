---
id: CA-938b
title: BUG-454 stale-expectation corrections, time-bomb fixtures, and repoNameFromDir Windows-path fix
type: BugFix
feature: dev-infra
date: 2026-09-23
status: done
---

## Context

BUG-454's remaining deterministic baseline reds were a mix of one real
production defect and nine stale/fixture-side failures — all verified red on
clean baseline `d191004f`:

- **Prod defect**: `repoNameFromDir` used `filepath.Base`, which on POSIX
  does not split `\` — `C:\working\flowpilot` returned the full string
  (`TestRepoNameFromDirUsesBasename`).
- **Time-bomb fixtures** (hardcoded `UpdatedAt` drifted past a window):
  `TestLocalFileSessionStoreAgentMetadataRoundTrip` (90-day
  `sessionStoreMaxAge` prune on reload) and
  `TestListRemoteChatSessionsIncludesRecordsFromSameDriveRootWith-
  DifferentProjectIDs` (`ece72e4d` moved the Codex seed to `time.Now()`,
  making the hardcoded Claude `2026-06-19` sort *older* → order flipped).
- **Stale expectations vs intentional contract changes**:
  `TestSupabaseCatalogStoreShaping` (`platform` column added by CP-56
  `4bb804e1`), `TestSkillsMergeClaudeProjectAndProviderHomeWithPrecedence`
  (`.agents/skills` common pack loads for every provider since BUG-062 F-2
  `bbca52dc`), `TestIsFlowPlannerExcludedPathCoversSkillpackScaffold`
  (Markdown exclusion intentional since `e39d261d`),
  `TestTryAdvanceFlowFromNodeBailsOnNonDelegateTarget` (`hub.inline`
  dispatches inline via `dispatchHubNotifyNode` since CP-58 `16e78b20`),
  skillpack count tests (`flow-harness-contract`, `vibe-lanes` added by
  Task-412/CA-903 `dd217c75`), `TestTask330_ResumeFromTdd…` (fixture lacked
  the locked SS artifact Task-327 requires),
  `TestFlowEngineSynthesisPromptIncludesJoinedReviewerNote` +
  E2E review-loop fakes (reviewer adapter must call
  `submit_review_outcome` per the CP-67 machine-verdict contract),
  `TestSpawnChildEmitsGraphAndBusEvents` (bus event is emitted on async
  child completion — the test must poll).

## Change

- `internal/structure/gitnexus*.go` prod: `repoNameFromDir` splits on both
  `/` and `\` so Windows-style paths basename correctly on any host.
- Fixtures/expectations updated to the *current* contract — each with a
  comment naming the commit that changed the contract; no assertion was
  weakened (several were strengthened: planner exclusion now uses
  `docs/report.go`; approval-bar test asserts [stop] hidden per CA-826;
  skills-merge test asserts `flowpilot` source and precedence order).

## Tests

- Each row: focused `go test -count=1 -run <name> ./internal/…` red→green;
  full `./internal/...` rerun leaves only the six env-waived tests plus
  TempDir-cleanup/order flakes already documented in BUG-454.

## Result

- Deterministic BUG-454 suite debt is resolved: 19/19 rows either fixed in
  production or corrected to the intentional current contract, with the
  evidence trail (commit ids) embedded in each fixture.

# ---8<--- flowpilot:change-ledger
feature_key: dev-infra
source_doc_id: BUG-454
change_type: bugfix
summary: repoNameFromDir handles Windows separators; nine stale/time-bomb test fixtures realigned to intentional contract changes (skillpack counts, platform column, common-pack skills, md exclusion, hub.inline dispatch, SS artifact, review-outcome bridge, async bus event)
# --->8---
