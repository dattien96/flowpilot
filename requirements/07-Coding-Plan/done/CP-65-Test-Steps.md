# CP-65 Test Steps — Multi-Candidate Tournament Harness

## Metadata

- Document ID: `CP-65-TEST-STEPS`
- Title: `CP-65 Verification Steps (Automated + gate-sandbox Manual)`
- Phase: `verification`
- Status: `ready`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-16`
- Last Updated: `2026-09-16`
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
| P1 | Sandbox là git repo | `git -C /Users/tiendat/Desktop/BE/gate-sandbox status` sạch | [ ] |
| P2 | Runner biên dịch | `cd apps/local-runner && go build ./...` thành công | [ ] |
| P3 | 2+ provider khả dụng | Claude + Codex (ít nhất 2 candidate) auth OK | [ ] |
| P4 | Seed 1 bài khó | 1 bug mà single-shot fix thất bại ≥1 lần trước đó | [ ] |
| P5 | Provider | Mọi manual run có agent turn trên máy này: **Grok only** (không e2e Claude/Codex — operator constraint 2026-09-16; tournament multi-candidate chạy N lần Grok thay vì N provider) | [ ] |

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
- [ ] M-1: standalone tournament thắng-merge-clean trên sandbox, zero worktree sót. *(blocked headless 2026-09-16: cần ≥2 provider thật spawn candidate — env không có key nào)*
- [ ] M-2: chạm trần → escalate → parent resume; flag-off giữ legacy. *(blocked cùng lý do M-1)*
- [ ] M-3: hòa → decision card ranking → human pick → merge. *(blocked cùng lý do M-1)*
- [ ] M-4: retry ≤2 attempts rồi human; merge-conflict human ngay. *(blocked cùng lý do M-1)*
- [ ] Toàn bộ test cũ nguyên vẹn, không chỉnh sửa.

(End of file - total 138 lines)
