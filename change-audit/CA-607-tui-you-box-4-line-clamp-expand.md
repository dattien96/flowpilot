# CA-607: restore 4-line clamp + click-to-expand on the You prompt box (TUI)

## What

Operator: "TUI ui bị lỗi khi show prompt box. rule là show max 4 line. nếu dài hơn thì show ... và cho phép click để expand" — the screenshot showed the 5-line Change Contract prompt overflowing the You box. The 4-line clamp removed by CA-603 is restored on top of the current full-width plain `youBox` frame: collapsed = 4 wrapped lines + `....` tail on line 4, click the whole box to expand/collapse, `[copy]` keeps its own last row and wins hit-testing.

## Why

- CA-603 dropped the clamp because the box only showed a few lines; the operator now restates the rule explicitly: max 4 lines, `....` when longer, click to expand. Supersedes CA-603's no-clamp decision (operator-approved test updates).
- Desktop `PromptCard` never changed (`PROMPT_MAX_LINES = 4`, `....`, click card) — parity restored.

## Fix

- `apps/local-runner/internal/tui/app/app.go` —
  - `chatRow.PromptExpandKey` re-added (non-empty for every row of a truncatable box).
  - `maxUserPromptLines = 4`, `userPromptEllipsis = "...."`, `userPromptExpanded(content)`, `toggleUserPrompt(content)`, `clampPromptLines(lines, innerW)` (cut last kept line so `....` fits before `youBox` hard-slices).
  - boxed-user branch: wrap at `innerW = width-3`, then `truncatable := len(lines) > 4`; if truncatable && !expanded → `clampPromptLines`; pass `truncatable, msg.Content` to `youBox`.
  - `chatRowsSig` hashes `expandedUserPrompts` (sorted keys, marker byte 3) so expand state busts the row cache.
- `apps/local-runner/internal/tui/app/chat_box.go` — `youBox` gains `truncatable, expandKey`; every row (top border, content, `[copy]` row, bottom border) carries `PromptExpandKey` when truncatable, so the whole box is clickable.
- `apps/local-runner/internal/tui/app/mouse.go` — `user-prompt-expand:` target case + `hitUserPromptChrome` after `hitCopyChrome` (copy wins) and before `hitToolGroupChrome`.
- `apps/local-runner/internal/tui/app/model.go` — `expandedUserPrompts map[string]bool` re-added (content-keyed, survives index shifts).
- `apps/local-runner/internal/tui/app/you_view_dump.go` — `youBoxLayoutRev` bumped `yb605` → `yb607`.

## Tests

- Restored `tui_user_prompt_clamp_test.go` to HEAD clamp version (operator-approved): truncate-to-4 / click-toggle / copy-wins / short-prompt-untouched × claude/codex/grok.
- Updated (operator-approved, were locking CA-603 no-clamp): `you_box_lines_test.go` `TestRegression_YouBoxUserPastedPromptAllLines`, `you_box_cellgrid_test.go` `TestRegression_YouBoxTrueColorViewAllLines`, `you_view_dump_test.go` `TestDumpView_WritesRevAndPrompt` — now collapsed (4 lines + `....`, `symbols:` hidden) → expand click (all 5 lines, no `....`) → collapse click, borders aligned, `[copy]` own row, TrueColor path.
- New `you_box_prompt_clamp_test.go` — `TestYouBoxClamp_ChangeContractCollapsedExpanded` (exact 5-line Change Contract prompt at 197/80 × 3 providers), `TestYouBoxClamp_CopyWinsOverExpand`, `TestYouBoxClamp_ShortPromptNoClamp`.
- `prompt_overflow_test.go` comment updated (CA-603 → CA-607); assertions untouched.
- `go test ./internal/tui/app -count=1` 14.3s green; `go test ./internal/tui/... ./internal/cli/... -count=1` green; `go vet ./internal/tui/...` clean.

## Provider parity

TUI render path is provider-agnostic (no provider-specific code). All new/updated matrix tests run × claude/codex/grok.

## Not undone

- `youBox` plain full-width frame (CA-600/601/602), `[copy]` own row (CA-604), no `styleCanvas` on box rows (CA-605), `/dumpview` rev stamp (CA-606), wrap at box inner width (CA-600), desktop 4-line clamp (CA-559).

## Residual risk

Ghostty mid-pane wrap on expanded long boxes would still need `/dumpview` (`yb607`) diagnosis — clamp makes collapsed boxes short, so bleed surface is smaller.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: restore the 4-line user-prompt clamp with '....' tail and click-to-expand on the full-width You box, superseding CA-603's no-clamp decision
# --->8---