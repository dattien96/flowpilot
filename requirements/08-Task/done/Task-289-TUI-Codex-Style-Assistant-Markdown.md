# Task-289: TUI Codex-Style Assistant Markdown (CP-56 P-8b)

## Metadata

- Document ID: `Task-289`
- Title: `TUI Codex-Style Assistant Markdown`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-13`
- Last Updated: `2026-08-13`
- Feature Keys: `cli-tui`
- Parent Documents: [CP-56](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (P-8b; P-8 polish), [CP-56-Test-Steps](../../07-Coding-Plan/inprogress/CP-56-Test-Steps.md), [SD-02: Architecture](../../06-System-Tech-Design/SD-02-Architecture.md), [Task-287](../todo/Task-287-TUI-Resume-Headless-And-Session-Reset.md)
- Child Documents: `none`
- Related Documents: [Task-279](../todo/Task-279-Bubble-Tea-Chat-Stream-Shell.md), [Task-108: Desktop Chat Markdown Rendering](./Task-108-Desktop-Chat-Markdown-Rendering.md), [CA-463](../../change-audit/CA-463-tui-raw-markdown-box.md), [CA-462](../../change-audit/CA-462-tui-scroll-markdown-cache.md), [CA-460](../../change-audit/CA-460-tui-grok-table-copy-input.md), [CA-464](../../change-audit/CA-464-tui-codex-style-markdown.md)
- Replaces: `None`
- Tags: `cli-tui, markdown, bubbletea, performance`

## AI Quick View

### Summary

- Feasible P-8 polish: render assistant Markdown the Codex way — parse each message to TUI lines, cache completed output, never parse inside `View()`.
- Do not port Codex `markdown_render.rs`, Charm Glamour, or `bubbles/viewport`. Go analog is goldmark GFM walk → Lip Gloss lines.
- Keep the CA-463 `markdown` stroke box, You box, glued `[copy]` (raw source), and `chatRows` scroll cache.
- Tables are in scope with a width fallback to pipe rows; syntax highlighting, mermaid, and a transcript viewport rewrite are not.

### Current Ask

Implement P-8b. Landed in TUI `renderMarkdown` + `buildChatRows`; A8.9–A8.15 green.

### Key Decisions

- `T-1` Codex architecture, not Codex source: goldmark parse + AST walk to `[]string` / Lip Gloss; forbid Glamour, chroma, and `bubbles/viewport` for the transcript.
- `T-2` Performance contract: parse cost is O(live assistant message), not O(transcript × frames). Cache by `(width, content hash)`. Scroll/`View()` must reuse `chatRows` and the markdown cache.
- `T-3` Only assistant `FormatHint == ""` uses styled markdown. User, thinking, tool, approval, question, and steps stay as today.
- `T-4` `[copy]` still copies raw Markdown source. The labeled `markdown` box stays.
- `T-5` Table v1: column-aligned grid when it fits; otherwise raw pipe rows (Codex fallback). No Grok outer-border table studio.
- `T-6` Fences are monospace / indent only — no lexer colors. Unclosed fences while streaming must not panic; show a readable tail.
- `T-7` Bound huge fence/tool bodies (head + tail) so one 10k-line dump cannot freeze a frame.
- `T-8` TUI chrome only. No `internal/runner`, Desktop Timeline, or `session/new` changes. Additive tests only.

### Constraints

- CP-56 D-1/D-7/D-8: thin client, no runner business edits, additive TUI tests.
- CA-463 will_not_undo: You box, copy glued to the answer, rounded input, `chatRows` cache.
- CP-56 non-goal remains: full markdown/diff studio.
- GitNexus impact before editing `renderMarkdown` / `buildChatRows` / `strokeChatRows` when those tools are available.

### Open Questions

- `Q-1` RESOLVED: 200 lines or 32 KiB, head 80 + tail 40, marker `… [N lines truncated] …`.
- `Q-2` RESOLVED: goldmark is a direct `go.mod` require (GFM extension). No Glamour.

### Source Refs

- CP-56 P-8 (optional styled markdown), D-2, D-7, D-8; §3.4 non-goals; A8.9–A8.15; M18.
- CA-463 raw-MD box baseline; CA-462 row cache; Task-108 Desktop subset (headings, fences, tables, lists, inline).

---

## 1. Goal

Assistant turns in `flowpilot chat` show styled Markdown (headings without leftover `#`, lists, inline emphasis/code, fenced code, tables) inside the existing `markdown` box, with Codex-like performance: completed messages are not re-parsed on scroll or cursor blink.

This task is feasible. Effort is real (table layout + streaming incomplete docs), not a 40-line Glamour sample.

## 2. Parent Links

- coding plan: CP-56 P-8b (narrows P-8 “optional glamour” without replacing Task-287)
- tech design: SD-02 (TUI is a parallel client; presentation-only)
- system spec: none dedicated; client chrome only
- specific upstream ids: CP-56 D-2, D-7, D-8; §3.4; P-8 polish bullet

## 3. Trigger

CA-463 parked Glamour because Charm `glamour.Render` in the View/chat-row path made scroll slow and still looked wrong. Codex CLI (open `codex-rs/tui` markdown pipeline) and Claude CLI both parse Markdown into their TUI line model and cache completed segments. Operator asked whether that direction is feasible; it is, as a scoped P-8b task — not as a port of Codex’s full renderer and not as Glamour+Viewport.

Current code (`markdown.go`) wraps raw source. `buildChatRows` still uses `wrapText` for assistant bubbles. `chatRows` already caches by width + transcript signature; this task adds a per-message markdown cache in front of that.

## 4. Exact Change

- `T-1` Replace wrap-only `renderMarkdown` with goldmark GFM parse + walk that emits wrapped Lip Gloss lines for: ATX headings, paragraphs, lists, blockquotes, HR, fenced code, pipe tables, inline bold/italic/code/links (link text visible; URL optional dim suffix).
- `T-2` Add `mdCache` keyed by width + content hash (LRU, ~64–128 entries). `renderMarkdown` must not run from `View()` except via cache miss. Streaming: only the last unfinished assistant message is a miss.
- `T-3` Wire `buildChatRows` so assistant `FormatHint == ""` uses `renderMarkdown` at the boxed inner width; keep `strokeChatRows` title `markdown`; keep `[copy]` on the last inner row; copy payload remains raw `msg.Content`.
- `T-4` Tables: measure columns; draw aligned rows when min width fits; else emit the original pipe lines wrapped. Spillover / ragged rows become plain text after the grid.
- `T-5` Code fences: keep language label on the first inner line; body monospace; no chroma. Unclosed fence at EOF renders body so far.
- `T-6` Truncate oversized fence/tool bodies per `Q-1` default; leftover stays in raw copy source.
- `T-7` ASCII mode: box-drawing tables degrade to `|` / `-` / `+` using existing `asciiMode`.
- `T-8` Additive tests listed in CP-56-Test-Steps A8.9–A8.15. Do not edit `chat_layout_test.go` or other legacy tests. Existing CA-463 raw-marker tests must be updated only if the operator already allowed untracked TUI-branch tests to track the new AC (same additive-tests-only exception as CA-458–463).

## 5. Touched Areas

- files: `apps/local-runner/internal/tui/app/markdown.go` (rewrite), `apps/local-runner/internal/tui/app/app.go` (`buildChatRows` assistant path), `apps/local-runner/internal/tui/app/model.go` (optional cache fields), `apps/local-runner/go.mod` (goldmark only — not glamour), additive `*_test.go` under `internal/tui/app`
- modules: `flowpilot-runner` TUI package only
- routes: none
- tables: none (DB)

## 6. Acceptance Check

- [x] A8.9–A8.15 green (`go test ./internal/tui/app -count=1`)
- [x] `TestChatRows_ScrollReusesRowCache` still green
- [ ] Manual M18: one assistant reply with heading, table, fence, and `**bold**` is readable inside the `markdown` box; `[copy]` pastes source with `#` / `\|` / ` ``` `
- [x] Scroll and cursor blink do not hitch on a 20+ turn transcript (parse counter / cache-hit test, not a stopwatch SLO)
- [x] `git diff` stays inside TUI + goldmark module deps + this Task/CA; no runner/Desktop

## 7. Out of Scope

- Charm Glamour, Tokyo Night stylesheets, chroma/syntax colors
- Replacing custom transcript viewport with `bubbles/viewport`
- Mermaid, diff studio, image-in-markdown
- Desktop `Timeline.tsx` / Task-108 parser changes
- Runner adapters, Grok `session/new`, approval/question HTTP
- Task-287 `/new` `--resume` `-p`
- Matching Codex/Grok table chrome pixel-for-pixel

## 8. Completion Notes

- result: implemented 2026-08-13. goldmark GFM walk + LRU cache; assistant box still titled `markdown`; copy remains raw source. `go test ./internal/tui/app -count=1` pass. GitNexus MCP unavailable this session; inspected `renderMarkdown` / `buildChatRows` / `strokeChatRows` locally. Provider-agnostic TUI chrome.
- follow-ups: operator M18 in a live TUI
- upstream docs updated: CP-56 child path → done; CP-56-Test-Steps A8.9–A8.15; CA-464
