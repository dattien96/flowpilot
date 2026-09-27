# BUG-523 — Quota respawn drops a tournament candidate's worktree

## Severity

High — provider routing can break tournament isolation.

## Evidence

- `apps/local-runner/internal/runner/quota_gate.go:326-368`
- `apps/local-runner/internal/runner/interactive_service.go:7734-7739`

When a flow child hits a quota route and `respawnChildOnRoute` creates the replacement, its `SpawnAgentInput` carries provider, model, account, label and prompt, but not `WorkspaceCwd`. A tournament candidate therefore respawns under the tournament parent and inherits the parent's main workspace. The replacement also does not preserve the candidate cohort identity.

## Impact

A provider switch/re-pin during a tournament can move a candidate out of its isolated worktree into the user's checkout. The arbiter continues inspecting the old candidate worktree, so it can score or merge incomplete evidence.

## Missing regression

Create a tournament candidate in a managed worktree, trigger quota rotation before dispatch, and assert that the replacement child preserves:

- the exact candidate worktree cwd,
- node label and cohort identity,
- the pending prompt,
- and zero writes to the main workspace.

## Scope

Capture only. No fix applied during the 2026-09-27 branch review.
