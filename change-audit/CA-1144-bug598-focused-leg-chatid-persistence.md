# CA-1144 — BUG-598: focused child legs paged the parent's transcript (or none)

- **Bug:** BUG-598 (live CP-03 run-150388 / tdd leg run-151518). Chat-backed
  runs write transcript records under `chatId` (`recordChatTranscript` keys
  by `chatRuns.chatIDFor`), while `GET /workflow-runs/{runId}/timeline`
  reads records keyed by `runId` — so every leg with a chat returns
  `records: []` + `chatId: ""` on the run-scoped endpoint. On the desktop
  side, `focusAgentRun` dropped `resumeRun`'s returned `handle.chatId`, so
  `loadEarlierTimeline` paged the parent's chat (or the empty run-scoped
  endpoint) and the focused view rendered nothing even though the leg's
  event stream and `chats/{chatId}/timeline` held real data. Snapshots made
  it worse: `chatId` was not in `RunSnapshot`, so `backToMainRun` could not
  restore the parent's own chat key after a focus cleared it.
- **Fix (UI-side only; runner endpoint unchanged per operator constraint):**
  `RunSnapshot.chatId` added and plumbed through `snapshotRunState` /
  `restoreRunSnapshot`; `focusAgentRun` now persists `handle.chatId` into
  state right after the authoritative resume — chat-backed legs page their
  own transcript via `chatTimeline`, chat-less legs clear the inherited id
  and fall back to run-keyed paging as before.
- **Contract preserved:** main-run flows unaffected (their `chatId` was
  already state); `loadEarlierTimeline`'s `chatId ?? runId` precedence is
  unchanged — this only supplies the correct key it was missing. Runner
  `/timeline` key-mismatch remains a tracked runner-side issue; the desktop
  no longer depends on that endpoint for chat-backed legs.
- **Tests:** typecheck clean; `store.ts` exercised through the existing
  phase-1 suite — 27 failures identical on clean HEAD (baseline, unrelated).
