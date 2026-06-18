---
name: BUG-084-Codex-History-Chat-Open-Fails-After-Switching-Active-Account
description: A Codex history chat created under active account A can be opened after restart, but fails after switching the active Codex account to B. Same-provider accounts should be able to open/cross-resume the same chat.
metadata:
  type: bugfix
---

## Metadata

- Document ID: `BUG-084`
- Title: Codex History Chat Open Fails After Switching Active Account
- Phase: `bugfix`
- Status: `done`
- Owner: DatNguyen
- Reviewers: —
- Created: 2026-06-18
- Last Updated: 2026-06-18
- Parent Documents: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: —
- Related Documents: [Task-067: Desktop Post-Restart Run Resume Via Provider Session ID](../../08-Task/todo/Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md), [BUG-082: Desktop History Chat Open Fails On Legacy Default Account](../done/BUG-082-Desktop-History-Chat-Open-Fails-On-Legacy-Default-Account.md), [BUG-083: Desktop Chat Resume Replays Composed Prompt, Not User Input](../done/BUG-083-Desktop-Chat-Resume-Replays-Composed-Prompt-Not-User-Input.md)
- Replaces: —
- Tags: desktop, history, resume, codex, provider-accounts, cross-account, local-runner, severity-high

## AI Quick View

### Summary

- User flow: active Codex account A -> create/open chat -> restart server -> history chat opens -> switch active Codex account to B -> same history chat cannot open.
- Expected behavior: any connected account under the same provider should be able to open/cross-resume a chat by preparing the provider session file on the active account.
- Current code supports first-time cross-account relocation, but treats any existing destination session file as `session_unavailable`.
- This blocks valid same-provider account switching when the target account already has the same chat session file, for example after a previous cross-account open, sync restore, or account toggle.

### Current Ask

- Fix implemented in `apps/local-runner/internal/runner/` and verified with runner tests.
- Same-provider Codex account switching is now idempotent when the destination already contains the same session file.

### Key Decisions

- `V-1` Opening a Codex chat from history must work after switching between connected Codex accounts A and B.
- `V-2` Repeated A -> B -> A account switching must be idempotent when the destination already contains the same provider session file.
- `V-3` Existing overwrite protection must remain for destination files with different content or incompatible session metadata.

### Constraints

- Do not weaken session-file overwrite safety; accepting an existing destination file must be limited to same/identical or otherwise validated session files.
- Keep provider-account scoping per provider; a Claude active account must not affect Codex history open.
- No implementation in this BUG-084 note yet; this file records diagnosis and expected correction path only.

### Open Questions

- `Q-1` Follow-up: do we want the same identical-destination acceptance for Claude cross-account resume too, or keep BUG-084 scoped to the confirmed Codex path?
- `Q-2` Follow-up: should BUG-083 turn-log sidecar files eventually participate in cross-account transcript portability, or is stable resume-file portability sufficient for now?

### Source Refs

- User report on 2026-06-18: "active codex acc A; Chat; Restart server -> can open chat; change active to B; can not open chat anymore; make sure all acc in same provider can work cross chat."
- `apps/local-runner/internal/runner/interactive_resume.go:184` -> `ensureResumeReady`
- `apps/local-runner/internal/runner/interactive_resume.go:214` -> `prepareCrossAccountResume`
- `apps/local-runner/internal/runner/session_file_locator.go:61` -> `RelocateSessionFile`
- `apps/local-runner/internal/runner/session_file_locator.go:78` -> destination-exists guard returns `destination session file already exists`
- `apps/local-runner/internal/runner/session_file_locator.go` -> identical destination file now returns success instead of conflict
- `apps/local-runner/internal/runner/cross_account_resume_test.go:573` -> first-time cross-account relocation succeeds
- `apps/local-runner/internal/runner/cross_account_resume_test.go:629` -> existing destination currently surfaces `session_unavailable`
- `apps/local-runner/internal/runner/cross_account_resume_test.go` -> added identical-destination and A -> B -> A switching tests

## 1. Issue Summary

Codex chat history can be opened after a runner/server restart while the original account A remains active. After switching the active Codex account to B, opening the same history chat fails. The user expectation is that all connected accounts under the same provider can cross-open the same chat.

This is a cross-account resume/idempotency bug, not a transcript rendering bug.

## 2. Parent Links

- impacted coding plan: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- impacted tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- related task: [Task-067](../../08-Task/todo/Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md)

## 3. Environment and Reproduction

- environment: desktop FlowPilot, local runner, Codex provider, at least two connected Codex accounts A and B.
- reproduction steps:
  1. Activate Codex account A.
  2. Create a chat and complete at least one turn.
  3. Restart the server/runner so the chat is reconstructed from local history.
  4. Open the history chat while account A is active; it opens.
  5. Activate Codex account B.
  6. Open the same history chat again.
- frequency: reported as reproducible by user; code inspection shows a deterministic failure path when the target account already has the destination session file.

## 4. Expected vs Actual

- expected: history chat opens from any connected Codex account. If the session file is already present in the active account home and represents the same chat, cross-account resume should treat it as prepared and rebind the stored `providerAccountID`.
- actual before fix: `prepareCrossAccountResume` mapped the relocation error to `session_unavailable`, the desktop marked the history row unavailable, and the chat did not open.
- actual after fix: existing identical destination files are treated as already prepared; the run rebinds to the active account and opens successfully.

## 5. Impact

- users affected: users with multiple connected Codex accounts who switch the active account after creating/restoring chats.
- workflows affected: history review, continued chat, cross-account usage, account rotation.
- severity: High — the chat exists locally, but history open is blocked by an idempotency failure in session preparation.

## 6. Root Cause

- hypothesis: `RelocateSessionFile` treats a valid pre-existing destination session file as a conflict, even when the file is identical to the source and already prepared for the active account.
- confirmed code cause: `prepareCrossAccountResume` calls `RelocateSessionFile`; for distinct account homes, the old `RelocateSessionFile` implementation returned `destination session file already exists` for any existing destination file. The caller mapped that to `session_unavailable`.
- evidence:
  - `TestPrepareCrossAccountResumeRelocatesAndRepointsRun` covers the first copy into account B and passes.
  - `TestPrepareCrossAccountResumeRelocationFailureKeepsHistoryVisible` deliberately creates an existing destination file and asserts `session_unavailable`; it does not distinguish identical safe files from conflicting files.
  - BUG-082 added a no-op only when `srcPath == dstPath` for same-home accounts. It does not cover distinct homes with an already-identical destination file.
  - New fix confirms the narrower rule: identical existing destination files are safe to reuse; different destination content still fails.

## 7. Fix Strategy

- `F-1` Implemented: `RelocateSessionFile` now compares source and destination contents when the destination already exists. If contents are identical, it returns success and reuses the destination path.
- `F-2` Implemented: overwrite protection remains unchanged for different destination content; the function still returns `destination session file already exists`.
- `F-3` Implemented: added tests for identical destination reuse and A -> B -> A switching across active Codex accounts.
- `F-4` Deferred: BUG-083 sidecar portability is not changed by this fix.

## 8. Validation

- `V-1` ✅ `TestRelocateSessionFileAcceptsExistingIdenticalSessionFile` — identical destination file is accepted and reused.
- `V-2` ✅ `TestRelocateSessionFileDoesNotOverwriteExistingSessionFile` — different destination content still fails.
- `V-3` ✅ `TestPrepareCrossAccountResumeAcceptsExistingIdenticalTargetFile` — `resumeRun` succeeds when switching Codex account A -> B and the destination already contains the same session file.
- `V-4` ✅ `TestPrepareCrossAccountResumeSupportsSwitchingBackAndForth` — repeated A -> B -> A switching succeeds and rebinds the stored `ProviderAccountID`.
- `V-5` ✅ Focused cross-account suite passed.
- `V-6` ✅ `go test ./internal/runner -count=1` passed: 627 tests.

## 9. Regression Guard

- tests: `cross_account_resume_test.go` now covers identical-destination reuse, overwrite-conflict retention, and repeated same-provider account switching.
- alerts: none.
- audit checks: GitNexus impact analysis for `RelocateSessionFile` and `prepareCrossAccountResume` was LOW before editing.

## 10. Follow-Up Document Updates

- upstream docs that must change: Task-067 likely should state that same-provider account switching must be idempotent across all connected accounts.
- notes left unchanged on purpose: this fix intentionally uses identical-content acceptance only; it does not broaden cross-account relocation to mismatched files or alter remote restore conflict rules.
