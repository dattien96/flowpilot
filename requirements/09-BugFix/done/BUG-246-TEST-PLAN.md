# BUG-246 Manual Test Plan (rtk sandbox) — Results

Environment: desktop app, project bound to `D:\working\gate-sandbox` (rtk-wrapped shell
commands via the user's global CLAUDE.md/RTK hook), YOLO = OFF.

## Round 2 — wrapper-aware `deriveApprovalRule` (skips `rtk`/`sudo`/`time`/`nice`/`npx`/`xargs`)

| # | Step | Expected | Result |
|---|------|----------|--------|
| 1 | Baseline: run any `rtk ...` command | Card shows `rtk ...`; checkbox preview includes the real subcommand (e.g. `rtk git status`, not `rtk git`) | ✅ Done |
| 2 | Remember a read command (`rtk ls`) | Checkbox "…starting with `rtk ls`"; approve+remember | ✅ Done |
| 2b | Repeat `rtk ls -la` | Auto-approve, no card | ✅ Done |
| 3 | Remember a tool-runner command (`rtk git status`) | Checkbox shows full rule `rtk git status` (not `rtk git`); approve+remember | ✅ Done |
| 3a | Repeat `rtk git status` / `rtk git status -sb` | Auto-approve, no card | ✅ Done |
| 3b | Run `rtk git push` | Still asks (rule is pinned to `status`, not the whole `git` tool) | ✅ Done |
| 4 | Run compound command `rtk git status && echo hi` | No checkbox offered; still asks even though `rtk git status` is remembered | ✅ Done |
| 5 | Run a different tool (`rtk npm test`) | Still asks (no matching rule) | ✅ Done |
| 6 | Inspect `.flowpilot/settings/approval-allowlist.json`; restart app / new chat | Rules persisted; remembered commands still auto-approve after restart | ✅ Done |
| 7 | Settings → Engine → "Auto-approved commands" panel | Lists remembered rules; Remove works and the command re-prompts afterward | ✅ Done |

## Outcome

All manual test items above are confirmed working by the user in the rtk sandbox. Automated
coverage for the wrapper-aware derivation lives in
`apps/local-runner/internal/runner/approval_allowlist_test.go`
(`TestDeriveApprovalRule`, `TestMatchesApprovalRule`).

## Round 3 — storage relocation (explored, then reverted)

Storage was briefly moved from `<bound-workspace>/.flowpilot/settings/approval-allowlist.json`
(inside the user's own project repo) to FlowPilot's own app-data dir keyed by `project_id`:
`<flowpilotAppDataDir>/settings/<project_id>/allowlist.json` (`%AppData%\FlowPilot\...` on
Windows), with Drive sync switched from `contextsync.SharedFiles` to an explicit push/pull.

**Reverted per user direction**: FlowPilot already stores several other pieces of per-project
state (ledger, catalog, gate-config, flow-rules) inside the bound workspace's own `.flowpilot/`;
keeping the allowlist there too is more consistent than a second, differently-scoped storage
location. Final design is back to `<bound-workspace>/.flowpilot/settings/approval-allowlist.json`,
riding the existing `EngineStore.SharedFiles` Drive push, with `restoreApprovalAllowlistFromDrive`
pulling on engine setup as before. The wrapper-aware `deriveApprovalRule` fix from Round 2 is
unaffected and remains in place.

Verified after revert: `go build ./...` clean, `approval_allowlist_test.go` (dotFP/workspace-keyed,
including `TestSharedFilesIncludesApprovalAllowlist`) pass, `internal/contextsync` assertions
(5-file `SharedFiles`) pass, desktop `tsc --noEmit` clean. See CA-246 for the full change note.
