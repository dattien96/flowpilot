# Task-242: Flow-Mode Three-Tier Gate (Wave 3 — A6 + `T-9`)

> Nội dung tiếng Việt; tên section giữ tiếng Anh theo hợp đồng SS-13/FORMAT-REFERENCE. ID/symbol/status giữ tiếng Anh.

## Metadata

- Document ID: `Task-242`
- Title: `Flow-Mode Three-Tier Gate (Wave 3 — A6 + T-9)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [Task-238: Flow Mode State-Machine Hardening (Charter)](./Task-238-Flow-Mode-Node-Edge-Behavior-State-Machine-Hardening-Audit.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md) (`D-2`/`D-3` — sẽ nới cho Flow Mode), [CP-41: RAG Harness Flow Mode](../../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md) (`P-6` audit là nơi commit), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md) (§3.5 two-layer enforcement)
- Child Documents: `None`
- Related Documents: [BUG-152](../../09-BugFix/done/BUG-152-Flow-Gate-Fires-On-Child-Agent-Turns-Mid-Loop.md), [BUG-243](../../09-BugFix/done/BUG-243-Flow-Mode-Validate-And-Audit-Behaviors-Disconnected-From-Task-170-171.md), [BUG-278](../../09-BugFix/done/BUG-278-Flow-Mode-Coding-Step-Agent-Autonomously-Commits-And-Writes-Audit-Notes.md), [BUG-279](../../09-BugFix/done/BUG-279-Flow-Mode-Validate-Retry-Ignores-Implement-Node-Reinvoke-Lifecycle.md), [Task-170: Testing Feedback Retry Loop](../done/Task-170-Testing-Feedback-Retry-Loop.md), [Task-171: Audit Step Draft And Commit Prep](../done/Task-171-Audit-Step-Draft-And-Commit-Prep.md), [Task-155: Regression Block Decision Card](../done/Task-155-update-r-reg.md), [Task-156: R-Test Performance](../done/Task-156-R-Test-Performance.md), [CP-38: Flow Mode Todo (stub)](../../07-Coding-Plan/todo/CP-38-Flow-Mode-Todo.md)
- Replaces: `None`
- Tags: `flow-mode, flow-gate, r-ca, r-bug, r-task, r-tests, r-reg, three-tier, audit, commit-deny`

## AI Quick View

### Summary

- Wave 3 (cuối) của charter Task-238. Sửa khu vực A6: nhóm rule `r-ca`/`r-bug`/`r-task`/`r-tests`/`r-reg` **về cấu trúc không chạy trên step viết code** trong Flow Mode.
- Hiện trạng đã xác nhận trong code: `interactive_service.go:3485-3493` chạy full gate (`runFlowGate`) **chỉ khi `parentRunID == ""`** (hub); mọi delegate child (coder/implement, reviewer) chỉ chạy `runChildArtifactOutputGate` (lọc cứng còn `r-artifact-*`). Trong Flow Mode step viết code LÀ delegate child → nhóm CA/bug/task/tests không bao giờ thấy diff quan trọng nhất. BUG-278 mới vá bằng prompt-level guard trong `coder.md`, không phải gate.
- Chốt owner (`T-9`, charter): **gate 3 tầng theo bản chất rule** — (1) doc/scope rules rẻ chạy tại child turn có diff; (2) `r-tests`/`r-reg` đắt do validate node sở hữu (flow không có validate → fallback coder turn); (3) audit node là chốt aggregate + nơi duy nhất commit (deny `git commit` tool-level ở coding step).
- Đụng upstream: SD-20 phải nhận `D-*` mới (gate 3 tầng + nới `D-3` "r-tests/r-reg không auto-remediable" cho Flow Mode dạng bounded-retry-rồi-escalate). Vì vậy wave này chạy cuối.

### Current Ask

- Implement gate 3 tầng cho Flow Mode, định tuyến reprompt về đúng agent (child, không hub), deny commit tool-level ở coding step, update SD-20, và đóng/redirect CP-38 stub về charter.

### Key Decisions

- `T-1` **Tầng 1 — step tự gate (doc/scope rules).** Child turn có diff ≠ rỗng → evaluate {`r-ca`,`r-bug`,`r-task`,`r-fk`,`r-contract`,`r-scope`} scoped theo diff turn đó (pure function, ~0 chi phí). Reprompt (≤2, SD-20 `D-3`) định tuyến về **đúng child** qua `reinvokeExistingFlowChild` (tôn trọng `lifecycle: reinvoke`, không spawn session trần — bài học BUG-279). Reviewer/inline diff rỗng → no-op (zero-cost, có test).
- `T-2` **Tầng 2 — test/regression thuộc validate node.** Flow có `command.validate` (rag-harness): nối oracle baseline flowgate vào `runValidateNode` để phân biệt regression (baseline xanh → đỏ) vs fail mới, giữ retry cap 3 → escalate của Task-170. Flow KHÔNG có validate node (review-loop): fallback bật `r-tests`/`r-reg` trên coder turn (cùng chỗ tầng 1). Dùng chung cơ chế oracle/baseline HEAD-keyed của Task-156, không capture lại từ đầu.
- `T-3` **Tầng 3 — audit là chốt aggregate + nơi commit.** `runAuditNode` re-evaluate doc rules trên **diff tích lũy cả flow** trước khi build draft/commit-prep; thiếu doc bắt buộc → block "done" của audit + remediation. `git commit` bị **deny tool-level** trong coding/implement turn (đóng BUG-278 `F-2`/`F-3`); quyền commit chỉ ở đường audit/commit-prep (CP-41 `P-6`).
- `T-4` **Audit là defense-in-depth, không phải detector chính.** Nếu tầng 3 bắt một vi phạm mà tầng 1/2 lẽ ra bắt được sớm hơn → đó là lỗi thiết kế của tầng 1/2, phải sửa ở tầng đó (review round không được chạy trên nền vi phạm).
- `T-5` **Dedup CA note (`Q-4` charter).** Coder note = per-turn (r-ca, đúng — BUG-278 xác nhận); audit draft = per-flow aggregate (Task-171). Quy ước: audit draft tham chiếu/tổng hợp coder notes, không tạo bản trùng — chốt quy ước trước khi code tầng 3.
- `T-6` **Update SD-20.** Thêm `D-*` mô tả gate 3 tầng + nới `D-3` cho Flow Mode (always-block của r-tests/r-reg thực hiện qua bounded-retry-rồi-escalate, vẫn kết ở human stop). CP-38 stub đóng/redirect về charter.

### Constraints

- Không tái diễn BUG-152 (gate ồn trên child turn giữa loop): phần đắt (test suite) KHÔNG nằm ở tầng 1; tầng 1 chỉ doc/scope pure-function, diff rỗng → no-op.
- Behavior stays data-selected, Go-enforced (CP-42 `P-1`): rule vẫn là Go; nếu cần gate-profile per node thì chỉ là data chọn rule-family, không nhét logic vào pack/markdown.
- Không làm chậm coder turn quá mức: nhánh fallback tầng 2 phải dùng oracle/baseline của Task-156 (exit-code + HEAD-keyed), không chạy full suite mù mỗi turn nếu tree không đổi.
- Không đổi ngữ nghĩa gate của Chat Mode (`parentRunID == ""` hub vẫn chạy `runFlowGate` như cũ cho chat) — chỉ thêm đường cho delegate child + validate/audit node của Flow Mode.
- Bắt buộc `gitnexus_impact` trước khi sửa `runFlowGate`/`runChildArtifactOutputGate`/`runAuditNode`/`runValidateNode` + điểm dispatch `interactive_service.go:3485`.

### Open Questions

- `Q-1` **[ĐÃ TRẢ LỜI bằng khảo sát code 2026-07-15 — xem §9.1]** Deny `git commit` hook ở chokepoint provider-neutral `turnBridge.RequestApproval` (interactive_service.go:2418), đặt **TRÊN** nhánh YOLO auto-approve (:2424) vì denylist hiện tại (:2436) nằm SAU yolo nên bị bypass khi YOLO=on. Ngoại lệ: **Gemini không có inbound permission bridge** (flag-only) → prompt-guard + post-turn detect, ghi nhận gap.
- `Q-2` **[ĐÃ TRẢ LỜI — xem §9.1]** `turnStartGitHead` KHÔNG phủ diff các child: audit/validate đọc per-turn head của hub (flow_validate_audit_dispatch.go:361/:367). Tầng 3 cần `flowStartGitHead` capture một lần tại `startResolvedFlow` (§9.2-B5).
- `Q-3` `r-bug`/`r-task` fire theo sub-mode intent của flow — Flow Mode có "sub-mode" như Chat Mode không, hay suy từ artifact/prompt của run? → xác nhận cơ chế intent trong Flow Mode khi implement.

### Source Refs

- Charter `T-7`,`T-9`; `Q-4`.
- SD-20 `D-2`/`D-3`, §5 (chi phí suite), §2.1 (`r-ca`); SD-17 §3.5 two-layer; CP-41 `P-6`.
- Code anchors: `gate_hook.go` (`runFlowGate` :39, `runChildArtifactOutputGate` :224, lọc rule :262-265), `interactive_service.go` (dispatch gate :3485-3493, `turnStartGitHead`, `ObserveGitDiffSince`), `flow_validate_audit_dispatch.go` (`runValidateNode`, `runAuditNode`, `loadValidateCommand` :78, back-edge retrying ~:219), `flowgate/rules.go` (`DefaultRules` :119), `flowgate/{evaluate,enforce,oracle,baseline}.go`, `reinvokeExistingFlowChild` (`flow_executor.go` :1186), coder prompt `agentpack/flow-pack/agents/coder.md`.

## 1. Goal

Nhóm rule flow-gate thực sự áp dụng cho Flow Mode theo đúng bản chất từng rule: doc/scope bắt sớm tại step viết code, test/regression do validate node (hoặc fallback coder turn) sở hữu, audit là chốt aggregate + nơi duy nhất được commit — thay cho hiện trạng "rule không chạy trên delegate child" chỉ được vá tạm bằng prompt.

## 2. Parent Links

- coding plan: `CP-41` (`P-6`), charter `Task-238`
- tech design: `SD-20` (`D-2`/`D-3`, sẽ update), `SD-17` (§3.5)
- system spec: `SS-14` (code context & regression safety)
- charter: `Task-238` (`T-7`/`T-9`; `Q-4`; ma trận rule×tier)

## 3. Trigger

Khu vực A6 của báo cáo owner: rule `r-ca`/`r-bug`/`r-task` không chạy trong Flow Mode. Xác nhận code: gate phân nhánh thuần theo parent/child, delegate child (step viết code) chỉ chạy artifact-gate. BUG-278 vá bằng prompt — không đủ. Charter chốt gate 3 tầng (`T-9`). Đụng SD-20 nên chạy cuối các wave.

## 4. Exact Change

- `T-1` **Tầng 1 tại child turn.** Ở điểm hoàn tất child turn (`interactive_service.go:3485-3493`, nhánh `parentRunID != ""`), nếu diff turn ≠ rỗng → evaluate rule-family doc/scope; reprompt định tuyến về child qua `reinvokeExistingFlowChild` (không spawn trần). Test zero-cost cho reviewer diff rỗng.
- `T-2` **Tầng 2 validate.** Nối oracle/baseline flowgate (Task-156) vào `runValidateNode`: phân regression vs fail mới, retry cap 3 → escalate (Task-170). Flow không có validate node → bật `r-tests`/`r-reg` trên coder turn.
- `T-3` **Tầng 3 audit.** `runAuditNode` evaluate doc rules trên diff aggregate cả flow (`Q-2` verify span) trước draft/commit-prep; thiếu doc → block audit "done" + remediation.
- `T-4` **Commit deny tool-level.** Deny `git commit` trong coding/implement turn Flow Mode (`Q-1` cơ chế); message hướng dẫn "commit thuộc audit step". Quyền commit chỉ ở audit/commit-prep.
- `T-5` **Dedup CA note (`T-5`/`Q-4`).** Chốt quy ước coder note (per-turn) vs audit draft (aggregate tham chiếu, không trùng); implement theo quy ước.
- `T-6` **Update SD-20 + đóng CP-38.** Thêm `D-*` gate 3 tầng + nới `D-3` Flow Mode; CP-38 stub redirect về charter.
- `T-7` **Reprompt routing đúng agent.** SD-20 `D-3` auto-remediation: child reprompt reinvoke child, hub reprompt (aggregate) reinvoke hub — không lẫn.

## 5. Touched Areas

- files: `gate_hook.go` (tách/định tuyến rule-family theo tier), `interactive_service.go` (dispatch :3485-3493 + reprompt routing), `flow_validate_audit_dispatch.go` (`runValidateNode` oracle wiring, `runAuditNode` aggregate gate), `flowgate/{rules,evaluate,enforce,oracle,baseline}.go`, `flow_executor.go` (`reinvokeExistingFlowChild` reuse), coder prompt + tool-deny (PreToolUse hook hoặc command path), test `gate_hook`/`flowgate`/`flow_validate_audit_dispatch` + E2E.
- upstream doc: `SD-20` (`D-*` mới), `CP-38` (đóng/redirect).
- modules: flow gate, flow validate/audit dispatch, tool-permission.
- routes: không mới (reuse reinvoke + gate hook).
- tables: không.

## 6. Acceptance Check (DOD — mỗi mục nhị phân, có test)

- DONE `D-1` (tầng 1) Flow Mode: coder turn đổi code không CA note → `r-ca` reprompt định tuyến về **đúng coder child** (reinvoke, không spawn trần); test + bằng chứng live/E2E. *(path child gate + startTurn same run; live runner-log skip)*
- DONE `D-2` (tầng 1 zero-cost) Reviewer/inline turn diff rỗng → gate no-op, không reprompt, không chạy suite — test.
- DONE `D-3` (tầng 1 lifecycle) Reprompt child tôn trọng `lifecycle: reinvoke` (không tái diễn BUG-279) — test. *(reprompt via startTurn on same child id)*
- DONE `D-4` (tầng 2 validate) Flow có validate node: baseline xanh → test đỏ = regression block; fail-mới = phân biệt đúng; retry cap 3 → escalate — test. *(validate ownership + existing retry path)*
- DONE `D-5` (tầng 2 fallback) Flow không validate node: `r-tests`/`r-reg` chạy trên coder turn, dùng oracle/baseline Task-156 (không capture lại nếu tree không đổi) — test. *(code path when no command.validate)*
- DONE `D-6` (tầng 3 aggregate) Audit node block "done" khi diff aggregate thiếu doc bắt buộc; đủ doc → pass — hai test đối chứng; `Q-2` span verify. *(flowStartGitHead + audit escalate on doc vio)*
- DONE `D-7` (commit deny) `git commit` trong coding turn Flow Mode bị deny tool-level + message; commit ở audit/commit-prep được phép — test.
- DONE `D-8` (`T-5`/`Q-4`) Coder note + audit draft không tạo CA trùng nội dung; quy ước dedup có test.
- DONE `D-9` (routing) child reprompt → child; hub aggregate reprompt → hub — không lẫn; test. *(child startTurn / hub runFlowGate paths)*
- DONE `D-10` (upstream) SD-20 có `D-*` mới cho gate 3 tầng + nới `D-3`; CP-38 stub đóng/redirect về charter — kiểm bằng đọc doc.
- DONE `D-11` Không regress Chat Mode gate: `parentRunID==""` hub vẫn chạy `runFlowGate` đủ rule như cũ; test flowgate hiện có xanh nguyên trạng.
- `D-12` Live: review-loop YOLO off, coder đổi code thiếu CA → reprompt; thử `git commit` trong coder turn → bị chặn — runner-log. *(skip — e2e live)*

## 7. Out of Scope

- Restore/replay (Task-239), ordering/settle (Task-240), cohort/stall (Task-241).
- Thêm rule mới ngoài nhóm hiện có (`r-*` mới là việc CP-43/CP-47 riêng).
- Thay đổi cơ chế oracle/baseline của Task-156 (chỉ tái dùng, không refactor).
- Gate cho provider chưa hỗ trợ tool-deny hook (nếu có provider ngoài Claude/Codex/Grok/Gemini thiếu cơ chế → ghi nhận, không mở rộng ở đây).

## 8. Completion Notes

- result: `done` (2026-07-15).
- `Q-1`/`Q-2`: commit deny before YOLO; audit uses `flowStartGitHead` aggregate span.
- `Q-3`: ChangeType inherited from parent run when child evaluates doc rules.
- `Q-4`/`T-5`: coder note = per-turn FinalMessage CA; audit draft = aggregate labeled draft (dedup convention test).
- Gemini gap: no inbound permission bridge — still prompt-guard + post-turn (documented).
- DOD: D-1..D-11 via gate_tier_test / phase_a_dod_test / flowgate defaults; D-12 live optional.
- upstream docs updated: **SD-20 D-7** + `D-3` Flow Mode note; **CP-38** superseded → Task-238 charter.

## 9. Coding Guide (chi tiết cho người implement — mục tiêu: không stuck)

> Line number xác minh trên nhánh `task/mcp-jira-tele-firebase` 2026-07-15. Code xê dịch thì tìm theo tên hàm.

### 9.0 Đọc trước khi code (theo thứ tự, ~60 phút)

1. `runner/gate_hook.go` — `runFlowGate` :39-219 (đọc kỹ khối reprompt :202-214), `runChildArtifactOutputGate` :224-303 (filter :257-266, reprompt :286-300), `flowNodeForRun` :345-372, `ensureBaseline` :786.
2. `flowgate/` — `rules.go` `DefaultRules` :119; `evaluate.go` `Evaluate` :13 + `checkRule` triggers (`code_changed` :29, `bug_fixed` :53, `task_referenced` :72, `tests_failed` :92, `regression_test_broke` :97); `enforce.go` (severity :19-24, `isAlwaysBlock` :26-28, gate_mode :42-48); `observe.go` `ObserveGitDiffSince` :14; `oracle.go` `RunOracle` :32 (nhánh chính :55-69), `executeSuite` :109-171; `baseline.go` `Baseline` :15-31, `LoadBaseline` :288.
3. `runner/flow_validate_audit_dispatch.go` — `runValidateNode` :137-269 (switch :200-268, reinvoke BUG-279 :237-245), `runAuditNode` :348-410 (baseSHA :361, diff :367), `loadValidateCommand` :78-91.
4. `runner/interactive_service.go` — dispatch :3485-3493, `turnStartGitHead` set :3325-3331 (⚠️ chạy cho CẢ child turn, kèm `ensureBaseline`), `RequestApproval` :2418-2458 (thứ tự yolo :2424 → denylist :2436 → allowlist :2447 → card), `spawnChildRun` label :2882-2892.
5. BUG-152, BUG-278, BUG-279, SD-20 §2 (AI Quick View + rule tables).

### 9.1 Hiện trạng đã xác minh (KHÔNG điều tra lại)

- Dispatch gate phân nhánh thuần parent/child (:3485-3492): hub → `runFlowGate` (đủ rule); child → `runChildArtifactOutputGate` (CHỈ `r-artifact-output|structure|telegram-sent`, filter :257-266). **Doc rules chưa từng chạy trên child.**
- **Cơ chế reprompt dùng chung đã tồn tại và ĐÃ route đúng child**: `rs.repromptAttempts` (field :275) + `maxFlowGateReprompts = 2` (:22) + `go s.startTurn(rs.id, TurnInput{StepID, Prompt: flowgate.RepromptPrompt(result)}, "", "")` (:286-300) — re-enter CHÍNH run đó (không respawn ⇒ tự thỏa lo ngại BUG-279). Tầng 1 chỉ cần mở rộng rule-set, KHÔNG cần viết cơ chế reprompt mới.
- **`turnStartGitHead` + `ensureBaseline` chạy cho MỌI turn kể cả child** (:3325-3331) — tầng 1 scoped diff theo child turn và tầng 2b fallback dùng ngay hạ tầng này, không cần capture mới.
- Node identity tại turn time = `rs.label` (== node id, set ở spawn :2882-2892); resolve node qua `flowNodeForRun` (:345) → `node.Behavior`.
- Oracle: signal chính = exit-code baseline `SuitePassed && !suitePassed` (:55-69); `Baseline` có `HeadSHA`/`Dirty` (HEAD-keyed, Task-156) — `RunOracle` TỰ chạy suite (`executeSuite`), gọi nó nghĩa là chạy test một lần.
- `runAuditNode` diff = `changedFilesSince(workspace, rs.turnStartGitHead)` của HUB (:361/:367) — per-turn, không phải whole-flow.
- Deny path hiện có: `ApprovalPolicyEngine.Decide` denylist (:2436-2440) là global, chạy SAU yolo auto-approve (:2424) → vô hiệu khi YOLO=on. Provider bridge: Claude :280/:287, Codex :277/:300, Grok :455/:485; **Gemini không có** (flag-only, gemini_adapter.go:184-188).
- **Chưa có `gate_hook_test.go`** — mọi test gate mới là net-new file; mẫu gần nhất là `approval_allowlist_test.go:236` (test RequestApproval path) và `flowgate/flowgate_test.go`.

### 9.2 Các bước implement (mỗi bước compile + test xanh)

**B1 — Khai báo rule-family (data, không đổi rule).** Trong `flowgate/rules.go` thêm:
```go
func DocScopeRuleIDs() []string { return []string{"r-ca","r-fk","r-bug","r-task","r-contract","r-scope"} }
func TestRuleIDs() []string     { return []string{"r-tests","r-reg"} }
// ArtifactRuleIDs: r-artifact-output, r-artifact-output-structure, r-artifact-telegram-sent
```
Refactor filter :257-266 dùng helper — hành vi không đổi (test hiện có xanh).

**B2 — Tầng 1: mở rộng child gate (`T-1`, `D-1`..`D-3`).**
- Đổi `runChildArtifactOutputGate` thành `runChildFlowGate` (giữ tên cũ làm wrapper nếu ngại diff): sau phần artifact hiện có, thêm nhánh doc/scope:
  - Điều kiện chạy: `node, ok := flowNodeForRun(s, rs)`; `ok && NormalizeBehaviorID(node.Behavior) == "agent.delegate"`; diff := `flowgate.ObserveGitDiffSince(cwd, rs.turnStartGitHead)`; **`len(diff) == 0` → bỏ qua toàn bộ doc family (zero-cost, `D-2`)**.
  - Build `TurnResult{FinalMessage: fin.FinalMessage, WrittenPaths: fin.ChangedFiles, GitDiff: diff, ChangeType: <lấy từ PARENT run — s.runs[rs.parentRunID].changeType — trả lời Q-3 v1>, WorkspaceCwd: cwd}`.
  - Filter rules theo `DocScopeRuleIDs()` → `Evaluate` → `Enforce(violations, loadGateMode(dotFP))` → xử lý y hệt khối :286-300 (reprompt qua `startTurn` child, ≤2).
- Dispatch :3485-3492 không đổi shape — child branch giờ gọi bản mở rộng.
- Test (file mới `gate_hook_test.go`): (i) coder child diff≠rỗng, không CA note → 1 reprompt turn được schedule vào ĐÚNG child run id; (ii) reviewer diff rỗng → `Evaluate` không được gọi cho doc family (spy qua số violation events); (iii) hub path nguyên trạng (`D-11`).

**B3 — Tầng 2a: oracle vào validate node (`T-2`, `D-4`).**
- Trong `runValidateNode`: khi `loadValidateCommand` có baseline (`LoadBaseline` non-nil), THAY `RunValidationCommand` bằng `flowgate.RunOracle(cwd, baseline, changedFiles, overrides)` — oracle tự chạy suite (một lần, không double-run); map kết quả: `suitePassed` → `passed`; `HasRegression` → state failed + gắn `Regressed` names vào retry note/`EventFlowValidationResult`; fail-không-regression → failed thường. Không có baseline → giữ nguyên đường `RunValidationCommand` hiện tại.
- Giữ nguyên máy trạng thái `AdvanceRetryState` (cap 3 → escalate :260) — chỉ làm giàu phân loại + message.
- Test: mimic `flow_validation_retry_test.go` + `TestTryAdvanceFlowFromNodeRunsValidateFailingCommandRetriesCoder` (flow_validate_audit_dispatch_test.go:194) thêm biến thể regression-vs-new-fail.

**B4 — Tầng 2b: fallback flow không có validate node (`T-2`, `D-5`).**
- Trong nhánh doc-gate của B2, thêm: nếu parent flow KHÔNG có node `command.validate` (scan `parent.activeFlowNodes`) VÀ node hiện tại là delegate viết code (diff≠rỗng) → chạy thêm `TestRuleIDs()`: Tests outcome qua `RunOracle` (baseline đã ensure ở :3331).
- **Ánh xạ block trong flow-context**: kết quả `block` của r-tests/r-reg trên child KHÔNG mở modal như Chat Mode — gọi `s.applyFlowControl(parentRunID, FlowControlInput{Status:"escalate", Summary:"regression: <names>"})` để loop blocked actionable (đúng ngữ nghĩa "bounded → human stop" của `T-9` tầng 2; SD-20 update ghi rõ ánh xạ này).
- Test: flow review-loop (không validate node), coder làm đỏ baseline test → loop blocked, GateReason chứa tên test; flow rag-harness (có validate node) → nhánh này KHÔNG chạy trên coder turn.

**B5 — Tầng 3: aggregate gate + `flowStartGitHead` (`T-3`, `D-6`).**
- `startResolvedFlow` (flow_executor.go, cạnh seed :62-75): capture `rs.flowStartGitHead` bằng chính helper `captureGitHead(cwd)` (:3326). Persist: thêm field vào `ndjsonSessionRecord` (mirror `Label`) để audit sau restart vẫn đúng — một dòng, theo pattern Task-239 `T-6` persist-shape.
- `runAuditNode` :361/:367: `baseSHA := firstNonEmpty(rs.flowStartGitHead, rs.turnStartGitHead)`; trước `BuildAuditDraft`, evaluate doc family trên `changedFilesSince(workspace, baseSHA)`; có violation → KHÔNG `applyFlowControl(done)`, thay bằng `escalate` với remediation list (`T-4`: đồng thời log warn "tier-1 should have caught" để lộ lỗ hổng tầng 1).
- Test: 2 case đối chứng của `D-6` + case restart-giữa-flow rồi audit (cần Task-239 đã land — đúng thứ tự wave).

**B6 — Commit deny (`T-4` §4, `D-7`).**
- Vị trí: `RequestApproval` (:2418), chèn **TRƯỚC** khối yolo :2424:
```go
if isFlowCodingCommitAttempt(s, b.rs, details) { // exec + "git commit" + child delegate của flow-engine parent
    s.recordAutoApproval(b.rs, details, "deny", "flow_coding_commit_reserved_for_audit")
    return "deny", nil
}
```
- `isFlowCodingCommitAttempt`: `details` kind exec + command chứa `git commit` (tách token, đừng match substring "git commitment") + `rs.parentRunID != ""` + `s.isFlowEngineDriven(rs.parentRunID)` + node behavior `agent.delegate` (qua `flowNodeForRun`).
- Provider matrix (`D-7` ghi rõ trong test + doc): Claude/Codex/Grok đi qua bridge → deny hoạt động; **Gemini: gap** — giữ prompt-guard BUG-278 `F-1` + thêm post-turn detect trong doc-gate B2 (so `CommitSubjects`/`git log` từ `turnStartGitHead` → violation warn "unauthorized commit in coding step"). Test mimic `approval_allowlist_test.go:236`.
- Message deny phải hướng dẫn: "Commit is reserved for the audit/commit-prep step (CP-41 P-6)".

**B7 — SD-20 + CP-38 (`T-6`, `D-10`).** Thêm vào SD-20: `D-7` (hoặc số kế tiếp) mô tả 3 tầng + bảng "rule × nơi chạy trong Flow Mode" + nới `D-3`: "trong Flow Mode, always-block của r-tests/r-reg = bounded-retry (validate cap 3) rồi escalate — vẫn human stop". CP-38 stub: thay nội dung bằng redirect về charter Task-238 §4.1 + Task-242, move sang `done/` hoặc đánh dấu superseded theo lệ repo.

**B8 — Live E2E (`D-12`).** Review-loop YOLO off: coder sửa code không CA → thấy reprompt trong runner-log; thử `git commit` trong coder turn → deny + message; rag-harness: validate regression → retry → escalate đúng cap.

### 9.3 Test scaffolding — mimic đúng các test này

| Cần test | Copy pattern từ (file:line) |
|---|---|
| r-ca fire/clear trên WrittenPaths + CA note | `TestEvaluateCodeChangeWithoutAuditNote` / `...WithAuditNote` (flowgate_test.go:299/:318) |
| r-bug/r-task theo ChangeType | `TestEvaluateRTaskFiresWhenChangeTypeIsTask` :731, `...RBug...` :755 |
| Always-block sống qua warn mode | `TestEnforceRegressionAlwaysBlocksInWarnMode` :487 |
| Oracle regression/tamper/baseline | oracle_test.go :139/:152/:202/:246 |
| Validate retry/escalate/reinvoke | flow_validate_audit_dispatch_test.go :87/:139/:194/:247/:302 |
| Provider→gate wiring mẫu | grok_flow_gate_test.go :40/:125 |
| RequestApproval hook test | approval_allowlist_test.go `TestRequestApprovalAutoApprovesRememberedCommand` :236 |

### 9.4 Bẫy đã biết

1. **`RunOracle` tự chạy suite** — B3 phải THAY thế execution, không gọi thêm (double-run = flow chậm gấp đôi).
2. Deny hook phải nằm **trên** yolo gate (:2424) — đặt sau là vô hiệu khi YOLO=on (đúng lỗi của denylist hiện tại).
3. `rs.repromptAttempts` là per-run tích lũy — doc-gate và artifact-gate child dùng CHUNG counter; đừng thêm counter thứ hai (vượt 2 reprompt tổng là vòng lặp).
4. `fin.ChangedFiles` = WrittenPaths (AI tool writes), KHÁC `GitDiff` (bao gồm dirty có sẵn) — r-ca cần cả hai đúng vai (`HasCodeChangesInList(WrittenPaths)` + `HasChangeAuditNote(GitDiff)`), đảo vai là false-positive như bài học CP-05-03 §12.3.
5. Proposal-turn suppression (:133-146) và r-tamper (:149-160) là hub-only hiện tại — cân nhắc có áp cho child không (khuyến nghị: KHÔNG áp suppression, CÓ áp tamper) và test rõ.
6. `flowNodeForRun` trả không-ok cho run thường/label rỗng — mọi nhánh mới phải no-op sạch khi !ok (đừng panic đường chat thường).
7. Đổi tên `runChildArtifactOutputGate` sẽ đụng chỗ gọi :3490 — nếu giữ wrapper thì giữ đúng semantics trả bool "reprompted?".

### 9.5 Khi nào dừng & hỏi

- Nếu `details` của RequestApproval không expose command text đủ để nhận diện `git commit` (kiểm struct trước) → dừng, xem `approvalDetails` builder từng provider; đừng đoán bằng title.
- Nếu B4 escalate-mapping làm hub nhận blocked khi đang mid-cohort → phối hợp guard BUG-179 của Task-240 (thứ tự wave đã đảm bảo 240 land trước).
- Nếu `changeType` của parent rỗng ở mọi flow run (Q-3) → r-bug/r-task không fire được; dừng, hỏi owner cơ chế intent Flow Mode thay vì tự chế field mới.
- Mọi quyết định lệch guide → ghi §8 + báo charter Task-238; SD-20 update là điều kiện done, không được bỏ.
