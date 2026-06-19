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


## Task 059

Handled now:
- `scripts/quicktest.ps1` portability canaries check Codex resume CLI surface, rollout metadata account-agnostic shape, and Claude session-store presence
- desktop Check Version path (`/compat`, `/compat/deep`) now includes the same portability contract checks at runner level

## Task 067

DOD-067-010 - Transcript view (stream prior messages to desktop): DONE (implemented 2026-06-18)
- transcript_loader.go: loadClaudeTranscriptEvents(filePath) reads Claude JSONL, calls mapClaudeLine per line
- interactive_resume.go: seedTranscriptFromDisk(rs) loads+stamps correlation fields, populates rs.events under s.mu
- interactive_handlers.go: resumeRun calls seedTranscriptFromDisk after ensureResumeReady
- SSE snapshot path replays rs.events to desktop (already in place — no SSE changes needed)
- Codex support deferred (rollout JSONL replay format unconfirmed)

# 5. Blocked / not-resolvable categories (audited 2026-06-18)
Codex
- Cross Acc: **Passed**
- Cross re-start: **Passed**
- Cross PC: waiting

Claude
- Cross Acc: CAN NOT TEST cause only 1 acc
- Cross re-start: **Passed**
- Cross PC: waiting

PENDING ITEM:
   - 071: 44, 89, 91
   - 073: DOD-091->093
   - 069: DOD-069-010
   - 074: TS-044->046
   - 072: TS-029-030-031,043,044-048

Case mem local + sync ok
Nhuwng test lai case may khac restore + open

Edge case not test: 
F-1 Active account home missing

result: account_unavailable
F-2 Active account not signed in

result: account_not_signed_in
F-3 Stable rollout file missing in source home

result: session_unavailable
F-4 Destination session file already exists with different bytes

result: relocation fails; FlowPilot refuses overwrite
F-5 Source and destination are the same physical file

result: relocation succeeds as a no-op; FlowPilot still rebinds provider_account_id
F-6 Old runs without turn-log sidecar

result: resume still works from the stable stored session file, but replay may only have single-file coverage