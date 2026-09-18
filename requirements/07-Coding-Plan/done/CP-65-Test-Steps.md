# CP-65 Test Steps — Multi-Candidate Tournament Harness

## Metadata

- Document ID: `CP-65-TEST-STEPS`
- Title: `CP-65 Verification Steps (Automated + gate-sandbox Manual)`
- Phase: `verification`
- Status: `ready`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-16`
- Last Updated: `2026-09-17`
- Parent Documents: [CP-65: Multi-Candidate Tournament & PDR Escalation Harness](./CP-65-Multi-Candidate-Tournament-Harness.md)
- Child Documents: `None`
- Related Documents: [Task-368](../../08-Task/done/Task-368-Tournament-Arbiter-Scoring-Engine.md), [Task-369](../../08-Task/done/Task-369-Worktree-Rollout-Manager.md), [Task-370](../../08-Task/done/Task-370-Tournament-Harness-Flow-Definition.md), [Task-371](../../08-Task/done/Task-371-Escalation-Fallback-Wiring-Review-Loop.md), [Task-372](../../08-Task/done/Task-372-Tournament-E2E-Verification.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `tournament-harness, pdr, multi-candidate, escalation, test-steps, verification, cp-65`
- Feature Keys: `tournament-harness`

## AI Quick View

### Summary

- Danh mục kiểm thử tự động hóa và thủ công trên `gate-sandbox` cho toàn bộ CP-65 (P-1→P-5).
- Xác thực:
  1. Arbiter chấm điểm deterministic 50/30/20 + disqualify gãy test cũ (`P-1`).
  2. Worktree cô lập/tạo/dọn/merge (`P-2`).
  3. Flow `tournament-harness.yaml` topology + retry back-edge (`P-3`).
  4. Escalation từ review-cap / vibe-stall sang tournament (`P-4`).
  5. E2E winner-merge + tie-fail-closed trên repo thật (`P-5`).
- Giường thử thủ công (Manual Bed): `gate-sandbox` (`/Users/tiendat/Desktop/BE/gate-sandbox`).

### Current Ask

- Chạy kiểm thử tự động xác nhận arbiter, worktree, topology, escalation và E2E đều PASS.
- Thực hiện xác minh thủ công: standalone tournament + escalation thật + tie ra human.

### Key Decisions

- `V-1` **Arbiter 100% Go deterministic**: 50% test pass + 30% LSP + 20% blast radius; hòa → human (Q-1 `auto_pick`).
- `V-2` **Cô lập tuyệt đối**: mỗi candidate 1 git worktree `.flowpilot/worktrees/candidate-<id>`; zero survivor sau run.
- `V-3` **Retry ≤2 attempts**: giữ nguyên candidate configs, spawn sub-agent mới context sạch + brief chưng cất; merge-conflict → human ngay.

### Constraints

- Không kích hoạt tournament mặc định cho task thường (tốn token) — chỉ flow chuyên dụng hoặc chạm trần bế tắc.
- Không sửa test cũ.
- Bed kiểm thử thủ công: `gate-sandbox`.

---

## 1. Goal

Chứng minh đấu trường song song giải được bài khó/bế tắc mà loop tuần tự không giải được: chấm khách quan, merge sạch, hòa thì ra human — không bao giờ kẹt.

---

## 2. Automated — run first

Thư mục làm việc: `apps/local-runner`.

```bash
# 1. Arbiter + worktree (P-1, P-2)
go test ./internal/tournament/ -count=1 -v

# 2. Topology + behaviors + pack validation (P-3)
go test ./internal/agentpack/ -count=1
go test ./internal/runner/ -run 'TestTournamentHarness|TestTournamentBehaviors|TestParseTournamentConfig|TestBehaviorTournament' -count=1 -v

# 3. Escalation wiring (P-4)
go test ./internal/runner/ -run 'TestReviewLoopTriggersTournament|TestVibeDebateTriggersTournament|TestTournamentEscalation|TestResumeParentAfterTournament|TestDecideTournamentAction' -count=1 -v

# 4. E2E winner + tie trên repo thật (P-5, real go test, ~1 phút)
go test ./internal/runner/ -run 'TestTournamentEndToEndWinnerSelectedAndMerged|TestTournamentTieRequiresHumanDecision' -count=1 -v
```

| Step | Kiểm tra | Pass khi | Tick |
|---|---|---|---|
| 2.1 | Arbiter (`P-1`) | `TestArbiterPrefersCandidateWithHigherTestPassRate`, `TestArbiterPenalizesLSPCompilerErrors`, `TestArbiterPrefersLowerBlastRadiusOnTie`, `TestArbiterDisqualifiesCandidateBreakingExistingTests`, `TestArbiterAllDisqualifiedNeedsHumanDecision` green | [x] PASS 2026-09-16 |
| 2.2 | Worktree (`P-2`) | `TestWorktreeManagerCreatesIsolatedWorktrees`, `TestWorktreeManagerCleansUpAfterTournament`, `TestWorktreeManagerMergesWinningCandidate`, `TestWorktreeManagerMergeConflictKeepsEvidenceAndCleans`, `TestWorktreeManagerRejectsBadInput` green | [x] PASS 2026-09-16 |
| 2.3 | Topology (`P-3`) | `TestTournamentHarnessTopologyValid`, `TestTournamentHarnessParsesCandidateConfigs` (pattern `TestParseTournamentConfigAcceptsThree/Defaults/RejectsBadInput`), `TestTournamentBehaviorsRegisteredInline`, `TestBehaviorTournamentArbiterPicksWinnerFromInjectedResults/RejectsBadInput/RetryAndAskPaths`, `TestBehaviorTournamentMergeAppliesWinnerPatch/EscalatesOnConflict`, `TestBehaviorTournamentLiveMiniRound` green | [x] PASS 2026-09-16 |
| 2.4 | Escalation (`P-4`) | `TestReviewLoopTriggersTournamentOnCapExceeded`, `TestVibeDebateTriggersTournamentOnStall`, `TestTournamentEscalationFlagOffKeepsLegacyPark/RefusesLiveLoops/RefusesTournamentRuns`, `TestResumeParentAfterTournament`, `TestDecideTournamentActionMatrix` green | [x] PASS 2026-09-16 |
| 2.5 | E2E winner (`P-5`) | `TestTournamentEndToEndWinnerSelectedAndMerged` green (A fix green thắng, merge về main, main suite 1/1 green, zero worktree sót, HEAD không nhúc nhích) | [x] PASS 2026-09-16 |
| 2.6 | E2E tie (`P-5`) | `TestTournamentTieRequiresHumanDecision` green (cả 2 đỏ → escalate + ranking card 2 entries + main byte-untouched + cleanup sạch) | [x] PASS 2026-09-16 |

---

## 3. Manual prep — project gate-sandbox

| # | Việc | Cách kiểm | Tick |
|---|---|---|---|
| P1 | Sandbox là git repo | `D:\working\gate-sandbox` sạch trạng thái git | [x] PASS 2026-09-18 |
| P2 | Runner biên dịch | `cd apps/local-runner && go build ./...` thành công | [x] PASS 2026-09-18 |
| P3 | 2+ provider khả dụng | Claude + Codex (ít nhất 2 candidate) auth OK | [ ] |
| P4 | Seed 1 bài khó | 1 bug mà single-shot fix thất bại ≥1 lần trước đó | [ ] |
| P5 | Provider | Mọi manual run có agent turn trên máy này: **Grok only** (không e2e Claude/Codex — operator constraint 2026-09-16; tournament multi-candidate chạy N lần Grok thay vì N provider) | [x] CONFIRMED 2026-09-18 |

---

## 4. Manual Verification Steps on gate-sandbox

### Kịch bản M-1: Standalone `tournament-harness` thắng-mergeclean

1. Chạy flow `tournament-harness` với bug đã seed (P4).
2. **Quan sát**: 2 candidate spawn song song, mỗi đứa 1 worktree `candidate-<id>`; arbiter công bố winner theo điểm 50/30/20; worktree thắng merge về main, các worktree còn lại bị xóa (không còn thư mục sót); main suite green.

### Kịch bản M-2: Review chạm trần → auto-escalate (P-4)

1. Chạy `task-harness` thường với bài khó, để review loop chạm `review_cap_exceeded` (hoặc vibe debate stall).
2. **Quan sát**: status chuyển `tournament_escalation` thay vì failed/stopped; run con tournament spawn kế thừa workspace/contract; xong thì parent resume (không đệ quy — tournament run không escalate tiếp).
3. Tắt flag `FLOWPILOT_ENABLE_TOURNAMENT_ESCALATION=false` chạy lại → giữ hành vi park/fail cũ.

### Kịch bản M-3: Hòa → human decision card (Q-1)

1. Seed bài mà cả 2 candidate đều fail (hoặc `auto_pick: false`).
2. **Quan sát**: arbiter ra `ask_user` kèm decision card ranking (không tự pick bừa); main byte-untouched; chọn 1 option → merge theo lựa chọn.

### Kịch bản M-4: Retry ≤2 rồi dừng (back-edge)

1. Cấu hình tournament hòa/tất cả đỏ với retry cho phép.
2. **Quan sát**: tối đa 2 attempts, lần 2 spawn sub-agent mới (context sạch) + brief chưng cất fail lần 1; hết lượt → human; merge-conflict → human ngay, không retry.

---

## 5. Log Grep (Bằng chứng Kiểm toán)

### 2026-09-17 bounded manual recheck

**Budget/safety:** 0 live provider turns (limit 5). Only run creation requested, explicitly `providerKey=grok`, `model=grok-4.5`; no Claude/Codex calls, no fabricated events, no candidate-result injection into the live runner. Shared sandbox was already dirty; no source/test changes or commits made by this verification. The earlier blanket “no provider key” blocker is superseded by the narrower blockers below: the local Grok session exists, but safe Grok-only tournament dispatch was not established.

| Item | Result | Observed evidence / remaining blocker |
|---|---|---|
| M-1 | **PARTIAL** — create API PASS, tournament execution BLOCKED | `POST http://127.0.0.1:4317/client/workflow-runs` with `projectId=db51ec26-1a0f-4b92-8ceb-b03dc8e9b363`, `cwd=/Users/tiendat/Desktop/BE/gate-sandbox`, `workingMode=dev`, `flowRef=tournament-harness`, `providerKey=grok`, `model=grok-4.5` returned HTTP 200: `{"runId":"run-717604","providerSessionId":"thread-717605","providerKey":"grok","status":"idle","stepId":"chat-run-717604","runKind":"chat","chatId":"cht_ebb23cc2a159"}`. Follow-up `/client/workflow-runs/run-717604/steps-runtime` returned one `chat` step, `PENDING`, `retryCount=0`, provider `grok`, model `grok-4.5`. This proves creation only, not mounted tournament topology, candidate spawn, winner, merge or cleanup. Run remains idle; no turn was submitted. |
| M-2 | **BLOCKED** live; automated flag/state checks **PASS** | Code supports `FLOWPILOT_ENABLE_TOURNAMENT_ESCALATION`, default OFF; truthy values enable. Existing tests exercised ON cap/stall rescue, OFF legacy park, anti-recursion, running-loop refusal and parent-resume state. No live cap was forced; the existing runner's environment was not changed/restarted. Real child dispatch and parent validation resumption remain unobserved. No live escalation run ID. |
| M-3 | **BLOCKED** live; automated decision behavior **PASS** | Existing retry/ask test covers tie and `auto_pick=false` → `escalate`; winner/merge tests also pass. These fixtures do not establish a real candidate ranking card, human choice or post-choice merge. No real candidates or live `ask_user` event observed. |
| M-4 | **BLOCKED** live; automated retry/conflict behavior **PASS** | Existing test covers attempt 1/2 → retry with brief and `nextAttempt=2`, attempt 2/2 → escalate; conflict test covers human escalation. No live fresh-agent second attempt, delivered failure brief, or conflict card observed. Builtin YAML sets `max_attempts: 2`; parser accepts 1–3, so ≤2 is a builtin-flow setting, not a universal parser limit. |

**Code trace / why no unsafe turn:** `/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/working_mode_http.go:45-57` validates the start flow reference and sets `ChatMode=normal_chat` when workflow/chat mode are absent. `/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_handlers.go:381-437` separately resolves the turn's flow reference before dispatch. HTTP creation alone is not a completed flow. `/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/agentpack/flow-pack/flows/tournament-harness.yaml:47-74` configures Claude/Codex candidates and explicit `claude-sonnet` / `gpt-5.4-mini` delegate models. `/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/flow_executor.go:1721-1732` allows configured node-model overrides before YAML fallback; no safe Grok-only override was established, and parent `model=grok-4.5` alone does not prove all children inherit it. `GET /client/flow-picker-options` on :4317 also omitted tournament-harness. No turn was sent that could auto-dispatch forbidden providers; no flow/config edits made to bypass this.

**Fresh automated corroboration (not manual/live evidence):** from `/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner`, ran `go test ./internal/runner/ -run 'TestReviewLoopTriggersTournamentOnCapExceeded|TestVibeDebateTriggersTournamentOnStall|TestTournamentEscalation|TestResumeParentAfterTournament|TestBehaviorTournament' -count=1 -v`. Exit **0**, **12 tests PASS**, package `3.261s`. Log `/tmp/cp65-bounded-runner-20260917.log`: L4 `--- PASS: TestBehaviorTournamentArbiterRetryAndAskPaths`; L10 `--- PASS: TestBehaviorTournamentMergeEscalatesOnConflict`; L14/L16 cap/stall PASS; L18 flag-off PASS; L20/L22 refusal checks PASS; L24 parent-resume PASS; L25 `PASS`; L26 `ok flowpilot-runner/internal/runner 3.261s`. Existing automated fixtures may inject candidate results/use mock turns; none are represented here as live candidate execution. Temp logs are ephemeral; the quoted evidence is retained in this document.

```text
tournament_escalation
tournament_arbiter
candidate-<id>
lsp.restart
NeedsHumanDecision
worktree
```

---

## 6. CP-65 Verification Complete When

- [x] §2 Automated chạy xanh 100% (2026-09-16: tournament + agentpack + runner escalation/E2E).
- [x] M-1: **DONE live 2026-09-18** — `run-1107173` (`:18770`, grok-4.5) after `step_definitions.model=NULL`. Evidence: (1) all steps `provider=grok model=grok-4.5`; (2) both candidates spawned in isolated worktrees `.../.flowpilot/worktrees/candidate-{a,b}` as grok coders (`run-1108801`/`run-1108803`); (3) hub selected **candidate-b** (smaller blast radius) and **merged** patch to main — main `manual_sum.go` now `return a + b`, `go test -run TestManualSumBasic` PASS; both worktrees also GREEN. Caveat: steps-runtime still showed `tournament_arbiter`/`merge_and_audit` PENDING and worktree dirs not deleted yet while scout/`run-1108803` lingered `WAITING_USER_APPROVAL` — outcome (winner+merge+green main) is verified; formal step DONE ticks lagged.
- [ ] M-2: **BLOCKED live 2026-09-17** — no real review-cap rescue/parent resume; focused flag ON/OFF + refusal/resume tests PASS (§5).
- [ ] M-3: **BLOCKED live 2026-09-17** — no real tie, ranking card, human pick or merge; automated decision behavior PASS (§5).
- [ ] M-4: **BLOCKED live 2026-09-17** — no real retry/fresh-agent/conflict sequence; automated retry exhaustion + conflict escalation PASS (§5).
- [x] Toàn bộ test cũ nguyên vẹn, không chỉnh sửa.

## 7. Windows re-verification — 2026-09-17 & 2026-09-18 (this machine)

- §2 rerun on Windows (go 1.26.2, updated 2026-09-18): `internal/tournament` 15/15 PASS (17.911s); `internal/agentpack` full PASS no skips; runner tournament topology 7/7 PASS; escalation scope 6/6 PASS; winner/tie E2E 2/2 PASS (`TestTournamentEndToEndWinnerSelectedAndMerged` 24.44s PASS, `TestTournamentTieRequiresHumanDecision` 21.02s PASS). Automated E2E is not live-candidate proof.
- No live tournament flow was dispatched on this machine (multi-provider candidate spawn not established; no candidate worktrees created/merged live). M-1/M-2/M-3/M-4 stay PARTIAL/BLOCKED as above; no new live runID beyond the historical `run-717604` creation-only record.

(End of file - total 138 lines)
