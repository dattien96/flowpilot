# BUG-502 — Second-project chat run durable rows lost across runner restart

## Status
Fixed — root cause proven 2026-09-25, fix landed on build b502, live-verified.

## Live-found during
CP-84 mux matrix + B-59 durability drill on bed `/Users/tiendat/fp-beds/full`.

- `run-260777` (project `db51ec26-...` = Gate-sandbox, second registered
  project): completed a real grok turn (`PONG-B` + transcript persisted to
  `run-260777-turns.ndjson`), then a write-intent turn parked it
  `waiting_approval` with gate card + approval `appr-260887`.
- `run-260889` = devin leg of the same chat (`cht_12594b9ce216`).
- After `kill -9` + restart (boot 19:36:34):
  - `GET /client/workflow-runs/run-260777` → `run_not_found`
  - chat timeline for `cht_12594b9ce216` → `chat_not_found`
  - pending approval `appr-260887` unrecoverable.
- Durable file audit: `sessions.ndjson` / `approvals.ndjson` /
  `questions.ndjson` contain **zero** rows for run-260777/260889; the shared
  session file holds only `project_id=957928cc` (workspace) rows — 377 rows.
  The per-run turns file for run-260777 still exists, so the run was never
  deleted through `deleteChatSession` (which would have removed it too).
- Control: `run-260775` (workspace project, same shape, seconds apart) —
  4 session rows present, rehydrates as `cancelled` after restart.

## Root cause — proven

**Out-of-band `git reset --hard HEAD~1` at 19:30:50 rewound the git-tracked
`.flowpilot/` durable files** while the runner kept serving.

Proof chain:

| Evidence | Detail |
|---|---|
| Reflog | `HEAD@{0}: reset: moving to HEAD~1` at 19:30:50 — the probe-commit cleanup in the CP-71 L-7 drill |
| File birth times | `sessions.ndjson` recreated 19:30:50, `approvals.ndjson` 19:30:49 — exact reset window |
| Byte-identical prefix | committed blob (Sep-24 snapshot, 318 lines) == current file's first 318 lines — the file was reverted to HEAD then appended onto |
| Git tracking | `.gitignore` excludes only `.flowpilot/worktrees/`; 152 files under `.flowpilot/` were tracked, including `sessions.ndjson`/`approvals.ndjson` |
| Gate diff log | `sessions.ndjson Status:M` at 19:23 — the lost rows were written to disk before the rewind |

So this was **not** a write-path gap (in-process reproducer and a fresh
second-project live run `run-266599` both persisted correctly). The store's
real defect: it **silently accepted an external rewind** — appended onto the
reverted file without noticing the 68 rows (19:22–19:29, both projects) that
had just been destroyed, and its in-memory map kept phantom state. A sibling
runner process (`:19500`, build `e85776b3`) also shares the same files with
no write exclusion — an append landing between `DeleteProviderSession`'s
read and atomic rename would be discarded the same silent way.

## Fix (b502)

Three layers in `internal/runner/local_file_session_store.go` +
`dispatch_flock_{unix,windows}.go`:

1. **Cross-process write exclusion** — every session/question/approval write
   runs under a blocking `flock` on a sibling `<file>.lock` (the existing
   `dispatch.lock` precedent applied per-operation). A sibling append can no
   longer land between a rewrite's read and its `os.Rename`.
2. **Out-of-band change guard + resync** — each durable file carries a
   size+mtime observation; every write re-stats first. On any change (grown,
   shrunk, replaced, deleted, appeared) the in-memory map is fully rebuilt
   from disk *before* the write — disk stays SSOT, divergent rows are dropped
   with an explicit `[session-store] … modified out-of-band — resynced to
   disk: N durable rows no longer on disk (run_ids: …)` log instead of
   silent phantom state, and sibling rows are merged in.
3. **Git protection** — `NewLocalFileSessionStore` appends `.flowpilot/` to
   `<repo>/.git/info/exclude` (repo-local, never touches tracked `.gitignore`)
   so new durable files stay out of git scope, and logs a loud warning with
   the `git rm -r --cached .flowpilot` remediation when durable files are
   already tracked — the live bed was exactly this case.

Fail-closed preserved: a torn/incomplete resync read surfaces
`sessionsLoadErr` and never replaces memory with a partial view.

## Live verification on b502 (2026-09-25)

| Check | Result |
|---|---|
| Second-project run write path | `run-273982` on `db51ec26`: 3 session rows appended across turns |
| Incident replay: `git checkout HEAD -- sessions.ndjson` (390→318 lines) | next write logged `modified out-of-band — resynced to disk: 14 durable rows no longer on disk` with all run_ids named; memory resynced to disk truth |
| Self-heal forward | run-273982 kept working; its new append landed on the rewound file |
| Sibling process append (`:19500` old binary, no flock) | `run-82865` created on `:19500` merged into `:19400`'s store via resync — visible in `GET /client/projects/db51ec26/workflow-runs` on `:19400` |
| Restart consistency | after restart: `run-273982` rehydrates `cancelled`, `run-82865` `idle` — no phantom `run_not_found` |
| Git protection | boot log shows the git-tracked WARNING; `.git/info/exclude` gained `.flowpilot/` |

## Regression guards

- `internal/runner/bug502_external_revert_test.go` — 6 tests: external rewind
  resync for sessions/approvals/questions, sibling-append merge, writer
  blocks on a held durable lock, `.git/info/exclude` provisioning.
- `internal/runner/bug502_cross_project_session_persist_test.go` —
  `TestBug502SessionRowPersistsForSecondProjectRun`: the write-path lock-in
  (passes before and after the fix).
