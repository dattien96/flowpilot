# Task-373: Knowledge Distiller Engine

## Metadata

- Document ID: `Task-373`
- Title: `Knowledge Distiller Engine`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [CP-66 P-1](../../07-Coding-Plan/done/CP-66-Living-Knowledge-Base-Context-Source.md)
- Child Documents: `None`
- Related Documents: [SS-09](../../05-System-Specs/SS-09-Artifact-Memory-Context-Retrieval.md), [SD-17](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SD-22](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [CP-63](../../07-Coding-Plan/todo/CP-63-IDE-Grade-LSP-Runtime.md)
- Replaces: `None`
- Tags: `knowledge-base, distiller, gitnexus, lsp, markdown, incremental-update`
- Feature Keys: `living-knowledge-base`

## AI Quick View

### Summary

- Slice đầu tiên của CP-66: package mới `internal/knowledge` — bộ chưng cất (Distiller) biến dữ liệu thô GitNexus (vĩ mô: execution flows) + LSP symbols (vi mô: data models) thành 3 file Markdown tinh gọn tại `.flowpilot/knowledge/`: `system-overview.md`, `execution-flows.md`, `data-models.md`.
- Định dạng chuẩn hóa theo CP-66 P-2 key decision: Markdown có mục lục cố định, mỗi block flow <1.000 token; mỗi flow là MỘT section `## Flow: <flow_id>` tách biệt để P-2 trích đúng 1 section.
- Distiller sinh kèm `index.json` (file/symbol → flow_id → section + ước lượng token) — tra cứu lúc Fetch của P-2 chỉ là đọc file thuần, không spawn subprocess.
- Vòng đời (CP-66 §AI Quick View): khởi tạo lần đầu khi bind project (background worker, tiền lệ `ensureGitNexusIndexAsync`), cập nhật vi sai chỉ tại node `audit` (P-3), không bao giờ giữa chừng code dở.

### Current Ask

- Implement P-1 theo CP-66 §4: `distiller.go` + `writer.go` trong package `internal/knowledge` + mở rộng surface đọc dữ liệu GitNexus trong `internal/structure`. 3 test signatures cho sẵn phải xanh.

### Key Decisions

- `T-1` **`internal/structure/gitnexus.go`** (modified, additive): đọc `gitnexus.go` hiện có trước (spike shape CLI JSON của `Dependents`), rồi thêm hàm export processes/flows (VD `Processes(ctx) ([]FlowSummary, error)`) parse cùng shape — không gọi MCP, không thêm dependency mới. LSP symbols qua `lsp.ServerSet` sẵn có (CP-63); LSP vắng mặt → degrade gracefully, distill chỉ từ GitNexus.
- `T-2` Cấu trúc file cố định (P-2 key decision): `system-overview.md` (tech stack, layer boundaries, core libraries), `execution-flows.md` (mỗi flow một section `## Flow: <id>` gồm: mục đích, các bước, symbols chính, files liên quan), `data-models.md` (entity + state schema). Mỗi section <1.000 token — vượt thì chưng cất lại ngắn hơn, không cắt cụt.
- `T-3` `index.json` là hợp đồng P-1↔P-2: map path/symbol → flow_id → vị trí section (file + heading) + token estimate; được ghi atomically cùng lúc với markdown (write-then-rename) để không bao giờ có index trỏ section không tồn tại.
- `T-4` Incremental update (writer.go): nhận `ChangedPaths`, qua index map ngược ra các flow bị ảnh hưởng, chỉ re-distill các flow đó (merge lại vào file giữ section khác nguyên vẹn). Full rebuild chỉ khi index mất hoặc version schema đổi.
- `T-5` Q-1 CHỐT 500 flows: khi số flow > 500, `execution-flows.md` tách thành `.flowpilot/knowledge/flows/<domain>.md` + file manifest liệt kê shard; index.json vẫn là mặt phẳng tra cứu duy nhất (P-2 không cần biết sharding).
- `T-6` Khởi tạo theo data backfill CP-66 §6: background goroutine khi bind project (once-per-process guard như `ensureGitNexusIndexAsync`), KHÔNG block runner startup (constraint <1s; R-1: quét lớn chạy ngầm).

### Constraints

- Additive tests only — không edit pre-existing tests; old test đỏ hoặc không compile → STOP, báo tên test + output, không sửa test, chờ user (R1 safe-fix-contract).
- Provider parity: Case-1 agnostic — distiller/writer/bootstrap chỉ đọc GitNexus/LSP/file, không nhận `providerKey` (grep verify 0 hit).
- Prior CA: none (new feature `living-knowledge-base`); đóng task phải kèm CA-NNN ledger entry.
- Background worker không bao giờ block UI/runner; lỗi distill chỉ log, không fail flow nào.
- Package `internal/knowledge` không import `runner` (tránh cycle; runner import knowledge là chiều duy nhất cho phép).
- GitNexus impact analysis trước symbol edit trong `internal/structure/gitnexus.go`.

### Open Questions

- None (Q-1 đã chốt 500 flows ở T-5).

### Source Refs

- CP-66 §3.1 (cấu trúc thư mục), §4 P-1, §6 (data backfill), §8 (fallback), §9 (R-1), Q-1, test signatures 1–3.
- `internal/structure/gitnexus.go` (CLI JSON parsing precedent), `internal/lsp/server_manager.go` + `runner_hook.go` (ServerSet), `internal/runner/gitnexus_autoindex.go` (background worker precedent).

## 1. Goal

Chạy distiller trên một repo mẫu sinh ra đúng 3 file markdown chuẩn cấu trúc + `index.json` nhất quán; cập nhật vi sai chỉ đổi các section liên quan đến file thay đổi; toàn bộ chạy nền không block.

## 2. Parent Links

- coding plan: `CP-66-Living-Knowledge-Base-Context-Source.md` P-1
- tech design: `SD-17-Context-And-Regression-Engine.md`, `SD-22-Pluggable-Context-Source-Registry.md`
- system spec: `SS-09-Artifact-Memory-Context-Retrieval.md`

## 3. Trigger

Agent tốn 40–60k token các turn đầu đọc lướt mã thô để hiểu kiến trúc (CP-66 pain point). P-1 là hạt nhân sinh tri thức — P-2/P-3/P-4 đều tiêu thụ sản phẩm của distiller.

## 4. Exact Change

- `T-1` **`internal/structure/gitnexus.go`** (modified, additive): hàm export processes/flows (VD `Processes(ctx) ([]FlowSummary, error)`) parse JSON CLI output — shape dữ liệu theo GitNexus Execution Flow schema mà CP-66 trích dẫn.
- `T-2` **`internal/knowledge/distiller.go`** (new): `Distiller` nhận structure provider + LSP ServerSet (tùy chọn) → sinh `KnowledgeBase` in-memory (3 tài liệu + index); section theo T-2 decisions, mỗi block <1.000 token.
- `T-3` **`internal/knowledge/writer.go`** (new): `WriteFull(repoDir, kb)` (atomic write-then-rename 4 file: 3 md + `index.json` có `schemaVersion` cho full-rebuild khi đổi schema) + `IncrementalUpdate(repoDir, changedPaths)` (map ngược từ index, re-distill flow affected, merge section); `Missing(repoDir) bool` helper cho P-2/P-3 kiểm tra tồn tại.
- `T-4` **`internal/runner/knowledge_bootstrap.go`** (new): `ensureKnowledgeBaseAsync(workspace)` — background goroutine once-per-process khởi tạo lần đầu khi bind project (guard map theo workspace, tiền lệ `ensureGitNexusIndexAsync`); gọi từ chỗ bind run hiện có.
- `T-5` **`internal/knowledge/distiller_test.go`** (new): 3 test signatures dưới đây trên fixture repo + fake structure/LSP doubles.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/knowledge/distiller.go` (new)
  - `apps/local-runner/internal/knowledge/writer.go` (new)
  - `apps/local-runner/internal/knowledge/distiller_test.go` (new)
  - `apps/local-runner/internal/structure/gitnexus.go` (modified — additive export API)
  - `apps/local-runner/internal/runner/knowledge_bootstrap.go` (new — background bootstrap)
- modules: `knowledge` (new), `structure`, `runner`
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: `TestDistillerGeneratesSystemOverview` — sinh `system-overview.md` với mục lục cố định (tech stack, layer boundaries, core libraries), nội dung bám đồ thị GitNexus thật của fixture.
- [ ] AC-2: `TestDistillerGeneratesExecutionFlows` — sinh `execution-flows.md` với ≥1 section `## Flow: <id>` đầy đủ (mục đích, bước, symbols, files); mỗi section <1.000 token; `index.json` map path/symbol → đúng section.
- [ ] AC-3: `TestDistillerIncrementalUpdateOnChangedFiles` — đổi 1 file thuộc flow X → chỉ section X được viết lại, section khác byte-identical; index cập nhật đồng bộ.
- [ ] AC-4: LSP vắng mặt (ServerSet rỗng) → distill vẫn thành công từ GitNexus, không panic.
- [ ] AC-5: Bootstrap chạy nền: gọi `ensureKnowledgeBaseAsync` 2 lần cùng workspace → distill chỉ chạy 1 lần; runner startup không bị block (goroutine).

## 7. Out of Scope

- Context Source `knowledge.flow` (P-2 / Task-374).
- Cập nhật vi sai tại node audit (P-3 / Task-375).
- Flow profiles + E2E (P-4 / Task-376).
- Sinh tri thức cho repo không có GitNexus index (fallback của source ở P-2 là trả rỗng).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
