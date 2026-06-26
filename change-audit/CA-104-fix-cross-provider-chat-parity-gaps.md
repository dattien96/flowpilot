# CA-104: Fix Cross-Provider Chat Parity Gaps

## Scope

Correct Claude account-switch documentation and fix provider-neutral Remote Chats discovery and old-chat model selection.

## Completed

- Documented the existing Claude connected-account fallback and original-turn retry behavior in `Task-018`.
- Removed machine-local project-id filtering from the already project-scoped chat-sync Drive root.
- Synchronized the desktop selected model with the provider of a resumed history run.
- Added focused desktop and local-runner regression tests.
- Recorded the correction in `BUG-090`.

## Verification

- Desktop TypeScript typecheck passed.
- Desktop store tests passed: `21/21`.
- Targeted Remote Chats local-runner tests passed.
- Full local-runner package was attempted and exposed unrelated environment-sensitive failures.

## Residual Notes

- Claude account choice is deterministic by slot when quota telemetry is unavailable; it is not a quota ranking.
- The repository Phase 1 Node test script still has environment/path resolution failures unrelated to this change.

# ---8<--- flowpilot:change-ledger
feature_key: cross-provider-handoff
source_doc_id: TASK-018
change_type: fix
summary: Fix Cross-Provider Chat Parity Gaps
# --->8---
