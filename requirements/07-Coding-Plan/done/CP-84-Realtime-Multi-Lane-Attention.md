# CP-84 — Realtime Multi-Lane Attention (Event Plane + Inbox Triage + Lane-Switch Cost)

- Document ID: `CP-84`
- Title: `Realtime Multi-Lane Attention`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-23`
- Last Updated: `2026-09-23`
- Parent Documents: `CP-82 (Multi-Project Parallel Vibe Ops), Task-404 (attention queue), SS-23/SD-27 (worktree merge decisions), SD-28/SS-24 (shared runner lifecycle)`
- Child Documents: `Task-429 (events mux), Task-430 (decision payload), Task-431 (inbox controls), Task-432 (chat drafts), Task-433 (run-view cache), Task-434 (modal routing), CP-84-Test-Steps`
- Related Documents: `Task-422 (board), Task-423 (inbox actions), Task-425 (spectator), CP-81 (lifecycle lease), CP-59 (chat SSOT / legs)`
- Replaces: ``
- Tags: `desktop, attention-queue, event-plane, inbox-triage, parallelism, vibe`

## AI Quick View

### Summary

- Mục tiêu use case: 1 operator vibe **5 app mobile song song** (≈5–12 live
  runs). Runner đã multi-run/multi-cwd sẵn (`runs map`, per-run
  `workspaceCwd`, per-project `.flowpilot/`); CP-82 đã ship awareness layer
  (SessionsBoard + AttentionInbox + SpectatorPane). Gap còn lại nằm ở
  **event delivery** và **triage actionability**, không phải engine.
- Giữ nguyên model "1 focus + N lane nền": một attached run, một composer,
  không multi-pane. CP này nâng lane nền từ poll-30s sang realtime, và biến
  inbox thành điểm quyết định đủ context — hầu hết action không cần switch
  chat.
- Ba khối việc: (a) multiplexed event endpoint thay polling, (b) normalize
  mọi user-decision thành attention item có payload đủ để quyết inline,
  (c) giảm chi phí switch lane (per-chat drafts + run-view cache) và chặn
  modal toàn cục hijack focus.
- Turn-level admission control (scheduler/backpressure) được ghi nhận là gap
  thật nhưng **defer** sang follow-up sau khi có metric — xem `P-7`.

### Current Ask

- Khi 1 lane nền vào trạng thái chờ (approval/question/gate/ss_lock/merge/
  quota), operator thấy trong <1s thay vì ≤30s; action được inline cho mọi
  decision kind có payload; switch chat không mất draft và không trả giá
  reload đầy đủ; event của lane nền không bao giờ bật modal che lane đang
  focus.

### Key Decisions

- `P-1` **Multiplexed level-state endpoint** (`GET /client/events/stream`
  SSE): phát bounded projection của user-visible top-level lanes, không mirror
  raw `ProviderEvent`/child-agent noise. Connect/reconnect nhận chunked full
  snapshot atomic-on-complete, sau đó idempotent upsert/remove theo per-run
  revision; slow clients coalesce latest state hoặc retryable reconnect.
- `P-2` **Durable decision projection contract**: chuẩn hoá pending records
  thành versioned `decisions[]` (`decisionId + revision`) với bounded/redacted
  context. Runner owns approval/question/gate/ss_lock/merge/dispatch projection;
  desktop provider-account authority enrich quota candidate. Payload invalid/
  stale/missing → "Open" only, never guess.
- `P-3` **Inbox per-kind controls + expandable preview**: inbox render đúng
  control theo `decision.kind`; expand `▸` xem payload (regressedTests +
  outputTail, SS quickView, conflict paths, quota candidates) mà không rời
  chat. Thêm triage: filter theo kind/project, batch-approve cho kind an
  toàn.
- `P-4` **Per-chat drafts**: `drafts: Record<key, DraftState>` key =
  `chatId | runId | "<projectId>:new"`; persist `localStorage`, cap ~50
  entry, prune on delete/send. Draft là per-device intent — không sync
  Drive/Supabase.
- `P-5` **Run-view cache (Layer-2 lite)**: extend `_runSnapshots` thành LRU
  giữ timeline window + scroll pos của N run gần nhất; switch-back render
  từ cache rồi revalidate — không phải full per-run interactive slices
  (`runs: Record<runId, RunViewState>` vẫn out of scope).
- `P-6` **Modal routing theo run**: chỉ focused run được raise modal
  (`gateBlock`, `pendingAccountSwitch`, provider-switch, GrokYolo). Event
  của run nền → attention item kind tương ứng (vd `quota`), action vẫn flip
  global account nhưng prompt surface là per-run.
- `P-7` *(deferred)* **Provider-turn admission control**: cap đặt ở hub
  dispatch/turn spawn (không phải run level — sub-agent fan-out không kiểm
  soát được), global ≈4–6 + per-provider ≈2–3 + fairness round-robin per
  run. Gate: cần metric trước (in-flight count, queue wait, 429 rate) —
  không implement trong CP này nếu thiếu số liệu.

### Constraints

- Single-focus model giữ nguyên: không thêm composer thứ hai, không
  multi-pane, không multi-window Electron.
- Additive tests only; không sửa/weaken test cũ (oracle-rule).
- Provider parity: decision payload serialization phải provider-agnostic
  (Claude/Codex/Grok/Gemini/Opencode/Devin); capability thiếu = typed
  degradation, không silent mask.
- Reuse seam hiện có: `subs`/SSE, `attentionQueue`, `_runSnapshots`,
  `projectHistoryById` — không tạo session/registry/snapshot model song
  song (AGENTS §1).
- Monitor data là derived, không authoritative; durability contract của
  run/session không đổi.
- Drafts chỉ local (per-device), không đi Drive sync.

### Open Questions

- `Q-1` Run park ở gate > `IdleTTL` (2h) → warm provider session bị
  `sweepIdleSessions` reap; run vẫn resumable nhưng approve muộn trả giá
  resume. Chấp nhận chi phí resume, hay thêm TTL riêng cho parked state?
  (Quyết ở P-1 design; lean về giữ nguyên + ghi rõ hành vi.)
- `Q-2` Batch-approve áp dụng cho kind nào là an toàn? Đề xuất: chỉ
  `approval`/`question` có options rõ; gate/merge/quota luôn per-item.
- `Q-3` Multiplexed endpoint có cần scope theo client lease (CP-81) hay
  broadcast cho mọi client attach? Lean: mọi authenticated local client
  nhận cùng stream (runner là single-user).
- `Q-4` `P-7` vào CP-84 hay tách CP-85 sau khi có metric? Lean: tách.

### Source Refs

- `CP-82` (awareness layer + P-1..P-4 đã ship), `Task-404` (attentionQueue
  semantics), `Task-423` (inline actions + `resolvedPending`/`evict`),
  `Task-425` (spectator), `CP-81`/`SD-28`/`SS-24` (client lease, lifecycle),
  `CP-59` (chatId/leg model), `SS-23`/`SD-27` (worktree merge decision
  kinds), `CA-922` (projectHistoryById + per-project attention slices).

## 1. Goal

Operator vibe 5 app mobile song song trên 1 runner + 1 desktop window:

- Nhận biết mọi lane cần hành động trong <1s (không phụ thuộc poll 30s).
- Quyết định inline từ inbox cho mọi decision kind có đủ payload — không
  cần rời chat đang focus.
- Switch sang lane khác để xem/act bình thường, quay lại không mất draft
  và không trả giá replay đầy đủ.
- Không bị modal của lane nền gián đoạn khi đang tương tác lane focus.

## 2. Input Documents

- `CP-82` — awareness layer CP này mở rộng; giữ nguyên `P-1` (single
  attached run) và `P-4` (spectator read-only) của nó.
- `Task-404` / `Task-423` — attention queue + inline action semantics.
- `CP-81` / `SD-28` / `SS-24` — lifecycle lease/clients registry.
- `CP-59` — chat SSOT: provider/model switch = leg mới (runId mới) dưới
  cùng chatId → mọi event/subscription key theo **runId**, UI group theo
  `groupRunsByChatId`.
- `SS-23` / `SD-27` — worktree merge decision (apply_patch | keep_branch |
  discard) là một decision kind của `P-2`.
- `AGENTS.md (apps/local-runner)` — hub-only routing, durability,
  provider-parity, fail-closed invariants.

## 3. Implementation Strategy

- overall approach: freeze durable decision projection trước (P-2), rồi
  transport nó qua level-state event plane (P-1), sau đó surface (P-3),
  switch cost (P-4/P-5), cuối cùng modal routing (P-6).
- sequencing logic: **Task-430/P-2 trước Task-429/P-1** để transport không
  invent transient decision authority; P-4 độc lập. P-5 có thể chuẩn bị độc
  lập nhưng mux dirty-mark integration land sau P-1. P-6 sau P-2/P-3.
- dependencies: runner chỉ động ở P-1/P-2 (endpoint + payload
  serialization); toàn bộ còn lại là desktop renderer. Không schema
  change, không Supabase migration.

## 4. Work Breakdown

- `P-1` **Multiplexed run-state endpoint** — `GET /client/events/stream`:
  initial/reconnect snapshot của top-level user lanes gửi theo bounded chunks,
  client stage rồi reconcile chỉ khi complete; live upsert/remove carry
  `{runId, projectId, chatId, revision, status, lastSummary, decisions[]}`.
  Không stream raw deltas/tools/tokens/child runs. Publisher non-blocking,
  latest-state coalescing; overflow đóng retryable để reconnect snapshot.
  Poll 30s giữ làm independent reconciliation fallback.
- `P-2` **Decision projection contract** — durable, deterministic
  `decisions[]`: mỗi item có `decisionId/revision/kind/createdAt/actionable`;
  gate/ss_lock/merge context capture + persist tại mutation point, builder
  dưới lock không làm I/O; field/list bounded + secret redaction. TS dùng
  discriminated union; quota candidate enrich tại desktop từ provider account
  snapshot và revalidate trước switch. Multiple decisions cùng run không
  overwrite; stale action trả 409 + refresh.
- `P-3` **Inbox per-kind controls + preview + triage** — item render
  control theo `decision.kind`: nút approve/deny, option chips, gate
  radio+text, merge 3-choice, quota switch-confirm. Expand `▸` hiện
  payload chi tiết inline (không rời chat). Filter theo kind + project;
  batch-approve cho kind an toàn (theo Q-2). Payload absent → "Open"
  only. Action đi qua `approveAttentionItem`-style path hiện có +
  `evictedRuns`; lỗi → toast + refresh.
- `P-4` **Per-chat drafts** — `drafts: Record<string, DraftState>`
  (text + mention/attachment refs + updatedAt), key `chatId|runId|
  "<projectId>:new"`. `ChatInput` đọc/ghi qua key của chat đang focus;
  `resetRun()`/`selectProject` không đụng drafts; clear on send; prune
  on chat delete + cap 50. Persist `localStorage` (pattern
  `LAST_PROJECT_KEY`).
- `P-5` **Bounded existing run snapshots** — extend `_runSnapshots`, không
  tạo `_runViews`: thêm access/freshness/stable-scroll-anchor metadata;
  LRU chỉ evict optional history views, pin current/main/active ancestry.
  Cache hit render ngay nhưng always revalidate với generation guard; cache
  miss fetch/replay thay vì silent no-op. P-1 revision mark dirty; terminal
  không xoá useful completed timeline.
- `P-6` **Modal routing theo run** — audit mọi `set({ pendingAccountSwitch
  | pendingProviderSwitch | gateBlock | grokYoloPostureLoading })` call
  site: nếu event thuộc `runId !== focusedRunId` → tạo attention item
  (`quota`/`gate`/…) thay vì modal. Modal giữ cho focused run. Test
  ma trận: quota-hit ở lane nền không bật overlay khi đang gõ lane khác.
- `P-7` *(deferred — cần CP/Task riêng khi có metric)* **Provider-turn
  admission control** — semaphore tại hub dispatch (`agent.delegate`
  spawn path), config `maxInFlightTurns` global + per-provider, park
  node dispatch (không fail run), expose queue position qua event P-1,
  token counter cross-lane. Pre-work trong CP này: chỉ emit metric
  (in-flight count, wait time, 429 rate) nếu rẻ.

## 5. Touched Areas

- files:
  - runner: `internal/runner/interactive_handlers.go` (events endpoint),
    `internal/runner/interactive_service.go` (event fan-out + decision
    payload), `internal/runner/types.go` (envelope/decision types),
    `internal/cli/root.go` (route wiring)
  - desktop: `src/state/attentionQueue.ts` (decision payload + kinds),
    `src/state/store.ts` (drafts, `_runSnapshots` LRU, modal routing),
    `src/components/AttentionInbox.tsx` + `AttentionQueue.tsx` (per-kind
    controls, preview, filter/batch), `src/components/ChatInput.tsx`
    (draft key), `src/client/HttpWsRunnerClient.ts` (events stream),
    `src/types/contract.ts` (mirror types), `src/styles.css`
- modules: `internal/runner` (event plane), desktop renderer
  (attention/inbox/state)
- database: none
- external systems: none

## 6. Data or Migration Steps

- schema: none.
- data backfill: none.
- config updates: none (P-7 deferred sẽ mang config keys riêng nếu làm).

## 7. Validation Plan

- tests to add (full names/matrix in `CP-84-Test-Steps`):
  - runner: `TestRunUpdates_InitialSnapshotIsAtomicWithSubscription`,
    chunk-complete, dirty-overflow, child exclusion, provider parity và
    `TurnCompleted`-before-gate non-terminal guards.
  - runner: `TestDecisionPayloads_RecoverSameIDsAndRevisionsAfterRestart`,
    multiple-decision ordering, bounded/redacted context, pure-no-I/O projector.
  - desktop: `test("inbox renders control per decision.kind", …)`;
    `test("payload-absent item renders Open only", …)`; `test("inline
    gate decision calls same store action as GateBlockModal", …)`.
  - desktop: `test("draft survives project switch and reload", …)`;
    `test("send clears draft for its key only", …)`.
  - desktop: `test("non-focused quota event creates inbox item, no modal",
    …)`; `test("focused-run quota still opens AccountSwitchModal", …)`.
  - desktop: existing `_runSnapshots` restore + always-revalidate, pinned
    LRU, rapid-switch stale-response, stable-anchor và mutable-alias tests.
- manual checks: 3–5 project × vibe-ingest/vibe-sprint song song — lane
  nền vào waiting hiện inbox <1s; inline approve không switch; draft còn
  nguyên sau vòng switch A→B→A; gate của lane nền không che chat đang gõ.
- failure cases: SSE drop → fallback poll vẫn đúng; action trên payload
  stale → 409 → toast + refresh; run terminal trong lúc preview đang mở.

## 8. Rollout and Fallback

- rollout order: P-2 durable projection → P-1 endpoint/client → P-3/P-6
  surface; P-4 độc lập; P-5 core LRU có thể land trước, mux dirty hook sau.
- fallback path: client gặp endpoint 404/disconnect tiếp tục poll theo CP-82;
  không cần runtime flag/config hoặc migration.
- monitoring: structured local logs cho connect/reconnect, snapshot commit,
  coalesced update/overflow, subscriber cleanup; không log payload text.

## 9. Risks

- `R-1` Non-terminal records không bị prune nên initial snapshot/dirty set
  có thể unbounded: chỉ user-visible top-level lanes, bounded snapshot chunks,
  per-run coalescing + dirty hard-cap; overflow close retryable → reconnect.
- `R-2` Inline action trên payload stale → 409; `evictedRuns` +
  `resolvedPending` đã có semantics cho việc này — reuse, thêm toast.
- `R-3` Decision payload drift giữa provider/flow-pack versions →
  fail-closed: kind không nhận diện được render "Open" only.
- `R-4` Draft key leak khi chat bị xoá/máy khác restore → prune on
  delete + cap; draft không bao giờ coi là durable data.
- `R-5` Scope creep thành full Layer-2 (`runs: Record<runId, RunViewState>`
  + N composer tương tác) — explicit non-goal; nếu P-5 không đủ, mở CP
  riêng.
- `R-6` Raw provider/child activity có thể starve attention → P-1 không
  mirror deltas/tools/tokens/child runs; chỉ bounded meaningful projection.
- `R-7` Reconnect cursor RAM tạo false exactly-once/lost terminal → không
  global cursor; full level snapshot reconcile latest actionable state.
- `R-8` LRU evict snapshot dùng cho back-to-main → pin active ancestry và
  fetch fallback; không tạo parallel `_runViews`.

## 10. Definition of Done

### Functional (per slice)

- [ ] `D-1` `GET /client/events/stream` phát bounded **level-triggered
      run-state projection** cho mọi user-visible top-level lane qua 1
      connection; initial/reconnect snapshot chunked + atomic-on-complete,
      idempotent upsert/remove; child-agent/raw delta/tool/token noise bị loại;
      slow subscriber coalesce latest state với bounded dirty set/retryable
      overflow; leg churn không reattach; poll 30s vẫn là independent
      reconciliation fallback (P-1 / Task-429).
- [ ] `D-2` Mọi durable decision kind (`approval`, `question`, `gate`,
      `ss_lock`, `worktree_merge`, `dispatch_attention`) serialize thành
      versioned `decisions[]` (`decisionId + revision`) với bounded/redacted
      context; quota được enrich từ desktop provider-account authority; kind
      lạ/payload thiếu/stale → "Open" only, never guess (P-2 / Task-430).
- [ ] `D-3` Inbox render control đúng `decision.kind`; expand `▸` hiện
      payload (regressedTests/outputTail, SS quickView, conflictPaths,
      quota candidates) mà không rời chat; filter theo kind + project;
      batch-approve chỉ bao eligible kinds (`approval`/`question` có
      options) — gate/merge/quota luôn per-item (P-3 / Task-431).
- [ ] `D-4` Inline action cho non-focused run hoạt động không cần switch
      — mọi submit path là ID-scoped (`submitApproval`, `answerQuestion`,
      gate decision, merge choice, account switch); lỗi 409 → toast +
      history refresh (P-3 / Task-431).
- [ ] `D-5` Draft keyed `chatId|runId|"<projectId>:new"` sống sót
      `selectProject`/`resetRun`/app reload; send chỉ clear key của nó;
      cap 50 + prune on delete; không bao giờ lên Drive/Supabase (P-4 /
      Task-432).
- [ ] `D-6` Existing `_runSnapshots` được bounded/LRU cho evictable history
      views (không tạo parallel `_runViews`); current/main/active ancestry
      được pin; switch-back restore timeline + stable item anchor rồi always
      revalidate với generation guard; mux revision chỉ mark dirty, terminal
      không xoá useful completed timeline (P-5 / Task-433).
- [ ] `D-7` Event thuộc `runId !== focusedRunId` không bao giờ raise
      `gateBlock`/`pendingAccountSwitch`/`pendingProviderSwitch`/
      `grokYoloPostureLoading` modal — thay vào đó tạo attention item;
      focused run giữ nguyên modal path (P-6 / Task-434).

### Cross-cutting

- [ ] `D-8` Lane nền vào waiting → inbox item + native notification
      (`notification:show`) trong <1s, đo tay trên live runner.
- [ ] `D-9` Provider parity: payload render + decision routing không
      assume provider; missing capability → typed degradation (xem
      `cross-provider-parity` skill).
- [ ] `D-10` SSE drop/reconnect: reconnect với backoff + full snapshot
      reconciliation; không global RAM cursor/exactly-once claim; stale item
      bị xoá, frame cũ bị bỏ theo `(runId, revision)`, poll fallback giữ
      đúng semantics.
- [ ] `D-11` Additive tests only — mọi test mới; pre-existing suite xanh
      hoặc fail byte-identical HEAD baseline; old test fail → STOP +
      report (safe-fix-contract R1).
- [ ] `D-12` `feature_key` per slice: reuse `attention-queue`,
      `project-nav`, `decision-card-ui`; append `event-plane` +
      `chat-drafts` vào `change-audit/FEATURE-KEYS.md`; CA entry mỗi
      slice; commit format `[Feature][<key>] ...` (git-commit-format).
- [ ] `D-13` `CP-84-Test-Steps` §2 automated green + manual M-* + live
      L-* ticked bởi operator.
- [ ] `D-14` GitNexus `detect_changes` trước commit chỉ thấy expected
      symbols (mux handler, emit fan-out, attentionQueue, store fields,
      inbox components).
