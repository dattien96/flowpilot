# CA-263: Load Built-In Orchestration Options Whenever Bugfix Active

## Scope

Fixed BUG-265, found immediately after verifying BUG-263: the Built-in orchestration select was missing entirely on first opening a chat with the Bug tab already active (e.g. after BUG-263's `openHistoryRun` restore set `chatStartMode="bugfix"` directly), because the options list was only ever fetched as a side effect of the Bug tab's own click handler.

## Changes

- `ChatWorkspace.tsx`: `ChatStartIntentPanel` gained a `useEffect` that loads `builtinOrchestrationOptions` whenever `chatStartMode === "bugfix"` and the list is empty, independent of how that mode was reached.

## Verification

- `npm --prefix apps/desktop-flowpilot run typecheck` — passed.
- No dedicated regression test — this app has no React component test harness; verified by code review.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-265
change_type: bugfix
summary: fetch built-in orchestration options whenever chatStartMode is bugfix, not only when the Bug tab is clicked, so the select renders correctly after BUG-263's restore path
# --->8---
