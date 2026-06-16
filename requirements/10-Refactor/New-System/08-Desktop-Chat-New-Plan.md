

# 1. History
page /workflow-runs
co ca vu kill process before timeout setting o page proj

luu history nhu nao
Threads persist as JSONL logs on disk. Resume reloads them. So resume needs both the stored thread.id and the local thread log on the same machine.

Resume requires the stored threadId (Codex) / session_id (Claude) and the session log present on the same machine. It is not bound to the specific account that created it — any active account for that provider can resume, as long as the session log is reachable. Different machine / missing log → resume may fail. (FlowPilot therefore does not persist an owning account on the session row.)
-> Neu active acc nhung khac acc thi co tim thay local log khong?
BUG: dang co 1 bug: start 2 run. swith qua lai 1 hoi , toi 1 luc press History se empty show
show list nhu nao
VI moi provider co cach lam khac nhau
Vi du codex la thread/list
Vay khi bam show history -> can call history cuar all available active provider-> sort lai theo time ?? Hay show theo history cuar run tren supabase

dang miss table workflow_provider_sessions trong migration?

support model+reason in CHAT MODE
press history run need see update of runs

/skills list actually skill
/ agents list actually agent
Based on current provider

# 2. YOLO

## 2.1 Chat mode
### 2.1.1 Normal approve
- test ok for codex. YOLO cho normal command nhu write file
- Chua test claude

### 2.1.2 MCP approve
Chua test cho ca codex/claude

- Codex: MCP ok for YOLO_on

## Flow mode
Chua test

# 3. model.skill.reason - PASSED
## 3.1 Codex
ok
## 3.2 Claude
ok

# Image attached
- Lam button cho attach image temporarily -> save vao local file ??
- attach image

# User Interaction (Structured Questions)

Suggestion FORM show to user
Separate from approvals, FlowPilot can ask the user a structured question (confirm/options popup) through the same pause/resume bridge

- ask_user MCP tool — a FlowPilot-registered custom tool (not built-in) the model discovers via tools/list and may call → best-effort, can not make sure it asked users
- Workflow-driven — the ported Go state machine emits user_question_required directly at a defined step → deterministic (use for required asks).