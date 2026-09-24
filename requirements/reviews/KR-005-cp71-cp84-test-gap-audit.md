# KR-005 — Audit test coverage hồi tố: CP-71 / CP-81 / CP-82 / CP-83 / CP-84

Ngày: 2026-09-24 · Môi trường thực thi: Windows · Reviewer: retrospective test-gap audit

Mục tiêu: đối chiếu spec + Test-Steps của 5 feature đã `done` với test thực tế,
xác định edge case còn thiếu ở cả 3 lớp (unit / e2e / live-manual) và các lỗi
hạ tầng test khiến claim "đã pass" không còn đáng tin trên Windows.

Trạng thái từng hạng mục dùng 3 nhãn:
- **[SYNCED]** — test tồn tại, đúng logic, đã chạy pass.
- **[MISSING]** — spec yêu cầu nhưng không có test (hoặc test không chạy được).
- **[BLOCKED]** — test tồn tại nhưng không compile/không pass trên Windows.

---

## 0. Kết quả thực thi đã đo

| Suite | Kết quả trên Windows |
|---|---|
| `internal/runner` (CP-71 E2E ×10 + CP-81 lifecycle + CP-84 runUpdates/decision) | PASS — 10/10 CP-71 E2E, 54/54 CP-81+CP-84 focused |
| `internal/worktree`, `internal/lifecycle` | PASS |
| `internal/tui/app` (lifecycle CP-81) | PASS |
| `internal/tui/runnerboot` | **[BLOCKED] không compile trên Windows** |
| `tests/phase1/supervisorLifecycle.test.ts` | **7 pass / 2 FAIL trên Windows** |
| Desktop vitest-equivalent (node --test trên `.phase1-tests` compiled) | 103/103 PASS |
| `npx vitest` (như docs ghi) | Không phải runner của repo — vitest không cài local, `npx` tải vitest@5.0.1 và fail toàn bộ (ESM/`localStorage`) |

---

## 1. Findings hạ tầng test (ưu tiên cao nhất)

### F-1 [BLOCKED][HIGH] `runnerboot_cp81_test.go` không compile trên Windows

`apps/local-runner/internal/tui/runnerboot/runnerboot_cp81_test.go:422`:

```go
if runtime.GOOS != "windows" && !cmd.SysProcAttr.Setsid {
```

`syscall.SysProcAttr` trên Windows **không có field `Setsid`** — runtime guard
không cứu được compile-time check. Hệ quả: toàn bộ 13 test CP-81 của package
runnerboot (compatible reuse, idle/busy stale replacement, protocol
incompatibility, legacy runner, workspace mismatch, unknown-port safety,
single-winner start, detached spawn, fenced kill, stale lock recovery) **không
chạy được trên Windows** — phần đặc thù nhất của CP-81 trên Windows lại là
phần không được kiểm chứng.

Khuyến nghị: tách assertion `Setsid` vào file `//go:build !windows` hoặc helper
per-OS (`assertDetachedPosix` trong file unix-only).

### F-2 [BLOCKED][HIGH] Supervisor: 2/9 test fail trên Windows

`tests/phase1/supervisorLifecycle.test.ts`:
- `TestSupervisor_ForceShutdownStopsRunnerWebDesktop`
- `TestSupervisor_TerminalCloseLeavesNoOrphans`

Nguyên nhân: test mock `process.kill` và expect record `SIGINT`/`SIGKILL`,
nhưng production path Windows (`scripts/supervisor.js:226-250`, `:882-904`) đi
qua `killProcessTree` → `execSync("taskkill /F /T /PID …")` — mock không bắt
được `execSync`. Đây nhiều khả năng là **test-seam mismatch** chứ chưa chứng
minh cleanup production hỏng, nhưng claim "supervisor suite green" **sai trên
Windows**.

Khuyến nghị: mock seam `execSync`/`child_process` hoặc tách assertion Windows
thành case riêng verify `taskkill /F /T` được gọi với đúng PID.

### F-3 [MED] Docs ghi `npx vitest` nhưng repo không dùng vitest

Các Test-Steps docs (CP-82/83/84) hướng dẫn `npx vitest`. Thực tế repo dùng
Node test runner trên output compile `.phase1-tests/`. Chạy đúng:
`node --test .phase1-tests/<file>.test.js`. Cần sửa docs hoặc thêm npm script —
nếu không ai chạy theo docs sẽ thấy "toàn bộ test fail" giả.

### F-4 [LOW] `quit_kills_reused_runner_test.go` — KHÔNG stale

Đã verify `cmdShutdownAndQuit` (`app.go:6820-6851`) là path "Turn off FlowPilot"
chủ động (CP-81 D-4, fenced shutdown) — POST `/system/shutdown` kể cả khi
`OwnsRunner=false` là đúng contract; ordinary close đi qua
`cmdExitIntent → cmdReleaseAndQuit`. **[SYNCED]**, không phải test cũ sót lại.

---

## 2. CP-71 — Run Worktree Isolation

Verdict: **core lifecycle được phủ tốt** (manager unit + 10 HTTP E2E + desktop
toggle tests đều pass). Gap tập trung ở **các merge-mode chưa được gọi qua HTTP**.

### Đã phủ [SYNCED]
- Manager: create/list/resolve/cleanup/conflict/GC trên repo thật, serialized
  apply mutex, dirty-main pre-check, uniqueness (same-chat share / cross-chat
  distinct / collision fail-closed).
- HTTP E2E (`cp71_worktree_e2e_test.go`): full lifecycle
  start→binding→cwd→terminal→merge→cleanup; leg-switch share; conflict
  evidence + retry (409 + conflictPaths + patchArtifactRef); resume sau
  restart; external delete → `lost` + chặn leg mới (E-8); client gate 403
  (nil/admin/mcp); boot GC chỉ prune orphan; session-write không xoá binding;
  off-byte-parity; delete-gate + inline `?worktree=discard`.
- Desktop: toggle render/disable/badge/merge card intent tests.

### Thiếu [MISSING]
| # | Edge case | Mức | Ghi chú |
|---|---|---|---|
| 71-a | `worktree_unavailable` khi cwd **không phải git repo** | MED | `enforceWorktreeStart` có implement, spec P-2/AC yêu cầu; 0 test gọi |
| 71-b | `keep_branch` qua HTTP resolve → state `kept_branch` + branch `run/<slug>` còn | MED | chỉ có `TestManager_CleanupKeepBranch` ở mức manager |
| 71-c | `discard` với untracked files → 409 `worktree_discard_confirm` + resend `confirm=true` | MED | two-step confirm path trong `run_worktree_merge.go:211-220` chưa ai gọi |
| 71-d | `archive` trên binding `lost` | MED | mode tồn tại (`:226`) nhưng chưa test |
| 71-e | `recreate_empty` trên `lost` → tạo worktree mới, rebind `workspaceCwd`, `priorChangesLost` | **HIGH** | Đây là recovery path duy nhất cho E-8 `lost` — spec offer trong `worktree_lost` event options mà không có test nào verify nó hoạt động |
| 71-f | `apply_patch`/`keep_branch` trên `lost` → 409 `worktree_lost` | LOW | fail-closed path chưa verify |
| 71-g | `recreate_empty` trên non-lost → `worktree_not_lost`; `invalid_mode` → 400 | LOW | negative-path |
| 71-h | `worktree_merge_failed` (apply error không phải conflict) | LOW | nhánh `:196` |
| 71-i | `ensureGitignore` tự thêm `.flowpilot/` vào `.gitignore` | LOW | side-effect chưa assert |
| 71-j | E2E **hai run đồng thời cùng project, file overlap → merge tuần tự, run 2 conflict** | MED | spec §7 nêu; hiện chỉ có single-run conflict + mutex unit test — chưa có kịch bản "A merge xong → HEAD đổi → apply của B conflict" |
| 71-k | Provider-cwd assert chỉ kiểm gián tiếp; chưa chứng minh `workspaceCwd` đúng cho cả 3 provider trong E2E | LOW | `run_worktree_parallel_test` check `workspaceCwd` nhưng provider parity thực chỉ implicit |
| 71-l | `markChatWorktreeState` lan state sang mọi session cùng chat | LOW | chưa test trực tiếp |

### Live/manual
- **Không có file `CP-71-Test-Steps.md`** — các feature khác đều có checklist
  manual; CP-71 thiếu hẳn artefact này.
- Chưa thấy bằng chứng live cho: restart runner giữa run, `keep_branch` để lại
  branch `run/<slug>`, 2 vibe run đồng thời cùng project (chỉ có Go
  concurrency test, chưa có live evidence).

---

## 3. CP-81 — Shared Runner Lifecycle

Verdict: **lifecycle core + HTTP contract + TUI phủ rất tốt và pass**; nhưng
runnerboot (phần Windows-critical) và supervisor **không xanh trên Windows**
(xem F-1, F-2).

### Đã phủ [SYNCED]
- `internal/lifecycle` (29 test): lease register/idempotent/heartbeat/stale
  generation+token/expiry/release, idle grace, attach cancels shutdown, active
  work block, orphan recovery, confirm token single-use/stale/expired/wrong
  instance, drain, redaction, persistent mode, boot grace, shutdown idempotent.
- `lifecycle_api_test.go` (9 test): HTTP register/heartbeat/release/snapshot/
  stale/drain/busy-shutdown/concurrent.
- `cp81_lifecycle_e2e_test.go` (19 test): full matrix gồm
  `/system/restart` 202 + confirm negotiation + heartbeat qua drain + restart
  metadata/deadline (`TestE2E_PlannedRestartReconnectsBothClients`,
  `TestE2E_ReconnectTimeoutClosesClient`), unplanned death, stale-runner
  replacement signal, …
- `tui/app/lifecycle_cp81_test.go` (14 test): registration, heartbeat,
  planned/unplanned loss, close choices, cancel, deadline, status display.
- Desktop: 17 lifecycle + 3 retry test — PASS (qua node --test).

### Thiếu / chưa chạy được
| # | Edge case | Mức | Ghi chú |
|---|---|---|---|
| 81-a | Toàn bộ runnerboot matrix | **HIGH** | [BLOCKED] F-1 — không compile trên Windows |
| 81-b | Supervisor force-kill tree / terminal-close orphan cleanup trên Windows | **HIGH** | [BLOCKED] F-2 — 2 test đỏ |
| 81-c | Supervisor "không trở thành permanent user lease" | MED | có trong suite nhưng cần re-run sau khi sửa seam |
| 81-d | Provider subprocess cleanup khi runner chết | MED | spec E2E yêu cầu; chưa thấy test riêng kill process tree của provider child |
| 81-e | Requester attribution (ai gọi shutdown/restart) | LOW | fenced bằng leaseId+token, có coverage gián tiếp |
| 81-f | Live: Task Manager kill runner → clients notify+close; crash mid-run | MED | chỉ mô phỏng `srv.Close()`; chưa live Windows evidence |

---

## 4. CP-82 — Multi-Project Parallel Vibe Ops

Verdict: **state/model layer phủ tốt**; thiếu ở component render và live matrix.

### Đã phủ [SYNCED]
- `boardModel.test.ts` — group theo project, ordering.
- `attentionQueue.inline.test.ts` + `store.attention-actions.test.ts` — inline
  approve/answer/gate/ss_lock/worktree_merge/dispatch; unactionable → Open-only;
  stale 409; focus preservation.
- `store.spectator.test.ts` — spectator read-only.
- Worktree uniqueness Go tests (same-chat share / cross-chat distinct /
  collision fail-closed).
- Live evidence đã ghi: distinct worktrees cùng project, collision behavior,
  mux attention cross-project, dispatch resolve.

### Thiếu [MISSING]
| # | Edge case | Mức | Ghi chú |
|---|---|---|---|
| 82-a | `SessionsBoard` component render test (grouping, badge, row state) | MED | hiện chỉ test model/derivation |
| 82-b | Spectator "không bao giờ attach stream" — spy assert ở component/client | MED | store-level đã có; mức "không gọi subscribe" chưa có spy trực tiếp |
| 82-c | Run finish **trong lúc row đang render** (stale frame race) | MED | spec nêu; chưa có test mô phỏng render race |
| 82-d | Resolved-elsewhere: item resolved ở focused run phải biến mất đồng nhất khỏi board+inbox | LOW | phủ gián tiếp qua evict tests |
| 82-e | Live `waiting_approval` thật (không phải `dispatch_attention` thay thế) | MED | môi trường test trước đây không ép được approval thật |
| 82-f | Manual M-1…M-7 (4 projects × parallel flows, board accuracy khi streaming, inline không cướp focus, spectator không attach) | MED | trong Test-Steps vẫn pending |

---

## 5. CP-83 — Embedded Terminal

Verdict: **contract `worktreePath` + cwd resolver + bridge/panel phủ tốt và
pass**; thiếu ở lifecycle Electron thật và manual matrix.

### Đã phủ [SYNCED]
- Go contract: `worktreePath` có khi bound / vắng khi unbound / tồn tại sau
  restart (persisted session).
- `terminal.test.ts` + `terminalPanel.test.ts` (19 test): spawn args, write/
  resize/kill/exit, spawn-failure cleanup, bridge channels + unsubscribe, cwd
  resolution 3 mức, zero-coupling spy.
- Live evidence: start/history/resume path; node-pty chạy được dưới Electron.

### Thiếu [MISSING]
| # | Edge case | Mức | Ghi chú |
|---|---|---|---|
| 83-a | Renderer reload → tất cả PTY die | MED | registry kill-all có unit test; reload Electron thật chưa có |
| 83-b | App quit → cleanup | MED | cùng lý do |
| 83-c | Worktree bị xoá ngoài → cwd fallback (M-6) | MED | spawn-failure generic đã cover; case filesystem-thật chưa live |
| 83-d | Interactive programs (vim/htop/watch) trên Windows PTY | MED | cần live evidence |
| 83-e | Narrow window layout (M-7) | LOW | manual pending |
| 83-f | "Terminal không mutate run/timeline/orchestration" — bằng chứng mạnh hơn spy | LOW | spy test tồn tại; chưa có mutation-guard test |

---

## 6. CP-84 — Realtime Multi-Lane Attention

Verdict: **projection/dedupe/coalesce/overflow core phủ rất tốt và pass** — đây
là phần được test kỹ nhất. Gap lớn nằm ở **lớp transport HTTP SSE, một số
decision-kind payload chưa có test, reconnect/backoff phía desktop, và
notification end-to-end**.

### Đã phủ [SYNCED] (Go `decision_payload_test.go` + desktop state tests)
- Snapshot atomic + subscribe trước khi snapshot; multi-subscriber fan-out;
  per-subscriber remove bookkeeping; terminal remove exactly-once; no
  cross-run contamination; upsert revision tăng khi `rs.seq` không đổi
  (`TestRunUpdates_UpsertRevisionAdvancesWithoutSeqChange` — bắt đúng bug
  dedupe); không emit message/tool/token noise; turn-completed trước gate-pass
  không terminal; provider-switch leg hiện không cần reattach; per-run stream
  không đổi; child-agent runs excluded; slow-subscriber coalesce giữ
  waiting-state; dirty overflow → resync + bound memory; dispatch resolve mark
  dirty; bounds+redaction; multiple pending; ID/revision recovery sau restart;
  missing SS quick view → non-actionable; live SS lock với quick view; JSON
  round-trip.
- Desktop: snapshot reconcile, stale revision suppression, chunked commit,
  terminal-merge retention, resync leaves lanes, drafts (key/nav/clear/
  failure/localStorage/prune/cap), runViewCache (LRU/pin/revalidate gen/
  scroll/copy/delete/replay), modalRouting (quota/gate/fail-safe/dedupe/
  provider switch), inboxDecisions (mọi kind, quick view, worktree merge
  controls, quota, Open-only degrade, batch, filter, 409, focus).

### Thiếu [MISSING]
| # | Edge case | Mức | Ghi chú |
|---|---|---|---|
| 84-a | **`handleAllEventsStream` HTTP handler: 0 test** — chunk boundary `runUpdateSnapshotChunk=32`, `Complete` chỉ ở chunk cuối, SSE wire format (`event:`/`data:`), heartbeat frame, resync → đóng stream | **HIGH** | mọi test dùng `subscribeRunUpdates`/`drainRunUpdates` internals, không qua HTTP |
| 84-b | `TestRunUpdates_ReconnectSnapshotReconcilesMissedTerminalRemove` — test đúng tên không tồn tại; snapshot mới sau reconnect loại run terminal (isLaneRelevant) chưa được assert ở Go | MED | desktop-side reconcile có, nhưng phía Go chưa chứng minh terminal-run-bị-miss không quay lại |
| 84-c | `TestDecisionPayloads_GateCarriesBoundedContext` — `gateDecisionPayload`/`pendingGateBlock` (bounded regressedTests/options) chưa test | MED | named trong Test-Steps §2 |
| 84-d | `TestDecisionPayloads_MergeUsesDurableBindingRevision` — `worktreeDecisionPayload` (conflictPaths/patchRef/durable revision) chưa test | MED | named trong Test-Steps §2 |
| 84-e | `TestDecisionPayloads_QuestionKeepsNativeQuestionOptions` — `questionDecisionPayload` options+multiSelect chưa test | MED | named trong Test-Steps §2 |
| 84-f | `dispatchDecisionFromAttention` payload content: `settle_pending` → non-actionable, repair → id `dispatch-repair:` | MED | chỉ có dirty-marking test, payload chưa |
| 84-g | `TestDecisionPayloads_ProjectorPerformsNoIOWhileLocked` — guard chống I/O trong critical section | LOW | design có 2-phase; chưa có test cấm regression |
| 84-h | `HttpWsRunnerClient` SSE parser: frame bị cắt giữa chunk, nhiều frame trong 1 chunk, malformed frame | **HIGH** | parser `HttpWsRunnerClient.ts:508-564` không có test nào |
| 84-i | `consumeRunUpdatesLoop` backoff 500ms→30s+jitter, reset sau healthy snapshot, reconnect sau stream-end | **HIGH** | `store.ts:3981-4031` không test; spec §4 nêu rõ |
| 84-j | Resync frame → reconnect thật + snapshot thay thế authoritative | MED | state-level "resync leaves lanes" có; transport-level chưa |
| 84-k | Stale `(runId, revision)` drop **xuyên reconnect** | MED | suppression trong 1 session có; qua reconnect chưa |
| 84-l | Notification e2e: `notification:show` → click → `notification:clicked` → `openRunAtAttention` đúng project/chat khi item vắng trong queue | **HIGH** | plumbing tồn tại (`App.tsx:141-151`, `electron/main.ts:231-245`) nhưng không test; spec đòi <1s — chưa đo |
| 84-m | Background-run event không mở modal focused | LOW | modalRouting cover phần lớn; verify thêm quota/notification path |
| 84-n | `r_requirement` kind: payload map về `submitGateDecision` ở desktop — có chủ đích không? | LOW | spec list không nêu kind này; cần xác nhận actionable hay Open-only |
| 84-o | Provider parity chỉ test Claude/Codex/Grok — spec D-9 nêu cả Gemini/OpenCode/Devin | LOW | projection provider-agnostic by construction; extend test cho đủ 6 hoặc ghi lý do |
| 84-p | Snapshot exclusion của terminal-run-còn-actionable khi subscriber mới connect | LOW | `isLaneRelevant` giữ lane; desktop có test tương đương, Go-side snapshot chưa |
| 84-q | Perf/bounded-memory toàn desktop queue+cache dưới burst tần số cao | LOW | từng phần có bound; chưa có soak test |
| 84-r | Manual M-1…M-11 + Live L-1…L-7 (notification <1s, reconnect đo lường, stale decision, payload degrade, focus giữ) | **HIGH** | toàn bộ checklist trong Test-Steps còn unchecked |

---

## 7. Danh sách hành động theo severity

**P0 — sửa trước khi tin "green"**
1. F-1: tách `Setsid` assertion ra file unix-only → runnerboot compile được trên
   Windows.
2. F-2: sửa seam supervisor test (mock `execSync` hoặc case Windows riêng cho
   `taskkill`).
3. F-3: sửa docs/npm script — runner thật là `node --test .phase1-tests/…`.

**P1 — thêm test (additive-only, không sửa test cũ)**
4. CP-84 transport: test `handleAllEventsStream` qua `httptest` (chunk 32,
   Complete flag, resync close, heartbeat) — 84-a.
5. CP-84 client: test SSE parser `HttpWsRunnerClient` (fragmented/multi/
   malformed frame) + `consumeRunUpdatesLoop` backoff/reset — 84-h, 84-i.
6. CP-84 payload: gate bounded context, worktree_merge durable revision,
   question native options, dispatch payload content — 84-c…84-f.
7. CP-71: `recreate_empty`/`archive`/`discard-confirm`/`worktree_unavailable`/
   `invalid_mode` qua HTTP — 71-a…71-g.
8. CP-84 notification deep-link test (mock Electron IPC) — 84-l.
9. CP-82 `SessionsBoard` render + spectator no-subscribe spy — 82-a, 82-b.

**P2 — live/manual evidence cần chạy**
10. CP-84 M-1…M-11, L-1…L-7 (đặc biệt notification <1s, reconnect timing).
11. CP-83 M-2…M-7 (real Electron reload/quit cleanup, vim/htop, narrow layout).
12. CP-82 M-1…M-7 + live `waiting_approval` thật.
13. CP-81 Task Manager kill / provider-subprocess cleanup live trên Windows.
14. CP-71: viết `CP-71-Test-Steps.md` cho đồng bộ với các CP khác + live 2-run
    overlap merge.

**Phân loại mâu thuẫn đã giải quyết**
- `quit_kills_reused_runner_test.go`: **[SYNCED]** — là path "Turn off" chủ
  động, đúng CP-81 D-4, không phải stale test.
