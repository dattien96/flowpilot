# BUG-267: Desktop History Open Account-Not-Signed-In Error Shows No User-Facing Modal

## Metadata

- Document ID: `BUG-267`
- Title: `Desktop History Open Account-Not-Signed-In Error Shows No User-Facing Modal`
- Phase: `bugfix`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [SD-15: Claude Cross-Acc Pc Sync Design](../../06-System-Tech-Design/SD-15-(Not-Test-Yet)-Claude-Cross-Acc-Pc-Sync-Design.md), [Task-063: Desktop Chat Provider Card Picker](../../08-Task/done/Task-063-Desktop-Chat-Provider-Card-Picker.md)
- Child Documents: `None`
- Related Documents: [BUG-091: Drive Restore Rejects Same Session Prefix Extension As Conflict](../done/BUG-091-Drive-Restore-Rejects-Same-Session-Prefix-Extension-As-Conflict.md), [BUG-170: Workflow Flow Mode History Unresumable After Server Restart](../done/BUG-170-Workflow-Flow-Mode-History-Unresumable-After-Server-Restart.md), [BUG-265: Built-In Orchestration Select Missing When ChatStartMode Restored Not Clicked](../done/BUG-265-Built-In-Orchestration-Select-Missing-When-ChatStartMode-Restored-Not-Clicked.md)
- Replaces: `None`
- Tags: `desktop-chat, history-open, drive-restore, provider-account, claude, ux, modal`

## AI Quick View

### Summary

- Khi user sync một chat Claude lên Drive rồi restore/pull về trên PC không có Claude account đang active, runner trả lỗi đúng: `code="account_not_signed_in"` với message `can't open — the active account isn't signed in`.
- Desktop store hiện coi lỗi này là một trạng thái "unavailable" của history item/remote session và **nuốt lỗi** sau khi ghi `unavailableReason`; UI không show modal/toast/message chủ động nên cảm giác với user là "bấm mà không có gì xảy ra".
- Đây là gap UX, không phải gap semantics của runner: error code/message hiện tại là đúng và còn cần được giữ.
- Bug cần fix sau: khi open/restore history vấp `account_not_signed_in` hoặc `account_unavailable`, desktop phải show **user-facing modal** giải thích rằng máy hiện tại không có provider này hoặc không có active account phù hợp.

### Current Ask

- Capture bug để fix sau: bổ sung feedback UI rõ ràng khi mở/restore chat history thất bại vì machine hiện tại không có provider account đang signed in.

### Key Decisions

- `V-1` Giữ nguyên error semantics ở runner (`account_not_signed_in` / `account_unavailable`); fix nằm ở **desktop UX surface**, không phải đổi runner thành success/fallback.
- `V-2` Modal phải là phản hồi **ngay tại thời điểm click**, không bắt user tự dò tooltip/row disabled state mới hiểu chuyện gì xảy ra.
- `V-3` Vẫn giữ `unavailableReason` trên row/session item để list view và state sync không mất thông tin; modal là lớp feedback bổ sung chứ không thay state marking.

### Constraints

- Không được biến case này thành "open thành công nhưng transcript rỗng" hay auto-switch provider âm thầm; bản chất vẫn là open/restore thất bại.
- Error copy cần nói rõ 2 case user-facing:
  - máy không có provider/account này
  - hoặc có provider nhưng chưa sign in active account phù hợp
- Fix phải cover cả `openHistoryRun` và `restoreRemoteChatSession` path nếu cả hai đều nuốt cùng error family.

### Open Questions

- `Q-1` Modal có nên cho action trực tiếp như "Open Provider Accounts" / "Sign in Claude" không? Đề xuất: có, nếu existing settings route/modal đã sẵn.
- `Q-2` Có nên gộp luôn `session_unavailable`, `account_not_signed_in`, `account_unavailable`, `resume_unsupported` vào một generic "History open failed" modal, hay chỉ special-case provider-account family? Đề xuất: special-case provider-account family trước.

### Source Refs

- `apps/local-runner/internal/runner/interactive_resume.go:933-1029` — runner trả `account_not_signed_in` với message hiện tại.
- `apps/desktop-flowpilot/src/state/store.ts:1454-1464` — `restoreRemoteChatSession` catch lỗi này, set `remoteChatSessions[].unavailableReason`, rồi return.
- `apps/desktop-flowpilot/src/state/store.ts:1496-1497` và test `store.test.ts:380-399` — `openHistoryRun` catch lỗi này, set `runHistory[].unavailableReason`, rồi giữ nguyên current run.
- `apps/desktop-flowpilot/src/components/Navigator.tsx` — item unavailable bị disable/tooltip, nhưng không có modal feedback chủ động.

## 1. Issue Summary

Desktop history open/restore hiện xử lý đúng về mặt state khi runner báo `account_not_signed_in`, nhưng trải nghiệm user bị cụt: click vào history item trên máy không có provider account phù hợp chỉ sinh log + stamp `unavailableReason`, không có modal hay thông báo chủ động để user biết vì sao thao tác của họ không mở được chat.

## 2. Parent Links

- impacted coding plan: none identified yet; likely desktop history/sync UX follow-up rather than runner protocol work
- impacted tech design: `SD-15` (cross-PC sync semantics: session data sync, auth does not)
- impacted system spec: none identified yet

## 3. Environment and Reproduction

- environment: desktop FlowPilot app, chat history/Drive restore flow, source run originally created with Claude, target PC has no Claude active account
- reproduction steps:
  1. Sync a Claude chat run to Drive on PC A.
  2. Pull/restore that run on PC B where Claude is not signed in or not available as the active account.
  3. Click the history entry / restore action.
  4. Observe runner logs `code="account_not_signed_in" message="can't open — the active account isn't signed in"`.
  5. Observe desktop does not show a modal or equivalent direct feedback; the item only becomes unavailable/disabled.
- frequency: deterministic whenever the target machine lacks a compatible signed-in provider account.

## 4. Expected vs Actual

- expected: user gets an immediate modal explaining that this machine does not have the required provider or active account, and what action they need to take next.
- actual: action silently returns to the current screen; only internal state changes (`unavailableReason`) and logs reveal the reason.

## 5. Impact

- users affected: anyone restoring/opening synced chat history across machines with provider-account mismatch, especially Claude history on a non-Claude-signed-in PC.
- workflows affected: local history reopen and remote Drive restore of chat sessions.
- severity: medium — no data corruption and runner semantics are correct, but the UX currently hides the actionable failure from the user.

## 6. Root Cause

- hypothesis: the desktop intentionally downgraded provider-account mismatch errors from "throw" to "unavailable row state" so the current run is not blown away, but never added a user-facing feedback surface on top of that downgrade.
- confirmed cause:
  - `openHistoryRun` catches `account_not_signed_in` / `account_unavailable`, updates `runHistory[].unavailableReason`, and returns without throwing.
  - `restoreRemoteChatSession` does the same for `remoteChatSessions[].unavailableReason`.
  - Navigator renders those items disabled / with tooltip text, but nothing opens a modal or toast at click time.
- evidence:
  - `store.test.ts` explicitly asserts the state behavior for `openHistoryRun` and considers it success if current run is preserved and `unavailableReason` is set.
  - User-visible symptom matches this path exactly: logs show the error, but UI does not actively announce it.

## 7. Fix Strategy

- `F-1` Add a dedicated desktop UI surface for provider-account availability failures when opening/restoring history — preferably a modal triggered immediately from the caught error path.
- `F-2` Keep the existing `unavailableReason` row/session stamping so list state, disabling, and retry behavior remain intact.
- `F-3` Normalize user-facing copy for:
  - provider missing on this machine
  - provider present but no active signed-in account
  - optional CTA to open Provider Accounts / sign in the required provider
- `F-4` Add tests for both `openHistoryRun` and `restoreRemoteChatSession` asserting modal-trigger state in addition to `unavailableReason`.

## 8. Validation

- `V-1` Reproduce the Claude-on-PC-B case and confirm a modal appears immediately with actionable copy.
- `V-2` Existing state behavior remains: the current run is not replaced, and the history row/session still records `unavailableReason`.
- `V-3` Remote restore path (`restoreRemoteChatSession`) shows the same user-facing modal for the same error family.
- `V-4` Non-provider-account failures continue to use their existing UX path; fix does not turn every history-open failure into the same modal.

## 9. Regression Guard

- tests: add store/UI tests covering modal-trigger state on `account_not_signed_in` and `account_unavailable`.
- alerts: none.
- audit checks: none.

## 10. Follow-Up Document Updates

- upstream docs that must change: none required to capture the bug; if fix lands, relevant desktop history/sync docs may need a note that provider-account mismatch now surfaces explicit UI feedback.
- notes left unchanged on purpose: runner error code/message contract remains correct and should not be weakened just to satisfy UX.
