# BUG-378: Work committed on the worktree branch is silently dropped by apply_patch merge-back

## Metadata

- Document ID: `BUG-378`
- Title: `worktree Diff uses bare `git diff` — committed branch work invisible, apply_patch reports merged:true while dropping it`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-23`
- Last Updated: `2026-09-23`
- Feature Keys: `run-worktree`
- Parent Documents: CP-83 (worktree isolation), CP-71 (merge-back contract)
- Child Documents: `none`
- Related Documents: CP-83 live validation session 2026-09-23
- Replaces: `none`
- Tags: `worktree, merge-back, data-loss, git-diff`

## AI Quick View

### Summary

- `worktree.Manager.Diff` ran `git add -N . && git diff` — working-tree vs index only. Work **committed on the worktree branch** (providers will `git commit` inside the worktree when instructed; nothing prevents it) never enters the patch.
- Live-verified 2026-09-23 on `:4317`: commit `10255fb` on branch `fp/run-900f1155` → `POST worktree/resolve {"mode":"apply_patch"}` → `{"applied":true,"worktreeState":"merged"}` → main repo unchanged, worktree deleted. Silent data loss.
- Fix: anchor the diff at the recorded base commit (`BaseSidecar`): `git diff --no-color <base>` covers committed-branch + index + working-tree + intent-to-add — the complete delta the merge-back contract promises. Sidecar missing → legacy bare `git diff` fallback preserved.

### Constraints

- safe-fix-contract: additive; all existing worktree tests green unchanged.
- Conflict oracle unchanged (`git apply --check`); StrictHead/tournament semantics untouched.

## 4. Expected vs Actual

- expected: apply_patch merges the complete delta between base and the worktree's current state.
- actual: committed work on the branch vanished; result reported `merged`.

## 5. Root Cause

`internal/worktree/manager.go` `Diff` ignored the recorded base commit (`BaseSidecar` exists and `ApplyWithOptions` already reads it for HEAD-drift detection).

## 6. Fix

`Diff` appends the base commit to the `git diff` args when the sidecar is readable and non-empty; otherwise keeps the bare `git diff` fallback.

## 7. Verification

- New tests (red→green): `TestManager_DiffIncludesCommittedBranchWork` (patch contains committed + uncommitted files), `TestManager_ApplyCarriesCommittedBranchWork` (apply lands the commit's content in the main workspace without creating a commit).
- `go test ./internal/worktree/` — all green.
- Runner worktree suite (`-run 'Worktree|worktree|Cp71|CP71'`): green.
