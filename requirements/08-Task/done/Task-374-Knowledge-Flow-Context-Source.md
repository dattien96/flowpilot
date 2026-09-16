# Task-374: Knowledge Flow Context Source

## Metadata

- Document ID: `Task-374`
- Title: `Knowledge Flow Context Source`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [CP-66 P-2](../../07-Coding-Plan/todo/CP-66-Living-Knowledge-Base-Context-Source.md)
- Child Documents: `None`
- Related Documents: [SD-22](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [SS-09](../../05-System-Specs/SS-09-Artifact-Memory-Context-Retrieval.md), [CP-44](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md), [CP-54](../../07-Coding-Plan/done/CP-54-Locus-Anchored-Context-Relevance.md)
- Replaces: `None`
- Tags: `knowledge-base, context-source, retrieval-locus, registry, token-budget`
- Feature Keys: `living-knowledge-base`

## AI Quick View

### Summary

- Slice 2 của CP-66: hiện thực Context Source `knowledge.flow` theo chuẩn CP-44/SD-22 — implement interface `ContextSource` trong `internal/runner/knowledge_flow_context_source.go`, đăng ký key `knowledge.flow` vào `DefaultContextSourceRegistry()`.
- Fetch theo CP-66 §3.2: từ `RetrievalLocus` (hints.ChangedPaths/ExplicitSourcePaths + UserPrompt) → tra `index.json` của P-1 (file/symbol → flow_id) → trích đúng các section flow liên quan từ markdown → đóng gói `FlowContextSection` ~500 token (cứng cap <1.000).
- Tra cứu 100% Go code thuần đọc file (P-1 key decision — không AI grep, không subprocess tại Fetch): `Deterministic()` trả true, thỏa invariant no-vector SD-22 D-2.
- Opt-in như `conventions`/`lsp.diagnostics`: đăng ký trong registry (để `ValidateFlowContextSources` resolve được) nhưng KHÔNG thêm vào `defaultContextSourceIDs` — flow chưa opt-in giữ output byte-identical (constraint zero-conflict).

### Current Ask

- Implement P-2 theo CP-66 §4: file nguồn mới + wire registry. 4 test signatures cho sẵn phải xanh.

### Key Decisions

- `T-1` Pattern theo `context_source_conventions.go` / `context_source_lsp.go`: struct `knowledgeFlowSource{priority int}`, `ID() = "knowledge.flow"`, `Deterministic() = true`, `Fetch(ctx, FlowContextHints)` đọc `.flowpilot/knowledge/` tại `hints.Workspace`.
- `T-2` Locus → flows: dùng `buildRetrievalLocus(workspace, runID, prompt, explicitPaths)` sẵn có để lấy Paths/Symbols (kèm `hints.ChangedPaths` hợp nhất), rồi tra `index.json` — match path trực tiếp + symbol (kèm suffix-match như `isInBaseline` của oracle cho tên qualified). Locus rỗng → trả section rỗng (không cảnh báo ồn).
- `T-3` Thứ tự ưu tiên: priority ngay sau `conventions` (tri thức kiến trúc nằm trước evidence chi tiết trong packing order của Budget Packer — Task-168 T-4).
- `T-4` Fallback CP-66 §8: thư mục/index không tồn tại hoặc parse lỗi → section rỗng + `Omitted: ["knowledge_base_missing"]`, KHÔNG trả error (error chỉ dành cho trường hợp Collect cần degrade-to-warning; missing là trạng thái bình thường của project chưa index).
- `T-5` Token limit: dùng token counter hiện có của context Budget Packer (không tự chế tokenizer mới); gộp các section match, cắt theo nguyên section (không cắt giữa section) đến khi tổng ≤ ~500 token; một section đơn lẻ >1.000 token (vi phạm contract P-1) được skip + warning thay vì phình slot.

### Constraints

- Additive tests only — không edit pre-existing tests; old test đỏ hoặc không compile → STOP, báo tên test + output, không sửa test, chờ user (R1 safe-fix-contract).
- Provider parity: Case-1 agnostic — `knowledgeFlowSource.Fetch` chỉ đọc file + locus, không nhận `providerKey` (grep verify 0 hit); golden fixtures context package giữ nguyên.
- Prior CA: none (new feature `living-knowledge-base`); đóng task phải kèm CA-NNN ledger entry.
- Fetch không được chạy subprocess (npx/lsp) — chỉ đọc file; mọi thứ nặng thuộc P-1 distiller.
- Không đổi `defaultContextSourceIDs` và output của các source hiện có (golden fixtures của context package phải giữ nguyên).
- GitNexus impact analysis trước khi sửa `DefaultContextSourceRegistry` / builtin wiring.

### Open Questions

- None.

### Source Refs

- CP-66 §3.2 (luồng Fetch), §4 P-2, §8 (fallback), test signatures 4–7.
- `internal/runner/context_source_registry.go` (ContextSource interface, Register contract), `internal/runner/context_sources_builtin.go` (defaultContextSourceIDs + opt-in precedent của conventions/lsp), `internal/runner/retrieval_locus.go` (`buildRetrievalLocus`), `internal/runner/context_source_conventions.go` (reference implementation).

## 1. Goal

`knowledge.flow` resolve được từ registry, Fetch trả đúng section flow nghiệp vụ liên quan locus trong ~500 token, và mọi trạng thái thiếu (chưa index, index lỗi) đều degrade êm về section rỗng — flow không bao giờ gãy vì source này.

## 2. Parent Links

- coding plan: `CP-66-Living-Knowledge-Base-Context-Source.md` P-2
- tech design: `SD-22-Pluggable-Context-Source-Registry.md`, `SD-17-Context-And-Regression-Engine.md`
- system spec: `SS-09-Artifact-Memory-Context-Retrieval.md`

## 3. Trigger

P-1 đã sinh được tri thức + index trên đĩa; cần lớp truy xuất chuẩn CP-44 để Planner/Scout tiêu thụ — đây là mảnh biến Knowledge Base thành ngữ cảnh thật sự trong flow.

## 4. Exact Change

- `T-1` **`internal/runner/knowledge_flow_context_source.go`** (new): `knowledgeFlowSource` implement `ContextSource` theo T-1/T-2; đọc `index.json` + trích section markdown từ vị trí index chỉ ra (hỗ trợ cả layout đơn file và sharded `flows/<domain>.md` của Q-1).
- `T-2` **`internal/runner/context_sources_builtin.go`** (modified): thêm `ContextSourceKnowledgeFlow ContextSourceID = "knowledge.flow"` + đăng ký vào `DefaultContextSourceRegistry()` với priority theo T-3; KHÔNG thêm vào `defaultContextSourceIDs` (comment ghi rõ opt-in, mirror ghi chú conventions/lsp).
- `T-3` **`internal/runner/knowledge_flow_context_source_test.go`** (new): 4 test signatures dưới đây dùng fixture `.flowpilot/knowledge/` trong temp dir (index + 2–3 section mẫu do P-1 format sinh).

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/knowledge_flow_context_source.go` (new)
  - `apps/local-runner/internal/runner/knowledge_flow_context_source_test.go` (new)
  - `apps/local-runner/internal/runner/context_sources_builtin.go` (modified — ID + registration)
- modules: `runner`
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: `TestKnowledgeFlowSourceKeyMatches` — `ID()` là `"knowledge.flow"`; registry Resolve thành công sau đăng ký; duplicate register bị từ chối theo contract.
- [ ] AC-2: `TestKnowledgeFlowSourceResolvesFlowFromLocus` — locus chứa path/symbol của flow "Checkout_Pipeline" → Body chứa đúng section đó (và không chứa section flow khác).
- [ ] AC-3: `TestKnowledgeFlowSourceReturnsEmptyGracefullyWhenMissing` — workspace không có `.flowpilot/knowledge/` → section rỗng, không error, `Omitted` ghi rõ lý do.
- [ ] AC-4: `TestKnowledgeFlowSourceRespectsTokenLimit` — 3 section match → Body ≤ ~500 token, cắt theo nguyên section.
- [ ] AC-5: Golden fixtures của context package không đổi (các flow chưa opt-in byte-identical).

## 7. Out of Scope

- Distiller/writer/bootstrap (P-1 / Task-373).
- Cập nhật vi sai tại node audit (P-3 / Task-375).
- Thêm `knowledge.flow` vào bất kỳ flow YAML nào (P-4 / Task-376 — nguồn chỉ đăng ký, chưa bật).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
