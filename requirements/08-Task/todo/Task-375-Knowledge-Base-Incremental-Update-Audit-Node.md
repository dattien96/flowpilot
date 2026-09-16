# Task-375: Knowledge Base Incremental Update At Audit Node

## Metadata

- Document ID: `Task-375`
- Title: `Knowledge Base Incremental Update At Audit Node`
- Phase: `task`
- Status: `draft`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [CP-66 P-3](../../07-Coding-Plan/todo/CP-66-Living-Knowledge-Base-Context-Source.md)
- Child Documents: `None`
- Related Documents: [SS-09](../../05-System-Specs/SS-09-Artifact-Memory-Context-Retrieval.md), [SD-17](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [CP-62](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [CP-55](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md)
- Replaces: `None`
- Tags: `knowledge-base, incremental-update, audit-node, background-worker, non-blocking`
- Feature Keys: `living-knowledge-base`

## AI Quick View

### Summary

- Slice 3 của CP-66: kích hoạt cập nhật vi sai Knowledge Base tại node `audit` — sau khi gate cho phép và audit hoàn tất, worker nền gọi `IncrementalUpdate(changedPaths)` của P-1 cho đúng các file trong `ChangedPaths` của turn.
- P-3 key decision: update CHỈ xảy ra ở node audit (code đã duyệt xong) — không bao giờ update giữa chừng khi code đang dở dang ở implement/validate; tri thức luôn phản ánh trạng thái đã được review.
- Non-blocking tuyệt đối (CP-66 Review Protocol #2): worker là goroutine fire-and-forget, mọi lỗi chỉ log — không bao giờ làm fail hay delay flow (test `TestAuditNodeNonBlockingOnKnowledgeError` chốt hành vi này).
- Guard: chỉ update khi `.flowpilot/knowledge/` đã tồn tại (đã bootstrap bởi P-1); project chưa có tri thức thì audit bỏ qua êm (fallback §8) — không tự bật full-rebuild giữa flow.

### Current Ask

- Implement P-3 theo CP-66 §4: hook tại audit dispatch + worker nền. 2 test signatures cho sẵn phải xanh.

### Key Decisions

- `T-1` Hook đặt tại audit node completion trong `flow_validate_audit_dispatch.go` (nơi audit node settle sau gate) — not `interactive_service.go` broadly; chỉ fire đúng khi node hiện tại là `audit` (behavior `artifact.audit_draft`) và turn có `ChangedPaths`/`WrittenPaths` không trống.
- `T-2` Worker nền: `go knowledge.UpdateAsync(workspace, changedPaths)` — serialize bằng per-workspace mutex/queue đơn giản (2 audit liên tiếp không ghi đè lẫn nhau); reuse guard map pattern của `ensureGitNexusIndexAsync`.
- `T-3` Source of paths: union `TurnResult.ChangedPaths` + code files trong `WrittenPaths` của turn audit (lọc `IsTestFile`/doc noise theo chuẩn locus — dùng `isConcreteCodeTarget`); rỗng → skip hoàn toàn (không spawn worker).
- `T-4` Lỗi bất kỳ (parse index, disk, distill) → log `[knowledge]` + trả về; KHÔNG propagate lên flow state, KHÔNG retry tự động trong CP này.

### Constraints

- Additive tests only — không edit pre-existing tests; old test đỏ hoặc không compile → STOP, báo tên test + output, không sửa test, chờ user (R1 safe-fix-contract).
- Provider parity: Case-1 agnostic — audit hook + `UpdateAsync` không nhận `providerKey` (grep verify 0 hit); verdict flow giữ nguyên mọi trường hợp.
- Prior CA: none (new feature `living-knowledge-base`); đóng task phải kèm CA-NNN ledger entry.
- Không thay đổi trạng thái node/kết quả flow vì knowledge update — audit node verdict giữ nguyên mọi trường hợp.
- Không update tại bất kỳ node nào khác (implement/validate/reviewer) — constraint kiến trúc của P-3.
- GitNexus impact analysis trước symbol edit trong `flow_validate_audit_dispatch.go` (file thuộc luồng dispatch lõi).

### Open Questions

- None.

### Source Refs

- CP-66 §4 P-3, §8 (fallback), Review Protocol #2, test signatures 8–9.
- `internal/runner/flow_validate_audit_dispatch.go` (audit dispatch), `internal/knowledge/writer.go` (`IncrementalUpdate` từ Task-373), `internal/runner/gitnexus_autoindex.go` (background worker precedent).

## 1. Goal

Sau mỗi task hoàn tất (audit node chạy xong), Knowledge Base tự phản ánh code mới trong vài giây dưới nền mà không ai biết đến — không block, không fail, không đổi verdict flow.

## 2. Parent Links

- coding plan: `CP-66-Living-Knowledge-Base-Context-Source.md` P-3
- tech design: `SD-17-Context-And-Regression-Engine.md`
- system spec: `SS-09-Artifact-Memory-Context-Retrieval.md`

## 3. Trigger

P-1 có writer incremental nhưng chưa có ai gọi theo lifecycle; không có update vi sai thì tri thức stale ngay sau task đầu tiên và `knowledge.flow` (P-2) sẽ trả ngữ cảnh lỗi thời — phá đúng giá trị "Living" cốt lõi của CP.

## 4. Exact Change

- `T-1` **`internal/runner/flow_validate_audit_dispatch.go`** (modified): tại completion path của node audit — sau gate pass, thu nhận code paths theo T-3, gọi worker nền theo T-2 (điều kiện: `.flowpilot/knowledge/` tồn tại).
- `T-2` **`internal/runner/knowledge_bootstrap.go`** (modified, từ Task-373): thêm `UpdateAsync(workspace string, changedPaths []string)` — serialize per-workspace qua `knowledgeUpdater{mu sync.Mutex}` (2 audit liên tiếp không ghi đè), error chỉ log `[knowledge]`.
- `T-3` **`internal/runner/knowledge_audit_hook_test.go`** (new): 2 test signatures dưới đây (dùng temp workspace có fixture knowledge của P-1; double cho dispatch path).

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go` (modified)
  - `apps/local-runner/internal/runner/knowledge_bootstrap.go` (modified — UpdateAsync)
  - `apps/local-runner/internal/runner/knowledge_audit_hook_test.go` (new)
- modules: `runner`, `knowledge`
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: `TestAuditNodeUpdatesKnowledgeBaseIncrementally` — turn audit với ChangedPaths trỏ file thuộc flow X → section X của `execution-flows.md` được viết lại (nội dung mới), section khác byte-identical, `index.json` đồng bộ.
- [ ] AC-2: `TestAuditNodeNonBlockingOnKnowledgeError` — knowledge dir corrupted (index malformed) → audit node vẫn settle done bình thường, error chỉ xuất hiện trong log, flow không fail.
- [ ] AC-3: Node khác audit (implement/validate) hoàn tất → KHÔNG có update knowledge (chỉ audit trigger).
- [ ] AC-4: Workspace chưa có `.flowpilot/knowledge/` → no-op hoàn toàn (không bootstrap giữa flow, không error).

## 7. Out of Scope

- Distiller/writer/bootstrap (P-1 / Task-373).
- Context source (P-2 / Task-374).
- Flow profiles + E2E (P-4 / Task-376).
- Retry/cron refresh Knowledge Base định kỳ (follow-up nếu có nhu cầu).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
