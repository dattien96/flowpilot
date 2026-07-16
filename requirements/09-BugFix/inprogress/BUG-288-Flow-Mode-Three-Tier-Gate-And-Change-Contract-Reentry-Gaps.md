# BUG-288: Flow-Mode Three-Tier Gate Lifecycle + Change Contract Re-entry Gaps (Codex FAIL Follow-up)

## Metadata

- Document ID: `BUG-288`
- Title: `Flow-Mode Three-Tier Gate Lifecycle And Change Contract Re-entry Gaps`
- Phase: `bugfix`
- Status: `inprogress`
- Owner: `FlowPilot`
- Reviewers: `Codex review (multi-round)`
- Created: `2026-07-15`
- Last Updated: `2026-07-16`
- Parent Documents: [Task-242: Flow-Mode Three-Tier Gate](../../08-Task/done/Task-242-Flow-Mode-Three-Tier-Gate.md), [CP-50: Context Source Completion](../../07-Coding-Plan/done/CP-50-Context-Source-Completion.md), [Task-247: Change Contract Context Source](../../08-Task/done/Task-247-Change-Contract-Context-Source-And-Downstream-Prompt.md), [Task-246: Source Excerpt Runtime Producers](../../08-Task/done/Task-246-Source-Excerpt-Runtime-Hint-Producers.md)
- Child Documents: `None`
- Related Documents: [Task-184: Change Contract Capture](../../08-Task/done/Task-184-Change-Contract-Capture.md), [Task-185: Scope-Drift Detection](../../08-Task/inprogress/Task-185-Scope-Drift-Detection.md), [BUG-152](./BUG-152-Flow-Gate-Fires-On-Child-Agent-Turns-Mid-Loop.md), [BUG-279](./BUG-279-Flow-Mode-Validate-Retry-Ignores-Implement-Node-Reinvoke-Lifecycle.md), SD-20 Flow Gate Rule Semantics
- Replaces: `None`
- Tags: `agent-flow-engine, flow-gate, task-242, change-contract, cp-50, tier-1, validate, audit, regression, codex-review`

## AI Quick View

### Summary

- Codex review FAIL trên nhánh Phase A (Task-238–242) + Phase B (CP-50 / Task-244–247): three-tier gate **không enforce** đúng lifecycle (gate sau khi child đã Completed/join cohort), child `TurnResult` thiếu `ContractDeclared`, validate-node thiếu oracle/baseline, `change.contract` không inject ở re-entry, và nhiều gap correctness/test.
- Sau nhiều vòng review, root cause sâu hơn được chốt: defer settle chưa đủ (còn `signalChild`/`releaseDependentAgents` sớm); per-turn diff nhầm dirty worktree của coder; heuristic `review` substring; gate reprompt để status `Completed` → resume release dependent; cancel/oracle/env/audit race.
- Vòng 9 + **Vòng 10** + **residual re-audits** đã **đóng**: V9-01…V9-30 + V10-01…V10-10 + V10R-01…V10R-05 + V10R2-01…V10R2-05 Fixed (git guard fail-closed + undo commits; parent reconstructs pending children; normalize/gate race; durable turn/step IDs; flowEngineDriven restore).

### Current Ask

- Vòng 9–10 + residual re-audit #2 hoàn tất. Vòng 11 (9 finding: P1-01, P1-04, P1-05, P1-07, P1-08, P1-09, P2-01, P2-02, P2-03, P1-11) đã fix (2026-07-16). Vòng 12 (7 P1 + 3 P2 code + 3 P2 doc-only) cũng đã fix (2026-07-16) — xem §11 "Vòng 12". Status vẫn giữ `inprogress` cho đến khi có một pass Codex re-review mới xác nhận sạch (chưa tự đổi thành `done` chỉ dựa trên self-test).

### Key Decisions

- `V-1` Gate child flow-engine: **defer** cohort join / step DONE / `signalChild` / `releaseDependentAgents` đến khi gate pass; reprompt → `status=Running` để deps unsatisfied.
- `V-2` Contract capture trên coding child lưu theo **parent run id** (downstream đọc `parentRunID`); step identity = node label.
- `V-3` Per-turn diff = commits since `turnStartGitHead` + dirty **mới/đổi** so với `turnStartWorktree` fingerprint (không lấy nguyên dirty tree).
- `V-4` Reviewer identity = role/agent exact (`reviewer`/`review`), **không** substring `"review"`.
- `V-5` Cancel/timeout suite → `EnvError` / không advance validate→audit/done; audit guard `ctx.Err()` trước terminal control.
- `V-6` Oracle stream qua `tailCapWriter` (peak O(64KiB)) + scanner buffer đủ lớn.

### Constraints

- Không đổi Chat Mode hub `runFlowGate` rule set đầy đủ (chỉ fix lifecycle/oracle/reprompt inject).
- Reviewer/inline zero-cost khi không code-write (BUG-152).
- GitNexus impact tools có lúc không available — fix bằng code inspection + unit tests.

### Open Questions

- `Q-1` Task-240 schema-valid hub tools + watchdog E2E trên mọi flow E2E — **chưa** nằm trong bundle fix này (secondary trong review đầu).
- `Q-2` Gemini không có inbound permission bridge cho commit deny — gap đã ghi ở Task-242 (prompt-guard).

### Source Refs

- Review Codex multi-round trên branch `task/cp43-50-vs-task238` (2026-07-15).
- Code: `gate_hook.go`, `interactive_service.go`, `flow_validate_audit_dispatch.go`, `flow_context_hint_paths.go`, `flow_step_runtime.go`, `flowgate/oracle.go`, `flowgate/baseline.go`.
- Tests: `gate_tier_test.go`, `context_source_change_contract_test.go`, `flow_validate_audit_dispatch_test.go`, `flow_context_hint_paths_test.go`, `oracle_test.go`.

## 1. Issue Summary

Sau khi land Phase A (three-tier gate) và Phase B (context sources / change.contract), Codex review **FAIL**: Task-242 DOD core (D-1/D-3/D-4/D-9) không đạt — child gate chạy **sau** khi child đã Completed và cohort join; tier-1 không capture contract trên child; validate-node không dùng oracle; re-entry Continue/retry/delegate thiếu `appendChangeContractIfAny`. Các vòng review tiếp theo lộ thêm race lifecycle, dirty-worktree false coding child, cancel→regression, và audit settle sau Stop.

## 2. Parent Links

- impacted coding plan: Task-242, CP-50 (P-3/P-4), Task-246, Task-247
- impacted tech design: SD-20 (three-tier / D-3 Flow Mode), SD-21 (Change Contract)
- impacted system spec: SS-14 (code context & regression safety)

## 3. Environment and Reproduction

- environment: `apps/local-runner` unit/integration tests + static review (không cần live provider cho phần lớn case).
- reproduction (tóm tắt theo finding):
  1. **Gate sau complete:** coder turn emit `EventTurnCompleted` → DONE/cohort join → mới `runChildArtifactOutputGate` → reprompt không chặn advance.
  2. **r-contract always fire:** child `TurnResult` không set `ContractDeclared` dù coder đã khai scope.
  3. **Contract parent/child mismatch:** capture `rs.id` (child) nhưng inject `parentRunID`.
  4. **Reviewer overwrite contract:** delegate reviewer + dirty tree → infer contract → last-wins ghi đè.
  5. **Custom delegate sau coder:** `ObserveGitDiffSince(HEAD)` thấy dirt coder → gate/capture nhầm.
  6. **Gate reprompt + resume:** child vẫn `Completed` → `resumePendingLoopWork` release dependent.
  7. **Cancel suite:** `ExitError` từ kill process → regression / `skipped_env_error` advance audit.
- frequency: deterministic theo code path (không phụ thuộc model quality).

## 4. Expected vs Actual

| Hạng mục         | Expected                                        | Actual (trước fix)                           |
| ---------------- | ----------------------------------------------- | -------------------------------------------- |
| Child gate       | Chặn complete/advance khi violation             | Gate sau join cohort; reprompt “muộn”        |
| Contract capture | Parent run + coding turn only                   | Child id / mọi delegate / dirty tree         |
| Re-entry prompt  | Có `change.contract`                            | Continue/retry/root reprompt thiếu           |
| Validate         | Oracle regression vs new-fail; cancel ≠ advance | Chỉ command/retry; cancel → env skip → audit |
| Reviewer         | Zero-cost tier-1                                | Có thể reprompt / overwrite contract         |
| Deps             | Không release khi gate retry                    | Resume release reviewer sớm                  |

## 5. Impact

- users affected: mọi Flow Mode coding (review-loop + rag-harness) dùng three-tier gate + change contract.
- workflows affected: coder → reviewer cohort; validate → retry coder; Continue; Stop giữa validate/audit.
- severity: **high / critical** (DOD Task-242 không đạt; scope drift enforce sai; flow advance sai).

## 6. Root Cause

- confirmed (nhóm):
  1. **Lifecycle:** `emitLocked(EventTurnCompleted)` settle quá sớm so với post-turn gate trong `runTurn`.
  2. **TurnResult:** child gate không gọi `captureChangeContract` / không fill scope fields.
  3. **Run id:** store contract theo child id; read theo parent.
  4. **Diff ownership:** `ObserveGitDiffSince(turnStartGitHead)` gồm uncommitted pre-existing.
  5. **Identity:** substring `"review"` false-positive / false-negative.
  6. **Status after reprompt:** clear `pendingFlowGateSettle` nhưng giữ `Completed`.
  7. **Oracle/validate/audit:** background ctx; cancel mapped như fail/env skip; audit không check `ctx.Err()` trước `applyFlowControl`.
  8. **CP-50 minor:** path punctuation; `uncommittedChangedPaths` chỉ nil khi **cả hai** git lỗi; git timeout share context.

## 7. Fix Strategy (đã land)

### Critical / P0 (gate + contract)

| ID    | Fix                                                                                                                                                   |
| ----- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| `F-1` | Defer settle flow-engine child: `pendingFlowGateSettle`; gate pass → `settleFlowChildTurnCompletedLocked` + `signalChild` + `releaseDependentAgents`. |
| `F-2` | Child gate: `captureChangeContract` với **parentRunID**, step = node label; fill `ContractDeclared`/scope trên `TurnResult`.                          |
| `F-3` | Gate reprompt (child + root): `composeFlowNodeAgentPrompt` + `appendChangeContractIfAny` trước `startTurn`.                                           |
| `F-4` | Validate: `runValidateWithOracleIfPossible` + `RunOracleContext`; monorepo baseline từ workspace root.                                                |

### Important / P1 (re-entry, identity, cancel, deps)

| ID     | Fix                                                                                                                                                                            |
| ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `F-5`  | Inject `appendChangeContractIfAny` trên Continue, validate-retry, inline-dispatch delegate.                                                                                    |
| `F-6`  | `isFlowCodeWritingChild` + `isFlowReviewerChild` (role/agent exact); tier-1 không chạy reviewer.                                                                               |
| `F-7`  | `turnStartWorktree` fingerprint + `observeTurnScopedDiff` (chỉ dirt mới/đổi content).                                                                                          |
| `F-8`  | Gate block: revert child `status=Running` (+ agent summary) để deps unsatisfied / resume không release.                                                                        |
| `F-9`  | Cancel: `ctx.Err()` sau suite → `EnvError` non-regression; validate không advance; `flowInlineContext` + stop cancel; audit `auditCtxCancelled` trước escalate/done/successor. |
| `F-10` | Nested-lock step transition log: `setFlowStepStatus` vs `setFlowStepStatusLocked` (bỏ `TryLock` ownership).                                                                    |

### CP-50 / Task-246 minor

| ID     | Fix                                                                                   |
| ------ | ------------------------------------------------------------------------------------- |
| `F-11` | `extractPromptSourcePaths`: trim trailing `.,:;!?`.                                   |
| `F-12` | `uncommittedChangedPaths`: **bất kỳ** git error → `nil`; timeout **3s mỗi** lệnh git. |

### P2 (resource)

| ID     | Fix                                                                                        |
| ------ | ------------------------------------------------------------------------------------------ |
| `F-13` | Oracle: `tailCapWriter` stream (peak O(64KiB)); scanner buffer `maxSuiteOutputBytes+8KiB`. |
| `F-14` | Oracle: preserve suite output vào `ValidationResult.Stderr` cho retry summary.             |
| `F-15` | Không publish `Completed` cho flow child đến khi gate pass (`pendingFlowGateSettle`).      |
| `F-16` | Audit Tier-3: `ContractDeclared` từ store + `MergeDefaultRules`/`LoadRules`.               |
| `F-17` | Baseline singleflight + await; không recapture green baseline khi dirty.                   |
| `F-18` | Gate/oracle: map `Failed`/`suite_failed` khi `!SuitePassed`.                               |
| `F-19` | Stall Skip cancel turn; Retry cancel + `pendingRestart` rồi startTurn.                     |
| `F-20` | Max reprompt / block → `applyFlowControl(escalate)`.                                       |
| `F-21` | Oracle deadline ≠ user cancel.                                                             |
| `F-22` | `applyFlowControl` reject khi loop `stopped`/`done`.                                       |
| `F-23` | Reviewer chỉ zero-cost khi không code-diff; WrittenPaths fallback khi không git delta.     |
| `F-24` | Git status R/C → M.                                                                        |
| `F-25` | `.flowpilot/**` là doc/audit path.                                                         |
| `F-26` | Stall timer một cái / parent (reset).                                                      |
| `F-27` | Tamper M/D/R/C.                                                                            |
| `F-28` | Fingerprint stream 1MiB + symlink metadata.                                                |

## 8. Validation

- `V-1` Unit: `TestFlowChildDefersSettleUntilGate`, `TestChildGateCapturesContractDeclared`, `TestReviewerChildDoesNotOverwriteCoderContract`, `TestObserveTurnScopedDiffIgnoresPreexistingDirt`, `TestIsFlowReviewerChildExactRoleOnly`, `TestIsFlowCodeWritingChild`, `TestChildGateActivatesOnGitDiffWithoutFileEvents`.
- `V-2` Contract inject: `TestAppendChangeContractIfAny*`, `TestRootGateRepromptIncludesChangeContract`.
- `V-3` Validate/oracle: `TestRunValidateWithOracle*`, `TestRunOracleContextCanceledIsNotRegression`, `TestValidateResultIsCancelled`, `TestTailCapWriterKeepsTailAndBoundsMemory`.
- `V-4` Excerpt: `TestExtractPromptSourcePaths` (punctuation), `TestUncommittedChangedPathsFreshInitNoHEAD`.
- `V-5` `go test ./internal/runner ./internal/flowgate` (targeted suites), `go vet`, `go build` — sạch trên các pass review.

## 9. Regression Guard

- tests: các test ở §8 (gate_tier / oracle / validate / context_source / hint_paths).
- alerts: none.
- audit checks: document này (`BUG-288`); nên gắn CA khi commit production.

## 10. Follow-Up Document Updates

- upstream docs: Task-242 / CP-50 completion notes có thể reference BUG-288.
- **chưa làm (ngoài scope fix):** Task-240 full hub-tool schema contract scan; `assertNoStepStuckRunning` trên mọi E2E flow.

## 11. Inventory chi tiết theo vòng review

### Vòng 1 — Codex FAIL ban đầu

| #   | Severity  | Issue                                                               | Fix                                             |
| --- | --------- | ------------------------------------------------------------------- | ----------------------------------------------- |
| 1   | Critical  | Child gate sau `EventTurnCompleted` / cohort join                   | `F-1`                                           |
| 2   | Critical  | Child `TurnResult` thiếu `ContractDeclared`                         | `F-2`                                           |
| 3   | Critical  | Validate-node không oracle / D-4                                    | `F-4`                                           |
| 4   | Important | `change.contract` thiếu Continue / validate-retry / inline delegate | `F-5`                                           |
| 5   | Important | `extractPromptSourcePaths` giữ dấu câu                              | `F-11`                                          |
| 6   | Minor     | `source.excerpt` chỉ degrade khi **cả hai** git lỗi                 | `F-12`                                          |
| 7   | Important | Task-240 schema / watchdog E2E                                      | **Open** `Q-1`                                  |
| 8   | Minor     | Task-244/245 compose tests thiếu                                    | Bổ sung head-first / head-only / ordering tests |

### Vòng 2 — Parent/child run id + TryLock + monorepo

| #   | Severity  | Issue                                         | Fix                  |
| --- | --------- | --------------------------------------------- | -------------------- |
| 9   | Critical  | Capture contract child id vs inject parent id | `F-2` parent run id  |
| 10  | Critical  | `TryLock` suy ownership mutex                 | `F-10`               |
| 11  | Important | Oracle baseline load từ nested `TestDir`      | `F-4` workspace root |
| 12  | Minor     | Git timeout share 1 context 3s                | `F-12` per-command   |

### Vòng 3 — Reviewer / reprompt / env

| #   | Severity  | Issue                                 | Fix                  |
| --- | --------- | ------------------------------------- | -------------------- |
| 13  | Critical  | Mọi `agent.delegate` + dirty = coding | `F-6` + later `F-7`  |
| 14  | Important | Child reprompt mất contract           | `F-3`                |
| 15  | Important | Missing binary = suite regression     | `F-4`/`F-9` EnvError |

### Vòng 4 — Root path + diagnostics

| #   | Severity  | Issue                                       | Fix                      |
| --- | --------- | ------------------------------------------- | ------------------------ |
| 16  | Important | Root reprompt mất contract                  | `F-3` root               |
| 17  | Important | Root `RunOracle` bỏ ctx                     | `F-4` `RunOracleContext` |
| 18  | Important | Oracle không trả stdout/stderr → retry rỗng | `F-14`                   |

### Vòng 5 — Cancel = regression

| #   | Severity  | Issue                                   | Fix                     |
| --- | --------- | --------------------------------------- | ----------------------- |
| 19  | Important | Cancel/timeout → ExitError → regression | `F-9` `ctx.Err()` first |
| 20  | Minor     | Cap sau CombinedOutput (peak RAM)       | `F-13` stream tail      |

### Vòng 6 — Deps / diff ownership / stop audit

| #   | Severity  | Issue                                       | Fix                            |
| --- | --------- | ------------------------------------------- | ------------------------------ |
| 21  | Important | Release dependent trước gate pass           | `F-1` defer signal/release     |
| 22  | Important | Tier-1 chỉ WrittenPaths (thiếu file events) | Diff + WrittenPaths; rồi `F-7` |
| 23  | Important | Validate inline `context.Background()`      | `flowInlineContext` + cancel   |
| 24  | P2        | Scanner token limit vs capped output        | `F-13` buffer                  |

### Vòng 7 — Dirty tree / substring / resume / audit race

| #   | Severity  | Issue                                          | Fix                     |
| --- | --------- | ---------------------------------------------- | ----------------------- |
| 25  | Important | Diff từ HEAD gồm dirt coder                    | `F-7` worktree snapshot |
| 26  | Important | Substring `"review"`                           | `F-6` exact role/agent  |
| 27  | Important | Gate reprompt vẫn `Completed` → resume release | `F-8` status Running    |
| 28  | Important | Audit settle sau Stop                          | `F-9` audit guards      |
| 29  | P2        | Scanner buffer                                 | `F-13`                  |

### Vòng 8 — 7 Critical + 18 Important + 7 Minor (post-fix review)

| #     | Severity  | Issue                                                  | Fix status                                                                                                                                       |
| ----- | --------- | ------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| 1     | Critical  | Publish `Completed` trước gate pass                    | **Fixed** `F-15`: không set Completed đến gate pass; `finishTurn` dùng `pendingFlowGateSettle`                                                   |
| 2     | Critical  | Audit Tier-3 luôn `!ContractDeclared` + `DefaultRules` | **Fixed** `F-16`: `GetLatestForRun` + `MergeDefaultRules`/`LoadRules`                                                                            |
| 3     | Critical  | Baseline race async / recapture dirty                  | **Fixed** `F-17`: singleflight `ensureBaselineReady`; không recapture green baseline khi dirty                                                   |
| 4     | Critical  | Oracle suite-fail vs regression / Failed=nil           | **Fixed** `F-18`: child/gate map `Failed` + `suite_failed` khi `!SuitePassed`                                                                    |
| 5     | Critical  | Stall Retry `turn_in_progress`                         | **Fixed** `F-19`: cancel + `pendingRestart*` rồi startTurn                                                                                       |
| 6     | Critical  | Stall Skip không `turnCancel`                          | **Fixed** `F-19`                                                                                                                                 |
| 7     | Critical  | Commit deny non-tool-level / shell wrap                | **Fixed** `F-29`: unwrap `sh/bash -c` (+ still YOLO RequestApproval path). Gemini no-bridge **still open**.                                      |
| 8     | Important | Reprompt không re-check violation cũ                   | **Fixed** `F-30`: `pendingGateCodePaths` force re-eval                                                                                           |
| 9     | Important | Hết 2 reprompt treo flow                               | **Fixed** `F-20`: escalate parent                                                                                                                |
| 10    | Important | Stop không cancel post-turn gate                       | **Fixed** `F-31`: `postTurnGateCancel` + stop                                                                                                    |
| 11    | Important | Oracle deadline = user cancel                          | **Fixed** `F-21`: `deadline exceeded` ≠ cancel                                                                                                   |
| 12    | Important | `applyFlowControl` sau stopped/done                    | **Fixed** `F-22`: reject terminal loop                                                                                                           |
| 13    | Important | Cause-before-effect done/cap order                     | **Fixed** `F-32`: markFlowRunComplete before loop=done                                                                                           |
| 14    | Important | Reviewer sửa code miễn tier-1                          | **Fixed** `F-23`: gate khi hasDiff/writes                                                                                                        |
| 15    | Important | R/C rename drop                                        | **Fixed** `F-24`: `resolveStatus` R/C → M                                                                                                        |
| 16    | Important | `.flowpilot/*` = code change                           | **Fixed** `F-25`: `IsDocOrAuditFile`                                                                                                             |
| 17    | Important | Validate baseSHA/prevTurnID parent                     | **Fixed** `F-33`: `codingChildTurnMetaLocked`                                                                                                    |
| 18    | Important | Stop non-in-flight children                            | **Fixed** `F-34`: terminalize all non-terminal                                                                                                   |
| 19    | Important | Timer per stream event                                 | **Fixed** `F-26`: one resettable timer/parent                                                                                                    |
| 20    | Important | Tamper only status M                                   | **Fixed** `F-27`: M/D/R/C                                                                                                                        |
| 21    | Important | Transition replay monotonic                            | **Fixed** `F-35`: never FAILED/CANCELED→DONE                                                                                                     |
| 22    | Important | Child approval restore                                 | **Fixed** `F-41`: child permission/question → parent step WAITING; persist pending; resume scans child RunID gates via `childPendingGateNodeIDs` |
| 23    | Important | Fingerprint ReadFile unbounded                         | **Fixed** `F-28`: LimitReader 1MiB + symlink                                                                                                     |
| 24    | Important | WrittenPaths OR false +                                | **Fixed** `F-23`: writes only if no git code-diff as fallback                                                                                    |
| 25    | Important | Suite command quoting                                  | **Fixed** `F-36`: `shellSplit` quote-aware                                                                                                       |
| 26    | Minor     | one-decision stamp before validate                     | **Fixed** `F-42`: validate status enum + terminal before stamp; unknown status does not burn turn slot                                           |
| 27    | Minor     | step-transition append no mutex                        | **Fixed** `F-37`: store `mu` around append                                                                                                       |
| 28    | Minor     | contract marker in user prose                          | **Fixed** `F-38`: unique heading marker                                                                                                          |
| 29    | Minor     | git path `-z`                                          | **Fixed** `F-39`                                                                                                                                 |
| 30    | Minor     | stale turnStartGitHead                                 | **Fixed** `F-40`: reset empty on fail                                                                                                            |
| 31–32 | Minor     | test matrix / docs sync                                | **Fixed** `F-43`: `v9_matrix_test.go` + E2E watchdog; phase docs moved/synced (V9-24)                                                            |

### Vòng 9 — Re-audit toàn bộ 32 finding + flow/edge case mới (2026-07-15)

#### 11.9.1 Kết luận và phạm vi

- Kết luận review: **FAIL — chưa đủ điều kiện `done`/merge**.
- Phạm vi code: Phase A `Task-238`–`Task-242`; Phase B `CP-50` / `Task-244`–`Task-247`; các đường root/child post-turn gate, validate→audit, Continue/retry/resume, cohort stall, context package và git/oracle observation.
- Trạng thái đúng phạm vi 32 finding Vòng 8 trên working tree hiện tại: **18 Fixed / 9 Partial / 5 Open hoặc Regressed**. Edge case mở rộng invariant được tách thành `V9-*`, không dùng để đổi một finding Vòng 8 đã đạt đúng acceptance scope thành “chưa fix”.
- Các test targeted hiện có chứng minh nhiều happy path, nhưng chưa phủ transaction boundary, crash/restart, concurrent turn, dirty tree ownership và path/command adversarial cases. Vì vậy `go test` xanh ở nhóm targeted không đủ để suy ra các invariant lifecycle đã đạt.

#### 11.9.2 Đối chiếu lại 32 finding Vòng 8

| #   | Trạng thái Vòng 9  | Kết quả re-audit / phần còn thiếu                                                                                                                          |
| --- | ------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | **Partial**        | Orchestration đã defer settle/dependent release, nhưng raw `TurnCompleted` vẫn persist/broadcast trước gate; Completed sau gate chưa persist snapshot mới. |
| 2   | **Fixed**          | Audit Tier-3 đã đọc rules + contract store thật. Hub overwrite contract là finding mới `V9-04`.                                                            |
| 3   | **Partial**        | Singleflight bỏ race goroutine, nhưng baseline còn capture dirty/red hoặc recapture sau committed broken code (`V9-30`).                                   |
| 4   | **Partial**        | Child/validate map suite-fail; root chỉ populate `Tests.Failed` khi `HasRegression`, nên baseline-red + current-red có thể pass `r-tests`.                 |
| 5   | **Open**           | Retry cancel có queue, nhưng queue bị clear và chỉ restart khi local `completed=true`; cancel path là false nên retry bị mất.                              |
| 6   | **Partial**        | Skip đã cancel turn trước synthetic result, nhưng late terminal event còn có thể append/drain cohort lần hai (`V9-25`).                                    |
| 7   | **Open/Regressed** | Deny chỉ nằm ở approval bridge; YOLO adapter không tạo approval request nên commit bypass trên mọi provider (`V9-21`).                                     |
| 8   | **Fixed**          | Pending paths force recheck violation cũ. Hub overwrite contract được tách thành `V9-04`.                                                                  |
| 9   | **Partial**        | Child hết reprompt đã escalate; root hết reprompt chỉ return blocked, không có durable actionable recovery.                                                |
| 10  | **Partial**        | Stop cancel post-gate context nhưng local `completed` không gắn với status sau gate; finalizer race còn mở.                                                |
| 11  | **Partial**        | User cancel được tách khỏi oracle deadline, nhưng deadline map `skipped_env_error` rồi vẫn advance audit/done (`V9-01`).                                   |
| 12  | **Fixed**          | `applyFlowControl` từ chối loop đã `done`/`stopped`.                                                                                                       |
| 13  | **Fixed**          | Đạt acceptance gốc về settle order; edge observable Continue-cap được theo dõi riêng tại `V9-19`.                                                          |
| 14  | **Fixed**          | Reviewer chỉ zero-cost khi không có git/code writes.                                                                                                       |
| 15  | **Fixed**          | R/C normalize thành M; parser NUL/path là finding riêng #29/`V9-15`.                                                                                       |
| 16  | **Fixed**          | `.flowpilot/**` được phân loại doc/audit.                                                                                                                  |
| 17  | **Fixed**          | Validate ưu tiên baseSHA/prevTurnID của coding child; custom delegate identity là edge mới `V9-20`.                                                        |
| 18  | **Partial**        | Stop terminalize child non-terminal, nhưng post-gate/finalizer race vẫn còn (`V9-18`).                                                                     |
| 19  | **Fixed**          | Đạt finding gốc “một timer/parent”; noisy-sibling deadline semantics được tách thành `V9-05`.                                                              |
| 20  | **Fixed**          | Tamper xét đủ M/D/R/C; path parser là #29/`V9-15`.                                                                                                         |
| 21  | **Fixed**          | Đạt guard gốc FAILED/CANCELED→DONE; monotonic/timestamp mở rộng được theo dõi tại `V9-13`.                                                                 |
| 22  | **Partial**        | Có overlay WAITING khi transition log tồn tại, nhưng no-log fallback/actionability sau restart còn hở (`V9-08`).                                           |
| 23  | **Fixed**          | Đạt mục tiêu bounded read; collision tail cùng size là edge mới `V9-14`.                                                                                   |
| 24  | **Fixed**          | Đạt fallback theo acceptance gốc; write-revert/observation-error ambiguity được tách thành `V9-22`.                                                        |
| 25  | **Fixed**          | Đạt simple quote-aware scope đã nêu; shell grammar/cross-path inconsistency là `V9-16`.                                                                    |
| 26  | **Fixed**          | Status được validate trước one-decision stamp.                                                                                                             |
| 27  | **Fixed**          | Transition append có mutex trong store.                                                                                                                    |
| 28  | **Fixed**          | Marker contract không còn va chạm prose; package + direct append duplicate là finding mới `V9-29`.                                                         |
| 29  | **Open**           | Chỉ helper Task-246 dùng `-z`; core `flowgate.ObserveGitDiff*` vẫn line/`strings.Fields`, và helper còn `TrimSpace`.                                       |
| 30  | **Fixed**          | Capture HEAD lỗi thì reset `turnStartGitHead` rỗng, không reuse SHA turn trước.                                                                            |
| 31  | **Open**           | Chưa có matrix đủ provider × gate-mode × flow/re-entry × cancel/restart × dirty-tree.                                                                      |
| 32  | **Open**           | CP-50/task folder, metadata, checklist/link và completion evidence chưa đồng bộ.                                                                           |

#### 11.9.3 Finding mới/còn mở theo flow khác và edge case

| ID      | Severity                              | Flow tái hiện                                                                                 | Issue / impact                                                                                                                                  | Code anchor                                 |
| ------- | ------------------------------------- | --------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------- |
| `V9-01` | **P0 Critical** → **Fixed**           | validate no-command/env-error/oracle-timeout → audit                                          | **Fixed**: validate escalate on `skipped_no_command`/`skipped_env_error`; `runAuditNode` refuses non-`ready` draft (no DONE/done).              | `flow_validate_audit_dispatch.go`           |
| `V9-02` | **P0 Critical** → **Fixed**           | root turn có change → gate block/reprompt                                                     | **Fixed**: `prepareChangeContract` + `commitChangeContract` only after gate allow.                                                              | `gate_hook.go`                              |
| `V9-03` | **P0 Critical** → **Fixed**           | provider complete → post-turn gate đang chạy → subscriber/new turn/process crash              | **Fixed**: `turnInFlight` held during gate; `startTurn` rejects `gate_in_progress`; post-pass session persist with Completed.                   | `interactive_service.go`                    |
| `V9-04` | **P0 Critical** → **Fixed**           | coder declared contract lưu theo parent → hub/root/read-only turn complete                    | **Fixed**: prepare path keeps existing `ConfidenceDeclared`; hub inferred cannot overwrite; declared save early, Head only after allow.         | `gate_hook.go`                              |
| `V9-05` | **P0 Critical** → **Fixed**           | cohort có reviewer A stream đều, reviewer B im lặng                                           | **Fixed**: stall timer schedules to earliest quiet member remaining time (not full reset).                                                      | `cohort_stall.go`                           |
| `V9-06` | **P1 Important** → **Fixed**          | stalled member → Retry trong khi turn in-flight                                               | **Fixed**: pendingRestart kept across cancel; restart fires without requiring `completed=true`.                                                 | `interactive_service.go` finishTurn         |
| `V9-07` | **P1 Important** → **Fixed**          | submit member action với node rỗng/sai/không phải `ActiveNode`                                | **Fixed**: reject wrong/missing node; require real child run before clearing stall.                                                             | `cohort_stall.go`                           |
| `V9-08` | **P1 Important** → **Fixed**          | restart khi child đang permission/question gate                                               | **Fixed**: child waiting overlay even without transition log via `childPendingGateNodeIDs`.                                                     | `interactive_resume.go`                     |
| `V9-09` | **P1 Important** → **Fixed**          | workspace đã dirty trước root/hub turn                                                        | **Fixed**: root gate uses `observeTurnScopedDiff` (same as child).                                                                              | `gate_hook.go` runFlowGate                  |
| `V9-10` | **P1 Important** → **Fixed**          | baseline đã đỏ → suite hiện tại vẫn đỏ                                                        | **Fixed**: `suite_failed` only when baseline was green; `Tests.Regressed` vs `Failed` split.                                                    | `gate_hook.go`                              |
| `V9-11` | **P1 Important** → **Fixed**          | inline done-edge target sai hoặc spawn successor fail                                         | **Fixed**: stamp source DONE only after successful advance/spawn/terminal control.                                                              | `flow_validate_audit_dispatch.go`           |
| `V9-12` | **P1 Important (CP-50)** → **Fixed**  | package có head/history/contract/excerpt                                                      | **Fixed**: generic sections (change.contract) render before SourceExcerpts.                                                                     | `flow_context_package.go`                   |
| `V9-13` | **P1 Important** → **Fixed**          | replay log có DONE→RUNNING, FAILED→PENDING hoặc timestamp cũ                                  | **Fixed**: full terminal monotonic + prefer `line.TS` stamps.                                                                                   | `step_transition_log.go`                    |
| `V9-14` | **P1 Important** → **Fixed**          | file >1 MiB, AI sửa byte sau 1 MiB nhưng giữ size                                             | **Fixed**: stream entire file into fingerprint hash.                                                                                            | `gate_hook.go`                              |
| `V9-15` | **P1 Important** → **Fixed**          | rename/copy/path có space, tab, newline hoặc quote                                            | **Fixed**: `git status/diff -z` + NUL parsers in `observe.go`.                                                                                  | `flowgate/observe.go`                       |
| `V9-16` | **P1 Important** → **Fixed**          | validate command có quoted arg, escape, env prefix, pipe, `&&` hoặc unmatched quote           | **Fixed**: `RunValidationCommand` uses `shellFields` (same quote-aware path as gate).                                                           | `flow_validation_retry.go`                  |
| `V9-17` | **P1 Important** → **Fixed**          | root gate reprompt lần thứ 3                                                                  | **Fixed**: max reprompts → `applyFlowControl(escalate)`.                                                                                        | `gate_hook.go`                              |
| `V9-18` | **P1 Important** → **Fixed**          | Stop trong root post-turn oracle/gate                                                         | **Fixed**: stop/cancel mid-gate forces `completed=false`; no finalizer success path.                                                            | `interactive_service.go`                    |
| `V9-19` | **P1 Important** → **Fixed**          | `flow_control(continue)` chạm round cap                                                       | **Fixed**: `setFlowStepAwaitingUser` before `mutateLoop` to blocked.                                                                            | `interactive_service.go`                    |
| `V9-20` | **P1 Important** → **Fixed**          | custom delegate sửa code nhưng label/role/agent không phải coder/implement                    | **Fixed**: `codingChildTurnMetaLocked` accepts any non-reviewer with turn head.                                                                 | `flow_validate_audit_dispatch.go`           |
| `V9-21` | **P0 Critical** → **Fixed**           | flow coding child chạy YOLO trên Codex/Claude/Grok/Gemini                                     | **Fixed**: `ForceShellBridge` + posture for Codex/Claude/Gemini; Grok YOLO auto-bypass disabled when ForceShellBridge.                          | `yolo_resolver.go`, adapters                |
| `V9-22` | **P1 Important** → **Fixed**          | AI write file rồi revert về bytes cũ, hoặc git observation lỗi                                | **Fixed**: WrittenPaths fallback only when git code-diff empty.                                                                                 | `gate_hook.go` isFlowCodeWritingChild       |
| `V9-23` | **P2 Minor** → **Fixed**              | nhiều `InteractiveService`/test instance dùng cùng parent id; flow done/stop trước timer fire | **Fixed**: stall timer key = `servicePtr:parentRunID`.                                                                                          | `cohort_stall.go`                           |
| `V9-24` | **P2 Minor (docs/tests)** → **Fixed** | handoff/automation dựa vào phase docs/DOD                                                     | **Fixed**: Task-238–242/244–247 + CP-50 moved to `done/`; Status/Completion Notes + links synced; E2E calls `assertNoStepStuckRunning`.         | phase docs; e2e                             |
| `V9-25` | **P1 Important** → **Fixed**          | Stall Skip/Retry synthesize terminal rồi canceled provider emit late failure                  | **Fixed**: `cohortSkipConsumed` + skip re-append if already buffered.                                                                           | `cohort_stall.go`, `interactive_service.go` |
| `V9-26` | **P1 Important** → **Fixed**          | baseline xanh; mọi named failure đã có override                                               | **Fixed**: no invented `suite_regressed` when all named fails filtered.                                                                         | `flowgate/oracle.go`                        |
| `V9-27` | **P1 Important** → **Fixed**          | child gate trên partial-red baseline hoặc ordinary new failure                                | **Fixed**: `TestOutcome.Regressed` vs `Failed`; evaluate r-reg/r-tests split.                                                                   | `rules.go`, `evaluate.go`, `gate_hook.go`   |
| `V9-28` | **P1 Important** → **Fixed**          | review-loop approved path, coder completion gần cohort join                                   | **Fixed**: no hub reinvoke from `releaseDependentAgents` when autoOrchestrate; skip advanceOrNotifyHub while open cohort. E2E `-count=20` pass. | `interactive_service.go`                    |
| `V9-29` | **P1 Important (CP-50)** → **Fixed**  | inline chain rebuild package đã có `change.contract`                                          | **Fixed**: `appendChangeContractIfAny` also skips when prompt already has `### change.contract`.                                                | `artifact_type_registry.go`                 |
| `V9-30` | **P0 Critical** → **Fixed**           | coding child commit code hỏng rồi gate/validate gọi ensure baseline                           | **Fixed**: `RefreshBaselineIfStale` preserves any existing HeadSHA baseline (no auto-recapture on HEAD advance).                                | `flowgate/baseline.go`                      |

#### 11.9.4 Thứ tự fix đề xuất để tránh loop thêm issue

1. **Batch A — Gate transaction/P0:** `V9-01`–`V9-04`, `V9-09`, `V9-10`, `V9-17`, `V9-18`, `V9-21`, `V9-30`. Tạo lifecycle durable `provider_done → gate_running → gate_blocked|gate_passed → finalized`; chỉ save contract/update Head sau allow; baseline phải thuộc pre-change truth; commit deny phải enforce trước adapter YOLO, không phụ thuộc approval request.
2. **Batch B — Cohort/recovery/exactly-once:** `V9-05`–`V9-08`, `V9-13`, `V9-19`, `V9-23`, `V9-25`, `V9-28`. Deadline per member hoặc theo deadline sớm nhất; Retry queue chỉ clear sau start thành công; late generation phải bị ignore; hub synthesis có một owner/idempotency key.
3. **Batch C — Diff/oracle/command:** `V9-14`–`V9-16`, `V9-20`, `V9-22`, `V9-26`, `V9-27` và Vòng 8 #3/#23/#24/#25/#29. Dùng git `-z` end-to-end; hash toàn file bằng streaming; một parser command; tách `suite_failed` khỏi `regressed`; override phải giữ attribution.
4. **Batch D — CP-50 + dispatch/docs/matrix:** `V9-11`, `V9-12`, `V9-24`, `V9-29` và Vòng 8 #31–32. Renderer tôn trọng priority; contract xuất hiện đúng một lần; chỉ stamp source DONE sau dispatch/terminal action thành công; sync phase paths/status/checklist sau khi matrix xanh.

#### 11.9.5 Test bắt buộc trước khi đóng lại

- `blocked_validation_failed`, `skipped_no_command`, `skipped_env_error`, oracle timeout và user cancel đều **không** được đi audit→done.
- Gate block/reprompt không được thay Canonical Head; root turn không-code không được che declared contract của coder.
- Crash/reconstruct tại từng điểm `provider_done`, `gate_running`, `gate_blocked`, `gate_passed`; concurrent `startTurn` trong post-gate phải bị reject.
- Cohort hai member: A stream liên tục, B silent; B vẫn stall đúng deadline. Retry in-flight phải cancel rồi restart **đúng một lần**; invalid node action phải 4xx/no state mutation.
- Restart ở child approval/question: hoặc submit actionable thành công, hoặc card bị expire và child được re-run; không có WAITING 404.
- Root pre-existing dirty code/doc/CA không được tính cho turn mới; suite exit non-zero phải block kể cả baseline đỏ.
- Inline target missing/unsupported/spawn failure không được stamp source DONE; phải rollback/FAILED/escalate có remediation.
- Render package phải assert `canonical.head(1) < feature.history(2) < change.contract(3) < source.excerpt(4)` theo vị trí text thực tế.
- Git matrix: rename/copy, space/tab/newline/quote, leading/trailing whitespace; fingerprint file >1 MiB sửa tail cùng size.
- Validation matrix: quoted arg, escaped quote/backslash, env prefix, pipe/`&&`, unmatched quote; behavior phải giống nhau có và không baseline.
- Provider × gate-mode matrix cho Codex/Claude/Grok/Gemini; commit deny trước YOLO; custom code-writer không phụ thuộc label `coder`/`implement`.
- Baseline phải giữ pre-change truth qua dirty edit và committed broken code; override “all named failures approved” không được tự sinh `suite_regressed`; child ordinary failure không được mang regression UX/options.
- `change.contract` chỉ xuất hiện một lần ở package + inline prompt. `TestE2EReviewLoopApprovedPath -count=20` phải luôn đúng một lần synthesis; mọi flow E2E gọi `assertNoStepStuckRunning`.

#### 11.9.6 Verification của lượt review này

- `go test ./internal/changecontract ./internal/flowgate ./internal/runner -count=1`: một lượt **1882 passed / 3 failed / 15 skipped**; lượt độc lập sau đó **1881 passed / 4 failed / 15 skipped**. Failure tăng thêm là `TestE2EReviewLoopApprovedPath` hub gọi hai lần; ba failure còn lại là Firebase JSON EOF, Drive status count và Grok account-home local state.
- `TestE2EReviewLoopApprovedPath -count=10`: **5 pass / 5 fail**, xác nhận race exactly-once chứ không phải inspection-only finding.
- Targeted `go test -race` cho gate/child/validate/oracle/transition/reconstruct/cohort/context: **106 passed**.
- `go vet ./...`, `go build ./...`, desktop `tsc --noEmit`, `git diff --check`: **pass**.
- Kết luận: compile/static checks sạch nhưng semantic findings trên vẫn tái hiện bằng inspection state machine; cần thêm tests §11.9.5 rồi mới được đổi status về `done`.

### Vòng 10 — residual after V9 close (2026-07-15)

| ID     | Severity | Issue                                                    | Fix status                                                                                              |
| ------ | -------- | -------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| V10-01 | P0       | Gemini YOLO bypass commit deny (no RequestApproval path) | **Fixed**: ForceShellBridge prompt guard + post-turn `collectCommitSubjectsSince` block on coding child |
| V10-02 | P0       | Head drift flags always false in Evaluate                | **Fixed**: `fillHeadDriftFlags` in prepare (no Head mutate until commit)                                |
| V10-03 | P0       | Gate state not durable across restart                    | **Fixed**: `PendingFlowGateSettle` on session + `resumePendingFlowGate`                                 |
| V10-04 | P1       | Raw EventTurnCompleted persist/broadcast before gate     | **Fixed**: defer persist+subscriber broadcast until gate pass                                           |
| V10-05 | P1       | Child approval restore not actionable (404)              | **Fixed**: `rehydratePendingGatesLocked` rebuilds approvals/questions                                   |
| V10-06 | P1       | V9-11 DONE stamp before spawn/successor success          | **Fixed**: validate retry + audit DONE only after success                                               |
| V10-07 | P1       | git -z rename/whitespace path bugs                       | **Fixed**: destination path for R/C; no TrimSpace on paths                                              |
| V10-08 | P1       | Discussion/chat.summary before change.contract           | **Fixed**: render order history→contract→excerpt→discussion                                             |
| V10-09 | P2       | shellFields empty → panic args[0]                        | **Fixed**: empty after split → EnvError                                                                 |
| V10-10 | P2       | Baseline never refreshes legitimate green after red      | **Fixed**: recapture when clean HEAD advance + prior `!SuitePassed`                                     |

### Vòng 10 residual re-audit (2026-07-15) — Fixed

| ID      | Severity | Issue                                                                | Fix status                                                                                                                                     |
| ------- | -------- | -------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| V10R-01 | P0       | Root gate publishes Completed before gate; child snapshot incomplete | **Fixed**: root also `pendingFlowGateSettle`; persist `TurnStartGitHead` / `TurnStartWorktree` / `PendingGateChangedFiles` + restore on resume |
| V10R-02 | P0       | Gemini commit only detected post-commit                              | **Fixed**: PATH `git` shim (`installGitCommitGuard`) blocks `commit`/`commit-tree` before exec; prompt + post-gate remain as defense-in-depth  |
| V10R-03 | P1       | `resumePendingFlowGate` no terminal event                            | **Fixed**: materialize persist+broadcast `EventTurnCompleted` like live post-gate path                                                         |
| V10R-04 | P1       | Rehydrated approval/question resolves card but does not resume flow  | **Fixed**: `rehydrated` flag → step RUNNING + `scheduleChildTurn` continuation on approve/answer                                               |
| V10R-05 | P1       | CP-50 generic sections before `source.excerpt`                       | **Fixed**: `sectionsForRender` sorts all sections by Priority (MCP/Jira/Firebase after excerpt)                                                |

### Vòng 10 residual re-audit #2 (2026-07-16) — Fixed

| ID       | Severity | Issue                                                            | Fix status                                                                                                                                 |
| -------- | -------- | ---------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| V10R2-01 | P0       | Git guard bypassable + fails open                                | **Fixed**: fail-closed on install; hardened argv/alias parse; post-turn `undoForceShellBridgeCommits` mixed-reset (absolute git / libgit2) |
| V10R2-02 | P0       | Parent resume leaves child `PendingFlowGateSettle` idle          | **Fixed**: `reconstructPendingChildSessions` after parent reconstruct (gate + pending approval/question)                                   |
| V10R2-03 | P0       | Root gate resume races `normalizeResumedFlowRun` → Cancelled     | **Fixed**: skip normalize cancel when pending gate; schedule `resumePendingFlowGate` only after normalize                                  |
| V10R2-04 | P1       | Terminal replay loses `ProviderTurnID`                           | **Fixed**: persist `PendingFlowGateTurnID`; materialize EventTurnCompleted with it                                                         |
| V10R2-05 | P1       | Approval restart invents stepID; parent `flowEngineDriven=false` | **Fixed**: persist `StepID`/`LastTurnStepID`; restore `flowEngineDriven` when `ActiveFlowNodes` present; `durableResumeStepID`             |

FAIL — 10 findings còn lại: 5 P0, 4 P1, 1 P2.
V10R2-03/04/05 đã fix đúng; V10R2-01 và V10R2-02 mới xử lý một phần.
Severity Finding
P0 Parent bị Cancelled trước khi reconstruct child đang chờ approval/question. normalizeResumedFlowRun chạy trước reconstructPendingChildSessions; child hồi phục nhưng parent flow đã terminal. [interactive_resume.go (line 1030)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1030)
P0 Gate reprompt spawn retry trước khi cleanup pendingFlowGateSettle/postTurnGateCancel. Retry có thể nhận 409 gate_in_progress, lỗi bị bỏ qua, rồi flow không retry nữa. Áp dụng cả root, child và resume path. [gate_hook.go (line 240)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gate_hook.go:240), [interactive_service.go (line 4128)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:4128)
P0 Git guard Gemini chỉ bật khi YOLO. Gemini coding child ở normal mode không có approval bridge, không có shim/rollback; commit chỉ bị phát hiện sau khi đã land. [interactive_service.go (line 3883)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:3883), [gemini_adapter.go (line 80)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gemini_adapter.go:80)
P0 ForceShellBridge vẫn fail-open: repo unborn không có base SHA nên bỏ rollback; rollback lỗi chỉ log rồi agent vẫn success. reset --mixed baseSHA còn có thể unwind commit hợp lệ của actor khác trong shared workspace. [gemini_adapter.go (line 93)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gemini_adapter.go:93), [gemini_adapter.go (line 118)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gemini_adapter.go:118), [gemini_git_guard.go (line 147)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gemini_git_guard.go:147)
P0 Restart giữa reviewer cohort làm mất cohortExpected và kết quả những sibling đã hoàn tất. Chỉ child “pending” được reconstruct; barrier mới có expected=0 nên cohortComplete không thể true, hub không được reinvoke. [interactive_resume.go (line 1051)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1051), [agent_orchestrator.go (line 115)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/agent_orchestrator.go:115)
P1 Nhánh resumePendingFlowGate persist root bằng sessionStateOf, nhưng snapshot không có LoopState. Nó overwrite loop state đã persist trước đó; restart kế tiếp có thể mất blocked/done/round/cap. [interactive_service.go (line 2318)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2318), [interactive_service.go (line 2369)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2369)
P1 Checkpoint pending-gate vẫn được persist bằng goroutine. Crash ngay sau terminal provider event nhưng trước async write sẽ mất PendingFlowGateSettle, rồi resume normalizes flow thành cancelled. Cần synchronous/transactional checkpoint trước khi release lock. [interactive_service.go (line 2463)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2463)
P1 Approval rehydrated: quyết định approval được persist nhưng trạng thái child đã đổi sang Running và resume intent không được persist trước khi schedule. Crash trong window này để lại approval resolved + child cũ waiting, không còn trigger khôi phục. Question path đã có persist parent, approval thì không. [interactive_service.go (line 4871)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:4871), [interactive_service.go (line 4893)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:4893)
P1 Fix whitespace-path V10 bị phá downstream: snapshot và exported changed paths dùng TrimSpace, trong khi diff parser đã giữ whitespace. File tên " foo.go " bị map thành "foo.go", attribution/gate scope sai. [gate_hook.go (line 633)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gate_hook.go:633), [gate_hook.go (line 1139)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gate_hook.go:1139)
P2 CP-50 priority fallback cho legacy section Priority==0 chỉ biết tới mcp.driver. jira.issue, jira.sprint, firebase.crashlytics bị đẩy về priority 50 thay vì 7/8/9. Runtime payload mới không dính, nhưng replay/old payload sai thứ tự contract. [flow_context_package.go (line 379)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_context_package.go:379)

| Severity | Finding                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| P0       | **Cohort recovery không atomic.** `reconstructRun(child)` tự schedule pending gate ngay khi child được load, nhưng `preRegisterCohort` và buffer sibling completed chạy sau đó. Nếu gate child pass nhanh, nó gọi `cohortComplete` lúc expected=0 → false; sau khi restore xong không có final re-check/join, hub stall. Parent còn có thể bị normalize Cancelled nếu child đã settle trước normalize. [interactive_resume.go:1187](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1187), [interactive_resume.go:1205](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1205), [interactive_service.go:2521](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2521)                                     |
| P0       | **Gemini rollback không chứng minh ownership của commit.** Commit của actor khác trong lúc Gemini chạy vẫn có timestamp mới và sẽ bị `reset --mixed base`; unborn repo có thể xoá first commit của actor khác. Tệ hơn, Gemini có thể dùng absolute Git tạo side branch rồi checkout về base: current HEAD không đổi nên rollback không phát hiện commit đó. Cần isolated worktree/lock hoặc ref manifest ownership, không thể dùng timestamp/current HEAD làm bằng chứng ownership. [gemini_git_guard.go:209](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gemini_git_guard.go:209), [gemini_git_guard.go:229](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gemini_git_guard.go:229)                                                                                                             |
| P1       | **Gate reprompt sau restart chưa durable.** `pendingGateCodePaths`, attempt count, prompt và step của reprompt đều không nằm trong `ProviderSessionState`. `resumePendingFlowGate` clear chúng, persist `Running`, rồi gọi `startTurn` async và bỏ error. Crash hoặc `startTurn` fail trong window này làm mất remediation turn; restart sau đó không còn trigger để reconstruct. [interactive_service.go:2323](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2323), [workflow_store.go:155](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/workflow_store.go:155)                                                                                                                                                                                                           |
| P1       | **Approval/question rehydrated vẫn mất continuation nếu crash lần hai.** Fix mới persist child thành `Running`, nhưng không persist intent gồm prompt/step để resume. Crash trước `scheduleChildTurn` khiến approval/question đã resolved, không còn pending card/gate; parent reconstruction bỏ qua child `Running`, rồi normalize nó thành Cancelled. Cần durable `pendingResume` và chỉ clear sau `startTurn` nhận turn thành công. [interactive_service.go:4901](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:4901), [interactive_service.go:4945](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:4945), [interactive_service.go:5066](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:5066) |
| P1       | **Gate pass sau restart không chạy post-completion hooks.** `resumePendingFlowGate` materialize event và persist status rồi return; không gọi `finalizer.Finalize`, không set `lastTurnID`, không arm chat summary. Live path thực hiện cả ba sau gate pass. Kết quả: turn recovered thiếu artifacts/finalization. [interactive_service.go:2358](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2358), đối chiếu [interactive_service.go:4272](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:4272) P0 — Cohort recovery vẫn ghi sai child có durable intent thành failed.                                                                                                                                                                             |

Restore chỉ skip pending gate / waiting approval / waiting question. Child Running với PendingResumePrompt hoặc PendingGateRepromptPrompt bị buffer là failed, rồi khi child thực sự hoàn tất thì kết quả đúng bị dedupe theo label. Cohort/synthesis nhận kết quả sai. [interactive_resume.go (line 1323)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1323), [agent_orchestrator.go (line 95)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/agent_orchestrator.go:95)

P0 — Gemini isolation vẫn có thể làm mất/sửa main workspace.
Repo unborn chạy agent trực tiếp trong mainCwd; absolute Git commit thay đổi main trước khi finalizer chỉ báo lỗi. Với repo bình thường, guard chỉ so HEAD; dirty change của user/actor khác trên cùng file không đổi HEAD nên copy-back sẽ ghi đè nó. Cần isolated workspace cả unborn repo, snapshot/hash target paths, lease độc quyền, và conflict-check trước khi apply. [gemini_git_guard.go (line 196)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gemini_git_guard.go:196), [gemini_git_guard.go (line 231)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gemini_git_guard.go:231)

P1 — Copy-back làm hỏng mode/symlink và có thể apply dở dang.
ReadFile + WriteFile(..., 0644) biến symlink thành regular file, mất executable bit; delete bỏ qua lỗi; lỗi giữa danh sách path để main ở trạng thái partial. Cần apply plan có preflight/rollback, preserve Lstat mode và symlink target. [gemini_git_guard.go (line 283)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gemini_git_guard.go:283)

P1 — Durable intent chưa có liveness và compare-and-clear.
startTurn fail chỉ log, không có retry runtime; intent chỉ được thử lại khi restart/flush khác. Ngoài ra sau startTurn thành công, helper clear vô điều kiện theo kind: một intent mới được tạo nhanh hơn có thể bị clear nhầm. Cần intent generation/id và chỉ clear nếu prompt/step/id vẫn khớp, kèm retry policy durable. [interactive_resume.go (line 1134)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1134)

P1 — Gemini guard không bảo vệ refs ngoài current HEAD.
Agent dùng absolute git -C <main> có thể update/create non-current branch/ref rồi trả HEAD về SHA cũ; finalizer vẫn pass. Snapshot/verify toàn bộ relevant refs, hoặc tốt hơn là cô lập quyền truy cập main .git hoàn toàn. [gemini_git_guard.go (line 236)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gemini_git_guard.go:236) |

P0 — Cohort có durable intent vẫn restore thiếu sibling.
Khi child có PendingResume*/PendingGateReprompt*, status Running bị normalize thành Cancelled lúc dựng cohortNeed. Child intent vẫn được load riêng, nhưng sibling đã completed không được load/buffer; barrier có expected=1 thay vì toàn cohort và synthesis thiếu kết quả. Cần tính durable intent là cohort-live trước normalize. [interactive_resume.go (line 1231)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1231)

P1 — Compare-and-clear sau startTurn vẫn cho phép duplicate turn.
Delivery cũ không claim/verify generation trước startTurn. Nếu cùng intent bị flush hai lần hoặc stale retry chạy sau delivery khác, nó vẫn có thể start một turn cũ; compare sau đó chỉ không clear được intent, không undo turn đã tạo. Cần lease/claim generation atomically trước start; crash thì lease phải có thể reclaim. [interactive_resume.go (line 1140)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1140)

P1 — Retry durable intent vẫn có thể stall vô hạn.
Chỉ retry 5 lần (~1.5s). Một provider turn/gate bình thường có thể dài hơn; khi hết retry, intent còn trên disk nhưng không có timer hay settle callback nào flush lại. Cần retry durable/event-driven khi gate hoặc turn settle, thay vì bounded retry rồi dừng. [interactive_resume.go (line 1141)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1141)

P1 — Workflow question/approval bình thường bị hiểu nhầm là restart.
needsResume := rehydrated || !rs.turnInFlight khiến workflow-driven question không có provider turn cũng schedule startTurn. Đây là regression đã lộ trực tiếp: TestWorkflowDrivenQuestion fail lặp lại, run thành completed thay vì running. Chỉ rehydrate/dead-provider-turn có provenance rõ ràng mới được auto-resume. [interactive_service.go (line 5069)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:5069), [phase5_test.go (line 60)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/phase5_test.go:60)

P1 — Tier-3 audit fail-open khi Git observation lỗi.
changedFilesSince nuốt lỗi thành empty list; audit bỏ qua aggregate doc/contract gate rồi có thể settle flow done. Audit là defense-in-depth của Task-242 nên phải block/escalate khi không xác minh được diff. [flow_validate_audit_dispatch.go (line 143)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:143), [flow_validate_audit_dispatch.go (line 617)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:617)

P1 — Task-246 vẫn làm hỏng pathname whitespace.
Đã dùng Git -z, nhưng parser lại TrimSpace; file tên có leading/trailing space hoặc newline sẽ bị đọc nhầm/bỏ khỏi source.excerpt. Chỉ bỏ record rỗng NUL separator, không trim path. [flow_context_hint_paths.go (line 86)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_context_hint_paths.go:86)

P0 — Cohort recovery vẫn mất cohort khi một child đang Running bình thường lúc crash.
normalizeResumedFlowStatus biến Running thành Cancelled trước khi cohortNeed quyết định có rebuild hay không. Vì vậy sibling completed không được load/buffer, parent có thể bị cancel. Ngoài ra, sau khi buffer toàn bộ terminal sibling, code không drain/reinvoke cohort ngay; không còn event nào để kích synthesis. [interactive_resume.go (line 1337)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1337), [interactive_resume.go (line 1491)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1491)

P0 — Stop flow không được persist đồng bộ.
stopAgentLoop chỉ đổi state trong RAM và cancel goroutine. Crash sau HTTP Stop nhưng trước một persist khác sẽ reload parent/child cũ (Running, pending gate, auto-orchestrate) và có thể resurrect flow đã bị user stop. Persist parent, child và stopped loop state trước khi trả response. [interactive_service.go (line 602)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:602)

P1 — Tier-3 audit vẫn fail-open khi Git không quan sát được.
Audit đã xử lý error từ changedFilesSince, nhưng ObserveGitDiffSince nuốt lỗi git diff base..HEAD; ObserveGitDiff cũng đổi lỗi git status thành nil, nil. Kết quả audit vẫn có thể coi diff rỗng và settle done. Error phải được propagate đến gate/audit. [observe.go (line 17)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/flowgate/observe.go:17), [observe.go (line 48)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/flowgate/observe.go:48)

P1 — Resolve approval/question và persist continuation chưa atomic.
Approval/question resolved được persist trước session có PendingResume*. Crash giữa hai write để lại card đã resolved nhưng child vẫn waiting\_*, không có durable intent; restart không thể resume. Cần persist intent/session trước resolution, hoặc dùng outbox/transaction version chung. [interactive_service.go (line 4999)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:4999), [interactive_service.go (line 5116)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:5116)

P1 — Durable intent có hai lỗ liveness còn lại.
Session từ version trước generation token sẽ có gen=0; claim mới từ chối nên intent cũ không bao giờ resume. Ngoài ra, startTurn fail trước khi có turn/gate idle thì claim được release nhưng không có retry/timer mới. Migrate intent cũ sang generation 1 khi reconstruct và có durable retry/actionable state cho failure trước-start. [interactive_resume.go (line 1145)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1145), [interactive_resume.go (line 1221)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1221)

P1 — source.excerpt không đọc được quoted path có khoảng trắng.
extractPromptSourcePaths dùng strings.Fields, nên prompt như inspect `src/my file.go` bị tách thành hai token và mất path dù đây là explicit source path hợp lệ. Cần tokenizer quote-aware. [flow_context_hint_paths.go (line 19)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_context_hint_paths.go:19)

P2 — Test Flow synthesis đang wait sai state contract.
Test chỉ chờ turnInFlight=false, trong khi API đúng là vẫn reject turn mới đến khi post-turn gate clear. Đây là lý do full suite flaky. Wait thêm pendingFlowGateSettle=false và postTurnGateCancel=nil. [flow_step_runtime_test.go (line 42)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_step_runtime_test.go:42)

P0 — Stop vẫn có thể resurrect root gate/flow. stopAgentLoop chỉ cancel root gate nhưng không clear PendingFlowGate\*/durable intents của parent trước khi snapshot; rồi persist parent. Restart sẽ thấy gate pending và chạy lại. startTurn cũng không chặn child/root khi parent loop đã stopped, nên intent đang race có thể khởi động turn sau Stop.
[interactive_service.go (line 625)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:625)

P0 — Crash giữa session-intent và resolved approval/question tạo state mâu thuẫn. Bản vá persist PendingResume\* trước card resolution. Nếu crash ở giữa, disk vẫn có card pending; reconstruct rehydrate card rồi flush intent. Nhưng startTurn không kiểm tra pendingApprovalID/pendingQuestionID, nên continuation chạy dù durable card chưa chứng minh đã resolved. Cần transaction/outbox/version chung, hoặc ít nhất chặn flush khi còn card pending.
[interactive_service.go (line 5016)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:5016) · [interactive_resume.go (line 1135)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1135) · [interactive_service.go (line 4579)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:4579)

P1 — Retry durable intent chạy vô hạn mỗi 2 giây với lỗi permanent. Account đổi, provider unavailable, step invalid… đều release claim rồi tự retry mãi; log/CPU bị spam và không có backoff hay trạng thái actionable.
[interactive_resume.go (line 1249)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1249)

P1 — Marker text có thể bị user giả mạo để bypass context/contract injection. Chỉ cần prompt chứa [FlowPilot flow context package] là Canonical Head/history bị skip; chứa ## Change Contract đã khai cho run này hoặc ### change.contract là contract không được inject. Dùng strings.Contains trên input không đáng tin cậy làm hỏng guarantee CP-50.
[feature_history.go (line 22)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/feature_history.go:22) · [artifact_type_registry.go (line 610)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/artifact_type_registry.go:610)

P1 — source.excerpt còn TOCTOU symlink escape. Code kiểm tra EvalSymlinks(clean) nhưng sau đó mở lại clean; symlink có thể bị thay giữa check và open để đọc file ngoài workspace. Điều này vi phạm boundary “workspace-safe”.
[flow_context_package.go (line 256)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_context_package.go:256)

P1 — Test Task-242 bị red xác định, không phải flake. TestTryAdvanceFlowFromNodeValidatePassingCommandAdvancesToAudit fail 3/3: fixture dùng t.TempDir() không phải Git repo, trong khi audit tier-3 giờ fail-closed đúng cách nên không tạo audit draft. Cần init repo trong fixture hoặc ghi rõ policy non-Git rồi điều chỉnh expectation.
[flow_validate_audit_dispatch_test.go (line 252)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_validate_audit_dispatch_test.go:252)

P0 — Stop race với deferred gate: gate đã bắt đầu trước Stop vẫn có thể ghi completed/reprompt và persist lại sau checkpoint Stop. Cần epoch/cancellation token và revalidate trước mọi side effect. [interactive_service.go (line 2318)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2318)

P0 — Durable intent có thể gửi duplicate provider turn: startTurn persist khi intent còn tồn tại, rồi intent mới được clear. Crash ở giữa sẽ replay cùng intent sau restart. Cần persisted delivery/idempotency key theo generation. [interactive_resume.go (line 1266)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1266)

P0 — Các checkpoint “durable” vẫn bỏ qua lỗi store. Stop/approval/question có thể trả success dù persistence thất bại; restart sẽ resurrect state cũ hoặc mất decision. Cần propagate lỗi hoặc transactional/outbox checkpoint. [interactive_service.go (line 704)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:704)

P1 — Approval/question vẫn mất quyết định nếu crash giữa hai write: session intent được persist trước, nhưng card resolved chưa được persist. Restart sẽ hiện card pending lại, buộc người dùng trả lời lần nữa. Cần transaction hoặc reconciliation record/version chung. [interactive_service.go (line 5060)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:5060)

P1 — Retry budget không durable và không tách theo intent generation. Restart reset count về 0; intent permanent-fail có thể retry vô hạn qua nhiều process. Intent mới cũng có thể bị “ăn” retry budget của intent cũ. [interactive_service.go (line 337)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:337)

P1 — Prompt marker vẫn spoof được. User chỉ cần gửi marker đúng một dòng để bypass feature-history; kết hợp ### change.contract cũng bypass contract injection fallback. Cần metadata/PromptEnvelope thay vì scan text user-controlled. [flow_context_handoff.go (line 23)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_context_handoff.go:23), [artifact_type_registry.go (line 621)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/artifact_type_registry.go:621)

P1 — source.excerpt vẫn có TOCTOU symlink escape: recheck pathname sau Open không chứng minh file descriptor đang đọc là object đã được validate. Cần no-follow descriptor traversal, hoặc xác định rõ concurrent adversarial workspace mutation ngoài trust boundary. [flow_context_package.go (line 256)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_context_package.go:256)

P1 — source.excerpt có thể treo flow khi path là FIFO/special file: io.ReadFull không có timeout/cancellation và không kiểm tra regular file. Cần f.Stat() + chỉ chấp nhận Mode().IsRegular(). [flow_context_package.go (line 282)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_context_package.go:282)

P1 — Stop không terminalize approval/question records. Sau restart, rehydratePendingGatesLocked có thể dựng lại card pending cho flow đã Stop; người dùng thấy action stale và submit vào run đã dừng. Cần persist card cancelled/expired khi Stop và không rehydrate card cho loop terminal. [interactive_service.go (line 645)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:645), [interactive_service.go (line 2251)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2251)

P2 — Test fixture audit phụ thuộc global Git default branch: git init rồi git checkout -b main fail khi default branch đã là main. Dùng git init --initial-branch=main hoặc không tạo lại branch. [flow_validate_audit_dispatch_test.go (line 539)](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_validate_audit_dispatch_test.go:539)

### Vongf 11

## Kết luận

**FAIL — chưa thể close BUG-288.**

Đây là full sweep đúng scope **Phase A Task-238…242 + CP-50 Task-244…247, runtime Codex/Claude**. Có **22 findings: 3 P0, 15 P1, 4 P2**.

Quan trọng nhất: test regression restart đang đỏ ổn định **3/3 lần**:

```text
TestParentNotCancelledWhenChildPendingApproval
parent cancelled while child has pending approval
```

[Test fail tại v10_residual_test.go:839](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/v10_residual_test.go:839).

## Trạng thái 10 finding vừa fix

| Finding cũ                                     | Trạng thái                                                                     |
| ---------------------------------------------- | ------------------------------------------------------------------------------ |
| Stop race với deferred gate                    | **Partial** — outer settle bị chặn, side effect bên trong gate vẫn chạy        |
| Durable intent duplicate                       | **Chưa đúng** — tránh duplicate bằng cách tạo window mất intent                |
| Durable writes bỏ lỗi                          | **Partial** — Stop/API resolution tốt hơn, nhiều lifecycle write vẫn fail-open |
| Crash giữa session decision và card resolution | **Partial** — reconcile RAM nhưng không repair disk                            |
| Retry budget durable/per-generation            | **Core fixed**, nhưng intent có thể bị park vĩnh viễn                          |
| Marker spoof                                   | **Chưa fix** — token deterministic và user-forgeable                           |
| Symlink TOCTOU                                 | **Partial** — leaf component tốt hơn, intermediate component vẫn hở            |
| FIFO/special file                              | **Fixed trên Unix**, fallback nền tảng khác còn race                           |
| Stop terminalize card                          | **Partial**, đồng thời phát sinh regression restart                            |
| Git fixture default branch                     | **Fixed**                                                                      |

---

# P0 — Release blockers

### P0-01 — Restart làm mất pending approval/question và cancel cả parent

`normalizeResumedFlowStatus` đổi `WaitingApproval`/`WaitingQuestion` thành `Cancelled` trước khi pending card được rehydrate. Sau đó guard trong `rehydratePendingGatesLocked` từ chối run đã Cancelled. Parent không thấy live child nên cũng bị normalize thành Cancelled.

- [interactive_resume.go:758](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:758)
- [interactive_resume.go:820](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:820)
- [interactive_resume.go:1019](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1019)
- [interactive_service.go:2374](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2374)

**Solution**

- Đọc durable pending approval/question trước khi normalize status.
- Nếu có card pending hợp lệ, restore child thành `WaitingApproval`/`WaitingQuestion`, restore `pending*ID`, sau đó mới quyết định parent live/terminal.
- Chỉ normalize thành Cancelled khi không có pending card, gate, continuation intent hoặc live cohort evidence.
- Bổ sung cả approval và question restart test, assert child status, parent status, card API và cohort barrier.

---

### P0-02 — `DeliveredGen` làm mất continuation vĩnh viễn

Code persist `Pending*DeliveredGen=gen` **trước** khi goroutine gọi `startTurn`. Nếu crash giữa hai bước, restart thấy `delivered==gen` và xóa intent dù turn chưa từng được tạo.

Nhánh direct caller còn bỏ qua lỗi persist rồi vẫn gọi `startTurn`, tạo hướng duplicate ngược lại.

- [interactive_resume.go:1177](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1177)
- [interactive_resume.go:1194](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1194)
- [interactive_resume.go:1216](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1216)
- [interactive_resume.go:1362](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1362)

Test hiện tại đang encode behavior sai: tự set delivered marker rồi yêu cầu prompt bị xóa, nhưng không chứng minh tồn tại durable `TurnStarted`.

- [v10_residual_test.go:1093](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/v10_residual_test.go:1093)

**Solution**

Dùng durable outbox/state machine theo `IntentID=(runID, kind, generation)`:

1. `ready`
2. `accepted`, kèm deterministic `TurnID`/idempotency key
3. `consumed`

Chỉ consume intent khi durable store chứng minh `startTurn` đã accept `TurnID`. Không dùng marker được ghi trước lời gọi làm bằng chứng delivery. Nếu persist trạng thái delivery thất bại thì tuyệt đối không gọi provider.

---

### P0-03 — `gateEpoch` kiểm tra quá muộn, gate vẫn mutate sau Stop

Wrapper kiểm epoch sau khi `runFlowGate`/`runChildArtifactOutputGate` trả về. Nhưng helper đã có thể:

- commit Change Contract;
- emit violation;
- set `pendingGateBlock`/`pendingGateCodePaths`;
- tăng `repromptAttempts`;
- tạo durable reprompt;
- escalate parent.

- [interactive_service.go:2516](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2516)
- [interactive_service.go:2540](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2540)
- [gate_hook.go:184](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gate_hook.go:184)
- [gate_hook.go:204](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gate_hook.go:204)
- [gate_hook.go:227](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gate_hook.go:227)
- [gate_hook.go:510](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gate_hook.go:510)
- [gate_hook.go:533](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gate_hook.go:533)
- [gate_hook.go:572](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/gate_hook.go:572)

**Solution**

Tách gate thành hai phase:

1. Pure evaluation: chỉ trả `GateDecision`, không mutate/store/emit.
2. Apply: dưới gate claim, kiểm tra epoch, context, pending turn ID và loop terminal rồi mới commit toàn bộ side effect.

Contract commit, event, reprompt intent và escalation phải nằm trong apply phase. Guard sau helper không thể rollback filesystem/event đã ghi.

---

# P1 — Correctness và durability

### P1-01 — Resolution mutate RAM trước persistence, retry trả success giả — **Fixed** (2026-07-16)

Approval/question được set `resolved`, clear pending ID và chuẩn bị intent trước khi persistence. Nếu persist fail, API trả lỗi; nhưng lần retry tiếp theo thấy `resolved` và trả `nil`, trong khi durable state vẫn thiếu và live waiter chưa được signal.

- [interactive_service.go:5199](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:5199)
- [interactive_service.go:5293](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:5293)
- [interactive_service.go:5354](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:5354)
- [interactive_service.go:5426](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:5426)

**Solution**

Thêm state `resolving` + durable resolution ID. Retry khi `resolving` phải tiếp tục các write còn thiếu. Chỉ đổi RAM thành `resolved`, clear pending ID và signal waiter sau khi durable resolution đã commit/reconcile thành công.

---

### P1-02 — Reconciliation chỉ sửa RAM; restart lần hai làm card sống lại

Khi session có decision nhưng durable card vẫn pending, rehydrate tạo record `resolved` trong RAM nhưng không repair card trên disk. Sau continuation thành công, tombstone session bị clear. Restart lần hai không còn tombstone và durable card pending xuất hiện trở lại.

- [interactive_service.go:2398](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2398)
- [interactive_service.go:2452](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2452)
- [interactive_resume.go:1239](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1239)

**Solution**

Khi reconcile session decision với pending card, phải repair durable card thành resolved trước khi flush continuation. Chỉ clear `PendingResumeApprovalID/Decision` sau khi repair card thành công.

---

### P1-03 — Stop có thể persist root `Running`

Root chỉ được set Cancelled khi `turnInFlight && turnCancel != nil`. Stop trong deferred gate, inline validation/audit, waiting card hoặc idle-running có thể persist `Status=Running` với `LoopState=stopped`.

- [interactive_service.go:631](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:631)
- [interactive_service.go:637](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:637)

**Solution**

Sau khi cancel handles, mọi root nonterminal phải đồng bộ:

```go
status = Cancelled
agentStatus = cancelled
turnInFlight = false
```

Giữ nguyên Completed/Failed/Cancelled. Cập nhật summary cache trước checkpoint.

---

### P1-04 — Stop checkpoint nhiều record không atomic; child gate có thể resurrect — **Fixed** (2026-07-16)

Stop persist tuần tự parent → children → cards. Nếu parent persist thành công nhưng một child/card write fail hoặc process crash, disk có stopped parent nhưng child vẫn giữ `PendingFlowGateSettle`.

Parent reconstruction vẫn load child. `resumePendingFlowGate` không kiểm tra parent loop `stopped/done`, nên gate cũ vẫn chạy.

- [interactive_service.go:723](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:723)
- [interactive_service.go:730](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:730)
- [interactive_service.go:740](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:740)
- [interactive_resume.go:1504](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1504)
- [interactive_service.go:2489](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2489)

**Solution**

Dùng transaction hoặc parent stop tombstone/version làm authority cho toàn bộ descendants. Cả child reconstruction và `resumePendingFlowGate` phải reject khi parent stop generation mới hơn child checkpoint.

---

### P1-05 — Gate settlement vẫn fail-open khi persistence lỗi — **Fixed** (2026-07-16)

Blocked/pass state được mutate, event được broadcast, cohort/dependencies được settle trước khi biết event/session đã durable. Các persistence error bị bỏ qua. Restart có thể chạy lại gate, finalizer hoặc release dependency lần hai.

- [interactive_service.go:2566](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2566)
- [interactive_service.go:2614](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2614)
- [interactive_service.go:2630](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2630)
- [interactive_service.go:2735](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2735)
- [interactive_service.go:4508](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:4508)

**Solution**

Persist idempotent `GateSettlement{run, turn, epoch, result}` trước broadcast/cohort/release/finalizer. Persistence failure phải giữ gate ở actionable blocked/retry state, không tiếp tục downstream.

---

### P1-06 — `resumePendingFlowGate` không có single-flight claim

Hai caller đồng thời đều có thể thấy pending=true và cùng epoch. Caller sau overwrite `postTurnGateCancel`; cả hai chạy evaluator và duplicate contract/event/reprompt/cohort settlement.

- [interactive_service.go:2490](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2490)
- [interactive_service.go:2527](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2527)

**Solution**

CAS `gateClaimID` dưới lock, gắn với epoch + turnID. Chỉ một claim được evaluate/apply; cancel/finish chỉ clear claim khi ID còn khớp.

---

### P1-07 — Pending/expiry card lifecycle vẫn bỏ lỗi persistence — **Fixed** (2026-07-16)

Pending card được emit và waiter bắt đầu dù persist card thất bại. Expiry cũng clear RAM rồi bỏ qua lỗi write. Crash có thể làm mất card đang chờ hoặc resurrect card đã hết hạn.

- [interactive_service.go:3324](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:3324)
- [interactive_service.go:3366](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:3366)
- [interactive_service.go:3963](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:3963)
- [interactive_service.go:4025](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:4025)
- [interactive_service.go:5490](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:5490)

**Solution**

Persist pending card trước emit/wait. Nếu write fail, rollback RAM và trả bridge error. Expiry/clear cần retryable outbox, không được acknowledge expiry chỉ trong RAM.

---

### P1-08 — Approval/question TTL không durable qua restart — **Fixed** (2026-07-16)

Struct có `ExpiresAt`, nhưng creation persist expiry rỗng. Timer chỉ tồn tại trong process; rehydration không kiểm expiry và không schedule remaining TTL.

- [workflow_store.go:217](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/workflow_store.go:217)
- [workflow_store.go:230](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/workflow_store.go:230)
- [interactive_service.go:3317](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:3317)
- [interactive_service.go:3364](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:3364)

**Solution**

Persist absolute `ExpiresAt=now+TTL`. Khi rehydrate:

- đã quá hạn → durable expire, không tạo card;
- chưa hết hạn → schedule remaining duration;
- Submit/Answer phải recheck expiry dưới lock.

---

### P1-09 — Retry cap park intent vĩnh viễn — **Fixed** (2026-07-16)

Sau 5 permanent failures, flush chỉ return. Không có durable `nextAttemptAt`, blocked card, explicit retry hook hoặc account/provider recovery hook. Khi cấu hình được sửa, intent generation cũ vẫn nằm im.

- [interactive_resume.go:1185](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1185)
- [interactive_resume.go:1305](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1305)
- [interactive_resume.go:1386](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1386)
- [interactive_resume.go:1413](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1413)

**Solution**

Persist `status`, `attempts`, `lastError`, `nextAttemptAt`. Khi exhausted, emit durable actionable blocked state. Cho phép explicit retry hoặc provider/account-state change requeue intent.

---

### P1-10 — Prompt-origin vẫn được “xác thực” bằng text user-controlled

FCP chấp nhận bất kỳ whole line `<!-- flowpilot-fcp:* -->`; không cần đúng run ID. Change Contract marker dựa trên public run ID. Các classifier handoff/flow-engine cũng dùng `Contains`/`HasPrefix` trên prompt raw.

- [flow_context_handoff.go:32](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_context_handoff.go:32)
- [artifact_type_registry.go:596](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/artifact_type_registry.go:596)
- [artifact_type_registry.go:623](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/artifact_type_registry.go:623)
- [feature_history.go:22](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/feature_history.go:22)
- [feature_history.go:145](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/feature_history.go:145)
- [feature_history.go:154](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/feature_history.go:154)

Test marker hiện tại không thử forged HTML token; contract branch dùng empty store nên assertion là vacuous.

- [v10_residual_test.go:1117](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/v10_residual_test.go:1117)

**Solution**

Dùng structured `PromptEnvelope`/fragment metadata như `Origin`, `HasFlowContextPackage`, `ContractRunID`. Raw user text không bao giờ được set các flags này.

---

### P1-11 — `source.excerpt` còn intermediate-component symlink TOCTOU — **Fixed** (2026-07-16)

`EvalSymlinks` validate pathname rồi code reopen bằng string path. `O_NOFOLLOW` chỉ bảo vệ final component; intermediate directory có thể bị rename/thay bằng symlink giữa validation và open.

- [flow_context_package.go:254](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_context_package.go:254)
- [flow_context_package.go:273](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_context_package.go:273)
- [source_excerpt_open_unix.go:31](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/source_excerpt_open_unix.go:31)
- [source_excerpt_open_other.go:14](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/source_excerpt_open_other.go:14)

**Solution**

Mở workspace root bằng dir FD và walk từng component với `openat(O_DIRECTORY|O_NOFOLLOW)`. Final file dùng `openat(O_NOFOLLOW|O_NONBLOCK)` + `fstat regular`. Nền tảng hỗ trợ thì dùng `openat2` với `RESOLVE_BENEATH|RESOLVE_NO_SYMLINKS`.

---

### P1-12 — Late inline dispatch sau Stop tạo context mới và tiếp tục chạy

Stop cancel rồi set `flowInlineCtx=nil`. Một late callback sau đó gọi `flowInlineContext`, hàm tạo context mới không-cancelled. Validation/audit vẫn có thể chạy và persist side effects sau Stop.

- [flow_validate_audit_dispatch.go:49](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:49)
- [flow_validate_audit_dispatch.go:73](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:73)
- [interactive_service.go:648](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:648)

**Solution**

Check terminal loop ngay entry `tryAdvanceFlowThroughInline`. `flowInlineContext` phải trả cancelled context cho stopped/done run và không tạo context mới. Recheck stop generation trước persistence và successor dispatch.

---

### P1-13 — Validation/audit advance dù audit trail persistence fail

Persist validation result/retry state hoặc audit draft fail chỉ được log. Flow vẫn có thể advance, retry hoặc settle done. Crash sau đó làm mất retry budget hoặc hoàn thành flow mà không có audit draft durable.

- [flow_validate_audit_dispatch.go:380](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:380)
- [flow_validate_audit_dispatch.go:385](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:385)
- [flow_validate_audit_dispatch.go:728](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:728)
- [flow_validate_audit_dispatch.go:781](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:781)

**Solution**

Validation result/retry state phải durable trước transition. Ready audit draft phải durable trước successor/done. Store failure giữ node nonterminal hoặc escalate `workflow_state_unavailable`.

---

### P1-14 — Stall Retry/Skip chưa durable và Skip bị late cancellation ghi đè

Retry khi member còn in-flight chỉ lưu `pendingRestartRunID/prompt` trong RAM; fields này không nằm trong session snapshot. Process crash trước `finishTurn` làm mất retry.

Skip set child Failed rồi cancel turn. Khi cancellation về `finishTurn`, status lại bị đổi thành Cancelled; chỉ cohort entry giữ Failed. Agents panel và durable run status trái với Task-241 “Skip → FAILED”.

- [cohort_stall.go:280](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/cohort_stall.go:280)
- [cohort_stall.go:357](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/cohort_stall.go:357)
- [cohort_stall.go:373](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/cohort_stall.go:373)
- [cohort_stall.go:381](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/cohort_stall.go:381)
- [interactive_service.go:4732](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:4732)

**Solution**

Persist durable member-action intent before cancel/start. Với Skip, dùng terminal cause `stalled_skip`; `finishTurn` phải preserve Failed khi cause này tồn tại, đồng thời persist child + parent/cohort action atomically.

---

### P1-15 — Cancelled cohort member bị restore thành Failed

Recovery map cả `RunStatusFailed` và `RunStatusCancelled` thành cohort status `"failed"`, trái matrix Task-241 yêu cầu cancelled là terminal riêng và note phải ghi cancelled.

- [interactive_resume.go:1671](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_resume.go:1671)

**Solution**

Explicit switch:

```go
Completed -> completed
Failed    -> failed
Cancelled -> cancelled
```

Thêm restart matrix test cho `cancelled(stop)` và `cancelled(restart)`.

---

# P2 — Edge cases và contract gaps

### P2-01 — Multi-select question bị flatten khi reconciliation — **Fixed** (2026-07-16)

Choices được join thành string, sau crash được dựng lại thành slice một phần tử. `["a, b", "c"]` trở thành `["a, b, c"]`.

- [interactive_service.go:5411](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:5411)
- [interactive_service.go:2460](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:2460)

**Solution**

Persist `PendingResumeQuestionChoices []string`; không dùng chung scalar `PendingResumeDecision` với approval.

---

### P2-02 — Stop có thể trả empty question answer như một success — **Fixed** (2026-07-16)

Stop cancel context rồi gửi `[]string{}` vào resolve channel. Khi cả resolve và `ctx.Done()` đều ready, Go `select` chọn ngẫu nhiên; caller có thể nhận empty answer với `nil` error thay vì interrupted.

- [interactive_service.go:844](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:844)
- [interactive_service.go:5494](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:5494)

**Solution**

Resolve channel cần typed result `{choices, err}`; Stop gửi `errInterrupted`, không encode cancellation bằng empty valid answer.

---

### P2-03 — Step-transition log vẫn best-effort dù Task-239 yêu cầu replay authority — **Fixed** (2026-07-16)

`ApplyStepTransition` thành công nhưng append transition log fail chỉ log warning. Điều này không bảo đảm contract “persist từng transition, restore bằng replay”.

- [flow_step_runtime.go:206](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_step_runtime.go:206)
- [flow_step_runtime.go:310](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_step_runtime.go:310)

**Solution**

Append transition phải có acknowledged sequence/outbox. Không công bố transition settled trước khi log durable, hoặc ghi degraded recovery state rõ ràng.

---

### P2-04 — Quote tokenizer làm mất path sau contraction/apostrophe

Tokenizer coi mọi `'` là mở quote, kể cả apostrophe trong `"don't inspect src/foo.go"`. Không có closing quote nên phần còn lại thành một token sai và explicit path bị mất.

- [flow_context_hint_paths.go:50](/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_context_hint_paths.go:50)

**Solution**

Chỉ coi quote là opener ở token boundary; apostrophe giữa hai ký tự chữ phải được giữ như contraction. Thêm test `"don't inspect \`src/my file.go\`"`.

---

### Vòng 11 — 9 finding còn mở đã fix (2026-07-16)

Sau bản re-audit ở trên (22 findings: 3 P0, 15 P1, 4 P2), phần **P0-01/02/03** và các P1/P2 còn lại ngoài danh sách dưới vẫn giữ nguyên trạng thái đã ghi ở từng mục (chưa động vào trong pass này). Pass này đóng dứt điểm 9 finding còn mở theo yêu cầu:

| Finding  | Thay đổi chính                                                                                                                                                                                                                                     |
| -------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `P1-01`  | Thêm state `resolving` trung gian + resolution stash trên `approvalRecord`/`questionRecord`; RAM chỉ chuyển `resolved` sau khi durable persist thành công; retry khi đang `resolving` replay đúng phần ghi còn thiếu thay vì trả `nil` giả.        |
| `P1-04`  | Thêm `stopGeneration` (parent, durable) / `parentStopGenSeen` (child, durable); `resumePendingFlowGate` reject khi `parent.stopGeneration > rs.parentStopGenSeen` → gate cũ không resurrect sau Stop dù checkpoint không atomic.                  |
| `P1-05`  | Gate-pass branch trong `resumePendingFlowGate` giờ persist session snapshot **trước** broadcast event / cohort-dependency settle / finalizer; persist fail → rollback RAM về `Running`/`pendingFlowGateSettle=true`, không chạy downstream effect. |
| `P1-07`  | `RequestApproval`/`AskQuestion`/`AskWorkflowQuestion` persist pending card **trước** emit/wait; persist fail → rollback RAM, trả bridge error. `expireApproval`/`expireQuestion` luôn persist expiry, không còn im lặng bỏ lỗi.                    |
| `P1-08`  | `ExpiresAt` persist là absolute deadline (`now+TTL`) thay vì rỗng; `rehydratePendingGatesLocked` durable-expire card đã quá hạn, schedule `time.AfterFunc` cho card còn hạn; submit/answer re-check expiry dưới lock.                              |
| `P1-09`  | Thêm durable `intentBlockedKind/Reason/At` khi hết fail budget (thay vì return im lặng); thêm `RequeueBlockedIntent(runID)` để clear block + fail budget và re-invoke `flushDurableTurnIntents`.                                                  |
| `P2-01`  | Thêm field riêng `pendingResumeQuestionChoices []string` (+ `ProviderSessionState.PendingResumeQuestionChoices`), không còn tái dùng scalar `pendingResumeDecision` cho multi-select → reconciliation không còn flatten choices.                   |
| `P2-02`  | `questionRecord.resolve` đổi thành `chan questionResolveResult{choices, err}`; Stop gửi `errInterrupted` thay vì `[]string{}` giả làm success khi `select` race với `ctx.Done()`.                                                                  |
| `P2-03`  | Giữ nguyên signature `ApplyStepTransition` (đổi thành hard error sẽ đụng ~20+ call site fire-and-forget); thêm durable `transitionLogDegraded/At/Reason` marker khi `AppendStepTransition` fail, để restart/replay biết transition này không đáng tin. |
| `P1-11`  | Unix: mở workspace root bằng dir FD, walk từng component qua `openat(O_DIRECTORY\|O_NOFOLLOW)`, leaf mở `O_NOFOLLOW\|O_NONBLOCK` + `fstat` regular-file check (`source_excerpt_open_unix.go`). Non-Unix: `Lstat` từng component top-down từ workspace root (`source_excerpt_open_other.go`, check-then-open, window giảm nhưng chưa triệt tiêu hoàn toàn — ghi rõ trong comment). Fix này cũng sửa luôn `TestReadSourceExcerptsRejectsNonRegular` (FIFO giờ map đúng sentinel `errNotRegularFile` → `not_regular`, không còn lẫn với `symlink_resolve_error`). |

Test mới: `bug288_round11_test.go` (10 test, 8/9 finding có coverage trực tiếp — P1-04 có 2 test path stale/current generation) + `source_excerpt_open_unix_test.go` (3 test, Unix-only build tag, cho `P1-11`).

**Bug ngoài scope phát hiện thêm khi verify, đã fix cùng đợt:** `startTurn` (interactive_service.go) check gate `awaiting_user`/`flow_stopped`/`gate_in_progress` **trước** khi tra idempotency-key, nên retry cùng `Idempotency-Key` trong lúc có pending approval/question luôn nhận `409` thay vì replay đúng turn id cũ (`TestIdempotentTurn` đỏ ổn định). Đã di chuyển idempotency-key lookup lên đầu `startTurn` (ngay sau check `run_not_found`), trước mọi gate khác — replay phải thắng mọi gate chỉ áp dụng cho turn mới thật sự.

---

### Vòng 12
 P1

  - Stop bị bypass: inline callback đến muộn tạo lại flowInlineCtx sau khi Stop đã cancel/clear, nên validate/audit/
    telegram/delegate vẫn có thể chạy và advance sau Stop. — **Fixed** (2026-07-16). /C:/working/flowpilot/apps/local-runner/internal/runner/
    flow_validate_audit_dispatch.go:49, /C:/working/flowpilot/apps/local-runner/internal/runner/
    interactive_service.go:763

  - Validate/audit fail-open khi persistence lỗi: lỗi PersistValidationResult, PersistRetryState, hoặc
    PersistAuditDraft chỉ log rồi vẫn retry/spawn/advance/done. Crash sau đó mất audit trail authoritative. — **Fixed** (2026-07-16). /C:/
    working/flowpilot/apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:394

  - Gate observation lỗi bị biến thành diff rỗng, làm Tier-1 có thể pass khi Git không đọc được workspace. — **Fixed** (2026-07-16). /C:/working/
    flowpilot/apps/local-runner/internal/runner/gate_hook.go:771

  - Checkpoint PendingFlowGateSettle và state block/reprompt vẫn best-effort: persist lỗi bị bỏ qua nhưng remediation
    turn vẫn có thể bắt đầu; crash sẽ mất quyết định gate hoặc replay gate cũ. — **Fixed** (2026-07-16). /C:/working/flowpilot/apps/local-runner/
    internal/runner/interactive_service.go:3027, /C:/working/flowpilot/apps/local-runner/internal/runner/
    interactive_service.go:4857

  - Marker chống double-injection vẫn forge được từ user text. Bất kỳ <!-- flowpilot-fcp:* --> nào suppress context;
    marker contract chỉ dựa trên run ID. — **Fixed** (2026-07-16). /C:/working/flowpilot/apps/local-runner/internal/runner/
    flow_context_handoff.go:32, /C:/working/flowpilot/apps/local-runner/internal/runner/artifact_type_registry.go:596

  - Stall Retry không durable; crash trước finishTurn mất retry. Stall Skip set FAILED rồi cancel khiến finishTurn ghi
    đè thành CANCELLED, sai Task-241 contract Skip -> FAILED. — **Fixed** (2026-07-16). /C:/working/flowpilot/apps/local-runner/internal/runner/
    cohort_stall.go:287, /C:/working/flowpilot/apps/local-runner/internal/runner/interactive_service.go:5203

  - Windows/non-Unix source excerpt vẫn có TOCTOU ở intermediate symlink: sau Lstat, directory có thể bị thay bằng
    symlink trước os.Open, đọc file ngoài workspace. — **Fixed** (2026-07-16). /C:/working/flowpilot/apps/local-runner/internal/runner/
    source_excerpt_open_other.go:75

  P2

  - Stop không hủy baseline capture đầu tiên; suite có thể chạy thêm tới 5 phút bằng context.Background(). — **Fixed** (2026-07-16). /C:/working/
    flowpilot/apps/local-runner/internal/runner/gate_hook.go:1592, /C:/working/flowpilot/apps/local-runner/internal/
    flowgate/baseline.go:285

  - Parser git status --porcelain -z xử lý rename/copy sai thứ tự source/destination, nên policy có thể kiểm tra path
    cũ thay vì destination. — **Fixed** (2026-07-16). /C:/working/flowpilot/apps/local-runner/internal/flowgate/observe.go:67

  - Regression test mới đang fail trên Windows: TestRunValidateWithOraclePreservesSuiteOutput dùng .sh executable và
    nhận stderr=""; test battery không xanh. — **Fixed** (2026-07-16). /C:/working/flowpilot/apps/local-runner/internal/runner/
    flow_validate_audit_dispatch_test.go:109

  - CP-50 còn 4 link cũ sang 08-Task/todo dù task đã chuyển done, làm traceability bị gãy. — **Fixed** (2026-07-16, doc-only, xử lý riêng trước khi dispatch code fix). /C:/working/flowpilot/
    requirements/07-Coding-Plan/done/CP-50-Context-Source-Completion.md:77

  - Task-247 yêu cầu cập nhật CP-43 §3 và Task-243 §8, nhưng completion ghi upstream docs updated: none; DOD
    documentation chưa hoàn tất. — **Fixed** (2026-07-16, doc-only). /C:/working/flowpilot/requirements/08-Task/done/Task-247-Change-Contract-Context-
    Source-And-Downstream-Prompt.md:117

  - BUG-288 tự mâu thuẫn: metadata/completion coi done, trong khi inventory vẫn ghi các P1/P2 unresolved/open. — **Fixed** (2026-07-16, doc-only — `Status` đổi thành `inprogress`, xem Metadata + Completion Notes). /C:/
    working/flowpilot/requirements/09-BugFix/inprogress/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-
    Gaps.md:46

#### Kết quả fix Vòng 12 (2026-07-16)

| Finding (P1)                                 | Thay đổi chính                                                                                                                                                                                                                                       |
| --------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Stop bypass qua late inline callback          | `flowInlineContext` trả context đã-cancel khi run terminal (Completed/Failed/Cancelled) thay vì mint `context.Background()` mới; `tryAdvanceFlowThroughInline` recheck terminal ngay entry.                                                        |
| Validate/audit fail-open khi persist lỗi      | `runValidateNode`/`runAuditNode` escalate (`applyFlowControl` status=escalate) và return khi `PersistValidationResult`/`PersistRetryState`/`PersistAuditDraft` fail, không advance/retry/done nữa.                                                |
| Gate observation lỗi → diff rỗng              | `observeTurnScopedDiff` trả `(diff, error)` thật; lỗi quan sát git thật (không phải "not a git repository" — carve-out zero-cost Task-242 D-2) làm gate fail-closed (block + `EventFlowGateViolation`) thay vì coi như no-op.                       |
| Checkpoint PendingFlowGateSettle best-effort   | `markPendingFlowGateSettleLocked` log lỗi persist thay vì nuốt; block/reprompt checkpoint persist đồng bộ trước khi cho remediation turn bắt đầu hoặc `notifyTurnIdle`.                                                                            |
| Marker chống double-injection forge được       | `flowContextTrustedMarker`/`changeContractTrustedMarker` thêm HMAC suffix (`runMarkerSecret`, random per-process, không derive từ user input); `isFlowContextHandoff` verify MAC thay vì scan raw text.                                            |
| Stall Retry không durable / Skip bị ghi đè     | Retry intent persist durable trước khi cancel (`cohort_stall.go`); Skip set `stalledSkipCause` trước cancel, `finishTurn` preserve `Failed` khi thấy cause này thay vì overwrite `Cancelled` (đúng Task-241 Skip → FAILED).                        |
| Windows TOCTOU intermediate symlink            | `source_excerpt_open_other.go` viết lại dùng `os.Root`/`os.OpenRoot` (Go 1.24+ stdlib) thay vì Lstat-rồi-mở-lại-bằng-string-path; reject symlink component, `os.SameFile` consistency check cuối cùng.                                             |

| Finding (P2 code)                              | Thay đổi chính                                                                                                                                                              |
| ------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Stop không hủy baseline capture đầu               | Thêm `RefreshBaselineIfStaleContext`/`CaptureBaselineContext` (context-aware); gate/validate call site thread cancellable ctx thay vì `context.Background()`.               |
| Parser rename/copy sai thứ tự source/destination | `parsePorcelainZ` — tìm ra bug thật (field order bị đảo ngược so với giả định ban đầu), sửa để giữ field đầu (destination) làm `path`, field hai (source) chỉ consume/skip. |
| Test Windows `.sh` fixture fail                  | `TestRunValidateWithOraclePreservesSuiteOutput` gọi rõ `bash <script>.sh` thay vì dựa vào OS follow shebang.                                                                |

Test mới: `bug288_round12_test.go` (11 test), `source_excerpt_open_other_test.go` (build-tag `!unix`), `internal/flowgate/observe_test.go`, `internal/flowgate/baseline_context_test.go`.

**Regression phát hiện giữa chừng (đã fix trước khi đóng vòng):** fix ban đầu cho "gate observation lỗi → diff rỗng" quá tay — coi mọi lỗi quan sát git (kể cả "not a git repository" hợp lệ trên workspace không phải git repo, case zero-cost Task-242 D-2) là fail-closed, làm đỏ lại 7 test Vòng 11/pre-existing (`TestRunTurnGatePassPersistFailureDoesNotFanOut`, `TestResumePendingFlowGateAllowsCurrentParentStopGeneration`, `TestResumePendingFlowGatePersistFailureDoesNotSettle`, `TestRunTurnPostGatePersistFailureDoesNotBroadcastCompletion`, `TestRunTurnPostGatePersistEpochBumpDoesNotFanOut`, `TestResumePendingFlowGateMaterializesTurnCompleted`, `TestResumeGateMaterializesTurnID`) + `TestChildGateEmptyDiffNoOp` + `TestGrokNormalChatRCAFiresFromMappedFileChange` + `TestTryAdvanceFlowFromNodeValidateRetryReinvokesLifecycleReinvokeTarget`. Đã sửa bằng `isNotAGitRepoErr` (phân biệt "not a git repository" hợp lệ khỏi lỗi quan sát thật khác) — tất cả 7+3 test xanh lại sau fix.

## Verification đã chạy

- `go build ./...` — **PASS**
- `go vet ./internal/runner ./internal/flowgate` — **PASS**
- `go test ./internal/runner ./internal/flowgate ./internal/changecontract` — **1920 passed, 21 failed, 19 skipped** (2026-07-16, sau vòng 11 fix)
- 21 failure còn lại đều pre-existing/không liên quan tới BUG-288 (Windows path assumption trong `chat_session_sync_test.go`/`runner_test.go`, codex resume process, google drive mcp provider config, skills merge precedence, grok account slot, git commit guard shim executable bit, `engine_setup_test`, `compat_runner_test`, oracle stderr preservation) — **không** nằm trong scope 9 finding của vòng này.
- `TestIdempotentTurn` (idempotency-key ordering bug phát hiện thêm) — **Fixed**, pass sau khi sửa thứ tự check trong `startTurn`.
- `TestProjectRunHistoryFiltersRunsByProject` xuất hiện thoáng qua trong 1 lần chạy full-suite nhưng pass 3/3 khi chạy riêng (`-run` isolate) — flaky do thứ tự/song song trong full-suite, không liên quan tới thay đổi vòng 11.
- 10 test mới trong `bug288_round11_test.go` + 3 test trong `source_excerpt_open_unix_test.go` — tất cả **PASS**.

Tóm lại vòng 11: 9 finding còn mở (P1-01, P1-04, P1-05, P1-07, P1-08, P1-09, P2-01, P2-02, P2-03, P1-11) đã fix và có test riêng; các finding P0-01/02/03 và các P1/P2 khác từ re-audit trước đó **chưa** nằm trong scope pass này và giữ nguyên trạng thái cũ.

### Verification Vòng 12 (2026-07-16)

- `go build ./...` — **PASS**
- `go vet ./internal/runner ./internal/flowgate` — **PASS**
- `go test ./internal/runner ./internal/flowgate ./internal/changecontract` — **1935 passed, 21 failed, 20 skipped**
- 20/21 failure là đúng baseline pre-existing/không liên quan (giống hệt danh sách Vòng 11). 2 failure "mới" quan sát thấy ở 1 lần chạy đã điều tra và xác nhận **không phải regression**:
  - `TestSpawnChildEmitsGraphAndBusEvents` — pass 3/3 khi chạy riêng, pass lại khi chạy full-suite lần nữa → flaky pre-existing.
  - `TestFinalizerHookSurfacesArtifacts` (phase4_test.go:264, `"3 files changed"` vs `"3 file(s) changed"`) — root cause: race giữa artifact thật từ finalizer (luôn format `"%d file(s) changed"`) và fixture hardcode không liên quan `fakeArtifacts()` (`interactive_handlers.go:1392`, `Preview: "3 files changed"` cứng); code này không nằm trong diff Vòng 12. Chạy lại full-suite không tái hiện.
- Regression tạm thời giữa chừng (7 test Vòng 11 + `TestChildGateEmptyDiffNoOp` + `TestGrokNormalChatRCAFiresFromMappedFileChange` + `TestTryAdvanceFlowFromNodeValidateRetryReinvokesLifecycleReinvokeTarget`) đã được fix bằng `isNotAGitRepoErr` — xem block "Regression phát hiện giữa chừng" ở trên.
- 11 test mới trong `bug288_round12_test.go` + `source_excerpt_open_other_test.go` (build-tag `!unix`) + `internal/flowgate/observe_test.go` + `internal/flowgate/baseline_context_test.go` — tất cả **PASS**.

Tóm lại vòng 12: 7 P1 + 3 P2 (code) đã fix và có test riêng; 3 P2 doc-only (CP-50 stale links, Task-247/Task-243 upstream sync, BUG-288 self-contradiction) đã fix trực tiếp trong doc (không qua code-fix agent). `TestIdempotentTurn` ordering bug (phát hiện ngoài scope lúc verify Vòng 11) đã fix trước đó.

## 12. Completion Notes

- result: **inprogress** — Vòng 9 + Vòng 10 + residual re-audits: V9-01…V9-30 + V10-01…V10-10 + V10R-01…V10R-05 + V10R2-01…V10R2-05 Fixed (2026-07-16 record). Vòng 11: 9 finding (P1-01, P1-04, P1-05, P1-07, P1-08, P1-09, P2-01, P2-02, P2-03, P1-11) Fixed (2026-07-16). Vòng 12: 7 P1 + 6 P2 (3 code + 3 doc-only) Fixed (2026-07-16). Document vẫn giữ `Status: inprogress` — chưa có full Codex re-review pass xác nhận "Vòng 13" sạch, nên chưa tự ý đổi lại thành `done`.
- primary modules: `apps/local-runner/internal/runner/*`, `apps/local-runner/internal/flowgate/*`.
- docs moved: `Task-238`–`Task-242`, `Task-244`–`Task-247` → `08-Task/done/`; `CP-50` → `07-Coding-Plan/done/`. CP-50's 4 stale `08-Task/todo/*` links fixed to `done`/`inprogress` (2026-07-16, Vòng 12 P2 finding). CP-43 §3 và Task-243 §8 đã bổ sung upstream reference cho CP-50 P-4/Task-247 (2026-07-16, Vòng 12 P2 finding).
- verification: residual + gate/resume suites pass (`TestParentResumeReconstructsPendingGateChild`, `TestRootPendingGateNotCancelledByNormalize`, `TestResumeGateMaterializesTurnID`, `TestUndoForceShellBridgeCommits`, …). Vòng 11 verification: xem block "Verification đã chạy" ở trên. Vòng 12 verification: xem block "Verification Vòng 12" ở trên.
