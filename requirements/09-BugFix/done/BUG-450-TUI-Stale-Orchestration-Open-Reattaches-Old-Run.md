---
id: BUG-450
title: TUI can reattach old orchestration stream after provider switch
status: done
version: 1
created: 2026-09-23
updated: 2026-09-23
owner: FlowPilot
linked: [BUG-428, CA-926]
---

## AI Quick View
- **What**: A late old-leg `orchStreamOpenedMsg` can replace/cancel the new-leg stream.
- **Why**: BUG-428 tags poll events and closes, but not stream-opening responses.
- **Key constraint**: Stream adoption must compare run ID/generation before mutating the TUI, including in-flight open commands.

## 1. Metadata
- Document ID: `BUG-450`
- Phase: `bugfix`
- Status: `done`
- Feature Keys: `cli-tui`
- Parent Documents: [BUG-428](../done/BUG-428-TUI-Post-Switch-Seed-Turn-Runs-Invisibly.md), [CA-926](../../../change-audit/CA-926-tui-post-switch-stream-flow-catalog-and-composer-fixes.md)

## 2. Symptom and Impact
`cmdStartOrchestrationStream` captures old `runID` and returns `orchStreamOpenedMsg{EvCh, Cancel}` **without run ID/generation** (`internal/tui/app/turn_stream.go:37-40,89-100`). `applyChatSwitched` cancels the existing stream and switches handle to the new run (`chat_switch.go:317-343`). But `app.go:1664-1667` always accepts any late `orchStreamOpenedMsg`, calls `stopOrchestrationStream` (cancels the new stream if already open) and attaches the old channel. The BUG-428 `msg.st` identity guard applies only to `orchStreamEventMsg`/`orchStreamClosedMsg` (`app.go:1669-1742`), so the old seed turn is invisible again. Severity: **high** for intermittent approval/stop soft-lock.

## 3. Reproduction / Evidence
Deterministic event-loop schedule: issue cmdStart for run-old while orchStream nil; switch to run-new and deliver its OpenedMsg; then deliver run-old OpenedMsg last. `m.orchStream` now points to run-old; run-new's stream was canceled. Existing `bug428_post_switch_stream_test.go` tests stale poll event/close, not stale **open**. **Code-path evidence; no new event-sequencing test run in this review.**

## 4. Acceptance and Verification
Add test that delivers open messages out of order across switch, ensure stale open is canceled and ignored, new run stream remains active; include reconnect/history navigation and provider-parity matrix (shared TUI). No change to old tests.

## 5. Resolution (2026-09-23, CA-933)

- `orchStreamOpenedMsg` carries run/leg identity; stream-open application is
  guarded so only the current run/leg can open/displace stream state — a stale
  open from the previous leg self-drops (`turn_stream.go`, `app.go`).
- Test: `TestBug450_StaleOrchOpenDoesNotDisplaceNewLegStream` — red before
  fix; `go test -count=1 ./internal/tui/app` focused green.
