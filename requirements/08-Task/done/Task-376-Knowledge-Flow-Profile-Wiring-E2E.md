# Task-376: Knowledge Flow Profile Wiring And E2E Validation

## Metadata

- Document ID: `Task-376`
- Title: `Knowledge Flow Profile Wiring And E2E Validation`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [CP-66 P-4](../../07-Coding-Plan/done/CP-66-Living-Knowledge-Base-Context-Source.md)
- Child Documents: `None`
- Related Documents: [SS-19](../../05-System-Specs/SS-19-Engineering-Harness-Flow-Family.md), [SD-22](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [CP-62](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [CP-66](../../07-Coding-Plan/done/CP-66-Living-Knowledge-Base-Context-Source.md)
- Replaces: `None`
- Tags: `knowledge-base, context-profiles, flow-wiring, e2e, token-economy`
- Feature Keys: `living-knowledge-base`

## AI Quick View

### Summary

- Slice cuối của CP-66: bật `knowledge.flow` trong `contextProfiles` của `task-harness.yaml` và `bug-plan-harness.yaml` — thêm vào `candidateSources` của các profile lập kế hoạch/điều tra (`scout`, `plan_writer`, và profile điều tra bug nếu flow khai báo), KHÔNG thêm cho `coder` (node code đọc mã thật, tri thức kiến trúc chỉ dành cho planner — CP-66 lifecycle).
- E2E chứng minh DOD "Planner nhận được tóm tắt luồng mà không phải đọc 50 file code thô": prompt của plan_writer chứa section flow đúng, prompt của coder KHÔNG chứa.
- Test guard token-economy: coder node không nhận knowledge.flow — chống context dilution tái diễn đúng nơi nó từng xảy ra.
- Sequencing: bắt buộc sau P-2 (registry đã resolve `knowledge.flow`, nếu không `ValidateFlowContextSources` fail ngay lúc load flow).

### Current Ask

- Implement P-4 theo CP-66 §4: sửa 2 flow YAML profiles + test E2E hội tụ P-1→P-3. 2 test signatures cho sẵn phải xanh; toàn bộ DOD CP-66 tick được.

### Key Decisions

- `T-1` Chỉ thêm `knowledge.flow` vào profile `scout` + `plan_writer` của 2 harness (đã verify không có profile investigate riêng); `reviewer`/`coder` giữ nguyên candidateSources — boundary rõ ràng: tri thức vĩ mô cho người lập kế hoạch, mã thật cho người viết code.
- `T-2` Không tăng `maxTokens` của profile: knowledge.flow cap ~500 token, nằm gọn trong budget hiện có (scout 6.000 / plan_writer 16.000) — Budget Packer (CP-23) tự phân phối.
- `T-3` E2E theo pattern test profile hiện có (Task-341): build context package cho node với fixture `.flowpilot/knowledge/` đầy đủ, assert section `knowledge.flow` render vào prompt delegate với Body đúng section flow; với coder — assert section vắng mặt.
- `T-4` Manual check CP-66 §7: ghi chú kết quả kiểm tra file markdown sinh trong `.flowpilot/knowledge/` của dự án mẫu vào completion notes (bằng chứng DOD mục 1).

### Constraints

- Additive tests only — không edit pre-existing tests; old test đỏ hoặc không compile → STOP, báo tên test + output, không sửa test, chờ user (R1 safe-fix-contract).
- Provider parity: Case-1 agnostic — profile wiring là data YAML + context render, không nhánh `providerKey` (grep verify 0 hit); coder không nhận knowledge.flow để chống dilution.
- Prior CA: none (new feature `living-knowledge-base`); đóng task phải kèm CA-NNN + tick toàn bộ DOD CP-66 §10 + manual check §7 có bằng chứng.
- 2 flow YAML khác (rag/bug-harness/vibe/cp-harness...) KHÔNG được đổi trong diff — chỉ đúng 2 file CP-66 chỉ định.
- Node-level `ContextSources` (Task-196) không đụng — chỉ sửa profile-level candidateSources.
- GitNexus impact analysis trước khi edit YAML load path (pack parser không đổi — chỉ data).

### Open Questions

- None.

### Source Refs

- CP-66 §4 P-4, §7 (manual checks), §10 Definition of Done, test signatures 10–11.
- `internal/agentpack/flow-pack/flows/task-harness.yaml` + `flows/bug-plan-harness.yaml` (contextProfiles hiện tại: scout/plan_writer/reviewer/coder), `internal/runner/context_sources_builtin.go` (`ValidateFlowContextSources` fail-fast), task341 profile tests (pattern).

## 1. Goal

Chạy task-harness trên dự án đã có Knowledge Base: Scout/Planner mở prompt ra thấy tóm tắt luồng nghiệp vụ liên quan (~500 token) thay vì phải đọc 40–60k token mã thô; Coder không thấy tri thức này; các flow không opt-in không đổi gì.

## 2. Parent Links

- coding plan: `CP-66-Living-Knowledge-Base-Context-Source.md` P-4
- tech design: `SD-22-Pluggable-Context-Source-Registry.md`
- system spec: `SS-19-Engineering-Harness-Flow-Family.md`, `SS-09-Artifact-Memory-Context-Retrieval.md`

## 3. Trigger

P-1→P-3 hoàn tất: tri thức sinh được, truy xuất được, tự cập nhật được — nhưng chưa node nào tiêu thụ. P-4 đóng vòng giá trị cuối: Planner thực sự nhận tri thức và DOD CP-66 chốt được.

## 4. Exact Change

- `T-1` **`internal/agentpack/flow-pack/flows/task-harness.yaml`** (modified): `contextProfiles.scout.candidateSources` → `[conventions, knowledge.flow, canonical.head, feature.history]` và `contextProfiles.plan_writer.candidateSources` → `[conventions, knowledge.flow, canonical.head, feature.history, change.contract, source.excerpt]` (knowledge.flow đứng sau `conventions` theo priority T-3 Task-374; `reviewer`/`coder` giữ nguyên).
- `T-2` **`internal/agentpack/flow-pack/flows/bug-plan-harness.yaml`** (modified): tương tự T-1 cho `scout`/`plan_writer` (file này không có profile investigate riêng — đã verify 4 profiles scout/plan_writer/reviewer/coder).
- `T-3` **`internal/runner/knowledge_flow_profile_test.go`** (new): 2 test signatures dưới đây + 1 case regression: flow không opt-in (rag-harness) render không có section knowledge.flow.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/agentpack/flow-pack/flows/task-harness.yaml` (modified)
  - `apps/local-runner/internal/agentpack/flow-pack/flows/bug-plan-harness.yaml` (modified)
  - `apps/local-runner/internal/runner/knowledge_flow_profile_test.go` (new)
- modules: `agentpack` (data only), `runner` (test only)
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: Pack load + `ValidateFlowContextSources` pass cho cả 2 flow sau khi thêm source.
- [ ] AC-2: `TestFlowPlanWriterReceivesExecutionFlowContext` — prompt/rendered package của node plan_writer chứa section `knowledge.flow` với Body đúng section flow theo locus; kích thước ≤ ~500 token.
- [ ] AC-3: `TestCoderNodeDoesNotReceiveKnowledgeFlow` — node coder (profile không opt-in) không có section `knowledge.flow` trong package.
- [ ] AC-4: Flow không khai báo (rag-harness) — output byte-identical trước/sau thay đổi.
- [ ] AC-5: Manual check §7 hoàn tất + bằng chứng ghi trong completion notes; toàn bộ DOD CP-66 tick được (3 file markdown chuẩn xác, locus resolve đúng, audit tự update, planner nhận tóm tắt, additive tests green).

## 7. Out of Scope

- Distiller/writer/source/audit-hook (P-1→P-3 / Task-373–375).
- Bật `knowledge.flow` cho các flow khác (rag/bug-harness/vibe/cp-harness) — follow-up sau khi mesure token ROI.
- Đổi maxTokens hay thành phần khác của profiles.

## 8. Completion Notes

- result: scout + plan_writer of task-harness.yaml and bug-plan-harness.yaml
  opt into `knowledge.flow` (position 2, after conventions); reviewer/coder
  untouched. 5/5 new E2E tests green (profile declares, pack validates,
  plan_writer renders the distilled Checkout section ~500 tokens, coder
  clean, rag-harness byte-identical behavior). Manual §7: distilled
  flowpilot itself — `.flowpilot/knowledge/` with system-overview.md (574 B),
  execution-flows.md (10 sections, 11.5 KB), data-models.md (relevance-ranked,
  e.g. `Struct Catalog` in 3 flows), index.json (16 KB; flow entries resolve
  to existing files).
- follow-ups: (1) measure token ROI before opting in rag/bug-harness/vibe
  flows; (2) production LSP SymbolLister adapter (v1 distills GitNexus-only);
  (3) observe >500-flow sharding on a monorepo (unit-covered, never live).
- upstream docs updated: task-harness.yaml + bug-plan-harness.yaml
  contextProfiles (scout/plan_writer); CP-66 DOD §10 fully ticked at
  closeout.
