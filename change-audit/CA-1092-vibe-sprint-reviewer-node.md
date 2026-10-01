# CA-1092: vibe-sprint had no reviewer node — coder → validate → synthesis skipped review

Date: 2026-10-01
Refs: live production run-3362 (vibe-tasks, PrivateVault); supersedes the
`TestPack_VibeSprintV2NoReviewerCohort` pinning (written when owner-debate
replaced review — the production requirement is a dedicated review leg).

## Symptom (live)

A vibe-tasks run on 5 existing Tasks never showed a review step: each sprint
ran `contract_plan → contract.freeze → context → tdd → coder → validate →
synthesis → audit`, with `synthesis` doubling as reviewer through a
`--continue--> coder` back-edge. No dedicated reviewer agent ever ran —
unlike `task-harness`/`cp-harness`, which route `validate --done--> reviewer`
and review the diff before synthesis.

## Fix (pack data only — engine already routes this shape)

- `internal/agentpack/flow-pack/flows/vibe-sprint.yaml`: added a `reviewer`
  node (`agent.delegate`, reviewer prompt/tools, cohort join) matching the
  task-harness reviewer wiring, with edges:
  - `validate --done--> reviewer` (validate now routes to review, not straight
    to synthesis),
  - `reviewer --done--> synthesis`,
  - `reviewer --rejected--> coder` is handled by the shared
    `changes_requested` → continue back-edge (`synthesis --continue--> coder`
    retained; reviewer reject resolves through the same join-verdict path —
    the new `validate --continue--> coder` back-edge covers the
    validate-failed retry loop that previously rode synthesis).
- Amended `task323_vibe_sprint_v2_test.go` pins minimally: the
  no-reviewer-cohort pin is superseded by the live requirement; the back-edge
  count now tolerates the new validate→coder edge.

## Verification

- Red→green: `ca1092_vibe_sprint_reviewer_test.go` — pack test asserts the
  reviewer node exists with cohort join + edges; engine test drives a
  vibe-sprint run, settles validate done, and asserts the reviewer child
  spawns.
- `go test ./internal/agentpack -run Task323` + reviewer suite — green.
- `renegotiate_signatures` still routes to `synthesis_negotiation` — verified
  it is not swallowed by the continue back-edge (hubFrom exact-match resolves
  `synthesis→coder` first).
