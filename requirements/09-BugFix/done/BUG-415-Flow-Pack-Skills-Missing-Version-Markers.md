# BUG-415: Flow-pack platform skills ship without `version:` markers — `skill_pack` perpetually stale, bind-init never skipped

## Metadata

- Document ID: `BUG-415`
- Title: `Platform skill-pack SKILL.md files lack version: 6 markers → skill_pack stale after fresh init; init never idempotent`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: [CP-34-Init-tool](../../07-Coding-Plan/done/CP-34-Init-tool.md), evidence `~/fp-beds/lt-evidence/cp34/RESULT.md` (BUG-LIVE-1)
- Feature Keys: `engine-init`, `skill-pack`, `tooling-status`

## AI Quick View

### Summary

- Flow-pack SKILL.md files under `golang/` (46/47), `reactjs/` (4/5), `react-native/` (1/10) ship without a `version: 6` frontmatter marker; `fileMatchesVersion` reads `version:` and reports them not-current.
- Result: `skill_pack` reports `stale` immediately after a successful init, `shouldSkipBindInit` never fires (init is never idempotent — all 9 steps re-run and ~46×4 files are rewritten every bind), and the `skill_pack` verdict is inconsistent (`ok` in init-time `tooling.json` vs `stale` in the status endpoint).
- Unit test `TestEngineInitSkipsCurrentBindTrigger` passes only because it omits `platform` → common group → all versioned.

### Current Ask

- Fixed and verified in the bug-fix wave — see Completion Notes (implemented 2026-09-23).

## Bug report

### Symptom

`GET /client/projects/{id}/engine/status` reports `skillPack.installed=true, current=false` and tooling row `skill_pack=stale` immediately after a fresh, fully-successful bind init. A second `POST /client/projects` on the same directory re-runs all 9 init steps (`skipped=false`) instead of short-circuiting.

### Expected

- After a successful init, all installed skill files carry `version: 6` → `skill_pack` = `ok`/`current=true`.
- A repeated bind on an unchanged pack returns `status=skipped` via `shouldSkipBindInit`.
- The `skill_pack` verdict is identical between init-time `tooling.json` and the status endpoint.

### Actual

- 46/47 `golang/` SKILL.md files, 4/5 `reactjs/`, 1/10 `react-native/` lack `version: 6` (only `golang-conventions` has it; `common/`, `android/` etc. are fine — verified `grep -rL "version: 6" flow-pack/<group> --include=SKILL.md`, 46 hits for golang on HEAD `435e336b`).
- `skill_pack` = `stale` forever; every bind rewrites all platform files and re-runs ledger/catalog rebuild.
- Init-time `tooling.json`/init response says `skill_pack ok` (sentinel-file check in `CheckTool`) while the status endpoint says `stale` (version check) — same tool, two verdicts in the same flow.

### Impact

Every bind on golang/reactjs/react-native projects re-runs full init (file rewrites + ledger/catalog rebuild); the Engine page would permanently show "stale". Idempotent-init guarantee is dead for the majority platform set.

## Reproduction

1. `POST /client/projects {directoryPath: <bed>, platform: "golang"}` → init `status=success`, 9/9 steps ok.
2. `GET /client/projects/{id}/engine/status` → `skillPack.installed=true, current=false`; tooling row `skill_pack=stale` (all 46 golang-group skills `current=false` in all 5 provider rows).
3. `POST /client/projects` same dir again → `engine-init.json`: `skipped=false`, all 9 steps re-ran — expected `status=skipped` per `shouldSkipBindInit`.

## Root cause

- `apps/local-runner/internal/skillpack/flow-pack/{golang,reactjs,react-native}/*/SKILL.md` — files ship without `version: 6` frontmatter.
- `apps/local-runner/internal/skillpack/install.go:340` — `fileMatchesVersion` reads `version:` from frontmatter/first line; missing marker → `skillpack.Status().Current=false`.
- `apps/local-runner/internal/runner/engine_setup.go:484-497` — `buildSkillPackToolStatus` maps `Current=false` → `stale`.
- `apps/local-runner/internal/runner/engine_setup.go:470-482` — `shouldSkipBindInit` requires `Current` → never skips.
- `apps/local-runner/internal/skillpack/install.go` `Install` — never treats the files as current → rewrites all files each run.

## Evidence

- `~/fp-beds/lt-evidence/cp34/RESULT.md` — BUG-LIVE-1 (L-34-1/L-34-2 rows).
- `l34-2-engine-status.json` (skillPack installed=true current=false), `l34-1-tooling.json` (`skill_pack ok`), `l34-1-rebind.json` (second init `skipped=false`).
- Worktree `flowpilot-lt-cp34` @ `435e336b`; verified on main worktree HEAD `435e336b` (`grep -rL "version: 6" internal/skillpack/flow-pack/golang --include=SKILL.md` → 46 files).

## Severity

- `medium` — functional idempotency break + permanent "stale" indicator on the dominant platform packs; no crash, no data loss.

## Completion Notes (implemented 2026-09-23, CA-925)

- Root cause confirmed: 51 embedded flow-pack SKILL.md files carry YAML
  frontmatter but no `version:` key, so `fileMatchesVersion` returns false
  for every installed copy — perpetual `stale`, never-skipped bind init.
- Fix: `skillpack.Install` writes `ensurePackVersionMarker(srcBytes,
  PackVersion)` — the marker is stamped inside existing frontmatter (or as
  a legacy first line when absent). Install-time stamping is self-healing:
  future markerless skills still install correctly versioned, and a real
  older-version file (`version: 5`) is still detected stale.
- Tests: `internal/skillpack/bug415_version_stamp_test.go` — all installed
  files versioned, reinstall idempotent (0 rewrites), `Status.Current`,
  stale-v5 detection preserved.
- Live: `/tmp/fp-live-j/ws` init → `skillpack_install` ok, 0/248 installed
  files missing `version: 6`; follow-up `bind` → `status=skipped`,
  `skillPack.current=true`.
