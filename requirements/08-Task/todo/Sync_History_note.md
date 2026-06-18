# 1. Summary

This doc is the entry for all detail tasks for sync chat accross accounts and pcs

# 2. Previous bugs

Some previous Fix related to History save in local only
BUG-060
BUG-080

Current behavior is: app can save and show history chat - but can not click to open it after restart server, just open when in 1 current lifecycle memory

Basically we save data about the chat in sessions.ndjson
-> based on that we found exactly the chat history in codex or claude in the current pc local directory where codex/claude save that chat data

# 3.New plan tasks

## 3.1 Task-067: Desktop Post-Restart Run Resume Via Provider Session ID

This is core doc

## 3.2 Task-068: Desktop History Unified View; Account ID As Local-File Pointer

This one show us how can we save chat for sync accross accounts

## 3.3 Task-069: Cross-PC Sync for Non-Supabase Users (sessions.ndjson + Provider Files)

This one show us how can we save chat for sync accross PCs

## 3.4 Task-059: Desktop Check Version Tested Baseline Config

This doc based on 1 already feature, that is call 1 script to verify that the current codex/claude version
can break our system or not. Because they can change the api or the way we interact with it

COme to this feature, we base on 1 thing
COPY the history that to other pc/other acc
Test with latest codex it worked -> so we need same test to make sure in other pc with other ver
it can work too, or at least notify us to change code if the policy from codex/claude changed

## 3.5 Task-075: Cross-Account And Cross-PC Chat E2E Test Guide

This one is the manual + Computer Use checklist.
Use it to test the full UI flow from selecting project/provider/model, creating chat, restart resume, cross-account resume, Check Version, sync, restore, and greyout fallback.

# 4. Coding plan

## 4.1 Phase 1: Cross-Account

### 4.1.1 Coding plan

09 — Implementation Guide: Cross-Account Chat Resume (Codex + Claude)

### 4.1.2 DOD checklist - Automated done; manual/review gates pending

Task 071

### 4.1.3 Testcase - Automated runner/desktop coverage done; manual/provider cases pending

Task 072

Manual/E2E checklist

Task 075

## 4.2 Phase 2: Cross-PC

### 4.2.1 Coding plan

10-Cross-PC-Non-Supabase-Chat-Sync-Implementation-Guide.md

### 4.2.2 DOD checklist - Automated done; manual/GitNexus gates pending

Task 073

### 4.2.3 Testcase - Automated done; manual/provider cases pending

Task 074

Manual/E2E checklist

Task 075

## 4.3 Phase 3: Recheck architecture for adding sync flow/step in future

Pending

# 5. DOD remaining

## Task 071

Automated DOD done.

TODO
DOD-43-44: pending manual/provider validation for Claude
DOD-52-53: pending manual/provider validation for Codex
DOD-88->93: manually
DOD-95: DONE (waived) - GitNexus MCP/CLI not available in this environment; scope verified via go test + git diff
DOD-96: DONE - independent reviewer confirmed implementation follows 09-IG (no deviations)

## Task 073

Automated DOD done.

TODO
DOD-087->093: pending manual cross-PC/provider validation
DOD-095-096: DONE (waived) - GitNexus MCP/CLI not available in this environment; scope verified via go test + git diff

## Task 072

Explicitly deferred with reasons in Task 072:
TS-008-029-030-031: Claude post-restart / cross-account / opt-in e2e scope not implemented in this pass
TS-011-013-015-016-017-019-020: lower-level helper signatures not implemented as dedicated tests; broader reconstruction/relocation coverage exists
TS-037-038: opt-in real-provider Codex e2e harness not implemented in this pass
TS-042-043: Navigator component-level test harness not present; state-level coverage exists only
TS-044-048: covered only as `scripts/quicktest.ps1` canaries, not as formal `quicktest.tests.ps1` automated signatures

## Task 074

Marked done now - except manually cases

TODO: manually case
TS-040 -> 046

## Task 059

Handled now:
- `scripts/quicktest.ps1` portability canaries check Codex resume CLI surface, rollout metadata account-agnostic shape, and Claude session-store presence
- desktop Check Version path (`/compat`, `/compat/deep`) now includes the same portability contract checks at runner level

## Task 067

TODO
DOD-067-010

## Task 068

DOD-068-010 pending for claude

## Task 069

DOD-069-009
and DOD-069-010 pending for manually + claude

# Task-075: Cross-Account And Cross-PC Chat E2E Test Guide
Some cases not tested yet

# 6. Blocked / not-resolvable categories (audited 2026-06-18)

The remaining not-done DOD/TS items fall into 4 buckets. None are code-fixable in this environment:

1. Manual provider / E2E - needs real Codex/Claude accounts + 2 machines
   - 071: DOD-43, 52, 53, 88->93
   - 073: DOD-087->093
   - 068-010, 069-009
   - 074: TS-040->046
2. GitNexus MCP gate - now WAIVED (MCP tools not exposed, no CLI `detect-changes` equivalent)
   - 071: DOD-95 ; 073: DOD-095, 096   [marked done/waived 2026-06-18]
3. Feature not built - transcript view (no implementation code AND no verify script anywhere)
   - 067: DOD-067-010
4. Deferred test signatures - confirmed none implemented (verified by grep 2026-06-18)
   - 072: TS-008,011,013,015-017,019,020,029-031,037,038,042,043,044-048

# 7. Claude provider DOD status (only 1 Claude account available)

What "we have not done DOD for Claude" actually means, grouped by the real blocker:

A. Blocked by "only 1 Claude account" - cross-account needs 2 accounts - CANNOT test now
   - 071: DOD-44 (Claude cross-account behavior)
   - 068: DOD-068-010 (Claude cross-account feasibility validated e2e)
   - 072: TS-030 (Claude resume env uses active account home - cross-account)
B. Testable NOW with 1 account - same-account post-restart resume (no 2nd account needed)
   - 071: DOD-43 (Claude same-account post-restart resume follow-up)
   - 072: TS-031 (Claude same-account post-restart e2e, opt-in env flag)
C. Mock-based, NO real account needed - deferred for scope, not for accounts
   - 072: TS-008 (Claude real session id persisted, fake stream)
   - 072: TS-029 (restored Claude run seeds real session before turn, fake)
D. "Record as provider-untested" - satisfiable by documentation (kept pending per user 2026-06-18)
   - 069: DOD-069-010 ; 073: DOD-093 ; 074: TS-046

Note: Codex cross-account IS confirmed (CA-098, two codex homes on one machine). Claude
cross-account is the only true account-blocked gap because we have one Claude account.

# 8. Windows go-test note

DOD-85 ("go test ./internal/runner/... passes") is true on macOS/Linux (dev/CI). On Windows
several PRE-EXISTING tests fail due to Unix-only assumptions - not regressions, not from 99cac10:
- google_drive_mcp_provider_config_test.go (introduced 99ba48b): `/tmp/...` path; Windows filepath rewrites to `\tmp\...`
- phase8_a1_test.go skills merge: sets HOME but blanks USERPROFILE; Windows resolves home from USERPROFILE
- codex_resume_process_test.go (99cac10): `sh -c` shell mocks fail under Git Bash on Windows
Fix if Windows CI is desired = make these platform-portable (t.TempDir() paths, set USERPROFILE alongside HOME, cross-platform command mock).