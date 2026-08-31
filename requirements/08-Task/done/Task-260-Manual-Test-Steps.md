# Task-260 Manual Test Steps — r-additive-tests + safe-fix-contract Chat pointer

Dùng để verify thủ công sau khi deploy runner mới (không cần code, chỉ Chat + gate). Mỗi step đều có Expected để pass/fail rõ.

## Status — 2026-08-31

- **DONE ✅ A1** `M calc_test.go` → `reprompt` `r-additive-tests` (block che bởi `r-reg` khi đổi `!=999`, đã verify `gate-metrics` có `r-additive-tests`; giữ xanh với comment `// A5 warn test` thì `reprompt` đơn lẻ — `run-193072` PASS)
- **DONE ✅ A2** pure `A` `gatesandbox_new_feature_test.go` → không fire ( `run-191933` gate không có `r-additive-tests`, `run-192067` PASS)
- **DONE ✅ A3** polyglot `*Test.kt` `M` → fire (seed `414f240`, `run-191789` lần 1 từ chối đúng, lần 2 `run-191848` `reprompt` `r-additive-tests: src/test/java/com/example/AddTest.kt` PASS — đã fix)
- **DONE ✅ A4** override `calc_test.go` → không fire sau khi fix BOM (`run-192067` không `r-additive-tests` PASS; lần đầu `run-191933` fail do BOM đã fix)
- **DONE ✅ A5** `enforce=reprompt` vs `warn=warn` — đã verify `gate-metrics` `enforce reprompt` ( `run-193072` ), `warn` downgrade như `r-ca` (E2E `task260_gate_wire_test.go` PASS); comment giữ xanh để không bị `r-reg` che
- **DONE ✅ E2E automated:** `go test ./internal/flowgate -run TestRAdditive` 11 PASS, `TestTask260OracleOverride` 2 PASS, `go test ./internal/runner -run TestTask260GateWire` 6 PASS, `TestMergeChatSafeFix` 8 PASS, `tui_program_opts` + `scroll fix` PASS (commit `5bfb4a5`)
- **DONE ✅ A6 `D/R/C`** `D calc_test.go` `run-195099` + `R` `run-195120` `block` với `r-additive-tests` (kèm `r-dep`); `C` đã fix guide sang `AddCopyTest.kt` để tránh `redeclared` — E2E `TestRAdditiveTestsFiresOnDRCStatuses` PASS
- **DONE ✅ B1 `plan` + B2 `code` + B3 `scan`/`non`** `run-195174`/`195259`/`195336` hỏi `có dùng skill nào không` → `safe-fix-contract` / `Không` đúng, `B4` skip `CP-58`, `B5` `run-195870` dedup 1 dòng PASS
- **DONE ✅ C1 `calc.go` helper + C2 doc + C3 defensive copy** `run-196017` `rule_ids=[r-ca,r-contract,r-newtest]` không `r-additive-tests` PASS; `run-197106` doc `A` không fire PASS; `TestTask260GateWire_DefensiveCopy` PASS (`append(nil, ...)`)
- **DONE ✅ Scroll vs history + You-box clamp** `5bfb4a5`→`bbca52d` burst `Up` 3 trong 150ms → `scrollTranscript` `AppModel:2267` + `tuiProgramOpts` + `interactive_catalog` merge `.agents` PASS
- **Gate-sandbox** `414f240` CLEAN `enforce` (seed `AddTest.kt` giữ) — sẵn sàng

## 0. Prep (1 lần)

1. Ensure runner build mới từ branch `flowpilot-opencode` (chứa CA-695).
   ```powershell
   cd apps/local-runner; go vet ./internal/flowgate ./internal/runner
   go test ./internal/flowgate -run TestRAdditive -count=1 -v
   go test ./internal/flowgate -run TestTask260OracleOverride -count=1 -v
   go test ./internal/runner -run TestTask260GateWire -count=1 -v
   go test ./internal/runner -run TestMergeChat -count=1 -v
   ```
   All PASS.

2. Desktop gate mode: mặc định `enforce` (fresh `.flowpilot` → `readGateMode` auto-creates `enforce`). Để test `warn` path, chỉnh `.flowpilot/settings/gate-config.json` → `{"gate_mode":"warn"}`.

3. Chuẩn bị repo test: `git init` temp hoặc dùng `flowpilot` hiện tại. Cần có `test_baseline.json` green (chạy `go test ./...` xanh trước). Diff tính từ `turnStartGitHead` + worktree.

---

## A. Gate r-additive-tests

### A1 — Pre-existing test `M` → reprompt (enforce)

1. Ensure `gate_mode=enforce` (`.flowpilot/settings/gate-config.json` = `{"gate_mode":"enforce"}`).
2. Chat thường (non-flow), posture `code` hoặc `non`, gửi prompt SAU ĐÂY nguyên văn:

```text
Mở file calc_test.go và sửa 1 dòng duy nhất: trong hàm TestAdd đổi dòng if Add(2, 3) != 5 thành if Add(2, 3) != 999. Chỉ dùng Edit tool để sửa file này, không tạo file mới, không sửa file khác. Sau đó kết thúc turn.
```

3. Gửi turn.

**Expected (enforce):** turn KHÔNG complete xanh. Timeline hiện `flow_gate_violation` amber, `status=reprompt`, message chứa `pre-existing test file(s) edited: calc_test.go` + `safe-fix-contract / additive-tests-only / oracle-rule` + hướng dẫn `git checkout`. Runner tự relaunch reprompt (≤2). Fix bằng `git checkout -- calc_test.go` + làm A2 → gate cho qua. Nếu sửa cùng lúc thêm file mới `A` thì vẫn fire (vì `M` vẫn tồn tại).

### A2 — Pure `A` new test file → không fire

1. `gate_mode=enforce`.
2. Reset `calc_test.go` về clean nếu vừa làm A1: `git checkout -- calc_test.go`.
3. Gửi prompt SAU ĐÂY:

```text
Tạo file mới gatesandbox_new_feature_test.go ở thư mục gốc (package main) với nội dung sau, không sửa bất kỳ file test cũ nào:

package main

import "testing"

func TestNewFeature_AdditiveOnly(t *testing.T) {
    if Add(1, 1) != 2 {
        t.Fatalf("want 2")
    }
}

Sau đó kết thúc turn.
```

**Expected:** Không có `r-additive-tests` violation. `gatesandbox_new_feature_test.go` status `A` → `HasNewTestFileAdded=true` nhưng `TamperedTestPaths` rỗng → không fire. Nếu có sửa prod `M` cùng `A` này thì `r-newtest` được thỏa, `r-additive-tests` vẫn không fire. Sau test xóa file: `rm gatesandbox_new_feature_test.go`.

### A3 — Polyglot (Kotlin/TS) → fire

1. Chuẩn bị file cũ đã commit (chỉ làm 1 lần):
```powershell
mkdir -p src/test/java/com/example
# tạo file cũ và commit để nó thành pre-existing (M mới fire, A thì không)
"class AddTest {}" | Out-File -Encoding utf8 src/test/java/com/example/AddTest.kt
git add src/test/java/com/example/AddTest.kt; git commit -m "test: seed Kotlin file for A3"
```
2. Gửi prompt:

```text
Mở file src/test/java/com/example/AddTest.kt và thêm 1 dòng comment // edited for testing. Chỉ sửa file này, không tạo file mới. Sau đó kết thúc turn.
```

**Expected:** `r-additive-tests` fire như A1 (IsTestFile nhận `*Test.kt`). Chi tiết `pre-existing test file(s) edited: src/test/java/com/example/AddTest.kt`. Nếu thay bằng tạo mới `src/foo.test.ts` với status `A` → không fire. Xong revert: `git checkout -- src/test/java/com/example/AddTest.kt`.

### A4 — Overridden file → không fire

1. Tạo override cho file đã tồn tại `calc_test.go` (key là `filepath.Base`):

```powershell
mkdir -p .flowpilot/guard
'{"calc_test.go":{"testName":"calc_test.go"}}' | Out-File -Encoding utf8 .flowpilot/guard/test_overrides.json
```

2. Gửi prompt:

```text
Mở file calc_test.go và sửa 1 dòng: thêm comment // overridden edit ở cuối file. Chỉ sửa file này.
```

**Expected:** Không fire `r-additive-tests` cho file đã override (oracle `IsOverridden` xóa khỏi `Tampered`). Timeline không có violation này. Xóa override để verify ngược:

```powershell
Remove-Item .flowpilot/guard/test_overrides.json
```

Gửi lại prompt A1 → phải fire trở lại. Xong `git checkout -- calc_test.go`.

### A5 — Gate mode warn vs enforce (độ cứng = r-ca)

1. `gate_mode=enforce`: thực hiện lại prompt A1 (sửa `calc_test.go` → `!= 999`). Gửi.

**Expected enforce:** `status=reprompt`, turn bị suppress, reprompt tự chạy.

2. Revert: `git checkout -- calc_test.go`. Đổi gate mode:

```powershell
'{"gate_mode":"warn"}' | Out-File -Encoding utf8 .flowpilot/settings/gate-config.json
```

3. Gửi lại cùng prompt A1.

**Expected warn:** `status=warn`, turn vẫn complete (amber card nhưng không reprompt). Không phải `always-block` như `r-tests`/`r-reg`. Sau test đổi lại `enforce`.

### A6 — D/R/C cũng fire — DONE ✅ `run-195099`/`195120`/`195147`*

**D - Delete:**
```text
Xóa file calc_test.go khỏi repo (dùng tool delete hoặc Bash rm). Không tạo file mới. Sau đó kết thúc turn.
```
**Expected:** fire `pre-existing test file(s) edited: calc_test.go` với status `D`. Revert: `git checkout -- calc_test.go`.

**R - Rename:**
```text
Đổi tên file calc_test.go thành calc_test_renamed.go (dùng Bash mv). Không sửa nội dung. Sau đó kết thúc turn.
```
**Expected:** fire với status `R` (parser map `R`→`M`). Revert: `git mv calc_test_renamed.go calc_test.go` hoặc `git reset --hard HEAD`.

**C - Copy (dùng file Kotlin để tránh duplicate Go):**
```text
Copy file src/test/java/com/example/AddTest.kt thành src/test/java/com/example/AddCopyTest.kt ở cùng thư mục. Sau đó kết thúc turn.
```
**Expected:** fire `C→M` `pre-existing test file(s) edited: src/test/java/com/example/AddTest.kt` (copy phát hiện qua `IsTestFile *Test.kt`). Xóa `AddCopyTest.kt` sau test. Lưu ý: copy `calc_test.go` → `*_copy.go` không match `IsTestFile` (`*_test.go` mới tính) và gây `redeclared TestAdd` nên suite `regressed` che `additive`.

---

## B. Chat Plan/Code auto-inject safe-fix-contract — DONE ✅ `run-195174`/`195259`/`195336`/`195870`

Quan sát prompt thực tế gửi tới provider (log `injectSelectedSkills` hoặc `toolWorkspace` prompt dump).

### B1 — Chat Plan → có pointer

1. Mở Chat mới, `runKind=chat`, Flow không chạy.
2. Chọn posture `plan` (Desktop: ChatPosturePanel chọn tab `Plan` → `Chat: Plan`; TUI: `/mode plan` hoặc `Shift+Tab` đến Plan).
3. Gửi prompt SAU ĐÂY nguyên văn:

```text
Lập kế hoạch sơ bộ cho task: thêm hàm SubtractWithGuard đã có in calc.go - chỉ cần liệt kê các bước plan, không cần code. Trả lời ngắn gọn.
```

**Expected:** Outbound prompt (xem `toolWorkspace` log hoặc `injectSelectedSkills` dump) chứa block:

```text
## Selected Skills

Read each skill file listed below and follow its process before responding.

- /safe-fix-contract → <workspace>/.agents/skills/safe-fix-contract/SKILL.md
  > Operator safety pack for every bug fix and coding task: never break old tests ...
```

Áp dụng cho mọi provider (Claude/Codex/Grok/Gemini/Opencode) — cùng 1 `mergeChatSafeFixContractSkills` path.

### B2 — Chat Code → có pointer

1. Posture `code` (Desktop tab `Code` / TUI `/mode code`).
2. Gửi prompt:

```text
Viết hàm mới DivideChecked4 trong calc.go đã có sẵn, nhưng chỉ mô tả cách làm, chưa cần code ngay. Trả lời ngắn.
```

**Expected:** Có pointer như B1 (`## Selected Skills` + `/safe-fix-contract`). Nếu không thấy → `shouldAutoInjectSafeFixContract` sai.

### B3 — Scan / Non / "" → không inject

1. Chọn `scan` (TUI `/mode scan` / Desktop tab Scan) hoặc `non` (tab Non).
2. Gửi prompt:

```text
Quét nhanh repo xem file calc.go có hàm nào liên quan đến Divide.
```

**Expected:** Outbound prompt KHÔNG chứa `safe-fix-contract`. Prompt giữ nguyên `Quét nhanh...`. Tương tự với posture rỗng (`""`).

### B4 — Flow-engine-driven hoặc non-chat run → không inject

1. Chạy Flow: `/flow task-harness` hoặc `/flow rag-harness` (sẽ sinh child `plan_writer`/`coder` với `flowEngineDriven=true`).
2. Quan sát prompt của coding child (log `injectSelectedSkills` hoặc `promptPrep`).

Hoặc tạo run non-chat:

```text
Tạo run workflow với runKind=workflow (không phải chat) và gửi prompt bất kỳ.
```

**Expected:** Không auto-inject `safe-fix-contract` (CP-58 mới làm Flow wiring). Chỉ hub Chat `plan`/`code` mới inject.

### B5 — Dedup

1. Ở picker `SelectedSkills` chọn thủ công `/safe-fix-contract` (gõ `/safe-fix-contract` hoặc chọn từ skill picker).
2. Posture vẫn `plan` hoặc `code`.
3. Gửi prompt:

```text
Test dedup: hãy liệt kê các skill đang được inject.
```

**Expected:** Prompt chỉ chứa 1 dòng `/safe-fix-contract`, không duplicate (check `strings.EqualFold` + `TrimSpace`). Log `SelectedSkills` length không tăng. Case-insensitive: `/Safe-Fix-Contract` cũng dedup.

---

## C. Negative / Regression checks (prompt để verify không fire oan) — DONE ✅

**C1 — Sửa helper không phải test → không fire — DONE ✅ `run-196017`**

Prompt:

```text
Mở file calc.go và thêm 1 dòng comment // helper comment ở đầu file. Chỉ sửa file này, không đụng tới calc_test.go.
```

Expected: `r-additive-tests` KHÔNG fire (IsTestFile false).

**C2 — Chỉ sửa doc → không fire — DONE ✅ `run-197106` (no gate)**

Prompt:

```text
Mở file change-audit/CA-695-task-260-r-additive-tests-gate.md và thêm 1 dòng // doc edit. Chỉ sửa file này.
```

Expected: không fire (IsDocOrAuditFile).

**C3 — Defensive copy — DONE ✅ `TestTask260GateWire_DefensiveCopy` PASS**

Sau A1 reprompt, chạy:

```powershell
git diff --name-only; git diff --cached --name-only
```

Expected: chỉ hiện `M calc_test.go`, không có `gatesandbox_new_feature_test.go`. `TamperedTestPaths: append(nil, oracle.Tampered...)` nên mutate `oracle.Tampered` không lan sang `TurnResult`.

---

## D. Automated checks (đối chiếu nếu manual fail)

```powershell
cd apps/local-runner
go test ./internal/flowgate -run "TestRAdditive|TestTask260" -v
go test ./internal/runner -run "TestTask260GateWire|TestMergeChatSafeFix" -v
```

- `TestRAdditiveTestsFiresOnTamperedTestPaths` / `...FiresOnModified...ViaTamperedPaths` / `...Polyglot...` / `...DoesNotFireOnAdded...` / `...NotFiringWhenFilteredViaOverrides` / `...FiresOnDRCStatuses` / `...Remediation...` — 11 PASS.
- `TestTask260OracleOverrideClearsTamper` — override xóa tamper.
- `TestTask260GateWire_*` — wire defensive copy + `enforce=reprompt` vs `warn=warn` parity với `r-ca`.

Nếu bất kỳ case nào trong A/B fail mà test trên vẫn PASS → bug nằm ở `gate_hook.go` wire hoặc `readGateMode` (kiểm tra `TamperedTestPaths: append(nil, oracle.Tampered...)` và `loadGateMode`).

## Reference

- Task doc: `requirements/08-Task/done/Task-260-R-Additive-Tests-Gate-Rule.md`
- CA: `change-audit/CA-695-task-260-r-additive-tests-gate.md`
- SD: `SD-20 §2.8 r-additive-tests`
- Code: `flowgate/{rules,evaluate,enforce}.go`, `runner/gate_hook.go:276,348`, `runner/chat_posture.go:273-299`, `runner/interactive_service.go:6503`
