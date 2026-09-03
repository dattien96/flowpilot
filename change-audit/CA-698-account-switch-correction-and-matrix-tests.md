# CA-698 — Account-switch interaction correction + full model/provider switch matrix tests (CP-59)

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: CP-59
change_type: docs
summary: SD-26 s14 correction — cross-account session copy (prepareCrossAccountResume) already provides account-switch continuity for codex/grok/claude/gemini; opencode uses the CP-59 envelope; plus test coverage for every model/provider switch case (same/cross provider x TUI entries x runner guards x multi-leg chain)
# --->8---

## Correction (operator review caught a wrong claim)

Earlier statements (CA-695 discussion, Task-318 proposal) claimed mid-conversation account switching "does not work" for codex/grok/claude. **Wrong.** The open/resume path copies the session into the active account's home and continues — `prepareCrossAccountResume` (`interactive_resume.go:2784`, `mode=cross_account`, gemini variant separate). The E2E-06 mismatch guard blocks only silent wrong-account resume. SD-26 §14 records the corrected picture and how the SSOT composes with it (active leg keeps copy-resume; other legs render from the chat transcript; opencode — shared `opencode.db`, no file-copy — relies on the CP-59 envelope).

Consequence for planning: the previously proposed "relax handoff_same_provider + account stamp override" Task-318 shrinks to optional UX wiring (in-place account change on a live chat reusing `prepareCrossAccountResume`); opencode needs nothing (already covered).

## Switch matrix test coverage (this change)

Runner (`chat_switch_test.go`):
- Guards: unknown 404; same-provider 409 (no leg minted); busy 409 via turnInFlight / pendingApprovalID / pendingQuestionID (3 variants); in-flight double-switch 409; detached chat 409 `chat_no_active_leg`; unavailable provider 422 pre-mutation.
- Happy path codex→claude (mint leg, close source `provider_switch`, E-9 exactly-once with from/to, model stamped on the new leg, envelope prefix + prior turns + actions digest, fresh_start no-envelope).
- Crash windows: two-active discriminator, orphan intent clear, closed-no-record heal + re-entry idempotence.
- Lock canary: `TestSwitchNeverHoldsLockAcrossCreateRun`.
- Multi-leg chain: codex→claude→grok in one chat — 3 legs ordered by legSeq, 2 E-9 records, transcript carries all legs' turns (X-1..X-3 boundary skips switch records).
- X-7: `switch_seed_failed` queryable record.

TUI (`chat_switch_tui_test.go`):
- `/model` foreign on live chat → routes; `/provider <key>` on live chat → routes; same-provider (both entries) → in-place nil; workflow run / no run / in-flight → legacy path; Tab cross-provider → routes + queue; Tab same-provider → in-place; bare-model pin derive-once + persist; adopt keeps transcript (success: zero added messages; failure: one error line, source leg kept); seed envelope collapse; prefix parity; client round-trip.

## R1

- New tests green: runner switch suite 14→**20** tests, TUI switch suite 5→**11** tests (total chat SSOT suite: 46). Full runner suite: 13 baseline failures + known stash-proven flakes only; TUI suite: 7 baseline.
