# Task-312: Chat SSOT Design Freeze (SD-26)

## Metadata

- Document ID: `Task-312`
- Title: `Chat SSOT Design Freeze (SD-26)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-29`
- Last Updated: `2026-08-31` (done — SD-26 approved, all §4 T-2 sections + §11 register closed, crash matrix 0 TBD, E-1..E-9 frozen, verified against Task-313..317 code; doc-only commit)
- Parent Documents: [CP-59: Chat SSOT — Continuous Cross-Provider Chat](../../07-Coding-Plan/todo/CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat.md)
- Child Documents: `None`
- Related Documents: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md), [Task-078: Cross-Provider Chat Handoff](../done/Task-078-Cross-Provider-Chat-Handoff.md)
- Replaces: `None`
- Tags: `chat-ssot, design-freeze, doc-only, durable-replay`

## AI Quick View

### Summary

- Author `SD-26: Chat Continuity SSOT` — the tech design that freezes the data model, seq model, event contract, leg state machine, **switch linearization + crash×step matrix**, and error taxonomy for the whole CP-59 stack **before any code lands**.
- ID note: the design slot is **SD-26** (SD-19..SD-25 already exist, `requirements/06-System-Tech-Design/`; the earlier SD-19 numbering collided and was renumbered in this batch).
- Resolve CP-59 `Q-1..Q-7` plus the review-added decisions (linearization, divider source, seed dispatch mode, restore-detach) with explicit recorded decisions.
- Doc-only task: zero code merged. It is the review gate for Task-313..317. **Claim to close here: "SD-26 + CP/task docs patched; protocol crash×step cells TBD = 0."**

### Current Ask

- Produce an approved SD-26 such that Task-313..317 implementers never make a storage/contract/locking decision on their own: every record shape, ordering rule, lock protocol, heal rule, and typed error is written down with a rationale and a traceability id.

### Key Decisions

- `T-1` SD-26 must contain, at minimum, the section set in §4 `T-2` — each section freezes one contract family (identity, storage, ordering, events, lifecycle, **linearization/heal**, errors, degradation).
- `T-2` CP-59 defaults are ratified unless review overrides: `Q-1` dedicated chat store + `ChatTranscriptStore` interface (local NDJSON per chat + Supabase `workflow_chat_events`); `Q-2` monotonic per-chat `chatSeq` minted under the service lock; `Q-3` provider process stays warm per scopeKey across legs; `Q-4` chat primary + run drill-down retained; `Q-5` gemini target-only until source extractor proven; `Q-6` per-leg token usage + chat total line; `Q-7` recall tool deferred out of CP-59.
- `T-3` **Seed-turn dispatch is server-side and fire-and-return** (resolves the former Task-314 open question): the switch endpoint opens the leg, dispatches the envelope seed, and returns the handle immediately — it does **not** wait for the seed's first stream event; the response's `handle.LastEventSeq` is the client attach point. The seed `turn_started` payload carries the handoff stats (`isHandoffSeed`, `carriedTurnCount`, `omittedTurnCount`, `handoffMode`) so the seed renders as the **single divider** (see `T-5`).
- `T-4` **Switch linearization is three-phase, never lock-across-createRun**: `createRun` (`interactive_handlers.go:795` region) takes `s.mu` itself and persists under it — the switch op must NOT hold `s.mu` across it. Phases: **A** (under `s.mu`): validate guards, stamp `switchFromRunID` intent on the source leg **and persist it** (`persistProviderSession` before unlock — crash-after-A leaves the marker on disk, heal row 2 non-vacuous; review I-R5), release; **B** (no lock): build envelope (chat-store I/O), `createRun` for the new leg, dispatch seed; **C** (under `s.mu`): close old leg (`provider_switch`), append `chat_provider_switch` record. Ordering is **start-new first, close-old second** so every crash window leaves a healable state (§4 `T-6` matrix).
- `T-5` **One divider per switch, single source of truth**: the divider is rendered **from the seed turn** (`isHandoffSeed` + embedded stats) on live streams, and **from the `chat_provider_switch` record** (SD26-E-9) on timeline replay — renderers dedupe by `toRunId` so both paths never produce two dividers. Clients must not synthesize their own divider line on switch success.
- `T-6` **Crash×step matrix must be closed in SD-26** (zero TBD cells) — mandated state machine section; the cells:

  | crash window | observable state | heal rule on load |
  |---|---|---|
  | before phase A commit | nothing persisted | none needed |
  | after A (intent stamped), before B createRun | old leg `active`, `switchFromRunID` set, no new leg | clear intent; leg stays active (switch never started) |
  | after B createRun, before C close | **two** active legs; both carry the field | **Discriminator (review I-R4): keep the leg whose `switchFromRunID != its own id`** (the new leg — it points at the source run); close the leg whose `switchFromRunID == its own id` (the old source); append switch record |
  | after C close, before record | new active, old closed, no `chat_provider_switch` record | append record idempotently (derived from leg fields) |
  | after record, seed fails mid-turn | new active, seed turn failed | leg stays active; seed failure surfaces typed `switch_seed_failed` on the leg; chat continuable (user resends or re-switches) |

- `T-7` **Degraded-store policy**: a failed transcript append never fails the turn — leg is marked `chatStoreDegraded`, one system notice is emitted, timeline responses carry `degraded:true` until a write succeeds again (Task-313 implements; SD-26 specifies the flag + notice contract).
- `T-8` **Restore-detach policy** (closes the former Task-317 I-8 gap): a restored chat has **no locally-active leg** — all legs restore as `closed(restored)`; the chat opens with the full timeline in detached state. Reattach is **direct `createRun(chatId, switchFromRunID=latestLeg)` — NOT the switch endpoint** (which correctly 409s `chat_no_active_leg` on a detached chat; review I-R3): the next user turn / Tab / provider pick routes through the reattach predicate on the client, the envelope seeds the new leg identically; picking a provider whose binary is absent → typed `provider_unavailable`. Task-315/316 own the one-line predicate.
- `T-9` Every artifact in SD-26 carries a stable id (`SD26-D-*` data, `SD26-E-*` events, `SD26-S-*` state machine, `SD26-X-*` errors) so Tasks cite ids, not prose. Event list is **unified E-1..E-9**: `turn_started`, `message_completed`, `tool_started`, `tool_completed`, `file_changed`, `approval_resolved`, `question_answered`, `token_usage`, `chat_provider_switch` (this closes the 312/313 record-list drift).

### Constraints

- Doc-only: no `apps/**` file may change in this task.
- SD-26 must follow `FORMAT-REFERENCE-SD.md` and pass the `phase-document-compliance` skill check.
- SS-05/SD-06 amendments are **owned by Task-314** (they land with the switch contract), not here — this task only records the pointer.

### Open Questions

- None — resolving CP-59 `Q-1..Q-7` plus `T-3..T-8` **is** this task's deliverable.

### Source Refs

- CP-59: AI Quick View `Key Decisions P-1..P-9`, `Open Questions Q-1..Q-7`, §3.1 continuity layers, §3.3 replay matrix, §4 Work Breakdown `P-1`.
- Grok review 2026-08-29 (`KILL_WITH_FINDINGS`): C-1 (SD id), C-2 (lock vs `createRun`), C-3 (heal cells), I-1..I-8 — all verified against code and folded into `T-3..T-9`.
- `interactive_handlers.go:795` region — `createRun` takes `s.mu` and persists under it (deadlock evidence for `T-4`); `handoff_context.go:22` (`handoffPromptPrefix`), `:281` (`renderHandoffPrompt`).
- `interactive_service.go:3947` (`persistEvent`, skips `EventMessageDelta`), `local_file_session_store.go:661` (`AppendEvent` — flow-sidecar types only), `supabase_workflow_store.go:544` (`AppendEvent`), `:62` (`workflow_provider_sessions` columns).

## 1. Goal

An approved `SD-26: Chat Continuity SSOT` that freezes: chatId format, leg lifecycle, chat transcript record schema, chat seq model, `chat_provider_switch` event contract, the three-phase switch linearization with its crash×step heal matrix, endpoint request/response shapes (for Task-313/314), degradation taxonomy (for Task-317), and the CP-59 open questions — with traceability ids used verbatim by Task-313..317.

## 2. Parent Links

- coding plan: `CP-59` Work Breakdown `P-1` (review gate)
- tech design: produces `SD-26`; references `SD-06`, `SD-14`
- system spec: `SS-05`, `SS-11`
- specific upstream ids: CP-59 `Q-1..Q-7` (must all be closed), CP-59 §3.3 (matrix SD-26 formalizes)

## 3. Trigger

CP-59 was approved for planning on 2026-08-29. Grok's `KILL_WITH_FINDINGS` review (same day) confirmed the design-freeze slot must also close the lock/deadlock and crash-heal questions before Task-314 can be coded — this task is that closure.

## 4. Exact Change

- `T-1` Create `requirements/06-System-Tech-Design/SD-26-Chat-Continuity-Ssot.md` (new, `Status: draft` → `approved` at close). Current: no chat-level tech design exists; SD-06 §3.2 pins provider per run only; SD-19..SD-25 are taken. Delta: SD-26 is additive to SD-06 — it layers chat over runs, never redefines the run contract (Task-078 T-1 stays canonical).
- `T-2` SD-26 mandated sections (each freezes one contract family):
  - `§2 Identity` — chatId format `cht_<12-hex>` (crypto/rand, machine-local uniqueness; Drive sync namespaces by project+chatId); mint/adopt point: **inside `createRun`** (the shared internal entry behind `handleStartRun`, `interactive_handlers.go:224`) so the switch path gets identical tagging — resolve `switchFromRunID`'s chatId **before** the lock (same pattern as `stampAccount`, `interactive_handlers.go:792`); legacy self-tag rule (`chatId=runId`, `legSeq=0`).
  - `§3 Leg lifecycle` (`SD26-S-1`): state machine `active → closed(provider_switch | chat_ended | restored | provider_switch_rolled_back)`; one active leg per chat (invariant); at most one in-flight switch per chat (in-flight guard flag, not a lock hold).
  - `§4 Storage` (`SD26-D-1..D-3`): `ChatTranscriptStore` interface + local NDJSON impl (per-chat file, O_APPEND, 1 MiB-line reader like `LoadFlowEvents`) + Supabase `workflow_chat_events(chat_id, chat_seq, leg_run_id, type, payload jsonb, created_at)` with **unique (chat_id, chat_seq)**; append is idempotent on that key (restore upsert semantics, Task-317); delta-skip policy identical to `persistEvent` (`interactive_service.go:3949`).
  - `§5 Ordering` (`SD26-D-4`): monotonic per-chat `chatSeq` (int64, minted under `s.mu` at append time); timeline reads paginate by `afterSeq` with tail budgets mirroring `chat_history_replay.go:6`.
  - `§6 Event contract` (`SD26-E-1..E-9`): the unified nine record types of `T-9` — payload JSON per type (see §4 `T-3` example); `chat_provider_switch` payload: `{fromProvider, fromModel, toProvider, toModel, fromRunId, toRunId, legSeq, handoffMode, includedTurnCount, omittedTurnCount, truncated}`.
  - `§7 Endpoint contracts` (`SD26-D-5..D-7`): `GET /client/chats/{chatId}/timeline`, `POST /client/chats/{chatId}/switch-provider` request/response DTOs (verbatim adopt of Task-313 `T-5` / Task-314 `T-1` drafts), seed dispatch = fire-and-return (`T-3`), divider single-source rule (`T-5`).
  - `§8 Switch linearization` (`SD26-S-2`): the three-phase protocol (`T-4`) + the crash×step heal matrix (`T-6`) — **all cells closed, zero TBD**; includes the lock rule: never hold `s.mu` across `createRun` or chat-store I/O (evidence: `createRun` locks `s.mu` itself, `interactive_handlers.go:795` region).
  - `§9 Error taxonomy` (`SD26-X-1..X-7`): `chat_not_found` 404, `chat_no_active_leg` 409, `handoff_run_busy` 409, `handoff_same_provider` 409, `provider_unavailable` 422 (+install hint), `chat_store_degraded` (state flag, not a turn failure), `switch_seed_failed` (leg-level state, chat continuable).
  - `§10 Degradation` (Task-317): restore-detach policy (`T-8`); missing sidecars → leg `session_unavailable` (resume seeds fresh session from envelope); missing provider binary → `provider_unavailable`; chat opens with full text in every case.
  - `§11 Decision register`: CP-59 `Q-1..Q-7` + `T-3..T-8` → decision → rationale → impacted Task ids.
- `T-3` Embed the frozen record contract (normative example to copy verbatim into SD-26 §6):
  ```json
  // chat transcript record (NDJSON line / workflow_chat_events row)
  {
    "chatId": "cht_9f2a71c04b8d",
    "chatSeq": 47,
    "legRunId": "run-314536",
    "type": "chat_provider_switch",
    "payload": {
      "fromProvider": "opencode", "fromModel": "opencode-go/muse-spark-1.2-contributor",
      "toProvider": "grok", "toModel": "grok-4.5",
      "fromRunId": "run-314536", "toRunId": "run-314601", "legSeq": 2,
      "handoffMode": "raw", "includedTurnCount": 14, "omittedTurnCount": 0, "truncated": false
    }
  }
  ```
  ```go
  // normative Go shape (SD-26 §4 SD26-D-1)
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
  ```
- `T-4` Close the decision register: a table in SD-26 §11 mapping CP-59 `Q-1..Q-7` and decisions `T-3..T-8` → decision → rationale → impacted Task ids. Update CP-59 `Open Questions` to point at SD-26 §11 answers (status note only — CP-59 body stays as the approved plan of record).

## 5. Touched Areas

- files: new `requirements/06-System-Tech-Design/SD-26-Chat-Continuity-Ssot.md`; edit `requirements/07-Coding-Plan/todo/CP-59-...md` (Open Questions pointer + Status flip to `approved` once review passes)
- modules: none (doc-only)
- routes: none
- tables: `workflow_chat_events` schema is *specified* here (incl. the unique key), created in Task-313

## 6. Acceptance Check

- SD-26 passes `phase-document-compliance` (metadata, AI Quick View, numbered sections, traceability links).
- Every CP-59 `Q-1..Q-7` and every `T-3..T-8` decision has a register row with rationale; every Task-313..317 `T-*` that touches storage/events/errors/locking can cite an `SD26-*` id.
- The crash×step matrix has zero TBD cells; the record/event/error contracts match the code drafts in Task-313/314 verbatim.

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` SD-26 exists (unique id — SD-19..SD-25 untouched) with all §4 `T-2` sections and passes the compliance skill check. `SD-26:1` exists, 257 lines, `Status approved`
- [x] `DOD-2` Decision register closes `Q-1..Q-7` + `T-3..T-8` (each: decision + rationale + impacted tasks). `SD-26:232` table Q-1..Q-7 + Seed/Linear/Restore/Divider/Degraded rows
- [x] `DOD-3` `ChatTranscriptRecord`, `ChatTranscriptStore` (idempotent append), timeline + switch DTOs, error codes, and the E-1..E-9 list are frozen verbatim (match Task-313/314 drafts). `SD-26:104` normative Go shape + `§6.1` table E-1..E-9
- [x] `DOD-4` `SD26-S-2` switch linearization (three-phase, lock rule) + crash×step matrix fully closed — zero TBD cells. `SD-26:172` §7.1 + `§7.2` 5-row matrix
- [x] `DOD-5` Divider single-source rule (`T-5`) and seed-stats contract (`T-3`) written into the event contract section. `SD-26:40` D-7 + `§6.1` E-1 payload + `§6.2` divider rule
- [x] `DOD-6` Degraded-store flag contract (`T-7`) and restore-detach policy (`T-8`) written into §10. `SD-26:41` D-8 + `§8` X-6 + `§7.3` reattach
- [x] `DOD-7` CP-59 `Open Questions` updated to reference SD-26 §11; CP-59 Status flips to `approved` at review. `CP-59:56` Open Questions already points to SD-26 §13 Decision Register (Task-312 close note)
- [x] `DOD-8` Zero `apps/**` diff in this task's commits (doc-only verified by `git diff --stat apps/` empty). Verified `git diff --stat apps/` empty for this slice (only doc files)

### 6.2 Test Signatures

None — doc-only task. Verification is the `phase-document-compliance` skill pass (`DOD-1`) plus the §6 spot-check against Task-313/314 code drafts (`DOD-3`). The first executable contracts land in Task-313 §6.2; the lock-order proof lands as `TestSwitchNeverHoldsLockAcrossCreateRun` (Task-314 §6.2).

## 7. Out of Scope

- Any code, schema migration, or endpoint implementation (Task-313+).
- SS-05/SD-06 amendments (owned by Task-314).
- Recall tool design (deferred, CP-59 `Q-7`).

## 8. Completion Notes

- result: done — SD-26 Chat Continuity SSOT approved (257 lines, `Status approved`), all contract families frozen with stable ids cited by Task-313..317; crash matrix 0 TBD; E-1..E-9 + X-1..X-7 + D-1..D-10 closed.
- follow-ups: none — Task-313..317 already verified against this doc; optional CP-59 Status flip to approved already reflected in SD-26 §13 register
- upstream docs updated: SD-26 approved; Task-312 moved to done
