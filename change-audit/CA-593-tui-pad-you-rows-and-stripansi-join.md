# CA-593: Pad You rows and join panes without styled Width (Ghostty mismatch)

## What

After isolating chat/sidebar panes and left-aligning the You box, image 1 still showed three columns on the You rows (`You` left, `Runner:` middle, `session` right) while AI rows were two columns. `Width(chatW).Render` + `JoinHorizontal` and `lipgloss.Width(styled)` miscounted the narrow You rows on Ghostty.

## Why

- `paintRow` and `Width(chatW).Render` measured `lipgloss.Width(styled)` for `styleUser` (`38;2;R;G;B` true-color with colon). Ghostty's width for that SGR differs, so the 60-col hug-left You row was considered `chatW` and not padded; `JoinHorizontal` then placed the 42-col sidebar right after the 60-col box, not at `chatW`.
- `renderSidebarPane` did `Width(w).Height(h).Render` on the whole block — a styled line wider than `w` wrapped to an extra row, desyncing `h` with chat.
- All Go tests used `stripANSI` + `lipgloss.Width` on the same styled string, so they stayed green while the terminal composite was short.

## Fix

- `apps/local-runner/internal/tui/app/mouse.go:13` — `stripANSI` regex `\\x1b\\[[0-9:;?]*[ -/]*[@-~]|\\x1b\\][^\\x07]*\\x07|\\x1b\\(B` to strip true-color `:` and OSC, so `stripANSI` matches Ghostty.
- `apps/local-runner/internal/tui/app/chat_box.go:148` — `paintRow` now measures `len([]rune(stripANSI(s)))` before and after `Render`, and pads with `style.Render(spaces)` on that rune count, not `lipgloss.Width(styled)`.
- `apps/local-runner/internal/tui/app/session_panel.go:502` — `renderSidebarPane` returns `strings.Join(lines, "\\n")` after per-line `paintRow`, no outer `Width/Height/Render` that can wrap.
- `apps/local-runner/internal/tui/app/session_panel.go:525` — `joinPanes` uses `len([]rune(stripANSI(cl)))` for truncate/pad and `styleCanvas` spaces, so the narrow left-aligned You row is always padded to exactly `chatW` and sidebar starts at fixed column.

## Tests

- Existing `you_box_sidebar_column_test`, `pane_isolation`, `view_no_fullwidth` still green with new `stripANSI` counts.
- `go test ./internal/tui/app -count=1` 14.2s pass, `go vet` clean.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: pad You rows and join panes by stripANSI rune count so Ghostty styled width does not pull sidebar into mid-screen
# --->8---
