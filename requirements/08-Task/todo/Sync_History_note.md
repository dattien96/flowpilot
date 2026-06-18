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

## Task 072

Automated runner-level signatures: all done (2026-06-18).
TS-008: DONE - TestClaudeAdapterPersistsRealSessionIDForResume (claude_adapter_test.go)
TS-011: DONE - TestResumeRunReconstructsChatRunSeedsChatStep (cross_account_resume_test.go)
TS-013: DONE - TestResumeRunMissingPersistedSessionReturnsRunNotFound (cross_account_resume_test.go)
TS-015: DONE - TestResolveAccountHomeExplicitAccount (cross_account_resume_test.go)
TS-016: DONE - TestResolveAccountHomeDefaultAccountFallback (cross_account_resume_test.go)
TS-017: DONE - TestResolveAccountHomeMissingAccount (cross_account_resume_test.go)
TS-019: DONE - TestLocateSessionFileCodexMissing (cross_account_resume_test.go)
TS-020: DONE - TestLocateSessionFileClaudeFindsProjectSession (cross_account_resume_test.go)
TS-029: DONE - TestRestoredClaudeRunSeedsPoolWithRealSessionBeforeTurn (claude_adapter_test.go)
TS-030-031: Claude cross-account env / opt-in e2e - blocked (need real Claude auth or 2nd account)
TS-037-038: opt-in real-provider Codex e2e - intentionally excluded from automated suite (keep go test token-free)
TS-042-043: Navigator component-level test harness not present in repo; state-level coverage via store.test.ts accepted
TS-044-048: covered as `scripts/quicktest.ps1` canaries + Settings Check Version UI; no Pester quicktest.tests.ps1 harness

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


## Task-075: Cross-Account And Cross-PC Chat E2E Test Guide
Some cases not tested yet

# 6. Blocked / not-resolvable categories (audited 2026-06-18)

The remaining not-done items fall into 4 buckets:

1. Manual provider / E2E - will test manually (see §9 checklist)
   - 071: DOD-43, 52, 53, 88->93
   - 073: DOD-087->093
   - 069: DOD-069-009
   - 074: TS-040->046
2. GitNexus MCP gate - WAIVED (MCP tools not exposed, no CLI `detect-changes` equivalent)
   - 071: DOD-95 ; 073: DOD-095, 096   [marked done/waived 2026-06-18]
3. Feature not built - transcript view (see §10)
   - 067: DOD-067-010 DONE (2026-06-18)
4. Remaining deferred test signatures - see §5 Task 072 for per-item reasons
   - 072: TS-030-031,037,038,042,043,044-048  (TS-008/011/013/015-017/019-020/029 now DONE)

# 7. Claude provider DOD status (only 1 Claude account available)

What "we have not done DOD for Claude" actually means, grouped by the real blocker:

A. Blocked by "only 1 Claude account" - cross-account needs 2 accounts - CANNOT test now
   - 071: DOD-44 (Claude cross-account behavior)
   - 068: DOD-068-010 (Claude cross-account feasibility validated e2e)
   - 072: TS-030 (Claude resume env uses active account home - cross-account)
B. Testable NOW with 1 account - same-account post-restart resume (no 2nd account needed)
   - 071: DOD-43 (Claude same-account post-restart resume follow-up)
   - 072: TS-031 (Claude same-account post-restart e2e, opt-in env flag)
C. Mock-based, NO real account needed - DONE (implemented 2026-06-18)
   - 072: TS-008 DONE (Claude real session id persisted, fake stream)
   - 072: TS-029 DONE (restored Claude run seeds real session before turn, fake)
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

# 9. Manual testing queue (2026-06-18)

Items the user will test manually. Mark each [ ] -> [x] when verified, note pass/fail/provider-untested.

## 9.1 Cross-account resume (Task-071)

- [ ] DOD-43: Claude same-account post-restart resume - start chat, restart app, click history, send follow-up
- [ ] DOD-52: Codex same-account post-restart resume - same flow as DOD-43 but with Codex
- [ ] DOD-53: Codex cross-account resume - copy rollout file under second Codex account home, open history
- [ ] DOD-88: Start chat → restart runner/app → click history item → send follow-up successfully
- [ ] DOD-89: Delete/move provider session file → restart → click history → verify greyout (not crash)
- [ ] DOD-90: Two accounts on one PC → start chat under A → switch to B → click A history item → continue (if portable)
- [ ] DOD-91: Active account not signed in → click history item → verify signed-out reason shown
- [ ] DOD-92: History list order + display fields match BUG-080 behavior after restart
- [ ] DOD-93: No provider session file contents appear in runner logs during above flows

Note: DOD-44 (Claude cross-account) and DOD-068-010 BLOCKED - need 2nd Claude account.

## 9.2 Cross-PC Codex sync/restore (Task-069 + Task-073 + Task-074)

- [ ] DOD-069-009 / DOD-087 / TS-040: PC1 Codex chat - sync to Drive - verify Drive has index + manifest + provider file
- [ ] DOD-088 / TS-041: PC2 (or isolated home) - restore from Drive - verify one local history item created
- [ ] DOD-089 / TS-042: Open restored run on PC2 + send follow-up - verify completes
- [ ] DOD-090 / TS-043: Restore when PC2 cwd differs - verify cwd remap flow
- [ ] DOD-091 / TS-044: Delete remote provider file or local restored file - attempt open - verify greyout with reason
- [ ] DOD-092 / TS-045: Restore into provider home without auth - verify account_not_signed_in reason shown
- [ ] DOD-093 / TS-046: Claude provider status - record as pass / fail / provider-untested

Note: DOD-069-010 BLOCKED - Claude cross-PC needs 2nd machine + 2nd Claude account.

# 10. Transcript view scope (DOD-067-010) - DONE NOW

**Status: BUILT (2026-06-18) — Claude provider.**

What works:
- Resume (sending new messages after restart/cross-account/cross-PC restore) WORKS
- Prior conversation messages ARE streamed back to the desktop when opening a resumed Claude run
- resumeRun calls seedTranscriptFromDisk after ensureResumeReady; rs.events populated before any SSE subscriber connects
- SSE snapshot path (afterSeq=0) replays all rs.events to the desktop — no SSE layer changes needed

What does NOT work yet:
- Codex transcript replay: rollout JSONL format not confirmed for conversation replay; deferred

Implementation:
- transcript_loader.go: loadClaudeTranscriptEvents — reads Claude JSONL, maps each line via mapClaudeLine
- interactive_resume.go: seedTranscriptFromDisk — resolves account home, locates session file, stamps correlation fields (Seq, ID, WorkflowRunID, StepRunID, ProviderSessionID, ProviderKey, OccurredAt), appends to rs.events under s.mu
- interactive_handlers.go: resumeRun — one line added after ensureResumeReady