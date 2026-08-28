# BUG-323: `structure.gitNexusProvider.Dependents` luôn trả rỗng — sai flag, thiếu `--repo`, sai schema JSON, và không nhận file-path

## Metadata

- Document ID: `BUG-323`
- Title: `gitNexusProvider.Dependents chạy "npx gitnexus impact <target> --json" — nhưng CLI thật không có flag --json, bắt buộc --repo khi máy index nhiều repo, trả schema JSON khác hoàn toàn, và chỉ resolve symbol chứ không nhận file path; bốn lỗi xếp chồng khiến MỌI truy vấn blast-radius trả rỗng trong im lặng`
- Phase: `bugfix`
- Status: `done` (2026-08-11 — F-1/F-2/F-3/F-5 implemented; see [CA-433](../../../change-audit/CA-433-gitnexus-structure-provider-dependents-fix.md))
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-27`
- Last Updated: `2026-08-11`
- Feature Keys: `context-regression-engine`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/done/CP-35-Context-And-Regression-Engine-Rollout.md) (§4.3 — nơi module `structure` ra đời)
- Child Documents: `none`
- Related Documents: [Task-185: Scope-Drift Detection](../../08-Task/done/Task-185-Scope-Drift-Detection.md) (**bị ảnh hưởng ngược** — `HighSeverity` không bao giờ true ⇒ `r-scope` không bao giờ block; claim `done-with-waiver` của [CA-353](../../change-audit/CA-353-task249-185-done-re-audit.md) bị vô hiệu một phần), [Task-259: source.dependence Context Source](../../08-Task/todo/Task-259-Source-Dependence-Context-Source.md) (**bị chặn** — T-3/T-5 bất khả thi khi chưa fix), [CP-43](../../07-Coding-Plan/done/CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (Q-3 "symbol-level block khi có GitNexus" — thực tế chưa từng chạy), [CP-43-CATALOG](../../07-Coding-Plan/done/CP-43-Context-Source-Catalog-And-Test-Log.md) (§5 ghi lệnh người chạy tay **đúng**; §6 B12 đã đặt sẵn điều kiện dừng này), [CP-54](../../07-Coding-Plan/todo/CP-54-Locus-Anchored-Context-Relevance.md) (P-6 symbol-tier thừa hưởng cùng blocker), [CA-294](../../change-audit/CA-294-scope-drift-detection.md) (ghi nhận `DeclaredSymbols` là extension point chưa ai populate)
- Replaces: `none`
- Tags: `context-regression-engine, gitnexus, structure, blast-radius, cli-contract-drift, silent-failure, scope-drift, severity-high`

## AI Quick View

### Summary

Phát hiện khi chuẩn bị implement Task-259 (`source.dependence`), trong bước verify CLI mà chính [CP-43-CATALOG §6 B12](../../07-Coding-Plan/done/CP-43-Context-Source-Catalog-And-Test-Log.md) yêu cầu làm trước. Module `structure` — nền của mọi truy vấn "đổi cái này ảnh hưởng cái nào" — **chưa từng hoạt động** kể từ khi viết:

`gitNexusProvider.Dependents` ([gitnexus.go:25](../../../apps/local-runner/internal/structure/gitnexus.go)) chạy `npx gitnexus impact <target> --json`. CLI thật **không có** flag `--json` ⇒ exit non-zero ⇒ [dòng 32-34](../../../apps/local-runner/internal/structure/gitnexus.go) nuốt lỗi và trả `DependentsSummary{Complete:false}` rỗng. Không log, không warning, không phân biệt được với "target thật sự không có dependents".

Sửa riêng flag vẫn chưa đủ: còn 3 lỗi nữa xếp chồng phía sau (thiếu `--repo`, schema JSON khác hoàn toàn, và CLI chỉ resolve **symbol** chứ không nhận **file path**). Lỗi #3 đặc biệt nguy hiểm vì nó **fail im lặng thành công** — `encoding/json` bỏ qua field lạ nên parser trả `ok=true` với toàn nil.

### Current Ask

Làm `structure.Provider.Dependents` thật sự trả được blast-radius từ GitNexus, và **không bao giờ fail im lặng** nữa: một truy vấn hỏng phải phân biệt được với một truy vấn thành công trả 0 dependents.

### Key Decisions

- `D-1` **Fail phải nhìn thấy được.** Nguyên nhân gốc khiến lỗi này sống sót nhiều tháng không phải flag sai, mà là [dòng 32-34](../../../apps/local-runner/internal/structure/gitnexus.go) biến *mọi* lỗi thành "rỗng thành công". Cần tách `Complete=false` (dynamic dispatch — kết quả đúng nhưng có thể thiếu) khỏi `Failed/Err` (truy vấn hỏng). Consumer vẫn degrade-mềm (AC-9) nhưng phải *biết* là mình đang mù.
- `D-2` **Giữ nhánh parse schema cũ, thêm nhánh schema thật.** Không sửa test cũ (`additive-tests-only`): `tryParseJSON` giữ nguyên cho schema `{dependents,nearest,flows}`, thêm parser cho schema thật và thử nó **trước**. Test cũ xanh nguyên, không cần đụng.
- `D-3` **`--repo` suy từ basename của repoDir.** CLI bắt buộc `--repo` khi máy index >1 repo. `cmd.Dir` không disambiguate. Basename (`flowpilot`) khớp cách GitNexus đặt tên repo.
- `D-4` **Chấp nhận GitNexus chỉ nhận symbol.** Không cố ép file-path. Việc "từ file suy ra symbol" là bài toán riêng (xem `Q-2`), không thuộc phạm vi fix này.

### Constraints

- **Không sửa test cũ** — `TestParseGitNexusJSONOutput`/`TestParseGitNexusJSONFallbackToDependents`/`TestParseGitNexusTextOutput`/`TestNearestLimit`/`TestCompleteIsFalseWhenDynamicDetected` phải xanh nguyên trạng (`additive-tests-only`, `oracle-rule`).
- Degrade-mềm phải giữ (AC-9): GitNexus hỏng/vắng **không bao giờ** chặn turn.
- Không thêm dependency, không thêm bảng DB.
- Bounded: giữ deadline hiện có; không để một truy vấn hỏng biến thành retry loop.

### Open Questions

- `Q-1` `Dependents` nên đổi chữ ký (thêm error/`Failed` field) hay giữ nguyên và chỉ log? Đổi chữ ký sạch hơn nhưng đụng caller (`scope.go`, `gate_hook.go`) — cần impact analysis trước.
- `Q-2` **Bài toán input symbol** (chặn Task-259, tách khỏi BUG này): `Contract.DeclaredSymbols` **không bao giờ được populate** ([parse.go:14-16](../../../apps/local-runner/internal/changecontract/parse.go) chỉ parse `feature:`/`intent:`/`files:`; [infer.go:35](../../../apps/local-runner/internal/changecontract/infer.go) chỉ set `DeclaredPaths`). Phương án: (a) thêm `symbols:` vào `ParseDeclaration` + skill `context-discipline`; (b) map file→symbol qua GitNexus; (c) chấp nhận `source.dependence` chỉ chạy khi AI khai symbol tường minh. Cần chốt trước khi mở lại Task-259.
- `Q-3` Có nên thêm một smoke-test chạy CLI **thật** (skip khi không có GitNexus) để chống drift hợp đồng CLI tái diễn? Đây chính là loại test mà bộ test hiện tại thiếu.

### Source Refs

- Code: `apps/local-runner/internal/structure/{gitnexus.go,provider.go,fallback.go}`, `apps/local-runner/internal/changecontract/{scope.go,parse.go,infer.go}`, `apps/local-runner/internal/runner/gate_hook.go`.
- CA: [CA-294](../../change-audit/CA-294-scope-drift-detection.md) (Task-185), [CA-353](../../change-audit/CA-353-task249-185-done-re-audit.md) (re-audit done-with-waiver), [CA-297](../../change-audit/CA-297-canonical-head-packing-and-admin.md) (Task-188).
- Verify chạy 2026-07-27 trên `flowpilot` @ `59451ff`, index up-to-date.

## 1. Bằng chứng (chạy thật, 2026-07-27)

Index sạch, không phải vấn đề stale:

```bash
npx gitnexus status
# Indexed commit: 59451ff / Current commit: 59451ff / Status: ✅ up-to-date
```

### L-1 — Flag `--json` không tồn tại

```bash
npx gitnexus impact apps/local-runner/internal/changecontract/scope.go --json
# error: unknown option '--json'
```

`npx gitnexus impact --help` xác nhận toàn bộ option hợp lệ: `-d/--direction`, `-r/--repo`, `--depth`, `--include-tests`. **Không có `--json`** — CLI vốn đã trả JSON mặc định.

Hệ quả trong code:

```go
cmd := exec.CommandContext(ctx, "npx", "gitnexus", "impact", target, "--json")  // gitnexus.go:25
...
if err := cmd.Run(); err != nil {
    return DependentsSummary{Complete: false}, nil   // gitnexus.go:32-34 — nuốt lỗi
}
```

⇒ **Mọi** lời gọi `Dependents` thoát ở dòng 33 với summary rỗng. Không log. Không phân biệt với "không có dependents".

### L-2 — Thiếu `--repo` khi máy index nhiều repo

```bash
npx gitnexus impact ScopeDiff
# {"error": "Multiple repositories indexed. Specify which one with the \"repo\" parameter.
#            Available: MPlan, BEMplan, flowpilot", ...}
```

`cmd.Dir = g.repoDir` **không** giải quyết — GitNexus giữ index toàn cục, phải chỉ định `--repo` tường minh.

### L-3 — Schema JSON khác hoàn toàn (fail im lặng thành công)

Output thật của `npx gitnexus impact ScopeDiff --repo flowpilot`:

```json
{
  "target": { "id": "Function:apps/.../scope.go:ScopeDiff", "name": "ScopeDiff",
              "type": "Function", "filePath": "apps/.../scope.go" },
  "direction": "upstream",
  "impactedCount": 4,
  "risk": "LOW",
  "summary": { "direct": 1, "processes_affected": 2, "modules_affected": 1 },
  "affected_processes": [ { "name": "runFlowGateAtEpoch", ... }, { "name": "runChildArtifactOutputGateAtEpoch", ... } ],
  "affected_modules": [ ... ]
}
```

Code lại chờ ([gitnexus.go:64-69](../../../apps/local-runner/internal/structure/gitnexus.go)):

```go
var raw struct {
    Dependents []string `json:"dependents"`
    Nearest    []string `json:"nearest"`
    Flows      []string `json:"flows"`
}
```

**Không field nào tồn tại trong output thật.** Vì `encoding/json` mặc định bỏ qua field lạ, `dec.Decode` **thành công** ⇒ `tryParseJSON` trả `ok=true` với `nearest=nil, flows=nil` ⇒ `Count=0`. Nghĩa là **kể cả sau khi sửa L-1 và L-2, kết quả vẫn rỗng — nhưng lần này rỗng trong im lặng hoàn toàn**, không còn cả tín hiệu exit-code.

### L-4 — CLI chỉ resolve symbol, không nhận file path

```bash
npx gitnexus impact ScopeDiff --repo flowpilot                                   # ✅ risk LOW, impactedCount 4
npx gitnexus impact apps/local-runner/internal/changecontract/scope.go --repo flowpilot
# {"error": "Target 'apps/local-runner/internal/changecontract/scope.go' not found"}
```

Help text nói rõ: *"what breaks if you change a **symbol**"*.

## 2. Vì sao test hiện tại không bắt được

`structure_test.go` kiểm `parseGitNexusOutput` bằng **JSON tổng hợp viết theo schema giả định** (`{dependents,nearest,flows}`). **Không test nào chạy CLI thật**, nên hợp đồng CLI trôi đi mà bộ test vẫn xanh 100%.

Trớ trêu: [CP-43-CATALOG §5](../../07-Coding-Plan/done/CP-43-Context-Source-Catalog-And-Test-Log.md) ghi lại các lần chạy impact **thủ công** với lệnh **đúng** (`--repo flowpilot`, không `--json`) và thu được kết quả thật. Người chạy đúng, code chạy sai, và không có gì đối chiếu hai đường đó.

## 3. Tác động

| Vùng | Tác động |
|---|---|
| **Task-185 / CP-43 P-2** | `ScopeDiff`→`HighSeverity` gọi `Dependents` với **path** ⇒ luôn rỗng ⇒ **không bao giờ** true ⇒ `r-scope` **không bao giờ escalate lên block**, luôn chỉ warn. Quyết định CP-43 `Q-3` ("symbol-level block khi có GitNexus") **chưa từng chạy thật**. Claim `done-with-waiver` ở CA-353 bị vô hiệu một phần. |
| **Task-259 / CP-43 P-6** | **Bất khả thi như đã spec.** T-3 tier-1 (`DeclaredSymbols`) là dead code (`Q-2`); tier-2 (file path) bị CLI từ chối (L-4). ⇒ `source.dependence` sẽ trả rỗng 100% trường hợp. Đúng điều kiện dừng B12. |
| **CP-54 P-6** | Symbol-tier thừa hưởng cùng blocker. **P-1→P-5 KHÔNG bị ảnh hưởng** (path-overlap thuần git, không đụng GitNexus). |
| **CP-52 P-3** | Xếp hạng cross-worktree theo blast-radius cũng đứng sau fix này. |
| **`gate_hook.go`** | Severity ranking của gate dựa trên dependents ⇒ hiện luôn tính như "không có dependents". |

## 4. Đề xuất hướng sửa (chưa implement)

- `F-1` Bỏ `--json`; thêm `--repo <basename(repoDir)>` (`D-3`).
- `F-2` Thêm parser cho schema thật (`affected_processes`/`affected_modules`/`impactedCount`/`risk`/`summary`), thử **trước** parser cũ; giữ nguyên parser cũ làm nhánh sau để test cũ xanh (`D-2`).
- `F-3` Phân biệt "hỏng" với "rỗng" (`D-1`) — theo `Q-1` chốt chữ ký; **bắt buộc impact analysis trước khi đổi** vì `Dependents` có caller ở `scope.go` và `gate_hook.go`.
- `F-4` Thêm smoke-test chạy CLI thật, skip khi GitNexus vắng (`Q-3`) — chống tái drift.
- `F-5` Test additive cho: flag đúng, `--repo` được truyền, parse schema thật, schema cũ vẫn parse được, target không tồn tại ⇒ *failed* chứ không phải *empty*, GitNexus vắng ⇒ degrade sạch.

## 5. Non-Goals

- Giải bài toán input symbol cho Task-259 (`Q-2`) — tách riêng.
- Làm `fallbackProvider` ctx-aware / rẻ hơn (đã ghi ở Task-259 T-5, vẫn hoãn).
- Đổi cách `r-scope` quyết định block (chỉ khôi phục tín hiệu đầu vào cho nó).

## 6. Definition of Done

- [x] `Dependents` trả blast-radius thật cho một symbol có thật (verify bằng cùng lệnh §1 L-4). Smoke: `TestGitNexusDependentsSmokeScopeDiff`, manual `ScopeDiff --repo flowpilot` → `impactedCount: 4`.
- [x] Truy vấn hỏng (target không tồn tại / CLI lỗi / GitNexus vắng) **phân biệt được** với "0 dependents"; không bao giờ chặn turn. Missing target → `Dependents` error; `HighSeverity` skips errors non-fatally (unchanged).
- [x] Test cũ `structure` xanh **nguyên trạng, không sửa dòng nào**.
- [x] Test mới additive phủ `F-5` (`gitnexus_bug323_test.go`).
- [ ] `HighSeverity` của Task-185 được đánh giá lại: **partial — provider fixed for symbols; `HighSeverity` still passes file paths (L-4), so `r-scope` block on file-only drift remains unwired until BUG-323 Q-2 / symbol input follow-up.** Waiver update tracked in CA-433, not CA-353 restore.
- [x] CA note ghi lại, tham chiếu BUG này ([CA-433](../../../change-audit/CA-433-gitnexus-structure-provider-dependents-fix.md)).
