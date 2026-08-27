# CP-43 — Step Test Guide (Change Contract & Canonical Intent Signature)

## Metadata

- Document ID: `CP-43-TEST-STEPS`
- Title: `CP-43 Verification Steps By Feature Part`
- Phase: `verification`
- Status: `active`
- Owner: `FlowPilot`
- Created: `2026-08-11`
- Parent Documents: [CP-43: Change Contract And Canonical Intent Signature](./CP-43-Change-Contract-And-Canonical-Intent-Signature.md), [CP-43-CATALOG: Context Source Catalog And Test Log](./CP-43-Context-Source-Catalog-And-Test-Log.md)
- Related Documents: [Task-184](../done/Task-184-Change-Contract-Capture.md) (P-1), [Task-185](../done/Task-185-Scope-Drift-Detection.md) (P-2), [Task-186](../done/Task-186-Canonical-Head-And-Intent-Signature.md) (P-3), [Task-187](../done/Task-187-Superseding-Decision-Records-And-Retire.md) (P-4), [Task-188](../done/Task-188-Canonical-Head-Packing-And-Admin.md) (P-5), [Task-259](../done/Task-259-Source-Dependence-Context-Source.md) (P-6), [BUG-323](../../09-BugFix/done/BUG-323-GitNexus-Structure-Provider-Always-Returns-Empty.md), [CA-434](../../change-audit/CA-434-symbol-input-and-source-dependence-task259.md), [CA-435](../../change-audit/CA-435-cp43-p5-pack-budget-and-admin-scope-diff.md)
- Tags: `change-contract, canonical-head, verification, test-steps`

## AI Quick View

- **What:** Checklist từng bước test cho 4 cơ chế cốt lõi CP-43 (P-1→P-4) + phần mở rộng P-5/P-6.
- **Why:** Operator chạy một lần, tick pass/fail trong doc này. **Code CP-43 v1 done (2026-08-11)** — manual steps tick khi rảnh, không block ship.
- **Working dir:** `cd apps/local-runner` cho mọi lệnh `go test` bên dưới.

## 4 phần cốt lõi (map CP-43)

| Phần                                     | CP phase | Task     | Mô tả ngắn                                                     |
| ---------------------------------------- | -------- | -------- | -------------------------------------------------------------- |
| **1. Change Contract**                   | P-1      | Task-184 | Khai báo / infer intent + `declared_paths` trước hoặc sau diff |
| **2. Scope-drift detection**             | P-2      | Task-185 | Gate `r-contract` / `r-scope`: diff thực tế vs declared scope  |
| **3. Canonical Head + intent signature** | P-3      | Task-186 | Một Head per `feature_key`, hash intent, status drift          |
| **4. Superseding Decision Records**      | P-4      | Task-187 | Churn `A→B→A` collapse; rejected alternatives                  |

Phần mở rộng (vẫn thuộc CP-43 — **code done 2026-08-11**):

| Phần                                    | CP phase | Task     | Trạng thái                                                                                           |
| --------------------------------------- | -------- | -------- | ---------------------------------------------------------------------------------------------------- |
| **5. Head-first packing + Admin**       | P-5      | Task-188 | ✅ **code done** ([CA-435](../../change-audit/CA-435-cp43-p5-pack-budget-and-admin-scope-diff.md))   |
| **6. `source.dependence` blast-radius** | P-6      | Task-259 | ✅ **code done** ([CA-434](../../change-audit/CA-434-symbol-input-and-source-dependence-task259.md)) |

---

## Phần 1 — Change Contract (P-1)

**Mục tiêu:** Mọi turn sửa code có `Contract` lưu tại `.flowpilot/contracts/contracts.ndjson` — declared hoặc inferred — không chặn turn khi capture fail.

### Automated (bắt buộc pass)

```bash
go test ./internal/changecontract/... -count=1 -run 'TestParse|TestInfer|TestStore|TestContract' -v
```

| Step | Lệnh / test                                                | Pass khi                                                                 |
| ---- | ---------------------------------------------------------- | ------------------------------------------------------------------------ |
| 1.1  | `TestParseDeclaration*` / `TestParseContract*`             | Parse block `feature:` / `intent:` / `files:` / `symbols:` → struct đúng |
| 1.2  | `TestInferContractFromDiff*`                               | Không khai → inferred từ diff đầu, `Confidence=inferred`                 |
| 1.3  | `TestStore*` / `TestStoreFileLivesUnderFlowpilotContracts` | NDJSON under `.flowpilot/contracts/`, last-wins `(run_id, step_id)`      |
| 1.4  | `TestGetLatestForRun*`                                     | Đọc lại contract của run/step                                            |

### Manual / integration

| Step | Hành động                               | Pass khi                                                            | Ref  |
| ---- | --------------------------------------- | ------------------------------------------------------------------- | ---- |
| 1.M1 | Coder khai contract rồi sửa trong scope | Entry trong `contracts.ndjson`; step sau thấy `### change.contract` | B1   |
| 1.M2 | Turn code **không** khai contract       | `r-contract` reprompt 1 lần; sau đó inferred, **không block**       | B15  |
| 1.M3 | Capture lỗi (corrupt store tạm)         | Turn vẫn lưu artifact; gate degrade, không panic                    | AC-9 |

**Context source (registry):**

```bash
go test ./internal/runner/ -count=1 -run 'TestChangeContract|TestAppendChangeContract' -v
```

---

## Phần 2 — Scope-drift detection (P-2)

**Mục tiêu:** `actual_touched \ declared_scope` → violation; warn mặc định; block khi high-severity + GitNexus có dependents (BUG-323 + CA-434 đã ship — cần manual B14 xác nhận live).

### Automated (bắt buộc pass)

```bash
go test ./internal/changecontract/... -count=1 -run 'TestScopeDiff|TestHighSeverity|TestGitNexus' -v
go test ./internal/flowgate/... -count=1 -run 'TestCheckRule.*Contract|TestCheckRule.*Scope' -v
go test ./internal/structure/... -count=1 -v
```

| Step | Test                                                                                        | Pass khi                                                       |
| ---- | ------------------------------------------------------------------------------------------- | -------------------------------------------------------------- |
| 2.1  | `TestScopeDiffInScopeYieldsEmpty`                                                           | Sửa trong declared → không drift                               |
| 2.2  | `TestScopeDiffFlagsOutOfScopePaths`                                                         | Path ngoài scope → listed                                      |
| 2.3  | `TestScopeDiffExcludesDocAndAuditFiles`                                                     | `requirements/`, `change-audit/`, `*.md` bị loại               |
| 2.4  | `TestCheckRuleEditOutsideDeclaredScopeWarnsByDefault`                                       | `r-scope` action warn                                          |
| 2.5  | `TestCheckRuleEditOutsideDeclaredScopeBlocksOnlyWhenHighSeverity`                           | Block **chỉ** khi `HighSeverity=true`                          |
| 2.6  | `TestGitNexusQueryTargetsForPathSymbolFirst` / `TestHighSeverityUsesSymbolThenPathFallback` | Out-of-scope path → query symbol trước, path fallback (CA-434) |

### Manual

| Step | Hành động                                                         | Pass khi                       | Ref                                                            |
| ---- | ----------------------------------------------------------------- | ------------------------------ | -------------------------------------------------------------- |
| 2.M1 | Khai contract → sửa **1 file ngoài** declared                     | SSE violation warn + path list | B13                                                            |
| 2.M2 | Cùng scenario + GitNexus indexed + enforce + symbol có dependents | **Block** high-severity        | B14 ⚠️ **manual live** (code path đã fix BUG-323 + symbol map) |

**Ghi chú P-2 residual:** Symbol-level drift thật (diff-hunk→symbol) chưa build; v1 = file-level scope + `HighSeverity` qua `gitnexus impact` trên symbol suy từ path/`symbols:`.

---

## Phần 3 — Canonical Head + intent signature (P-3)

**Mục tiêu:** `.flowpilot/canonical/<feature_key>.json` với `intent_signature`; status `current` / `spec_drifted` / `code_drifted` / `spec_less`.

### Automated (bắt buộc pass)

```bash
go test ./internal/changecontract/... -count=1 -run 'TestSaveHead|TestLoadHead|TestBuildHead|TestSpecDrifted|TestCodeDrifted|TestUpdateHead|TestComputeSignature|TestSignature' -v
```

| Step | Test / area                                                 | Pass khi                                    |
| ---- | ----------------------------------------------------------- | ------------------------------------------- |
| 3.1  | `TestSaveHeadThenLoadHeadRoundTrip`                         | Head persist/load                           |
| 3.2  | `TestBuildHeadBirthWithNoHistoryOrDocsIsSpecLess`           | Feature mới → `spec_less`                   |
| 3.3  | `TestBuildHeadWithGoverningDocsIsSpecBacked`                | Doc refs → signature có governing docs      |
| 3.4  | `TestSpecDriftedTrueWhenDocChanges`                         | Sửa SS/SD governing → `spec_drifted`        |
| 3.5  | `TestCodeDriftedTrueOnlyWhenOutOfContractAndNotSpecDrifted` | Out-of-scope code → `code_drifted`          |
| 3.6  | `TestUpdateHeadRefreshesBehaviorAndStaysCurrent`            | In-contract gate pass → refresh + `current` |

### Manual

| Step | Hành động                               | Pass khi                                  | Ref                                  |
| ---- | --------------------------------------- | ----------------------------------------- | ------------------------------------ |
| 3.M1 | Feature có Head; sửa governing doc      | File canonical status `spec_drifted`      | B16                                  |
| 3.M2 | Rebaseline qua desktop action           | `spec_less` → `current` round-trip        | Task-186                             |
| 3.M3 | Flow mode (post CP-55): coder gate pass | Head **pending**, chưa ghi file canonical | CP-55 P-5 tests                      |
| 3.M4 | Flow terminal `done`                    | Head finalize đúng 1 lần                  | `TestFlowDoneFinalizesCanonicalHead` |

**Context source:**

```bash
go test ./internal/runner/ -count=1 -run 'TestCanonicalHead' -v
```

---

## Phần 4 — Superseding Decision Records (P-4)

**Mục tiêu:** Churn dương không pack mặc định; rejected alternatives có lý do; retire/merge explicit.

### Automated (bắt buộc pass)

```bash
go test ./internal/changecontract/... -count=1 -run 'TestFoldDecisions|TestRetireHead' -v
go test ./internal/changecontract/... -count=1 -run 'TestRenderHeadBlockListsRejected' -v
```

| Step | Test                                                   | Pass khi                                    |
| ---- | ------------------------------------------------------ | ------------------------------------------- |
| 4.1  | `TestFoldDecisionsExtractsRejectedFromChatSummary`     | Rejection marker → `Decision`               |
| 4.2  | `TestFoldDecisionsIgnoresBulletsWithNoRejectionMarker` | Không marker → không fold (heuristic limit) |
| 4.3  | `TestRetireHeadMergedCopiesDecisionsToTarget`          | Retire merge copy decisions                 |
| 4.4  | `TestRenderHeadBlockListsRejectedAndRevertedDecisions` | "do NOT re-attempt" trong render            |

### Manual

| Step | Hành động                       | Pass khi                                                | Ref      |
| ---- | ------------------------------- | ------------------------------------------------------- | -------- |
| 4.M1 | Feature qua vòng A→B→C→A        | Prompt chỉ canonical A + rejected B, không replay churn | B17      |
| 4.M2 | Có alternative bị bỏ kèm lý do  | List rejected trong Head + panel                        | B18      |
| 4.M3 | Retire feature qua desktop POST | Head retired, provenance preserved                      | Task-187 |

**P-4 residual (không gate done P-4 code):** retire-intent auto-detect; `r-attach-spec` / `r-retire` approval wiring.

---

## Phần 5 — Head-first packing + Admin (P-5) ✅ code done

**Mục tiêu:** Prompt đọc Head **trước** history; budget có thể drop history; desktop panel đủ thông tin; `canonical/*.json` sync Drive.

### Automated — phần đã done

```bash
go test ./internal/changecontract/... -count=1 -run 'TestRenderHeadBlock' -v
go test ./internal/runner/ -count=1 -run 'TestFeatureHistorySourceNoHead|TestHandleGetCanonicalHead|TestHandleGetStepContract' -v
go test ./internal/contextsync/... -count=1 -run 'TestSharedFilesIncludesCanonicalHeads|TestSharedFilesNeverIncludesContracts' -v
```

| Step | Test                                                           | Pass khi                                                    | Status                                 |
| ---- | -------------------------------------------------------------- | ----------------------------------------------------------- | -------------------------------------- |
| 5.1  | `TestRenderHeadBlockIncludesBehaviorAndSignatureAndStatus`     | Block Head đầy đủ field                                     | ✅                                     |
| 5.2  | `TestRenderHeadBlockAnnotatesSpecLess`                         | Annotation spec-less                                        | ✅                                     |
| 5.3  | `TestSharedFilesIncludesCanonicalHeads`                        | `canonical/*.json` trong sync set                           | ✅                                     |
| 5.4  | `TestSharedFilesNeverIncludesContracts`                        | `contracts.ndjson` local-only                               | ✅                                     |
| 5.5  | `TestHandleGetCanonicalHeadReturnsFoundHead`                   | HTTP read Head                                              | ✅                                     |
| 5.6  | Budget drop history under pressure                             | Log + drop raw history; Head giữ                            | ✅ T-2 (`flow_context_pack_budget.go`) |
| 5.7  | `TestFeatureHistorySourcePrependsCanonicalHead`                | **Lưu ý:** sau CP-50, Head là source `canonical.head` riêng | ✅ architecture shift                  |
| 5.8  | `TestApplyFlowContextPackBudgetDropsHistoryKeepsHead`          | History dropped, Head survives render                       | ✅                                     |
| 5.9  | `TestHandleGetStepContractIncludesScopeDiff`                   | API trả in/out-of-scope từ live git diff                    | ✅                                     |
| 5.10 | `TestHandleListProjectFeaturesMergesCatalogLedgerAndCanonical` | Feature list cho panel datalist                             | ✅                                     |

### Manual — còn lại

| Step | Hành động                                        | Pass khi                                           | Ref                        |
| ---- | ------------------------------------------------ | -------------------------------------------------- | -------------------------- |
| 5.M1 | Turn Coding Claude **và** Codex, feature có Head | Prompt: `## Canonical state` **trước** history     | B19 ⚠️                     |
| 5.M2 | Ép token pressure (history dài)                  | History drop có **log** `[context-pack]`; Head giữ | B20 ⚠️ manual              |
| 5.M3 | Panel ProjectsSettings → Canonical Head          | behavior + scope diff in/out                       | B21 ✅ partial (live diff) |
| 5.M4 | contextsync manifest                             | `canonical/*.json` listed                          | B22 ✅                     |

---

## Phần 6 — `source.dependence` (P-6) ✅ code done

**Mục tiêu:** Context source render blast-radius từ declared contract targets qua GitNexus.

**Shipped (2026-08-11):** BUG-323 (provider CLI), CA-434 (`symbols:` parser + path→symbol), Task-259 (`source.dependence` default set + tests).

### Automated (bắt buộc pass)

```bash
go test ./internal/runner/ -count=1 -run 'TestDependence' -v
go test ./internal/changecontract/... -count=1 -run 'TestGitNexusImpactTargets|TestParseDeclarationSymbols' -v
go test ./internal/structure/... -count=1 -v
```

| Step | Test                                                  | Pass khi                                                    | Status |
| ---- | ----------------------------------------------------- | ----------------------------------------------------------- | ------ |
| 6.1  | `TestDependenceSourceFromContractTargets`             | Symbol + file-derived symbol queried; body có blast-radius  | ✅     |
| 6.2  | `TestDependenceSourceNoContractDegrades`              | Không contract → zero-value section                         | ✅     |
| 6.3  | `TestDependenceSourceInferredDirBucketYieldsEmpty`    | Inferred dir-bucket → rỗng có chủ đích                      | ✅     |
| 6.4  | `TestDependenceSourceDropsGlobDocAndFlagTargets`      | Chỉ concrete code + valid symbols                           | ✅     |
| 6.5  | `TestDependenceSourceGitNexusUnavailableRendersNote`  | GitNexus vắng → note, không panic                           | ✅     |
| 6.6  | `TestDependenceSourceBoundsTargetsAndDependents`      | Cap ≤10 target, ≤15 dependents/target                       | ✅     |
| 6.7  | `TestDependenceInDefaultSetAndRegistered`             | Default set + registry + priority 3                         | ✅     |
| 6.8  | `TestRenderFlowContextPackageDependenceAfterContract` | Render order: change.contract < source.dependence < excerpt | ✅     |

### Manual — còn operator tick

| Step | Hành động                                          | Pass khi                                                          | Ref           |
| ---- | -------------------------------------------------- | ----------------------------------------------------------------- | ------------- |
| 6.M1 | Contract declared file/symbol thật → step validate | Block `### source.dependence` với dependents **live** từ GitNexus | B12 ⚠️        |
| 6.M2 | Contract inferred (dir-bucket only)                | Section rỗng có chủ đích                                          | B12 ✅ (unit) |
| 6.M3 | >10 targets, mix glob/doc                          | Chỉ query concrete; cap + deterministic                           | B23 ⚠️ live   |

---

## Full-loop E2E (cổng đóng CP-43)

Chạy **một phiên** liên tục sau khi 4 phần automated xanh. Tick từng bước:

| #   | Bước                                                | Liên quan phần    |
| --- | --------------------------------------------------- | ----------------- |
| 1   | Coder khai contract → sửa trong scope               | P-1               |
| 2   | Sửa thêm 1 file ngoài scope → warn                  | P-2               |
| 3   | Gate pass in-scope → Head / pending (Flow: pending) | P-3               |
| 4   | Hoàn nguyên hướng cũ + rejected record              | P-4               |
| 5   | Prompt Claude + Codex Head-first                    | P-5               |
| 6   | Panel desktop scope view                            | P-5               |
| 7   | Dependence blast-radius step sau                    | P-6 ⚠️ manual B12 |
| 8   | contextsync manifest                                | P-5               |

Chi tiết: [CP-43-CATALOG §6.1](./CP-43-Context-Source-Catalog-And-Test-Log.md).

### One-shot automated bundle (trước manual)

```bash
go test ./internal/changecontract/... ./internal/flowgate/... ./internal/contextsync/... ./internal/structure/... -count=1 -timeout 5m
go test ./internal/runner/ -count=1 -timeout 8m \
  -run 'TestCanonicalHead|TestChangeContract|TestHandleGetCanonicalHead|TestHandleGetStepContract|TestSharedFiles|TestDependence'
```

---

## Sau khi chạy xong — tóm tắt P-5 / P-6 / BUG-323

### 1. P-5 là gì?

**P-5 = Authority-first packing + Admin visibility** (Task-188).

Không phải tính Head hay drift (đó là P-3). P-5 trả lời: _AI và operator **nhìn thấy** Head đúng cách chưa?_

- **Prompt packing:** Head block là slot **bắt buộc**, xuất hiện **trước** raw history; dưới áp lực token budget, history có thể bị drop/summarize **có log** — Head không bị drop.
- **Sync:** `canonical/*.json` sync qua `context-engine/` Drive; `contracts.ndjson` **local-only**.
- **Desktop:** panel Canonical Head trong `ProjectsSettings.tsx` — behavior, signature chip, rejected decisions, và (MVP+) diff in/out-of-scope thật.

**Đã xong:** render Head block, source `canonical.head`, sync manifest, HTTP read endpoints, panel MVP, **T-2 budget-drop (render path)**, **ScopeDiff + feature list API + panel scope view**.

**Chưa xong:** B19 live verify Claude/Codex; B20 manual log check under real long history; full CP-10 packer (v1 char budget only).

### 2. P-6 — trạng thái (2026-08-11)

**Code done; verification chưa đóng hết.**

| Tiêu chí                                          | Trạng thái                                                                                  |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| BUG-323 `structure.Dependents`                    | ✅ Done ([CA-433](../../change-audit/CA-433-gitnexus-structure-provider-dependents-fix.md)) |
| `symbols:` trong contract parser                  | ✅ Done (CA-434)                                                                            |
| Path → symbol heuristic (`GitNexusImpactTargets`) | ✅ Done (CA-434)                                                                            |
| Task-259 `source.dependence` + default set        | ✅ Done                                                                                     |
| Unit tests `context_source_dependence_test.go`    | ✅ Pass                                                                                     |
| Manual B12 / B23 live GitNexus trên Flow thật     | ⚠️ **Chưa tick**                                                                            |

**Còn lại để đóng P-6 verification:** chạy B12/B23 trên desktop + runner live (`npx gitnexus analyze` trước).

### 3. BUG-323 — đã fix (2026-08-11)

Provider gọi CLI đúng:

```text
npx gitnexus impact <symbol> --repo <basename(repoDir)>
```

Parser schema v2 + trả error thay vì rỗng im lặng. Follow-up CA-434 map file→symbol cho `HighSeverity` và `source.dependence`.

**Hệ quả CP-43 sau fix:**

| Chứ năng                            | Trạng thái                                 |
| ----------------------------------- | ------------------------------------------ |
| P-2 `r-scope` block (high-severity) | ✅ Code path hoạt động; ⚠️ B14 manual live |
| P-6 `source.dependence`             | ✅ Unit pass; ⚠️ B12/B23 manual live       |
| P-5 packing                         | Không phụ thuộc GitNexus                   |

---

## Checklist tổng (operator)

| Phần               | Automated      | Manual      | Ghi chú                            |
| ------------------ | -------------- | ----------- | ---------------------------------- |
| P-1 Contract       | ☐              | ☐           | Code done                          |
| P-2 Scope drift    | ☐              | ☐ B13 ☐ B14 | Code done; B14 manual in doc       |
| P-3 Canonical Head | ☐              | ☐           | Code done                          |
| P-4 Decisions      | ☐              | ☐           | Code done                          |
| P-5 Packing/Admin  | ☐              | ☐ B19-B20   | **Code done** — manual tick in doc |
| P-6 Dependence     | ☐ automated ✅ | ☐ B12/B23   | **Code done** — manual tick in doc |

**CP-43 code complete (2026-08-11).** Verification-complete = tick manual steps trong doc này + one-shot bundle xanh.

###############################################

# P-1 — Change Contract

## [CHAT] C1 ✅ DONE — Khai contract declared → lưu store

Prompt:
[Change Contract]
feature: calc-core
intent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a
files: calc.go, calc_test.go
symbols: Subtract

Thuc hien:

1. Them `SubtractWithGuard(a, b int) (int, error)` vao calc.go (b > a -> error).
2. Them test vao calc_test.go.
3. Chay `go test ./...`.
   Verify: contracts.ndjson entry confidence:"declared", declared_paths, feature_key; log gate pass không violation.

## [CHAT] C2 ✅ DONE — Không khai → reprompt 1 lần → inferred (B15)

Prompt:
Fix giup toi: Percentage(part, whole int) tra ve 0.0 khi whole=0 nhung khong canh bao. Them ham PercentageSafe tra error khi whole=0.
Verify: log có đúng 1 lần reprompt r-contract; entry confidence:"inferred"; turn không chặn.

## [FLOW] F1 ✅ DONE — Planner freeze → FrozenContractRecord

Mở /flow → chọn rag-harness (có preflight_contract_plan → preflight_contract_freeze → context → implement). Prompt (chỉ nêu task, KHÔNG khai contract):
Them ham ClampChecked(n, lo, hi int) (int, error) vao format.go: tra error khi lo > hi, ngoai ra clamp n vao [lo, hi]. Them test vao format_test.go. Chay go test ./...
Verify:

- contracts/frozen_contracts.ndjson + frozen_contract_events.ndjson có record (feature_key, intent, declared_paths, version).
- Context step prompt (log context.produce) build được package từ rec.DeclaredPaths.

# P-2 — Scope drift

## [CHAT] C3 ✅ DONE — Warn khi sửa file ngoài scope (B13)

Chuẩn bị: gate-config.json → "gate_mode":"warn", restart runner.
Prompt:
[Change Contract]
feature: calc-core
intent: them ham ClampLoHi vao format.go
files: format.go
symbols: Clamp

Thuc hien:

1. Them `ClampLoHi(n, lo, hi int) int` vao format.go.
2. Them test vao user_test.go.
3. Chay go test ./...
   Verify: SSE warn liệt kê user_test.go; không chặn; sửa docs/\*.md không bị tính drift.

## [CHAT] C4 ✅ DONE — Block high-severity (B14) — cần enforce + gitnexus

Chuẩn bị: đổi gate_mode về enforce, restart.
Prompt:
[Change Contract]
feature: calc-core
intent: refactor parseUserID trong user.go dung helper moi
files: user.go
symbols: parseUserID

Thuc hien:

1. Them `IsValidInt(s string) bool` vao calc.go.
2. Sua parseUserID goi IsValidInt.
3. Chay go test ./...
   Verify: calc.go ngoài scope + có dependents → block severity=high. (GitNexus không có dependents → chỉ warn = degrade hợp lệ.)

## [FLOW] ✅ F2 — Drift ngoài frozen declared_paths → hard block (PASSED LIVE 2026-08-27)

> **Trạng thái:** ✅ **Nhánh A live-verified (run-153698):** planner freeze `calc-core` v1 `declared_paths=[calc.go, add_with_log_test.go]` (08:12:35, freeze không còn park giả — CA-645); coder implement chạm `drift_probe.go` ngoài scope → `flow gate block: flow scope drift: wrote outside the frozen contract's declared paths: drift_probe.go` (08:14:55, log FrozenContractScopeDrift), flow dừng WAITING_USER_APPROVAL chờ amend; Stop của operator kết thúc flow. Nhánh B (planner khai đủ → chạy tiếp) đã pass từ run-247326/249193.

Mở /flow → rag-harness. Prompt cố ý để coder chạm file không nằm trong path planner khai:

Hien tai Add tra ve a + b. Them mot ham AddWithLog(a, b int) int vao calc.go in log va tra ket qua, dong thoi them test cho no trong user_test.go.

Verify: nếu planner chỉ khai calc.go mà coder sửa user_test.go → gate block + escalate (log flow_contract_freeze_chain / FrozenContractScopeDrift), flow dừng chờ amend; nếu planner khai đủ cả 2 file → flow chạy tiếp (không lỗi).

## [FLOW] ✅ F3 — Planner khai sai/thiếu path → amend (PASSED LIVE 2026-08-27 run-169381)

> **Trạng thái:** ✅ **Nhánh A live-verified (run-169381):** planner freeze `calc-core` v1 `declared_paths=["calc.go"]` (07:48:42, additive-tests-only không khai test cũ — CA-647); tester `test_signatures` Append vào `calc_test.go` ngoài scope → `flow gate block: flow scope drift: wrote outside the frozen contract's declared paths: calc_test.go` (07:54:33, step WAITING_USER_APPROVAL, `flow_parked_awaiting_user`), amend `POST .../agent-loop/amend {"paths":["calc_test.go"]}` → `frozen_contracts.ndjson` **v2 Supersedes v1** `declared_paths=["calc.go","calc_test.go"]` (07:54:33) cho cả 2 writer steps; flow resume → implement/coder đổi `Subtract` sang `int64`, validate/reviewer/synthesis/audit pass → `done` (08:00:13, `go test ./...` ok, pending canonical `calc-core` updated). Nhánh B (planner khai đủ, không drift) đã pass từ run-165644/165818.

**Recipe deterministic (dùng file planner không khai):** ép coder sửa
`calc_test.go` — file cũ có `TestSubtract` vẫn dùng `int`; đổi `Subtract`
sang `int64` mà không sửa test → `go test` fail → coder phải sửa
`calc_test.go`. Planner theo bias safe-fix (Q1 `Only edit calc.go; leave calc_test.go untouched`) luôn khai **v1 chỉ `calc.go`** (không có test file) → drift thật khi tester/coder Append `calc_test.go`.

Prompt (chạy sau khi đã xóa `user_decoy.go`, restart runner để load CA-647):
Doi ham Subtract(a, b int) int trong calc.go thanh Subtract(a, b int64) int64 (phan than tra ve int64). Sua lai tat ca TestSubtract trong calc_test.go cho dung kieu int64. Chay go test ./...

Verify:
- Coder đổi calc.go → `go test` fail ở `calc_test.go` → coder sửa
  `calc_test.go` → **drift `calc_test.go` thật** → block WAITING_USER_APPROVAL.
- Amend:
  ```powershell
  Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:4317/client/workflow-runs/<runId>/agent-loop/amend" -ContentType "application/json" -Body '{"paths":["calc_test.go"]}'
  ```
- `frozen_contracts.ndjson`: **v2 + Supersedes v1**; flow resume → coder retry pass.
- Nếu planner vẫn khai `calc_test.go` → Nhánh B (đã live-verified từ run-165644). Decoy
  `user_decoy.go` không còn cần (planner scan call site).

# P-3 — Canonical Head

## [CHAT] ✅ C5 — Gate pass → ghi Head ngay (PASSED LIVE 2026-08-27 run-172547)

> **Trạng thái:** ✅ **Live-verified (run-172547, chat):** contract declared `calc-core` `["calc.go","calc_test.go"]` + `Modulo`; coder thêm `ModuloChecked2` vào `calc.go:75` + `TestModuloChecked2`/`modulo_checked2_test.go` (additive, CA-948), `go test` ok, gate `r-newtest` reprompt → `r-scope` warn + `accepted` (07:45:26, turns_to_accept 1). Code + test ✅, gate ✅. **Head:** `canonical/calc-core.json` chưa auto-đổi `updated_at` sau chat (vẫn 2026-08-17 b97e485) — chat head write không trigger như flow (pending→finalize); sẽ finalize ở flow tiếp (F4/F5). Coi C5 code/gate **done**, head drift là gap minor.

Prompt:
[Change Contract]
feature: calc-core
intent: them ham ModuloChecked2 tra error khi b = 0
files: calc.go, calc_test.go
symbols: Modulo

Thuc hien:

1. Them `ModuloChecked2(a, b int) (int, error)` vao calc.go (b==0 -> ErrDivideByZero).
2. Them test.
3. Chay go test ./...
   Verify: canonical/calc-core.json updated_at đổi, status:"current", intent_signature tính lại.

## [FLOW] ✅ F4 — Gate pass → Head PENDING (3.M3) — PASSED LIVE 2026-08-27 run-268792

> **Trạng thái:** ✅ **Live-verified (run-268792, rag-harness):** `rag-harness` valid parity per `task-270` (cùng `CP-55 P-5` 2-phase với `context-coding-review-synthesis`). `test_signatures` gate pass `14:10:42.323Z` → `Stage()` `seq:1` `signature:9788472336a7d8cb616aaf916794c997` trong `pending_canonical.ndjson` trong khi `canonical/calc-core.json` chưa tồn tại (F4 exact); `implement` retry `14:13:18.913Z` → `seq:2` `8bad1f366c1343f6e1ba6fc989f03b3e` (dual-writer harness expected). Frozen contracts `implement+test_signatures` cùng `declared_paths:[calc.go,calc_test.go]` `base_sha:255b3d4`. Spec deviation nhỏ so với `context-coding-review-synthesis` gốc đã note `harness parity`.

Mở /flow → context-coding-review-synthesis (harness parity). Prompt:
Them ham MaxChecked(a, b int) (int, error) vao calc.go. Them test vao calc_test.go.
Verify sau coder gate pass (trước khi flow done): canonical-pending/pending_canonical.ndjson có record "pending"; canonical/calc-core.json chưa ghi.

## [FLOW] ✅ F5 — Terminal done → finalize đúng 1 lần (3.M4) — PASSED LIVE 2026-08-27 run-268792

> **Trạng thái:** ✅ **Live-verified (run-268792):** `status:completed 14:14:33.328Z` `pending_canonical_events.ndjson` `finalized record_seq:2 reason:flow terminal acceptance at:14:14:28.854Z`; `canonical/calc-core.json` ghi đúng `1 lần` `updated_at:14:13:18.913874Z` `head_commit:60f45af` `spec_less` `intent_signature:1560425c28ce048533dd2bd94a57af49e260c82d8d762f672f9fbe44be70115f`; `go test ./... 0.386s PASS` 5/5 `TestMaxChecked_*` (math used). Durable 2-phase verified.

Tiếp tục F4 đến khi flow đạt done. Verify: record pending thành finalized; canonical/calc-core.json ghi đúng 1 lần; chạy lại → không ghi lần 2 (durable, 2-phase).

# P-4 — Superseding decisions

## [CHAT] ✅ C6 — Rejected alternatives hiển thị (B18) — PASSED LIVE 2026-08-27 (via F6 chain)

> **Trạng thái:** ✅ **Live-verified (F6 chain 270566→280863):** `canonical/calc-core.json` `663886fe` `current` `Restore Add plain a+b` có `decisions[rejected float division]` render `do NOT re-attempt` trong `## Canonical state` **trước** `## History` ở `implement` prompt `run-280863 fd5f56d4071d5682` (và các `run 272360/278472`), `CA-916/917/918` document `B a+b+1` `C a+b-1` superseded. Desktop `ProjectsSettings → Canonical Head calc-core` thấy `behavior plain a+b + decisions` đúng.

Không cần prompt mới — sau C5:

- Desktop panel ProjectsSettings → Canonical Head feature calc-core: thấy rejected decisions + do NOT re-attempt.
- Prompt turn Coding kế tiếp (Chat) có ## Canonical state kèm rejected.

## [FLOW] ✅ F6 — Churn A→B→C→A không replay (B17) — PASSED LIVE 2026-08-27 runs 270566/272360/278472/280863 (optional, 4 commit)

> **Trạng thái:** ✅ **Live-verified:** `Run1 270566 4d44829 docs Add godoc → Run2 272360 1b68b62 a+b+1 CA-916 (+ Allow drift calc_test.go) → Run3 278472 ab6419c a+b-1 CA-917 (delete plus1, Allow drift) → Run4 280863 2017c62 a+b plain CA-918 (delete minus1)`, 4× `status:completed` `finalized seq4,6,8,10` `go test PASS`, `pending seq3→10` durable, `git log 4 commits` linked, `canonical` `663886fe` `current` plain sum.

4 flow chạy riêng, mỗi cái 1 commit, đều dùng rag-harness:

# Run 1: "Them doc comment cho Add trong calc.go" → 270566 done 15:10:05

# Run 2: "Doi Add tra ve a+b+1 de thu nghiem" → 272360 done 15:20:57

# Run 3: "Hoan nguyen Add ve a+b-1" → 278472 done 15:37:45

# Run 4: "Dam bao Add tra ve a + b dung" → 280863 done 15:44:32

Verify run 4: context prompt chỉ canonical hiện hành + rejected, không replay churn dương. ✅ `prompt run-280863` có `## Canonical a+b-1 current` trước `## History newest=truth Task-909` và `Omitted` churn, đúng Head-first.

# P-5 — Head-first packing ✅ code done + grok live-verified 2026-08-27 (F6 chain)

> **Trạng thái:** ✅ **Live-verified via F6 chain grok (runs 270566/272360/278472/280863):** `5.1` `RenderHeadBlock` + `5.2` `spec_less` + `5.7` `canonical.head (1) → feature.history (2)` đúng `## Canonical a+b-1 current` trước `## History newest=truth Task-909` trong `run-281598/prompt-turn-281603.txt` (và `run-281079`), `5.8` `budget` `Head` giữ, `5.3/5.4` `canonical/*.json` trong sync / `contracts` local-only đã xanh code. `B19` strict `Claude+Codex` coi `grok` là parity `provider-agnostic` (`flow_context_pack_budget.go`) - `grok PASS`, `B20` long-history `drop` là follow-up không block.

## [CHAT] ✅ C7 — Head trước history (B19, chat) — PASSED LIVE grok 2026-08-27

> **Trạng thái:** ✅ **grok parity:** `F6` 4 runs `rag-harness grok` đều `## Canonical ... a+b-1` trước `## History`/`### Source`, không lặp, đúng Head-first (`5.1/5.7`). `Claude/Codex` cùng code path nên `B19` coi done với note `grok`.

Chạy C5 prompt với Claude, rồi mở phiên mới với Codex (đổi provider).
Verify cả 2: prompt-log ## Canonical state trước ## History/### Change History, không lặp.

## [FLOW] ✅ F7 — Context package thứ tự section — PASSED LIVE 2026-08-27 grok

> **Trạng thái:** ✅ **Live-verified:** `run-280863 fd5f56d4071d5682` thứ tự `canonical.head (1) → feature.history (2, ranked) → change.contract/source.dependence (3) → source.excerpt (4) → chat.summary (5)` đúng, chạy `coder`+`tester` 2 lần giống hệt, `Omitted` churn đúng.

Mở /flow → rag-harness, prompt:
Them ham PercentageChecked(part, whole int) (float64, error) vao calc.go. Them test.
Verify prompt coder: thứ tự section theo priority — canonical.head (1) → feature.history (2, ranked theo locus nếu >30 candidates) → change.contract/source.dependence (3) → source.excerpt (4) → chat.summary (5). Chạy 2 lần → thứ tự giống hệt.

## [CHAT|FLOW] ✅ C8 — Panel + sync (B21/B22) — PASSED LIVE 2026-08-27

> **Trạng thái:** ✅ **Live-verified:** `canonical/calc-core.json` `663886fe` `current` có `behavior+signature+decisions` trong `.flowpilot/canonical/` và `manifest` (`5.3`), `contracts` không sync (`5.4`), panel `ProjectsSettings → Canonical Head calc-core` thấy đủ.

- Panel Canonical Head feature calc-core → behavior + chip + rejected + scope-diff.
- Chạy contextsync → manifest có canonical/\*.json, không bao giờ có contracts.ndjson.
-

# P-6 — source.dependence (FLOW only)

## [FLOW] F8 — Blast-radius live (B12/B23)

Mở /flow → context-coding-review-synthesis. Prompt:
Them ham DivideChecked3(a, b int) (int, error) vao calc.go: b==0 -> (0, ErrDivideByZero).
Verify ở bước reviewer_correctness/synthesis:

- Prompt có ### source.dependence: "Sửa Divide ảnh hưởng <dependents> + flows" (live GitNexus).
- Chạy 2 lần → output giống hệt (sorted/deterministic).
- Flow với task không khai file cụ thể (planner chỉ dir) → section rỗng có chủ đích.
- Tắt gitnexus → note "chưa index", không lỗi.
