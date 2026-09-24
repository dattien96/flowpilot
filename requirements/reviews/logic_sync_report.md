# logic_sync_report — CP-71/81/82/83/84 test-coverage audit

Ngày: 2026-09-24 · Chi tiết đầy đủ: `KR-005-cp71-cp84-test-gap-audit.md`

## Tổng hợp trạng thái

| Feature | SYNCED | MISSING | BLOCKED | Verdict |
|---|---|---|---|---|
| CP-71 Worktree Isolation | Core lifecycle + 10 HTTP E2E + desktop toggle | 12 edge (merge modes `recreate_empty`/`archive`/`discard-confirm`, `worktree_unavailable`, `invalid_mode`, 2-run overlap E2E, `ensureGitignore`, `markChatWorktreeState`); thiếu Test-Steps doc | — | Phủ tốt; gap ở merge-mode HTTP paths |
| CP-81 Shared Runner Lifecycle | lifecycle pkg 29 + HTTP API 9 + runner E2E 19 + TUI 14 + desktop 20 | provider-subprocess cleanup, requester attribution, live Task-Manager-kill | **runnerboot suite không compile trên Windows** (Setsid); **supervisor 2/9 fail** (taskkill vs process.kill mock) | Core xanh; Windows-critical paths đỏ |
| CP-82 Multi-Project Ops | boardModel, inline actions, spectator store, uniqueness Go tests | `SessionsBoard` render test, spectator no-subscribe spy, row-render race, live `waiting_approval` thật, M-1…M-7 | — | State layer tốt; component/live thiếu |
| CP-83 Embedded Terminal | worktreePath contract ×3, terminal+panel 19 test | Electron reload/quit cleanup thật, deleted-worktree live, interactive programs, narrow layout | — | Contract tốt; lifecycle Electron thiếu |
| CP-84 Realtime Multi-Lane | ~20 mux/payload Go tests + ~40 desktop state tests, tất cả pass | `handleAllEventsStream` HTTP (0 test), SSE parser, backoff/reconnect loop, gate/worktree_merge/question/dispatch payload tests, notification deep-link e2e, M-1…M-11 + L-1…L-7 | — | Core mạnh; transport + notification + live timing thiếu |

## Findings blocking (không phải missing test — là test không chạy được)

1. `runnerboot_cp81_test.go:422` — `cmd.SysProcAttr.Setsid` không tồn tại trên
   Windows → package fail compile → 13 test CP-81 unrunnable.
2. `supervisorLifecycle.test.ts` — 2 test expect `process.kill` records nhưng
   Windows path dùng `execSync(taskkill)` → fail trên Windows.
3. Docs ghi `npx vitest` — sai runner; đúng là `node --test .phase1-tests/…`
   (103/103 pass khi chạy đúng).

## OUTDATED check

- `quit_kills_reused_runner_test.go`: **SYNCED** — là explicit "Turn off" path
  (fenced shutdown), không phải stale pre-CP-81 test.

## SYNCED-tests verified to pass

- `go test` internal/runner + internal/worktree + internal/lifecycle +
  internal/tui/app: PASS (CP-71 E2E 10/10, CP-81/84 focused 54/54).
- Desktop compiled tests: 103/103 PASS.
- Runnerboot: không verify được (compile fail — BLOCKED).
- Supervisor: 7/9 pass, 2 fail — BLOCKED.
