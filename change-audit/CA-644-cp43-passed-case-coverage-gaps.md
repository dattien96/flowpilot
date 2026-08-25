# CA-644 — CP-43 test-guide marks + coverage gaps for passed cases (C1–C4, F1)

## What

- CP-43-Test-Steps.md: C1, C2, C3, C4, F1 marked `✅ DONE`; F2 marked
  `PENDING-F2` (Nhánh B pass live run-247326/run-249193; Nhánh A hard-block
  still only covered by automated tests).
- Test-only: filled the automated coverage gaps for the passed cases.

## Why

Rà soát lại coverage cho các case đã pass live:

- **F1:** `frozen_contract_events.ndjson` chưa từng có test khẳng định nội
  dung file (chỉ có rejection/cleanup/corrupt-line tests) — verify F1 yêu cầu
  record ở CẢ hai file.
- **C1:** `prepareChangeContract` + `commitChangeContract` (cổng gate thật)
  chưa có test chứng minh contract declared được persist vào
  `contracts.ndjson` (confidence/feature_key/declared_paths) ở mức runner.
- **C2:** path undeclared → inferred chưa có test runner-level khẳng định
  `declared=false`, confidence=inferred, inferred paths phủ diff, không drift.
- **C3:** chưa có test runner-level khẳng định out-of-scope liệt kê đúng file
  code ngoài scope và LOẠI docs (`requirements/`, `change-audit/`).

## Fix

Test-only (không sửa production code; additive, không đụng legacy tests):

- `internal/changecontract/cp43_f1_frozen_records_gap_test.go` (new):
  - `TestFrozenStoreFilesContainF1Fields` — đọc thẳng 2 file ndjson sau
    SaveFrozen + AppendStatus: record có feature_key/intent/declared_paths/
    version/run_id/coder_step_id; event có contract_id + status.
  - `TestFrozenStoreEventsAppendInOrder` — frozen → accepted append 2 dòng,
    đúng thứ tự, reason giữ nguyên.
- `internal/runner/cp43_gate_capture_gap_tests_test.go` (new, 3-provider loop
  claude/codex/grok — Case 1 parity):
  - `TestPrepareChangeContractDeclaredPersistsToStore` — declared → commit →
    `store.GetLatestForRun` đọc lại đúng (confidence declared, feature_key,
    2 declared_paths), out-of-scope rỗng (C1).
  - `TestPrepareChangeContractUndeclaredInfersFromDiff` — không khai →
    declared=false, inferred, paths phủ diff, không drift (C2).
  - `TestPrepareChangeContractOutOfScopeExcludesDocs` — out-of-scope đúng
    `[user_test.go]`, docs bị loại (C3).

## Verification

- `go test ./internal/changecontract/ -count=1` → ok (gồm 2 test mới).
- `go test ./internal/runner/ -count=1 -run 'TestPrepareChangeContract|TestRunContractFreezeNode|TestFlowScopeDrift|TestFlowFrozen|TestScopeEcho|TestHighSeverity|TestFrozenContract'` → ok.
- `go test ./internal/flowgate/... ./internal/contextsync/...` → ok.
- `go test ./internal/structure/...` → 2 failures pre-existing (gitnexus smoke
  phụ thuộc index máy; xác nhận fail y hệt trên tree sạch khi stash).

# ---8<--- flowpilot:change-ledger
feature_key: change-contract
source_doc_id: CP-43
change_type: test
summary: fill automated coverage gaps for passed CP-43 cases C1/C2/C3/F1 and mark them done in the test guide (CA-644)
# --->8---