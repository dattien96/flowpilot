# Phase A review fixes + Phase B (CP-50) walkthrough

## Review fixes (Codex Important)

1. **git commit deny** — `looksLikeGitCommitCommand` skips global opts (`-C`, `-c`, `--git-dir`, …) before the verb.
2. **Stall without Continue** — `maybeScheduleStallCheck` on cohort child events + existing Continue path.
3. **flowStartGitHead persist** — field on session NDJSON + `reconstructRun` restore.

## Phase B (Task-244…247)

| Task | Change |
|------|--------|
| 244 | `canonical.head` first-class source, default set, head-first render |
| 245 | `composeFeatureBlocks` head-first (chat/handoff) |
| 246 | `extractPromptSourcePaths` + `uncommittedChangedPaths` wired in `behaviorContextProduce` |
| 247 | `change.contract` source + `appendChangeContractIfAny` on flow node prompts |

Live E2E DOD items left unmarked (per plan). Unit DODs marked DONE in task docs.
