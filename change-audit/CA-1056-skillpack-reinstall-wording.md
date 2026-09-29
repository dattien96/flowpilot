# CA-1056 — skill-pack re-init wording: "skipped" → "already current"

## What changed

User report: after init, skills are correctly present in the target project yet
the Desktop panel (and TUI) report "0 installed, N skipped". Diagnosis: not a
counting bug — `skillpack.Install` is version-idempotent; a destination
`SKILL.md` already declaring `version: <PackVersion>` is counted in `Skipped`
(never rewritten). The first bind/manual init installs every file; every later
re-init reports `0 installed, ~N skipped` and overwrites `engine-init.json`,
so the panel permanently shows a line that reads like a failed install.

- `engine_setup.go` — `skillpack_install` step detail now reads
  `"%d installed, %d already current, %d install errors"` (both the full and
  skill-only init paths). `Install`'s only skip reason is version-match, so
  "already current" is the truthful label; the wire field stays
  `skippedPaths`.
- `tui/app/init_engine.go` — same wording in the init result message.
- Desktop `projectEngine.ts` — `summarizeProjectEngineInit` renders
  "Completed: all N file(s) already current." when a successful init wrote
  nothing (0 installed, >0 skipped, 0 errors), and "already current" instead
  of "skipped" in the general counts line.

## Red → green

`internal/runner/engine_init_wording_test.go` (new):
`TestEngineInitReinstallReportsAlreadyCurrent` — red before
(`"0 installed, 248 skipped, 0 install errors"`), green after.
Desktop `projectEngine.test.ts` (new): 4 cases pin the summary wording —
all-current, mixed install, error visibility, bind-skip.
