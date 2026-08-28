# Task-309: TUI And Desktop Retry/Stop/Allow For Frozen-Contract Scope Drift

## Metadata

- Document ID: `Task-309`
- Title: `TUI And Desktop Retry/Stop/Allow For Frozen-Contract Scope Drift`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-27`
- Last Updated: `2026-08-27`
- Parent Documents: [CP-43](../../07-Coding-Plan/done/CP-43-Change-Contract-And-Canonical-Intent-Signature.md), [CP-43-Test-Steps F3](../../07-Coding-Plan/done/CP-43-Test-Steps.md), [CP-55](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md)
- Child Documents: `None`
- Related Documents: [CA-647](../../../change-audit/) (live F3 amend REST), BUG-231 FlowAwaitingUser Continue/Stop parity
- Tags: `cli-tui, desktop, frozen-contract, scope-drift, cp-43, cp-55`

---

## AI Quick View

### Summary

- Frozen-contract scope-drift park (`WAITING_USER` / escalate `flow scope drift: wrote outside the frozen contract's declared paths: ...`) chỉ có `[Continue]` `[Stop]` — Continue retry freeze cũ không nới scope, block lại (F3 `run-169381`).
- REST `POST /client/workflow-runs/{runId}/agent-loop/amend {"paths":[...]}` đã ship (CP-55 P-4, `handleAmendFlow` → `AmendFrozenContract` union+v2+Supersedes) nhưng chưa có UI.
- Thêm UI **Retry / Stop / Allow** trên TUI blocked bar và Desktop `FlowAwaitingUserCard`. `Allow` (amend) 1 click gửi paths parse từ `GateReason`, resume như Continue.

### Current Ask

Implement TUI + Desktop `Retry`/`Stop`/`Allow` per T-1…T-6. Additive tests only. Do not change runner amend API.

### Key Decisions

- `T-1` Nút `Allow` **chỉ hiện khi** `GateReason` chứa `wrote outside the frozen contract's declared paths:` (copy y nguyên `gate_hook.go:863`). Không hiện khi `cap` / `member_stalled` / reviewer escalate không kèm drift.
- `T-2` Paths cho `Allow` = suffix sau `:` của câu trên, split `,`, trim, bỏ rỗng. Không form nhập path, không slash command. Parse rỗng → không chip.
- `T-3` 3 nút đều hiện khi drift: **Retry** — run again with old scope (`POST .../continue`), **Stop** — end flow (`POST .../stop`), **Allow** — continue with new scope (widen freeze union drifted paths, `POST .../amend`).
- `T-4` TUI và Desktop cùng ngữ nghĩa (operator chốt TUI+Desktop).
- `T-5` Chỉ test mới. Không sửa test Continue/Stop cũ cho xanh.
- `T-6` Không đổi `handleAmendFlow` / `AmendFrozenContract`.

### Constraints

- Additive-tests-only + oracle-rule — không sửa matrix cũ.
- Giữ `Retry` + `Stop` trên mọi blocked park; chỉ thêm `Allow` khi drift.
- Không path picker, không ẩn Retry lúc drift, không đổi API.

### Open Questions

- None (phạm vi TUI+Desktop + label Retry/Stop/Allow đã chốt).

### Source Refs

- CP-43 F3 `Doi ham Subtract(a, b int) int …` — freeze chỉ `calc.go` → tester ghi `calc_test.go` → drift; live `run-169381`.
- CP-55 P-4 `AmendedFrozenContract` / `POST .../agent-loop/amend`.

## 1. Goal

Operator bị frozen-contract scope drift có thể chọn 1 trong 3 hành vi từ TUI/Desktop mà không cần PowerShell: `Retry` (retry old scope), `Stop` (end flow), `Allow` (widen freeze union drifted paths và resume). **Retry** lặp lỗi nếu không Allow; **Allow** ghi v2 `Supersedes` v1.

## 2. Parent Links

- coding plan: [CP-43](../../07-Coding-Plan/done/CP-43-Change-Contract-And-Canonical-Intent-Signature.md) F3, [CP-55](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md) P-4
- tech design: delta UI; API đã ship `interactive_handlers.go:1511`
- system spec: n/a
- specific upstream ids: BUG-231 Continue/Stop chips, CA-647 amend REST

## 3. Trigger

F3 `run-169381` block chuẩn (`flow scope drift: wrote outside the frozen contract's declared paths: calc_test.go`) nhưng TUI chỉ `[Continue] [Stop]` (hiện rename: Retry/Stop). Operator phải REST amend. Continue lúc drift = retry freeze cũ → block lại → khó hiểu. Yêu cầu 3 nút có description rõ: Retry / Stop / Allow.

## 4. Exact Change

- `T-1` **Parse helper** từ `flowGateReason`/`loopState.gateReason`: nếu chứa `wrote outside the frozen contract's declared paths:` thì lấy suffix sau `:`, `strings.Split(..., ",")`, trim, bỏ rỗng → `[]string`. Empty → không Allow.

- `T-2` **TUI** — `apps/local-runner/internal/tui/`:
  - `client/client.go`: `AmendFlow(ctx, runID string, paths []string) (*AgentGraphSnapshot, error)` → `POST /client/workflow-runs/{runId}/agent-loop/amend`.
  - `app/step_runtime.go`: `parseDriftedPaths(gate string) []string` + expose cho bar; `renderBlockedBar()` render `flow [blocked] (escalate) awaiting your decision` + `reason: flow scope drift: ...` + dòng chip:
    - Luôn: `[Retry]`  `retry — run again with old scope` + `[Stop]`  `stop — end flow`
    - Thêm khi parse non-empty: `[Allow]`  `allow — continue with new scope (match code changed)` — style `styleLink` như Continue/Stop, có suffix description `styleSystem`.
  - `app/mouse.go`: `hitBlockedChrome` map click `[Retry]`/`[Allow]`/`[Stop]` → `retry`/`allow`/`stop`. Giữ hit cho `[Continue]` legacy alias nếu cần compat, nhưng View mới không render nó.
  - `app/app.go`: `cmdAmendFlow(runID, paths)` POST amend, trả `AgentGraphHydratedMsg`; `case target == "retry": cmdContinueFlow`, `case target == "allow": cmdAmendFlow`, `case target == "stop": cmdStopTurn`. Alias `continue` → `retry` để không gãy test cũ chờ deprecation.
  - `app/app_extras.go` / `model.go` nếu cần: không đổi `flowLoopBlocked()`.

- `T-3` **Desktop** — `apps/desktop-flowpilot/src/`:
  - `types/contract.ts`: `amendFlow?(runId: string, paths: string[]) => Promise<AgentGraphSnapshot>`.
  - `client/HttpWsRunnerClient.ts`: `amendFlow(parentRunId, paths) => postJSON(.../agent-loop/amend, {paths})`.
  - `client/MockRunnerClient.ts`: stub `amendFlow`.
  - `state/store.ts`: `amendFlow(paths: string[])` đọc `agentGraphSnapshot.loopState.gateReason`, guard `blocked`, gọi `client.amendFlow`, `applyAgentGraphSnapshot`. Giữ `continueFlow`/`stop`.
  - `components/FlowAwaitingUserCard.tsx`: khi `loopState.status==="blocked"` render card. Parse như TUI. Luôn có 2 nút base; khi drift parse non-empty thêm nút Allow:
    ```
    [Stop]         — end flow
    [Retry]        — run again with old scope
    [Allow]        — continue with new scope (match code changed)  // primary (btn-primary) khi drift
    ```
    `handleRetry` → `continueFlow(feedback.trim())`, `handleAllow` → `amendFlow(paths)`, `handleStop` → `stop()`. Giữ layout `Other row`. `stalled` (`member_stalled`) không hiện Allow.
  - `FlowAwaitingUserCard` vẫn hide khi `gateBlock` open (CP-51).

- `T-4` **Label/description contract** (để test assert, không hardcode trôi):
  - `Retry` label: `Retry`; desc: `run again with old scope`
  - `Stop` label: `Stop`; desc: `end flow`
  - `Allow` label: `Allow`; desc: `continue with new scope match code changed` (UI render `continue with new scope (match code changed)` cho vừa width)

- `T-5` **Tests — additive only** (không sửa file test cũ):
  - TUI: `tui_blocked_retry_stop_allow_test.go` — View khi `blocked+escalate+GateReason=drift: calc_test.go` chứa `[Retry]`/`[Stop]`/`[Allow]` + desc; View khi `cap`/`member_stalled`/không GateReason chỉ Retry+Stop; click Allow POST body `{"paths":["calc_test.go"]}` và hydrates `running`; Claude/Codex/Grok param.
  - Desktop: `FlowAwaitingUserCard.test.tsx` + `store.test.ts` amendFlow — hiện/ẩn Allow, click gọi client với paths, không gọi khi stalled.

- `T-6` **I18n/width**: wrap `reason` như cũ (`wrapText`); chip row không wrap description dài — description là suffix nhỏ bên phải mỗi chip.

## 5. Touched Areas

- files: `apps/local-runner/internal/tui/client/client.go`, `apps/local-runner/internal/tui/app/step_runtime.go`, `apps/local-runner/internal/tui/app/mouse.go`, `apps/local-runner/internal/tui/app/app.go`, new `apps/local-runner/internal/tui/app/tui_blocked_retry_stop_allow_test.go`, `apps/desktop-flowpilot/src/types/contract.ts`, `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`, `apps/desktop-flowpilot/src/client/MockRunnerClient.ts`, `apps/desktop-flowpilot/src/state/store.ts`, `apps/desktop-flowpilot/src/components/FlowAwaitingUserCard.tsx` + tests, `apps/desktop-flowpilot/src/state/*.test.ts`
- modules: TUI blocked bar, Desktop FlowAwaitingUser
- routes: existing `POST /client/workflow-runs/{runId}/agent-loop/amend` only (no new route)
- tables: none

## 6. Acceptance Check

- GateReason `flow scope drift: wrote outside the frozen contract's declared paths: calc_test.go` → TUI bar và Desktop card hiện 3 nút `Retry`/`Stop`/`Allow` kèm description đúng chữ.
- Click `Allow` → `POST .../amend {"paths":["calc_test.go"]}`; frozen `frozen_contracts.ndjson` v2 `Supersedes` v1, `declared_paths` = union (`calc.go` + `calc_test.go`), flow resume (hydrated `running`). Multi-path `a.go, b.go` → union cả hai.
- Click `Retry` lúc drift → `POST .../continue` với freeze cũ; gate block lại same drift (không nới).
- Click `Stop` → `POST .../stop`, flow ended.
- Khi `cap` / `member_stalled` / reviewer escalate không kèm drift string / parse rỗng → chỉ `Retry`+`Stop`, không `Allow`.
- Old tests `TestBlockedBar_*ContinueStop*` vẫn xanh (không sửa) — alias `continue`→`retry` giữ compat hoặc test mới không phá cũ.
- F3 manual: `run-169381` pattern block trên `test_signatures` → click `Allow` trong TUI (không REST) → v2 + resume; Desktop tương tự.

## 7. Out of Scope

- Sửa `handleAmendFlow` / `AmendFrozenContract` / `gate_hook.go` message prefix
- Ẩn `Retry` lúc drift, thêm path picker text input, slash `/amend` / `/retry`
- OrchestrationBoard UI riêng cho drift
- Rewrite CP-43 Test-Steps tick wording (optional follow-up)
- I18n tiếng Việt cho label (giữ English cho code parity)

## 8. Completion Notes

- result: (empty until done)
- follow-ups: optional tick `CP-43-Test-Steps.md:430` "Allow từ TUI/Desktop" thay vì REST
- upstream docs updated: no (UI delta only)
