# CA-888 — CP-62 Windows OS fixture parity: home-env + path-separator in 2 test fixtures

# ---8<--- flowpilot:change-ledger
feature_key: zcode-parity
source_doc_id: CP-62
change_type: bugfix
summary: Fix 2 Windows-only CP-62 test fixtures without touching business assertions: fetchConventions helper selects home env by runtime.GOOS, handoff pinned-path expectation uses filepath.Join; 32/32 follow-up + 5/5 conventions green on Windows
# --->8---

## Why

Windows re-verification 2026-09-17 (go 1.26.2) reproduced 2 CP-62
follow-up failures, both fixture-vs-OS mismatches, not business logic:

1. `TestConventionsSource_UserBeatsWorkspace`
   (`internal/runner/context_source_conventions_test.go:63`): helper set
   only `HOME`, but production `conventionsBodyFor` resolves the user layer
   via `os.UserHomeDir()` (Task-343 §4), which reads `USERPROFILE` on
   Windows — the user layer was never found, only the workspace layer.
2. `TestHandoffEnrichment_EmitAtPinnedIndex`
   (`internal/runner/sprint_handoff_enrichment_test.go:159`): expected path
   built with `cwd + "/requirements/..."` while production
   `sprintHandoffPath` uses `filepath.Join` — `\` vs `/` mismatch on
   Windows. Both sides used the same short-path dir; only the separator
   differed.

## Change

- `fetchConventions` helper: pick the home env var by `runtime.GOOS`
  (`USERPROFILE` windows, `home` plan9, else `HOME`), then assert
  `os.UserHomeDir()` resolves to the isolated temp dir (FAIL if the OS
  resolves elsewhere — no silent pass). If home is unresolvable, SKIP with
  reason instead of PASS. All business assertions untouched.
- `TestHandoffEnrichment_EmitAtPinnedIndex`: expected path via
  `filepath.Join(cwd, "requirements", ".flowpilot", "vibe", "handoffs",
  "handoff-sprint-2.yaml")`. Sprint-2 pin assertion untouched.
- Impact: `gitnexus impact fetchConventions` LOW (4 test callers, 0
  processes); `gitnexus impact TestHandoffEnrichment_EmitAtPinnedIndex` LOW
  (0 callers, 0 processes). No production file touched.

## Verification (this machine, Windows)

- `go test ./internal/runner/ -run
  'TestReviewACCoverage_|TestInjectSkillContent_|TestDriftPause_|TestHandoffEnrichment_|TestConventionsSource_'`
  → 32/32 PASS (incl. 5/5 `TestConventionsSource_*`), 0 SKIP.
- `go vet ./internal/runner/ ./internal/flowgate/
  ./internal/changecontract/` PASS; `go build ./...` PASS.
- CP-62 regression suite PASS (incl. full `TestCP61HubDone`
  claude/codex/grok matrix — the earlier TempDir-cleanup flake did not
  reproduce); `TestDecisionCard_*` 3/3 + `TestRunHeadless*` 3/3 PASS.
- `-race` still BLOCKED on this machine (CGO/GCC missing) — not claimed.

## Residual

- Test-Step §6 "test cũ nguyên vẹn" now carries a ledgered exception for
  these 2 OS fixtures (CP-62-Test-Steps §6 + §8). Zero business assertions
  changed; fixtures now pass on both Windows and macOS/Linux by construction
  (`runtime.GOOS` switch + `filepath.Join`).
