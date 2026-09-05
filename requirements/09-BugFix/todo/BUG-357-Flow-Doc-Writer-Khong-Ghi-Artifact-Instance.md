# BUG-357: flow doc-writer không ghi artifact instance/run — Artifact panel trống sau run (E1 FAIL)

## Metadata

- Document ID: `BUG-357`
- Title: `flow doc-writer không ghi artifact instance/run — Artifact panel trống`
- Phase: `bugfix`
- Status: `open`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-05`
- Last Updated: `2026-09-05`
- Feature Keys: `artifacts, agent-flow-engine`
- Parent Documents: [CP-58-Test-Steps](../../07-Coding-Plan/inprogress/CP-58-Test-Steps.md), [Task-307](../../08-Task/inprogress/Task-307-Harness-Plan-Artifact-Types-And-Panel-Parity.md)
- Child Documents: `none`
- Related Documents: `none`
- Replaces: `none`
- Tags: `artifacts, task-harness, file-artifact, artifact-instances, panel-empty, live-2026-09-05`

## AI Quick View

### Summary

- Symptom (live 2026-09-05, Desktop Projects → Gate-sandbox → Artifacts): cả 2 mục trống — "No local artifacts yet" + "No artifact runs yet" — sau run-548341 mà `plan_writer` đã viết `Task-910-calc-core-lcm.md` qua bound `plan_md` output.
- Expected (CP-58 E1 + Task-307 panel parity): `plan_md` instance hiện với path file thật đã viết.
- Actual: không có gì. Verify trực tiếp: `GET /client/workflow-runs/run-548341/artifacts` chỉ trả 2 finalizer artifacts generic (`final-response.md`, `changes.diff` — dates 2026-06-12 stub), không có `plan_md`/Task file.
- Root cause (code): hạ tầng artifact hiện chỉ có bindings (input) + type registry (resolve path) + seed rows — KHÔNG có chỗ nào materialize per-run OUTPUT instance khi writer hoàn thành (không INSERT `artifact_instances`/`artifact_runs`, không ghi finalizer artifact cho bound outputs). E1 expect thứ chưa từng được implement (Task-307 lo bindings + mirror seeding, thiếu khâu ghi instance lúc writer xong).
- Impact: E1 FAIL; Artifact panel vô dụng cho mọi flow runs (không riêng task-harness); E2 (no-reprompt) vẫn OK độc lập.

### Current Ask

- Quyết: writer completion có phải ghi artifact instance/run không (và ghi ở đâu: finalizer artifacts, Supabase `artifact_runs`, hay cả hai) — rồi implement + panel hiện path đã resolve.
- Capture-only ở bước này: chưa sửa code, chưa thêm test.

### Key Decisions

- D-1: Đánh E1 FAIL thay vì "chưa tìm đúng tab" — đã verify 2 nguồn độc lập (panel + API).
- D-2: Không nhét vào BUG-355/356 — khác layer (artifacts vs history/audit).

### Constraints

- additive-tests-only / oracle-rule như mọi fix.
- Panel đọc từ admin API (`listLocalArtifacts` + `listRuns`) — fix phía ghi (runner), không sửa panel đọc trừ khi cần.

### Open Questions

- Q-1: Instance ghi ở finalizer (local, per-run) hay Supabase `artifact_runs` (sync được)? Panel hiện cả 2 mục — có thể cần cả hai.
- Q-2: `changes.diff` preview "3 files changed" + dates 2026-06-12 stub trong finalizer artifacts có phải placeholder cần thay luôn không?

### Source Refs

- `apps/local-runner/internal/runner/interactive_handlers.go:206-213` — `handleListArtifacts` (finalizer → fallback catalog).
- `apps/local-runner/internal/runner/artifact_type_registry.go:174` — resolve binding paths (đọc, không ghi instance).
- `apps/local-runner/internal/runner/builtin_artifact_bindings.go:24-38` — seed instance IDs (templates, không phải per-run records).
- Live: run-548341 artifacts API 200 với 2 generic items, zero `plan_md`.

## Evidence

- Screenshot operator: Artifacts panel "No local artifacts yet" / "No artifact runs yet" sau run done.
- `curl /client/workflow-runs/run-548341/artifacts` → chỉ final-response + diff_snapshot stub.
- grep toàn `runner/` + `agentpack/`: không có INSERT/append artifact instance nào lúc writer/agent completion (ngoài seed + resolve).

## Root Cause

Thiếu khâu "ghi instance lúc writer xong" trong thiết kế Task-307: bindings bảo writer VIẾT đúng chỗ, nhưng không ai GHI NHẬN đã viết → panel + API per-run không thấy gì. Không phải regression (chưa từng có).

## Fix direction (proposed, NOT implemented)

- Khi node có file_artifact OUTPUT binding hoàn thành (doc-writer/coder/splitter): ghi 1 run artifact record (path đã resolve + runId + nodeId + timestamp) vào finalizer artifacts (local, hiện ngay ở per-run API) và/hoặc Supabase `artifact_runs` (sync).
- Panel hiện path thật (đã resolve placeholders `{{idx}}/{{slug}}` — cũng là regression guard cho E2 nếu writer viết sai path).
- Validation: unit mới (writer completion ghi instance; API trả về; panel mapping) + live rerun task-harness → panel hiện `plan_md` → E1 PASS.
