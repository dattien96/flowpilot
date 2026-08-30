# Task-313: ChatId Data Model, Transcript Store, Timeline

## Metadata

- Document ID: `Task-313`
- Title: `ChatId Data Model, Transcript Store, Timeline`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-29`
- Last Updated: `2026-08-30` (done — CA-693 substrate + CA-694 completion; 24 tests green; R1: baseline 13 failures + 2 flakes proven pre-existing on stashed clean tree, zero new)
- Parent Documents: [CP-59: Chat SSOT — Continuous Cross-Provider Chat](../../07-Coding-Plan/todo/CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat.md), [Task-312: Chat SSOT Design Freeze (SD-26)](./Task-312-Chat-Ssot-Design-Freeze-SD26.md)
- Child Documents: `None`
- Related Documents: [Task-078: Cross-Provider Chat Handoff](../done/Task-078-Cross-Provider-Chat-Handoff.md), [SD-26: Chat Continuity SSOT](../../06-System-Tech-Design/SD-26-Chat-Continuity-Ssot.md) (authored by Task-312)
- Replaces: `None`
- Tags: `chat-ssot, chatId, transcript-store, timeline, runner, durable-replay`

## AI Quick View

### Summary

- Land the chat substrate: `chatId`/`legSeq` minted on `normal_chat` runs and carried on `StartRunInput`/`RunHandle`; a durable per-chat transcript store (local NDJSON + Supabase) recording the full normalized event stream (turns, tool calls, approvals, questions, usage); and the read model `GET /client/chats/{chatId}/timeline` that joins legs into one ordered view.
- Nothing behavioral changes yet: no switch endpoint, no UI routing. Legacy single-run chats must behave byte-identically (chatId self-tag is invisible).
- This task's store is what Task-314's envelope and Task-315..317's UIs/restore read from.

### Current Ask

- Implement CP-59 `P-2` + `P-3`: chat tagging on runs, `ChatTranscriptStore` (local + Supabase), record capture on the existing persist path, and the timeline endpoint with tail budgets — all behind the `FLOWPILOT_CHAT_SSOT` flag (inert when off).

### Key Decisions

- `T-1` **chatId mint/adopt point is `createRun`** (the shared internal entry behind `handleStartRun`, `interactive_handlers.go:224`): when `ChatMode=="normal_chat"` and no `ChatID` supplied → mint `cht_<12-hex>`; when `SwitchFromRunID` supplied → adopt that run's `chatId` and set `legSeq = source.legSeq + 1`, source resolved **before** `createRun` takes `s.mu` (`stampAccount` pattern, `interactive_handlers.go:792`). Task-314's switch and the Task-317 reattach path are the callers of the adopt branch. Identical to §4 `T-1` (review I-R1 alignment).
- `T-2` **Record capture rides the existing persist hook** (`interactive_service.go:3947 persistEvent`): a sibling `recordChatTranscript` maps the same normalized `ProviderEvent`s to chat records with the identical delta-skip policy (`EventMessageDelta` skipped, anchor `:3949`) — one mapping table, no second event vocabulary.
- `T-3` **Local store = per-chat NDJSON, O_APPEND** (crash-safe, mirrors flow-events sidecar `local_file_session_store.go:657`); Supabase store = `workflow_chat_events` additive table. Both behind `ChatTranscriptStore` (SD26-D-1).
- `T-4` **Legacy self-tag is lazy and non-blocking**: on first touch (load, timeline read, or switch), an untagged chat-kind run self-tags `chatId=runId, legSeq=0`; legacy chat *transcript* backfill into the store is best-effort raw mode (from `transcriptTurnsFromRun`, `handoff_context.go:156`) and never blocks the turn.
- `T-5` **RunHandle carries `ChatID`+`LegSeq`** (additive JSON fields — `client.go` `RunHandle` mirrors `:260`); desktop/TUI type mirrors land with their own tasks (Task-315/316) to keep this task runner-scoped.

### Constraints

- Flag `FLOWPILOT_CHAT_SSOT` (default off) gates all new behavior; flag off = byte-identical behavior (chatId fields absent from responses is acceptable; presence is additive JSON either way).
- Additive-only: `StartRunInput` (`provider_event.go:296`) and `RunHandle` gain **appended** fields; `workflow_provider_sessions` gains appended columns (`supabase_workflow_store.go:62` column list extended at the end); no existing column reordered.
- No UI, no switch endpoint, no posture change — Task-314+.
- Do not touch provider folder seeders (`interactive_resume.go:3008`) — they remain the leg-resume bootstrap.
- Old tests untouched (additive tests only).

### Open Questions

- Whether `latestChatSeq` should also be exposed on the timeline response for incremental polling (default: yes, `nextSeq`) — confirm during review.

### Source Refs

- CP-59 Work Breakdown `P-2`, `P-3`; Key Decisions `P-1`, `P-2`; SD-26 `SD26-D-1..D-5`, `SD26-S-1`.
- `interactive_service.go:161` (`runKind`), `:3565` (`persistenceStore`), `:3947` (`persistEvent`), `:3667` (`RunKind` in session record); `provider_event.go:296` (`StartRunInput`); `interactive_handlers.go:224` (`handleStartRun`), `:962` (`runHistoryItem`).
- `local_file_session_store.go:661` (`AppendEvent` + `LoadFlowEvents` 1 MiB reader pattern); `supabase_workflow_store.go:544` (`AppendEvent`), `:62` (`providerSessionSelect` columns), `:656` (`ProviderSessionState`).
- `handoff_context.go:156` (`transcriptTurnsFromRun` — legacy backfill source); `tui/app/chat_history_replay.go:6` (tail budget pattern); Task-312 `SD26-D-1..D-4`, `SD26-E-1..E-9`, `SD26-X-1`, `T-7` degraded-store contract.

## 1. Goal

A chat kind run carries `chatId`/`legSeq` end-to-end; every normalized turn/approval/question/tool event of that run is durably recorded into a per-chat transcript (local NDJSON + Supabase); `GET /client/chats/{chatId}/timeline` returns the joined, ordered view (legs + records) with tail/chunk budgets — flag-gated, zero behavior change when off, legacy chats self-tag on touch.

## 2. Parent Links

- coding plan: `CP-59` Work Breakdown `P-2` (data model), `P-3` (store + timeline)
- tech design: `SD-26` (`SD26-D-1..D-5`, `SD26-S-1`), `SD-06`
- system spec: `SS-05`, `SS-11`
- specific upstream ids: CP-59 `P-2` mint/tag/backfill, `P-3` store/timeline; SD26-D-1 store interface, SD26-D-4 chatSeq, SD26-X-1..X-2 errors

## 3. Trigger

Task-312 froze the contracts. Every later slice (switch, UIs, restore) reads the timeline and tags legs — without this substrate they have nothing to read or tag. Sequenced first per CP-59 §3 sequencing logic ("make the chat transcript durable first").

## 4. Exact Change

- `T-1` `apps/local-runner/internal/runner/chat_ssot.go` (new) — chat identity + leg fields.
  Current: `interactiveRun` has no chat concept; `runKind` at `interactive_service.go:161` is the only chat marker. Delta:
  ```go
  // chat_ssot.go
  const (
      LegStateActive = "active"
      LegStateClosed = "closed"
  )
  const (
      LegClosedReasonProviderSwitch        = "provider_switch"
      LegClosedReasonChatEnded             = "chat_ended"
      LegClosedReasonRestored              = "restored"
      LegClosedReasonProviderSwitchRolledBack = "provider_switch_rolled_back"
  )

  // appended to interactiveRun (interactive_service.go, after runKind at :161):
  chatID          string
  legSeq          int
  legState        string // "" == legacy-untagged; normalized by ensureChatTagging
  legClosedReason string
  switchFromRunID string // durable switch intent (SD26-S-2 phase A)

  func newChatID() string {
      b := make([]byte, 6)
      if _, err := crypto_rand.Read(b); err != nil {
          return fmt.Sprintf("cht_%d", time.Now().UnixNano())
      }
      return "cht_" + hex.EncodeToString(b)
  }

  // ensureChatTagging — legacy self-tag (SD26 §2): lazy, idempotent, called on
  // load / timeline read / switch entry. Caller holds s.mu.
  func ensureChatTagging(rs *interactiveRun) {
      if rs.runKind != "chat" || rs.chatID != "" {
          return
      }
      rs.chatID = rs.id
      rs.legSeq = 0
      if rs.legState == "" {
          rs.legState = LegStateActive
      }
  }
  ```
  Mint/adopt lives **inside `createRun`** (the shared internal entry behind `handleStartRun`, `interactive_handlers.go:224`) so the Task-314 switch path gets identical tagging: when `in.ChatMode=="normal_chat"`, resolve `in.ChatID` (or, when `in.SwitchFromRunID` is set, look up the source leg's chatId and `legSeq+1`) **before `createRun` takes `s.mu`** — the same resolve-before-lock pattern as `stampAccount` (`interactive_handlers.go:792`); `createRun` then stamps the fields on the new `interactiveRun` and persists them (`:3667` region gains `chat_id`,`leg_seq`,`leg_state`,`leg_closed_reason` keys; `ProviderSessionState` at `supabase_workflow_store.go:656` gains mirrored appended fields). Tests: `TestChatIDMintedOnFirstNormalChat`, `TestChatIDAdoptedFromSwitchFromRunID`, `TestLegacyRunSelfTagsChatIDOnLoad`.
- `T-2` `apps/local-runner/internal/runner/provider_event.go:296` + `apps/local-runner/internal/tui/client/client.go` — additive DTO fields.
  Current: `StartRunInput{ProjectID, WorkflowID, StepID, ProviderKey, Model, YoloMode, ReasoningEffort, ChatMode, Cwd}`; `RunHandle{RunID, ProviderSessionID, ProviderKey, Status, StepID, LastEventSeq, RunKind, WorkflowID, FlowRef}`. Delta (append at struct end, never reorder):
  ```go
  // provider_event.go:296 StartRunInput — appended
  ChatID          string `json:"chatId,omitempty"`
  SwitchFromRunID string `json:"switchFromRunId,omitempty"`
  LegSeq          int    `json:"legSeq,omitempty"`

  // client.go RunHandle — appended (runner echoes in handleStartRun response)
  ChatID string `json:"chatId,omitempty"`
  LegSeq int    `json:"legSeq,omitempty"`
  ```
  The TUI's own `StartRunInput` mirror (`tui/client/client.go:377`) gains the same `ChatID`/`SwitchFromRunID`/`LegSeq` appended fields — the Task-315/317 reattach path sends them from the client. `handleStartRun` response gains `chatId`/`legSeq` echo. Base-regression: JSON of old field set byte-identical when new fields empty (`omitempty`). Tests: `TestStartRunInputChatFieldsAdditiveJSON`, `TestRunHandleChatFieldsAdditiveJSON`.
- `T-3` `apps/local-runner/internal/runner/chat_transcript_store.go` (new) — store interface + local NDJSON impl.
  Current: no chat-level durable store; `localFileSessionStore.AppendEvent` (`local_file_session_store.go:661`) persists only flow-sidecar types. Delta:
  ```go
  // chat_transcript_store.go
  type ChatTranscriptRecord struct {
      ChatID   string          `json:"chatId"`
      ChatSeq  int64           `json:"chatSeq"`
      LegRunID string          `json:"legRunId"`
      Type     string          `json:"type"` // SD26-E-1..E-9
      Payload  json.RawMessage `json:"payload"`
  }

  type ChatTranscriptStore interface { // SD26-D-1 — append idempotent on (chatId, chatSeq)
      AppendChatRecords(ctx context.Context, recs []ChatTranscriptRecord) error
      ReadChatRecords(ctx context.Context, chatID string, afterSeq int64, limit int) ([]ChatTranscriptRecord, error)
      LatestChatSeq(ctx context.Context, chatID string) (int64, error)
  }

  // localFileChatTranscriptStore — one NDJSON file per chat, O_APPEND writer,
  // line reader with 1 MiB buffer (copy LoadFlowEvents pattern).
  // Path rule: <localSessionDir>/chats/<chatId>/transcript.ndjson
  // Idempotency: ReadChatRecords latest seq is checked before append; records
  // whose (chatId, chatSeq) already exist are skipped — Task-317 restore reuses
  // this as its upsert.
  ```
  Append is single-writer (service goroutine under `s.mu`); `chatSeq` allocated by the service (`nextChatSeqLocked(chatID)`) at append time, never by the store. Corrupt trailing line → skip with log (mirror `LoadFlowEvents` malformed-line policy). Tests: `TestLocalChatTranscriptAppendReadRoundTrip`, `TestLocalChatTranscriptAppendIdempotentOnDuplicateSeq`, `TestLocalChatTranscriptCorruptTailSkipped`, `TestChatSeqMonotonicUnderConcurrency` (10 goroutines → 10 distinct ascending seqs).
- `T-4` `apps/local-runner/internal/runner/chat_transcript_store_supabase.go` (new) — Supabase impl over `workflow_chat_events(chat_id text, chat_seq bigint, leg_run_id text, type text, payload jsonb, created_at timestamptz, unique(chat_id, chat_seq))`.
  Current: `SupabaseWorkflowStore.AppendEvent` (`supabase_workflow_store.go:544`) proves the REST insert pattern. Delta: `AppendChatRecords` → POST `/rest/v1/workflow_chat_events` with `Prefer: resolution=ignore-duplicates` (idempotent on the unique key — restore upsert semantics); `ReadChatRecords` → GET with `chat_id=eq.&chat_seq=gt.&order=chat_seq.asc&limit=`; `LatestChatSeq` → `select=chat_seq&order=chat_seq.desc&limit=1`. Tests: `TestSupabaseChatTranscriptStoreRoundTrip` (skips without creds, matching existing Supabase test gating).
- `T-5` `apps/local-runner/internal/runner/interactive_service.go` — record capture hook next to `persistEvent` (`:3947`).
  Current: `persistEvent` skips `EventMessageDelta` (`:3949`) and fans to the persistence store. Delta: sibling method called from the same sites that call `persistEvent` (turn start/complete, message completed, tool, file change, approval resolved, question answered, usage) gated on flag + `rs.chatID != ""`:
  ```go
  // recordChatTranscript mirrors persistEvent's skip policy exactly and never
  // fails the turn (SD26 §9 chat_store_degraded, Task-312 T-7).
  func (s *InteractiveService) recordChatTranscript(rs *interactiveRun, event ProviderEvent) {
      if !s.chatSSOTEnabled() || rs == nil || rs.chatID == "" {
          return
      }
      if event.Type == EventMessageDelta { // same skip as persistEvent :3949
          return
      }
      recs := chatRecordsFromProviderEvent(rs, event) // mapping table SD26 §6, E-1..E-9
      if err := s.chatTranscriptStore().AppendChatRecords(context.Background(), recs); err != nil {
          if !rs.chatStoreDegraded {
              rs.chatStoreDegraded = true
              // one-time system notice on the run stream; timeline responses
              // carry degraded:true until an append succeeds again
          }
          return
      }
      rs.chatStoreDegraded = false
  }
  ```
  Mapping table (SD26-E-1..E-9): `EventTurnStarted→turn_started(payload:{prompt, isHandoffSeed?})`, `EventMessageCompleted/EventTurnCompleted→message_completed(payload:{text})`, tool started/completed → `tool_started/tool_completed(payload:{title,kind,ok})`, `EventFileChanged→file_changed(payload:{path,oldText?,newText?})`, approval/question resolve → `approval_resolved/question_answered(payload:{decision|answer,id})`, token usage → `token_usage(payload:{last,total,contextWindow})`. Legacy backfill: on first timeline read of a legacy chat, synthesize records from `transcriptTurnsFromRun` (raw mode, one-shot, guarded by a `backfilled` marker record). Tests: `TestRecordChatTranscriptSkipsDeltas`, `TestRecordChatTranscriptCapturesTurnToolFileApprovalQuestionUsage`, `TestRecordChatTranscriptDegradedFlagOnAppendFailure`, `TestLegacyChatBackfillRawOneShot`.
- `T-6` `apps/local-runner/internal/runner/interactive_handlers.go` — timeline endpoint.
  Current: no chat routes (run routes at `:29`, `:209`). Delta:
  ```go
  mux.HandleFunc("GET /client/chats/{chatId}/timeline", s.handleChatTimeline)

  type chatLegView struct {
      RunID           string `json:"runId"`
      ProviderKey     string `json:"providerKey"`
      LegSeq          int    `json:"legSeq"`
      LegState        string `json:"legState"`
      LegClosedReason string `json:"legClosedReason,omitempty"`
  }
  type chatTimelineResponse struct {
      ChatID    string                 `json:"chatId"`
      Legs      []chatLegView          `json:"legs"`
      Records   []ChatTranscriptRecord `json:"records"`
      NextSeq   int64                  `json:"nextSeq"`
      Truncated bool                   `json:"truncated"`
  }
  ```
  Handler: `ensureChatTagging` on all resident runs of the chat (+ `loadPersistedRun` for non-resident legs, `interactive_resume.go:4784`), join legs sorted by `legSeq`, records via `ReadChatRecords(afterSeq, limit)` with tail budget mirroring `chatReplayTailEventBudget` (`chat_history_replay.go:6`). Errors: unknown chat → `chat_not_found` 404 (SD26-X-1). Tests: `TestChatTimelineJoinsLegsInOrder`, `TestChatTimelineTailBudgetTruncatesOldest`, `TestChatTimelineUnknownChat404`, `TestChatTimelineSelfTagsLegacyRun`.
- `T-7` `apps/local-runner/internal/runner/runner.go` — flag + wiring.
  Current: provider agent flags pattern (`grokAgentEnabled`, `opencodeAgentEnabled`). Delta: `chatSSOTEnabled()` reading `FLOWPILOT_CHAT_SSOT` (default off) copying that pattern; `chatTranscriptStore()` accessor returning local or Supabase impl by `persistenceStore()` kind (`interactive_service.go:3565`). Tests: `TestChatSSOTFlagDefaultOff`, `TestChatSSOTFlagOptIn`.

## 5. Touched Areas

- files: new `chat_ssot.go`, `chat_transcript_store.go`, `chat_transcript_store_supabase.go` (+ `_test.go` each); edits `interactive_service.go` (`interactiveRun` fields, capture hook, accessors), `interactive_handlers.go` (route + decode/echo), `provider_event.go:296` (appended fields), `runner.go` (flag), `supabase_workflow_store.go` (`ProviderSessionState` appended fields + `workflow_chat_events` client), `local_file_session_store.go` (chat dir helper)
- modules: runner interactive service + persistence
- routes: `GET /client/chats/{chatId}/timeline` (new)
- tables: `workflow_provider_sessions` additive columns (`chat_id`,`leg_seq`,`leg_state`,`leg_closed_reason`); new `workflow_chat_events`

## 6. Acceptance Check

- With flag on: a normal chat run mints `cht_*`, echoes it on `RunHandle`, and its turns/tool/approval/question events land in `transcript.ndjson` with monotonic `chatSeq`; timeline returns them joined with the leg view.
- With flag off: responses identical to baseline (additive fields empty), no NDJSON written, no route behavior change beyond 404 for unknown chat.
- Legacy run self-tags on first timeline read; backfill happens once (marker record).
- Full `go test ./internal/...` green; no pre-existing test edited.

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` chatId mint/adopt/self-tag proven via real `createRun` (`TestChatIDMintedOnFirstNormalChat`, `TestChatIDAdoptedFromSwitchFromRunID`) + unit (`TestEnsureChatTaggingLegacySelfTag`).
- [x] `DOD-2` Additive JSON fields, old payload byte-identical when empty (`TestChatFieldsAdditiveJSON` pins both DTOs).
- [x] `DOD-3` Local NDJSON store round-trip + idempotent duplicate-seq append + corrupt-tail skip + monotonic seq under concurrency (`TestLocalChatTranscript*`, `TestChatSeqMonotonicUnderConcurrency`).
- [x] `DOD-4` Supabase store implemented + **live round-trip PASS** on the real project post-migration (2026-08-30): append ×2 replay = 3 rows (ignore-duplicates on the unique key), afterSeq pagination, LatestChatSeq seed; test is creds-gated (skips without SUPABASE_API_URL/SUPABASE_SERVICE_ROLE_KEY). Boundary fix landed with it: workflow_chat_events columns are snake_case — `dbChatEventRow` converts at the store boundary (public record JSON stays camelCase).
- [x] `DOD-5` Capture hook mirrors `persistEvent` skip policy; turns/tools/files/approvals/questions/usage captured per `SD26-E-1..E-9` (`TestRecordChatTranscriptCapturesTurnToolFileApprovalQuestionUsage`).
- [x] `DOD-5b` Append failure never fails the turn: registry degraded flag (`TestRecordChatTranscriptDegradedFlagOnAppendFailure`).
- [x] `DOD-6` Timeline joins legs by `legSeq`, paginates `afterSeq`/`limit` + `truncated` (`TestChatTimelineJoinsLegsInOrderAndSelfTags`, `TestChatTimelineTailPaginationTruncates`).
- [x] `DOD-7` Typed 404s: `chat_not_found` unknown + `chat_ssot_disabled` flag-off (`TestChatTimelineUnknownChat404`, `TestChatTimelineFlagOffDisabled404`).
- [x] `DOD-8` Legacy backfill raw one-shot, marker-guarded (`TestLegacyChatBackfillRawOneShot`).
- [x] `DOD-9` Flag default off; off = baseline behavior (`TestChatSSOTFlagDefaultOffAndOptIn` + full-suite R1 diff vs stashed-clean baseline).
- [x] `DOD-10` Migrations additive: `20260830080000_chat_ssot_chat_columns_and_events.sql` (5 session columns + `workflow_chat_events`, PK `(chat_id, chat_seq)`).

### 6.2 Test Signatures

```go
// chat_ssot_test.go
func TestChatIDMintedOnFirstNormalChat(t *testing.T)                 // startRun normal_chat no chatId → handle.ChatID matches ^cht_[0-9a-f]{12}$; second run w/ same ChatID → legSeq increments
func TestChatIDAdoptedFromSwitchFromRunID(t *testing.T)              // StartRunInput{ChatID:"cht_x", SwitchFromRunID:"run-1"} → new run adopts chatId, legSeq=source+1
func TestLegacyRunSelfTagsChatIDOnLoad(t *testing.T)                 // persisted pre-CP run loaded → chatID==runID, legSeq==0, idempotent on second call
func TestStartRunInputChatFieldsAdditiveJSON(t *testing.T)           // json.Marshal without chat fields == baseline snapshot
func TestRunHandleChatFieldsAdditiveJSON(t *testing.T)

// chat_transcript_store_test.go
func TestLocalChatTranscriptAppendReadRoundTrip(t *testing.T)        // 5 records appended → ReadChatRecords(after 0, 100) returns 5 in chatSeq order
func TestLocalChatTranscriptAppendIdempotentOnDuplicateSeq(t *testing.T) // append same (chatId, chatSeq) twice → read returns one copy (restore upsert semantics)
func TestLocalChatTranscriptCorruptTailSkipped(t *testing.T)         // append garbage bytes → read returns valid prefix, no error, log emitted
func TestChatSeqMonotonicUnderConcurrency(t *testing.T)              // 10 goroutines append → seqs strictly increasing, none lost
func TestSupabaseChatTranscriptStoreRoundTrip(t *testing.T)          // creds-gated; insert 3 → read back ordered; ignore-duplicates on unique(chat_id, chat_seq)

// interactive chat capture (in chat_ssot_test.go or interactive service test file)
func TestRecordChatTranscriptSkipsDeltas(t *testing.T)               // EventMessageDelta → no record; EventMessageCompleted → one record
func TestRecordChatTranscriptCapturesTurnToolFileApprovalQuestionUsage(t *testing.T) // scripted run → turn_started, tool_started/completed, file_changed, approval_resolved, question_answered, token_usage records with payload fields
func TestRecordChatTranscriptDegradedFlagOnAppendFailure(t *testing.T)       // store append error → turn completes, chatStoreDegraded=true, one notice, timeline degraded:true
func TestLegacyChatBackfillRawOneShot(t *testing.T)                  // legacy chat timeline read twice → backfill records appear once (marker present)

// timeline handler
func TestChatTimelineJoinsLegsInOrder(t *testing.T)                  // 2 legs (seq 0,1) → Legs[0].LegSeq==0; Records span both legRunIds
func TestChatTimelineTailBudgetTruncatesOldest(t *testing.T)         // 9k records → Truncated true, oldest dropped, NextSeq points past last returned
func TestChatTimelineUnknownChat404(t *testing.T)                    // GET /client/chats/cht_unknown/timeline → 404 {"error":"chat_not_found"}
func TestChatTimelineSelfTagsLegacyRun(t *testing.T)

// flag
func TestChatSSOTFlagDefaultOff(t *testing.T)                        // env unset → chatSSOTEnabled()==false; FLOWPILOT_CHAT_SSOT=1 → true; =0/false/no → false
func TestChatSSOTFlagOptIn(t *testing.T)
```

## 7. Out of Scope

- Switch endpoint + envelope (Task-314); any UI (Task-315/316); sync/restore (Task-317); posture routing changes; `chat_provider_switch` record emission (type reserved in mapping table, emitted only by Task-314).
- Deleting or rewriting provider folder seeders.
- Chat retention/compaction policy (note as SD-26 follow-up if it surfaces).

## 8. Completion Notes

- result: DONE 2026-08-30 — CA-693 (substrate slice) + CA-694 (completion slice). 24 new tests green (`chat_ssot_test.go`, `chat_transcript_store_test.go`, `chat_timeline_test.go`, `chat_wiring_integration_test.go`). Full-suite R1: exactly the 13 pre-existing baseline failures; TUI suite 7 failures proven identical on the stashed clean tree; 2 flaky runner tests proven pre-existing via stash runs. Commits: `451cfd26` (substrate) + slice-2 commit (this close).
- follow-ups: apply migration `20260830080000` to the Supabase project + live `TestSupabaseChatTranscriptStoreRoundTrip`; local store restart index persistence (Task-317 hardening); SD-26 §6.1 wording note — E-6/E-7 record at request time with replay-carried decision/answer (the event vocabulary has no separate resolution event; mapper honors `Decision`/`Answer` on replay).
- upstream docs updated: CP-59 Open Questions → SD-26 §13 pointer; Task-313 moved to `done/`; Task-314/315/316/317 reference this task as done.

