# CA-246: Persistent Per-Project "Don't Ask Again" Command Allowlist (YOLO-Off)

## Summary

BUG-246: under YOLO-off, FlowPilot re-prompted for essentially every command a coding
agent ran (safe or not) with no way to remember a decision. Empirical testing (real
`claude` 2.1.191 with FlowPilot's exact flags) confirmed Claude's `default` mode already
auto-allows read-only tools/bash and only gates writes/`cd`/compound/tool-runners; the
missing piece was persisted trust. This adds a per-project, Drive-synced "don't ask again"
allowlist at FlowPilot's provider-neutral approval layer, so it fixes Claude and Codex
together. Claude's own `settings.json` allow-rule wipe (CA-079) is intentionally kept —
trust lives in FlowPilot's config, never Claude's, so BUG-069 cannot recur.

The user verified the feature end-to-end in an rtk-wrapped sandbox (`D:\working\gate-sandbox`)
across three iterations: (1) the initial cut, (2) a wrapper-aware fix to `deriveApprovalRule`
so `rtk`-prefixed commands pin the real executable+subcommand instead of over-widening to
`rtk <tool>`, and (3) a same-day exploration that relocated storage to FlowPilot's own
app-data dir keyed by `project_id` (so rules would follow a project across rebinds), which
was then reverted back to the workspace-rooted `.flowpilot/settings/` — see Residual Notes.

## What Changed

### Runner (`apps/local-runner`)

- `approval_allowlist.go` (new): storage for `.flowpilot/settings/approval-allowlist.json`,
  rooted at the bound workspace (same directory ledger/catalog/gate-config/flow-rules already
  use); `deriveApprovalRule` (granularity B: executable + subcommand, e.g. `git status`; skips
  leading wrapper tokens `rtk`/`sudo`/`time`/`nice`/`npx`/`xargs` so `rtk git status` pins
  `rtk git status`, not the too-broad `rtk git`); `isCompoundCommand` (fail-closed on `&&`,
  `||`, `|`, `;`, `&`, `>`, `<`, backtick, `$(`, `(`, `)`, `{`, `}`, newlines);
  `matchesApprovalRule` (exact token-prefix, rejects compound); read/write/add/remove helpers;
  `GET`/remove HTTP handlers resolve `workingDirectory` via `resolveEngineWorkingDirectory`,
  matching the existing gate-config endpoints.
- `provider_event.go`: `ApprovalDetails.Kind` ("exec"|"file"|"mcp"|"other") so only shell
  commands are eligible for the allowlist.
- `claude_permission_mcp.go` / `codex_adapter.go`: set `Kind` from the tool/method
  (Bash & exec approval methods → "exec"; file-edit/apply-patch → "file"; MCP elicitation → "mcp").
- `interactive_service.go`: `RequestApproval` evaluates the admin denylist first, then a
  user-remembered exec-command match keyed by `rs.workspaceCwd` (auto-approve, audited as
  `policy_user_remembered`), then the admin allowlist, then ask. `submitApprovalDecision(
  approvalID, decision, remember)`: on approve+remember it derives a granularity-B rule from
  an exec command and persists it under `<rs.workspaceCwd>/.flowpilot` (compound commands and
  non-exec approvals are never remembered).
- `interactive_handlers.go`: `handleApprovalDecision` parses `remember`; registers the two
  approval-allowlist routes.
- `contextsync/local.go`: `approval-allowlist.json` is part of `EngineStore.SharedFiles()`
  (Drive push), alongside ledger/catalog/chat-summary/flow-rules.
- `engine_drive_sync.go`: `restoreApprovalAllowlistFromDrive(projectID, dotFlowpilotDir)`
  (additive union pull), mirroring the chat-summary restore.
- `engine_setup.go`: calls the allowlist restore during engine init/bind, next to the
  chat-summary restore, before the generic `SharedFiles` push.
- **Not changed**: `ensureClaudeConfigSettings` — the per-turn allow-rule wipe is kept
  deliberately (load-bearing for BUG-069).

### Desktop (`apps/desktop-flowpilot`)

- `components/ApprovalCard.tsx`: a "Don't ask again" checkbox shown only for single (non-compound)
  shell-command approvals, with a wrapper-aware preview of the rule that will be stored; carried
  on approve only, never on deny. Covers both single and grouped approval cards.
- `types/contract.ts`, `state/store.ts`, `client/HttpWsRunnerClient.ts`, `client/MockRunnerClient.ts`:
  thread the optional `remember` flag through `submitApproval` / `approve`.
- `components/settings/projectEngine.ts`: `fetchApprovalAllowlist(projectId, workingDirectory)` +
  `removeApprovalAllowRule(projectId, workingDirectory, rule)` helpers.
- `components/settings/EngineSettings.tsx`: "Auto-approved commands" panel to view and remove
  persisted rules (they persist forever, so removal is essential); gated on a project +
  workspace binding being selected, same as the rest of the Engine settings panel.

### Tests

- `approval_allowlist_test.go`: compound detection, rule derivation (including wrapper cases),
  token-prefix matching, read/write/remove round-trip, Claude/Codex `Kind` mapping,
  `EngineStore.SharedFiles` inclusion, persist round-trip (approve/deny/compound/file), and
  auto-approve through `RequestApproval`.
- `internal/contextsync/contextsync_test.go`: shared-file count assertions include the allowlist
  (5 files total).

### Docs

- `requirements/09-BugFix/done/BUG-246-Yolo-Off-Gates-All-Commands-Add-Persistent-Command-Allowlist.md`.
- `requirements/09-BugFix/done/BUG-246-TEST-PLAN.md` — manual rtk-sandbox test log across all
  three iterations (all items done).

## Verification

- `go build ./...` — clean; `gofmt` clean.
- `approval_allowlist_test.go` and `internal/contextsync` — pass; runner approval HTTP tests — pass.
- Desktop `tsc --noEmit` — clean.
- Manual: user verified the full checkbox → remember → auto-approve → compound-still-asks →
  persist → Settings-remove flow in an rtk-wrapped sandbox, confirming behavior both before and
  after the storage-location detour.
- Pre-existing failures confirmed unrelated via `git stash` on a clean tree:
  `TestCodexResumeCommandUsesYoloDerivedSandboxAndApproval` (environmental `sh` on Windows) and
  phase1 test-fake drift (SupabaseAuth / navigatorCatalog / settingsHelpers).

## Residual Notes

- **Storage location detour (explored and reverted the same day)**: a variant stored rules in
  FlowPilot's own app-data dir (`%AppData%\FlowPilot`, `<home>/.flowpilot` fallback) keyed by
  `project_id` rather than workspace path, so a project's rules would follow it across
  rebinds/machines without relying on Drive sync alone. The user chose to revert this: FlowPilot
  already stores several other pieces of per-project state (ledger, catalog, gate-config,
  flow-rules) inside the bound workspace's own `.flowpilot/`, and keeping the allowlist there too
  was judged more consistent than introducing a second, differently-scoped storage location for
  one file. If a project is rebound to a different local path, the allowlist does not
  automatically follow — Drive sync (push on write, pull on engine setup) is what carries it to
  a fresh binding/machine.
- `addApprovalAllowRule` is read-modify-write without a lock; approvals are human-paced and
  effectively serialized per run, so a lost update is unlikely — a mutex could be added if
  concurrent multi-run remembers on one workspace ever prove racy.
- The `GET`/remove approval-allowlist HTTP handlers are additive; only the desktop settings panel
  consumes them today.

# ---8<--- flowpilot:change-ledger
feature_key: yolo-policy
source_doc_id: BUG-246
change_type: bugfix
summary: Persistent per-project Drive-synced "don't ask again" command allowlist (stored under the bound workspace's .flowpilot/settings, alongside ledger/catalog/gate-config) so YOLO-off stops re-prompting known single shell commands (Claude + Codex), without weakening gating for compound/dangerous commands
# --->8---
