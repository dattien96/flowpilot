# CA-558: highlight @file paths and skill names in composer + timeline

## Fix

Desktop: `findMentionSpans` marks file paths and selected/`[skill]` names.
Composer backdrop and prompt bubbles (`MentionText`) use the same spans.
TUI: `highlightMentions` styles those tokens in the input and in sent user
bubbles (`styleStatusHi`).

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CP-56
change_type: feature
summary: highlight attached file paths and skill names in the composer and timeline
# --->8---
