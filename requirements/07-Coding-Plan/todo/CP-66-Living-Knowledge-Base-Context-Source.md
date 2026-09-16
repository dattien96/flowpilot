# CP-66: Living Knowledge Base & `knowledge.flow` Context Source

## Metadata

- Document ID: `CP-66`
- Title: `Living Knowledge Base and Execution Flow Context Source`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot Architecture`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [SS-09: Artifact Memory Context Retrieval](../../05-System-Specs/SS-09-Artifact-Memory-Context-Retrieval.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md)
- Child Documents: [Task-373](../../08-Task/todo/Task-373-Knowledge-Distiller-Engine.md) (P-1), [Task-374](../../08-Task/todo/Task-374-Knowledge-Flow-Context-Source.md) (P-2), [Task-375](../../08-Task/todo/Task-375-Knowledge-Base-Incremental-Update-Audit-Node.md) (P-3), [Task-376](../../08-Task/todo/Task-376-Knowledge-Flow-Profile-Wiring-E2E.md) (P-4)
- Related Documents: [CP-44: Pluggable Context Source Registry](../done/CP-44-Pluggable-Context-Source-Registry.md), [CP-54: Locus-Anchored Context Relevance](../done/CP-54-Locus-Anchored-Context-Relevance.md), [CP-62: Zcode Harness Parity](../done/CP-62-Zcode-Harness-Parity.md), [CP-63: IDE-Grade LSP Runtime](CP-63-IDE-Grade-LSP-Runtime.md)
- Replaces: `None`
- Tags: `knowledge-base, execution-flows, gitnexus, context-source, deepwiki, prompt-compression`
- Feature Keys: `living-knowledge-base`
- Implementation Owner: `Claude Sonnet MAX`

---

## AI Quick View

### Summary

- Trong các codebase lớn, Agent thường tốn từ 40.000 đến 60.000 token ở các turn đầu chỉ để đọc lướt mã nguồn thô nhằm hiểu luồng kiến trúc, dẫn đến context dilution và ảo giác.
- Học tập Devin "DeepWiki": Chưng cất đồ thị thực thi của GitNexus (vĩ mô) và bảng symbol của LSP (vi mô) thành bộ tài liệu tinh gọn lưu tại `.flowpilot/knowledge/` (`system-overview.md`, `execution-flows.md`, `data-models.md`).
- Hiện thực hóa Context Source mới `knowledge.flow` theo chuẩn CP-44 / SD-22. Runner Go code chịu trách nhiệm tra cứu file $\rightarrow$ flow và nạp đúng 1 trang tóm tắt vào context slot của Planner/Scout.
- Lifecycle: Khởi tạo khi bind project, cập nhật vi sai tại node `audit`, AI chỉ đọc ở các node lập kế hoạch / điều tra bug.

### Current Ask

- Triển khai 4 slice công việc (P-1 đến P-4) xây dựng bộ sinh Knowledge Base, Context Source `knowledge.flow`, tích hợp vào node `audit` và cập nhật các flow profiles.

### Key Decisions

- `P-1` Tra cứu luồng (Flow Lookup) hoàn toàn do code Go thực hiện dựa trên `RetrievalLocus`, không để AI tự grep tìm file.
- `P-2` Định dạng tài liệu kiến trúc được chuẩn hóa thành Markdown có cấu trúc mục lục cố định để nén token tối đa (<1.000 token mỗi block).
- `P-3` Node `audit` chịu trách nhiệm cập nhật vi sai (Incremental Update) sau mỗi lần code được duyệt merge, không bao giờ update giữa chừng khi code đang dở dang.

### Constraints

- Không làm tăng độ trễ khởi động runner quá 1 giây (quét GitNexus và LSP chạy ngầm trong background worker).
- Zero conflict với các context source hiện có (`conventions`, `source.dependence`, `canonical.head`).
- Additive tests only — không sửa test cũ.
- GitNexus impact analysis trước mỗi symbol edit.

### Open Questions

- `Q-1` — CHỐT (Task-373): >500 flows thì shard `execution-flows.md` thành `.flowpilot/knowledge/flows/<domain>.md` + manifest; `index.json` vẫn là mặt phẳng tra cứu duy nhất.

### Source Refs

- SS-09; SD-17; SD-22; CP-44; CP-54; CP-62 P-5 (Task-341 context profiles).
- Devin DeepWiki architectural pattern.
- GitNexus Execution Flow schema (`apps/local-runner/internal/structure/gitnexus.go`).

---

## 1. Goal

Chấm dứt tình trạng Agent "mù kiến trúc" và lãng phí token bằng việc tự động xây dựng và duy trì Bộ Tri Thức Sống (Living Knowledge Base) từ GitNexus và LSP, cung cấp nguồn ngữ cảnh `knowledge.flow` chuẩn xác cho các giai đoạn lập kế hoạch và sửa bug.

---

## 2. Input Documents

### 2.1 Governing documents
- SS-09: Artifact Memory & Context Retrieval.
- SD-17: Context and Regression Engine.
- SD-22: Pluggable Context Source Registry.

### 2.2 Implemented foundations
- CP-44: Pluggable Context Source Registry.
- CP-54: Locus-Anchored Context Relevance.
- CP-62: Per-Node Context Profile (Task-341) & Conventions Source (Task-343).
- CP-63: IDE-Grade LSP Runtime.

### 2.3 Current code locations
- `apps/local-runner/internal/runner/retrieval_locus.go`: Xây dựng điểm neo locus từ prompt/diff.
- `apps/local-runner/internal/structure/gitnexus.go`: Tương tác với đồ thị GitNexus.
- `apps/local-runner/internal/runner/conventions_context_source.go`: Reference implementation của một Context Source chuẩn CP-44.

---

## 3. Implementation Strategy

### 3.1 Cấu trúc thư mục Tri Thức Sống
```text
.flowpilot/knowledge/
├── system-overview.md       (Tech stack, layer boundaries, core libraries)
├── execution-flows.md        (Top business execution flows mapped to symbols)
└── data-models.md            (Core entity definitions and state schemas)
```

### 3.2 Luồng xử lý của `knowledge.flow` Context Source
```text
Input Request (có RetrievalLocus từ prompt/diff)
  │
  ▼
KnowledgeFlowSource.Produce()
  ├── 1. Lấy target files từ locus: []string{"PaymentService.go"}
  ├── 2. Tra cứu đồ thị GitNexus: file -> flow_id ("Checkout_Pipeline")
  ├── 3. Đọc section "Checkout_Pipeline" từ knowledge/execution-flows.md
  └── 4. Đóng gói thành ContextPayload (slot: "knowledge_flow", ~500 tokens)
```

---

## 4. Work Breakdown

### P-1: Knowledge Distiller Engine (GitNexus + LSP $\rightarrow$ Markdown)

**Status: draft**

**Production changes**
- `internal/knowledge/distiller.go` (**new**): Chuyển đổi dữ liệu thô từ GitNexus CLI/MCP và LSP symbols thành các bản tóm tắt Markdown có cấu trúc.
- `internal/knowledge/writer.go` (**new**): Ghi và cập nhật vi sai vào `.flowpilot/knowledge/` (atomic write-then-rename, `index.json` có `schemaVersion`; shard `flows/<domain>.md` khi >500 flows — Q-1 đã chốt).
- `internal/structure/gitnexus.go` (modified, additive): export processes/flows (parse CLI JSON, spike shape `Dependents` trước).
- `internal/runner/knowledge_bootstrap.go` (**new**): `ensureKnowledgeBaseAsync` background once-per-process khi bind project (không block startup).

**Test signatures**
```go
func TestDistillerGeneratesSystemOverview(t *testing.T)
func TestDistillerGeneratesExecutionFlows(t *testing.T)
func TestDistillerIncrementalUpdateOnChangedFiles(t *testing.T)
```

---

### P-2: `knowledge.flow` Context Source Implementation

**Status: draft**

**Production changes**
- `internal/runner/knowledge_flow_context_source.go` (**new**): Implement interface `ContextSource` (CP-44). Đăng ký key `knowledge.flow` (priority ngay sau `conventions`; token cap ~500/1000 đo bằng counter của Budget Packer, cắt theo nguyên section).
- `internal/runner/context_sources_builtin.go`: Wire nguồn mới vào registry chung (opt-in, không vào `defaultContextSourceIDs`).

**Test signatures**
```go
func TestKnowledgeFlowSourceKeyMatches(t *testing.T)
func TestKnowledgeFlowSourceResolvesFlowFromLocus(t *testing.T)
func TestKnowledgeFlowSourceReturnsEmptyGracefullyWhenMissing(t *testing.T)
func TestKnowledgeFlowSourceRespectsTokenLimit(t *testing.T)
```

---

### P-3: Tích Hợp Incremental Update vào Node `audit`

**Status: draft**

**Production changes**
- `internal/runner/flow_validate_audit_dispatch.go` (modified): Tại completion path node `audit` sau gate pass — thu code paths (`ChangedPaths`+`WrittenPaths` lọc `isConcreteCodeTarget`), gọi worker nền khi `.flowpilot/knowledge/` đã tồn tại.
- `internal/runner/knowledge_bootstrap.go` (modified): thêm `UpdateAsync(workspace, changedPaths)` serialize per-workspace (`knowledgeUpdater{mu}`), lỗi chỉ log `[knowledge]`.

**Test signatures**
```go
func TestAuditNodeUpdatesKnowledgeBaseIncrementally(t *testing.T)
func TestAuditNodeNonBlockingOnKnowledgeError(t *testing.T)
```

---

### P-4: Cập nhật Flow Profiles & E2E Validation

**Status: draft**

**Production changes**
- Cập nhật `task-harness.yaml`, `bug-plan-harness.yaml`: Thêm `knowledge.flow` vào `candidateSources` của `scout` → `[conventions, knowledge.flow, canonical.head, feature.history]` và `plan_writer` (giữ nguyên `reviewer`/`coder`; không có profile investigate riêng — đã verify).
- Viết test E2E kiểm tra prompt của Planner nhận đúng nội dung tóm tắt luồng nghiệp vụ.

**Test signatures**
```go
func TestFlowPlanWriterReceivesExecutionFlowContext(t *testing.T)
func TestCoderNodeDoesNotReceiveKnowledgeFlow(t *testing.T)
```

---

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/knowledge/` (new package)
  - `apps/local-runner/internal/structure/gitnexus.go` (modified — additive export)
  - `apps/local-runner/internal/runner/knowledge_bootstrap.go` (new — ensure + UpdateAsync)
  - `apps/local-runner/internal/runner/knowledge_flow_context_source.go` (new)
  - `apps/local-runner/internal/runner/context_sources_builtin.go` (modified)
  - `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go` (modified — audit hook)
  - `apps/local-runner/internal/agentpack/flow-pack/flows/task-harness.yaml` (modified)
  - `apps/local-runner/internal/agentpack/flow-pack/flows/bug-plan-harness.yaml` (modified)
- modules: `knowledge`, `runner`, `agentpack`

## 6. Data or Migration Steps

- schema: none
- data backfill: `flowpilot init` sinh thư mục `.flowpilot/knowledge/` lần đầu cho project.

## 7. Validation Plan

- tests to add: ~11 tests
- manual checks: Kiểm tra nội dung các file markdown được sinh ra trong `.flowpilot/knowledge/` của dự án mẫu.

## 8. Rollout and Fallback

- Rollout: P-1 $\rightarrow$ P-2 $\rightarrow$ P-3 $\rightarrow$ P-4.
- Fallback: Nếu thư mục `.flowpilot/knowledge/` chưa tồn tại, context source trả về rỗng, flow tiếp tục bình thường mà không gây lỗi.

## 9. Risks

- `R-1` **Độ trễ khi quét lần đầu:** Dự án cực lớn có thể mất 30s để GitNexus index. Mitigation: Chạy ngầm bằng background goroutine, không block UI của TUI.

## 10. Definition of Done

- [ ] Bộ chưng cất sinh ra 3 file markdown chuẩn xác trong `.flowpilot/knowledge/`.
- [ ] Context Source `knowledge.flow` giải nghĩa đúng luồng nghiệp vụ từ Locus.
- [ ] Node `audit` tự động cập nhật vi sai sau khi hoàn thành task.
- [ ] Planner nhận được tóm tắt luồng mà không phải đọc 50 file code thô.
- [ ] Toàn bộ test additive đều green.

---

## Review Protocol

1. Implementer hoàn thành từng slice và tạo Task + CA doc.
2. Reviewer verify kích thước token của `knowledge.flow` (<1.000 tokens) và tính an toàn non-blocking của worker nền.
