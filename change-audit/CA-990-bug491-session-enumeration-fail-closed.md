# CA-990 — BUG-491: session-index enumeration errors fail closed across resume and pinning

## What changed

`apps/local-runner/internal/runner/`:

- `foreignProviderSessionIDs` (interactive_service.go): propagates the
  `ListAllProviderSessions` error — provider-session ownership checks no
  longer go blind and treat every session as foreign.
- `collectDeleteRunTree` (interactive_resume.go): enumeration error
  propagates instead of deleting a partial tree and orphaning persisted
  children.
- `resumedFlowStepRows`: enumeration error → 502 `session_index_unavailable`
  inside `reconstructRunInternal`.
- `childPendingGateNodeIDs` → `([]string, error)`; unreadable index aborts
  resume (a pending child gate must not look like "no gate").
- `reconstructPendingChildSessions` → `error`; enumeration failure aborts
  the parent resume rather than silently dropping pending/cohort children.
- `persistedLiveCoderExists` → `(bool, error)`; the vibe coder-resume
  caller parks a requirement instead of spawning a duplicate coder.
- `listAgentRunSummaries` → `([]AgentRunSummary, error)`; callers
  (`BuildChatSessionSyncManifest`, `handleListAgentRuns`) surface 502.
- `persistedCompletedChildExists`, `seedIDCounter`,
  `ScanPersistedChatsForSummaries`, `resumedParentAgentAnnotations`: keep
  their safe-direction semantics (block / skip) but now log loudly — the
  blind read is always observable.

## Why

BUG-485/486 hardened the stores to *return* errors; these consumers still
folded `err != nil` into "empty". The worst arms: pinning checks treating
every session as foreign, resume promoting parent nodes while pending
children were never rehydrated, and delete dropping a partial tree.

## Contract

- Reads that decide uniqueness, ownership, or resume state propagate.
- Best-effort enrichments (annotations, summaries, id seeding) keep
  degraded-safe behavior but always log.

## Verification

- `bug491_session_enumeration_swallow_test.go`: pinning, resume rows, and
  delete-tree enumeration faults surface typed errors; healthy stores
  unchanged.
- Runner package suite green (5 env-baseline flakes excluded by clean-tree
  A/B).
