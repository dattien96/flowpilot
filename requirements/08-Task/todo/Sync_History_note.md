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

# 4. Coding plan

## 4.1 Phase 1: Cross-Account

### 4.1.1 Coding plan

09 — Implementation Guide: Cross-Account Chat Resume (Codex + Claude)

### 4.1.2 DOD checklist - Automated done; manual/review gates pending

Task 071

### 4.1.3 Testcase - Automated runner/desktop coverage done; manual/provider cases pending

Task 072

## 4.2 Phase 2: Cross-PC

### 4.2.1 Coding plan

10-Cross-PC-Non-Supabase-Chat-Sync-Implementation-Guide.md

### 4.2.2 DOD checklist - Automated done; manual/GitNexus gates pending

Task 073

### 4.2.3 Testcase - Automated done; manual/provider cases pending

Task 074

## 4.3 Phase 3: Recheck architecture for adding sync flow/step in future

Pending

# 5. DOD remaining

## Task 071

Automated DOD done.

TODO
DOD-43-44: pending manual/provider validation for Claude
DOD-52-53: pending manual/provider validation for Codex
DOD-88->93: manually
DOD-95: GitNexus detect unavailable in this session
DOD-96: independent reviewer agent timed out

## Task 073

Automated DOD done.

TODO
DOD-087->093: pending manual cross-PC/provider validation
DOD-095-096: GitNexus tools unavailable in this session

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
