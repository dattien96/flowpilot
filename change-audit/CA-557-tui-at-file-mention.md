# CA-557: TUI @ file mention picker

## Problem

CA-556 added `@file` on Desktop. TUI still had no way to pick a workspace
path into the prompt.

## Fix

Same contract as Desktop:

- `@` at start or after space opens the file picker (`files:`).
- Query filters `GET /client/workspace-files?cwd=&q=` (git ls-files).
- Tab / Enter replace `@query` with the path. Prompt is not sent.
- Start-of-prompt `@coder` (exact live agent name, no `/` or `.`) stays agent.

## Tests (additive)

- `ca557_tui_file_mention_test.go`
- `ca557_list_workspace_files_test.go`

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: feature
summary: TUI @ picker lists workspace files and inserts the chosen path
# --->8---
