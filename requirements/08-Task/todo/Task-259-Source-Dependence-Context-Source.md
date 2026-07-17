# Task-259: `source.dependence` Context Source (GitNexus Blast-Radius From Change Contract)

## Metadata

- Document ID: `Task-259`
- Title: `source.dependence Context Source (GitNexus Blast-Radius From Change Contract)`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-17`
- Last Updated: `2026-07-17`
- Parent Documents: [CP-43: Change Contract And Canonical Intent Signature](../../07-Coding-Plan/inprogress/CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (P-6, mới), [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (US-3, AC-7, AC-9)
- Child Documents: none
- Related Documents: [CP-43-CATALOG: Context Source Catalog And Test Log](../../07-Coding-Plan/inprogress/CP-43-Context-Source-Catalog-And-Test-Log.md) (§3 mô tả source; §4.1 hàng test; §6 B12 e2e), [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md), [CP-50: Context Source Completion](../../07-Coding-Plan/done/CP-50-Context-Source-Completion.md), [Task-247: Change-Contract Context Source](../done/Task-247-Change-Contract-Context-Source-And-Downstream-Prompt.md), [Task-244: Canonical Head First-Class Context Source](../done/Task-244-Canonical-Head-First-Class-Context-Source.md)
- Replaces: `None`
- Tags: `context-source, gitnexus, change-contract, dependence, blast-radius, structure, default-set`

## AI Quick View

### Summary

- Thêm context source thứ 10 `source.dependence` vào **default set**: khi run đã có `change.contract` khai `declared_paths`/`declared_symbols`, source này chạy GitNexus impact trên từng target và render **blast-radius** ("đổi cái này ảnh hưởng cái nào") cho step sau (validate/audit/review) đọc.
- Tái dùng module có sẵn `internal/structure` (`Provider.Dependents` → `npx gitnexus impact <target> --json`, có sẵn **fallback file-level** khi GitNexus vắng). Không thêm dependency, không thêm bảng DB.
- Deterministic, không AI ở collect (tool/graph). Degrade-mềm AC-9: không contract / GitNexus stale / lỗi → section rỗng hoặc fallback, không bao giờ chặn turn.
- Tiêu thụ `change.contract` ⇒ đăng ký **sau** `change.contract` (priority 3), **trước** `source.excerpt` (priority 4) → đề xuất priority `3.5` (dùng số nguyên thực tế, xem T-2).

### Current Ask

- Implement source `source.dependence` + đăng ký vào registry + đưa vào `defaultContextSourceIDs`; render qua generic pass; bounded; unit + integration test; UI descriptor + docs sync (CP-43 gốc + CP-43-CATALOG).

### Key Decisions

- `T-1` Input **chỉ** từ `change.contract` của run (`changecontract.Store.GetLatestForRun`) — không tự suy diễn target từ diff (đó là việc của `source.excerpt`). Không contract → section rỗng.
- `T-2` Priority: đăng ký giữa `change.contract` (3) và `source.excerpt` (4). Vì priority là `int`, dời `source.excerpt` 4→5, `chat.summary` 5→6, và các custom source +1 (mcp 6→7, jira 7→8/8→9, firebase 9→10) — HOẶC cấp `source.dependence` = 4 và đẩy phần còn lại xuống 1. Chọn cách ít xáo trộn nhất lúc implement, ghi rõ mapping vào Completion Notes. Thứ tự tương đối (sau contract, trước excerpt) là ràng buộc; con số cụ thể không.
- `T-3` Collect qua `structure.New(cwd, hasGitNexus).Dependents(ctx, target)`; `hasGitNexus` lấy từ cùng chỗ gate hook đang dùng (`gate_hook.go` `structure.New(cwd, hasGitNexus)`). Bounded: cap ≤ 20 target (khớp cap contract/excerpt) + cap ≤ N dependents/target (đề xuất 15) để tránh prompt bloat.
- `T-4` Render đi qua **generic pass** (`### source.dependence` + `_Source:_` tự có) — KHÔNG thêm case đặc biệt trong renderer (khác canonical.head vốn có heading riêng). `Complete=false` (dynamic dispatch) → thêm 1 dòng chú thích "danh sách có thể chưa đủ".
- `T-5` `Deterministic() = true`. Section rỗng ⇒ golden/fixture không đổi (fixture không có contract → không dependence) — cấm sửa golden expectation.

### Constraints

- Giữ bất biến CP-41/SD-22: deterministic, **không vector**, mọi source degrade-mềm (AC-9), không chặn turn/run.
- GitNexus exec bounded (đã có timeout trong `structure.gitNexusProvider`); nếu index stale, chấp nhận kết quả có thể cũ — KHÔNG tự chạy `npx gitnexus analyze` trong path collect (side-effect nặng); chỉ đọc.
- `apps/admin-web` deprecated — UI chỉ sửa `apps/desktop-flowpilot`.
- Không skip `verify`: chạy test battery §6 + build/vet sạch trước khi đóng.
- GitNexus impact analysis cho các hàm fan-in **đã chạy 2026-07-17** (index fresh `6741bdc`, xem CP-43-CATALOG §5). Ràng buộc rút ra: (1) đường `Collect`→`buildFlowContextPackage`→generic `RenderFlowContextPackage` toàn **LOW** — an toàn thêm source; (2) **KHÔNG** đưa `source.dependence` vào skip-list renderer; (3) **KHÔNG** append thẳng vào prompt kiểu P-4 qua `composeFlowNodeAgentPrompt` (**HIGH**); (4) nếu phải sửa `behaviorContextProduce` để wire provider, trace behavior-registry tay — impact=0 là báo-thiếu do dynamic dispatch.

### Open Questions

- `Q-1` Contract v1 lấy theo **run** (`GetLatestForRun`, một feature/mạch việc mỗi run — nhất quán Task-247 R-5). Nếu thực tế cần per-step dependence, mở Task riêng.
- `Q-2` Symbol-level vs file-level: khi `structure.Available()==false` (project GitNexus chưa index) → fallback file-level (import-dependents + git co-changed). Chấp nhận độ chính xác thấp hơn, đúng CP-43 Q-3 nuance (block chỉ khi symbol-level; đây là **context**, không phải gate, nên không block bao giờ).
- `Q-3` Có nên cache kết quả Dependents theo (target, index-commit) trong run để tránh gọi GitNexus nhiều lần? v1: không cache (mỗi target ≤ 20, mỗi lệnh bounded). Theo dõi latency, mở follow-up nếu chậm.

### Source Refs

- `CP-43` P-2 (drift = declared vs touched, dùng `structure.Dependents`), P-3/P-5, Q-3 (symbol-level gated on `structure.Available()`).
- `CP-44` P-4/P-5/D-5 (registry, opt-in vs default, bounded external source shape).
- `CP-50` P-4 (`change.contract` source — nguồn input), P-1 (canonical.head — mirror struct shape).
- `structure` module: `Provider.Dependents`, `DependentsSummary{Count,Nearest,Flows,Complete}`, `New(repoDir, isGitNexusOK)`.

## 1. Goal

Cho mọi run đã khai Change Contract: step sau (validate/audit/review/reprompt) nhìn thấy **blast-radius** của vùng đã khai — "sửa các path/symbol này thì những symbol/file/flow nào bị ảnh hưởng" — được suy ra deterministic từ GitNexus knowledge graph (không AI), bounded, degrade-mềm khi GitNexus vắng hoặc chưa có contract. Đây là câu trả lời code-level cho câu hỏi "đã handle context-depend-code qua gitnexus chưa: đổi cái này ảnh hưởng cái nào".

## 2. Parent Links

- coding plan: `CP-43` (thêm P-6 — `source.dependence`), catalog `CP-43-CATALOG`
- tech design: `SD-21` (change contract), `SD-22` (registry)
- system spec: `SS-14` US-3 / AC-7 (records symbols/files changed) / AC-9 (non-fatal)
- specific upstream ids: `CP-43 P-2/P-3/Q-3`, `CP-50 P-4`, `CP-44 P-4/P-5`

## 3. Trigger

Câu hỏi owner (2026-07-17): "chúng ta đã handle context-depend-code qua gitnexus chưa — kiểu change cái này ảnh hưởng cái nào". Hiện `change.contract` mới chỉ khai *declared scope*; chưa có source nào biến declared scope thành **impact/dependents**. `internal/structure` (GitNexus wrapper) đã tồn tại từ CP-35/CP-43 nhưng mới dùng trong flow **gate** (drift detection), chưa từng surface như một **context source** cho prompt. Task này lấp đúng khoảng đó.

## 4. Exact Change

- `T-1` File mới `apps/local-runner/internal/runner/context_source_dependence.go`: `const ContextSourceDependence ContextSourceID = "source.dependence"`; struct `dependenceSource{priority int}` (+ inject được một `structure.Provider` factory để test dùng fake); `ID/Priority/Deterministic()=true`. `Fetch`: guard `hints.WorkflowRunID==""` / `hints.Workspace==""` → rỗng; mở `changecontract.OpenStoreReadOnly` → `GetLatestForRun` → không có → rỗng; gộp+dedup `DeclaredPaths`+`DeclaredSymbols`, cap 20; với mỗi target gọi `provider.Dependents(ctx, target)`; build body bounded (cap dependents/target, note `Complete=false`); `SourceRef = filepath.ToSlash(.../contracts/contracts.ndjson)` (nguồn input) hoặc `"gitnexus:impact"`.
- `T-2` `context_sources_builtin.go`: `mustRegisterContextSource(r, &dependenceSource{priority: <sau change.contract, trước source.excerpt>})`; thêm `string(ContextSourceDependence)` vào `defaultContextSourceIDs` (đặt sau `change.contract`). Điều chỉnh priority các source khác nếu cần (T-2 Key Decision) — ghi mapping.
- `T-3` Wiring provider: `dependenceSource` cần `cwd` + `hasGitNexus`. Lấy từ `hints.Workspace` + cùng cơ chế `gate_hook.go` dùng để dựng `structure.New`. Không tạo provider nếu workspace rỗng.
- `T-4` Render: xác nhận generic pass tự thêm `### source.dependence` — KHÔNG thêm vào skip-list của `RenderFlowContextPackage` (khác canonical.head). Không double-render.
- `T-5` Test file mới `context_source_dependence_test.go` (xem §6).
- `T-6` UI: thêm `{ id: "source.dependence", label: "Source Dependence (impact)" }` vào `contextSourceOptions` trong `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`; `npx tsc --noEmit`.
- `T-7` Docs: CP-43 gốc thêm P-6 note + link Task-259; CP-43-CATALOG §1.3/§3 chuyển source từ "planned" → "landed" khi xong; đánh dấu §4.1 hàng `source.dependence` test thật (bỏ "planned"); §6 B12 giữ.

## 5. Touched Areas

- files (mới): `internal/runner/context_source_dependence.go`, `internal/runner/context_source_dependence_test.go`
- files (sửa): `internal/runner/context_sources_builtin.go`, có thể `internal/runner/flow_context_package.go` (chỉ nếu cần chỉnh generic-render note), `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`
- modules: runner context assembly, `structure` (reuse, read-only), changecontract (read-only)
- routes: không
- tables: không (đọc `.flowpilot/contracts/contracts.ndjson` + GitNexus index)

## 6. Acceptance Check

- Build/vet sạch: `go build ./...` && `go vet ./internal/runner/...`.
- Unit (fake provider): `TestDependenceSourceFromContractTargets` (contract có 2 declared path/symbol → body chứa dependents của cả hai), `TestDependenceSourceNoContractDegrades` (không contract → rỗng, no warning), `TestDependenceSourceGitNexusUnavailableFallsBackFileLevel` (provider fallback → vẫn có body file-level, không lỗi), `TestDependenceSourceBoundsTargetsAndDependents` (>20 target / nhiều dependents → cap), `TestDependenceInDefaultSetAndRegistered` (`defaultContextSourceIDs` chứa id + `Resolve` ok, đăng ký sau change.contract trước source.excerpt).
- Render/golden: `TestRenderFlowContextPackageDependenceAfterContract` (block `### source.dependence` đứng sau `### change.contract`); `TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor` pass **không sửa expectation** (fixture không có contract).
- Full: `go test ./internal/runner/ -count=1` không regression mới so baseline flake.
- Live (CP-43-CATALOG §6 B12): feature GitNexus-indexed + turn Coding khai contract → prompt validate/audit sau chứa block "sửa `<target>` ảnh hưởng `<dependents>` + flows"; GitNexus stale → fallback, không lỗi.
- GitNexus impact analysis cho hàm fan-in (`Collect`, `RenderFlowContextPackage`, …) đã chạy + ghi risk (CP-43-CATALOG §5, 2026-07-17). Sau khi land code, chạy lại impact cho `source.dependence` `Fetch` + `detect_changes` để xác nhận blast-radius thực tế TRƯỚC khi merge.

## 7. Out of Scope

- KHÔNG biến `source.dependence` thành **gate rule** (block/warn) — đó là `r-scope` drift của CP-43 P-2, đã có/đang riêng. Task này chỉ là **context** cho prompt.
- KHÔNG tự chạy `npx gitnexus analyze` (re-index) trong path collect.
- KHÔNG suy diễn target từ diff (việc của `source.excerpt`); chỉ dùng contract đã khai.
- KHÔNG thêm per-symbol Canonical Head hay đổi granularity contract (CP-43 Q-4).
- KHÔNG cache cross-run (v1).

## 8. Completion Notes

- result: `draft` — chưa implement (planned). Sẽ cập nhật khi land code + test.
- follow-ups: cân nhắc cache Dependents theo (target, index-commit) nếu latency cao (Q-3); cân nhắc per-step dependence nếu run-level thô (Q-1).
- upstream docs updated: CP-43 gốc (P-6 + link) và CP-43-CATALOG (§1.3/§3/§4.1/§6) — đồng bộ trong turn tạo task này; đánh dấu "landed" khi code xong.
