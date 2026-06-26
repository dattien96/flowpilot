# Task-163: Chat-Summary Generation Triggers (Context Hardness Phase 2.1)

## Metadata

- Document ID: `Task-163`
- Title: `Chat-Summary Generation Triggers`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-26`
- Last Updated: `2026-06-26`
- Parent Documents: [CP-37: Prompt Context Continuity](../../07-Coding-Plan/done/CP-37-Prompt-Context-Continuity.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-161: Per-Feature Chat-Summary Timeline](Task-161-Per-Feature-Chat-Summary-Timeline.md), [Task-162: Summary-Based Cross-Provider Handoff](Task-162-Summary-Based-Cross-Provider-Handoff.md), [Task-157: Feature-Key Accuracy For History Context](Task-157-Improve-Context-Hardness.md)
- Replaces: `None`
- Tags: `changeledger, chat-summary, summarizer, idle-timer, upsert, bucketing, startup-scan, desktop, runner, no-rag, phase-2`

## AI Quick View

### Summary

- **Refines, does not replace, Task-161.** Task-161 delivered the per-feature chat-discussion timeline, but generated the summary **per turn** (one cheap-model call per content-changing turn) and **appended** a line each time. In real use that surfaced three costs: (1) a model call on every turn, (2) `chat_summary.ndjson` growing one line per turn, and (3) cross-feature mixing when a single chat spanned two features (the summary was fed the whole transcript). This task fixes all three.
- **Three deliberate triggers instead of per-turn.** Generation now fires from: a **5-minute idle timer** (any new turn resets it to zero), a **manual "Gen summary" button** (generate-now, bypassing the wait), and a **one-shot startup background scan** (backfill chats with no summary or a stale hash). Per-turn generation is removed.
- **One rolling row per `(run, feature)`, upserted.** Summaries are rewritten in place (atomic temp+rename), so a chat keeps one line per feature regardless of length; the timeline still surfaces the most-recent chats per feature.
- **Per-feature bucketing — no mixing.** Each turn is bucketed to the feature it belongs to (explicit continuation turns attach to the running feature; greetings/acks/off-topic are dropped, not bucketed); a feature's summary is built only from its own turns.
- **Hashed `state_key`.** The transcript fingerprint is a SHA-256 (was the raw concatenated transcript), so the dedup key is O(1) in size and the file no longer embeds the whole conversation on every line.
- **Best-effort and non-fatal throughout.** Every trigger degrades to the deterministic heuristic summarizer (and to no-op) on any failure; nothing blocks a turn, a switch, or boot.

### Current Ask

- This task is delivered: chat summaries are produced by an idle timer, a manual control, and a startup scan; stored as one upserted row per `(run, feature)`; and built per-feature without mixing.

### Key Decisions

- `T-1` **Idle timer (5 min), reset on activity.** On turn completion arm a per-run timer; a new turn cancels/re-arms it, so the window always counts from the last turn. Window overridable via `FLOWPILOT_SUMMARY_IDLE_MS` (tests). Fires only when the run is still present and idle (not mid-turn / awaiting approval).
- `T-2` **Manual generate endpoint + button.** `POST /client/workflow-runs/{runId}/chat-summary` generates immediately (supersedes the pending timer), returns `409 chat_summary_run_busy` while a turn is active, and is a **no-op when the stored hash already matches**. The desktop renders a "Gen summary" button near the YOLO toggle, enabled only when the chat is idle/completed.
- `T-3` **Startup backfill scan.** One background goroutine at `serve` boot walks persisted root chat runs and generates any missing/stale summary (hash-match → skip; live in-memory runs → skip, owned by their timer/button).
- `T-4` **Upsert per `(run, feature)`.** Replace the append model with `ChatSummaryLedger.UpsertForRun` (rewrite the matching row, atomic). One rolling summary per chat-session per feature.
- `T-5` **Per-feature bucketing.** `bucketTurnsByFeature` groups turns by resolved feature (explicit continuation turns inherit the running feature; greetings/acks/off-topic are dropped); the summary uses only the current feature's turns, and `state_key` is scoped to those turns.
- `T-6` **Hashed `state_key`.** `transcriptStateKey = runID + ":" + sha256(featureTurns)`.

### Constraints

- Reuse Task-161's shared summarizer (`Runner.SummarizeChatTranscript`) unchanged — cheap-tier model of the chat's own provider, heuristic fallback; no second summarizer, no RAG.
- Non-fatal and off the turn path: no trigger may block turn finalization, a provider switch, or server boot.
- Deterministic placement: `feature_key`, time-order, and the `(run, feature)` upsert key stay deterministic; only the summary text is AI-generated (`SD-17 D-4`).
- Per-project, local-first storage; Drive sync continues via `contextsync` `SharedFiles` (`CP-35 §4.8`).

### Open Questions

- `Q-1` Should the idle window (5 min) be user-configurable in the desktop, or is the env override sufficient? (Proposal: env override now; UI later if requested.)
- `Q-2` Should `deleteRun` proactively cancel a pending idle timer (vs. the harmless fire-then-noop today)? (Proposal: harmless now; add cancel if timer churn ever matters.)
- `Q-3` Should the startup scan be bounded (top-N most-recent chats) on very large histories? (Proposal: throttled sequential pass now; cap with a `log()`-style note if needed.)

### Source Refs

- Builds on Task-161 (timeline + shared summarizer) and Task-162 (state-key cache, handoff reuse). `SD-17 §3.2` (chat-summary sibling entry), `§5.1` (on-disk layout), `§6.1` (injection seam). `CP-37 §5` (shared mechanisms), `§7.3` Test F + `§7.4` index (`V-161-12`…`V-161-16`).
- Code: `apps/local-runner/internal/runner/{chat_summary.go,feature_history.go,interactive_service.go,interactive_handlers.go}`, `internal/changeledger/chat_summary.go`, `internal/cli/root.go`; `apps/desktop-flowpilot/src/{components/ChatInput.tsx,state/store.ts,client/HttpWsRunnerClient.ts,client/MockRunnerClient.ts,types/contract.ts,styles.css}`.

## 1. Goal

Replace Task-161's per-turn, append-only chat-summary generation with a triggered, upserted, per-feature model that fixes its three operational costs — per-turn model calls, unbounded file growth, and cross-feature mixing — while keeping the shared summarizer, determinism, and non-fatal posture intact.

## 2. Parent Links

- coding plan: `CP-37` (coordinates the context-injection theme; `§7.2` carries this task's validation cases)
- tech design: `SD-17` §3.2 (chat-summary sibling Change-plane entry), §5.1, §6.1
- system spec: `SS-14` `AC-3` (history awareness), `AC-10` (NL resolution)
- specific upstream ids: phase-2.1 refinement of Task-161; consumes the summarizer and state-key cache from Task-161/Task-162

## 3. Trigger

Task-161 shipped the per-feature chat-discussion timeline but generated the summary on every completed turn and appended a new line each time. Operating it revealed: a cheap-model call per turn (token cost on long sessions), `chat_summary.ndjson` growing one line per turn (and, with the pre-hash `state_key`, embedding the whole transcript per line), and cross-feature mixing when one chat spanned two features. The capability is correct; its *generation cadence, storage shape, and feature scoping* needed to change.

## 4. Exact Change

- `T-1` `runner` — add a per-run idle timer (`scheduleChatSummary`/`cancelChatSummary`, `summaryTimers` map + `summaryMu`). Arm on turn completion; cancel on new turn start; fire `summarizeRunIfIdle` after `summaryIdleWindow()` (5 min default, `FLOWPILOT_SUMMARY_IDLE_MS` override) only when the run is present and idle. Removes the per-turn `recordChatSummaryIfNeeded` call.
- `T-2` `runner` + `desktop` — add `POST /client/workflow-runs/{runId}/chat-summary` → `generateChatSummaryNow`: 409 while running, bypass the timer, no-op on hash match, returns `{runId, generated, skipped, reason}`. Desktop: `ChatSummaryResult` type, `generateChatSummary` client method + store action, and a "Gen summary" button near the YOLO toggle (enabled only when idle/completed).
- `T-3` `runner`/`cli` — `ScanPersistedChatsForSummaries(ctx)` enumerates persisted root chat runs (`SessionIndexReader`), loads + seeds each, and runs the summary core (hash-match → skip; live run → skip). Wired as one background goroutine in the `serve` command.
- `T-4` `changeledger` — `ChatSummaryLedger.UpsertForRun` replaces the `(run_id, feature_key)` row and atomically rewrites the file (temp + rename).
- `T-5` `runner`/`featurecatalog` — `bucketTurnsByFeature` groups turns per resolved feature (explicit continuation turns attach to the running feature; greetings/acks/off-topic are dropped, not bucketed); `runChatSummaryJob` summarizes only the current feature's turns and scopes `state_key` to them.
- `T-6` `runner` — `transcriptStateKey` returns `runID + ":" + sha256hex(latestTranscriptState(featureTurns))`.
- `T-7` tests — `TestGenerateChatSummaryNow` (busy/now/no-op), `TestBucketTurnsByFeatureSeparatesFeatures` (no mixing), `TestRecordChatSummaryRefreshesInPlaceAfterNewTurn` (upsert + hashed key), plus the existing record/inject/handoff suite stays green.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/{chat_summary.go,feature_history.go,interactive_service.go,interactive_handlers.go}`, `internal/changeledger/chat_summary.go`, `internal/cli/root.go`; `apps/desktop-flowpilot/src/{components/ChatInput.tsx,state/store.ts,client/HttpWsRunnerClient.ts,client/MockRunnerClient.ts,types/contract.ts,styles.css}`
- modules: `runner`, `changeledger`, `featurecatalog`, `cli`, desktop chat controller + run state + client contract
- routes: new `POST /client/workflow-runs/{runId}/chat-summary`
- tables: none (sibling NDJSON; Drive sync via `contextsync` `SharedFiles`)

## 6. Acceptance Check

- A chat idle for the window generates its summary once; a new turn resets the countdown; no summary is produced mid-turn.
- The manual button generates immediately when idle, is disabled while running (server returns 409 if forced), and is a no-op when the transcript is unchanged.
- On server start, chats with no/stale summary are backfilled in the background; up-to-date and live runs are skipped.
- `chat_summary.ndjson` holds one rolling row per `(run, feature)`, rewritten in place; `state_key` is a fixed-length hash.
- A two-feature chat produces one summary per feature, each from only that feature's turns (no mixing).
- All generation is best-effort/non-fatal and reuses Task-161's summarizer; no RAG.
- `go test ./internal/{changeledger,featurecatalog,runner}` and desktop `tsc --noEmit` pass.

### 6.1 Definition of Done (DOD)

All items are true for Task-163:

- [x] **DOD-1 (idle timer):** a 5-minute idle timer (env-overridable) generates the rolling summary after the chat is quiet; any new turn resets it; it fires only when the run is present and idle.
- [x] **DOD-2 (manual control):** `POST …/chat-summary` generates now (bypassing the wait), returns 409 while running, and is a no-op on hash match; the desktop button is enabled only when idle/completed.
- [x] **DOD-3 (startup scan):** one boot-time background goroutine backfills missing/stale summaries, skipping up-to-date and live runs.
- [x] **DOD-4 (upsert):** summaries are stored one row per `(run, feature)`, atomically rewritten in place — no per-turn growth.
- [x] **DOD-5 (no mixing):** turns are bucketed per feature; a feature's summary uses only its own turns; explicit continuation turns inherit the running feature, while greetings/acknowledgements/off-topic turns are dropped (not bucketed).
- [x] **DOD-6 (hashed state_key):** `state_key` is a SHA-256 of the feature's transcript turns, O(1) in size, refreshing on change and matching (skip) when unchanged.
- [x] **DOD-7 (non-fatal + reuse):** every trigger degrades to the heuristic and to no-op on failure, never blocking a turn/switch/boot, and reuses Task-161's summarizer unchanged.
- [x] **DOD-8 (tests):** `TestGenerateChatSummaryNow`, `TestBucketTurnsByFeatureSeparatesFeatures`, `TestRecordChatSummaryRefreshesInPlaceAfterNewTurn` pass; the runner + non-runner CP-37 sweeps and desktop typecheck are green.

## 7. Out of Scope

- The summarizer implementation and provider model selection — owned by Task-161.
- The discussion-block injection and feature-history injection — owned by Task-161/Task-157 (this task only changes *production* of the summary, not its consumption).
- RAG / embeddings / semantic recall (stays `SD-10`/CP-10).
- A user-facing idle-window setting and proactive timer cancellation on `deleteRun` (`Q-1`/`Q-2`).

## 8. Completion Notes

- result: done
- implementation notes: idle timer + manual endpoint + startup scan all funnel through the shared `runChatSummaryJob` core (bucket → resolve current feature → hash → upsert), which reuses Task-161's `Runner.SummarizeChatTranscript`. Per-turn generation was removed from the finalize path. Race-safe: the timer/manual paths snapshot the run before the model call.
- follow-ups: `Q-1`/`Q-2`/`Q-3` (idle-window UI, timer cancel on delete, scan bound) remain optional hardening.
- upstream docs updated: `SD-17 §3.2` records the chat-summary sibling entry and points here for the generation triggers; `CP-37 §7.2` carries the validation cases (`V-161-12`…`V-161-16`); `CA-132` records the code change.
