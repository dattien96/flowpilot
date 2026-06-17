# CHAT MODE

## 1. History

See Sync_History_note.md

## 2. YOLO - PASSED

### 2.1.1 Codex

OK for both MCP + Command tool

### 2.1.2 CLAUDE

OK for both MCP + Command tool

## 3. model.skill.reason - PASSED

### 3.1 Codex

ok

### 3.2 Claude

ok

## 4 Image attached - PASSED

Done in Task-052

### Has bug

------------------------- later prompt can not see image attached in previous prompt

## 5. User Interaction (Structured Questions) - PASSED

Suggestion FORM show to user
Separate from approvals, FlowPilot can ask the user a structured question (confirm/options popup) through the same pause/resume bridge

- ask_user MCP tool — a FlowPilot-registered custom tool (not built-in) the model discovers via tools/list and may call → best-effort, can not make sure it asked users
- Workflow-driven — the ported Go state machine emits user_question_required directly at a defined step → deterministic (use for required asks).

Other input was not worked - Passed now

## 6. Show token/context - PASSED

Check for codex -> OK
Check for claude -> OK

Recorded in [Task-061: Desktop Chat Token Usage And Context Window](../../08-Task/done/Task-061-Desktop-Chat-Token-Usage-And-Context-Window.md)

Whenever once AI return response
-> Check current provider
-> See is it configured to visible in account active right bar or not
-> if yes, Check the current active acc of this provider
-> reload IT ONLY to refresh the token on acc UI

## 7. How to check change in feature of codex/claude - PASSED

Retest with codex/claude latest
Task 059- DONE

If affected to our app cause we call its functions

## 8. Ui of provider with icon - PASSED

update ui of provider selection
new rule: can not CHANGED provider after run started

## 9. Feature cancel current running chat turn - PASSED

## 10. Load long chat - PASSED

Task 062

## 11. skill attached - AI k doc dc ? sai folder? - PASSED

Via Bug 077 + Task 065

## 12. Sync artiact for chat mode first

Not implemented

# Flow mode - Pending

model
skill
agen-multi auto interact with each other
reasong
yolo
user question list
MCP call
image attach
