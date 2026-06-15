# OK

2 mode: normal chat - workflow
normal phai co du change model, yolo
See Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md

- Lam button cho attach image temporarily -> save vao local file ??

# Test CHAT MODE

- model-reason-skill
- attach image

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

# YOLO

- Real safety = permission_required (app-server) + Codex sandbox + FlowPilot policy, configured together. YOLO is the single value that configures all three consistently, resolved per workflow run/step and applied per Codex thread/turn.

- YOLO is not global to the shared app-server. It is resolved per workflow run/step
  | YOLO | Codex sandbox | Codex approval mode | FlowPilot runner | Proxy MCP |
  | -----------| -----------------| -----------------------------------------------------------| ----------------------------------------| -----------------------------|
  | **true** | full-access | never (auto-run) | auto-approve any `permission_required` | auto-approve policy-allowed |
  | **false** | workspace-write | on-request (emit `permission_required` for dangerous ops) | show approval card, require decision | require approval |

# User Interaction (Structured Questions)

Suggestion FORM show to user
Separate from approvals, FlowPilot can ask the user a structured question (confirm/options popup) through the same pause/resume bridge

- ask_user MCP tool — a FlowPilot-registered custom tool (not built-in) the model discovers via tools/list and may call → best-effort, can not make sure it asked users
- Workflow-driven — the ported Go state machine emits user_question_required directly at a defined step → deterministic (use for required asks).

# Test for codex

Test all items in this checklist: 06-DOD-And-Verification-Checklist.md

# Test for claude

Follow 07-Claude-Adapter-Plan.md
