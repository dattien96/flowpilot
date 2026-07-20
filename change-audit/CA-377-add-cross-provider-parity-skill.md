# CA-377: Add cross-provider-parity skill; fix stale skillpack version drift

## Summary

Added a new internal skill, `cross-provider-parity`, to the embedded skillpack (`apps/local-runner/internal/skillpack/flow-pack/common/`), mirroring the existing `additive-tests-only`/`oracle-rule` convention: whenever a bugfix or task touches Claude, Codex, or Grok provider code, the agent must classify the code path (provider-agnostic / shared-logic-per-adapter / genuinely-per-provider) and verify or test all three accordingly, instead of assuming a fix that works for the reported provider also works for the other two. Grounded directly in this session's BUG-296 (Claude-only permission-wiring gap, hidden until each adapter's own code was read) and BUG-298 (a genuinely provider-agnostic dispatch predicate, correctly covered by one parameterized test).

While wiring the new skill into `skillpack_test.go`'s `commonSkills` list, discovered the list was already stale — missing four skills that already existed in the embedded pack (`additive-tests-only`, `kill-review`, `codex-claude-review-loop`, `codex-grok-review-loop`), causing three install-count tests to fail independent of this change. Fixing the list surfaced a second, real pre-existing bug: those same four skills' `SKILL.md` files had never had their `version:` frontmatter field bumped to the current `PackVersion` (6) — two carried stale numbers (`additive-tests-only: 1`, `kill-review: 2`) and two had no `version:` field at all (`codex-claude-review-loop`, `codex-grok-review-loop`). `fileMatchesVersion` therefore always returned false for them, so `Install()` reinstalled all four on every call instead of skipping them once current, and `Status()` never reported the pack as `Current`. Confirmed via a stashed-skill baseline check that the count-test failures predate this change; per `additive-tests-only`, both fixes (the test list, and the four skills' version fields) were made only after explicit user approval.

## Verification

- `go test ./internal/skillpack/... -count=2`: 26 passed (13 tests × 2 runs), fully deterministic.
- `go build ./internal/skillpack/...` / `go vet ./internal/skillpack/...`: clean.
- Confirmed via a temporary removal of the new skill folder that the three install-count test failures (`TestInstall_CommonOnlyForNonePlatform`, `TestInstall_AndroidIncludesCommonAndAndroid`, `TestInstall_KMMIncludesCommonAndroidIosAndKmm`) were already present on the unmodified baseline (stale `commonSkills` list), not introduced by this change.
- additive-tests-only honored: `commonSkills`'s stale-list fix and the four skills' version-field fix were both made only after explicit user approval (two separate confirmations), per this repo's own skill.

## Files

- `apps/local-runner/internal/skillpack/flow-pack/common/cross-provider-parity/SKILL.md`: new skill (`version: 6`, matching current `PackVersion`).
- `apps/local-runner/internal/skillpack/skillpack_test.go`: `commonSkills` list updated to include all five previously-missing skills.
- `apps/local-runner/internal/skillpack/flow-pack/common/additive-tests-only/SKILL.md`: `version: 1` → `6`.
- `apps/local-runner/internal/skillpack/flow-pack/common/kill-review/SKILL.md`: `version: 2` → `6`.
- `apps/local-runner/internal/skillpack/flow-pack/common/codex-claude-review-loop/SKILL.md`: added missing `version: 6`.
- `apps/local-runner/internal/skillpack/flow-pack/common/codex-grok-review-loop/SKILL.md`: added missing `version: 6`.

# ---8<--- flowpilot:change-ledger
feature_key: skill-injection
source_doc_id: -
change_type: feature
summary: Added the cross-provider-parity skill to the embedded skillpack and fixed stale version/list drift in four other common skills that prevented Install() from ever skipping them.
# --->8---
