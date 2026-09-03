# SD-26: Chat Continuity SSOT (chatId, Legs, Switch Linearization)

## Metadata

- Document ID: `SD-26`
- Title: `Chat Continuity SSOT (chatId, Legs, Switch Linearization)`
- Phase: `tech_design`
- Status: `approved`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-30`
- Last Updated: `2026-08-31` (approved — Task-312 close, all CP-59 Q-1..Q-7 + T-3..T-8 frozen, code in Task-313..317 verified against this doc)
- Parent Documents: [SS-05: Workflow AI Provider](../05-System-Specs/SS-05-Workflow-Ai-Provider.md), [SS-11: Workflow With Session](../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: [CP-59: Chat SSOT — Continuous Cross-Provider Chat](../07-Coding-Plan/done/CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat.md)
- Related Documents: [SD-06: AI Provider Integration](./SD-06-AI-Provider-Integration.md), [SD-14: Codex Cross-Account Chat Resume And Home Sync](./SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [Task-078: Cross-Provider Chat Handoff](../08-Task/done/Task-078-Cross-Provider-Chat-Handoff.md), [Task-312: Chat SSOT Design Freeze](../08-Task/todo/Task-312-Chat-Ssot-Design-Freeze-SD26.md)
- Replaces: `None` (additive layer above SD-06's per-run provider contract; Task-078 T-1 stays canonical for runs)
- Tags: `chat-ssot, chatId, cross-provider, switch-linearization, durable-replay, runner, tui, desktop`

## AI Quick View

### Summary

- A **chat** (`chatId`) is the user-facing continuity unit that outlives runs; a **leg** is one run on one provider. FlowPilot owns a durable per-chat transcript (local NDJSON + Supabase, idempotent on `(chat_id, chat_seq)`); provider folders demote to per-leg engine session state.
- Provider switch is a **three-phase linearized runner operation** (`SD26-S-2`) — intent under `s.mu` (persisted before unlock), `createRun` + envelope + seed with the lock released (createRun locks `s.mu` itself), close-old + record under `s.mu` — ordered start-new-first so all five crash windows heal with a closed matrix (zero TBD).
- The handoff envelope is **chat-scoped** (all legs, `<previous_conversation>` + `<actions_summary>`), budgeted by the target model's context window; the seed turn IS the single divider (stats embedded, `isHandoffSeed`); restored chats are **detached** and reattach via `createRun(chatId, switchFromRunID=latestLeg)` — never the switch endpoint.
- Contract families frozen here with stable ids (`SD26-D-*`, `SD26-E-*`, `SD26-S-*`, `SD26-X-*`) are cited verbatim by Task-313..317.

### Current Ask

- Freeze the storage schema, ordering model, event contract, endpoint DTOs, switch linearization + crash-heal matrix, error taxonomy, and degradation policy so Task-313..317 implement without making any contract decision themselves.

### Key Decisions

- `D-1` **Chat over runs, never inside runs** (`SD26-S-1`): `chatId`/`legSeq` are additive fields on the run record; `rs.providerKey` stays pinned per run (Task-078 T-1 preserved). Alternatives: mutable per-run provider (rejected — breaks run invariants, huge blast radius), provider-side session forging (rejected — CLIs own proprietary session formats).
- `D-2` **Two SSOTs, hard split** (`SD26-D-1..D-3`): FlowPilot chat transcript = truth for display/replay/restore; provider folders = per-leg engine state for turn execution and same-leg resume. No renderer reads provider folders once the store exists.
- `D-3` **Three-phase linearization** (`SD26-S-2`, review C-2 fix): never hold `s.mu` across `createRun` (it locks `s.mu` itself, `interactive_handlers.go:795` region) or chat-store I/O; start-new-first, close-old-second; phase-A intent **persisted before unlock** (review I-R5); two-active discriminator: keep `switchFromRunID != own id`, close `switchFromRunID == own id` (review I-R4).
- `D-4` **Dedicated transcript store, idempotent append** (`SD26-D-1`): local NDJSON per chat (O_APPEND, 1 MiB-line reader) + Supabase `workflow_chat_events` with `unique(chat_id, chat_seq)`; append skips duplicates — restore reuses it as upsert (review I-2).
- `D-5` **Monotonic per-chat `chatSeq`** (`SD26-D-4`) minted under `s.mu` at append time; timeline paginates `afterSeq` with tail budgets mirroring `chat_history_replay.go:6`.
- `D-6` **Seed is fire-and-return** (review I-5 closure): the switch endpoint dispatches the envelope seed server-side and returns; `handle.LastEventSeq` is the client attach point; the seed `turn_started` payload carries `isHandoffSeed` + carried/omitted counts + `handoffMode`.
- `D-7` **One divider per switch, single source**: live renders the seed as the divider; replay renders the `chat_provider_switch` record (`SD26-E-9`) and suppresses the duplicate seed; dedupe by `toRunId`; clients never synthesize a divider (review I-4).
- `D-8` **Restore-detach + reattach** (review I-8/I-R3): restored chats have zero active legs (all `closed(restored)`); the next turn/Tab/provider pick reattaches via direct `createRun(chatId, switchFromRunID=latestLeg)` — the switch endpoint correctly 409s `chat_no_active_leg` there and is never called for detached chats.
- `D-9` **Envelope: chat-scoped, context-budgeted, action-aware** (`SD26-D-6`): budget `min(contextWindowTokens × 3 chars, 512 KiB)`, floor 64 KiB (`handoffMaxBytes`) when unknown; `<previous_conversation>` (Task-078 `packConversationTurns` semantics) + optional `<actions_summary>` from tool/file/approval records; `handoffPromptPrefix` preserved for feature-resolution skip.
- `D-10` **Always ON (dev branch `cp59-chat-ssot`; flag removed)**: additive — all migrations additive columns + one new table. `FLOWPILOT_CHAT_SSOT` env is ignored (see `D-10` history).

### Constraints

- Chat-kind gate absolute: `runKind=="chat"` only; workflow/flow runs keep the provider-pinned lifetime and the `/provider` block.
- Never write to provider session folders; no cross-CLI session forging.
- TUI/Desktop are thin clients (runner SSOT, posture-doc pattern).
- Old single-run chats with zero switches behave byte-identically; legacy runs self-tag lazily (`chatId=runId`, `legSeq=0`).
- safe-fix-contract: additive tests only; Codex+Claude+Grok+opencode parity; read CA-679/BUG-329/Task-078 history before code.

### Open Questions

- None — all CP-59 `Q-1..Q-7` are closed in §13 Decision Register (this document's review gate).

### Source Refs

- CP-59 Key Decisions `P-1..P-9`, §3.1 continuity layers, §3.3 replay matrix, Work Breakdown `P-2..P-8`.
- Code anchors: `interactive_service.go:161` (`runKind`), `:3947/:3949` (`persistEvent` + delta skip), `:3565` (`persistenceStore`); `interactive_handlers.go:224` (`handleStartRun`), `:795` region (`createRun` takes `s.mu` + persists under it — the deadlock evidence behind `SD26-S-2`), `:792` (`stampAccount` resolve-before-lock pattern), `:962` (`runHistoryItem`); `local_file_session_store.go:661` (`AppendEvent`, flow-sidecar-only — why a dedicated store is needed); `supabase_workflow_store.go:544/:62/:656`; `handoff_context.go:15/:22/:59/:67/:73/:142/:156/:281/:307`; `interactive_resume.go:3008/:4784`; `provider_registry.go:202`; `types.go:34` (`ContextWindowTokens`); `tui/app/chat_history_replay.go:6`; `store.ts:966/:1010/:1058/:308`.
- Grok review batches (2026-08-29): `KILL_WITH_FINDINGS` ×2 → all C/I findings folded; `KILL_CLEAN` on the plan delta.

## 1. Goal

Define the technical contracts that make one logical chat survive provider switches across codex/claude/grok/opencode with (a) 100% display continuity from a FlowPilot-owned store, (b) best-possible model memory via a chat-scoped envelope, (c) crash-safe switch linearization with a closed heal matrix, and (d) chat-level sync/restore/reopen — such that Task-313..317 require zero further design decisions.

## 2. Input Documents

- SS-05 (provider-selection invariant — gains the chat-switch note via Task-314), SS-11 (session lifecycle)
- CP-59 §3.1/§3.3/§4 (continuity layers, replay matrix, work breakdown)
- Task-078 (envelope contract: `handoffPromptPrefix`, `packConversationTurns`, summary ladder)
- SD-06 (per-run provider contract), SD-14 (cross-account resume/sync precedent)
- Grok review register 2026-08-29 (C-1..C-3, I-1..I-8, I-R1..I-R6 — all closed)

## 3. Architecture Decision

- `D-1` Chat-over-runs layering (see Key Decisions; alternatives and rejection reasons inline).
- **Why this option:** reuses every durable primitive the runner already has (`createRun`, `persistProviderSession`, turn send path, Task-078 envelope, per-provider resume seeders) and adds exactly one new durable artifact (the chat transcript). The alternative designs either violate run invariants (mutable provider) or depend on undocumented CLI session formats (forging).

## 4. Component Impact

- Impacted modules (extend): runner interactive service + handlers + resume + handoff + chat_session_sync; TUI chat/restore; Desktop store/timeline/history; client-core contract.
- New modules: `chat_ssot.go`, `chat_transcript_store.go` (+supabase), `chat_switch.go`, `chat_timeline.go`, `chat_sync_manifest.go`, `tui/app/chat_switch.go`.
- Unchanged modules: provider adapters (all four), flow/workflow orchestration, Task-078 endpoint, provider folder seeders (fallback-only role).

## 5. Data Model (`SD26-D`)

### 5.1 Identity (`SD26-D-0`)

- `chatId`: `cht_<12-hex>` (crypto/rand; machine-local uniqueness — Drive sync namespaces by project+chatId).
- Mint/adopt **inside `createRun`** (shared entry behind `handleStartRun`, `interactive_handlers.go:224`): `normal_chat` with no `ChatID` → mint; with `SwitchFromRunID` → adopt source's `chatId`, `legSeq = source.legSeq + 1`; **source resolved before the lock** (`stampAccount` pattern, `:792`).
- Legacy self-tag (lazy, idempotent, under `s.mu`): untagged chat-kind run → `chatId=runId`, `legSeq=0`, `legState=active`.
- Leg fields appended to `interactiveRun` and the durable session record (`workflow_provider_sessions`, `ProviderSessionState`): `chat_id`, `leg_seq`, `leg_state`, `leg_closed_reason`, `switch_from_run_id`.

### 5.2 Leg lifecycle (`SD26-S-1`)

- States: `active` → `closed(reason)`; reasons: `provider_switch` | `chat_ended` | `restored` | `provider_switch_rolled_back`.
- Invariants (enforced under `s.mu`): **one active leg per chat**; **≤1 in-flight switch per chat** (guard flag, not a lock hold).
- Crash healing reopens the lifecycle per `SD26-S-2` (§7.2).

### 5.3 Transcript storage (`SD26-D-1..D-3`)

- Interface:
  ```go
  type ChatTranscriptRecord struct {
      ChatID   string          `json:"chatId"`
      ChatSeq  int64           `json:"chatSeq"`
      LegRunID string          `json:"legRunId"`
      Type     string          `json:"type"` // SD26-E-1..E-9
      Payload  json.RawMessage `json:"payload"`
  }

  type ChatTranscriptStore interface { // append idempotent on (chatId, chatSeq)
      AppendChatRecords(ctx context.Context, recs []ChatTranscriptRecord) error
      ReadChatRecords(ctx context.Context, chatID string, afterSeq int64, limit int) ([]ChatTranscriptRecord, error)
      LatestChatSeq(ctx context.Context, chatID string) (int64, error)
  }
  ```
- Local impl: one NDJSON per chat at `<localSessionDir>/chats/<chatId>/transcript.ndjson`, O_APPEND writer, 1 MiB-line reader (LoadFlowEvents pattern, corrupt tail skipped with log), duplicate `(chatId, chatSeq)` skipped on append.
- Supabase impl: `workflow_chat_events(chat_id text, chat_seq bigint, leg_run_id text, type text, payload jsonb, created_at timestamptz, unique(chat_id, chat_seq))`; insert `Prefer: resolution=ignore-duplicates`.
- Capture policy: hook beside `persistEvent` (`interactive_service.go:3947`), identical skip set (`EventMessageDelta` dropped, `:3949`); capture failure never fails the turn → `chat_store_degraded` (`SD26-X-6`).
- `chatSeq` allocation (`SD26-D-4`): monotonic int64 per chat, minted under `s.mu` at append time, never by the store.

### 5.4 Normative record example

```json
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

## 6. Interfaces and Contracts

### 6.1 Event contract (`SD26-E-1..E-9`)

| Id | Type | Payload (normative fields) | Source event(s) |
|---|---|---|---|
| `E-1` | `turn_started` | `{prompt, isHandoffSeed?, carriedTurnCount?, omittedTurnCount?, handoffMode?}` | `EventTurnStarted` |
| `E-2` | `message_completed` | `{text}` | `EventMessageCompleted` / `EventTurnCompleted` |
| `E-3` | `tool_started` | `{title, kind}` | tool start events |
| `E-4` | `tool_completed` | `{title, ok}` | tool completion events |
| `E-5` | `file_changed` | `{path, oldText?, newText?}` | `EventFileChanged` |
| `E-6` | `approval_resolved` | `{id, decision}` | approval resolution |
| `E-7` | `question_answered` | `{id, answer}` | question resolution |
| `E-8` | `token_usage` | `{last, total, contextWindow}` | usage events |
| `E-9` | `chat_provider_switch` | switch payload (§5.4) | switch op only, exactly once per `(chatId, fromRunId, toRunId)` |

### 6.2 Endpoint contracts (`SD26-D-5..D-7`)

- `GET /client/chats/{chatId}/timeline` → `{chatId, legs:[{runId, providerKey, legSeq, legState, legClosedReason}], records:[ChatTranscriptRecord], nextSeq, truncated, degraded}`; errors: `SD26-X-1`; tail/chunk budgets mirror `chat_history_replay.go:6`; read triggers legacy self-tag + one-shot raw backfill (marker-guarded).
- `POST /client/chats/{chatId}/switch-provider` request `{targetProviderKey, model?, reasoningEffort?, yoloMode?}` → response `{handle, chatId, legSeq, model, handoff:{handoffMode, includedTurnCount, omittedTurnCount, truncated, actionsDigestIncluded}}` (`Model` echoed — clients set it on adopt); errors per §8; requires an active leg (detached chats → `SD26-X-2`, clients reattach per `D-8`).
- Divider single-source rule (`D-7`): live = seed turn (stats embedded via `E-1` payload); replay = `E-9` record; renderers dedupe by `toRunId`.
- Task-078's run-scoped `POST /client/workflow-runs/{runId}/handoff-context` stays untouched (legacy fallback path).

### 6.3 DB contracts

- `workflow_provider_sessions`: appended columns only (`chat_id, leg_seq, leg_state, leg_closed_reason, switch_from_run_id`), never reordered (`supabase_workflow_store.go:62` list extended at the end); `workflow_chat_events` created by migration committed with Task-313.

## 7. Execution Flow

### 7.1 Provider switch (`SD26-S-2`, three phases)

1. **Phase A — under `s.mu`**: resolve active leg (else `SD26-X-1/X-2`); guards: `runKind=="chat"`, not `turnInFlight`/pending approval/question (`SD26-X-3`), target ≠ current (`SD26-X-4`), no in-flight switch; stamp `src.switchFromRunID = src.id` **and persist via `persistProviderSession` before unlock** (review I-R5); set in-flight flag; release.
2. **Phase B — no lock** (createRun locks `s.mu` itself — holding here deadlocks, review C-2): build chat envelope from the transcript store (fresh_start when empty — no error); `createRun` (new leg, `ChatID`, `SwitchFromRunID=src.id`, target provider/model/effort/yolo); dispatch seed turn fire-and-return (`D-6`); failure → best-effort intent clear (heal row 2 also covers on load).
3. **Phase C — under `s.mu`**: close old leg (`provider_switch`); append `E-9` record exactly once; clear in-flight flag; release. Respond with handle + stats.

### 7.2 Crash×step heal matrix (closed — zero TBD)

| Crash window | Observable state | Heal on load |
|---|---|---|
| before phase A commit | nothing persisted | none |
| after A intent, before B | old leg `active` + `switchFromRunID` set, no new leg | clear intent; leg stays active (retryable) |
| after B createRun, before C close | **two** active legs, both carry the field | **Discriminator (I-R4): keep leg with `switchFromRunID != own id`; close leg with `switchFromRunID == own id`** (`provider_switch`); append record |
| after C close, before record | new active, old closed, no record | append record idempotently (derived from leg fields) |
| after record, seed fails mid-turn | new active, seed turn failed | leg stays active; typed `switch_seed_failed` (`SD26-X-7`); chat continuable |

### 7.3 Timeline read / reattach

1. `ensureChatTagging` resident legs (+ `loadPersistedRun` for absent ones) → join legs by `legSeq` → records by `afterSeq` → respond.
2. Detached chat (post-restore): zero active legs; the next user turn/Tab/provider pick routes through `createRun(chatId, switchFromRunID=latestLeg)` — envelope seeds the new leg identically (`D-8`); the switch endpoint is **not** called.

## 8. Failure and Edge Handling (`SD26-X`)

- `X-1` `chat_not_found` 404 — unknown chatId.
- `X-2` `chat_no_active_leg` 409 — detached chat; clients reattach (`D-8`), never retry the switch.
- `X-3` `handoff_run_busy` 409 — in-flight turn, pending approval, pending question, or in-flight switch; no mutation.
- `X-4` `handoff_same_provider` 409 — same-provider continuity is the in-place `set_config`/`set_model` path; no leg minted.
- `X-5` `provider_unavailable` 422 — target provider not installed/available; install hint; enforced before any mutation.
- `X-6` `chat_store_degraded` — transcript append failure state (not an HTTP error): turn proceeds, leg flagged, one-time system notice, timeline `degraded:true` until a write succeeds.
- `X-7` `switch_seed_failed` — leg-level state after a committed switch whose seed turn failed; chat continuable (resend or re-switch).
- Restore edges (§ per Task-317): missing sidecars → leg `session_unavailable` (resume seeds a fresh provider session from the envelope); provider binary absent → `X-5`; v1 manifests restore via the legacy path unchanged; restore is idempotent (unique-key append).

## 9. Security and Operational Concerns

- auth: endpoints ride the existing runner client auth surface; no new credentials.
- secrets: envelope never embeds provider credentials; transcripts may embed user content — chat files live under the local session dir with existing permissions.
- audit: `E-9` records are the switch audit trail; legs stay queryable for drill-down.
- rollback: (dev branch) flag removed — always ON. Crash heal is rollback-safe per §7.2; reverting requires a code rollback, not an env toggle.

## 10. Risks and Trade-Offs

- `R-1` Dual-store drift (provider folder vs transcript) — accepted: transcript is display truth; drift only bounds leg-resume quality.
- `R-2` Lock discipline regression (future code holding `s.mu` across `createRun`) — mitigated by `TestSwitchNeverHoldsLockAcrossCreateRun` + the §7.1 comment anchors.
- `R-3` Budget misestimation when `ContextWindowTokens` is absent — floor to 64 KiB + summary ladder; overflow degrades to `target_summary`, never fails the turn.
- `R-4` Migration surface (`workflow_provider_sessions` additive columns) — append-only, lazy self-tag backfill; no startup rewrite.
- `R-5` Desktop timeline resets elsewhere — audit list + store tests lock the switch path only.
- `R-6` Gemini asymmetry (target-only) — typed rows in the matrix until a source extractor is proven; no silent behavior.

## 11. Validation Strategy

- unit: Task-313 store/seq/degraded tests; Task-314 matrix (12 directed pairs) + guards + heal cells + lock probe; Task-315/316 adopt/attach/divider/detached tests; Task-317 manifest/restore/idempotence tests (signatures frozen in each task doc).
- integration: fake-adapter switch end-to-end incl. crash injection per §7.2 row; legacy-transparency (flag off) snapshot.
- manual: full operator walk (opencode→grok→codex, restart, Drive restore on a claude-only machine — CS-14) on real providers; TUI + Desktop cross-surface timeline equality.
- observability: switch lifecycle log lines (phase A/B/C, handoff mode + counts), degraded-store flag, in-flight switch metric.

## 12. Traceability to Spec

- SS-05 provider-selection invariant → §7.1 guards (`X-3`/`X-4`), chat-kind gate; amendment owned by Task-314.
- SS-11 session lifecycle → §5.2 leg lifecycle; run invariants (Task-078 T-1) preserved by `D-1`.
- CP-59 `P-2/P-3` → §5; `P-4` → §6.2/§7.1; `P-5/P-6` → §6.2 divider + attach contracts; `P-7` → `X-4`; `P-8` → §7.3; CS-01..CS-14 → §11.

## 13. Decision Register (closes CP-59 `Q-1..Q-7` + review decisions)

| Id | Decision | Rationale | Impacted tasks |
|---|---|---|---|
| `Q-1` store shape | Dedicated `ChatTranscriptStore` (local NDJSON + `workflow_chat_events`) | `AppendEvent` only persists flow-sidecar types locally; chat needs its own durable artifact | 313, 317 |
| `Q-2` seq model | Monotonic per-chat `chatSeq` under `s.mu` | Simple pagination/tail; tuple ordering needs composite keys everywhere | 313, 317 |
| `Q-3` process lifecycle | Provider process stays warm per scopeKey across legs | BUG-329 reuse semantics; switch is leg-level, not process-level | 314 |
| `Q-4` history unit | Chat primary; run drill-down retained | Operator sees one chat; audit keeps legs | 316 |
| `Q-5` gemini | Target-only until source extractor proven | No proven gemini transcript extractor; typed rows, no silent gaps | 314 matrix |
| `Q-6` token usage | Per leg + chat total line | Matches `latestTokenUsage` reset semantics per leg | 316 (deferred line) |
| `Q-7` recall tool | Deferred out of CP-59 | Envelope budget + ladder first; tool needs its own contract | — |
| Seed dispatch | Fire-and-return; `LastEventSeq` attach; stats in seed payload | Client never blocks on seed; single divider source (`D-6`/`D-7`) | 314, 315, 316 |
| Linearization | Three-phase, `s.mu` never across `createRun`/store I/O; intent persisted in phase A; discriminator `switchFromRunID != own id` | Deadlock evidence `createRun:795`; crash matrix closed (`SD26-S-2`) | 314 |
| Restore | Detach all legs; reattach via direct `createRun(chatId, switchFromRunID=latest)` — never the switch endpoint | Detached chat has no active leg; `X-2` is correct there | 315, 316, 317 |
| Divider | Seed is the only live divider; `E-9` is the replay divider; dedupe by `toRunId` | Kills double-divider (review I-4) | 315, 316 |
| Degraded store | Flag + one notice + timeline `degraded:true`; never fails a turn | Chat availability > transcript completeness | 313 |

## 14. Account-Switch Interaction (correction — 2026-08-30, operator review)

Account-switch continuity for codex/grok/claude (+ a gemini variant) **already exists** and this design does NOT replace it: reopening a chat whose stored `providerAccountID` differs from the active account copies the session into the active account's home and continues there — `prepareCrossAccountResume` (`interactive_resume.go:2784`, log `mode=cross_account … target_home=…`). The E2E-06 mismatch guard only blocks **silent** wrong-account resume; the deliberate copy path is the product behavior. Earlier statements claiming mid-conversation account switching "does not work" for those providers were wrong and are corrected here.

Composition with this design:

- The **active leg** keeps using the existing copy-cross-home resume (100% provider memory) — CP-59 does not touch that path.
- Legs from **other providers** render from the chat transcript (§6.1) — full multi-provider scrollback the old mechanism never had; only the active leg continues executing.
- **Opencode** cannot use file-copy resume (sessions live in the shared `opencode.db`; `LocateSessionFile` typed-unsupported per CP-57 §10.2) — the switch endpoint's envelope (§7.1) is opencode's continuity mechanism for provider AND account changes, already shipped (Task-314/315).
- Optional future wiring (not this design's scope): a same-provider account change on a live chat can reuse `prepareCrossAccountResume` in place (activate → copy → stamp update) instead of close+reopen; opencode keeps the envelope fallback. Any such wiring must keep the silent-resume mismatch guard intact on all other paths.
