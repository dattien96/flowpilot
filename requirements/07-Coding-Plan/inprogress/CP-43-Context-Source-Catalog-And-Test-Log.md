# CP-43 Companion: Context Source Catalog + Separated Phase-B Test Log

## Metadata

- Document ID: `CP-43-CATALOG`
- Title: `Context Source Catalog And Separated Context Test Log`
- Phase: `coding_plan` companion / catalog + verification log (not a new CP number)
- Status: `inprogress` (catalog canonical; test battery runnable; source.dependence planned via Task-259)
- Owner: `FlowPilot`
- Created: `2026-07-17`
- Last Updated: `2026-07-17`
- Parent Documents: [CP-43](./CP-43-Change-Contract-And-Canonical-Intent-Signature.md)
- Child Documents: none
- Related: [CP-44: Pluggable Context Source Registry](../done/CP-44-Pluggable-Context-Source-Registry.md), [CP-45: Generic Artifact Types And Instances](../done/CP-45-Generic-Artifact-Types-And-Instances.md), [CP-50: Context Source Completion](../done/CP-50-Context-Source-Completion.md), [SD-21](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SD-22](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [Task-259: source.dependence Context Source](../../08-Task/todo/Task-259-Source-Dependence-Context-Source.md)
- Tags: `context-source, catalog, registry, canonical-head, change-contract, source-excerpt, gitnexus, source-dependence, test-log`
- Feature Keys: `agent-flow-engine, change-contract`

## Purpose

Đây là file **metadata/catalog** đi kèm CP-43 gốc (cùng mô hình 2-file như CP-51 có `CP-51` + `CP-51-PhaseAB-Timeline-And-Verification-Log`). Mục tiêu:

1. **Liệt kê canonical toàn bộ context source** đang có trong runner — mỗi loại: implemented ở CP/Task nào, collect **cái gì**, collect **bằng cách nào**, và quan trọng nhất: **AI hay code của ta collect** (§1–§2).
2. **Test context tách biệt tại đây** — battery Phase B (per-source automated tests + live E2E checklist) được **move từ [CP-51 companion §2.3/§3.2](../done/CP-51-PhaseAB-Timeline-And-Verification-Log.md)** sang, để test từng source độc lập (§4–§6).
3. Ghi nhận **source mới `source.dependence`** (Task-259) theo đúng format các source cũ + thêm test cho nó (§3, §4.2 hàng mới, §6 B12).

> **Ranh giới với CP-51 companion:** CP-51 companion giờ chỉ giữ Phase A (flow-graph) + CP-51 (durable turn dispatch). Mọi thứ **context/Phase B** ở đây.

---

## 1. Context source catalog (canonical)

Hai nhóm, khớp mô hình runtime:

- **Nhóm Default** — nằm trong `defaultContextSourceIDs`; là bộ mặc định **Chat Mode** dùng, và Flow Mode cũng có node built-in (`context.produce`) dùng chính bộ default này khi flow không khai `contexts.sources` tường minh.
- **Nhóm Custom / opt-in** — đã đăng ký nhưng **không** vào default; chỉ chạy khi flow/instance khai tường minh. Chủ yếu là MCP/external adapter.

Nguồn sự thật code: [context_sources_builtin.go](../../../apps/local-runner/internal/runner/context_sources_builtin.go) (`defaultContextSourceIDs`, `registerBuiltinContextSources`).

### 1.1 Nhóm Default (5)

| Prio | Source ID | Implemented (CP / Task) | Collect **cái gì** | Store / nguồn đọc |
|---|---|---|---|---|
| 1 | `canonical.head` | CP-50 P-1 / [Task-244](../../08-Task/done/Task-244-Canonical-Head-First-Class-Context-Source.md) (concept: CP-43 P-3/P-5) | Current-truth statement + rejected dead-ends của feature | `.flowpilot/canonical/<feature_key>.json` |
| 2 | `feature.history` | CP-41 / CP-35 (migrated to registry CP-44) | Ordered commit/CA history slot ("newest = truth") | `.flowpilot/ledger/feature_history.ndjson` |
| 3 | `change.contract` | CP-50 P-4 / [Task-247](../../08-Task/done/Task-247-Change-Contract-Context-Source-And-Downstream-Prompt.md) (concept: CP-43 P-1) | Declared scope (feature/intent/paths/symbols) của turn Coding trong run | `.flowpilot/contracts/contracts.ndjson` |
| 4 | `source.excerpt` | source: CP-41; producer runtime: CP-50 P-3 / [Task-246](../../08-Task/done/Task-246-Source-Excerpt-Runtime-Hint-Producers.md) | Excerpt file thật từ diff chưa commit + path nêu trong prompt | Đọc file trực tiếp trong workspace (bounded) |
| 5 | `chat.summary` | CP-41 / CP-35 (migrated CP-44) | Long-term chat memory slot per-feature | `.flowpilot/ledger/chat_summary.ndjson` |

### 1.2 Nhóm Custom / opt-in (4)

| Prio | Source ID | Implemented (CP / Task) | Collect **cái gì** | Nguồn |
|---|---|---|---|---|
| 6 | `mcp.driver` | CP-44 P-5 / [Task-195] | Nội dung 1 driver ref qua MCP đã kết nối (vd. Google Drive doc) | Live MCP call, bounded 5s / 8KB |
| 7 | `jira.issue` | [Task-229] (CP-05-06 P-3/P-4) | 1 Jira issue theo key | Jira qua credential đã connect |
| 8 | `jira.sprint` | [Task-229] | Danh sách issue của sprint (`active` hoặc id) | Jira qua credential đã connect |
| 9 | `firebase.crashlytics` | [Task-231] (CP-05-04 P-3/P-4) | 1 Crashlytics issue theo crash id | MCP client → `firebase-tools mcp --only crashlytics` |

### 1.3 Planned (1) — sẽ vào Default sau Task-259

| Prio | Source ID | Task | Collect **cái gì** | Nguồn |
|---|---|---|---|---|
| **3** (= `change.contract`; sắp sau nó & trước `source.excerpt` nhờ SourceType tiebreak, không renumber — Task-259 T-2) | `source.dependence` | [Task-259](../../08-Task/todo/Task-259-Source-Dependence-Context-Source.md) (CP-43 P-6) | Blast-radius: symbol/file nào bị ảnh hưởng nếu sửa các **target code cụ thể** đã khai trong `change.contract` (symbol trước; loại dir-bucket/glob/doc) | GitNexus impact (`structure.Provider.Dependents`), **v1 GitNexus-only** |

---

## 2. Cách collect: AI hay code? (chiều quan trọng nhất)

**Bất biến CP-41/CP-44/SD-22:** collect (hàm `Fetch`) **luôn deterministic bằng code** — không vector, không gọi AI *lúc collect*. Điều gì "do AI" chỉ xảy ra ở **write-time trước đó** (khi tạo ra file head/contract/summary), không phải lúc source đọc lên. Phân loại đầy đủ:

| Source | Content sinh ra bởi | Collect (`Fetch`) bởi | Ghi chú phân loại |
|---|---|---|---|
| `canonical.head` | **AI** (cheap-tier viết canonical behavior statement + lý do reject dead-ends — CP-43 P-3/P-4), ghi lúc turn trước | **Code** (`changecontract.LoadHead` đọc JSON → `RenderHeadBlock`) | **Hybrid**: AI viết head sớm, code đọc lúc collect |
| `feature.history` | **Code** (commit ledger + change-audit CA notes — facts từ git/CA) | **Code** (`featurecatalog.HistorySlot` đọc NDJSON) | **Thuần code** — không AI ở bất kỳ đâu |
| `change.contract` | **AI** (khi AI khai explicit block, Confidence=`declared`) **hoặc Code** (khi infer từ diff đầu tiên, Confidence=`inferred`) | **Code** (`Store.GetLatestForRun` đọc NDJSON) | **Hybrid write, code read** |
| `source.excerpt` | **Code** (git diff `--name-only HEAD` + untracked; extract path từ prompt bằng heuristic deterministic — **không AI**) | **Code** (`readSourceExcerpts` đọc file, guard workspace/symlink/binary/cap) | **Thuần code** |
| `chat.summary` | **AI** (cheap-tier tóm tắt thảo luận — SD-17 §3.2), ghi lúc turn trước | **Code** (`featurecatalog.ChatSummarySlot` đọc NDJSON) | **Hybrid** |
| `mcp.driver` | External (nội dung do hệ thống ngoài như Google Drive tạo) | **Code** lái MCP call qua adapter (timeout+cap) | **External, code-driven** |
| `jira.issue` / `jira.sprint` | External (Jira) | **Code** lái adapter (HTTP qua credential) | **External, code-driven** |
| `firebase.crashlytics` | External (Firebase Crashlytics) | **Code** lái MCP client tới `firebase-tools` | **External, code-driven** |
| `source.dependence` (mới) | **Tool/Code** (GitNexus knowledge graph — impact/dependents; **không AI**) | **Code** (`structure.Provider.Dependents` chạy `npx gitnexus impact --json`; **v1 GitNexus-only** — GitNexus vắng → render note "chưa index", KHÔNG chạy fallback walk vì fallback provider uncancellable + O(repo)/target, xem Task-259 T-5) | **Thuần tool/code**; *input* là **target code cụ thể** trích từ `change.contract` (symbol trước; dir-bucket/glob/doc bị loại — Task-259 T-3) |

**Chốt cho câu hỏi "AI hay code của ta collect":**
- **Không source nào gọi AI lúc collect.** Collect = code deterministic (đúng bất biến "no vector").
- **Content** thì 3 source *hybrid* (canonical.head, change.contract-declared, chat.summary) mang chữ do AI viết ở write-time trước.
- **`source.dependence` mới hoàn toàn không AI** ở cả write lẫn collect — nó là **code của ta gọi GitNexus tool** để suy ra "đổi cái này ảnh hưởng cái nào", lấy đầu vào từ `change.contract`.

---

## 3. Source mới: `source.dependence` (Task-259) — theo format catalog

**Ý tưởng (khớp câu hỏi ban đầu "đã handle context depend code qua gitnexus chưa — đổi cái này ảnh hưởng cái nào"):** khi run đã có `change.contract` khai `declared_paths`/`declared_symbols`, source này chạy GitNexus impact trên từng target đó và render **blast-radius note** (danh sách dependents gần nhất + execution flows bị chạm), để step sau (validate/audit/review) biết ngay "sửa vùng đã khai sẽ động tới đâu" mà không phải tự grep.

| Trường | Giá trị |
|---|---|
| Source ID | `source.dependence` |
| Priority | **`3`** (bằng `change.contract`); xếp sau `change.contract` & trước `source.excerpt` nhờ tiebreak SourceType của stable-sort — **không renumber** các source khác (Task-259 T-2) |
| Nhóm | **Default** (owner muốn vào default set, giống canonical.head/change.contract) |
| Deterministic | `true` (tool/graph, không AI; output **sorted** để reproducible) |
| Collect **cái gì** | Với mỗi **target code cụ thể** trích từ contract của run (symbol trước; loại dir-bucket/glob/doc — T-3) → `DependentsSummary{Count, Nearest, Flows, Complete}` |
| Collect **bằng cách nào** | `structure.New(cwd, hasGitNexus).Dependents(bctx, target)` → `npx gitnexus impact <target> --json`, dưới **một** `context.WithTimeout` chia sẻ (~25s) cho toàn Fetch (T-4). `hasGitNexus` đọc từ cache `tooling.json` — **no subprocess** (T-8). **v1: GitNexus-only** — không chạy fallback (T-5) |
| **AI hay code** | **Code của ta** gọi **GitNexus tool** — không AI |
| Input phụ thuộc | `change.contract` (`GetLatestForRun`); run chưa có contract, hoặc contract **inferred** (chỉ dir-bucket) → section zero-value, degrade-mềm AC-9 |
| Store / nguồn | Không đọc file store riêng; đầu vào là contract (`.flowpilot/contracts/contracts.ndjson`) + `tooling.json` cache + GitNexus index |
| Degrade | GitNexus vắng → note "chưa index" (không walk); contract rỗng/inferred/target rỗng → section zero-value (**Warnings nil, Omitted nil** — golden-safe); mọi lỗi/target → skip, không chặn turn |
| Bound | Cap ≤ **10** target (mỗi target 1 npx) + cap ≤ **15** dependents/target + shared wall-clock budget ~25s |

Chi tiết implementation, T-* và DOD nằm trong [Task-259](../../08-Task/todo/Task-259-Source-Dependence-Context-Source.md). File này chỉ giữ vai trò catalog + test.

---

## 4. Automated test inventory — per source (moved từ CP-51 §2.3)

Working directory: `cd apps/local-runner`.

`go test` một dòng (rộng, chạy nhanh — chi tiết theo **từng source** ở bảng §4.1):

```bash
go test ./internal/runner/ -count=1 -timeout 5m \
  -run 'TestContext|TestChangeContract|TestRenderFlowContext|TestContract|TestSourceExcerpt|TestArtifactBinding|TestResolveEnabledContext'
```

**Vì sao cần bảng chi tiết:** pattern rộng không nói rõ **source nào** hỏng, và không tách được source **mới** (canonical.head, change.contract) với source **cũ** (feature.history/chat.summary/source.excerpt/jira/mcp/firebase) + registry/precedence. CP-51 (durable dispatch) không đổi package/registry, nhưng regression ở turn dispatch có thể vô tình đổi thời điểm `Collect`/`buildFlowContextPackage` được gọi — nên **cả nhóm cũ lẫn mới đều phải xanh**.

### 4.1 Theo từng context source

| Source | Vai trò / khi nào có mặt | Test file | Test chính | Lệnh |
|---|---|---|---|---|
| `canonical.head` (mới, P-1) | Priority 1; current-truth + rejected dead-ends; default set | `context_source_canonical_head_test.go` | `TestCanonicalHeadSourceFetchesHeadBlock`, `TestCanonicalHeadSourceNoHeadDegrades`, `TestCanonicalHeadSourceUnverifiedFeatureEmpty`, `TestCanonicalHeadInDefaultSetAndRegistered`, `TestFeatureHistorySourceNoLongerPrependsHead`, `TestFeatureHistorySourceNoHeadFallsBackToPriorBehavior` | `go test ./internal/runner/ -run 'TestCanonicalHead\|TestFeatureHistorySourceNo' -count=1` |
| `feature.history` (**cũ**, pre-CP-43) | Priority 2; ledger prior-work slot; **không còn** prepend Head (tách bởi P-1) | `context_source_canonical_head_test.go`, `flow_context_package_test.go` | `TestBuildFlowContextPackageVerifiedFeatureIncludesHistory`, `TestBuildFlowContextPackageIncludesChatSummaryWhenPresent`, `TestBuildFlowContextPackageLowConfidenceDoesNotInjectWrongHistory`, `TestBuildFlowContextPackageMissingLedgersDegrades` | `go test ./internal/runner/ -run 'TestBuildFlowContextPackage(VerifiedFeature\|IncludesChatSummary\|LowConfidence\|MissingLedgers)' -count=1` |
| `change.contract` (mới, P-4) | Priority 3; declared scope từ turn Coding, xuất hiện ở prompt step sau | `context_source_change_contract_test.go` | `TestChangeContractSourceFetchAndDefault`, `TestAppendChangeContractIfAnyNoDouble`, `TestAppendChangeContractIfAnyParentKeyedFromChildCapture`, `TestComposeFeatureBlocksHeadOnly/NoHeadNoHistory/HeadBeforeHistory`, `TestRenderFlowContextPackageHeadFirst` | `go test ./internal/runner/ -run 'TestChangeContract\|TestAppendChangeContract\|TestComposeFeatureBlocks\|TestRenderFlowContextPackageHeadFirst' -count=1` |
| `source.excerpt` (**cũ** source, producer **mới** P-3) | Priority 4; excerpt runtime từ diff chưa commit + path nêu trong prompt | `flow_context_package_test.go`, `flow_context_hint_paths_test.go` (nếu có) | `TestBuildFlowContextPackageSourceExcerptCapsAndOmissions`, `TestBuildFlowContextPackageRejectsOutsideWorkspacePath` | `go test ./internal/runner/ -run 'TestBuildFlowContextPackageSourceExcerpt\|TestBuildFlowContextPackageRejectsOutsideWorkspacePath\|TestExtractPromptSourcePaths\|TestUncommittedChangedPaths' -count=1` |
| `chat.summary` (**cũ**, CP-41) | Priority 5; long-term chat memory | `flow_context_package_test.go` | `TestBuildFlowContextPackageIncludesChatSummaryWhenPresent` | `go test ./internal/runner/ -run 'TestBuildFlowContextPackageIncludesChatSummary' -count=1` |
| `jira.issue` / `jira.sprint` (**cũ**, non-default) | Runtime-target-note; chỉ khi flow khai tường minh | `context_source_jira_test.go` | `TestJiraIssueSourceProducesBoundedSectionWithSourceRef`, `TestJiraSprintSourceProducesBoundedSectionWithSourceRef`, `TestJiraSourcesRegisteredAndNotDefault`, `TestFlowContextPackageStillHasNoVectorDependencyWithJiraSources` | `go test ./internal/runner/ -run 'TestJira' -count=1` |
| `mcp.driver` (**cũ**, non-default) | Google Drive / MCP-backed excerpt; bounded content | `context_source_mcp_test.go`, `context_source_mcp_production_test.go` | `TestMcpDriverSourceProducesBoundedSectionWithSourceRef`, `TestMcpDriverSourceBoundsLargeContent`, `TestMcpDriverSourceDegradesOnMcpTimeout`, `TestTwoMCPBackedSourcesCoexistIndependently`, `TestSetMCPDriverAdapterWiresRegisteredSource` | `go test ./internal/runner/ -run 'TestMcpDriverSource\|TestTwoMCPBackedSourcesCoexist\|TestSetMCPDriverAdapter\|TestGoogleDriveDriverAdapter' -count=1` |
| `firebase.crashlytics` (**cũ**, non-default) | Crash-signal excerpt; bounded, degrade sạch nếu adapter lỗi | `context_source_firebase_test.go` | `TestFirebaseCrashlyticsSourceProducesBoundedSectionWithSourceRef`, `TestFirebaseCrashlyticsSourceDegradesOnAdapterError`, `TestFirebaseCrashlyticsSourceRegisteredAndNotDefault` | `go test ./internal/runner/ -run 'TestFirebaseCrashlyticsSource' -count=1` |
| Registry / precedence (**cũ**, CP-44) | Duplicate-ID reject, unknown-ID degrade/fail, priority ordering, step-vs-flow override | `context_source_registry_test.go`, `context_source_step_precedence_test.go`, `context_source_flow_binding_test.go` | `TestContextSourceCollectStableOrderByPriorityThenID`, `TestContextSourceCollectDegradesOnSourceError`, `TestUnknownSourceIDFailsFlowLoad`, `TestResolveEnabledContextSourceIDsStepOverridesFlow`, `TestResolveEnabledContextSourceIDsOldCP44FlowUnaffectedByArtifactValidation` | `go test ./internal/runner/ -run 'TestContextSourceRegistry\|TestContextSourceCollect\|TestResolveEnabledContextSourceIDs\|TestUnknownSourceIDFailsFlowLoad\|TestValidateFlowContextSources\|TestValidateFlowArtifactBindings' -count=1` |
| Golden / render-stability | Output không đổi trên fixture cũ; sections không double-render | `context_source_migration_golden_test.go`, `flow_context_package_test.go` | `TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor`, `TestRenderFlowContextPackageStableSections`, `TestFlowContextPackageCarriesRunAndStepIDs`, `TestFlowContextPackagePersistsAgainstPlanStep`, `TestFlowContextPackageLookupSurvivesRunnerRestart`, `TestFlowContextPackageDoesNotCreateParallelSessionState` | `go test ./internal/runner/ -run 'TestBuildFlowContextPackageOutputUnchanged\|TestRenderFlowContextPackageStableSections\|TestFlowContextPackageCarriesRunAndStepIDs\|TestFlowContextPackagePersistsAgainstPlanStep\|TestFlowContextPackageLookupSurvivesRunnerRestart\|TestFlowContextPackageDoesNotCreateParallelSessionState' -count=1` |
| `source.dependence` (**MỚI**, Task-259) | Priority 3 (tiebreak sau change.contract); blast-radius từ contract targets qua GitNexus; default set | `context_source_dependence_test.go` (**sẽ tạo ở Task-259**) | *(planned — Task-259 §6.2)* `TestDependenceSourceFromContractTargets`, `TestDependenceSourceNoContractDegrades`, `TestDependenceSourceInferredDirBucketYieldsEmpty`, `TestDependenceSourceDropsGlobDocAndFlagTargets`, `TestDependenceSourceGitNexusUnavailableRendersNote`, `TestDependenceSourceBoundsTargetsAndDependents`, `TestDependenceSourceDeterministicOutput`, `TestDependenceSourceIncompleteNote`, `TestDependenceInDefaultSetAndRegistered`, `TestRenderFlowContextPackageDependenceAfterContract` | `go test ./internal/runner/ -run 'TestDependenceSource\|TestDependenceInDefaultSet\|TestRenderFlowContextPackageDependence' -count=1` |

**One-shot Phase B bundle (mọi source cũ + mới):**

```bash
go test ./internal/runner/ -count=1 -timeout 8m \
  -run 'TestCanonicalHead|TestFeatureHistorySourceNo|TestBuildFlowContextPackage|TestChangeContract|TestAppendChangeContract|TestComposeFeatureBlocks|TestRenderFlowContextPackage|TestJira|TestMcpDriverSource|TestTwoMCPBackedSourcesCoexist|TestSetMCPDriverAdapter|TestGoogleDriveDriverAdapter|TestFirebaseCrashlyticsSource|TestContextSourceRegistry|TestContextSourceCollect|TestResolveEnabledContextSourceIDs|TestUnknownSourceIDFailsFlowLoad|TestValidateFlowContextSources|TestValidateFlowArtifactBindings|TestFlowContextPackage|TestDependenceSource|TestDependenceInDefaultSet'
```

## 5. GitNexus impact analysis cho context assembly — **đã chạy 2026-07-17**

**Lịch sử:** CP-50 §3 chỉ ghi *"chạy `gitnexus_impact` … nếu MCP khả dụng"* (optional). Đầu phiên 2026-07-17 index còn **stale** (commit `e9dbf74`) và MCP tool GitNexus vắng ⇒ chưa từng có evidence impact cho context-dependent code. Đã khắc phục trong phiên này:

- `npx gitnexus analyze` → index về **`6741bdc`** (up-to-date; 29,185 nodes / 61,654 edges / 300 flows).
- Chạy `npx gitnexus impact <symbol> --repo flowpilot --direction upstream` (CLI fallback, vì MCP tool không kết nối) cho 6 hàm fan-in. Kết quả:

| Symbol (file) | Risk | Direct callers (depth-1) | impacted / processes | Note |
|---|---|---|---|---|
| `ContextSourceRegistry.Collect` (`context_source_registry.go`) | **LOW** | `buildFlowContextPackage` | 5 / 0 | Mọi source đi qua đây nhưng fan-out hẹp, chỉ module Runner. Thêm source mới (Task-259) qua registry là an toàn. |
| `buildFlowContextPackage` (`flow_context_package.go`) | **LOW** | `BuildFlowContextPackage`, `BuildFlowContextPackageCtx`, `BuildFlowContextPackageWithSources` | 4 / 0 | Lớp wrapper; chỉ Runner. |
| `RenderFlowContextPackage` (`flow_context_package.go`) | **LOW** | `ComposeFlowCodingPrompt`, `ComposeFlowCodingPromptWithSecret` | 8 / 0 | Nơi P-1 chèn Head-first + skip-list. `source.dependence` render qua **generic pass** nên KHÔNG đụng skip-list → giữ LOW. Lan tới retry/handoff compose (depth 2) nhưng cùng Runner. |
| `composeFeatureBlocks` (`feature_history.go`) | **LOW** | `handoffFeatureBlocks`, `injectFeatureHistoryBody` | 6 / 0 | Chat-mode + handoff; chỉ Runner. |
| `behaviorContextProduce` (`behavior_registry_builtin.go`) | **LOW (⚠️ under-reported)** | 0 non-test (chỉ 2 test caller) | 0 / 0 | ⚠️ **Blast radius thật KHÔNG phải 0.** Hàm được gọi qua **behavior registry map (dynamic dispatch)** — static call-graph của GitNexus không bắt được (đúng ý nghĩa `DependentsSummary.Complete=false`). Trước khi sửa hàm này phải trace tay qua `registerBuiltinBehaviors`/registry, đừng tin con số 0. |
| `composeFlowNodeAgentPrompt` (`artifact_type_registry.go`) | **🔴 HIGH** | `maybeReinvokeCoderForContinue` (`interactive_service.go`), `runChildArtifactOutputGateAtEpoch` (`gate_hook.go`) | 7 / **3 processes** | Điểm P-4 append `change.contract` vào prompt step sau. Ảnh hưởng **3 execution flow**: `runFlowGateAtEpoch`, `handleSubmitFlowControl`, `runChildArtifactOutputGateAtEpoch` (2 module: Runner + Flowgate). **Cảnh báo cho Task-259:** nếu về sau muốn `source.dependence` cũng *append thẳng vào prompt* (kiểu P-4) thì đường đó là HIGH — phải cực cẩn thận. Kế hoạch Task-259 hiện tại **né** đường này (chỉ đi qua context package/generic render = LOW), nên thiết kế đang đúng. |
| `source.dependence` `Fetch` (Task-259, chưa tồn tại) | n/a | — | — | Sẽ đo lại sau khi land; dự kiến LOW vì chỉ cắm vào `Collect`/generic render (đã LOW ở trên). |

**Kết luận cho Task-259:** đường thêm source mới (`Collect` → `buildFlowContextPackage` → generic `RenderFlowContextPackage`) toàn **LOW** — an toàn. Chỉ **2 điểm phải nhớ**: (1) KHÔNG đưa `source.dependence` vào skip-list của renderer (giữ generic pass); (2) KHÔNG mở rộng sang append-prompt kiểu P-4 (`composeFlowNodeAgentPrompt` = HIGH). Riêng `behaviorContextProduce` là ngoại lệ báo-thiếu do dynamic dispatch — nếu Task-259 cần sửa nó (để wire provider), phải trace registry tay, không tin impact=0.

## 6. Live desktop E2E checklist — context (moved từ CP-51 §3.2)

Ghi ☐ khi pass. Chạy với desktop + `flowpilot serve`, project git thật.

| # | Scenario | Steps | Expect | Evidence | ☐ |
|---|----------|-------|--------|----------|---|
| B1 | Contract inject | Coding child declare scope/contract; Continue/retry | Downstream re-entry có contract inject | Prompt/log excerpt có contract | |
| B2 | Reviewer dirty tree | Reviewer turn sau coder edits | Không overwrite contract của coder | Contract owner still coder | |
| B3 | No double FCP | Flow context + feature history head | Không double-inject feature history | Rendered package / head once | |
| B4 | Source excerpt | Context sources enabled | Excerpt/bindings appear; missing source fails closed hoặc skip theo config | Context package sections | |
| B5 | Gate × contract | YOLO off + contract change mid-flow | Gate tier đúng; contract không mất sau approve | Card + re-entry prompt | |
| B6 | Canonical Head dẫn đầu (**mới**, P-1) | Feature verified có Head (`.flowpilot/canonical/<key>.json`) | Prompt/package: `## Canonical state` xuất hiện **trước** `### Change History`; không lặp 2 lần | Prompt-log index check | |
| B7 | Head-only (chưa có commit history) | Feature có Head, ledger rỗng | Chat-mode inject head-only (không rỗng hoàn toàn, không lỗi) | Prompt-log | |
| B8 | feature.history đúng khi **không có** Head (**cũ**, regression) | Feature verified, không có file canonical head | `feature.history` render y hệt hành vi trước CP-50 (prior-work slot, warning "no change history found" khi ledger rỗng) | Prompt-log so với baseline pre-CP-50 | |
| B9 | Source-excerpt runtime thật (**cũ** source / producer **mới** P-3) | Workspace có file sửa dở (uncommitted diff) + prompt nêu 1 path tường minh | Package có `### Source: <path>` cho cả file diff lẫn path nêu trong prompt; workspace không git hoặc prompt không path → behavior y hệt trước | Context package sections | |
| B10 | Jira/MCP/Firebase optional sources (**cũ**, non-default) | Flow instance khai tường minh `sources: ["jira.issue"]` (hoặc mcp.driver/firebase.crashlytics) | Section chỉ xuất hiện khi khai tường minh; không tự vào default; degrade rỗng khi adapter lỗi/timeout, không chặn turn | Context package + no-crash on adapter failure | |
| B11 | Registry/precedence không vỡ bởi turn dispatch mới (CP-51 cross-cut) | Flow cũ (pre-CP-45, không khai `sources`) chạy dưới V2 dispatch | Vẫn dùng default set đúng thứ tự priority; step-level override vẫn thắng flow-level | Package sections order | |
| B12 | **source.dependence** blast-radius (**MỚI**, Task-259) | Feature primary (GitNexus indexed) + turn Coding khai `change.contract` **declared** với **file/symbol code thật** (không dir-bucket) → step validate/audit sau. **Verify trước:** `npx gitnexus impact <file.go> --json` trả schema `{dependents/nearest/flows}` dùng được (E-schema/Q-4) | Prompt step sau có block `### source.dependence`: "Sửa `<target>` ảnh hưởng `<dependents>` + flows `<...>`"; run chưa có contract / contract inferred (dir-bucket) → không có block (rỗng có chủ đích); GitNexus vắng/stale → **note "chưa index", không lỗi** (v1 không fallback). Nếu impact chỉ nhận symbol không nhận file-path → dừng + follow-up | Prompt-log block dependence + output `npx gitnexus impact <file> --json` + `npx gitnexus status` | |

---

## 7. Links nhanh

| Doc | Role |
|-----|------|
| [CP-43](./CP-43-Change-Contract-And-Canonical-Intent-Signature.md) | Parent — Change Contract + Canonical Head |
| [CP-44](../done/CP-44-Pluggable-Context-Source-Registry.md) | Registry substrate (mọi source cắm vào) |
| [CP-45](../done/CP-45-Generic-Artifact-Types-And-Instances.md) | Artifact instances chọn source |
| [CP-50](../done/CP-50-Context-Source-Completion.md) | canonical.head / change.contract / source.excerpt producer |
| [Task-259](../../08-Task/todo/Task-259-Source-Dependence-Context-Source.md) | source.dependence (mới) |
| [CP-51 companion](../done/CP-51-PhaseAB-Timeline-And-Verification-Log.md) | Phase A + CP-51 turn dispatch (Phase B đã move sang đây) |
