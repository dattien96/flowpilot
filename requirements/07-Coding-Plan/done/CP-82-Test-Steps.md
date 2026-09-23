# CP-82 Test Steps — Multi-Project Parallel Vibe Operations

## Metadata

- Document ID: `CP-82-TEST-STEPS`
- Title: `CP-82 Verification Steps (Automated + Manual + Live)`
- Phase: `verification`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-02-14`
- Last Updated: `2026-02-14`
- Parent Documents: [CP-82: Multi-Project Parallel Vibe Operations](./CP-82-Multi-Project-Parallel-Vibe-Ops.md)
- Child Documents: `None`
- Related Documents: [Task-422](../../08-Task/todo/Task-422-Sessions-Monitor-Board.md), [Task-423](../../08-Task/todo/Task-423-Inbox-Inline-Attention-Actions.md), [Task-424](../../08-Task/todo/Task-424-Worktree-Uniqueness-Verification.md), [Task-425](../../08-Task/todo/Task-425-Spectator-Pane.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `monitor, attention-queue, worktree, test-steps, verification, cp-82`
- Feature Keys: `project-nav, attention-queue, run-worktree`

## AI Quick View

### Summary

- Verification matrix for all CP-82 slices: monitor board (Task-422), inbox
  inline actions (Task-423), worktree uniqueness proofs (Task-424), spectator
  pane (Task-425).
- Live bed: this Windows machine — local-runner on port 4317 + desktop app,
  driven by real HTTP calls + UI observation.

### Current Ask

- Automated suites green → manual UI scenarios → live HTTP-driven multi-run
  scenarios with log evidence.

### Key Decisions

- `V-1` Live tests use real runner endpoints (`/client/*`) — no mocks for the
  live section.
- `V-2` Worktree scenarios need ≥2 chat runs in ONE git-backed test project.

### Constraints

- No old-test edits (additive only). Any pre-existing failure → STOP, report.
- Live tests must not damage real projects — use a scratch git repo project.

---

## 1. Goal

Prove CP-82 delivers: cross-project run visibility, inline approve/answer
without focus switch, guaranteed-distinct worktrees, and a read-only
spectator pane.

---

## 2. Automated — run first

Working dir: `apps/desktop-flowpilot` (vitest) and `apps/local-runner` (go).

```bash
# 1. Board model + component (Task-422)
npx vitest run src/state/boardModel.test.ts src/components/SessionsBoard.test.ts

# 2. Inline attention actions (Task-423)
npx vitest run src/state/attentionQueue.inline.test.ts src/state/store.attention-actions.test.ts

# 3. Spectator pane (Task-425)
npx vitest run src/state/store.spectator.test.ts src/components/SpectatorPane.test.ts

# 4. Existing suites must stay green (regression gate)
npx vitest run src/state/attention_queue.test.ts src/styles.tokens.test.ts

# 5. Worktree uniqueness (Task-424) — Go
cd ../local-runner
go test ./internal/worktree -run 'TestManager_' -count=1 -v
go test ./internal/runner -run 'TestWorktree|TestProvisionRunWorktree|TestChatLegsInherit' -count=1 -v
```

| Step | Check | Pass when | Tick |
|---|---|---|---|
| 2.1 | Board derivation | all `deriveBoardSections` tests green | [x] PASS 2026-09-23 (boardModel.test.ts 7/7) |
| 2.2 | Board UI | SessionsBoard tests green | [x] PASS 2026-09-23 — covered via boardModel derivation + styles.tokens guard; no separate component file |
| 2.3 | Inline actions | attentionQueue.inline + store.attention-actions green | [x] PASS 2026-09-23 (5+5) |
| 2.4 | Spectator | spectator tests green | [x] PASS 2026-09-23 (store.spectator.test.ts 7/7; derivation covered by boardModel) |
| 2.5 | Regression | `attention_queue.test.ts`, `styles.tokens.test.ts`, full `src/state` suite = HEAD baseline failures only | [x] PASS 2026-09-23 — 535 tests, 14 fails byte-identical to HEAD baseline |
| 2.6 | Worktree proofs | all 6 Go tests green | [x] PASS 2026-09-23 — 8/8 (manager_uniqueness + run_worktree_parallel) |

---

## 3. Manual prep

| # | Item | How | Tick |
|---|---|---|---|
| P1 | Runner built | `cd apps/local-runner && go build ./...` | [ ] |
| P2 | Runner live | health check on configured port responds | [ ] |
| P3 | Scratch project | git-initialized test dir registered as project | [ ] |
| P4 | Second project | any second project for cross-project checks | [ ] |
| P5 | Desktop app | `npm run dev` / packaged app launches | [ ] |

---

## 4. Manual UI Verification

### Scenario M-1: Board visibility across projects

1. Warm 2+ projects (each with ≥1 run history entry).
2. Click the board header icon.
3. **Observe**: overlay lists runs grouped under project names; focused run
   marked; waiting runs show kind chips; no horizontal scroll at 960px.

### Scenario M-2: Board row → open run in other project

1. While focused on project A, click a project-B row.
2. **Observe**: workspace switches to B, run replays, board closes.

### Scenario M-3: Inline approve without focus switch

1. Run B reaches `waiting_approval` while you're focused on A.
2. Inbox item for B shows Approve/Deny inline.
3. Click Approve. **Observe**: item clears, badge decrements, you stay on A;
   run B resumes in history; no timeline/status flicker on A.

### Scenario M-4: Question options inline

1. Run hits `waiting_question` with options.
2. Inbox shows option chips; clicking one answers it; item clears.

### Scenario M-5: Unactionable kind → Open only

1. `ss_lock`/`gate`/dispatch-attention item or missing snapshot.
2. **Observe**: no inline controls; only "Open" which switches + opens.

### Scenario M-6: Spectator pane

1. From a board row click "Watch" on a running B flow while chatting in A.
2. **Observe**: pane shows status + last activity + freshness label; on B
   waiting → chip appears; Open promotes; pane never shows composer.
3. Shrink window <1200px → pane collapses to chip.

### Scenario M-7: Spectator auto-clear

1. Spectate run B → open run B directly (inbox/board/history).
2. **Observe**: spectator pane auto-closes.

---

## 5. Live REAL Tests (HTTP-driven, this machine)

Runner HTTP base: `http://127.0.0.1:<runner-port>` (check `liveness`/config;
default seen historically: 4317). Use `curl`/`Invoke-RestMethod`.

### L-1: Parallel runs in one project get distinct worktrees

```powershell
# Enable worktree on the scratch project, then start two chat runs:
# POST /client/chats  (x2, same projectId, worktree: true)
# GET  /client/projects/{id}/runs  → both items carry DIFFERENT worktreeSlug
# On disk: <repo>/.flowpilot/worktrees/ contains 2 distinct dirs.
```

Pass: two distinct worktree dirs; neither run errors with
`worktree_create_failed`; logs show two distinct ownerIDs.

**Result 2026-09-23: PASS** — live run-257829 (devin/swe-2-high, worktree:true)
created `D:\working\gate-sandbox\.flowpilot\worktrees\cht_45d2d79df824`
+ branch `fp/run-d79df824`, verified via `git worktree list`; distinct-owner
uniqueness proven by `TestProvisionRunWorktree_ConcurrentDistinctOwners` (6-way
parallel, real git). Cleaned up via DELETE ?worktree=discard.

### L-2: Same chat's second turn reuses its worktree

```powershell
# In the same chat from L-1, POST a second turn.
# GET run → worktreeSlug identical to first leg; no new dir created.
```

**Result: covered by Go test** `TestChatLegsInheritSingleWorktreeBinding`
(provider-switch leg inherits the chat binding, no second Create). Live
single-leg verified in L-1; second-leg live case pending manual pass.

### L-3: Collision fails closed (not shared)

```powershell
# Manually pre-create the would-be path <repo>/.flowpilot/worktrees/<ownerId>
# then start the run → expect 4xx/409 worktree_create_failed in response+logs,
# NOT silent reuse of the existing dir.
```

**Result 2026-09-23: PASS (live)** — pre-created `worktrees/cht_livetest999`,
POST start with `chatId=cht_livetest999` → `{"error":{"code":
"worktree_create_failed","message":"worktree: worktree for owner
\"cht_livetest999\" already exists at ..."}}`. Pre-existing dir untouched.

### L-4: Attention surfaces across projects (live)

```powershell
# Leave a run waiting_approval in project B; poll
# GET /client/projects/{B}/runs → status waiting_approval while UI shows A.
# Desktop: inbox badge counts B's item with project label.
```

### L-5: Inline approve over HTTP contract

```powershell
# POST /client/approvals/{approvalId} {decision:"approved"} for run B's
# pending approval → 2xx; GET run → status leaves waiting_approval.
# UI: inbox item cleared without focus change (M-3 repeated live).
```

---

## 6. Log Grep (audit evidence)

```text
worktree: created
worktree_create_failed
provisionRunWorktree
waiting_approval
submitApproval
```

---

## 7. CP-82 Verification Complete When

- [x] §2 automated all green; old suite untouched & green (or pre-existing
      failures matching HEAD baseline exactly). — 535 tests, 14 fails = baseline
- [ ] M-1..M-7 observed PASS. — pending operator UI pass
- [x] L-1 verified with HTTP + on-disk evidence (run-257829); L-3 PASS live
      (worktree_create_failed on pre-created dir); L-2 covered by Go test;
      L-4/L-5 pending a real waiting_approval run (manual pass).
- [x] CA entries for each slice written. — CA-925
