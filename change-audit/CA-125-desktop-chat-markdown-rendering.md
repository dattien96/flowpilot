# CA-125: Desktop Chat Markdown Rendering

## Summary

Added a zero-dependency inline Markdown renderer to the desktop chat Timeline so AI assistant responses display formatted headings, tables, fenced code blocks, lists, and inline bold/italic/code rather than raw Markdown text.

## Changed Files

- `apps/desktop-flowpilot/src/components/Timeline.tsx` — added `parseMdBlocks`, `renderInline`, `MarkdownContent`; updated `assistant` case in `Item`
- `apps/desktop-flowpilot/src/styles.css` — added `.md-body`, `.md-h*`, `.md-hr`, `.md-p`, `.md-ul`, `.md-icode`, `.md-pre`, `.md-table-wrap`, `.md-table` styles

## Notes

Copy button continues to write raw Markdown to clipboard (`CopyBubble.text` prop unchanged). Plain-text messages are unaffected — they render as `md-p` paragraphs with `white-space: pre-wrap`.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: Task-108
change_type: feature
summary: add inline Markdown renderer to desktop chat timeline assistant bubbles
# --->8---
