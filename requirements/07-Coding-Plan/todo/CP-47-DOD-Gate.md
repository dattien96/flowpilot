# CP-47: Definition-of-Done Flow Gate (`r-dod`)

## Metadata

- Document ID: `CP-47`
- Title: `Definition-of-Done Flow Gate (r-dod)`
- Feature Keys: `context-regression-engine`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-14`
- Last Updated: `2026-07-14`
- Parent Documents: [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md)
- Child Documents: `None`
- Related Documents: [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md) (defines the `Definition of Done` section this gate reads), [CP-43: Change Contract And Canonical Intent Signature](./CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (sibling gates `r-scope`/`r-spec-drift`), [Task-223: File-Artifact Output Contract](../../08-Task/done/Task-223-File-Artifact-Output-Contract-And-Review-Input-Chain.md) (prior art: required-output gate `r-artifact-output`); `flowgate` rule registry (`apps/local-runner/internal/flowgate/rules.go`), skills `add-new-task`, `add-new-bug`, `phase-document-compliance`
- Replaces: `None`
- Tags: `context-regression-engine, flow-gate, flowgate, definition-of-done, dod, task, bugfix, r-dod`

## AI Quick View

### Summary

- Add a **flow-gate rule family `r-dod`** to the existing `flowgate` registry so that task/bug work is anchored to a checkable Definition of Done, not just prose.
- Two enforcement moments (mirrors the `r-artifact-output` / `r-artifact-output-structure` split):
  - **`r-dod-present`** — when a turn creates or edits a `Task-*`/`BUG-*` doc for the work being done, that doc must contain a `## Definition of Done` section with at least one checkbox item (`- [ ]`). Nag (reprompt) until it does.
  - **`r-dod-complete`** — when the work is marked **done** (doc `Status: done` / moved to `done/`), every DOD checkbox must be `[x]`. If any remain `[ ]`, the turn must **explain** which items are unfinished and why (an "or-explained" escape, exactly like `r-tests`), otherwise **block** the done transition.
- Deterministic markdown parse of the DOD checkboxes — zero LLM cost, reuses the checkbox convention every phase doc already uses (`SS-13`, this file's own §10).

### Current Ask

- Land `r-dod-present` + `r-dod-complete` in `flowgate.DefaultRules()` with detectors, a DOD-section parser, evaluate wiring, and the reprompt/block/explain UX — behind the same enable/merge machinery as the existing rules.

### Key Decisions

- `D-1` **Reuse `flowgate`, do not build a parallel gate.** `r-dod` is two more `Rule{ID,Scope,Trigger,RequiredOutput,Action,Enabled}` entries in `DefaultRules()`, evaluated by the same `Evaluate`/`checkRule` path as `r-ca`/`r-tests`/`r-artifact-output`. `MergeDefaultRules` back-fills them into older `flow-rules.json` so they are never silently off.
- `D-2` **"Done" signal = the doc's own status, not a guess.** `r-dod-complete` fires when this turn transitions a governing `Task-*`/`BUG-*` doc to done: `Status: done` in its metadata block **or** the file moved into a `.../done/` folder (both already meaningful in the repo layout). No completion heuristic on commit messages.
- `D-3` **`present` nags, `complete` blocks-or-explains.** `r-dod-present.Action = reprompt` (like `r-bug`/`r-task`: get the DOD written, never hard-stop). `r-dod-complete.Action = block` with `RequiredOutput = dod_all_checked_or_explained` (like `r-tests` `tests_green_or_explained`): all boxes checked → pass; unchecked boxes **with** an explanation this turn → pass with warning; unchecked boxes **without** explanation → block the done transition.
- `D-4` **DOD parse is deterministic and shared.** One `parseDefinitionOfDone(md)` helper returns `{present bool, total int, checked int, openItems []string}` by reading the `## Definition of Done` heading and its `- [ ]`/`- [x]` list — used by both rules and by `phase-document-compliance`. `[x]` and `[X]` both count as done; nested/indented boxes count.

### Constraints

- Deterministic + offline: the detector never calls an LLM (it is a gate that runs every turn).
- Non-invasive to existing rules: additive entries only; no change to `r-ca`/`r-tests`/`r-reg` semantics or ordering.
- Respect the doc contract: `r-dod` reads the `## Definition of Done` section exactly as `SS-13` defines it; it does not invent a new location or syntax.
- Never auto-check a box for the user — the gate only reads DOD state and reprompts/blocks/explains; marking done stays the agent's/human's explicit act.

### Open Questions

- Scope of `r-dod-present`: only `Task-*`/`BUG-*` docs, or any governing doc (SS/SD/CP) that has a DOD/acceptance section? (Leaning: Task/BugFix first — that is the "làm task/bug" case; CP acceptance is a later extension.)
- What counts as a valid "explanation" for `r-dod-complete`? A per-item note beside the unchecked box, a dedicated `Open items` subsection in the doc, or a statement in the turn's final message — or any of the three? (Leaning: any of the three, matching how `r-tests` accepts an explanation in the final message.)
- Should marking a doc done with **zero** DOD items (section present but empty) be a `present` failure or a `complete` failure? (Leaning: `present` — an empty DOD is really a missing one.)

### Source Refs

- `flowgate/rules.go` `DefaultRules()` (the registry `r-dod` is added to; `r-tests` = the "or-explained" template; `r-artifact-output` = the "required output present" template).
- `flowgate/evaluate.go` `Evaluate`/`checkRule` (how a `Trigger` maps to a `Violation` and an `Action`).
- `SS-13` `Definition of Done` section definition + the `- [ ]`/`- [x]` checkbox convention.
- Skills `add-new-task`/`add-new-bug` (which already create the `Task-*`/`BUG-*` doc this gate expects a DOD in).

## 1. Goal

Make "the work is done" a **checked, gate-enforced fact** rather than an unverified claim: every task/bug carries a Definition of Done in its doc, and the flow cannot mark that work done while DOD items are still open unless it explicitly explains why — reusing the existing `flowgate` engine so the rule is configurable, mergeable, and evaluated on the same path as `r-ca`/`r-tests`.

## 2. Input Documents

- design source: `SD-17` (context + regression engine — the flow gate this rule joins).
- contract: `SS-13` (defines the `## Definition of Done` section + checkbox syntax the detector parses).
- prior art: `flowgate` rules `r-tests` (block-or-explained), `r-bug`/`r-task` (required doc present), `r-artifact-output`/`r-artifact-output-structure` (required output present + shape) — `r-dod` is the same pattern applied to a doc's own DOD checkboxes.

## 3. Implementation Strategy

- overall approach: add `r-dod-present` and `r-dod-complete` to `flowgate.DefaultRules()`. Add a shared `parseDefinitionOfDone` markdown helper. In the caller that builds a `TurnResult`, compute two new signals — `DodPresent`/`DodItemTotal`/`DodItemChecked` for the governing doc(s) touched this turn, and `DodTransitionedToDone` (a `Task-*`/`BUG-*` doc whose `Status` became `done` or which moved into `done/`) — then let `Evaluate` fire the rules.
- sequencing logic: ship the parser + `r-dod-present` (reprompt) first — low-risk, purely additive nagging. Then `r-dod-complete` (block-or-explained) once the "done transition" signal and the explanation-detection are proven, since that one can stop a flow.
- dependencies: the `flowgate` `Rule`/`TurnResult`/`Evaluate` machinery already exists; `MergeDefaultRules` already back-fills new default rules into stored `flow-rules.json`. The DOD checkbox convention is already universal across `requirements/`.

## 4. Work Breakdown

- `P-1` **DOD parser (deterministic).** `parseDefinitionOfDone(md string) DodStatus{Present bool; Total int; Checked int; OpenItems []string}`: locate the `## Definition of Done` heading (case-insensitive, tolerate `Definition of Done`/`DoD`), read its bullet list until the next heading, count `- [ ]` vs `- [x]`/`- [X]` (including indented), collect open-item label text. Unit-tested against real repo docs as fixtures.
- `P-2` **`r-dod-present` rule (reprompt).** Registry entry `{ID:"r-dod-present", Scope:"step", Trigger:"task_or_bug_doc_missing_dod", RequiredOutput:"definition_of_done_section", Action:"reprompt", Enabled:true}`. Trigger computed when this turn wrote a `Task-*`/`BUG-*` doc (reuse `WrittenPaths`, not `GitDiff`) whose `parseDefinitionOfDone().Present == false` (or `Total == 0`). Violation detail names the doc + how to add the section.
- `P-3` **"done" transition signal.** In the `TurnResult` builder: set `DodTransitionedToDone` + attach the doc's `DodStatus` when a written `Task-*`/`BUG-*` doc's metadata `Status` became `done` this turn, or the file moved into a `.../done/` path. Deterministic; no commit-message parsing (D-2).
- `P-4` **`r-dod-complete` rule (block-or-explained).** Registry entry `{ID:"r-dod-complete", Scope:"step", Trigger:"marked_done_with_open_dod", RequiredOutput:"dod_all_checked_or_explained", Action:"block", Enabled:true}`. `checkRule`: if `DodTransitionedToDone` and `Checked < Total` → block, **unless** an explanation for the open items is present (per-item note, an `Open items`/`Deferred` subsection, or the turn's final message references the unfinished items) → downgrade to warn. Violation lists the specific `OpenItems`.
- `P-5` **UX surfacing.** Reprompt/decision text for both rules routed through the same gate-hook/decision-card path the other rules use (e.g. the "Flow gate: …" surface). `r-dod-complete` block presents the open-item list and the two ways forward: check the remaining items, or explain why they are deferred.

## 5. Touched Areas

- files: `apps/local-runner/internal/flowgate/rules.go` (2 new `DefaultRules` entries), `apps/local-runner/internal/flowgate/evaluate.go` (`checkRule` cases + `TurnResult` fields), a new `dod.go` (parser) + `dod_test.go`, and the `TurnResult` builder in `apps/local-runner/internal/runner/` (compute the DOD signals from written docs).
- modules: `flowgate` (rule registry + evaluate), `runner` (signal computation), optionally `phase-document-compliance` skill (reuse the same DOD-completeness check when auditing a doc).
- database: none (operates on markdown docs + turn signals).
- external systems: none.

## 6. Data or Migration Steps

- schema: none.
- data backfill: none required — `MergeDefaultRules` injects `r-dod-present`/`r-dod-complete` into any existing `flow-rules.json` on load, so older workspaces gain the rules without a migration.
- config updates: none; the rules ship enabled by default and can be disabled per-workspace via `flow-rules.json` like any other rule.

## 7. Validation Plan

- tests to add: `parseDefinitionOfDone` table tests (present/absent, empty section, all-checked, some-open, `[X]` vs `[x]`, indented boxes, DOD followed by another heading); `r-dod-present` fires only for a Task/BUG doc lacking a non-empty DOD; `r-dod-complete` blocks a done transition with open boxes and no explanation, passes (warn) with an explanation, passes clean when all checked; `MergeDefaultRules` includes both new IDs.
- manual checks: run a task via `add-new-task` without a DOD → expect the `r-dod-present` reprompt; add DOD, leave one box open, mark the doc `Status: done` → expect the `r-dod-complete` block listing the open item; add an `Open items` note explaining it → expect it to pass with a warning.
- failure cases: a doc with no DOD heading at all (present=false, not a parse panic); a DOD section with prose but no checkboxes (Total=0 → treated as missing per Open Questions); a done transition on a doc that never had a DOD (present-rule already nagged; complete-rule treats absent DOD as an open explanation requirement, not a silent pass).

## 8. Rollout and Fallback

- rollout order: `P-1`+`P-2` (parser + reprompt-only `r-dod-present`) first — additive, cannot stop a flow. Then `P-3`+`P-4`+`P-5` (`r-dod-complete`, which can block).
- fallback path: either rule can be disabled in `flow-rules.json` (`"enabled": false`) without a code change; disabling reverts to today's behavior exactly.
- monitoring: gate violations already surface in the flow transcript; a spike in `r-dod-complete` blocks signals work being marked done with open DODs (the exact behavior this gate exists to catch).

## 9. Risks

- `R-1` False "done" detection stops a flow spuriously. Mitigation: the done signal is strictly the doc's own `Status: done`/`done/`-folder move (D-2), not a heuristic; `Action:block` is escapable via explanation.
- `R-2` DOD parser mis-reads an unusual DOD layout and under/over-counts. Mitigation: parser is spec-pinned to `SS-13`'s checkbox convention + table tests over real repo docs; tolerant heading match.
- `R-3` "Explanation" acceptance is too loose (any text passes) or too strict (blocks a legitimate defer). Mitigation: settle the Open Question on what counts; start by matching `r-tests`' proven final-message-explanation acceptance and tighten only if abused.
- `R-4` Nag fatigue from `r-dod-present` on docs that legitimately have no DOD yet. Mitigation: it is a reprompt (never a block) and only fires on `Task-*`/`BUG-*` docs the turn actually wrote.

## 10. Definition of Done

- [ ] `P-1` `parseDefinitionOfDone` returns `{Present,Total,Checked,OpenItems}` deterministically for the `## Definition of Done` checkbox convention; table-tested (present/absent/empty/all-checked/some-open/`[X]`/indented); never panics on malformed input.
- [ ] `P-2` `r-dod-present` is in `DefaultRules()`, fires (reprompt) only when this turn wrote a `Task-*`/`BUG-*` doc with no non-empty DOD, and names the doc + fix in its detail.
- [ ] `P-3` The `TurnResult` builder computes a deterministic `DodTransitionedToDone` + `DodStatus` from `Status: done`/`done/`-folder moves of written Task/BUG docs — no commit-message heuristic.
- [ ] `P-4` `r-dod-complete` is in `DefaultRules()`, blocks a done transition with open DOD items and no explanation, downgrades to warn when the open items are explained, and passes clean when all boxes are `[x]`; violation lists the specific open items.
- [ ] `MergeDefaultRules` injects both new rule IDs into an older `flow-rules.json`; both are disable-able via config with no code change.
- [ ] Manual walkthrough (§7) passes: no-DOD task nags; done-with-open-box blocks; explained-open-box warns; all-checked passes.

## 11. Out of Scope

- Extending `r-dod` to SS/SD/CP acceptance/DOD sections (governing-doc completeness) — a later extension once the Task/BUG case is proven (Open Questions).
- Auto-checking DOD boxes or auto-writing the DOD section — the gate only reads, reprompts, and blocks/explains; authoring the DOD stays with the agent/human (`add-new-task`/`add-new-bug`).
- Cross-doc DOD rollups (e.g. a CP done only when all its child Tasks' DODs are done) — a separate aggregation concern, not this per-turn gate.
