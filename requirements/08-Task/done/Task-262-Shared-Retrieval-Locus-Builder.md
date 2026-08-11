# Task-262: Shared Retrieval-Locus Builder (CP-54 P-2)

## Metadata

- Document ID: `Task-262`
- Title: `Shared Retrieval-Locus Builder`
- Phase: `task`
- Status: `done` (2026-07-27 — implement + verify xong, xem [CA-423](../../../change-audit/CA-423-shared-retrieval-locus-builder.md))
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-27`
- Last Updated: `2026-07-27`
- Feature Keys: `context-regression-engine`
- Parent Documents: [CP-54: Locus-Anchored Relevance Retrieval](../../07-Coding-Plan/todo/CP-54-Locus-Anchored-Context-Relevance.md) (P-2), [SD-17](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md) (D-4 no-vector), [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (AC-7)
- Child Documents: `none`
- Related Documents: [Task-261](../done/Task-261-Persist-Changed-Paths-In-Change-Ledger.md) (P-1, done — cung cấp `Entry.ChangedPaths` để đối chiếu), [Task-246](../done/Task-246-Source-Excerpt-Runtime-Hint-Producers.md) (**tái dùng** `extractPromptSourcePaths` + `uncommittedChangedPaths`), [Task-247](../done/Task-247-Change-Contract-Context-Source-And-Downstream-Prompt.md) (pattern đọc contract store), [Task-259](./Task-259-Source-Dependence-Context-Source.md) (**hạ tầng dùng chung** — cùng bộ chuẩn hóa target T-3), [CP-52](../../07-Coding-Plan/todo/CP-52-Multiple-Agent-Context-Sync.md) (P-3 sẽ gọi lại), [BUG-323](../../09-BugFix/todo/BUG-323-GitNexus-Structure-Provider-Always-Returns-Empty.md) (**không chặn** — task này thuần git/contract, không đụng GitNexus)
- Replaces: `None`
- Tags: `context-regression-engine, locus, retrieval, change-contract, shared-infra, deterministic, additive`

## AI Quick View

### Summary

Một nơi duy nhất dựng **locus** — "vùng code turn này đang chạm" — hợp nhất từ ba nguồn theo thứ tự tin cậy giảm dần: `change.contract` đã khai → uncommitted diff → path nêu trong prompt. Kèm **bộ chuẩn hóa target dùng chung** loại dir-bucket/glob/doc, đúng thứ Task-259 T-3 cần.

Đây là mảnh hạ tầng mà **ba nơi** cắm vào: CP-54 P-3/P-4 (backward — lịch sử quanh vùng), Task-259 (forward — blast-radius quanh vùng), CP-52 P-3 (lọc noise cross-worktree). Làm một lần.

### Current Ask

Thêm kiểu `RetrievalLocus` + hàm dựng nó. **Chưa nối vào bất kỳ context source nào** — P-4 mới wiring. Sau task này hành vi runtime **không đổi một chút nào**.

### Key Decisions

- `T-1` **Kiểu đặt ở `featurecatalog`, builder đặt ở `runner`.** Scorer P-3 sống ở `featurecatalog/relevance.go` và nhận `RetrievalLocus`; mà chiều import là `runner → featurecatalog → changeledger` (đã verify: `featurecatalog` **không** import `runner`). Nếu đặt kiểu trong `runner` thì P-3 không compile được. Builder ở lại `runner` vì nó cần `flowgate.IsDocOrAuditFile` + hai helper Task-246 vốn nằm đó.
- `T-2` **Tái dùng, không viết lại.** `uncommittedChangedPaths` và `extractPromptSourcePaths` (Task-246) đã xử lý NUL-delimited git output, chuẩn hóa separator, và lọc doc/audit. Chỉ path lấy từ contract mới cần lọc thêm.
- `T-3` **Bộ chuẩn hóa target dùng chung** (bản dùng được của Task-259 T-3): giữ **file code cụ thể**; loại (a) dir-bucket không đuôi (`"apps"`, `"internal"` — dạng `InferFromDiff` sinh ra), (b) glob (`*?[`), (c) doc/audit (`flowgate.IsDocOrAuditFile`), (d) target mở đầu bằng `-` (chống bị đọc nhầm thành CLI flag khi Task-259 truyền vào `npx`).
- `T-4` **Tất định.** Dedupe + **sort** `Paths`/`Symbols`. Thứ tự ưu tiên nguồn chỉ dùng để dedupe; scorer coi locus là **tập hợp**, nên sort cho kết quả reproducible (kỷ luật BUG-266).
- `T-5` **`Symbols` thực tế sẽ luôn rỗng — ghi trung thực, không giả vờ.** `Contract.DeclaredSymbols` chưa bao giờ được populate (`ParseDeclaration` chỉ đọc `feature:`/`intent:`/`files:`; `InferFromDiff` chỉ set `DeclaredPaths`) — xem BUG-323 `Q-2`. Field vẫn giữ làm điểm cắm sẵn, nhưng comment phải nói rõ nó rỗng cho tới khi `Q-2` được giải, để người sau không tưởng nó đang chạy.
- `T-6` **Non-fatal.** Không contract / không git / prompt rỗng → locus rỗng, không lỗi. Locus rỗng chính là tín hiệu để P-4 rơi về recency (nhánh degrade của CP-54 QĐ-4).

### Constraints

- **Không sửa test cũ.** Không đổi `extractPromptSourcePaths`/`uncommittedChangedPaths` (đang được `source.excerpt` dùng) — chỉ **gọi** chúng.
- Không đụng GitNexus/`structure` (BUG-323).
- Không đổi hành vi runtime ở task này (chưa wiring).
- Deterministic, no-vector (`SD-17 D-4`).

### Open Questions

- `Q-1` Chat-mode per-turn injection không có `WorkflowRunID` rõ (CP-54 `Q-1`) ⇒ ở đó locus chỉ còn diff + prompt. Builder đã hỗ trợ sẵn (`runID` rỗng → bỏ qua contract); việc *gọi* từ chat-mode chốt ở P-4.
- `Q-2` Có cần cap số path trong locus không (contract khai rất rộng)? Hoãn tới P-3 khi đo được ảnh hưởng lên scoring.

## 1. Goal

Cho một `(workspace, runID, prompt)`, trả về tập path code cụ thể mà turn đang chạm — đủ để P-3 chấm điểm overlap với `Entry.ChangedPaths` (Task-261).

## 2. Parent Links

- coding plan: `CP-54` P-2
- tech design: `SD-17` D-4
- system spec: `SS-14` AC-7

## 3. Trigger

P-1 đã lưu "commit này đụng file nào". Để so khớp, read-side cần vế còn lại: "turn này đang đụng file nào". Task này dựng vế đó.

## 4. Exact Change

- `T-A` `internal/featurecatalog/locus.go` (**mới**): kiểu `RetrievalLocus{Paths, Symbols, RunID}` + `IsEmpty()`.
- `T-B` `internal/runner/retrieval_locus.go` (**mới**): `isConcreteCodeTarget(p string) bool` (bộ lọc `T-3`) + `buildRetrievalLocus(workspace, runID, prompt string) featurecatalog.RetrievalLocus`.
- `T-C` Test additive: `internal/runner/retrieval_locus_test.go`, `internal/featurecatalog/locus_test.go`.

## 5. Touched Areas

- files: `internal/featurecatalog/locus.go` (mới), `internal/runner/retrieval_locus.go` (mới), 2 file test mới
- modules: `featurecatalog`, `runner`
- routes / tables: none

## 6. Acceptance Check (DoD)

- [x] Contract declared có file thật → `locus.Paths` đúng — `TestBuildRetrievalLocusUsesDeclaredContractPaths`.
- [x] Contract **inferred** (dir-bucket) bị loại — `TestBuildRetrievalLocusDropsInferredDirectoryBuckets` (chỉ `go.mod` sống sót).
- [x] Glob / doc / `-`-prefix bị loại — `TestBuildRetrievalLocusDropsGlobsDocsAndFlags` + 4 test đơn vị `TestIsConcreteCodeTarget*`.
- [x] Diff + prompt được gộp, dedupe — `TestBuildRetrievalLocusMergesDiffAndPromptPaths`, `...DedupesAcrossSources` (cùng path từ cả 3 nguồn → 1).
- [x] Sorted + tất định — `TestBuildRetrievalLocusIsSortedAndDeterministic`; symbol: `TestDedupeSortedSymbolsSortsAndDedupes`.
- [x] Không contract / không git / blank workspace → rỗng, `IsEmpty()` true — `...EmptyWhenNothingToAnchorOn`, `...NonFatalOutsideGitRepo`, `...BlankWorkspaceIsSafe`.
- [x] `runID` rỗng → bỏ qua contract — `TestBuildRetrievalLocusEmptyRunIDSkipsContract`.
- [x] **Test cũ xanh nguyên trạng** — `git status` chỉ có **file mới**; không file có sẵn nào bị sửa. Va tên `containsString` xử lý bằng cách đổi tên helper **của mình** (`locusHasPath`), không đụng khai báo cũ.
- [x] `go build ./...` + `go vet` sạch; `featurecatalog` full xanh; runner context+golden xanh.
- [x] Hành vi runtime không đổi — golden fixture (`TestBuildFlowContextPackage*`, `TestRenderFlowContextPackage*`) giữ nguyên.
- [x] CA note: [CA-423](../../../change-audit/CA-423-shared-retrieval-locus-builder.md).
- [x] Cross-provider Case 1 agnostic — grep `providerKey|ProviderKey` trên 2 file mới → **0 kết quả**.
- [x] `Q-1` đã xử: builder nhận `runID` rỗng (chat-mode) → chỉ dùng diff + prompt; việc *gọi* từ chat-mode chốt ở P-4.

## 7. Out of Scope

- Scorer (P-3), wiring vào `HistorySlot`/`feature.history` (P-4), `chat.summary` (P-5), symbol tier (P-6).
- Populate `DeclaredSymbols` (BUG-323 `Q-2`).

## 8. Cross-Provider Note

Provider-agnostic (Case 1): builder đọc git + contract store, không nhận `providerKey`, không branch theo provider. Cần grep xác nhận và ghi vào CA note.
