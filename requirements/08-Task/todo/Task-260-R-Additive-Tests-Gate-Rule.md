# Task-260 — r-additive-tests Gate Rule (Hard Enforce additive-tests-only)

## Metadata

- Document ID: `Task-260`
- Title: `r-additive-tests — gate rule so pre-existing tests cannot be edited without human approval`
- Phase: `task`
- Status: `draft`
- Owner: `DatNguyen`
- Reviewers: `TBD`
- Created: `2026-07-17`
- Last Updated: `2026-07-17`
- Parent Documents: `SD-20`, `CP-35`, `SS-14`
- Child Documents: none
- Related Documents: `Task-100`, `Task-113`, `Task-155`, `Task-159`, skill `additive-tests-only`, skill `oracle-rule`, synthetic gate signal `r-tamper`
- Replaces: none (promotes / supersedes soft-only path of `r-tamper` warn)
- Tags: `flow-gate, r-additive-tests, r-tamper, additive-tests-only, oracle, regression-safety`

## AI Quick View

### Summary

- Skill `additive-tests-only` already tells the AI: on bugfix/task work, **only add new tests**; never edit pre-existing tests without asking the user first.
- Soft skill compliance is insufficient: if the model ignores it, greenwash is still possible until a human notices.
- Today the runner only surfaces a **synthetic warn** `r-tamper` when a pre-existing test file is modified/deleted/renamed — step still completes.
- This task adds a **first-class flow-gate rule** (same family as `r-ca` / `r-task` / `r-bug`) that **reprompts or blocks** when pre-existing tests are edited without an explicit human-approved override.

### Current Ask

- Design + implement `r-additive-tests` (or promote `r-tamper` to a real `DefaultRules()` entry with non-warn action) end-to-end: rule, evaluate/observe, remediation prompt, tests, SD-20 update.

### Key Decisions

- `T-1` **Allowed without approval:** new test files / new test cases only (`Status == "A"` on `IsTestFile` paths, or additive content only if we can detect it — v1 may only enforce at **file** level like current tamper).
- `T-2` **Forbidden without approval:** `M` / `D` / `R` / `C` on a pre-existing test file (reuse oracle tamper signal already in `RunOracle`).
- `T-3` **Action:** default `reprompt` with explicit “revert legacy test edits; add only new tests; if old test is wrong, stop and ask user” — stronger than current warn-only `r-tamper`. Optional escalate to `block` when combined with suite greenwash (see Open Questions).
- `T-4` **Human override path:** align with Task-155 spirit — only after explicit user approval may a pre-existing test be edited; record override so the next turn does not re-fire forever.
- `T-5` **Does not replace** `r-reg` / `r-tests`: those catch red suite; this rule catches **silent edit of the oracle** even when suite stays green.

### Constraints

- Must remain provider-agnostic (post-turn gate in runner).
- Must not break legitimate “add new `_test.go`” workflows.
- Must reuse `IsTestFile` polyglot coverage (Task-159), not Go-only heuristics.
- Do not invent full AST “assertion-only edit” detection in v1 unless already cheap; file-level M/D/R/C is the v1 bar (same as current tamper).
- Chat Mode + Flow Mode tier-1 child self-gate must both see the rule (doc/scope family or adjacent; decide in Exact Change).
- Additive skill remains the soft contract; gate is the hard backstop when AI does not follow the skill.

### Open Questions

- `Q-1` Promote existing synthetic `r-tamper` (change action + register in `DefaultRules`) vs introduce new id `r-additive-tests` and keep `r-tamper` as alias/warn shim?
- `Q-2` Default action: `reprompt` only, or `block` when suite is green **and** pre-existing test was weakened (harder signal)?
- `Q-3` Shared test helpers (`testutil`, fixtures) — treat as test surface (fire rule) or only `IsTestFile` paths?
- `Q-4` How does UI/user “yes, edit this old test” stamp an override for the next gate eval (reuse r-reg decision card storage)?

### Source Refs

- Skill: `.grok/skills/additive-tests-only/SKILL.md` (and skillpack mirror)
- Skill: `oracle-rule` (fix code, not tests)
- `SD-20` §2.8 `r-tamper` (warn-only synthetic)
- `SD-20` §1 gate pipeline; `SS-14` AC-6 oracle integrity
- `CP-35` E2E-9 oracle tampering
- Code: `flowgate/{rules,evaluate,enforce,oracle,observe}.go`, `runner/gate_hook.go` (hardcoded warn append)

## 1. Goal

When an AI turn **modifies, deletes, or renames a pre-existing test file** without a recorded human override, the Flow Gate must **not** let the turn complete as a clean pass. It must **reprompt** (or block) with actionable guidance that matches the `additive-tests-only` skill: revert legacy test edits, add only new tests, or stop and ask the user.

This hardens the soft skill into a **runner rule** next to `r-ca` / `r-task` / `r-bug`, so non-compliance is machine-enforced.

## 2. Parent Links

- coding plan: `CP-35-Context-And-Regression-Engine-Rollout.md` (gate + oracle E2E-9)
- tech design: `SD-20-Flow-Gate-Rule-Semantics.md` (§2.8 r-tamper → upgrade)
- system spec: `SS-14-Code-Context-And-Regression-Safety.md` (oracle integrity / no silent test weaken)
- specific upstream ids: `SD-20 D-1..D-3`, `SS-14 AC-6`, skill `additive-tests-only`

## 3. Trigger

### 3.1 Product gap

| Layer | Today | Gap |
|-------|--------|-----|
| Skill `additive-tests-only` | Prompt/proactive instruction | AI can ignore |
| `r-reg` / `r-tests` | Block when suite red vs baseline | Misses **greenwash** (edit test so suite stays green) |
| Synthetic `r-tamper` | Warn on pre-existing test M/D/R/C | Step **still completes**; easy to ignore |

### 3.2 Relation to UI spawn **Wait for result** (context only — not this task’s implement scope)

UI Agents panel has a **Wait for result** toggle (`SpawnAgentInput.wait` / `waitForResult`, BUG-133):

| Toggle | Meaning |
|--------|---------|
| **Off** | Child runs in background; main turn can continue; fan-out OK |
| **On** | Parent is **blocked** until that child finishes; HTTP spawn waits; further UI spawns gated while blocking child is active |

That is **session exclusivity for a tool/UI spawn** (“main waits for this child”), **not** the flow-graph rule “delegate nodes fire immediately; only `inline` nodes wait for hub control.”

Related but separate residual (from Review Loop analysis, e.g. `run-9437`): hub can still take a **gate reprompt turn** while a flow `coder` child is `RUNNING` because engine advance of `delegate` does not park the hub session. **Hub park while children write** is a follow-up design slice — **out of scope for Task-260** (see §7). Documented here so implementers do not confuse `wait=true` spawn with hub-park or with this test rule.

### 3.3 Why a gate rule now

We already detect the signal (`oracle.HasTampering` / `Tampered` paths). We only need to **raise severity and remediate like r-ca/r-task**, and align copy with `additive-tests-only`.

## 4. Exact Change

- `T-1` **Rule registration** in `DefaultRules()` (and `MergeDefaultRules` / `DocScopeRuleIDs` or TestRuleIDs as appropriate):

  ```text
  ID: r-additive-tests   # or promote r-tamper — resolve Q-1
  Trigger: pre_existing_test_edited
  RequiredOutput: additive_tests_only_or_user_approved_legacy_edit
  Action: reprompt   # default; see Q-2
  Enabled: true
  ```

- `T-2` **Evaluate** — fire when oracle/diff shows pre-existing test file `M|D|R|C` and no human override for those paths/tests. Prefer reusing `RunOracle` tamper list; do **not** fire on pure `A` (new test file).

- `T-3` **Remove or demote** hardcoded warn-only append in `gate_hook.go` so one rule owns the signal (avoid double `r-tamper` + `r-additive-tests` noise).

- `T-4` **`remediationFor`** text (actionable, like Task-113 / BUG-140):
  - Name the tampered paths.
  - Instruct: revert those legacy test edits; keep/add **new** test files only.
  - If production API break forces old-test compile failure: stop — do not mass-edit old tests; ask user (shim vs approved scoped update).
  - Cite skill name `additive-tests-only` + `oracle-rule`.

- `T-5` **Override / approval hook** (minimal v1): document how a user “allow edit of `foo_test.go`” is recorded (may stub to existing override store used by r-reg if present; otherwise Open Question → small follow-up).

- `T-6` **Tests** (additive — new files / new cases only, ironically following the skill):
  - fires on `M` pre-existing `*_test.go`
  - does not fire on `A` new test file
  - does not fire when only production code + new tests change
  - remediation prompt contains path + “do not edit pre-existing tests”
  - `TestDefaultRules` / rule-id lists updated
  - gate_hook: no silent complete when rule in enforce mode

- `T-7` **Docs:** update `SD-20` §2.8 (and rule table); mention in CP-35 E2E matrix or add E2E row; optional skill note “enforced by r-additive-tests when gate runs”.

## 5. Touched Areas

- files (expected):
  - `apps/local-runner/internal/flowgate/rules.go`
  - `apps/local-runner/internal/flowgate/evaluate.go`
  - `apps/local-runner/internal/flowgate/enforce.go`
  - `apps/local-runner/internal/flowgate/oracle.go` / `observe.go` (if signal needs WrittenPaths vs GitDiff clarity)
  - `apps/local-runner/internal/runner/gate_hook.go`
  - `apps/local-runner/internal/flowgate/*_test.go` (new cases; prefer new test file)
  - `requirements/06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md`
- modules: `flowgate`, runner gate hook
- routes: none (post-turn gate only)
- tables: none (unless override store needs persistence — prefer existing override path)

## 6. Acceptance Check

- `go test ./internal/flowgate/...` green with new rule tests.
- Turning a pre-existing test file `M` with no override → gate **reprompt** (enforce mode), not silent complete; detail lists path(s).
- Adding only a **new** `*_test.go` (`A`) → rule does **not** fire.
- Flow Mode coding child with non-empty diff: rule participates in tier-1 self-gate (or documented tier placement).
- Skill `additive-tests-only` still present; gate is hard backstop, not a replacement for the skill text in agent packs.
- No double-warn from leftover synthetic `r-tamper` append.

## 7. Out of Scope

- Hub session park while flow children are `RUNNING` (Review Loop concurrent hub gate fix / `run-9437` class) — separate task; only related conceptually to “wait.”
- Changing UI spawn `wait` / `dependsOn` semantics.
- AST-level “only changed assertion X” detection.
- Auto-approving legacy test edits.
- Weakening `r-reg` / `r-tests` always-block spirit.
- Full Task-155 decision-card UI (may reuse patterns; full card is not required for v1 if reprompt + documented override is enough).

## 8. Completion Notes

- result: draft capture only (this file) — not implemented yet
- follow-ups:
  - Hub park / no gate re-enter hub while delegate child RUNNING (from Review Loop concurrency discussion)
  - Optional promote action to always-block when greenwash suspected
- upstream docs updated: none yet (update SD-20 on implement)

## 9. Suggested rule sketch (for implementer)

| Field | Value |
|-------|--------|
| ID | `r-additive-tests` (preferred over silent rename of warn-only id) |
| Trigger | `pre_existing_test_edited` |
| Signal | `IsTestFile(path) && status ∈ {M,D,R,C}` on turn diff / WrittenPaths intersection; exclude pure `A` |
| Required output | no unapproved legacy test mutation; new tests only **or** recorded user approval |
| Action | `reprompt` (enforce); honor `gate_mode` like `r-ca` unless Q-2 chooses always-block |
| Sibling | `r-reg` = red baseline tests; this rule = edited the oracle files themselves |

### Remediation sketch

```text
Flow gate: pre-existing test file(s) edited: <paths>.
additive-tests-only: do not modify legacy tests without user approval.
1. Revert edits to those files.
2. Add NEW regression tests in a new file (or new tests only).
3. Fix production code so old + new tests pass.
4. If an old test is truly wrong vs AC: stop and ask the user — do not silent-edit.
Do NOT edit change-audit solely to dismiss this rule.
```
