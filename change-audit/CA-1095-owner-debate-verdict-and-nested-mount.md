# CA-1095: owner-debate synthesis lost verdict content + nested debate mounts

Date: 2026-10-01
Refs: live production run-3362 (vibe-tasks, PrivateVault) — owner debates hung
repeatedly; the synthesis kept voting `continue` on empty context and
remounting debates while one was already active.

## Part A — verdict content never reached `debate_synthesis`

Owner children run `posture: verdict_only` and report through
`submit_review_outcome`. Only the status string survived:
`buildCohortNote` rendered `FinalMessage` alone, so the joined note the hub
synthesizes carried no decision content — the "reprompt the coder, fill the
TODO bodies" verdicts never reached it, and it kept voting `continue`.

Fix (`internal/runner`):
- New parent-run map `pendingReviewVerdictDetailByLabel` +
  `reviewVerdictDetailForCohort` renders summary/feedback, per-AC verdict
  rows, and issue titles/locations from the machine verdict.
- `recordReviewCohortMemberVerdict` stores status + detail durably; the
  child-settle path extracts both into `cohortEntry.VerdictDetail`;
  `buildCohortNote` renders `verdict=` + `verdict detail:` per member.
- Callers that only record a status pass empty detail — unchanged behavior.

## Part B — nested owner-debate mounts

`applyVibeDriftOnlyResolver` had the `inDebate` suppression (CA-1063) but the
violation path mounted unconditionally: a gate violation during an active
debate spawned a second debate over the parked sprint.

Fix (`internal/runner/vibe_gate.go`): `stashVibeFlowForDebate` is now the
single atomic mount decision — it reports `false` when a debate is already
mounted, so every trigger path (drift-only and violation) suppresses nested
mounts. The owner-fail retry path stays bounded by `maxVibeOwnerFailRetries`.

## Verification

- Red→green: `ca1095_owner_verdict_note_test.go` (joined note carries verdict
  detail lines) and `ca1095_nested_debate_test.go` (second trigger while a
  debate is mounted does not re-mount).
- `go test ./internal/runner -run 'CA1095'` — green; debate/verdict suites
  green in the package sweep.
