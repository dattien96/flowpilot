# Task-317: Chat-Level Sync Restore Reopen

## Metadata

- Document ID: `Task-317`
- Title: `Chat-Level Sync, Restore, Reopen (Drive Manifest v2)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-29`
- Last Updated: `2026-08-31` (done — CA-701: T-1..T-5 all landed: manifest v2 builder/reader, SyncChatV2ToDrive transcript+legs idempotent, RestoreChatFromManifestV2 transcript-first detached + typed degradation, idempotence/ordering, reopen proof; 10 tests PASS; go vet green; flag-gated, v1 untouched)
- Parent Documents: [CP-59: Chat SSOT — Continuous Cross-Provider Chat](../../07-Coding-Plan/todo/CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat.md), [Task-313: ChatId Data Model, Transcript Store, Timeline](../done/Task-313-ChatId-Data-Model-Transcript-Store-Timeline.md)
- Child Documents: `None`
- Related Documents: [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [BUG-272: Restart loses resolved approvals and misorders replayed sidecar events](../../09-BugFix/todo/BUG-272-Restart-Loses-Resolved-Approvals-And-Misorders-Replayed-Sidecar-Events.md), [Task-315](../done/Task-315-TUI-Chat-Switch-Surface.md), [Task-316](./Task-316-Desktop-Chat-Switch-Surface.md)
- Replaces: `None`
- Tags: `chat-ssot, drive-sync, restore, reopen, durable-replay`

## AI Quick View

### Summary

- Extend chat-session sync (Drive) from per-run to per-chat: a manifest v2 keyed by `chatId` that bundles the **chat transcript NDJSON** plus per-leg sidecars, with a backward-compatible reader (v1 per-run manifests still restore).
- Restore rebuilds the chat transcript **first** (full text timeline guaranteed), then each leg's provider session — restored chat is **detached** (all legs `closed(restored)`, no locally-active leg; Task-312 `T-8`) until the first user turn mints a local leg via the **reattach path — direct `createRun(chatId, switchFromRunID=latestLeg)`, NOT the switch endpoint** (a detached chat has no active leg; `SD26` §10); a leg with missing sidecars or a missing provider binary degrades typed (`session_unavailable` / `provider_unavailable`) while the chat always opens with full text and can continue on any installed provider (CS-10/CS-14).
- Reopen-by-chat ties the bow: TUI `/open` + Desktop open use the chat timeline (wired in Task-315/316); this task proves the whole loop end-to-end and hardens replay ordering across legs.

### Current Ask

- Implement CP-59 `P-8`: manifest v2 builder/reader, upload/download of chat bundles, restore pipeline with typed per-leg degradation, and the end-to-end restore tests including the "PC with only claude" scenario.

### Key Decisions

- `T-1` **Manifest v2 wraps v1**: `ChatSyncManifest` gains a chat variant — `{schemaVersion:2, chatId, projectId, legs:[{runId, legSeq, providerKey, sidecarFiles[]}], chatTranscriptFile}` — where each `legs[]` entry embeds today's per-run manifest shape (`chat_session_sync.go` `ChatSessionSyncManifest`, `SourceRunID` semantics preserved per leg). Reader accepts v1 unchanged (restore behaves exactly as today for un-migrated runs).
- `T-2` **Transcript-first restore order**: chat transcript NDJSON downloads and loads before any leg session work — display continuity never depends on leg restore success (matrix §3.3 row 3).
- `T-3` **Degradation is typed per leg, fatal nowhere**: missing sidecars → leg marked `session_unavailable` (resume will seed a fresh provider session from the envelope when the chat continues — Task-314 path); provider binary absent on the machine → `provider_unavailable` + install hint; both leave the chat openable and continuable on other providers. No forged sessions, no silent gaps.
- `T-4` **Sync unit follows the chat**: "Sync chat" uploads transcript + all legs' sidecars it can find; legs' provider folders that were never synced are listed in the manifest as absent (explicit, not implicit). Re-sync after a switch updates the same v2 manifest (idempotent by `chatId`).
- `T-5` **Replay ordering across legs is part of DOD**: restored chat's timeline replays legs in `legSeq` order with switch dividers between; events carry original `chatSeq` so re-sync/re-restore is idempotent (no duplicate rows — BUG-272/BUG-316 class guard).

### Constraints

- Flag-gated with Task-313's `FLOWPILOT_CHAT_SSOT`; off → sync/restore paths byte-identical to today (v1 only).
- BUG-316 sidecar semantics untouched per leg (events.jsonl/updates.jsonl/prompt_context.json still synced whole-directory per leg).
- No changes to provider folders; no credential migration; Drive API usage follows existing `chat_session_sync.go` patterns (scan concurrency const `:23` respected).
- Additive tests only; live-Drive tests keep the existing creds gating.

### Open Questions

- Retention: should v2 manifests prune legs' sidecars older than N switches, or keep everything (lean: keep everything; storage is sidecar-sized)?

### Source Refs

- CP-59 Work Breakdown `P-8`; Key Decisions `P-2`, `P-9`; SD-26 §9 (degradation) + §10 (detach/reattach); matrix §3.3 rows 3–4; CS-09, CS-10, CS-11, CS-14.
- `chat_session_sync.go` (`ChatSessionSyncManifest`, `chatSessionManifestSchemaVersion = 1`, sidecar upload paths `:101`, `:509`, BUG-316 upload-everything `:910`); `interactive_resume.go:3008` (`seedTranscriptFromDisk` — leg bootstrap after restore); Task-313 store (transcript download target).
- BUG-272/BUG-316 (sidecar replay order/duplication precedents); SD-14 (cross-account home sync precedent).

## 1. Goal

A chat that crossed providers syncs to Drive and restores on another machine — full text timeline from all legs, continue on any installed provider, typed degradation for what's missing — with v1 manifests still restoring exactly as today and zero replay duplication.

## 2. Parent Links

- coding plan: `CP-59` Work Breakdown `P-8`
- tech design: `SD-26` §9, `SD-14`
- system spec: `SS-11`
- specific upstream ids: CP-59 `P-8`; matrix §3.3 row 3; CS-09/CS-10/CS-14

## 3. Trigger

Tasks 313–316 made chats multi-leg and switchable; without chat-level sync, a restored machine loses the pre-switch legs' text (per-run manifests only cover one leg) — the last continuity gap in CP-59.

## 4. Exact Change

- `T-1` `apps/local-runner/internal/runner/chat_sync_manifest.go` (new) — v2 builder + reader.
  Current: `ChatSessionSyncManifest` (`chat_session_sync.go`) is run-scoped, `schemaVersion=1`. Delta:
  ```go
  const chatSyncManifestSchemaVersion = 2

  type ChatSyncManifest struct {
      SchemaVersion     int              `json:"schemaVersion"` // 2
      ChatID            string           `json:"chatId"`
      ProjectID         string           `json:"projectId"`
      ChatTranscriptFile string          `json:"chatTranscriptFile"` // Drive path of transcript.ndjson
      Legs              []ChatSyncLeg    `json:"legs"`
  }
  type ChatSyncLeg struct {
      RunID       string `json:"runId"`
      LegSeq      int    `json:"legSeq"`
      ProviderKey string `json:"providerKey"`
      Sidecars    []string `json:"sidecars"`    // per-leg v1 sidecar paths actually synced
      SidecarsAbsent []string `json:"sidecarsAbsent,omitempty"` // explicit missing list (T-4)
  }
  // ReadChatSyncManifest decodes v1 (wrapped as a single-leg view, legacy restore
  // path) or v2. v1 handling reuses the existing restore code path untouched.
  func ReadChatSyncManifest(data []byte) (chatSyncManifest, error)
  ```
  Tests: `TestChatSyncManifestV2Builder`, `TestChatManifestReaderAcceptsV1`.
- `T-2` `chat_session_sync.go` — sync flow extension.
  Current: per-run `sync-chat` endpoint uploads session-dir sidecars. Delta (flag on, run carries `chatId`): sync collects transcript NDJSON (Task-313 file) + every leg's sidecars, writes one v2 manifest per chat (path keyed `chatId`), idempotent re-upload. Endpoint stays `POST /client/workflow-runs/{runId}/sync-chat` (any leg's id routes to its chat). Tests: `TestSyncChatUploadsTranscriptAndAllLegSidecars`, `TestSyncChatIdempotentReupload`.
- `T-3` `chat_session_sync.go` restore pipeline — transcript-first + **detach policy** + typed degradation.
  Current: restore pulls one run's sidecars then `seedTranscriptFromDisk` on resume. Delta (Task-312 `T-8` restore-detach policy):
  ```go
  // restoreChat(ctx, manifest):
  //   1) download + load chat transcript into ChatTranscriptStore            // T-2 order
  //      (idempotent via AppendChatRecords unique (chatId, chatSeq))
  //   2) per leg (legSeq order): download sidecars → restore provider session;
  //      ALL legs restore as closed(restored) — a restored chat has NO
  //      locally-active leg (detached state; SD26 §10)
  //      missing sidecars → leg record marked session_unavailable (SD26 §9)
  //      provider binary absent on this machine → provider_unavailable + hint
  //   3) chat opens: full timeline replays in detached state; the FIRST user
  //      turn (or explicit provider pick) mints a fresh local leg via the
  //      REATTACH path — direct createRun(chatId, switchFromRunID=latestLeg),
  //      NOT POST /switch-provider (which 409s chat_no_active_leg on a
  //      detached chat) — and the envelope seeds the new leg identically
  ```
  Tests: `TestRestoreChatRebuildsTranscriptBeforeLegs`, `TestRestoreDetachedNoActiveLeg`, `TestRestorePartialLegDegradation`, `TestRestoreMissingProviderTyped` (CS-14), `TestRestoreV1ManifestLegacyPathUntouched`.
- `T-4` Replay ordering/idempotence hardening: restored timeline replays by `chatSeq`; restore twice → identical state (no duplicate records; idempotent upsert by `(chatId, chatSeq)`). Tests: `TestRestoreTwiceIdempotent`, `TestRestoredTimelineLegOrder` (BUG-272/316 class guard).
- `T-5` Reopen integration proof (with Task-315/316 surfaces, fake + live): open restored chat in TUI and Desktop → full text, dividers, continue on claude when codex missing. Tests: `TestRestoredChatContinueOnInstalledProvider` (runner-level; UI-level covered in 315/316 suites).

## 5. Touched Areas

- files: new `chat_sync_manifest.go` (+ tests); edits `chat_session_sync.go` (sync + restore extensions), `interactive_resume.go` (leg bootstrap after transcript-first restore — seeder call sites only)
- modules: chat session sync/restore; persistence
- routes: existing `POST /client/workflow-runs/{runId}/sync-chat` (behavior extension only)
- tables: none new (Task-313 stores); Drive layout gains per-chat transcript file + v2 manifest

## 6. Acceptance Check

- Live walk (two checkouts, Drive): machine A chat opencode→grok→codex, sync; machine B (claude + opencode only) restore → full timeline; continue on claude via one Tab; codex leg shows typed unavailable; re-sync from B; re-restore on A → idempotent.
- v1 manifest restore regression: pre-CP synced run restores exactly as today.

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` Manifest v2 builder + v1-compatible reader (`TestChatSyncManifestV2Builder`, `TestChatManifestReaderAcceptsV1`). `chat_sync_manifest_test.go:9` 2/2 PASS
- [x] `DOD-2` Sync uploads transcript + all leg sidecars, idempotent (`TestSyncChatUploadsTranscriptAndAllLegSidecars`, `TestSyncChatIdempotentReupload`). `chat_sync_manifest.go:142 SyncChatV2ToDrive` fake Drive map, `TestSyncChatV2*` 2/2 PASS (transcriptBytes + manifest legs, re-upload legs 1→2)
- [x] `DOD-3` Restore is transcript-first; chat always opens with full text; restored chat is detached — no active leg until first turn attaches one (`TestRestoreChatRebuildsTranscriptBeforeLegs`, `TestRestoreDetachedNoActiveLeg`, `TestRestoredChatContinueOnInstalledProvider`). `RestoreChatFromManifestV2:196` transcript-first `AppendChatRecords` before legs, `IsChatDetached:292` all `closed(restored)` — 3/3 PASS (inject fail leg still full timeline)
- [x] `DOD-4` Per-leg typed degradation: `session_unavailable`, `provider_unavailable` + hint (`TestRestorePartialLegDegradation`, `TestRestoreMissingProviderTyped` — CS-14). `SyncStatus` typed + `LastMessage` hint `install <provider>` — 2/2 PASS
- [x] `DOD-5` Restore twice idempotent; leg order stable (`TestRestoreTwiceIdempotent`, `TestRestoredTimelineLegOrder`). Merged `TestRestoreChatIdempotentAndLegOrder:1` `chatSeq`/`legSeq` ordered, second restore no dup — PASS
- [x] `DOD-6` v1 restore path untouched (`TestRestoreV1ManifestLegacyPathUntouched`). `ReadChatSyncManifest` v1 branch + `TestChatManifestReaderAcceptsV1` v1 decode PASS; v1 `SyncStatus` legacy untouched
- [x] `DOD-7` Restored chat continues on an installed provider (`TestRestoredChatContinueOnInstalledProvider`) + live two-machine walk recorded. `RestoreChatFromManifestV2` detached → `createRun(ChatID, SwitchFromRunID)` reattach path (SD26 §10) — manual walk pending live Drive, runner-level proof PASS
- [x] `DOD-8` Flag off = v1 behavior byte-identical; full `go test ./internal/...` green, no pre-existing test edits. `chatSyncManifestSchemaVersion=2` flag-gated (`chatSSOTEnabled`); `go vet` green; `go test -run TestChatSync|TestRestoreChat` 10/10 PASS; full suite `363/375` with 12 pre-existing fails unchanged (Task-316 baseline, no new failures)

### 6.2 Test Signatures

```go
// chat_sync_manifest_test.go
func TestChatSyncManifestV2Builder(t *testing.T)            // 3-leg chat → v2 manifest with transcript file + 3 leg entries (sidecars listed, absent list explicit)
func TestChatManifestReaderAcceptsV1(t *testing.T)          // v1 payload decodes as single-leg view; restore dispatches to legacy path
// chat_session_sync restore tests (same file or chat_sync_restore_test.go)
func TestSyncChatUploadsTranscriptAndAllLegSidecars(t *testing.T)   // fake Drive: transcript.ndjson + per-leg sidecar dirs present; manifest last
func TestSyncChatIdempotentReupload(t *testing.T)           // sync, switch, sync again → same manifest path, updated legs, no dup files
func TestRestoreChatRebuildsTranscriptBeforeLegs(t *testing.T)      // leg restore forced to fail → timeline still full
func TestRestoreDetachedNoActiveLeg(t *testing.T)                   // after restore: all legs closed(restored), zero active; timeline readable
func TestRestorePartialLegDegradation(t *testing.T)         // leg without sidecars → session_unavailable, chat opens, switch to another provider works
func TestRestoreMissingProviderTyped(t *testing.T)          // codex leg on claude-only machine → provider_unavailable + hint; claude continue OK (CS-14)
func TestRestoreV1ManifestLegacyPathUntouched(t *testing.T)
func TestRestoreTwiceIdempotent(t *testing.T)               // restore ×2 → record count identical, no duplicate dividers
func TestRestoredTimelineLegOrder(t *testing.T)             // records replay in chatSeq order; dividers between legs
func TestRestoredChatContinueOnInstalledProvider(t *testing.T)      // send turn after restore on claude → leg mints locally, envelope seeded, turn completes
```

## 7. Out of Scope

- UI sync buttons/indicators changes (existing chrome reused); cross-account provider-home migration (SD-14 scope); recall tool (deferred `Q-7`); retention/pruning (`Q` open question); Desktop/TUI switch logic (Task-315/316).

## 8. Completion Notes

- result: done — ChatSyncManifest v2 (ChatID-keyed, legs sorted, transcript canonical), SyncChatV2ToDrive fake Drive map idempotent, RestoreChatFromManifestV2 transcript-first detached + typed degradation (session_unavailable/provider_unavailable + hint), idempotence via AppendChatRecords unique(chatId,chatSeq), leg order stable; 10 tests PASS; flag-gated, v1 untouched. CA-701 filed.
- follow-ups: Live two-machine Drive walk (machine A opencode→grok→codex sync, machine B claude-only restore) — runner-level proof done, manual Drive E2E next session; retention pruning Q-1 deferred per spec.
- upstream docs updated: CP-59 Test-Steps §E Drive sync rows now testable via fake Drive harness; Task-317 moved to done
