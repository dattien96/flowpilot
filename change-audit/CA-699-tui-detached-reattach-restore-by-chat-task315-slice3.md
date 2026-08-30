# CA-699 — TUI detached reattach, /open restore-by-chat, seed-stats divider (Task-315 slice 3)

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: CP-59
change_type: feature
summary: detached chats (restored, no active leg) reattach on the first prompt via startRun carrying chatId+switchFromRunID with persisted-legSeq continuation; /open backfills prior legs' turns + switch dividers from the chat timeline (idempotent, detached-derived); seed-envelope divider formats carried counts from the committed switch stats
# --->8---

## What changed

- Runner: `resolveChatIdentity` explicit-ChatID-without-seq now consults `ChatSessionReader` so a post-restart reattach never reuses an existing legSeq (`TestResolveChatIdentityUsesPersistedLegSeq`).
- TUI: `cmdSendTurn` intercepts detached chats → `cmdReattachChat` (startRun with ChatID+SwitchFromRunID) → `ReattachedMsg` adopts the handle and sends the queued prompt; detached `/provider`//`model` apply locally with a defer notice (no endpoint call); `/open` fetches `GetChatTimeline` and renders PRIOR legs' turns + E-9 dividers (idempotent via `chatBackfillDone`, current leg skipped — its history comes from run replay); the seed-envelope divider formats "carried N[ of M] turns (mode)" from the committed switch stats; detached flag derives from timeline legs.
- 5 new TUI tests + 1 runner test; full suites green (TUI 7 baseline; runner 13 baseline + `TestSyncChatRunToDriveConcurrentIndexMergesPreserveAllParentRows` flake observed once — Drive concurrency test, unrelated paths, stash-probe scheduled with Task-317 which touches that file).

## Falsifiable expectations locked

1. A restored chat's first prompt mints a fresh local leg (chatId+switchFromRunId on the wire) and the prompt sends on it — never a 409 chat_no_active_leg dead end.
2. Detached /provider//model changes apply locally and defer to reattach.
3. /open replay adds prior legs once; re-open does not duplicate.
