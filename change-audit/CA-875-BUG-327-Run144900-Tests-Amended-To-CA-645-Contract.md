# CA-875 — BUG-327/run144900 tests amended to the CA-645 contract (operator arbitration)

# ---8<--- flowpilot:change-ledger
feature_key: change-contract
source_doc_id: BUG-327
change_type: test
summary: amend the two red run144900 tests to the arbitrated CA-645 contract (scaffold changes exempt, mutated or not); no production change
# --->8---

## Why

Two green old tests contradicted each other on the identical path
`.agents/skills/flow-mode-orchestrator/SKILL.md` in `baselineWorktreeFingerprint`:
run151954 (`TestBaselineWorktreeFingerprintExcludesSkillpackScaffold`, CA-645,
2026-08-27) demands exclusion, run144900 (`...PreexistingSkill...:51` and
`...MutatedLeftoverStillBlocks:206`, CA-634, 2026-08-25) demands capture and
mutated-blocks. Timeline: CA-634 first, CA-645 two days later broke its
assumption. No production code can satisfy both. Operator arbitrated 2026-09-16:
**CA-645 wins — scaffold changes are OK, no check needed** — with explicit
approval to amend the two losing tests (R1 waiver, this note is the record).

## Change

- **No production change.** Verified the winning contract is already the
  behavior: the writer gate exempts `IsToolOwnedScaffoldPath`
  (`.agents/.claude/.grok/AGENTS.md/CLAUDE.md/.gitignore`, CA-648) plus
  `IsMarkdownDocPath` (BUG-370) before `FrozenContractScopeDrift`, so
  scaffold leftovers — unchanged or mutated — never drift.
- **`run144900_..._test.go` (test-only, 2 functions + header comment):**
  `PreexistingSkill...` now asserts the baseline EXCLUDES the scaffold
  SKILL.md (mirroring the run151954 pin) while the gate still must not block;
  `MutatedLeftoverStillBlocks` keeps its name (CA-634/CA-741/CA-869
  references) but asserts a mutated scaffold leftover does NOT block;
  true non-scaffold drift coverage stays in `TrueDriftViaExtraFileStillBlocks`
  (untouched). Dropped the now-unused `strings` import.

## Tests

- All 7 tests in the run144900 file green (3-provider matrix each), including
  the 5 untouched companions.
- Related suites green: run151954 (except the pre-existing
  `TestIsFlowPlannerExcludedPathCoversSkillpackScaffold` `docs/report.md`
  failure — fails identically on clean HEAD, separate run-201704
  `IsDocOrAuditFile` breadth question, left for owners), run147126,
  run201704, run243681, freeze/frozen-contract families.
- Providers: existing grok/codex/claude subtests all pass (R2 via matrix).

## Prior CA claims kept intact

- CA-645/CA-648 (scaffold exemption), CA-634 F-3 (Continue writer retry),
  CA-427 (no wholesale `.flowpilot/**` exemption — untouched), BUG-370
  (markdown filter), CP-64 (untouched files).
- Partially superseded BY OPERATOR DECISION (not by code drift): CA-634 F-1/F-2
  fingerprint-subtraction for *scaffold* paths. Non-scaffold pre-existing
  dirt subtraction (`turnStartWorktree`/baseline paths in gate_hook.go) is
  unchanged and still active.
