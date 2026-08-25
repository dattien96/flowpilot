# CA-632: run-142155 — sidebar 9-step truncation + blocked bar without reason

## What

Rag-harness run-142155 (grok, reviewer cohort fail `no connected local account found for provider "codex"`):
1. Right sidebar hiển thị 8/9 bước, ẩn `audit` sau `… +1 more` dù terminal height 50 dư chỗ.
2. Flow park `blocked: escalate` có `[Continue]/[Stop]` nhưng **không có reason** — TUI chỉ thấy `(no detail from runner)` cho step FAILED, trong khi runner đã stamp đầy đủ `loop_gate_reason="Review cannot proceed: the Codex reviewer failed with ..."` (log `flow_parked_awaiting_user`).

## Why

1. `session_panel.go` `flowStepsPanelLines()` hard-cap `limit > 8`. Rag-harness có 9 node → `audit` (acceptance) bị ẩn, còn `… +1 more` ngay cả khi sidebar còn hàng chục dòng trống.
2. Cohort `EventTurnFailed` (`interactive_service.go:5001`) settle node bằng `setFlowStepStatusLocked(FAILED)` — **không** RejectionNote — trong khi non-cohort dùng `setFlowStepFailedWithReasonLocked` (CA-616). Step FAILED lúc đó không có lý do → TUI render `reason: (no detail from runner)`.
3. `renderBlockedBar` chỉ in `flowBlockReason` (`escalate`), không in `LoopState.GateReason`; model không giữ gate.

## Fix

- **Runner** `interactive_service.go` cohort fail branch: `setFlowStepFailedWithReasonLocked(ctx, parentRunID, rs.label, truncateDisplayField(ev.Error, 500))` — thay `setFlowStepStatusLocked` — stamp RejectionNote thật (giống non-cohort CA-616).
- **TUI model** `model.go`: thêm `flowGateReason` (LoopState.GateReason).
- **TUI step_runtime.go**:
  - `applyAgentGraph`: set `flowGateReason`; re-banner khi blocked **và** gate tới muộn (trước đó trống).
  - `renderBlockedBar`: thêm dòng `reason:` từ `blockedDecisionReason()` — GateReason trước, fallback RejectionNote của step FAILED đầu tiên, rỗng thì bỏ dòng (chip vẫn hiện, không bịa reason).
- **TUI session_panel.go**: thêm `flowStepsPanelLinesMax(maxRows)` (height-aware); `flowStepsPanelLines()` = legacy 8-row giữ contract cũ cho overlay/tests. `renderRightSidebar` cấp budget `h - sessionRows - 4` (floor 8) → 9 bước rag-harness hiện hết, `+N more` chỉ khi thật sự tràn.

Will not undo: CA-616/617 non-cohort note + fallback, CA-619 chip/Thinking, BUG-231 park, YOLO Approve/Deny, CA-528 chip truncate, CA-542 header/back.

## Tests (additive, no old edit)

- `runner/run142155_cohort_fail_stamps_reason_test.go` (table grok/codex/claude):
  - `TestRun142155_CohortMemberFailStampsRejectionNote` — cohort reviewer fail → step FAILED + RejectionNote chứa `codex`.
- `tui/app/run142155_sidebar_and_blocked_reason_test.go` (matrix claude/codex/grok):
  - `SidebarShowsAllNineSteps` — 9 bước, height 50 → sidebar chứa `audit`, không `+1 more`.
  - `SidebarOverflowStillShowsMoreTail` — maxRows=5 → `+4 more`; legacy `flowStepsPanelLines()` vẫn cap 8 (lock cũ).
  - `BlockedBarShowsGateReason` — blocked+escalate+gate codex → View có `[Continue]` `[Stop]` + `codex`.
  - `BlockedBarFallsBackToRejectionNote` — gate rỗng + FAILED note → bar có note.
  - `BlockedBarNoReasonOmitsLine` — không gate + không note → chip vẫn có, không bịa `reason:`.
  - `LateGateReasonRebanners` — blocked trước (gate rỗng) → snapshot sau có gate → banner mới + bar hiện gate.
- Verify: `go vet ./internal/tui/app ./internal/runner` clean; `go test ./internal/tui/app -count=1` xanh 15.2s; `go test ./internal/runner -run TestRun142155|TestRun135037|TestCohort|TestFlowStepRuntime` xanh. Full runner suite có 7 fail **pre-existing** (đã xác nhận bằng `git stash` baseline: TestSupabaseCatalogStoreShaping, TestProviderRegistryForUsesLiveWhenFlagOn, TestRunCompatCheckIncludesPortabilityCanaries, TestValidatePassedSpawnsReviewerCohortMember, TestValidatePassedStillChainsInlineAuditForLegacyEdges, TestRun75035_SeedChildKeepsOwnMultipleCodexRollouts, TestRootFlowEngineDefersCompletedUntilGate — env supabase/live-adapter/timing, fail giống hệt ở HEAD sạch).

## Provider parity

Agnostic: runner branch không đọc `providerKey` (table 3 provider trong test mới); TUI render không nhánh provider (matrix claude/codex/grok).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CA-632
change_type: bugfix
summary: sidebar hiện đủ 9 bước rag-harness (height-aware) và blocked bar có reason từ GateReason/RejectionNote; cohort fail stamp RejectionNote thật (run-142155)
# --->8---