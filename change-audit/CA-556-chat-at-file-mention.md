# CA-556: @ in chat composer mentions a workspace file path

## Problem

Typing `@` in the desktop composer only routed `@agent`. There was no way to
pick a project file and drop its path into the prompt the way `/s` drops a
skill name.

## Fix

- `@` after start-or-space opens a file picker. Query filters
  `git ls-files` (cached + untracked, gitignored skipped). Tab/Enter inserts
  the path and replaces the `@query` token — same as a skill name.
- Start-of-prompt `@coder` (no `/` or `.`, name matches a live child) still
  routes to the agent. Escape dismisses the file picker.
- Runner: `GET /client/workspace-files?cwd=&q=` returns at most 40 paths.
  Desktop + mock clients consume it. File body is not loaded.

## Provider parity

Agnostic: picker only inserts a path string into the user prompt. No
`providerKey` branch.

## Tests (additive)

- `ca556_workspace_files_test.go` — filter + git ls-files
- `chatFileMention.test.ts` — `@` parse, agent vs file, insert path

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CP-43
change_type: feature
summary: @ in the chat composer lists workspace files and inserts the chosen path
# --->8---
