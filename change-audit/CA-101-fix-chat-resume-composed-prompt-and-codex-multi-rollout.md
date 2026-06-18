---
name: CA-101-fix-chat-resume-composed-prompt-and-codex-multi-rollout
description: Fix BUG-083 — chat resume after restart displayed composed CLI prompt instead of raw user input, rendered CLI-injected context as a prompt bubble (Codex), and dropped all Codex turns after the first.
metadata:
  type: change-audit
---

## Change Audit CA-101

**Title:** Fix Desktop Chat Resume — Composed Prompt, Codex Injection Bubble, Multi-Rollout Turns
**Bug:** BUG-083
**Branch:** bug/task-after-app-server
**Date:** 2026-06-18
**Author:** DatNguyen

---

### What Changed

#### New file: `apps/local-runner/internal/runner/turn_log.go`

Defines `TurnLogStore` interface and `turnLogLine` NDJSON type.
Each run gets a sidecar `<runId>-turns.ndjson` in `.flowpilot/chats/` with two kinds of lines:
- `kind:"prompt"` — the raw user input typed at `startTurn`, before FlowPilot appends the `ask_user` reinforcement or skill/MCP preamble.
- `kind:"codex_session"` — the new Codex rollout session id produced after each turn (Codex writes one file per turn with a distinct id).

#### Modified: `apps/local-runner/internal/runner/local_file_session_store.go`

Added `TurnLogStore` implementation (`AppendTurnLog`, `ReadTurnLog`, `DeleteTurnLog`).
Sidecar path: `filepath.Dir(sessions.ndjson) / <runID>-turns.ndjson`.

#### Modified: `apps/local-runner/internal/runner/interactive_service.go`

- **`startTurn`**: after emitting `EventTurnStarted` and persisting session state, appends `{kind:"prompt", prompt:in.Prompt}` to the turn log (best-effort, outside lock).
- **`refreshResumeHandleLocked`**: signature changed to `string` return. It rediscovers the newest Codex rollout id after each turn for sidecar replay, but keeps the durable `realProviderSessionID` stable once known. `lastCodexTurnSessionID` tracks the newest rollout logged to the sidecar so replay can grow without replacing `sessions.ndjson.provider_session_id`.
- **`runTurn`**: captures the returned new Codex session id and appends `{kind:"codex_session", session_id:newID}` to the turn log after unlocking.

#### Modified: `apps/local-runner/internal/runner/interactive_resume.go`

- **`reconstructRun`**: initializes `lastCodexTurnSessionID` from the persisted session id so the stored baseline is treated as already known.
- **`seedTranscriptFromDisk`**: reads the turn log; for Codex, loads the persisted session file first, then de-dupes/appends every per-turn rollout file recorded in the sidecar (F-3); overrides each `turn_started.Prompt` with the stored raw prompt (F-1) for both providers.
- **`deleteChatSession`**: also removes the per-run turn-log sidecar.

#### Modified: `apps/local-runner/internal/runner/codex_event_mapper.go`

Added `isCodexInjectedContext(text string) bool` — detects CLI-injected preamble (`<INSTRUCTIONS>`, `<environment_context>`, `# AGENTS.md instructions for`, `# CLAUDE.md instructions for`).
`mapCodexRolloutLine` now skips `role:user` frames that match this predicate (F-2) instead of emitting them as prompt bubbles.

#### New file: `apps/local-runner/internal/runner/bug083_test.go`

13 new tests:
- F-2 filtering: 3 tests (`TestCodexLoaderFiltersInjectedContextFrame`, `TestCodexLoaderFiltersInstructionsTag`, `TestCodexLoaderDoesNotFilterNormalMentionOfAgentsMd`)
- F-1 prompt override: 1 integration test (`TestSeedTranscriptUsesRawPromptFromTurnLog`)
- F-3 multi-rollout: 2 integration tests (`TestSeedTranscriptLoadsAllCodexRolloutFiles`, `TestSeedTranscriptIncludesStoredCodexSessionWhenTurnLogIsPartial`)
- Stable Codex resume id: 1 regression test (`TestRestoredCodexRunKeepsStablePersistedResumeIDAndLogsNewRollout`)
- `TurnLogStore` unit tests: 3 (`TestTurnLogStoreRoundTrip`, `TestTurnLogStoreMissingFileReturnsNil`, `TestTurnLogStoreDeleteRemovesFile`)
- shared test helpers: `writeLines083`, `writeLinesToPath083`, `writeProviderAccountsConfig083`, `writeCodexRolloutLines`

---

### Defects Fixed

| ID | Defect | Root cause |
|----|--------|-----------|
| F-1 | Composed prompt (+ reinforcement suffix) shown instead of raw input | Resume seeded from provider file which stores composed prompt; no durable raw-input record existed |
| F-2 | CLI-injected AGENTS.md/context rendered as prompt bubble (Codex) | `mapCodexRolloutLine` mapped every `role:user` frame including CLI preamble |
| F-3 | All Codex turns after the first dropped | Codex writes one rollout file per turn; resume loaded only the stored (turn-1) session id's file |
| F-4 | BUG-083 regression could replace stable Codex resume id | Newest per-turn rollout id was being used both as sidecar replay input and durable resume handle |

---

### Backward Compatibility

- Old runs without a turn log: F-1 falls back to showing the composed prompt (unchanged behavior); F-3 falls back to loading the single stored session file (unchanged behavior). F-2 always applies (an improvement for old runs too).
- Legacy runs with a partial turn log: Codex replay still loads the persisted baseline session file before appending sidecar rollout ids.
- Continued/resumed Codex runs keep the stored durable resume id stable; newer rollout ids are sidecar-only replay inputs.
- No migration of `sessions.ndjson` required.
- `fakeWorkflowStore` (used in unit tests) does not implement `TurnLogStore`; all callers use the ok-pattern and treat a missing log as a no-op — tests that do not exercise resume are hermetic.

---

### Test Results

`go test ./internal/runner -count=1` passed: 624 tests, 0 failures.
