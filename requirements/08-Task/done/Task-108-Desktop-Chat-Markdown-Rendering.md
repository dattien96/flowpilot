# Task-108: Desktop Chat Markdown Rendering

## Metadata

- Document ID: `Task-108`
- Title: `Desktop Chat Markdown Rendering`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-24`
- Last Updated: `2026-06-24`
- Parent Documents: [CP-36: Agent Review Loop And Main-Hub Orchestration](../../07-Coding-Plan/todo/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md)
- Child Documents: `none`
- Related Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](../done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [Task-053: Desktop Chat Turn Skills Summary Chip](../done/Task-053-Desktop-Chat-Turn-Skills-Summary-Chip.md), [CA-125: Desktop Chat Markdown Rendering](../../change-audit/CA-125-desktop-chat-markdown-rendering.md)
- Replaces: `none`
- Tags: `desktop, chat, markdown, timeline, ui`

## AI Quick View

### Summary

- AI assistant responses in the chat Timeline were rendered as raw plain text (`white-space: pre-wrap`), making Markdown-formatted responses (headings, tables, code blocks, lists) unreadable.
- Added a zero-dependency inline Markdown renderer (`parseMdBlocks` + `MarkdownContent`) directly in `Timeline.tsx` — no new npm package required.
- The copy button on each assistant bubble still writes the **raw Markdown string** to the clipboard, preserving the original text for paste operations.
- Supported elements: ATX headings (`#`–`######`), fenced code blocks, horizontal rules, pipe tables (with optional header/separator row), unordered lists, paragraphs, and inline bold / italic / inline-code.

### Current Ask

- Done. `MarkdownContent` replaces the raw `{it.text}` render in the `assistant` case of `Item`. CSS added in `styles.css`.

### Key Decisions

- `T-1` No external Markdown library — a 170-line custom block + inline parser in `Timeline.tsx` is sufficient for the AI response vocabulary and avoids a new dependency.
- `T-2` Copy always uses raw Markdown — `CopyBubble`'s `text` prop is unchanged; only the visual `children` change.
- `T-3` Always render through the Markdown renderer (no detection gate) — plain text produces paragraphs with `white-space: pre-wrap`, which is visually identical to the previous rendering.

### Constraints

- Do not change the `CopyBubble` component — raw-text copy must be preserved.
- Do not add a runtime dependency for this feature.
- Streaming (non-finalized) messages go through the same renderer; the streaming caret appears after the `.md-body` block.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/components/Timeline.tsx`
- `apps/desktop-flowpilot/src/styles.css`

## 1. Goal

Render AI assistant chat messages using proper Markdown formatting (headings, tables, code blocks, lists, bold/italic/inline-code) while preserving the raw Markdown text when the user copies a message.

## 2. Parent Links

- coding plan: [CP-36](../../07-Coding-Plan/todo/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) — CP-36 generates complex Markdown responses (tables, code blocks) that motivated this feature
- tech design: n/a — standalone UI presentation change
- system spec: [SS-12: Multiple Agents](../../05-System-Specs/SS-12-Multiple-Agents.md)
- specific upstream ids: none

## 3. Trigger

CP-36 review loop agents produce multi-section Markdown responses (tables summarising task scope, fenced code dependency diagrams, bold callouts). These rendered as literal `**` and `|` characters in the chat timeline, making responses hard to read.

## 4. Exact Change

- `T-1` Added `parseMdBlocks(text)` — line-by-line block parser producing typed `MdBlock` union objects (code, heading, hr, table, list, paragraph).
- `T-2` Added `renderInline(text, keyPrefix)` — regex-based inline parser for bold (`**`/`__`), italic (`*`/`_`), and inline code (`` ` ``).
- `T-3` Added `MarkdownContent` React component that maps `MdBlock[]` to React elements using stable `bi` index keys.
- `T-4` Changed `assistant` case in `Item` from `{it.text}` to `<MarkdownContent text={it.text} />` — `CopyBubble`'s `text` prop unchanged.
- `T-5` Added `.bubble.assistant { white-space: normal; }` — overrides the inherited `pre-wrap` so block layout works correctly.
- `T-6` Added CSS section `/* Markdown renderer */` in `styles.css` with styles for `.md-body`, `.md-h1`–`.md-h6`, `.md-hr`, `.md-p`, `.md-ul`, `.md-icode`, `.md-pre`, `.md-table-wrap`, `.md-table`.

## 5. Touched Areas

- files:
  - `apps/desktop-flowpilot/src/components/Timeline.tsx`
  - `apps/desktop-flowpilot/src/styles.css`
- modules: Desktop chat timeline
- routes: n/a
- tables: n/a

## 6. Acceptance Check

- Assistant message containing `## Heading` renders as an HTML `<h2>`, not literal `## Heading`.
- Pipe table (`| col | col |`) renders as a formatted `<table>`.
- Fenced code block (` ``` `) renders in a `<pre><code>` block with monospace font and horizontal scroll.
- Clicking the copy button on an assistant bubble places raw Markdown text on the clipboard.
- Plain-text (non-Markdown) assistant messages still render correctly with preserved newlines.
- TypeScript `tsc --noEmit` passes with no errors.

## 7. Out of Scope

- Syntax highlighting inside code blocks.
- Ordered lists (`1. item`).
- Nested lists or block-quotes.
- Markdown rendering in prompt (user) bubbles — those remain plain text.
- Setext-style headings (`===` / `---` underline).

## 8. Completion Notes

- result: Implemented in one pass. TypeScript check passes. No new npm dependency added.
- follow-ups: Syntax highlighting (e.g. highlight.js) can be added as a T-7 in a future task.
- upstream docs updated: none required — this is a pure presentation change with no effect on business rules or provider contracts.
