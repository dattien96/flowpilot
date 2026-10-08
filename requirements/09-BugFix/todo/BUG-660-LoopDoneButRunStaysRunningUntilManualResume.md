# BUG-660 — When the sprint loop reaches `done` the run does not settle: status stays `running` with all steps terminal and no gate/callable action, until an operator POSTs `/resume` which reconciles it to `completed`

- **ID:** BUG-660
- **Severity:** Medium — the run reports "running" forever after real
  work ends; automation polling the run sees a live run and watchers
  never terminate; observed ~7min limbo before manual resume.
- **Status:** OPEN (captured live, run-523131)

## Evidence chain (all live)

1. Sprint 5 closed (`tdd/synthesis/audit=DONE`, rest SKIPPED), loop
   state `done`, no pending gate, no live leg — yet run status stayed
   `running`; `GET /agent-graph` showed nothing in flight.
2. `POST /resume` reconciled: loop `done` → run `completed` immediately.
   The settle path EXISTS but only fires on the resume entry, not on
   loop completion.
3. Same terminal-shape family as BUG-647 (leg→step) but at loop→run
   level: completion of the container does not propagate.

## Root cause (hypothesis)

The sprint-loop completion writes loop state but not the run-status
transition; run completion is computed lazily at resume/read time, so a
finished run without a subsequent event never flips.

## Fix direction

- `F-1` On sprint-loop `done`, synchronously settle the run status
  (completed) in the same ledger write — run status is derived state,
  never left stale.
- `F-2` Sweep-level fallback: a run with loop `done` + all steps
  terminal self-heals to `completed` on the next wedge sweep.

## Regression coverage

- `TestBug660_LoopDoneSettlesRun` — last sprint closes → run
  `completed` without operator input.
- `TestBug660_WatcherSeesTerminal` — polling API returns terminal status
  promptly (watcher auto-exit works).
