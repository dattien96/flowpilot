# CA-147: Flow Gate Exempt Child Agent Runs

Guard the post-turn flow gate so it only fires on root/hub runs, not on child agent (coder, reviewer) runs that make code changes as part of their normal job.

## Changes

- `apps/local-runner/internal/runner/interactive_service.go`: Changed `if completed {` to `if completed && rs.parentRunID == "" {` at the `runFlowGate` call site. Child runs (`rs.parentRunID != ""`) are now exempt from gate enforcement; root/hub runs (`rs.parentRunID == ""`) behave exactly as before.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-152
change_type: bugfix
summary: exempt child agent runs from post-turn flow gate to fix false CA-note violations mid review loop
# --->8---
