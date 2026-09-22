# CA-906: Task/BugFix Format References Gain Signature Sections

## Summary

- The embedded `reqscaffold` format references were stale snapshots: the
  live `requirements/` copies had already evolved with `Code Guide
  Signatures`, `Test Signatures`, and `Definition of Done` sections, but
  the pack templates (copied verbatim into every newly scaffolded project)
  still carried the pre-signature layout.
- `08-Task/FORMAT-REFERENCE-TASK.md` gained `§6 Code Guide Signatures`
  (exact production signatures per file, fenced blocks in the file's real
  language) and `§7 Test Signatures` (named tests mapped to ACs, covering
  the safe-fix-contract matrix); Acceptance Check moved to §8, Out of
  Scope to §9, a new `§10 Definition of Done` checklist enforces §6/§7
  plus safe-fix-contract/provider-parity/ledger items, Completion Notes
  to §11.
- `09-BugFix/FORMAT-REFERENCE-BUGFIX.md` gained `§7a Code Guide
  Signatures` and `§8a Test Signatures` (lettered inserts, no renumber)
  plus `§10 Definition of Done`; Follow-Up Document Updates moved to §11.
- Both files are now byte-identical to their live `requirements/`
  counterparts (verified via `diff`), so `reqscaffold.Scaffold` emits the
  current doc contract for new projects.

## Files

- `apps/local-runner/internal/reqscaffold/scaffold-pack/08-Task/FORMAT-REFERENCE-TASK.md`
- `apps/local-runner/internal/reqscaffold/scaffold-pack/09-BugFix/FORMAT-REFERENCE-BUGFIX.md`
- `requirements/08-Task/FORMAT-REFERENCE-TASK.md`
- `requirements/09-BugFix/FORMAT-REFERENCE-BUGFIX.md`

## Out of Scope

- No Go changes: the files are `go:embed` pack data; `go build
  ./internal/reqscaffold/` passes.
- Existing scaffolded projects keep their current FORMAT-REFERENCE files —
  `Scaffold` never overwrites them by design.
- GitNexus MCP was unreachable; no symbols were touched (markdown only),
  so detect_changes had nothing to report regardless.

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: user-report
change_type: docs
summary: scaffold-pack task/bugfix format refs gain Code Guide Signatures + Test Signatures + DoD, synced byte-identical to live requirements/ copies
# --->8---
