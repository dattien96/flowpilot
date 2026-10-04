# CA-1175 — amend endpoint hardened against gate-reason prose injection

## What changed

`apps/local-runner/internal/changecontract/paths.go`:

- New `prosePathChars` (`;:'"` + backtick + em/en dashes + tab/CR/LF) —
  characters a repo-relative path entry never carries, rejected by
  `IsConcreteCodeTarget` and `IsUserAllowableDriftPath`.
- New `StripDriftProseTail` — cuts the gate reason's own prose suffixes
  (`; not written via this leg's tool calls: ...`, ` — if these are
  operator edits ...`) from a would-be path entry.

`apps/local-runner/internal/runner/interactive_handlers.go`:

- `handleAmendFlow` sanitizes each request path through
  `StripDriftProseTail` before the amendable/unamendable partition, so a
  client that forwards the raw gate tail still sanctions the real path.

`apps/local-runner/internal/tui/app/step_runtime.go` +
`apps/desktop-flowpilot/src/components/flowAwaitingUserDrift.ts`:

- `parseDriftedPaths` (both copies) now cuts the post-marker rest at the
  first `;` / `—` / `–` before comma-splitting, so the prose tail never
  enters the amend payload in the first place.

## Why (live evidence, run-183756 / Task-038)

- The scope-drift gate builds its reason as
  `"…declared paths: <paths>; not written via this leg's tool calls:
  <paths> — if these are operator edits, amend …"` (gate_hook.go
  external-drift branch, D10).
- Both `parseDriftedPaths` copies split only on `,` — the first "path"
  they produced was the entire tail:
  `"core/…/CMakeLists.txt; not written via this leg's tool calls:
  core/…/CMakeLists.txt — if these are operator edits"`.
- `filepath.Ext` on that string returns a non-empty "extension"
  (`.txt — if these…`), so `IsConcreteCodeTarget` passed it and
  `AmendFrozenContractForAllow` wrote it into `declared_paths` verbatim.
- The real `CMakeLists.txt` stayed unsanctioned → the same drift gate
  re-fired → the operator's Allow clicks looped forever while
  `declared_paths` accumulated garbage entries (observed in the run's
  v2 contract records).

Defense in depth: even a client that still sends the tail gets the real
path amended (endpoint strip), anything residual fails the predicates
loudly (CA-427 error / `UnamendablePaths` surface), and prose-shaped
strings can never pass either predicate again.

## Regression coverage

- `changecontract/bug1175_amend_prose_injection_test.go` — live-sample
  rejection through `IsConcreteCodeTarget`, `IsUserAllowableDriftPath`,
  `AmendFrozenContractForAllow` (single + mixed batch), and
  `StripDriftProseTail` table cases; legit paths incl. spaces/parens and
  doc-class routing to `AllowedExtraPaths` pinned.
- `tui/app/tui_blocked_retry_stop_allow_test.go` — two new
  `parseDriftedPaths` cases with the external-drift tail (single +
  multi).
- `desktop-flowpilot …/flowAwaitingUserDrift.test.ts` — same two cases
  for the desktop parser (the one the live Allow button used).

## Notes / boundaries

- `;`, `—`, `–` are legal filename chars in theory; they are the gate's
  message delimiters and virtually absent from real repos — a path
  genuinely containing one cannot be sanctioned through Allow and would
  have produced an unparseable gate reason anyway.
- Corrupted `declared_paths` entries already persisted in old contract
  records are dead weight (they can never match a real written path);
  superseding versions render them inert — no migration.
- Existing v3+ contracts in run-183756 were written manually with clean
  paths before this fix existed; the fix prevents recurrence for every
  future Allow.
