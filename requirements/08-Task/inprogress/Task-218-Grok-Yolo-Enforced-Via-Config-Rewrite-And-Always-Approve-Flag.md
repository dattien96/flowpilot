# Task-218: Grok YOLO Enforced Via Config Rewrite And Always-Approve Flag

## Metadata

- Document ID: `Task-218`
- Title: `Grok YOLO Enforced Via Config Rewrite And Always-Approve Flag`
- Phase: `task`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-10`
- Last Updated: `2026-07-10`
- Parent Documents: [Task-208: Grok Permission Channel And YOLO Posture](./Task-208-Grok-Permission-Channel-And-Yolo-Posture.md), [SS-08: Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md), [SD-09: Approval Gates](../../06-System-Tech-Design/SD-09-Approval-Gates.md)
- Child Documents: `None`
- Related Documents: `None`
- Replaces: `None`
- Tags: `grok, yolo, approval, permission, config-toml, local-runner, desktop`

## AI Quick View

### Summary

- Task-208 built the `session/request_permission` wiring and discovered (DOD-5, live-verified) that Grok has no CLI flag equivalent to Codex's `-c approval_policy=...` or Claude's `--permission-mode` for the "force gating back on" direction — an account whose own `config.toml` has `[ui] permission_mode="always-approve"` makes Grok skip the permission channel entirely, and FlowPilot could only log a warning about it, not fix it.
- This task closes that specific gap two ways: (1) YOLO=true now actually reaches the spawned `grok agent` process as `--always-approve` (previously `GrokPermissionMode` was computed but never read — dead code); (2) YOLO=false rewrites the active account's own `config.toml` `permission_mode` value away from `"always-approve"` and forces a process respawn, since there is no flag for that direction.
- **Enforcement is automatic, not toggle-gated (revised 2026-07-10):** the YOLO=false config rewrite fires inside `ensureGrokProcess` on **every launch** where a process spawns under YOLO=false and the account's config currently bypasses — so gating holds even if the user never touches the desktop toggle (SS-08: YOLO is the SSOT and must win by default). The desktop toggle / `ApplyGrokYoloPosture` endpoint is now the *eager* path (apply-now + loading modal), not the *only* path.
- Because the YOLO=false apply is a real file mutation + process respawn (not an instant local toggle), the desktop's Grok-only YOLO switch is asynchronous and shows a loading modal while it applies — Claude/Codex/Gemini's YOLO toggle is untouched and stays fully synchronous.

### Current Ask

- Implement `ensureGrokProcess`'s `--always-approve` launch flag, `setGrokConfigPermissionMode`'s config.toml rewrite, `Runner.ApplyGrokYoloPosture`, the `POST /provider-accounts/grok-yolo-posture` endpoint, and the desktop's async Grok-only toggle + loading modal. Document why this mechanism is structurally different from Claude/Codex so a future reader does not assume parity.

### Key Decisions

- `T-1` Store the desired Grok posture (`grokDesiredAlwaysApprove`) directly on `Runner`, set explicitly by `ApplyGrokYoloPosture`, rather than threading `yolo` through the shared `ProviderRegistration.newAdapterForTurn`/`Adapter()` signature — Claude/Codex/Gemini registrations never change, never have to accept-and-ignore a parameter that isn't theirs.
- `T-2` `config.toml` is rewritten with a minimal line-based patch of exactly the `[ui] permission_mode` key (mirroring the existing `grokConfigPermissionModeBypassesGating` reader's own line-scanning approach), not a full TOML parse/re-marshal — every other line, key, comment, and section ordering is preserved byte-for-byte. Written atomically (temp file + `os.Rename`), mirroring `local_file_session_store.go`'s own sessions.ndjson rewrite.
- `T-3` YOLO=true never touches `config.toml` — `--always-approve` bypasses regardless of whatever the file says, so there is nothing to gain from also rewriting it (and one fewer chance to race a concurrently open grok TUI).
- `T-4` A YOLO flip forces `ensureGrokProcess` to respawn even on an otherwise-matching scope/model/effort — the same way a model or reasoning-effort change already does — because a live process only has whatever posture flag it was launched with.
- `T-6` **Auto-enforcement on launch (revised):** `ensureGrokProcess` itself rewrites an always-approve config to `"default"` whenever it spawns under YOLO=false, before `grok` reads the file at startup — so enforcement does not depend on the user ever flipping the desktop toggle. Deliberately conservative: it only rewrites when `grokConfigPermissionModeBypassesGating` reports the value actually bypasses (always-approve), leaving a default/absent/other config untouched, and is a no-op when already correct. Accepted tradeoff (SS-08 justifies it): a user who deliberately runs the standalone `grok` TUI in always-approve will have that flipped to default the first time a FlowPilot YOLO=false turn spawns for that account — YOLO=false is the safe default and is designed to win. A cleaner, non-clobbering alternative (a `--agent-profile` permission override, if Grok supports one) was not investigated and is a possible future refinement.
- `T-5` The mechanism is deliberately NOT symmetric with Claude/Codex. Those two providers enforce YOLO purely through a CLI flag/config override passed on every launch (`codex exec resume -c sandbox_mode=... -c approval_policy=...`, `claude --permission-mode ...`) — no file on disk is ever touched, no process is torn down just because YOLO changed. Grok's CLI (`grok agent stdio`, confirmed via `--help`) only exposes `--always-approve`, a one-directional flag with no counterpart to force gating back on — this was live-verified in Task-208 DOD-5 by toggling the account's real `config.toml` directly, not by finding a flag. So Grok's YOLO=false direction has exactly one lever: rewrite the account's own file. This is why (and only why) Grok's desktop toggle is async with a loading modal while Claude/Codex/Gemini's stays a synchronous local state flip.

### Constraints

- Do not change `ProviderRegistration`, `ProviderRegistry.Adapter()`, or any Claude/Codex/Gemini registration branch — additive to Grok-specific files only (mirrors Task-208's own `P-0` base-regression constraint).
- Never touch `config.toml` for `yolo=true`.
- Never do a full TOML re-marshal — line-based patch of the one key only.
- The desktop loading modal must be non-dismissible while in flight (nothing to cancel — the backend call is already running) and must disable the toggle control to prevent a double-fire.

### Open Questions

- `Q-1` Whether `--always-approve` is purely a launch-scoped flag or also persists back into `config.toml` itself (mirroring Grok's own `/always-approve` slash command) has not been live-verified against the real `grok` binary in this pass — only unit-tested against a scripted fake process. If it turns out to also write the file, `setGrokConfigPermissionMode`'s "no-op when already correct" check still protects against redundant writes, but the DOD-9 live pass below must confirm this either way.
- `Q-2` Two concurrent runs on the SAME Grok account with different desired YOLO values will thrash (`grokProcess` is one shared OS process per account, keyed by `scopeKey == account.ID`) — this is the same pre-existing tradeoff a concurrent model/reasoningEffort mismatch already has (see `ensureGrokProcess`'s own doc comment) and is not solved by this task.

### Source Refs

- `Task-208` DOD-5 (live-verified finding: no flag exists for the gate-back-on direction), `SS-08` (YOLO is the SSOT and must override everything), `SD-09`.
- `apps/local-runner/internal/runner/yolo_resolver.go`, `grok_process.go` (`ensureGrokProcess`, `grokConfigPermissionModeBypassesGating`, new `setGrokConfigPermissionMode`, new `ApplyGrokYoloPosture`), `provider_registry.go` (Grok registration branch), `runner.go` (`Runner` struct), `internal/cli/root.go` (new endpoint).
- `apps/desktop-flowpilot/src/state/store.ts` (`confirmAccountSwitch`/`accountSwitchLoading` is the async-action-with-modal pattern mirrored here), `ChatWorkspace.tsx` (`AccountSwitchModal`), `ChatInput.tsx` (YOLO toggle), `client/HttpWsRunnerClient.ts`, `types/contract.ts`.

## 1. Goal

Make Grok's YOLO=false posture actually block a gated action end-to-end even when the active account's own `config.toml` persists `permission_mode="always-approve"`, and make YOLO=true reach the spawned process directly instead of relying solely on the runner-side bridge auto-answering whatever permission requests happen to arrive — while keeping Claude/Codex/Gemini's existing YOLO enforcement completely unchanged.

## 2. Parent Links

- coding plan: `CP-46` (Grok Build Controlled Adapter Over ACP), via Task-208
- tech design: `SD-09`
- system spec: `SS-08`
- specific upstream ids: `Task-208 DOD-5`, `SS-08` YOLO SSOT rule

## 3. Trigger

Live testing after Task-208/CP-46 landed showed Grok writing a file under YOLO=false when the account's own `config.toml` had `permission_mode="always-approve"` — FlowPilot's runner-side gate had nothing to intercept because Grok itself never sent `session/request_permission`. Task-208 diagnosed this exact condition (`grokConfigPermissionModeBypassesGating`, DOD-5) but only added a diagnostic warning log, explicitly leaving `~/.grok/config.toml` authoring out of scope. This task picks that up as its own scoped slice.

## 4. Exact Change

- `T-1` Add `alwaysApprove bool` to `grokProcessHandle` and a 7th `alwaysApprove bool` parameter to `ensureGrokProcess`; widen its reuse check to also require a matching `alwaysApprove`; append `--always-approve` to the launch args when true.
- `T-2` Add `grokDesiredAlwaysApprove bool` to `Runner` (guarded by the existing `grokProcessMu`); the Grok registration branch in `provider_registry.go` reads it and passes it into `ensureGrokProcess` on every turn.
- `T-3` Add `setGrokConfigPermissionMode(grokHome, desiredMode string) (changed bool, err error)` — line-based `[ui] permission_mode` patch, insert-if-missing, append-section-if-missing, create-file-if-missing, no-op if already correct, atomic write.
- `T-4` Add `(r *Runner) ApplyGrokYoloPosture(ctx, yolo bool) error`: resolves the active Grok account, rewrites `config.toml` only when `yolo == false`, sets `grokDesiredAlwaysApprove`, and force-closes any live `grokProcess` handle so the next turn respawns under the new posture.
- `T-5` Add `POST /provider-accounts/grok-yolo-posture` (`internal/cli/root.go`), body `{"yolo": bool}`, calling `ApplyGrokYoloPosture`.
- `T-6` Add `applyGrokYoloPosture` to `RunnerClient` (`contract.ts`), `HttpWsRunnerClient.ts`, `MockRunnerClient.ts`.
- `T-7` Add `grokYoloPostureLoading` state and `toggleYoloForActiveProvider` action to `store.ts` (mirrors `confirmAccountSwitch`/`accountSwitchLoading`): synchronous local flip for every provider except Grok; for Grok, awaits `client.applyGrokYoloPosture`, only commits `yoloMode` on success, pushes a timeline error on failure.
- `T-8` Wire `ChatInput.tsx`'s YOLO toggle to `toggleYoloForActiveProvider`, disabled while `grokYoloPostureLoading`; add a `GrokYoloPostureModal` in `ChatWorkspace.tsx` (mirrors `AccountSwitchModal`'s overlay shell, non-dismissible, no cancel action).

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/yolo_resolver.go` (unchanged, referenced only), `grok_process.go`, `provider_registry.go`, `runner.go`, `internal/cli/root.go`; `apps/desktop-flowpilot/src/state/store.ts`, `store.test.ts`, `types/contract.ts`, `client/HttpWsRunnerClient.ts`, `client/MockRunnerClient.ts`, `components/ChatInput.tsx`, `components/ChatWorkspace.tsx`
- modules: local runner Grok process lifecycle, provider-accounts HTTP surface, desktop YOLO toggle UI/state
- routes: `POST /provider-accounts/grok-yolo-posture` (new)
- tables: none (config.toml is a file on the account's home directory, not a DB table)

## 6. Acceptance Check

- Unit: `setGrokConfigPermissionMode` changes the value, inserts the key when the section exists but the key is missing, appends the section when missing entirely, creates the file when missing, and is a true no-op (no write, mtime unchanged) when already correct — while leaving every other key/section untouched.
- Unit: `ensureGrokProcess` appends `--always-approve` to launch args when requested, and a YOLO flip on an otherwise-identical scope/model/effort forces a respawn (new handle, not reused).
- Unit: `ApplyGrokYoloPosture(false)` rewrites `config.toml` and closes/nils the live process; `ApplyGrokYoloPosture(true)` never touches the file; no-resolvable-account returns a typed error.
- Frontend: `toggleYoloForActiveProvider` is a synchronous local flip for non-Grok providers (byte-identical to the old `setYoloMode` behavior) and an awaited, loading-gated call for Grok that only commits `yoloMode` on success.
- `go build ./...` and `go test ./internal/runner/...` pass with no regressions beyond this session's documented pre-existing failure baseline; desktop `tsc --noEmit` and the existing `store.test.ts`/`timelineReducer.test.ts` suite pass with no new failures.
- **Live pass (not yet done — see DOD below):** on a real machine with a Grok account whose `config.toml` has `permission_mode="always-approve"`, toggle YOLO off in the desktop app; confirm the loading modal appears and clears, `config.toml`'s `permission_mode` is rewritten to `"default"` with every other key intact, and a subsequent write tool call is genuinely gated (shows an approval card) instead of silently executing. Also confirm `--always-approve` is accepted by the real `grok agent stdio` invocation alongside `--model`/`--reasoning-effort` (Q-1).

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` `ensureGrokProcess` passes `--always-approve` when requested and stores it on the handle (unit-tested against a scripted fake process).
- [x] `DOD-2` A YOLO flip forces a respawn on an otherwise-matching scope/model/effort (unit-tested).
- [x] `DOD-3` `setGrokConfigPermissionMode` covers change/insert/append-section/create-file/no-op, preserving unrelated content (unit-tested).
- [x] `DOD-4` `ApplyGrokYoloPosture` covers the false/true/no-account paths (unit-tested).
- [x] `DOD-5` Desktop `toggleYoloForActiveProvider` and the loading modal are wired and typecheck-clean; existing test suite green.
- [x] `DOD-5b` **Auto-enforcement (revised ask):** `ensureGrokProcess` rewrites an always-approve config to `"default"` on every YOLO=false spawn, not only on the desktop toggle; leaves non-bypassing configs and YOLO=true untouched. Unit-tested (`TestEnsureGrokProcessAutoEnforcesGatingUnderYoloOff`, three sub-cases).
- [ ] `DOD-6` **Not done.** Live verification against the real `grok` binary that `--always-approve` (a) is accepted positionally alongside `--model`/`--reasoning-effort`/`stdio`, and (b) actually suppresses `session/request_permission` end-to-end for a real write tool call (Task-208's own live-verification discipline — this task has so far only proven the mechanism against a scripted fake process).
- [ ] `DOD-7` **Not done.** Live pass toggling YOLO off in the desktop app against a real account with `permission_mode="always-approve"`, confirming the modal, the file rewrite, and that a subsequent write is genuinely blocked.

## 7. Out of Scope

- Concurrent multi-run YOLO mismatch on the same shared Grok process/account (`Q-2`) — pre-existing tradeoff, not solved here.
- Any change to Claude, Codex, or Gemini's YOLO enforcement — untouched by design.
- A Grok-side `--allow`/`--deny` glob-rule fallback (Task-208 `T-5`) — unrelated lever, not needed once `config.toml` itself can be corrected.

## 8. Completion Notes

- result: Backend + frontend wiring implemented and unit/typecheck verified in this pass; live verification (DOD-6, DOD-7) still outstanding.
- implementation notes: see Key Decisions above for the full rationale on why this differs from Claude/Codex.
- verification: `go test ./internal/runner/...` (full suite, no new regressions beyond the pre-existing baseline); desktop `tsc --noEmit` clean; `store.test.ts`/`timelineReducer.test.ts` green (same pre-existing 3 failures as before this task, unrelated to YOLO/Grok).
- follow-ups: close DOD-6/DOD-7 with a live pass on a real Grok account.
- upstream docs updated: Task-208's Out of Scope note cross-references this task (see Task-208 doc).
