# BUG-246: YOLO-Off Prompts For Every Command; Add A Persistent Per-Project "Don't Ask Again" Command Allowlist

## Metadata

- Document ID: `BUG-246`
- Title: `YOLO-Off Prompts For Every Command; Add A Persistent Per-Project "Don't Ask Again" Command Allowlist`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-07`
- Last Updated: `2026-07-07`
- Parent Documents: `none`
- Child Documents: `none`
- Related Documents: `change-audit/CA-246-persistent-approval-command-allowlist.md`, `change-audit/CA-079-fix-claude-yolo-off-always-auto-approve.md`, `change-audit/CA-081-fix-codex-yolo-off-write-gate-and-selected-skill-names.md`
- Replaces: `none`
- Tags: `yolo-policy, approval, permission, claude, codex, drive-sync, approval-fatigue`

## AI Quick View

### Summary

- Report: in YOLO-off mode every command asks for approval; only genuinely mutating commands (edit/delete/write) should ask, not reads / `cd` / etc.
- Investigation (empirical, real `claude` 2.1.191, FlowPilot's exact flags) showed Claude's `default` permission mode ALREADY auto-allows read-only tools (`Read`) and read-only bash (`pwd`, `cat`, `ls`, `grep`) — only writes, `rm`, redirects, `cd`, and every compound command route to FlowPilot's approve tool. Codex `untrusted` behaves analogously. So reads are not the real pain.
- Real pain: Claude/Codex native classification is conservative — it gates `cd`, ALL compound commands (`&&`, `|`, ...), and every tool-runner (`git`, `npm`, `go` ...). For a coding agent that is most commands, and FlowPilot never accumulates trust because Claude's own `settings.json` allow-rules are deliberately wiped every turn (BUG-069 / CA-079). The desktop app feels quiet only because it persists "don't ask again" allow-rules; FlowPilot had no equivalent.
- Fix: a per-project, persisted "don't ask again" allowlist at FlowPilot's provider-neutral approval layer (`turnBridge.RequestApproval`, shared by Claude and Codex). Approving a shell command with "remember" stores an executable+subcommand rule (granularity B, e.g. `git status`); future matching single commands auto-approve. Compound commands are never remembered and never auto-approved. Claude's `settings.json` wipe is intentionally left intact (trust lives in FlowPilot's layer, never in Claude's config, so BUG-069 cannot recur).
- Storage: rules live in `.flowpilot/settings/approval-allowlist.json` inside the user's bound project workspace — the same place ledger/catalog/gate-config/flow-rules already live — and ride the existing `contextsync.SharedFiles` Drive push, plus a pull on engine setup mirroring the chat-summary restore. (A same-day exploration relocated storage to FlowPilot's own app-data dir keyed by `project_id`, so rules would follow a project across rebinds/machines; the user reverted this for consistency with the rest of the context-engine state — see Residual Notes in CA-246.)

### Current Ask

- Stop YOLO-off from re-prompting for the same safe/known command every time, without weakening gating for genuinely dangerous or compound commands, and let that trust persist per project and sync across machines like other `.flowpilot` config.

### Key Decisions

- `D-1` Granularity B (executable + subcommand): remember `git status`, not the exact line (too narrow) nor just `git` (would green-light `git push`). Confirmed with the user. Leading wrapper tokens (`rtk`, `sudo`, `time`, `nice`, `npx`, `xargs`) are skipped so a wrapped command like `rtk git status` pins the real exec+subcommand (`rtk git status`) instead of collapsing to `rtk git` (which would green-light `rtk git push`) — needed for rtk sandboxes.
- `D-2` Only single shell commands are rememberable/auto-approvable. Compound commands (`&&`, `||`, `|`, `;`, `&`, `>`, `<`, backtick, `$(`, `(`, `)`, `{`, `}`) are always re-asked, so a remembered `git status` can never green-light `git status && rm -rf /`.
- `D-3` Trust is persisted in FlowPilot's own `.flowpilot` config inside the bound workspace, NOT Claude's `settings.json`. The Claude `settings.json` allow-rule wipe (CA-079) is kept — this sidesteps BUG-069 entirely because Claude/Codex still route every non-native-read command to FlowPilot.
- `D-4` Scope: persist forever, per project, synced to Drive (rides the existing `contextsync.SharedFiles` push; a pull/restore on engine setup mirrors the chat-summary restore). Applies to Claude and Codex uniformly because both go through `RequestApproval`.
- `D-5` Denylist precedence preserved: the admin policy denylist is evaluated before the user allowlist, so a denied command is never auto-approved by a remembered rule.

### Constraints

- Do not modify `ensureClaudeConfigSettings` (the per-turn allow-rule wipe is load-bearing for BUG-069).
- "Don't ask again" is offered only for `kind == "exec"` approvals; file writes (`applyPatch`/`fileChange`/Write/Edit) and MCP elicitation are never remembered.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/approval_allowlist.go` (new) — storage rooted at `<workingDirectory>/.flowpilot`, rule derivation (wrapper-aware), compound detection, token-prefix matcher, GET/remove HTTP handlers (resolve `workingDirectory` via `resolveEngineWorkingDirectory`).
- `apps/local-runner/internal/runner/interactive_service.go` — `RequestApproval` consults the allowlist via `rs.workspaceCwd` (exec only, denylist first); `submitApprovalDecision(..., remember)` persists the rule under `rs.workspaceCwd`.
- `apps/local-runner/internal/runner/provider_event.go`, `claude_permission_mcp.go`, `codex_adapter.go` — `ApprovalDetails.Kind` classification.
- `apps/local-runner/internal/contextsync/local.go`, `engine_drive_sync.go`, `engine_setup.go` — Drive push (`EngineStore.SharedFiles`) + pull (`restoreApprovalAllowlistFromDrive`).
- `apps/desktop-flowpilot/src/components/ApprovalCard.tsx`, `state/store.ts`, `client/*`, `components/settings/EngineSettings.tsx`, `settings/projectEngine.ts` — checkbox + settings management UI; `fetchApprovalAllowlist`/`removeApprovalAllowRule` take `projectId` + `workingDirectory`.

## 1. Issue Summary

Under YOLO-off, FlowPilot asked for approval on essentially every command a coding agent ran, including safe/known ones, with no way to remember a decision — so the user faced constant approval fatigue.

## 2. Parent Links

- Same feature area (`yolo-policy`) as BUG-069/CA-079 (Claude YOLO-off allow-rule wipe) and BUG-070/CA-081 (Codex YOLO-off write-gate). This builds a persistent allowlist ON TOP of those without regressing them.

## 3. Environment and Reproduction

- environment: any project, YOLO-off, Claude or Codex provider, coding agent running shell commands.
- reproduction: run a YOLO-off turn; observe an approval card for `cd`, `git status`, `npm test`, etc. every time, with no "don't ask again" option.
- frequency: every turn (deterministic).

## 4. Expected vs Actual

- expected: once the user approves a safe command with "don't ask again", the same command auto-approves next time; dangerous/compound commands still ask.
- actual: every command re-prompts; no persistence.

## 5. Impact

- users affected: everyone using YOLO-off (the safer default posture).
- severity: medium UX (approval fatigue pushes users to YOLO-on, defeating gating).

## 6. Root Cause

- Confirmed empirically (real `claude` 2.1.191 with FlowPilot's exact flags `--permission-mode default` + empty allow-list + `--permission-prompt-tool`): Claude's `default` mode auto-allows read-only tools/commands, but gates `cd`, all compound commands, and all tool-runners. FlowPilot had no mechanism to remember an approval, and deliberately wipes Claude's own allow-rules each turn (CA-079), so it never accumulates trust the way the desktop app does.

## 7. Fix Strategy

- `F-1` Add a per-project persisted allowlist consulted by the provider-neutral `turnBridge.RequestApproval` (fixes Claude and Codex together).
- `F-2` `ApprovalDetails.Kind` classifies exec vs file vs mcp so only shell commands are eligible.
- `F-3` "Remember" flows from the desktop approval card → `submitApprovalDecision(remember=true)` → `deriveApprovalRule` (granularity B, compound rejected) → `addApprovalAllowRule`, stored under the bound workspace's `.flowpilot`.
- `F-4` Matcher uses exact token-prefix matching and rejects compound commands, so remembered rules can never green-light chained/dangerous commands.
- `F-5` Drive push via `contextsync.SharedFiles` (existing) + a new `restoreApprovalAllowlistFromDrive` pull on engine setup (additive union) so the allowlist travels across machines.
- `F-6` Settings → Engine "Auto-approved commands" panel to view/remove rules (they persist forever); gated on a project + workspace binding being selected, matching the rest of the panel.

## 8. Validation

- `V-1` `go build ./...` — clean; `gofmt` clean.
- `V-2` `approval_allowlist_test.go` — rule derive/compound/token-prefix match (including wrapper cases), read/write/remove round-trip, `Kind` mapping (Claude + Codex), `SharedFiles` inclusion, persist round-trip (approve/deny/compound/file cases), and auto-approve through `RequestApproval` — all pass.
- `V-3` `internal/contextsync` — shared-file count assertions (5 files including the allowlist); all pass.
- `V-4` Runner approval HTTP tests (`TestApprovalDecision*`, policy auto-approve/deny) — pass.
- `V-5` Desktop `tsc --noEmit` — clean (app + settings UI).
- `V-6` Manual round-trip verified by the user in an rtk-wrapped sandbox (`D:\working\gate-sandbox`) across three iterations (initial cut, wrapper-aware fix, storage-location exploration + revert) — see `BUG-246-TEST-PLAN.md`.
- `V-7` Pre-existing failures unrelated to this change confirmed via `git stash`: `TestCodexResumeCommandUsesYoloDerivedSandboxAndApproval` (environmental `sh` on Windows) and phase1 test-fake drift (SupabaseAuth/navigatorCatalog/settingsHelpers).

## 9. Regression Guard

- tests: `TestIsCompoundCommand`, `TestDeriveApprovalRule`, `TestMatchesApprovalRule`, `TestApprovalAllowlistReadWriteRoundTrip`, `TestCodexApprovalKind`, `TestClaudeApprovalDetailsKind`, `TestSharedFilesIncludesApprovalAllowlist`, `TestSubmitApprovalDecisionRemembersExecCommand`, `TestRequestApprovalAutoApprovesRememberedCommand`.

## 10. Follow-Up Document Updates

- none — additive feature; no contract/schema break. New HTTP endpoints are additive.
