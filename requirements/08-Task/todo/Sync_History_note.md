# Summary

This doc is the entry for all detail tasks for sync chat accross accounts and pcs

# Previous bugs

Some previous Fix related to History save in local only
BUG-060
BUG-080

Current behavior is: app can save and show history chat - but can not click to open it after restart server, just open when in 1 current lifecycle memory

Basically we save data about the chat in sessions.ndjson
-> based on that we found exactly the chat history in codex or claude in the current pc local directory where codex/claude save that chat data

# New plan tasks

## Task-067: Desktop Post-Restart Run Resume Via Provider Session ID

This is core doc

## Task-068: Desktop History Unified View; Account ID As Local-File Pointer

This one show us how can we save chat for sync accross accounts

## Task-069: Cross-PC Sync for Non-Supabase Users (sessions.ndjson + Provider Files)

This one show us how can we save chat for sync accross PCs

## Task-059: Desktop Check Version Tested Baseline Config

This doc based on 1 already feature, that is call 1 script to verify that the current codex/claude version
can break our system or not. Because they can change the api or the way we interact with it

COme to this feature, we base on 1 thing
COPY the history that to other pc/other acc
Test with latest codex it worked -> so we need same test to make sure in other pc with other ver
it can work too, or at least notify us to change code if the policy from codex/claude changed

## 09 — Implementation Guide: Cross-Account Chat Resume (Codex + Claude)

This is detail code guide we need follow

## Testcase

Task 072

## DOD item checklist

Task 071

## The complete, linked doc set for this feature

| Doc                                                                                                          | Role                                                     | State   |
| ------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------- | ------- |
| [CA-098](change-audit/CA-098-spike-provider-session-portability.md)                                          | Proven portability + reusable test checklist             | done    |
| [09-IG](requirements/10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md)            | **Codex-ready implementation guide** (phases A–E + test) | ready   |
| [Task-067](requirements/08-Task/todo/Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md)    | Post-restart resume foundation                           | todo    |
| [Task-068](requirements/08-Task/todo/Task-068-Desktop-History-Unified-View-Account-As-Local-File-Pointer.md) | Unified history + account pointer + cross-account        | todo    |
| [Task-069](requirements/08-Task/todo/Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md)                 | Cross-PC sync (deferred)                                 | todo    |
| [Task-070](requirements/08-Task/done/Task-070-History-Supabase-Reader-Production-Fix.md)                     | Supabase reader (renumbered from dup 056)                | done    |
| [Task-059](requirements/08-Task/done/Task-059-Desktop-Check-Version-Tested-Baseline-Config.md)               | Compat-test owner → now points at portability canaries   | done    |
| [08-Desktop-Chat-New-Plan.md](requirements/10-Refactor/New-System/08-Desktop-Chat-New-Plan.md)               | Plan index → lists both resume + cross-PC                | updated |
