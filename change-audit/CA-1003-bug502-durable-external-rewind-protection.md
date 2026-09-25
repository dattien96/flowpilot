# CA-1003 — BUG-502 fix: durable-file external-rewind detection + cross-process write exclusion

## What

Production fix for BUG-502. Root cause was proven after CA-1002 filed it:
an out-of-band `git reset --hard HEAD~1` (CP-71 drill probe cleanup) rewound
the **git-tracked** `.flowpilot/chats/sessions.ndjson` + `approvals.ndjson`
on the live bed while the runner kept appending — 68 durable rows
(19:22–19:29, both projects) silently destroyed; `run-260777`/`run-260889`
then failed rehydration (`run_not_found`/`chat_not_found`, lost
`appr-260887`).

Changes in `apps/local-runner/internal/runner/`:

- `dispatch_flock_unix.go` / `dispatch_flock_windows.go` — new `flockBlock`
  (blocking `LOCK_EX`; Windows no-op per existing best-effort policy).
- `local_file_session_store.go`:
  - `withDurableFileLock` — every session/question/approval write runs under
    a sibling `<file>.lock` blocking flock (dispatch.lock precedent applied
    per-operation), so a sibling runner's append can no longer land between
    `DeleteProviderSession`'s read and its atomic rename.
  - `durableFileGuard` per file — size+mtime of last observation; every write
    re-stats first.
  - `resync{Sessions,Questions,Approvals}Locked` — on out-of-band change
    (grown/shrunk/replaced/deleted/appeared), in-memory map is rebuilt from
    disk before the write: sibling rows merge in, rows no longer on disk are
    dropped with an explicit `modified out-of-band — resynced to disk` log
    naming the run/approval/question ids. Incomplete reads stay fail-closed
    (`*LoadErr` set, memory untouched).
  - `protectDurableFilesFromGit` — appends `.flowpilot/` to
    `<repo>/.git/info/exclude` at store init (repo-local; does not touch the
    tracked `.gitignore`) and warns loudly when durable files are already
    git-tracked (exclude cannot untrack; remediation is `git rm -r --cached`).
- `bug502_external_revert_test.go` — 6 additive tests: external-rewind resync
  for all three files, sibling-append merge, writer blocks on a held lock,
  git-exclude provisioning.

Ordering invariants preserved: durable append still precedes the in-memory
commit (BUG-499); read-modify-rename still aborts on incomplete reads
(BUG-485).

Provider parity: change lives in the durable-store layer below all provider
adapters — provider-agnostic by construction; live-verified on Grok turns.

## Verification

- `go test -run BUG502 ./internal/runner` — 6/6 new tests green.
- `go test -count=1 ./...` (module-wide, `FLOWPILOT_CHAT_STORE_DIR` isolated
  — see below): all packages green except `internal/runner` (env-baseline
  fails: provider detection, MCP adapters, catalog stores, provider-accounts;
  plus the pre-existing TempDir-cleanup flake class — baseline-verified at
  ~33% via stash-compare, not worsened by this change) and
  `internal/structure` (`TestGitNexusDependentsSmokeScopeDiff` — missing
  GitNexus repo index `flowpilot-cp-test`, environment).
- Live on bed (b502 binary): rewind replay detected + logged with dropped
  run_ids; sibling `:19500` append merged via resync; restart rehydration
  consistent; `.git/info/exclude` gained `.flowpilot/` + tracked-state
  WARNING fired at boot.
- Full task-harness on b502 (`run-278048`): 13-node chain ran to
  `flow_run_complete_done` — durable writes under the new flock+resync
  path held up across 3 loop rounds, 2 plan-approval parks, coder
  escalate/continue re-drives, and child-agent churn with zero lost rows.

## Note: test-env isolation issue found (not introduced here)

`ensureChatTranscriptWriter` defaults `FLOWPILOT_CHAT_STORE_DIR` to
`~/.flowpilot/chat-transcripts` — a shared global that accumulated ~88k chat
dirs from test+live runs, pushing `LegIndex` scans inside
`projectRunHistory` heavy enough to tip the package suite past the 10m
default timeout. Suite runs should set `FLOWPILOT_CHAT_STORE_DIR` to a temp
dir (the env var exists exactly for test/per-worktree isolation).
