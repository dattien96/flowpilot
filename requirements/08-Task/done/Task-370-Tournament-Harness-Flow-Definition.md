# Task-370: Tournament Harness Flow Definition

## Metadata

- Document ID: `Task-370`
- Title: `Tournament Harness Flow Definition`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-16`
- Completed: `2026-09-16` (CA-878)
- Parent Documents: [CP-65 P-3](../../07-Coding-Plan/done/CP-65-Multi-Candidate-Tournament-Harness.md)
- Child Documents: `None`
- Related Documents: [SS-19](../../05-System-Specs/SS-19-Engineering-Harness-Flow-Family.md), [SD-19](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [CP-36](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [CP-62](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md)
- Replaces: `None`
- Tags: `tournament, flow-pack, parallel-rollout, cohort, behavior-registry`
- Feature Keys: `tournament-harness`

## AI Quick View

### Summary

- Slice 3 của CP-65: flow độc lập `tournament-harness.yaml` với topology `problem_scout → parallel_rollout → tournament_arbiter → merge_and_audit` — chọn được từ flow picker cho các bài toán hóc búa (không bao giờ là default cho task thường — CP-65 constraint).
- `parallel_rollout` là cohort nhiều node candidate (tiền lệ: `cohort: review` + `join: all` của review-loop.yaml), mỗi candidate khai báo provider/model riêng (Claude/Codex/Grok) và chạy trong worktree riêng của Task-369.
- Cần 2 behavior runtime mới: `tournament.arbiter` (inline Go — thu thập metric trong từng worktree, gọi Arbiter P-1, quyết định winner hoặc ask_user theo `auto_pick`) và `tournament.merge` (inline Go — merge winner + dọn worktree, tái sử dụng `artifact.audit_draft` cho phần audit).
- Q-1 (CP-65): config `auto_pick: true/false` trên node `tournament_arbiter` — false → escalate `ask_user` kèm decision card ranking (tiền lệ decision card CP-62).

### Current Ask

- Implement P-3 theo CP-65 §4: flow YAML mới + 2 behavior runtime + đăng ký manifest/registry. 2 test signatures cho sẵn phải xanh.

### Key Decisions

- `T-1` Topology 4 node theo CP-65 §4 P-3: `problem_scout` (delegate — scout/context + freeze scope, posture `read_only` theo Task-340) → `parallel_rollout` (cohort các node candidate `lifecycle: spawn`, `join: all`) → `tournament_arbiter` (inline `tournament.arbiter`) → `merge_and_audit` (inline `tournament.merge` + draft audit) → done. Từ `tournament_arbiter` có 3 cạnh: `done` (có winner) → `merge_and_audit`; `retry` (back-edge về `parallel_rollout`, cùng pattern `synthesis` → `coder` của review-loop) khi verdict hòa/tất cả đỏ + còn lượt + `auto_pick=true`; `escalate` → `ask_user` khi hết lượt, `auto_pick=false`, hoặc merge conflict (conflict không retry — chạy lại cũng conflict tiếp). Mỗi lượt qua `parallel_rollout` spawn run candidate MỚI (`lifecycle: spawn` mới mỗi lượt, context sạch — cùng candidate configs/model, không đổi model); brief chưng cất fail lần trước do behavior `tournament.arbiter` đóng dấu vào config lượt rollout tiếp theo. Retryability suy ra từ `TournamentVerdict.NeedsHumanDecision` — không đổi P-1.
- `T-2` Candidate configs khai báo trong config của node `parallel_rollout` (danh sách `{candidate_id, provider, model}`), pack parse vào `FlowNode` config — mặc định 2 candidate Claude+Codex theo R-1 (§9), cho phép 3; có cờ `serial: true` fallback cho máy < 8GB RAM. Node `tournament_arbiter` thêm `max_attempts: 2` (default 1 = đúng plan cũ, không retry — retry là opt-in).
- `T-3` Behavior `tournament.arbiter` (inline, scope `inline`, runtimeHandler `behaviorTournamentArbiter`): với mỗi candidate — chạy test suite trong worktree, thu LSP diagnostics (`lsp.ServerSet.CheckFiles`), đếm dependents (`structure.Provider`), nạp vào `CandidateResult`, gọi `TournamentArbiter.Decide`; sản xuất payload verdict + decision card.
- `T-4` Behavior `tournament.merge` (inline, runtimeHandler `behaviorTournamentMerge`): gọi `WorktreeManager.MergeWinner` cho candidate thắng rồi `Cleanup` toàn bộ (thua xóa tại verdict, thắng xóa khi flow done — policy Task-369); merge conflict → escalate `ask_user` kèm patch + path conflict trong card rồi vẫn dọn (không terminate im lặng, không mồ côi).
- `T-5` Runtime registry là `runner/behavior_registry_builtin.go`; entry reference trong `behaviors/registry.yaml`; flow mới đăng ký `builtin.selectableIn: [flow]` như các harness khác (không mặc định).

### Constraints

- Additive tests only — không edit pre-existing tests; old test đỏ hoặc không compile → STOP, báo tên test + output, không sửa test, chờ user (R1 safe-fix-contract).
- Provider parity: Case-3 per-provider — candidate configs khai `{candidate_id, provider, model}` cho Claude/Codex/Grok; default 2 (Claude+Codex) theo R-1 nhưng config 3 (thêm Grok) phải parse + dispatch được; glue thu metric là agnostic nhưng spawn per-provider phải verify 3 mặt hoặc ghi evidence nếu Grok opt-in chưa cover.
- Prior CA: none (new feature `tournament-harness`); đóng task phải kèm CA-NNN ledger entry.
- Flow KHÔNG được tự kích hoạt cho task thường: chỉ chọn tay từ picker (và sau này P-4 wire từ escalation) — policy cap hợp lý để chặn vòng lặp token.
- Pack validation phải pass: edges hợp lệ, đúng một continue back-edge (nếu có), behaviors resolve được ở runtime registry.
- GitNexus impact analysis trước symbol edit trong `behavior_registry_builtin.go` và pack parser.

### Open Questions

- None (Q prompt candidate đã chốt: thêm `prompts/tournament-candidate.md` mới, tái dùng `agents/coder.md`).

### Source Refs

- CP-65 §3.1 (kiến trúc), §4 P-3, test signatures 8–9, §9 (R-1 default 2 candidates), Q-1 (auto_pick).
- `flows/review-loop.yaml` (cohort + join precedent), `flows/cp-harness.yaml` (harness shape), `behaviors/registry.yaml` (reference registry), `internal/agentpack/pack.go` (FlowNode config parsing), `internal/runner/behavior_registry_builtin.go`.

## 1. Goal

Flow `tournament-harness` load/validate/select được từ picker: chạy N candidate song song trong worktree cô lập, Arbiter Go chấm điểm khách quan, winner được merge về workspace chính kèm audit — hoặc escalate cho con người khi hòa/`auto_pick=false`.

## 2. Parent Links

- coding plan: `CP-65-Multi-Candidate-Tournament-Harness.md` P-3
- tech design: `SD-19-Agent-Flow-Engine.md`
- system spec: `SS-19-Engineering-Harness-Flow-Family.md`

## 3. Trigger

P-1 (scorer) và P-2 (worktree) là thư viện thuần; cần một flow bọc chúng thành quy trình chạy được end-to-end từ UI picker trước khi P-4 có thể gọi như nhánh cứu hộ.

## 4. Exact Change

- `T-1` **`internal/agentpack/flow-pack/flows/tournament-harness.yaml`** (new): 4 node theo T-1 + edges + policy (cap hợp lý, `onCap: escalate`); candidate configs + `auto_pick` + `serial` trong node config; candidate nodes dùng `promptTemplate: prompts/tournament-candidate.md` (persona `agents/coder.md`); đăng ký `builtin` selectable.
- `T-2` **`internal/agentpack/flow-pack/manifest.yaml`**: đăng ký flow mới + `prompts/tournament-candidate.md` mới (Q đã chốt: prompt riêng, tái dùng persona `agents/coder.md` để không phình manifest).
- `T-3` **`internal/runner/behavior_registry_builtin.go`**: đăng ký `tournament.arbiter` + `tournament.merge` (2 handler inline mới); **`behaviors/registry.yaml`**: thêm 2 entry reference docs (goRuntime inputs/outputs, usedBy trỏ flow mới).
- `T-4` **`internal/runner/tournament_behavior.go`** (new, tên chốt): `behaviorTournamentArbiter` + `behaviorTournamentMerge` — glue gọi `TournamentArbiter`, `WorktreeManager`, thu metric test/LSP/dependents trong từng worktree (spawn suite cmd + `lsp.ServerSet.CheckFiles` + `structure.New(...).Dependents`).
- `T-5` **`internal/agentpack/tournament_flow_test.go`** (new, không sửa pack test cũ): 4 test signatures dưới đây (2 topology/config + 2 retry).

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/agentpack/flow-pack/flows/tournament-harness.yaml` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/prompts/tournament-candidate.md` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml` (modified)
  - `apps/local-runner/internal/agentpack/flow-pack/behaviors/registry.yaml` (modified — reference)
  - `apps/local-runner/internal/runner/behavior_registry_builtin.go` (modified)
  - `apps/local-runner/internal/runner/tournament_behavior.go` (new — metric glue + 2 handlers)
  - `apps/local-runner/internal/agentpack/tournament_flow_test.go` (new)
- modules: `agentpack`, `runner`, `tournament`
- routes: none
- tables: none

## 6. Acceptance Check

- [x] AC-1: Pack load + validate pass — topology 4 node hợp lệ, behaviors resolve runtime được.
- [x] AC-2: Candidate configs parse đúng: default 2 candidate (Claude + Codex) theo R-1, chấp nhận 3, `serial: true` chuyển sang chạy tuần tự.
- [x] AC-3: `auto_pick=false` hoặc hòa → flow dừng ở escalate `ask_user` với decision card ranking đầy đủ (không merge); `auto_pick=true` + có winner → đi tiếp `merge_and_audit`.
- [x] AC-4: Merge conflict trong `tournament.merge` → escalate `ask_user` với patch + path conflict trong card, worktree vẫn dọn sạch (bằng chứng trong card, không giữ điều tra).
- [x] AC-5: Flow không xuất hiện làm default cho task thường (chỉ selectable từ picker).
- [x] AC-6: 4 test signatures green:
  - `TestTournamentHarnessTopologyValid`
  - `TestTournamentHarnessParsesCandidateConfigs`
  - `TestTournamentSecondAttemptUsesDistilledFailure`
  - `TestTournamentNoThirdAttempt`
- [x] AC-7: Hòa/tất cả đỏ + còn lượt + `auto_pick=true` → back-edge retry về `parallel_rollout` với brief chưng cất, spawn run candidate mới (không tái dùng run cũ); hết 2 attempts hoặc merge-conflict → `ask_user`, không retry thêm; `max_attempts` default 1 giữ đúng hành vi plan cũ.

## 7. Out of Scope

- Escalation tự động từ review loop / debate stall (P-4 / Task-371).
- E2E chạy thật 2 candidate với merge thành công (P-5 / Task-372).
- Thay đổi behavior của các flow harness hiện có.

## 8. Completion Notes

- result: done 2026-09-16 — CA-878; 17/17 new tests green; full agentpack + related runner suites green; 12→13 inventory update operator-authorized; R1/R2/R3 hold.
- follow-ups: P-5 live rollout-controller dispatch, 3rd-candidate runtime expansion, worktree-write gate posture (see CA-878).
- upstream docs updated:
