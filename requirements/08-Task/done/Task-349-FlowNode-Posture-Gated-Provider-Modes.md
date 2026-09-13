# Task-349: Flow-Node Posture Ép Gated Provider Modes (fix Task-340 P0)

## Metadata

- Document ID: `Task-349`
- Title: `Flow-Node Posture Ép Gated Provider Modes (fix Task-340 P0)`
- Feature Keys: `zcode-parity, node-isolation`
- Phase: `task`
- Status: `done`
- Created: `2026-09-14`
- Last Updated: `2026-09-14`
- Parent Documents: [CP-62](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [Task-340](./Task-340-Per-Node-Read-Only-Enforcement-And-Isolation.md)
- Related Documents: [CA-861](../../../change-audit/CA-861-FlowNode-Posture-Gated-Modes.md), [BUG-299](../../09-BugFix/done/BUG-299-Flow-YOLO-Override.md)
- Tags: `zcode-parity, node-isolation, yolo, posture, cp-62`

## AI Quick View

### Summary
- Review CP-62 phát hiện **P0**: flow runs bị khóa YOLO=true (BUG-299), ForceShellBridge loại trừ reviewer children → không approval request nào tới bridge → `decideFlowNodePosture` không bao giờ được hỏi cho node `read_only`/`verdict_only` — enforcement chỉ tồn tại trên giấy.

### Current Ask
- Fix P0 + P1 (verdict tool exact-match) + parity test.

### Key Decisions
- `D-1` Mở rộng YOLO SSOT: `resolveYoloPostureForFlowNode(yolo, forceShellBridge, chatPosture, flowNodePosture)` — posture node gated (read_only/verdict_only) ép cùng bộ mode gated như chat posture read-only (Claude `default`, Codex `untrusted`/`workspace-write`, Grok `""`, Opencode `""`) **và** `RunnerAutoApprove=false` — mọi tool call phải qua bridge, matrix quyết định.
- `D-2` `TurnRequest.FlowNodePosture` mới; site build duy nhất (interactive_service) set bằng `flowNodePostureFor(rs)`. Provider-agnostic: cả 3 adapter resolve qua SSOT — parity by construction (thay cho parity test fakes CP-62 yêu cầu).
- `D-3` P1: `isVerdictToolCall` nhận cả tên MCP wrap (`mcp__flowpilot__*`, `flowpilot__*` — suffix `__submit_review_outcome`); `readOnlyApprovalDecision` cho phép verdict face trên kind mcp/other (tool runner-hosted, nội dung enforce ở SubmitFlowControl).

## 6. Acceptance Check
- [x] AC-1: posture child chạy gated modes + RunnerAutoApprove=false (test pin).
- [x] AC-2: standard posture giữ nguyên YOLO bypass (test pin).
- [x] AC-3: verdict face wrap-name được nhận diện trên cả read_only và verdict_only.
- [x] AC-4: parity — 3 adapter dùng chung 1 SSOT resolver (một bảng test phủ claude/codex/grok/opencode outputs).

## 8. Completion Notes
- Đã ship + test (`flow_node_gating_test.go` 5 test). Suite liên quan `-race` xanh. Chi tiết CA-861.
