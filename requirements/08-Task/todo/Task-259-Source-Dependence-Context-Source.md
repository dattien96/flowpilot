# Task-259: `source.dependence` Context Source (GitNexus Blast-Radius From Change Contract)

## Metadata

- Document ID: `Task-259`
- Title: `source.dependence Context Source (GitNexus Blast-Radius From Change Contract)`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-17`
- Last Updated: `2026-07-17` (readiness review — added edge-case handling, code guide, test-signature guide, full DoD; priority decision finalized to int `3` via SourceType tiebreak; v1 = GitNexus-only)
- Parent Documents: [CP-43: Change Contract And Canonical Intent Signature](../../07-Coding-Plan/inprogress/CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (P-6, mới), [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (US-3, AC-7, AC-9)
- Child Documents: none
- Related Documents: [CP-43-CATALOG: Context Source Catalog And Test Log](../../07-Coding-Plan/inprogress/CP-43-Context-Source-Catalog-And-Test-Log.md) (§3 mô tả source; §4.1 hàng test; §6 B12 e2e), [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md), [CP-50: Context Source Completion](../../07-Coding-Plan/done/CP-50-Context-Source-Completion.md), [Task-247: Change-Contract Context Source](../done/Task-247-Change-Contract-Context-Source-And-Downstream-Prompt.md), [Task-244: Canonical Head First-Class Context Source](../done/Task-244-Canonical-Head-First-Class-Context-Source.md)
- Replaces: `None`
- Tags: `context-source, gitnexus, change-contract, dependence, blast-radius, structure, default-set`

## AI Quick View

### Summary

- Thêm context source thứ 10 `source.dependence` vào **default set**: khi run đã có `change.contract` khai `declared_paths`/`declared_symbols`, source này chạy GitNexus impact trên từng **target đã chuẩn hoá** và render **blast-radius** ("đổi cái này ảnh hưởng cái nào") cho step sau (validate/audit/review) đọc.
- Tái dùng module có sẵn `internal/structure` (`Provider.Dependents` → `npx gitnexus impact <target> --json`). Không thêm dependency, không thêm bảng DB, **không thêm field `FlowContextHints`** (đọc `hints.Workspace`/`hints.WorkflowRunID` + `tooling.json` cache — giữ blast-radius LOW per CP-43-CATALOG §5).
- Deterministic, không AI ở collect (tool/graph), output **sorted** để reproducible. Degrade-mềm AC-9: không contract / target rỗng sau chuẩn hoá / GitNexus vắng / lỗi → **section zero-value** (Body rỗng, không warning, không omitted) ⇒ golden fixtures không đổi.
- Tiêu thụ `change.contract` ⇒ xếp **sau** `change.contract`, **trước** `source.excerpt`. Chốt: **priority = `3`** (bằng `change.contract`) và dựa vào tiebreak SourceType của stable-sort (`change.contract` < `source.dependence` < `source.excerpt`) — **không phải shift lại** như draft cũ (xem `T-2`).

### Current Ask

- Implement source `source.dependence` + đăng ký vào registry + đưa vào `defaultContextSourceIDs`; **chuẩn hoá target** (symbol trước, code-file cụ thể; loại dir-bucket/glob/doc); render qua generic pass; bounded (cap target + cap dependents + **shared wall-clock budget**); unit + integration test; UI descriptor + docs sync (CP-43 gốc + CP-43-CATALOG).

### Key Decisions

- `T-1` Input **chỉ** từ `change.contract` của run (`changecontract.OpenStoreReadOnly` → `Store.GetLatestForRun`) — không tự suy diễn target từ diff (đó là việc của `source.excerpt`). Không contract → section zero-value.
- `T-2` **Priority = `3` (FINAL).** Đăng ký giữa `change.contract` và `source.excerpt`. Vì `Collect` (context_source_registry.go) **và** `sectionsForRender` (flow_context_package.go) đều `sort.SliceStable` theo `(Priority asc, SourceType asc)`, cho `source.dependence` cùng priority `3` với `change.contract` là ĐỦ: tiebreak chuỗi cho `"change.contract"(c)` < `"source.dependence"(s)` < `"source.excerpt"(s…e)`. **Không cần** shift `source.excerpt`/`chat.summary`/mcp/jira/firebase (draft cũ đề xuất shift +1 — bỏ, vì đụng nhiều hằng số + assertion int trong test mà không cần thiết). Chỉ cần: (a) `mustRegisterContextSource(&dependenceSource{priority: 3})`, (b) thêm id vào `defaultContextSourceIDs` sau `change.contract`, (c) thêm `case ContextSourceDependence: sections[i].Priority = 3` vào fill-switch của `sectionsForRender` (phòng payload cũ Priority==0).
- `T-3` **Target normalization (mới — quyết định cốt lõi).** `DeclaredPaths` trong thực tế **không phải symbol**: contract *inferred* (phổ biến, vì symbol-level chưa bao giờ được build — xem `scope.go`) đặt `DeclaredPaths` = **top-level dir bucket** (`"apps"`, `"internal"`, `"go.mod"` — xem `infer.go` `topLevelDir`); contract *declared* liệt kê file path/glob. Nhưng `gitnexus impact <target>` là **symbol/file-oriented** (CP-43-CATALOG §5 chạy trên tên hàm). ⇒ Phải chuẩn hoá trước khi query: **(1)** `DeclaredSymbols` trước (đúng đối tượng của impact); **(2)** `DeclaredPaths` chỉ giữ **file code cụ thể** (có đuôi code, chứa `/` hoặc là root code-file); **loại** dir-bucket (`"apps"`), glob (`*?[`), doc/audit (`flowgate.IsDocOrAuditFile`), và target bắt đầu bằng `-` (chống bị hiểu là CLI flag). Contract inferred (chỉ có dir-bucket) ⇒ target rỗng ⇒ section rỗng **có chủ đích** — source này chỉ hữu ích khi có contract **declared** liệt kê file/symbol thật.
- `T-4` **Bounded execution (mới).** `structure.gitNexusProvider.Dependents` spawn `npx gitnexus impact` (cold-start ~1–2s) và `ensureDeadline` áp **30s mỗi target**; `behaviorContextProduce` có thể truyền ctx không deadline ⇒ nếu loop N target theo kiểu ngây thơ, worst case ~N×30s block cả bước context. Fix: bọc **một** `context.WithTimeout(ctx, dependenceTotalBudget)` (≈25s) chia sẻ cho mọi target; cap `dependenceMaxTargets` (≈10, vì mỗi target 1 npx); cap `dependenceMaxDependents`/target (≈15). Lỗi `Dependents` từng target → skip, không fatal (giống `HighSeverity` trong `scope.go`).
- `T-5` **v1 = GitNexus-only (mới, sửa Q-2).** `structure.fallbackProvider.Dependents` **bỏ qua ctx** và `filepath.WalkDir` **toàn repo đọc mọi .go mỗi target** (byte-substring match, nhiễu) ⇒ không cancel được bằng budget ở `T-4`, và với dir-bucket target thì cực đắt + vô nghĩa. Nên v1: chỉ chạy khi `provider.Available()==true`; khi GitNexus vắng → render **note trung thực** ("blast-radius unavailable: GitNexus chưa index; chạy `npx gitnexus analyze`"), **không** chạy walk. Fallback file-level dời sang follow-up sau khi `structure` fallback được làm ctx-aware + rẻ hơn.
- `T-6` `Deterministic() = true`; **sort** `Nearest`/`Flows` trước khi render (gitnexus không đảm bảo thứ tự). Section rỗng ⇒ golden/fixture không đổi (fixture không có contract → không dependence) — cấm sửa golden expectation.
- `T-7` Render đi qua **generic pass** (`default` case trong `renderFlowContextSection` → `### source.dependence` + `_Source:_`) — KHÔNG thêm case đặc biệt/skip-list (khác canonical.head/change.contract). **KHÔNG** đi qua `composeFlowNodeAgentPrompt` (append-prompt kiểu P-4 = HIGH risk, CP-43-CATALOG §5). `Complete=false` → thêm 1 dòng chú thích "danh sách có thể chưa đủ (dynamic dispatch)".
- `T-8` **Capability không qua subprocess (mới).** Lấy `hasGitNexus` từ **cache** `tooling.LoadToolingStatus(dotFP)` + `tooling.StatusOf(_, "gitnexus").Status=="ok"` (đã được `tooling.CheckAll` ghi tại engine setup — `engine_setup.go`), **không** gọi `tooling.CheckTool` (spawn `npx gitnexus --version`) trong path collect. tooling.json vắng → `hasGitNexus=false` → note `T-5`.

### Constraints

- Giữ bất biến CP-41/SD-22: deterministic, **không vector**, mọi source degrade-mềm (AC-9), không chặn turn/run.
- GitNexus exec bounded (`T-4`); nếu index stale, chấp nhận kết quả có thể cũ — KHÔNG tự chạy `npx gitnexus analyze` trong path collect (side-effect nặng); chỉ đọc.
- `apps/admin-web` deprecated — UI chỉ sửa `apps/desktop-flowpilot`.
- Không skip `verify`: chạy test battery §6 + build/vet sạch trước khi đóng; **live B12 bắt buộc** để verify CLI contract (`E-schema`) trước merge.
- GitNexus impact analysis cho các hàm fan-in **đã chạy 2026-07-17** (index fresh `6741bdc`, CP-43-CATALOG §5). Ràng buộc rút ra: (1) đường `Collect`→`buildFlowContextPackage`→generic `RenderFlowContextPackage` toàn **LOW**; (2) **KHÔNG** đưa `source.dependence` vào skip-list renderer; (3) **KHÔNG** append thẳng prompt kiểu P-4 qua `composeFlowNodeAgentPrompt` (**HIGH**); (4) nếu phải sửa `behaviorContextProduce` để wire provider → trace behavior-registry tay (impact=0 là báo-thiếu do dynamic dispatch). **Thiết kế này không đụng (2),(3),(4)** — không thêm field hints, render generic, provider dựng trong `Fetch`.

### Open Questions

- `Q-1` Contract v1 lấy theo **run** (`GetLatestForRun`, một feature/mạch việc mỗi run — nhất quán Task-247 R-5). Nếu thực tế cần per-step dependence, mở Task riêng.
- `Q-2` **(RESOLVED → `T-5`)** File-level fallback khi GitNexus vắng bị **hoãn**: fallback provider không cancel được + O(repo)/target. v1 render note "unavailable". Đây là **context**, không phải gate, nên không bao giờ block (đúng CP-43 Q-3 nuance).
- `Q-3` Cache kết quả `Dependents` theo (target, index-commit) trong run? v1: **không** cache (cap ≤10 target, budget chia sẻ). Theo dõi latency, mở follow-up nếu chậm.
- `Q-4` **(mới, `E-schema`)** `structure.gitNexusProvider` chạy `impact <target> --json` (không `--repo`/`--direction`) và parse `{dependents,nearest,flows}`, nhưng **không test nào chạy binary thật**, và CP-43-CATALOG §5 dùng cờ khác (`--repo flowpilot --direction upstream`). Cùng giả định chưa verify này nằm dưới gate `HighSeverity` (`scope.go`). **B12 phải verify** `npx gitnexus impact <file.go> --json` trả schema dùng được trên project chính; nếu impact **chỉ nhận symbol** (không nhận file-path) → dừng, mở follow-up resolve path→symbol (hoặc giới hạn chỉ `DeclaredSymbols`).

### Source Refs

- `CP-43` P-2 (drift = declared vs touched, dùng `structure.Dependents`), P-3/P-5, P-6, Q-3 (symbol-level gated on `structure.Available()`).
- `CP-44` P-4/P-5/D-5 (registry, opt-in vs default, bounded external source shape).
- `CP-50` P-4 (`change.contract` source — nguồn input, mirror `Fetch` shape), P-1 (canonical.head — mirror struct shape).
- `structure` module: `Provider.Dependents`, `DependentsSummary{Count,Nearest,Flows,Complete}`, `New(repoDir, isGitNexusOK)`.
- Đọc để hiểu input shape: `changecontract/infer.go` (`InferFromDiff`/`topLevelDir` → dir-bucket), `changecontract/parse.go` (`ParseDeclaration` → file list), `changecontract/scope.go` (`matchesDeclaredScope`, `HighSeverity` — pattern loop-Dependents có sẵn).

## 1. Goal

Cho mọi run đã khai Change Contract **declared** (liệt kê file/symbol thật): step sau (validate/audit/review/reprompt) nhìn thấy **blast-radius** của vùng đã khai — "sửa các file/symbol này thì những symbol/file/flow nào bị ảnh hưởng" — suy ra deterministic từ GitNexus knowledge graph (không AI), bounded, degrade-mềm khi GitNexus vắng hoặc chưa có contract. Đây là câu trả lời code-level cho câu hỏi *"đã handle context-depend-code qua gitnexus chưa: đổi cái này ảnh hưởng cái nào"*.

## 2. Parent Links

- coding plan: `CP-43` (P-6 — `source.dependence`), catalog `CP-43-CATALOG`
- tech design: `SD-21` (change contract), `SD-22` (registry)
- system spec: `SS-14` US-3 / AC-7 (records symbols/files changed) / AC-9 (non-fatal)
- specific upstream ids: `CP-43 P-2/P-3/P-6/Q-3`, `CP-50 P-4`, `CP-44 P-4/P-5`

## 3. Trigger

Câu hỏi owner (2026-07-17): "chúng ta đã handle context-depend-code qua gitnexus chưa — kiểu change cái này ảnh hưởng cái nào". Hiện `change.contract` mới chỉ khai *declared scope*; chưa có source nào biến declared scope thành **impact/dependents**. `internal/structure` (GitNexus wrapper) đã tồn tại từ CP-35/CP-43 nhưng mới dùng trong flow **gate** (drift detection `r-scope`/`HighSeverity`), chưa từng surface như một **context source** cho prompt. Task này lấp đúng khoảng đó.

## 4. Code Guide (Exact Change)

> Đọc kèm §4.5 Edge cases. Mọi identifier/code bằng English; prose bằng Việt cho khớp doc lân cận.

### 4.1 `T-a` File mới `apps/local-runner/internal/runner/context_source_dependence.go`

```go
package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/flowgate"
	"flowpilot-runner/internal/structure"
	"flowpilot-runner/internal/tooling"
)

// ContextSourceDependence — Task-259 (CP-43 P-6). Blast-radius của declared
// change scope, suy ra từ GitNexus impact. Default set, priority 3 (sau
// change.contract, trước source.excerpt qua SourceType tiebreak — T-2).
const ContextSourceDependence ContextSourceID = "source.dependence"

const (
	dependenceMaxTargets    = 10               // mỗi target spawn 1 npx (T-4)
	dependenceMaxDependents = 15               // cap/target chống prompt bloat (T-4)
	dependenceTotalBudget   = 25 * time.Second // shared wall-clock cho toàn Fetch (T-4)
)

// dependenceSource. newProvider là test seam: nil ⇒ production dựng
// structure.New từ capability cache (T-8). KHÔNG thêm field vào FlowContextHints.
type dependenceSource struct {
	priority    int
	newProvider func(cwd string, hasGitNexus bool) structure.Provider
}

func (s *dependenceSource) ID() string          { return string(ContextSourceDependence) }
func (s *dependenceSource) Priority() int       { return s.priority }
func (s *dependenceSource) Deterministic() bool { return true }

func (s *dependenceSource) Fetch(ctx context.Context, hints FlowContextHints) (FlowContextSection, error) {
	section := FlowContextSection{SourceType: s.ID(), Priority: s.priority}
	if strings.TrimSpace(hints.WorkflowRunID) == "" || hints.Workspace == "" {
		return section, nil // không run/workspace → zero-value (T-6 golden-safe)
	}

	// T-1: input = latest Change Contract của run. Không có → zero-value.
	store, err := changecontract.OpenStoreReadOnly(hints.Workspace)
	if err != nil || store == nil {
		return section, nil
	}
	c, ok := store.GetLatestForRun(hints.WorkflowRunID)
	if !ok {
		return section, nil
	}

	// T-3: chuẩn hoá target (symbol trước; chỉ file code cụ thể; bỏ dir/glob/doc/flag).
	targets := dependenceTargets(c)
	if len(targets) == 0 {
		return section, nil // vd. contract inferred (chỉ dir-bucket) → rỗng có chủ đích
	}

	// T-8: capability từ cache tooling.json — KHÔNG spawn subprocess ở collect.
	dotFP := filepath.Join(hints.Workspace, ".flowpilot")
	statuses, _ := tooling.LoadToolingStatus(dotFP)
	hasGitNexus := tooling.StatusOf(statuses, "gitnexus").Status == "ok"

	provider := s.provider(hints.Workspace, hasGitNexus)

	// T-5: v1 GitNexus-only. Fallback provider walk toàn repo/target + bỏ qua ctx
	// ⇒ không chạy; render note trung thực.
	if !provider.Available() {
		section.SourceRef = "gitnexus:impact"
		section.Body = "_Blast-radius unavailable: GitNexus chưa index project này (chạy `npx gitnexus analyze` để bật)._"
		return section, nil
	}

	// T-4: một budget chia sẻ ⇒ N target không thể serialize thành N×30s.
	bctx, cancel := context.WithTimeout(ctx, dependenceTotalBudget)
	defer cancel()

	body := renderDependenceBody(bctx, provider, targets)
	if strings.TrimSpace(body) == "" {
		return section, nil // không target nào resolve được → zero-value
	}
	section.SourceRef = filepath.ToSlash(filepath.Join(dotFP, "contracts", "contracts.ndjson"))
	section.Body = body
	return section, nil
}

func (s *dependenceSource) provider(cwd string, hasGitNexus bool) structure.Provider {
	if s.newProvider != nil {
		return s.newProvider(cwd, hasGitNexus)
	}
	return structure.New(cwd, hasGitNexus)
}

// dependenceTargets: symbols trước (đúng đối tượng gitnexus impact), rồi file
// code cụ thể. Bỏ dir-bucket (InferFromDiff), glob, doc/audit, và target dạng
// CLI-flag. Dedup + sort (T-6 determinism) + cap dependenceMaxTargets.
func dependenceTargets(c changecontract.Contract) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(t string) {
		t = strings.TrimSpace(t)
		if t == "" || strings.HasPrefix(t, "-") { // '-' → chống bị parse thành flag (E-flag)
			return
		}
		if _, dup := seen[t]; dup {
			return
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	for _, sym := range c.DeclaredSymbols { // symbol: bỏ qua path-filter
		add(sym)
	}
	for _, p := range c.DeclaredPaths {
		p = filepath.ToSlash(strings.TrimSpace(p))
		if isConcreteCodeTarget(p) {
			add(p)
		}
	}
	sort.Strings(out)
	if len(out) > dependenceMaxTargets {
		out = out[:dependenceMaxTargets]
	}
	return out
}

// isConcreteCodeTarget loại các shape mà gitnexus impact không dùng được:
// glob (*?[), doc/audit (requirements/, change-audit/, *.md), và dir-bucket
// (không có đuôi code — "apps", "internal"). Chỉ nhận file có đuôi code.
func isConcreteCodeTarget(p string) bool {
	if p == "" || strings.ContainsAny(p, "*?[") {
		return false
	}
	if flowgate.IsDocOrAuditFile(p) {
		return false
	}
	return dependenceCodeExts[strings.ToLower(filepath.Ext(p))]
}

var dependenceCodeExts = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".py": true, ".rs": true, ".java": true, ".kt": true, ".swift": true,
}

// renderDependenceBody chạy Dependents/target dưới ctx budget, cap + sort để
// reproducible; lỗi/empty từng target → skip. Trả "" nếu không target nào có
// dependent (⇒ Fetch trả zero-value section).
func renderDependenceBody(ctx context.Context, provider structure.Provider, targets []string) string {
	var b strings.Builder
	any, incomplete := false, false
	for _, t := range targets {
		summary, err := provider.Dependents(ctx, t)
		if err != nil {
			continue // non-fatal/target (AC-9, giống HighSeverity)
		}
		nearest := append([]string(nil), summary.Nearest...)
		sort.Strings(nearest) // T-6: gitnexus không đảm bảo thứ tự
		if len(nearest) > dependenceMaxDependents {
			nearest = nearest[:dependenceMaxDependents]
		}
		if summary.Count == 0 && len(nearest) == 0 {
			continue
		}
		any = true
		fmt.Fprintf(&b, "- Sửa `%s` ảnh hưởng %d dependent:\n", t, summary.Count)
		for _, d := range nearest {
			fmt.Fprintf(&b, "  - %s\n", d)
		}
		if len(summary.Flows) > 0 {
			flows := append([]string(nil), summary.Flows...)
			sort.Strings(flows)
			fmt.Fprintf(&b, "  - flows: %s\n", strings.Join(flows, ", "))
		}
		if !summary.Complete {
			incomplete = true
		}
	}
	if !any {
		return ""
	}
	out := "Blast-radius của declared scope (deterministic, từ GitNexus impact — không AI):\n\n" + b.String()
	if incomplete {
		out += "\n_Note: một số caller dùng dynamic dispatch; danh sách có thể chưa đủ (Complete=false)._"
	}
	return strings.TrimSpace(out)
}
```

### 4.2 `T-b` Đăng ký + default set — `context_sources_builtin.go`

- Trong `registerBuiltinContextSources`: thêm `mustRegisterContextSource(r, &dependenceSource{priority: 3}) // Task-259: sau change.contract, trước source.excerpt (SourceType tiebreak)`.
- Trong `defaultContextSourceIDs`: chèn `string(ContextSourceDependence)` **ngay sau** `string(ContextSourceChangeContract)` (thứ tự slice không ảnh hưởng output vì `Collect` sort, nhưng đọc dễ hiểu).

### 4.3 `T-c` Render fill-switch — `flow_context_package.go`

- Trong `sectionsForRender`, thêm vào `switch` gán priority mặc định (nhánh Priority==0 cho payload cũ):
  ```go
  case ContextSourceDependence:
      sections[i].Priority = 3
  ```
- **KHÔNG** thêm case vào `renderFlowContextSection`/`renderGenericSections` — dependence render qua `default` (generic) là đúng (T-7).

### 4.4 `T-d` UI descriptor — `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`

- Thêm `{ id: "source.dependence", label: "Source Dependence (impact)" }` vào `contextSourceOptions`; `npx tsc --noEmit` sạch.

### 4.5 Edge cases & inherited risks (bắt buộc handle / verify)

| # | Edge case | Xử lý trong task này |
|---|---|---|
| `E-target` | `DeclaredPaths` là **dir-bucket** (inferred) hoặc **glob/file** (declared) — KHÔNG phải symbol; `gitnexus impact` là symbol/file-oriented | `dependenceTargets`/`isConcreteCodeTarget`: symbol trước, chỉ file code cụ thể; bỏ dir-bucket/glob/doc. Inferred contract → rỗng có chủ đích (documented) |
| `E-budget` | `ensureDeadline` 30s **mỗi target** + ctx có thể không deadline ⇒ N×30s block | Một `context.WithTimeout(ctx, 25s)` chia sẻ; cap ≤10 target |
| `E-fallback` | `fallbackProvider.Dependents` bỏ qua ctx + walk toàn repo/target (nhiễu) | v1 GitNexus-only; unavailable → note, không walk (T-5) |
| `E-latency` | `tooling.CheckTool` spawn `npx gitnexus --version` mỗi lần | Đọc cache `tooling.json` (`LoadToolingStatus`/`StatusOf`) — no subprocess (T-8) |
| `E-flag` | target bắt đầu `-` bị `npx gitnexus impact` hiểu là flag | `add()` bỏ target prefix `-` |
| `E-schema` | `structure` parse `{dependents,nearest,flows}` **chưa verify** với binary thật; §5 dùng cờ khác | **B12 live verify bắt buộc**; nếu impact chỉ nhận symbol → follow-up (Q-4). Degrade an toàn (empty) nếu schema lệch |
| `E-golden` | thêm source vào default set ⇒ thêm 1 empty section vào `pkg.Sections` | Section rỗng khi no-contract; các test đếm section dùng **explicit binding** (không default) nên không vỡ; RENDER bỏ qua empty Body ⇒ `TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor` pass không sửa |

**Shipped-code risks kế thừa (KHÔNG sửa ở task này — ghi để follow-up):** (a) `structure.ensureDeadline` rò cancel func (`//nolint:govet`); (b) `fallbackProvider.importDependents` uncancellable + O(repo)/target; (c) CLI contract chưa test bằng binary thật (dùng chung với gate `HighSeverity`). Nếu B12 lộ (c) sai → nâng ưu tiên fix `structure` trước khi source này có giá trị.

## 5. Touched Areas

- files (mới): `internal/runner/context_source_dependence.go`, `internal/runner/context_source_dependence_test.go`
- files (sửa): `internal/runner/context_sources_builtin.go` (register + default set), `internal/runner/flow_context_package.go` (chỉ 1 `case` fill-switch trong `sectionsForRender`), `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`
- modules: runner context assembly; `structure` (reuse, read-only); `changecontract` (read-only); `tooling` (read cache); `flowgate` (`IsDocOrAuditFile`)
- **KHÔNG** đụng: `FlowContextHints` (không field mới), `composeFlowNodeAgentPrompt`, renderer skip-list, `behaviorContextProduce`
- routes: không · tables: không (đọc `.flowpilot/contracts/contracts.ndjson` + `.flowpilot/tooling.json` + GitNexus index)

## 6. Test Signature Guide

Working dir: `cd apps/local-runner`. Test file mới `context_source_dependence_test.go`.

### 6.1 Fake provider (test seam)

```go
type fakeDependenceProvider struct {
	available bool
	byTarget  map[string]structure.DependentsSummary
	calls     []string // thứ tự target được query — để assert cap/filter
}

func (f *fakeDependenceProvider) Available() bool { return f.available }
func (f *fakeDependenceProvider) Dependents(_ context.Context, target string) (structure.DependentsSummary, error) {
	f.calls = append(f.calls, target)
	return f.byTarget[target], nil
}

// helper: dựng source với fake
func newDependenceSourceWithFake(f *fakeDependenceProvider) *dependenceSource {
	return &dependenceSource{priority: 3, newProvider: func(string, bool) structure.Provider { return f }}
}

// helper: workspace + contract đã lưu (dùng changecontract.NewStore(ws).Save(...))
```

### 6.2 Test battery (unit — fake provider, không chạy npx thật)

| Test | Setup | Assert chính |
|---|---|---|
| `TestDependenceSourceFromContractTargets` | contract declared: `DeclaredPaths=["apps/local-runner/internal/runner/foo.go"]`, `DeclaredSymbols=["Bar"]`; fake.available=true, dependents cho cả `"Bar"` và file | Body chứa cả 2 target + dependents của chúng; `SourceRef` trỏ `contracts.ndjson`; `fake.calls` gồm cả symbol lẫn file |
| `TestDependenceSourceNoContractDegrades` | workspace không có `contracts.ndjson` | `section.Body==""` **và** `section.Warnings==nil` **và** `section.Omitted==nil` (golden-safe, T-6) |
| `TestDependenceSourceInferredDirBucketYieldsEmpty` | contract inferred: `DeclaredPaths=["apps","internal"]`, no symbols | `section.Body==""`; `len(fake.calls)==0` (dir-bucket bị loại — E-target) |
| `TestDependenceSourceDropsGlobDocAndFlagTargets` | `DeclaredPaths=["internal/**","requirements/x.md","internal/runner/x.go"]`, `DeclaredSymbols=["-rf"]` | `fake.calls==["internal/runner/x.go"]` (glob/doc/flag/symbol-flag đều bị loại) |
| `TestDependenceSourceGitNexusUnavailableRendersNote` | fake.available=false, có contract declared hợp lệ | Body chứa "unavailable"/"analyze"; **không** panic; `len(fake.calls)==0` (không chạy Dependents) |
| `TestDependenceSourceBoundsTargetsAndDependents` | 30 declared code-file path; fake trả 40 dependents/target | `len(fake.calls) <= dependenceMaxTargets` (10); mỗi target render ≤ `dependenceMaxDependents` (15) dòng dependent |
| `TestDependenceSourceDeterministicOutput` | contract cố định; fake trả `Nearest` thứ tự **đảo** giữa 2 lần Fetch | Body 2 lần **giống hệt** (đã sort — T-6) |
| `TestDependenceSourceIncompleteNote` | fake trả `Complete=false` cho 1 target | Body có dòng note "chưa đủ (Complete=false)" |
| `TestDependenceInDefaultSetAndRegistered` | — | `DefaultContextSourceRegistry().Resolve("source.dependence")` ok; `defaultContextSourceIDs` chứa id; `src.Priority()==3`; `ValidateFlowContextSources` chấp nhận flow khai `source.dependence` |
| `TestRenderFlowContextPackageDependenceAfterContract` | pkg.Sections gồm change.contract(3)+source.dependence(3)+source.excerpt(4) đều có Body/Excerpt | render: `index("### change.contract") < index("### source.dependence") < index("### Source:")` (T-2 tiebreak) |

### 6.3 Regression (không sửa expectation)

- `TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor` — pass nguyên trạng (fixture không có contract → dependence rỗng → render không đổi). **Nếu vỡ ⇒ dependence đang emit khi không nên.**
- `go test ./internal/runner/ -count=1 -timeout 8m` — không regression mới so baseline flake. Đặc biệt xanh: `TestContextSource*`, `TestResolveEnabledContextSourceIDs*`, `TestFlowContextPackage*`, `TestArtifactBinding*` (đếm section dùng explicit binding — phải xác nhận vẫn đúng số).

### 6.4 Lệnh

```bash
# focus source mới
go test ./internal/runner/ -run 'TestDependenceSource|TestDependenceInDefaultSet|TestRenderFlowContextPackageDependenceAfterContract' -count=1
# structure (reuse) vẫn xanh
go test ./internal/structure/... -count=1
# full runner
go build ./... && go vet ./internal/runner/... && go test ./internal/runner/ -count=1 -timeout 8m
```

## 7. Out of Scope

- KHÔNG biến `source.dependence` thành **gate rule** (block/warn) — đó là `r-scope` drift của CP-43 P-2. Task này chỉ là **context** cho prompt.
- KHÔNG tự chạy `npx gitnexus analyze` (re-index) trong path collect.
- KHÔNG suy diễn target từ diff (việc của `source.excerpt`); chỉ dùng contract đã khai.
- KHÔNG file-level fallback khi GitNexus vắng (v1 — T-5); dời follow-up.
- KHÔNG sửa shipped `structure` bugs (ensureDeadline leak, fallback walk) trong task này — chỉ ghi risk.
- KHÔNG thêm per-symbol Canonical Head / đổi granularity contract (CP-43 Q-4); KHÔNG cache cross-run (v1, Q-3); KHÔNG thêm field `FlowContextHints`.

## 8. Definition of Done

- [ ] `context_source_dependence.go`: `dependenceSource` với `ID()="source.dependence"`, `Priority()==3`, `Deterministic()==true`; `newProvider` test seam; production dùng `structure.New` từ capability cache.
- [ ] **Target normalization (T-3):** `DeclaredSymbols` trước; `DeclaredPaths` chỉ giữ file code cụ thể; loại dir-bucket / glob / doc-audit / target prefix `-`; dedup + sort + cap `dependenceMaxTargets`.
- [ ] **Capability không subprocess (T-8):** `hasGitNexus` từ `tooling.LoadToolingStatus`/`StatusOf`; tooling.json vắng → false.
- [ ] **GitNexus-only v1 (T-5):** `!provider.Available()` → note trung thực, KHÔNG chạy fallback walk.
- [ ] **Bounded (T-4):** một `context.WithTimeout` chia sẻ mọi target; per-target `Dependents` error non-fatal; dependents cap `dependenceMaxDependents`.
- [ ] **Render (T-7):** qua generic `default` case (`### source.dependence` + `_Source:_`); KHÔNG vào skip-list; KHÔNG qua `composeFlowNodeAgentPrompt`; `Complete=false` → note; `Nearest`/`Flows` sorted.
- [ ] **Degrade zero-value (T-6):** no-contract / target rỗng / unresolved → Body rỗng, **Warnings nil, Omitted nil** — golden fixtures không đổi.
- [ ] **Registry/priority (T-2):** registered; trong `defaultContextSourceIDs` sau `change.contract`; fill-switch có `case ContextSourceDependence: =3`; test-lock thứ tự `change.contract < source.dependence < source.excerpt`.
- [ ] Unit battery §6.2 xanh; regression §6.3 xanh **không sửa expectation**; `go build ./... && go vet ./internal/runner/...` sạch.
- [ ] UI: `WorkflowsSettings.tsx` có option `source.dependence`; `npx tsc --noEmit` sạch.
- [ ] **Live B12 (E-schema, bắt buộc trước merge):** feature GitNexus-indexed + turn Coding khai contract **declared** (file/symbol thật) → prompt validate/audit sau chứa block "Sửa `<target>` ảnh hưởng `<dependents>` + flows"; GitNexus stale → note, không lỗi; run chưa có contract → không block. **Verify `npx gitnexus impact <file.go> --json` trả schema dùng được**; nếu chỉ nhận symbol → dừng + follow-up (Q-4).
- [ ] Post-land: rerun `gitnexus_impact` cho `dependenceSource.Fetch` + `gitnexus_detect_changes` xác nhận blast-radius thực tế trước merge (kỳ vọng LOW, CP-43-CATALOG §5).
- [ ] Docs sync: CP-43 gốc P-6 (note landed + caveat inferred→empty); CP-43-CATALOG §1.3/§3 "planned"→"landed", §4.1 hàng test tên thật, §6 B12 giữ; AC-9 degrade khẳng định.
- [ ] Satisfies `SS-14` US-3 / AC-7 (records symbols/files affected) / AC-9 (non-fatal, không chặn turn).

## 9. Completion Notes

- result: `draft` — chưa implement (planned). Readiness review 2026-07-17 đã bổ sung code guide + test signatures + DoD + edge cases; 4 gap chính (E-target/E-budget/E-fallback/E-schema) đã có hướng xử lý trong §4.
- follow-ups: (1) file-level fallback sau khi `structure.fallbackProvider` được làm ctx-aware + rẻ hơn (Q-2); (2) resolve path→symbol nếu `gitnexus impact` không nhận file-path (Q-4/E-schema); (3) cache Dependents theo (target, index-commit) nếu latency cao (Q-3); (4) hardening shipped `structure` (ensureDeadline leak, fallback walk) — §4.5.
- upstream docs updated: CP-43 gốc (P-6) và CP-43-CATALOG (§1.3/§3/§4.1/§6) — đồng bộ; đánh dấu "landed" khi code xong.
