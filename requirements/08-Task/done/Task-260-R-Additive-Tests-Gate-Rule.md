# Task-260 — safe-fix-contract Chat Plan/Code Pointer + r-additive-tests Gate

## Metadata

- Document ID: `Task-260`
- Title: `safe-fix-contract — Chat Plan/Code skill pointer + r-additive-tests gate (hard enforce additive-tests-only)`
- Phase: `task`
- Status: `done`
- Owner: `DatNguyen`
- Reviewers: `TBD`
- Created: `2026-07-17`
- Last Updated: `2026-08-31` — code + tests + SD-20 landed; see §8
- Parent Documents: `SD-20`, `CP-35`, `SS-14`, `CP-58` (note only for Flow plan/coder steps)
- Child Documents: none
- Related Documents: `Task-100`, `Task-113`, `Task-155`, `Task-159`, `CP-53`, skill `safe-fix-contract`, skill `additive-tests-only`, skill `oracle-rule`, skill `cross-provider-parity`, skill `context-discipline`, synthetic gate signal `r-tamper`
- Replaces: none (promotes / supersedes warn-only `r-tamper` path)
- Tags: `flow-gate, r-additive-tests, r-tamper, additive-tests-only, oracle, regression-safety, safe-fix-contract, skill-injection, chat-posture`

## AI Quick View

### Summary

- Umbrella skill `safe-fix-contract` (R1 no old-test edit, R1 old-test red → stop, R2 Claude+Codex+Grok, R3 matrix + history) is soft; AI can ignore it unless explicitly mentioned.
- Today the runner only surfaces a **synthetic warn** `r-tamper` when a pre-existing test file is `M/D/R/C` — step still completes.
- This task does two things in **Chat Mode only**: (a) auto-mention `safe-fix-contract` as a **pointer** (`## Selected Skills` → path + 1-line desc, not full content) on every **Plan** and **Code** posture turn, and (b) promote tamper to a **first-class gate rule** `r-additive-tests` (`reprompt`) that reuses `oracle.HasTampering` and cites the umbrella skill. Flow plan/coder steps are **noted in CP-58** only — no Flow wiring in this task.

### Current Ask

- Rewrite this Task doc to the new scope (Chat Plan+Code pointer + single gate), then implement end-to-end: rule, evaluate/observe, Chat Plan/Code init merge, remediation prompt, additive tests, SD-20 update, CP-58 note.

### Key Decisions

- `T-1` **Gate id:** new `r-additive-tests` in `DefaultRules()`; **remove** hardcoded warn `r-tamper` append in `gate_hook.go` (Q-1 resolved: new id, not alias).
- `T-2` **Signal:** `IsTestFile(path) && status ∈ {M,D,R,C}` on turn diff, excluding pure `A` (new test file). Reuse `RunOracle` `Tampered` list; respect `test_overrides.json` keyed by `filepath.Base(path)`.
- `T-3` **Action:** `reprompt` only (Q-2 resolved) — explicit “revert legacy test edits; add only new tests; if old test is wrong, stop and ask user”. No always-block.
- `T-4` **Test-file scope:** only `IsTestFile` paths fire (Q-3 resolved); shared helpers/fixture edits do not fire. Polyglot coverage from Task-159.
- `T-5` **Override:** user “yes, edit this old test” is recorded via existing `test_overrides.json` (Task-155) keyed by `filepath.Base(path)` so the next oracle pass no longer re-flags that file. CP-53 waiver ledger not required in v1.
- `T-6` **Init Chat Plan+Code:** on `runKind==chat && !flowEngineDriven && ChatPosture ∈ {plan, code}`, merge `{Name: "safe-fix-contract"}` into `SelectedSkills` **before** `promptPrep` so `injectSelectedSkills` emits a pointer (`- /safe-fix-contract → <path>` + 1-line desc) for every provider (Claude/Codex/Grok/Gemini/Opencode). Dedup if user already selected it. `scan`/`non`/`""` never auto-inject. Flow turns never auto-inject in this task.
- `T-7` **Family is prompt-only:** do **not** invent `r-parity` / AST matrix detectors in v1. R2 (3-provider) and R3 (matrix completeness) and history reading stay enforced by the skill text injected at Plan/Code time. Existing gates `r-newtest`, `r-tests`, `r-reg`, `r-ca`, `r-fk`, `r-contract` remain unchanged; `r-additive-tests` remediation cites `safe-fix-contract` umbrella + child skills.
- `T-8` **No Flow gate wiring:** do **not** add `r-additive-tests` to `DocScopeRuleIDs()` / child self-gate. CP-58 owns Flow `preflight_contract_plan` / `plan_writer` / `plan_reviewer` and coding-child wiring.

### Constraints

- Gate must remain provider-agnostic (post-turn runner gate).
- Must not break legitimate “add new `_test.go`” workflows — pure `A` never fires.
- Must reuse `IsTestFile` polyglot coverage, not Go-only heuristics; no full AST assertion diff in v1.
- Init must be **pointer only** — do not embed full `SKILL.md` into the prompt (token budget; same contract as `injectSelectedSkills`).
- Init must not inject on Flow turns, `scan`/`non`, or when the user already selected the skill (dedup).
- Additive skill remains the soft contract; gate is the hard backstop when AI does not follow the skill.
- Chat Mode hub (`runFlowGate`) only; no `DocScopeRuleIDs` change in this task.

### Open Questions

- None — `Q-1`..`Q-4` resolved in Key Decisions above. Any future Flow wiring is tracked in `CP-58` (note only).

### Source Refs

- Skill: `.agents/skills/safe-fix-contract/SKILL.md` (+ skillpack mirror `common/safe-fix-contract`)
- Child skills: `additive-tests-only`, `oracle-rule`, `cross-provider-parity`, `context-discipline`
- `SD-20` §2.8 `r-tamper` (warn-only synthetic → upgrade to `r-additive-tests`)
- `SD-20` §1 gate pipeline (Observe → LoadRules → Evaluate → r-tamper append → Enforce); `SS-14` AC-6 oracle integrity
- `CP-35` E2E-9 oracle tampering; `CP-58` (Flow plan/coder note — no code in this task)
- Code: `flowgate/{rules,evaluate,enforce,oracle,observe}.go`, `runner/gate_hook.go:309`, `runner/runner.go:injectSelectedSkills`, `runner/interactive_service.go:runTurn`, `runner/chat_posture.go:ChatPosturePlan/ChatPostureCode/IsReadOnlyChatPosture`, `skillpack/flow-pack/common/safe-fix-contract/SKILL.md`
- Prior CA: `CA-442` (`r-newtest`), `CA-128` (`r-task`), `CA-154`/`CA-177` (`subMode/flowRef` — not used here)

## 1. Goal

When an AI turn in **Chat Mode** modifies, deletes, or renames a pre-existing test file without a recorded human override, the Flow Gate must **not** let the turn complete as a clean pass — it must **reprompt** with actionable guidance that cites `safe-fix-contract` / `additive-tests-only` / `oracle-rule`. Separately, every **Chat Plan** and **Chat Code** posture turn must auto-mention `safe-fix-contract` as a skill pointer so planning and coding both know the umbrella contract without the operator repeating `/safe-fix-contract`.

This hardens the soft umbrella skill into a runner rule next to `r-ca` / `r-task` / `r-bug`, while keeping the skill as the pre-turn source of planning/coding discipline.

## 2. Parent Links

- coding plan: `CP-35-Context-And-Regression-Engine-Rollout.md` (gate + oracle E2E-9)
- tech design: `SD-20-Flow-Gate-Rule-Semantics.md` (§2.8 r-tamper → upgrade to `r-additive-tests`)
- system spec: `SS-14-Code-Context-And-Regression-Safety.md` (oracle integrity / no silent test weaken, AC-6)
- coding plan (note only, no code): `CP-58-Bug-Task-Cp-Harness-Plan-Review-Loop.md` (Flow plan/coder skill wiring — deferred)
- specific upstream ids: `SD-20 D-1..D-3`, `SS-14 AC-6`, skills `safe-fix-contract` / `additive-tests-only` / `oracle-rule`

## 3. Trigger

### 3.1 Product gap

| Layer | Today | Gap |
|-------|--------|-----|
| Skill `safe-fix-contract` (+ 4 children) | Prompt/proactive instruction when explicitly mentioned | AI can ignore; operator must repeat `/safe-fix-contract` on every bug/task turn |
| `r-reg` / `r-tests` | Block when suite red vs baseline | Misses **greenwash** (edit test so suite stays green) |
| Synthetic `r-tamper` | Warn on pre-existing test `M/D/R/C` | Step **still completes**; easy to ignore |
| Chat Plan posture | Read-only (no writes) | Plan can be written without knowing R1/R2/R3/history discipline |

### 3.2 Why gate + init now

We already detect the tamper signal (`oracle.HasTampering` / `Tampered` paths from `IsTestFile` + `M/D/R/C`). We only need to **raise severity and remediate like r-ca/r-task** and **auto-mention the umbrella skill on Plan+Code turns** so the model reads it before planning/coding. Flow plan/coder wiring is structurally separate (pack YAML `promptTemplate` + `DocScopeRuleIDs`) and is tracked in `CP-58` to avoid scope creep here.

### 3.3 Relation to UI spawn Wait for result (context only — not this task’s implement scope)

UI Agents panel has a **Wait for result** toggle (`SpawnAgentInput.wait` / `waitForResult`, BUG-133):

| Toggle | Meaning |
|--------|---------|
| **Off** | Child runs in background; main turn can continue; fan-out OK |
| **On** | Parent is **blocked** until that child finishes; HTTP spawn waits; further UI spawns gated while blocking child is active |

That is **session exclusivity for a tool/UI spawn** (“main waits for this child”), **not** the flow-graph rule “delegate nodes fire immediately; only `inline` nodes wait for hub control.”

Related but separate residual (from Review Loop analysis, e.g. `run-9437`): hub can still take a **gate reprompt turn** while a flow `coder` child is `RUNNING` because engine advance of `delegate` does not park the hub session. **Hub park while children write** is a follow-up design slice — **out of scope for Task-260** (see §7).

## 4. Exact Change

- `T-1` **Rule registration** in `DefaultRules()` (`flowgate/rules.go`) and `MergeDefaultRules`:

  ```text
  ID: r-additive-tests
  Scope: step
  Trigger: pre_existing_test_edited
  RequiredOutput: additive_tests_only_or_user_approved_legacy_edit
  Action: reprompt
  Enabled: true
  ```

  Resolve Q-1: **new id** `r-additive-tests`, not an alias of `r-tamper`. Q-2: `reprompt` only.

- `T-2` **Evaluate** (`flowgate/evaluate.go` → `checkRule` case `pre_existing_test_edited`):

  - Fire when `IsTestFile(path) && status ∈ {M,D,R,C}` and no active override for that file. Prefer reusing `RunOracle` `Tampered` list (already `IsTestFile` + `M/D/R/C` + `!IsOverridden(overrides, filepath.Base(path))`); do **not** fire on pure `A` (new test file).
  - Polyglot: reuse Task-159 `IsTestFile` (Go, JS/TS, Python, Kotlin, Java, Swift, Dart) — no Go-only heuristic.
  - Respect `test_overrides.json` (Task-155) — `filepath.Base(path)` key, same as oracle.

- `T-3` **Remove hardcoded warn** in `runner/gate_hook.go:309` (`if oracle.HasTampering { append r-tamper warn }`) so one rule owns the signal and there is no double `r-tamper` + `r-additive-tests` noise.

- `T-4` **`remediationFor`** (`flowgate/enforce.go`):

  - Name the tampered paths (`oracle.Tampered` / violation `Detail`).
  - Instruct: revert those legacy test edits; keep/add **new** test files only.
  - If production API break forces old-test compile failure: stop — do not mass-edit old tests; ask user (shim vs approved scoped update).
  - Cite umbrella `safe-fix-contract` + child skills `additive-tests-only` / `oracle-rule`.
  - Sketch:

  ```text
  Flow gate (r-additive-tests): pre-existing test file(s) edited: <paths>.
  safe-fix-contract / additive-tests-only: do not modify legacy tests without user approval.
  1. Revert edits to those files (git checkout / git restore).
  2. Add NEW regression tests in a new file (or new cases in a new file only).
  3. Fix production code so old + new tests pass.
  4. If an old test is truly wrong vs AC: stop and ask the user — do not silent-edit.
  Do NOT edit change-audit solely to dismiss this rule.
  ```

- `T-5` **Override / approval hook** (minimal v1): reuse existing `flowgate/override.go` store (`.flowpilot/guard/test_overrides.json`, keyed by `filepath.Base(path)`). A user “allow edit of `foo_test.go`” recorded there prevents the next oracle/evaluate pass from re-flagging that file. Document in SD-20 / CA note. CP-53 waiver ledger expiry not required in v1.

- `T-6` **Chat Plan+Code skill pointer** (`runner/interactive_service.go:runTurn`):

  - Condition: `rs.runKind=="chat" && !rs.flowEngineDriven && posture ∈ {ChatPosturePlan, ChatPostureCode}` (`runner/chat_posture.go`). `scan` / `non` / `""` never auto-inject. Flow turns (`flowEngineDriven==true` or `runKind != chat`) never auto-inject in this task.
  - Merge: if `SelectedSkills` does not already contain a case-insensitive `safe-fix-contract` entry, prepend/append `{Name: "safe-fix-contract", Source: "builtin"}` (use existing `SkillSelection` shape) **before** `promptPrep` so `injectSelectedSkills` (already wired for all providers in `provider_registry.go`) emits:

  ```text
  ## Selected Skills

  Read each skill file listed below and follow its process before responding.

  - /safe-fix-contract → <workspace>/.agents/skills/safe-fix-contract/SKILL.md
    > Operator safety pack for every bug fix and coding task: never break old tests ...
  ```

  - Pointer only — do not embed full `SKILL.md` content (token budget; same contract as existing selected-skills injection).
  - All providers share `injectSelectedSkills`; the merge is in `runTurn` once (provider-agnostic, Case 1 per `cross-provider-parity`).
  - Chat Plan is read-only (`IsReadOnlyChatPosture`) and never changes code, but the plan must still know how to plan compliant with R1/R2/R3/history.

- `T-7` **Family housekeeping — no new gate ids in v1**:

  - Do **not** invent `r-parity` / matrix / history gates. R2 (Claude+Codex+Grok) and R3 (matrix completeness) and “read FEATURE-KEYS + CA history” remain enforced by the skill text injected at Plan/Code time.
  - Existing gates `r-newtest` (`production_change_no_new_test`), `r-tests`, `r-reg`, `r-ca`, `r-fk`, `r-contract`, `r-scope` remain unchanged; `r-additive-tests` is the only new `DefaultRules` entry. `r-additive-tests` stays in **hub `runFlowGate` only** — do **not** add to `DocScopeRuleIDs()` / child self-gate (CP-58 owns Flow plan/coder wiring).
  - Keep `skillpack` common group as-is: `safe-fix-contract` already in `.agents/.claude/.grok/.opencode` mirrors; no new skill files.

- `T-8` **Tests** (additive — new files / new cases only, following the skill itself):

  - Gate: `M` pre-existing `*_test.go` (and polyglot `IsTestFile`) → violation `r-additive-tests` `reprompt`; detail lists path(s).
  - Gate: `A` new test file → does **not** fire (include polyglot variants, e.g. `*_test.go` + `*.test.ts`).
  - Gate: production code + new test file → does **not** fire.
  - Gate: remediation prompt contains tampered path + “do not edit pre-existing tests” / `safe-fix-contract` cite.
  - Gate: `TestDefaultRules` includes `r-additive-tests`; `MergeDefaultRules` preserves it.
  - Gate: `gate_hook.go` — no silent complete when `r-additive-tests` in enforce mode (reprompt suppressed detail + re-launch).
  - Gate: overridden `filepath.Base(path)` (via `IsOverridden`) → does **not** fire tamper for that file.
  - Init: `ChatPosturePlan` and `ChatPostureCode` chat turns → outbound prompt contains `## Selected Skills` + `/safe-fix-contract` pointer; `scan` / `non` / Flow `flowEngineDriven` → does **not** inject; pre-selected `safe-fix-contract` → dedup (no double pointer).
  - Init: provider parity — `interactive_service.runTurn` merge covers all provider adapters (one test proving Codex + Claude + Grok share `injectSelectedSkills` path is sufficient; note evidence in CA).

- `T-9` **Docs**:

  - Update `SD-20` §2.8: replace `r-tamper` synthetic warn description with `r-additive-tests` (`DefaultRules` `reprompt`, hard fail) and retain `r-tamper` as historical alias only (no code).
  - Update rule table (§2) and gate pipeline (§1) to list `r-additive-tests`.
  - Add **CP-58 note** (no code): Flow `preflight_contract_plan` / `plan_writer` / `plan_reviewer` / `cp_plan_writer` + coding-child `r-additive-tests` wiring will be added when CP-58 lands (Flow plan/coder pointer = `promptTemplate: prompts/plan-safe-fix-contract.md` reuse or `SelectedSkills` equivalent; coding child = `DocScopeRuleIDs` inclusion). Reference this Task as prerequisite.
  - Optional: add E2E row in CP-35 matrix for `r-additive-tests` (greenwash catch).

## 5. Touched Areas

- files (expected):
  - `apps/local-runner/internal/flowgate/rules.go` (new `r-additive-tests`)
  - `apps/local-runner/internal/flowgate/evaluate.go` (new `pre_existing_test_edited` case)
  - `apps/local-runner/internal/flowgate/enforce.go` (new `remediationFor` case)
  - `apps/local-runner/internal/runner/gate_hook.go` (remove synthetic `r-tamper` warn append)
  - `apps/local-runner/internal/runner/interactive_service.go` (Chat Plan+Code `safe-fix-contract` pointer merge in `runTurn`)
  - `apps/local-runner/internal/flowgate/*_test.go` (new cases; prefer new file `r_additive_tests_test.go`)
  - `apps/local-runner/internal/runner/*_test.go` (new Chat Plan/Code inject tests; prefer new file `chat_plan_code_safefix_inject_test.go`)
  - `requirements/06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md` (§2.8 + rule table)
  - `requirements/08-Task/todo/Task-260-R-Additive-Tests-Gate-Rule.md` (this file)
- modules: `flowgate`, runner gate hook + `runTurn` skill merge, chat posture
- routes: none (post-turn gate + in-process prompt merge only)
- tables: none (override store is file-based `.flowpilot/guard/test_overrides.json`; no schema change)

## 6. Acceptance Check

- `go test ./internal/flowgate/...` green with new `r-additive-tests` tests; `go test ./internal/runner -run TestChatPlanCode` green.
- Turning a pre-existing test file `M` with no override → gate **reprompt** (enforce mode), not silent complete; `EnforceResult` detail lists path(s) and cites `safe-fix-contract` / `additive-tests-only`.
- Adding only a **new** `*_test.go` / `*.test.ts` (`A`) → `r-additive-tests` does **not** fire.
- Overridden `filepath.Base(path)` (user-approved) → `r-additive-tests` does **not** fire for that file on next turn.
- Chat `plan` posture turn and chat `code` posture turn (both `runKind==chat`, `!flowEngineDriven`) → outbound prompt contains `## Selected Skills` + `/safe-fix-contract` pointer (path + 1-line desc); `scan` / `non` / Flow-driven turns → no auto pointer; pre-selected `safe-fix-contract` → no duplicate.
- Flow Mode coding child with non-empty diff: `r-additive-tests` **not** added to tier-1 `DocScopeRuleIDs` in this task (CP-58 owns it) — document tier placement explicitly.
- Skill `safe-fix-contract` + `additive-tests-only` still present in common skillpack; gate is hard backstop, not a replacement for the skill text.
- No double-warn from leftover synthetic `r-tamper` append; `SD-20` §2.8 describes `r-additive-tests`.

## 7. Out of Scope

- Hub session park while flow children are `RUNNING` (Review Loop concurrent hub gate fix / `run-9437` class) — separate task.
- Changing UI spawn `wait` / `dependsOn` semantics.
- Flow plan/coder skill wiring and `DocScopeRuleIDs` promotion for `r-additive-tests` — **noted in CP-58**, not implemented here.
- AST-level “only changed assertion X” detection; shared test helpers / `testutil` / fixtures treated as non-test surface.
- Auto-approving legacy test edits or weakening `r-reg` / `r-tests` always-block spirit.
- Full Task-155 decision-card UI (reuse `test_overrides.json` pattern; full card not required for v1 if reprompt + documented override is enough).
- New skill files or embedding full `safe-fix-contract` content into prompts (pointer only).

## 8. Completion Notes

- result: **implemented 2026-08-31** — Chat Plan+Code `safe-fix-contract` pointer (`runner/chat_posture.go` + `interactive_service.go` `runTurn` merge, all providers via `injectSelectedSkills`) + `r-additive-tests` gate (`flowgate/rules/evaluate/enforce`, `gate_hook` `TamperedTestPaths` defensive copy `append(nil, oracle.Tampered...)`, `gate_hook.go:348` `default enforce` comment) ; no Flow child wiring (CP-58).
- verification: `go vet ./internal/flowgate ./internal/runner` clean; `go test ./internal/flowgate -run TestRAdditive` 11 PASS; `go test ./internal/flowgate` PASS (incl. `task260_oracle_override_test.go` `TestTask260OracleOverrideClearsTamper`); `go test ./internal/runner -run TestMergeChat|TestShouldAuto|TestInjectSelected` 11 PASS; `go test ./internal/runner -run TestTask260GateWire` 6 PASS (wire `oracle→TamperedTestPaths` defensive copy, `enforce=reprompt` vs `warn=warn` parity with `r-ca`, override clears wire, pure `A` no fire). `TestRootFlowEngineDefersCompletedUntilGate` FAIL is pre-existing on `origin/flowpilot-opencode` (not from this change).
- follow-ups / residuals:
  - CP-58: Flow `preflight_contract_plan` / `plan_writer` / `plan_reviewer` / `cp_plan_writer` auto-pointer + coding-child `r-additive-tests` in `DocScopeRuleIDs` + dual back-edge wiring
  - Hub park / no gate re-enter hub while delegate child `RUNNING`
  - Optional promote `r-additive-tests` to `block` when greenwash — not in v1
- upstream docs updated: `SD-20` §1 pipeline + §2.8 `r-additive-tests` (2026-08-31); `CP-58` Related Documents + note (2026-08-31); `change-audit/CA-695-task-260-r-additive-tests-gate.md` (2026-08-31)
