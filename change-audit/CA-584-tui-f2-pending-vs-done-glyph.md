# CA-584: F2 pending vs done glyph (no more pending = ✓)

## What

F2 steps showed `[✓] validate` / `[✓] audit` while they were still `PENDING`/empty (rag-harness F1: implement DONE, validate/audit pending but looked done). Chat also never showed `> [RUNNING] validate` because the status was pending.

## Why

- `session_panel.go:90-96` comment said `[✓] done, [ ] pending` but code set `glyph = "✓"` / `"+"` as **default** for every non-RUNNING/FAILED status. Empty/PENDING inherited the done glyph, so pending ≡ done visually. No test asserted pending `[ ]`; existing `tui_f2_right_sidebar_test.go:161` only checked DONE `[+]` and RUNNING spinner.
- Screenshot proof: `run-121885` `+ 2. [DONE] implement` in chat, F2 ` [✓] validate` `[✓] audit` with green check, no `DONE`/`PENDING` distinction.

## Fix

- `apps/local-runner/internal/tui/app/session_panel.go:90` — default `glyph = " "` (→ `[ ]` pending). Explicit `case "DONE"` sets `glyph = "✓"` / `"+`"` (ascii) and `styleStepDone`. Added `case "SKIPPED","CANCELLED"` → `"-"` so they never reuse pending/done. RUNNING still spinner + `RUNNING` token (CA-537).
- No change to `tui_f2_right_sidebar_test.go` (DONE `[+]` still passes); pending now correctly differs.

## Tests

- New additive `apps/local-runner/internal/tui/app/f2_step_pending_glyph_test.go:12` — `TestF2Step_PendingVsDoneGlyph` (pending `""`/`PENDING` → `[ ]` not `✓`, DONE → `[✓]`/`[+]` ascii), `TestF2Step_RunningSpinnerAndFailedGlyph`, `TestF2Step_RagHarness_FullFlowPendingVsDone` — all parameterized over `claude/codex/grok` and ascii.
- `go test ./internal/tui/app -count=1` 18.6s green (old `TestStepTodoRestyle_KeepsLegacyTokens` still passes).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: fix F2 pending glyph to [ ] so pending validate/audit no longer looks done like [✓]
# --->8---
