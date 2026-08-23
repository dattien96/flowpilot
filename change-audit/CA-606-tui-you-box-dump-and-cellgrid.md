# CA-606: /dumpview + TrueColor cell-grid test to lock the live prompt box

## What

Operator: "cứ fix xong tôi test lại vẫn lỗi" — every fix produced green unit tests but the Ghostty screenshot stayed broken (You box losing prompt lines, `│` mid-pane). The loop must stop: capture the live View() to a file and assert the real paint path under TrueColor (the color profile Ghostty uses), instead of guessing from screenshots.

## Why

- Unit tests ran under the Ascii color profile (non-TTY lipgloss strips SGR), while Ghostty renders TrueColor (`48;2;R;G;B`). SGR width drift is invisible in Ascii assertions.
- No way to tell which binary revision the operator was actually running.

## Fix

- `apps/local-runner/internal/tui/app/you_view_dump.go` (new) — `youBoxLayoutRev = "yb605"` stamp; `writeYouViewDump(m, path)` writes: rev, pid, width/height, chatW/sideW/sidebar, colorProfile, raw `View()`, plain `View()`, and every You-box row cut to chatW with its right-border column (`bar=N`).
- `apps/local-runner/internal/tui/app/app.go` — `/dumpview` slash command writes `/tmp/flowpilot-you-view.txt` + toast `dumped yb605 → …`.
- `apps/local-runner/internal/tui/app/model.go` — `/dumpview` entry in `knownSlashCommands` (/help).

## Tests

- `apps/local-runner/internal/tui/app/you_box_cellgrid_test.go` (new) — `forceTrueColor` + the exact 5-line pasted prompt at 197 × h∈{24,30} × claude/codex/grok: all 5 lines present in the chat column in order, no `error│` mid-word break, all `│`/`┐`/`┘` at the top-bar column, `[copy]` on its own row. Green under the current code.
- `apps/local-runner/internal/tui/app/you_view_dump_test.go` (new) — dump file contains `rev=yb605`, geometry, and all prompt lines.
- `go test ./internal/tui/app -count=1` 14s green; `go vet ./internal/tui/...` clean.

## Next step (operator)

`pkill -f "flowpilot chat"` → `just chat-dev <path>` → send the 5-line prompt → `/dumpview` → share `/tmp/flowpilot-you-view.txt`. If `bar=` columns match the test but Ghostty still drops lines, the defect is in the terminal-emulator layer (or a stale binary — the rev stamp reveals it), not in `View()`.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: add /dumpview snapshot and TrueColor cell-grid assertions to break the fix-test-still-broken loop on the You box
# --->8---