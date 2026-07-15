# CP-50: Context Source Completion (canonical.head, Chat-Mode Head, source.excerpt, change.contract)

## Metadata

- Document ID: `CP-50`
- Title: `Context Source Completion (canonical.head, Chat-Mode Head, source.excerpt, change.contract)`
- Phase: `coding_plan`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: [Task-244: Canonical Head First-Class Context Source](../../08-Task/todo/Task-244-Canonical-Head-First-Class-Context-Source.md) (P-1), [Task-245: Chat-Mode Canonical-Head-First Injection](../../08-Task/todo/Task-245-Chat-Mode-Canonical-Head-First-Injection.md) (P-2), [Task-246: Source-Excerpt Runtime Hint Producers](../../08-Task/todo/Task-246-Source-Excerpt-Runtime-Hint-Producers.md) (P-3), [Task-247: Change-Contract Context Source And Downstream Prompt Injection](../../08-Task/todo/Task-247-Change-Contract-Context-Source-And-Downstream-Prompt.md) (P-4)
- Related Documents: [Task-243: Review Và Capture Các Context Artifact Source](../../08-Task/done/Task-243-Context-Artifact-Sources-Review-And-Capture.md) (nguồn gốc — mọi finding G-* và quyết định Q-* trích ở đây), [CP-43: Change Contract And Canonical Intent Signature](../inprogress/CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (P-5 sẽ được sửa wording bởi P-1 T-6), [CP-44: Pluggable Context Source Registry](../done/CP-44-Pluggable-Context-Source-Registry.md), [CP-45: Generic Artifact Types And Instances](../done/CP-45-Generic-Artifact-Types-And-Instances.md), [Task-188: Canonical-Head Packing And Admin](../../08-Task/todo/Task-188-Canonical-Head-Packing-And-Admin.md) (T-1 prepend bị supersede bởi P-1), [Task-184: Change Contract Capture](../../08-Task/done/Task-184-Change-Contract-Capture.md) (store mà P-4 mở rộng), [BUG-268](../../09-BugFix/done/BUG-268-Flow-Coding-Prompt-Duplicates-Feature-History.md) (bẫy duplicate-block phải né), [BUG-269](../../09-BugFix/done/BUG-269-CP45-Artifact-Bound-Context-Sources-Bypass-Unknown-Source-Validation.md)
- Replaces: `None`
- Tags: `context-source, canonical-head, change-contract, source-excerpt, chat-mode, registry, local-runner`

## AI Quick View

### Summary

- Hoàn thiện 4 hạng mục context-source mà owner đã chốt trong Task-243 (Q-1..Q-4, 2026-07-15): (P-1) `canonical.head` thành source đăng ký thật + vào default set; (P-2) Chat-mode prompt cũng dẫn đầu bằng Canonical Head; (P-3) `source.excerpt` có producer thật lúc runtime; (P-4) source `change.contract` để prompt các step sau mang theo phạm vi đã khai.
- Mỗi phase có **coding guide từng bước** (file, function, code skeleton, bẫy phải né) + **DOD checklist riêng** — mục tiêu: bất kỳ ai cầm file này cũng implement được mà không cần hỏi lại.
- P-1 hấp thụ luôn 3 micro-fix từ Task-243: `G-4` (warning suppression), `G-5` (heading lồng sai cấp), `G-8` (path separator).
- Toàn bộ giữ bất biến CP-41/SD-22: deterministic, không vector; mọi source degrade-mềm (AC-9), không bao giờ chặn turn/run.

### Current Ask

- Implement 4 phase theo thứ tự P-1 → P-2 → P-3 → P-4, mỗi phase một Task doc + verify riêng, không gộp thành một commit lớn.

### Key Decisions

- `P-1` `canonical.head` là source **first-class, priority 1, nằm trong `defaultContextSourceIDs`** (owner chốt Task-243 Q-1). Head **tách khỏi** prepend trong `feature.history` — không giữ song song (né duplicate kiểu BUG-268).
- `P-2` Chat-mode (`composeFeatureBlocks`) prepend Head; feature có Head nhưng chưa có commit history vẫn inject **head-only** (đây là behavior change có chủ đích).
- `P-3` Producer cho `source.excerpt` đặt ở `behaviorContextProduce` (caller), **không** đặt trong `buildFlowContextPackage` — để ~30 test caller trực tiếp và golden test không bị ảnh hưởng.
- `P-4` `RenderContractBlock` trả body **không có heading** — caller tự thêm heading (generic render đã tự thêm `### change.contract`; chỗ append trực tiếp tự thêm `##`). Contract lấy theo **run** (`GetLatestForRun`), không theo step.
- Chung: không sửa struct `FlowContextPackage`, không thêm bảng DB, không thêm dependency mới — đúng contract CP-44 P-3/DOD-3.

### Constraints

- `TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor` (golden, `context_source_migration_golden_test.go`) phải pass **không sửa expectation** — fixture không có Head/Contract nên section mới rỗng → render không đổi. Nếu phải sửa golden tức là làm sai.
- Không đụng `flow_executor.go`'s filter list (`filterString(sourceIDs, ...)`) cho source mới — `canonical.head`/`change.contract` là source collect thật, KHÔNG phải runtime-target-note như mcp.driver/jira/firebase.
- Mọi lỗi đọc Head/Contract/git degrade thành section rỗng hoặc bỏ qua — không error, không warning ồn (SS-14 AC-9).
- `apps/admin-web` đã deprecated — UI chỉ sửa `apps/desktop-flowpilot`.
- Không skip `verify`: mỗi phase chạy đúng test battery ghi ở §7 + build/vet sạch.

### Open Questions

- `Q-1` P-3 T-2: dùng `git diff --name-only HEAD` có bao gồm cả staged; có cần cả untracked (`git ls-files --others --exclude-standard`) không? Default plan: CÓ cả hai, cap tổng 20 path. Implementer có thể thu hẹp nếu package phình to trong thực tế.
- `Q-2` P-4 T-3: điểm neo inject contract vào prompt của node sau — plan chỉ định `composeFlowNodeAgentPrompt`; nếu lúc implement phát hiện validate/audit không đi qua hàm này (đường BUG-243 F-0 inline dispatch), neo bổ sung tại chỗ dispatch đó và ghi rõ vào Task doc.

### Source Refs

- Task-243 `G-1`..`G-8`, `Q-1`..`Q-4`. `SD-21` D-3 (Head-first), §6. `SD-22` D-2/D-3/D-4/D-5. `SS-14` US-3, AC-3, AC-8, AC-9. `CP-43` §3, §4.5, P-5. `CP-44` P-6/P-8/DOD-3/DOD-7.

## 1. Goal

Sau CP-50: (1) Canonical Head là một context source độc lập, chọn được qua CP-45 artifact instance, có mặt mặc định trong mọi package và luôn render **trước** raw history; (2) prompt Chat mode (và cross-provider handoff) cũng dẫn đầu bằng Head; (3) `source.excerpt` thực sự tạo excerpt lúc runtime từ diff chưa commit + path nêu trong prompt; (4) các step sau step khai báo (validate/audit/reprompt) nhìn thấy Change Contract đã khai ngay trong prompt. Không đổi contract `FlowContextPackage`, không vector, mọi đường degrade-mềm.

## 2. Input Documents

- `requirements/06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md` (D-3, §5, §6)
- `requirements/06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md`
- `requirements/05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md` (US-3, AC-8, AC-9)
- `requirements/08-Task/done/Task-243-Context-Artifact-Sources-Review-And-Capture.md` (bản đồ kiến trúc + findings — đọc trước khi code)
- Code vào cuộc: `apps/local-runner/internal/runner/{context_source_registry,context_sources_builtin,context_source_canonical_head_test,flow_context_package,feature_history,handoff_context,behavior_registry_builtin,flow_executor}.go`, `apps/local-runner/internal/changecontract/{head,pack,contract}.go`, `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`

## 3. Implementation Strategy

- overall approach: mỗi phase là một Task doc + commit riêng, behavior-preserving với fixture cũ (source mới rỗng khi không có data). Code skeleton trong §4 là bản đã được rehearse trên codebase này (2026-07-15) — các anchor/function/line đều có thật, cứ bám theo.
- sequencing logic: `P-1` trước (định nghĩa source + render Head-first — nền của P-2/P-4). `P-2` ngay sau (nhỏ, tái dùng `RenderHeadBlock`). `P-3` độc lập, làm lúc nào cũng được. `P-4` cuối (cần store API mới + điểm neo prompt).
- dependencies: không có dependency ngoài; toàn bộ module đã tồn tại (`changecontract` từ Task-184/186, registry từ Task-191/192). GitNexus impact-analysis: chạy `gitnexus_impact` cho `RenderFlowContextPackage`, `composeFeatureBlocks`, `behaviorContextProduce`, `Fetch` (featureHistorySource) trước khi sửa nếu MCP khả dụng.

## 4. Work Breakdown

### 4.1 `P-1` — `canonical.head` thành first-class source, vào default set — [Task-244](../../08-Task/todo/Task-244-Canonical-Head-First-Class-Context-Source.md)

**Kết quả:** Head tách khỏi `feature.history`, tự là source id `canonical.head` (priority 1), có trong default set, render dẫn đầu package. Sửa xong thì `G-1` (nửa canonical.head), `G-4`, `G-5`, `G-8` của Task-243 đóng.

**`T-1` Tạo file mới `apps/local-runner/internal/runner/context_source_canonical_head.go`:**

```go
package runner

import (
    "context"
    "path/filepath"
    "strings"

    "flowpilot-runner/internal/changecontract"
)

// ContextSourceCanonicalHead — Task-244 (CP-50 P-1, CP-43 P-5, CP-44 DOD-7).
const ContextSourceCanonicalHead ContextSourceID = "canonical.head"

type canonicalHeadSource struct{ priority int }

func (s *canonicalHeadSource) ID() string          { return string(ContextSourceCanonicalHead) }
func (s *canonicalHeadSource) Priority() int       { return s.priority }
func (s *canonicalHeadSource) Deterministic() bool { return true }

func (s *canonicalHeadSource) Fetch(_ context.Context, hints FlowContextHints) (FlowContextSection, error) {
    section := FlowContextSection{SourceType: s.ID(), Priority: s.priority}
    // Gate giống featureHistorySource: chỉ tra khi feature verified —
    // hint không chắc chắn không bao giờ được inject Head của feature khác.
    if hints.FeatureConfidence != ConfidenceVerified || hints.FeatureKey == "" {
        return section, nil
    }
    head, found, err := changecontract.LoadHead(hints.Workspace, hints.FeatureKey)
    if err != nil || !found {
        return section, nil // degrade AC-9: không error, không warning
    }
    // G-8 fix: dùng filepath.ToSlash cho SourceRef nhất quán trên Windows.
    section.SourceRef = filepath.ToSlash(filepath.Join(hints.Workspace, ".flowpilot", "canonical", hints.FeatureKey+".json"))
    section.Body = strings.TrimSpace(changecontract.RenderHeadBlock(head))
    return section, nil
}
```

**`T-2` Sửa `context_sources_builtin.go` (3 chỗ):**
1. `defaultContextSourceIDs`: thêm `string(ContextSourceCanonicalHead)` lên **đầu** slice (thứ tự slice không quyết định packing — `Collect` sort theo priority — nhưng để đầu cho dễ đọc). Cập nhật comment: 3 id gốc reproduce CP-41; canonical.head vào default set theo Task-243 Q-1.
2. `registerBuiltinContextSources`: thêm dòng `mustRegisterContextSource(r, &canonicalHeadSource{priority: 1})` trước dòng featureHistory (priority 2).
3. `featureHistorySource.Fetch`: **xóa nguyên block** prepend Head (block comment "Task-188 (CP-43 P-5, SD-21 D-3)..." + `if head, found, err := changecontract.LoadHead(...)`). Sau đó xóa import `flowpilot-runner/internal/changecontract` (không còn ai trong file dùng — verify bằng `go build`). Việc xóa này tự khôi phục semantics warning "no change history found" (đóng `G-4`).

**`T-3` Sửa renderer `flow_context_package.go` (2 chỗ):**
1. Trong `RenderFlowContextPackage`, ngay **sau** dòng `sb.WriteString("- **No vector retrieval used**\n")` và **trước** block `if pkg.HistoryBlock != ""`:
```go
// CP-50 P-1 (SD-21 D-3): Canonical Head dẫn đầu — current truth + rejected
// dead-ends trước mọi raw history. Body tự mang heading "## Canonical state"
// (sibling của history theo template CP-43 §4.5 — đóng G-5), viết verbatim
// ở đây và skip ở renderGenericSections bên dưới.
for _, s := range pkg.Sections {
    if ContextSourceID(s.SourceType) == ContextSourceCanonicalHead && strings.TrimSpace(s.Body) != "" {
        sb.WriteString("\n" + strings.TrimSpace(s.Body) + "\n")
    }
}
```
2. Trong `renderGenericSections`, thêm `ContextSourceCanonicalHead` vào case skip: `case ContextSourceFeatureHistory, ContextSourceChatSummary, ContextSourceSourceExcerpt, ContextSourceCanonicalHead:` — nếu quên, Head render 2 lần (bẫy BUG-268).

**`T-4` Viết lại test `context_source_canonical_head_test.go`:** file hiện có 2 test của Task-188. `TestFeatureHistorySourcePrependsCanonicalHead` phải **thay** (assert điều ngược lại: feature.history KHÔNG còn chứa "Canonical state"); `TestFeatureHistorySourceNoHeadFallsBackToPriorBehavior` giữ nguyên (vẫn đúng). Test mới:
- `TestCanonicalHeadSourceFetchesHeadBlock` — SaveHead vào temp dir → Fetch → Body chứa `Canonical state of` + decision bị reject; SourceRef khác rỗng.
- `TestCanonicalHeadSourceNoHeadDegrades` — không có head → Body rỗng, Warnings nil, err nil.
- `TestCanonicalHeadSourceUnverifiedFeatureEmpty` — `ConfidenceLow` → section rỗng.
- `TestCanonicalHeadInDefaultSetAndRegistered` — `defaultContextSourceIDs` chứa id; `NewDefaultContextSourceRegistry().Resolve("canonical.head")` ok.
- `TestFeatureHistorySourceNoLongerPrependsHead` — có Head + ledger rỗng → feature.history Body rỗng VÀ warning "no change history found" fire (G-4 khôi phục).
- `TestRenderFlowContextPackageLeadsWithCanonicalHead` — dùng helper `fcpFixture(t)` (có sẵn trong package test), SaveHead cho feature `agent-flow-engine` vào workspace fixture → `BuildFlowContextPackage` → render: `strings.Index(rendered, "## Canonical state")` phải `>= 0` và `<` index của `"### Change History"`.

**`T-5` Desktop UI `WorkflowsSettings.tsx`:** thêm `{ id: "canonical.head", label: "Canonical Head" }` vào đầu mảng `contextSourceOptions` (dòng ~68). Chạy `npx tsc --noEmit` trong `apps/desktop-flowpilot`. Nhớ: descriptor này sync tay với Go registry (comment ngay trên mảng đã ghi rõ).

**`T-6` Sync docs:** CP-43 §4.5 (đổi wording "feature.history slot prepends" → "canonical.head là source riêng, priority 1, default set — Task-244"); CP-43 §10 P-5 thêm note; Task-188 §6 T-1 thêm dòng "superseded by CP-50 P-1 (Task-2xx): prepend chuyển thành source canonical.head"; CP-44 §4 P-1 ví dụ interface đã nhắc `"canonical.head" (CP-43)` — thêm note "(landed via CP-50 P-1)".

**Bẫy phải né (đọc kỹ trước khi code):**
- Golden test: fixture không có Head → section canonical.head Body rỗng → render y hệt. **Không** được emit warning/SourceRef khi thiếu Head, nếu không golden fail.
- KHÔNG thêm `canonical.head` vào chuỗi `filterString(...)` trong `startInlineEntryChain` (`flow_executor.go:353-356`) — chuỗi đó chỉ dành cho runtime-target-note sources.
- `contextsync.EngineStore.SharedFiles()` đã glob `canonical/*.json` (Task-188 T-4) — không cần đụng.
- Các test kiểu "X must not be in defaultContextSourceIDs" chỉ áp cho jira/mcp/firebase — không có test nào cấm thêm id mới vào default set.

**DOD `P-1`:**
- [ ] `canonical.head` đăng ký priority 1, có trong `defaultContextSourceIDs`; `Resolve` ok; CP-45 artifact instance khai `sources: ["canonical.head"]` chạy được (validate pass).
- [ ] `feature.history` không còn prepend Head; warning "no change history found" fire lại khi ledger rỗng kể cả khi có Head.
- [ ] Render: block `## Canonical state` đứng trước `### Change History`; không xuất hiện lần 2 ở generic pass.
- [ ] `TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor` pass **không sửa expectation**.
- [ ] 6 test mới ở T-4 pass; `go build ./...`, `go vet ./internal/runner/...` sạch; `npx tsc --noEmit` sạch.
- [ ] Docs T-6 đã sửa; Task doc mới (status done) link CP-50 P-1.

### 4.2 `P-2` — Chat-mode prompt dẫn đầu bằng Canonical Head — [Task-245](../../08-Task/todo/Task-245-Chat-Mode-Canonical-Head-First-Injection.md)

**Kết quả:** `composeFeatureBlocks` (dùng bởi per-turn injection Chat mode + cross-provider handoff) trả `head + history + discussion`. Đóng `G-2`.

**`T-1` Sửa `feature_history.go` — thay nguyên hàm `composeFeatureBlocks`:**

```go
func composeFeatureBlocks(dotFlowpilotDir string, featureKey string) string {
    ledger, err := changeledger.New(dotFlowpilotDir)
    if err != nil {
        return ""
    }
    history := strings.TrimSpace(featurecatalog.HistorySlot(featureKey, ledger))

    // CP-50 P-2 (Task-243 Q-2): Chat-mode cũng dẫn đầu bằng Canonical Head.
    // Head thiếu/lỗi → không có block (AC-9). Feature có Head nhưng chưa có
    // commit history vẫn inject head-only (behavior change có chủ đích).
    headBlock := ""
    if head, found, herr := changecontract.LoadHead(filepath.Dir(dotFlowpilotDir), featureKey); herr == nil && found {
        headBlock = strings.TrimSpace(changecontract.RenderHeadBlock(head))
    }
    if history == "" && headBlock == "" {
        return ""
    }
    var parts []string
    if headBlock != "" {
        parts = append(parts, headBlock)
    }
    if history != "" {
        parts = append(parts, history)
    }
    if summaryLedger, err := changeledger.NewChatSummaryLedger(dotFlowpilotDir); err == nil {
        if discussion := featurecatalog.ChatSummarySlot(featureKey, summaryLedger); strings.TrimSpace(discussion) != "" {
            parts = append(parts, discussion)
        }
    }
    return strings.Join(parts, "\n\n")
}
```
Thêm import `"flowpilot-runner/internal/changecontract"`. Cập nhật doc comment của hàm (nói rõ head-only case). Caller thứ hai `handoff_context.go:239` hưởng tự động — không sửa gì.

**`T-2` Tests** (đặt cạnh test hiện có của feature_history; nếu chưa có file test riêng thì tạo `feature_history_canonical_head_test.go`):
- `TestComposeFeatureBlocksLeadsWithCanonicalHead` — ledger có entry + SaveHead → block bắt đầu bằng `## Canonical state`, history theo sau.
- `TestComposeFeatureBlocksHeadOnlyWhenNoHistory` — chỉ có Head → trả head-only (khác "" như trước).
- `TestComposeFeatureBlocksEmptyWhenNoHeadNoHistory` — giữ contract cũ.

**Bẫy phải né:**
- KHÔNG sửa các điều kiện skip trong `injectFeatureHistoryPrompt` (`isHandoffPrompt`/`isFlowContextHandoff`/`isFlowEnginePrompt`/`isFlowReviewHandoffPrompt`) — chính chúng ngăn Head bị inject đè lên Flow package đã có Head từ P-1 (bài học BUG-268/BUG-277).
- `changeledger.New` lỗi → vẫn return "" sớm (giữ nguyên) — chấp nhận mất head-only trong case hiếm này, không đổi error handling.
- `LoadHead` nhận **workspace**, hàm này nhận **dotFlowpilotDir** → phải `filepath.Dir(dotFlowpilotDir)`.

**DOD `P-2`:**
- [ ] Prompt Chat mode cho feature verified có Head → dẫn đầu bằng `## Canonical state`, sau đó `## Prior work`, sau đó discussion.
- [ ] Feature có Head, chưa có history → inject head-only (có test).
- [ ] Flow-mode prompt không bị double-Head (skip conditions giữ nguyên — verify bằng test hiện có `injectFeatureHistoryPrompt` + chạy lại toàn bộ `go test ./internal/runner/`).
- [ ] 3 test T-2 pass; build/vet sạch; Task doc link CP-50 P-2.

### 4.3 `P-3` — Producer thật cho `source.excerpt` lúc runtime — [Task-246](../../08-Task/todo/Task-246-Source-Excerpt-Runtime-Hint-Producers.md)

**Kết quả:** `behaviorContextProduce` tự derive `ChangedPaths` (diff chưa commit) + `ExplicitSourcePaths` (path nêu trong prompt), nên package live có excerpt thật. Đóng `G-3`.

**`T-1` File mới `apps/local-runner/internal/runner/flow_context_hint_paths.go` — 2 helper thuần:**
1. `extractPromptSourcePaths(prompt string) []string` — deterministic, không AI:
   - Tách token theo whitespace; lột dấu bọc `` ` ``, `'`, `"`, `(`, `)`, `,`.
   - Token hợp lệ khi: chứa `/` hoặc `\`, KHÔNG bắt đầu `http://`/`https://`, có phần mở rộng file (`filepath.Ext != ""`), không match `flowgate.IsDocOrAuditFile`.
   - Chuẩn hóa `\` → `/`; dedup giữ thứ tự (`fcpDedup` có sẵn); cap 8 token đầu.
2. `uncommittedChangedPaths(workspace string) []string` — chạy git bounded:
   - `git -C <workspace> diff --name-only HEAD` và `git -C <workspace> ls-files --others --exclude-standard` (CP-50 Q-1: lấy cả hai), mỗi lệnh timeout 3s (`exec.CommandContext`).
   - Bất kỳ lỗi nào (không phải git repo, git thiếu, timeout) → return nil, không log ồn (một dòng `log.Printf` debug là đủ).
   - Lọc `flowgate.IsDocOrAuditFile`; cap 20 path.
   - Trước khi viết mới, grep `exec.Command` trong `internal/flowgate/observe.go` — nếu đã có helper diff export được thì tái dùng thay vì exec mới.

**`T-2` Sửa `behaviorContextProduce` (`behavior_registry_builtin.go:86`):** sau khi dựng `hints`, thêm:
```go
if in.WorkspaceCwd != "" {
    hints.ExplicitSourcePaths = extractPromptSourcePaths(in.Prompt)
    hints.ChangedPaths = uncommittedChangedPaths(in.WorkspaceCwd)
}
```
KHÔNG đụng `buildFlowContextPackage` — mọi caller trực tiếp (test/golden) giữ nguyên hành vi.

**`T-3` Tests:**
- Unit extraction: prompt có path backtick-quoted, path Windows-style, URL (phải loại), token không có ext (loại), file .md (loại), >8 path (cap).
- Unit git helper: `t.TempDir()` + `git init` + commit 1 file + sửa nó + thêm file untracked → trả đúng 2 path; dir không phải git repo → nil.
- Integration: `behaviorContextProduce` với workspace là git repo có file sửa dở + prompt nêu 1 path → `pkg.SourceExcerpts` chứa excerpt của các file đó; workspace temp thường (không git, prompt không path) → package y như trước (bảo vệ mọi behavior-test hiện có).

**Bẫy phải né:**
- Fixture/golden an toàn vì temp dir không phải git repo và prompt fixture không chứa path-token — nhưng vẫn PHẢI chạy full `go test ./internal/runner/ -count=1` để chắc không behavior-test nào dựng git repo trong workspace.
- `readSourceExcerpts` đã guard outside-workspace/symlink/binary/cap — đừng guard lại lần hai trong helper.
- Git exec ở Plan-time thêm latency: bounded 2 lệnh × 3s worst-case; ghi nhận vào §8 monitoring.

**DOD `P-3`:**
- [ ] Run flow live trên workspace có file sửa dở → package có `### Source: <path>` excerpt tương ứng (manual check trong prompt-log, kiểu CP-44 §11.1).
- [ ] Prompt nêu path tường minh → file đó vào excerpt (nếu trong workspace).
- [ ] Workspace không phải git / prompt không path → hành vi y hệt trước (không warning mới, golden pass).
- [ ] Unit + integration tests T-3 pass; full `go test ./internal/runner/ -count=1` không regression mới; build/vet sạch; Task doc link CP-50 P-3.

### 4.4 `P-4` — Source `change.contract`: step sau nhìn thấy phạm vi đã khai — [Task-247](../../08-Task/todo/Task-247-Change-Contract-Context-Source-And-Downstream-Prompt.md)

**Kết quả:** Contract của run xuất hiện trong package (khi rebuild) và được append vào prompt các node sau node khai báo. Đóng nốt `G-1` (nửa change.contract) — câu hứa CP-43 §3 thành sự thật.

**Lưu ý timing (quan trọng, đọc trước):** Contract được capture **sau** turn code đầu tiên (gate hook parse final message — Task-184). Package build ở Plan-time của run mới ⇒ section này **rỗng ở lần build đầu** — đúng thiết kế. Giá trị nằm ở: (a) Plan-step rerun / continue-round rebuild package (CH-4), (b) T-3 append trực tiếp vào prompt validate/audit — luôn có data vì chạy sau Coding.

**`T-1` `internal/changecontract` — 2 bổ sung:**
1. `contract.go` — `func (s *Store) GetLatestForRun(runID string) (Contract, bool)`: scan `contracts.ndjson` tuần tự (file append-only, last-wins), giữ entry **cuối cùng** có `run_id == runID` bất kể step; mutex như `Get`. Test: 2 step cùng run → trả step sau; run lạ → false.
2. `pack.go` — `func RenderContractBlock(c Contract) string`: trả body **không heading** (Key Decision P-4):
```
Feature: <feature_key>
Intent: <intent>                     ← bỏ dòng nếu rỗng
Declared scope (KHÔNG sửa ngoài các path này):
- <declared_paths từng dòng>
Confidence: declared | inferred      ← inferred thì thêm "(suy ra từ diff, chưa được AI xác nhận)"
```
Zero-value Contract (FeatureKey + DeclaredPaths đều rỗng) → return `""`. Test reproducibility + zero-value.

**`T-2` File mới `apps/local-runner/internal/runner/context_source_change_contract.go`:** mirror `canonicalHeadSource` (P-1 T-1): id `change.contract`, `priority: 3` (sau head 1/history 2, trước excerpt 4), `Deterministic() = true`. `Fetch`: `hints.WorkflowRunID == ""` → section rỗng; mở store `changecontract.NewStore(filepath.Join(hints.Workspace, ".flowpilot"))` (dùng đúng constructor Task-184 — check tên thật trong `contract.go` trước khi gọi), `GetLatestForRun` → không có → section rỗng; có → `SourceRef = filepath.ToSlash(...contracts.ndjson)`, `Body = changecontract.RenderContractBlock(c)`. Đăng ký trong `registerBuiltinContextSources` + thêm vào `defaultContextSourceIDs` (owner chốt Q-4: prompt phải mang contract). Render đi qua **generic pass** (`### change.contract` + `_Source:_` tự có) — KHÔNG thêm case đặc biệt trong renderer.

**`T-3` Append contract vào prompt node sau:** neo tại `composeFlowNodeAgentPrompt` (`flow_executor.go` — hàm compose prompt cho delegate node, đã được cả `startInlineEntryChain` dùng):
- Thêm bước: nếu `GetLatestForRun(parentRunID)` có contract và prompt **chưa chứa** marker `"Declared scope (KHÔNG sửa ngoài"` (guard chống double-append) → append:
```
## Change Contract đã khai cho run này
<RenderContractBlock(c)>
```
- Nếu lúc implement thấy validate/audit KHÔNG đi qua `composeFlowNodeAgentPrompt` (đường inline dispatch BUG-243 F-0) → neo bổ sung tại chỗ dispatch validate/audit, ghi rõ anchor thật vào Task doc (CP-50 Q-2).
- Node entry (context.produce) không append (chưa có contract, và không phải AI node).

**`T-4` UI + docs:** thêm `{ id: "change.contract", label: "Change Contract" }` vào `contextSourceOptions`; CP-43 §3 thêm note "(landed via CP-50 P-4)".

**Bẫy phải né:**
- Store đọc phải mutex-safe và **không tạo file/dir mới** khi chỉ đọc (workspace có thể chưa từng có contract) — mọi lỗi mở store → section rỗng.
- Contract `confidence=inferred` vẫn render (kèm chú thích) — đừng lọc bỏ, vì phần lớn turn thực tế là inferred (Task-184 F-3).
- Golden/fixture: không có contracts.ndjson → section rỗng → pass không sửa.
- Đừng quên guard double-append ở T-3 — reprompt/retry đi qua compose nhiều lần.

**DOD `P-4`:**
- [ ] `change.contract` đăng ký priority 3, trong default set, CP-45 instance chọn được; validate pass.
- [ ] Run flow live: prompt của validate/audit (sau Coding) chứa block Change Contract với declared paths của turn Coding (manual check prompt-log).
- [ ] Plan-step rerun trên run đã có contract → package rebuild có section `### change.contract`.
- [ ] Run mới chưa có contract → section rỗng, render không đổi, golden pass.
- [ ] Không double-append khi retry/reprompt (có test hoặc manual check 2 vòng reprompt).
- [ ] `GetLatestForRun` + `RenderContractBlock` có unit test; build/vet sạch; Task doc link CP-50 P-4.

## 5. Touched Areas

- files (mới): `internal/runner/context_source_canonical_head.go` (P-1), `internal/runner/flow_context_hint_paths.go` (P-3), `internal/runner/context_source_change_contract.go` (P-4)
- files (sửa): `internal/runner/context_sources_builtin.go` (P-1), `internal/runner/flow_context_package.go` (P-1), `internal/runner/feature_history.go` (P-2), `internal/runner/behavior_registry_builtin.go` (P-3), `internal/runner/flow_executor.go` (P-4 T-3), `internal/changecontract/{contract,pack}.go` (P-4), `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (P-1/P-4)
- modules: runner context assembly, changecontract, desktop settings UI
- database: không
- external systems: git CLI (P-3, bounded exec, degrade khi vắng)

## 6. Data or Migration Steps

- schema: không.
- data backfill: không — Head/Contract đọc từ store local có sẵn (`.flowpilot/canonical/`, `.flowpilot/contracts/`).
- config updates: không — default set đổi trong code; flow/instance cũ khai sources tường minh không tự nhận source mới (đúng kỳ vọng: instance config là allowlist).

## 7. Validation Plan

- tests to add: liệt kê per-phase ở §4 (T-4 của P-1, T-2 của P-2, T-3 của P-3, T-1/T-2 của P-4).
- test battery mỗi phase (bắt buộc trước khi đóng Task):
  - `go build ./...` && `go vet ./internal/runner/... ./internal/changecontract/...`
  - `go test ./internal/changecontract/... -count=1`
  - `go test ./internal/runner/ -run 'ContextSource|FlowContextPackage|CanonicalHead|RenderFlowContextPackage|FeatureHistorySource|ComposeFeatureBlocks|ChangeContract' -count=1`
  - `go test ./internal/runner/ -count=1` (full; so với baseline flake đã biết — 16 fail môi trường tính đến 2026-07-13, không được có fail MỚI)
  - `npx tsc --noEmit` trong `apps/desktop-flowpilot` khi đụng UI
- manual checks (kiểu CP-44 §11, sandbox `D:\working\gate-sandbox`): (1) run `rag-harness` trên feature có Head → prompt-log dẫn đầu `## Canonical state` trước `### Change History`; (2) Chat-mode turn thường trên cùng feature → prompt inject dẫn đầu Head; (3) workspace có file sửa dở → package có Source excerpt; (4) sau turn Coding, prompt validate/audit có block Change Contract.
- failure cases: Head file hỏng/JSON lỗi → section rỗng, run vẫn chạy; workspace không git → P-3 im lặng; contracts.ndjson thiếu → P-4 rỗng; feature unverified → cả head/contract không inject.

## 8. Rollout and Fallback

- rollout order: P-1 → P-2 → P-3 → P-4, mỗi phase một Task + commit riêng, verify xong mới sang phase sau.
- fallback path: mỗi phase revert độc lập bằng git; P-1 revert ⇒ hành vi prepend cũ vẫn còn ở lịch sử (không mất Head khỏi prompt vĩnh viễn); source mới rỗng khi thiếu data nên không có trạng thái trung gian nguy hiểm.
- monitoring: log thời gian fetch per-source đã có ở registry path (CP-44 §8); P-3 theo dõi thêm thời lượng git exec (log 1 dòng khi >1s); quan sát prompt_len trong prompt-log trước/sau (đề phòng prompt bloat).

## 9. Risks

- `R-1` Golden/regression vỡ do source mới — Mitigation: section rỗng khi không có data; cấm sửa golden expectation (Constraint); full test battery mỗi phase.
- `R-2` Duplicate block kiểu BUG-268 (Head/Contract xuất hiện 2 lần trong một prompt) — Mitigation: P-1 skip ở generic pass; P-2 giữ nguyên skip-conditions; P-4 marker guard; manual check prompt-log là DOD bắt buộc.
- `R-3` Prompt bloat (thêm Head + Contract + excerpts vào mọi prompt) — Mitigation: mọi block đều bounded (Head ngắn theo thiết kế; excerpt cap 4KB/16KB; contract chỉ vài dòng); theo dõi prompt_len ở §8.
- `R-4` P-3 git exec treo/chậm trên repo lớn — Mitigation: timeout 3s/lệnh, cap 20 path, degrade nil.
- `R-5` P-4 contract sai step (lấy contract của step khác trong run) — Mitigation: v1 chấp nhận theo-run (GetLatestForRun) — nhất quán với mô hình "một feature/một mạch việc mỗi run"; nếu thực tế cần per-step, mở Task riêng, không vá nóng.
- `R-6` Doc drift lặp lại (đúng cái Task-243 vừa bắt) — Mitigation: T-6 (P-1) và T-4 (P-4) sửa CP-43/CP-44/Task-188 là DOD item, không phải "nice to have".

## 10. Definition of Done

- [ ] `P-1` xong toàn bộ DOD §4.1 — canonical.head first-class, default set, Head-first render, golden pass nguyên trạng, docs sync.
- [ ] `P-2` xong toàn bộ DOD §4.2 — Chat-mode + handoff dẫn đầu bằng Head, head-only case có test, không double-Head.
- [ ] `P-3` xong toàn bộ DOD §4.3 — excerpt thật từ diff + prompt paths, degrade sạch khi không git/không path.
- [ ] `P-4` xong toàn bộ DOD §4.4 — change.contract source + append vào prompt step sau, không double-append.
- [ ] Mỗi phase có Task doc riêng (format chuẩn, link CP-50 P-x) + change-audit note (`audit-logging` skill) cho code change.
- [ ] Task-243 §8 follow-ups cập nhật trạng thái khi từng phase land.
- [ ] Sau P-4: chạy lại 4 manual checks §7 trong một run live duy nhất và ghi kết quả (run id, prompt-log path) vào Task doc của P-4 — đây là bằng chứng đóng CP.
