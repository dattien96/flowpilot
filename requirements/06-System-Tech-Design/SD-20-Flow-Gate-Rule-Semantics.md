# SD-20: Flow Gate Rule Semantics

## Metadata

- Document ID: `SD-20`
- Title: `Flow Gate Rule Semantics`
- Phase: `tech_design`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-24`
- Last Updated: `2026-08-31`
- Parent Documents: [SD-17: Context And Regression Engine](./SD-17-Context-And-Regression-Engine.md)
- Child Documents: [CP-35: Context And Regression Engine Rollout](../07-Coding-Plan/done/CP-35-Context-And-Regression-Engine-Rollout.md)
- Related Documents: [SD-16: Agent Spawn And Tool-Calling Design](./SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [SS-14: Code Context And Regression Safety](../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [Task-155: Regression Block Decision Card (r-reg)](../08-Task/todo/Task-155-update-r-reg.md), [Task-156: Regression Oracle — Polyglot Signal And Baseline Cost](../08-Task/todo/Task-156-R-Test-Performance.md), [Task-157: Feature-Key Accuracy For History Context](../08-Task/done/Task-157-Improve-Context-Hardness.md)
- Replaces: `None (expands SD-17 §3.5 / §6.3 / Flow Gate rules table)`
- Tags: `flow-gate, regression, oracle, rules, enforcement, local-runner, change-audit`

## AI Quick View

### Summary

- SD-17 introduced the Post-Step Flow Gate and listed five rules (`r-ca`, `r-bug`, `r-tests`, `r-reg`, `r-dep`) at a glance. This document is the **exact, code-level contract** for each rule — now six rules including `r-task` (Task-113) — covering trigger condition, signal, required output, action, and `gate_mode` effect.
- The gate runs **once per turn** in the runner after `finishTurn`, observes the turn's git diff + test outcome, evaluates every enabled rule, and resolves a single highest-severity action (`approve` < `warn` < `reprompt` < `block`).
- Two rules (`r-tests`, `r-reg`) are **always-block** — never downgraded by `gate_mode` — because a previously-green test going red is the strongest, cheapest regression signal we have.
- `r-tests` and `r-reg` are **coupled in v1**: both fire on the identical condition (`Tests.Ran && len(Failed) > 0`) with an identical detail string. The engine dedupes the message and treats it as one hard stop.
- The regression oracle requires a **green baseline captured before the turn ran**. The baseline is captured once at first turn-start (`ensureBaseline`), not lazily inside the gate — otherwise the broken state would be captured as "green" and no regression would ever be detected.
- **Known performance cost (deferred):** the gate runs the project's full test suite on **every turn** to detect regressions. On large/slow suites this is expensive and will need an opt-in / scoped / cached strategy later (§5).
- **Planned hardening (not yet implemented):** three drafted tasks evolve this contract — `Task-155` turns the `r-reg` block from a dead-end modal into a 3-option decision card (§2.4, §3); `Task-156` makes the oracle polyglot (exit-code signal) and the baseline HEAD-keyed/affordable (§2.4, §4, §5, resolving `Q-1`/`Q-2`); `Task-157` may add a 7th rule enforcing the commit `[feature]` key (§2.9). Sections below mark each affected point inline.

### Current Ask

- Provide an unambiguous, implementation-accurate description of each flow-gate rule so that behavior, tests, and UX (reprompt vs. block-modal) are consistent and reviewable, and record the regression-test performance concern as an explicit deferred item.

### Key Decisions

- `D-1` Each rule is a pure function of one `TurnResult` (final message, git diff, test outcome); rules never call providers and never mutate the repo.
- `D-2` `r-tests` and `r-reg` are always-block regardless of `gate_mode`; `r-ca`, `r-bug`, `r-dep` honor `gate_mode` (enforce → their declared action; warn → downgraded to `warn`).
- `D-3` `r-ca`, `r-bug`, and `r-task` are **auto-remediable**: on violation the gate reprompts the AI (≤2 attempts) with the missing requirement. `r-tests`/`r-reg` are **not** auto-remediable — they are a hard stop surfaced to the user as a modal. (`r-bug` was initially declared `block`; corrected to `reprompt` — BUG-139.) **→ Planned (Task-155):** the `r-reg` hard stop becomes a *user-resolved decision card* (keep test → fix code · suggest requirement change → human agrees → test unlocks · custom), not a dead-end modal; severity stays always-block.
- `D-4` The test baseline is captured once, **before** the first turn executes, and reused for the session; the gate only ever *loads* it. **→ Planned (Task-156):** supersede "capture once" with a HEAD-SHA + dirty-tree-keyed baseline that is re-captured only when the tree changed (per chat, not per turn), so a legitimate committed behavior change refreshes the baseline (`Q-2`) without re-running an unchanged suite.
- `D-5` `r-tests`/`r-reg` coupling is accepted for v1; the emitted message is deduped so the user sees one line, not two.
- `D-6` Running the full suite per turn is the v1 regression mechanism; its cost is a known trade-off recorded here for a later pass (scoped/affected-tests-only, caching, or opt-in). **→ Planned (Task-156):** the regression signal moves to the suite **exit code** against an explicit per-project `test_command` (universal across Go/Node/Python/Android/iOS/…), with named-test granularity only where a structured format is parseable; scope-to-changed-packages + async capture address the cost.
- `D-7` **Flow Mode three-tier gate (Task-238 `T-9` / Task-242).** Rules run by *nature*, not only on the hub turn:
  | Tier | When | Rules | Action mapping |
  |---|---|---|---|
  | 1 — step self-gate | `agent.delegate` child turn with non-empty git diff | `r-ca`, `r-fk`, `r-bug`, `r-task`, `r-contract`, `r-scope` | reprompt routes to **same child** (`startTurn` on that run; ≤2). Empty-diff (reviewer) → no-op (BUG-152). |
  | 2 — tests/reg | Flow has `command.validate` → that node owns suite; else coding child turn (review-loop fallback) | `r-tests`, `r-reg` | Always-block spirit preserved: validate uses bounded retry (cap 3 → escalate, Task-170); child fallback maps block → parent `applyFlowControl(escalate)` so the hub is actionable. |
  | 3 — audit aggregate | `artifact.audit_draft` node before draft settle | doc family on **flow-start HEAD → now** aggregate diff | Violations escalate (do not `done`); `git commit` is **tool-denied** on coding children (reserved for audit/commit-prep, CP-41 P-6). |
  Chat Mode (`parentRunID == ""`) still runs full `runFlowGate` as before (`D-1`..`D-6`).
- `D-3` **nới cho Flow Mode (Task-242):** always-block of `r-tests`/`r-reg` in Flow Mode is realized as **bounded-retry-then-escalate** (validate) or **escalate-to-hub** (no-validate fallback), still ending at human stop — not silent auto-pass and not an unanswerable child modal.

### Constraints

- Must remain provider-agnostic: the gate runs in the runner after every turn (`SD-16`), independent of Claude/Codex/Gemini.
- Must be non-fatal: any internal error inside the gate degrades to "pass" (turn completes normally).
- Must not edit or delete tests to make them pass (oracle integrity, `SS-14 AC-6`).

### Open Questions

- `Q-1` Regression scope: run the full suite, only tests in changed packages, or only tests reachable from changed symbols (needs GitNexus)? **→ Being settled by Task-156.**
- `Q-2` Baseline refresh: when should a stale baseline be re-captured (e.g. after the user legitimately changes behavior and commits)? **→ Being settled by Task-156 (HEAD-SHA + dirty-tree keyed).**
- `Q-3` `r-dep` precision: v1 fires on any deleted `.go` file; should it gate on actual remaining callers (GitNexus) before blocking?
- `Q-4` Feature-key enforcement: should a missing/unregistered commit `[feature]` key become a formal gate rule (§2.9)? **→ Being explored by Task-157.**

### Source Refs

- `SD-17 §3.5` two-layer enforcement, `§6.3` flow rule + gate, Flow Gate rules table, `§7.2` regression suite + oracle rule, `§13.8` gate execution.
- Code: `internal/flowgate/{rules,evaluate,enforce,observe,oracle,baseline}.go`, `internal/runner/gate_hook.go`, `interactive_service.go` (`ensureBaseline`, turn lifecycle).

## 1. Where the gate runs

The gate is a single post-turn hook (`runFlowGate` in `gate_hook.go`), invoked from `interactive_service.go` after `finishTurn` returns a clean completion:

```
runTurn:
  ① ensureBaseline(cwd)        // before the AI runs — capture green baseline once
  ② captureGitHead(cwd)        // snapshot HEAD before the AI runs
  ③ sendTurnWithRetry(...)     // the AI turn
  ④ finishTurn(...)            // emits turn_completed; completed=true
  ⑤ if completed && runFlowGate(...) { completed = false }   // gate may suppress
  ⑥ if completed { finalizer.Finalize(...) }
```

`runFlowGate` steps:

1. `ObserveGitDiffSince(cwd, baseSHA)` — committed (since turn-start HEAD) **plus** uncommitted changes, deduped by path. Untracked files are listed individually (`git status --porcelain -uall`) and treated as `Added`.
2. `LoadBaseline(dotFP)` — load `.flowpilot/guard/test_baseline.json`. **No lazy capture here** (see §4).
3. `RunOracle(cwd, baseline, diff)` — runs the suite, classifies regressions and tampering (`oracle.Tampered` = `IsTestFile` `M/D/R/C` filtered by `test_overrides.json`).
4. Build `TurnResult{FinalMessage, GitDiff, Tests{Ran, Failed}, TamperedTestPaths: oracle.Tampered}`.
5. `LoadRules(settings/)` or `DefaultRules()` (now includes `r-additive-tests`, Task-260).
6. `Evaluate(tr, rules)` → `[]Violation` (including `r-additive-tests` when `TamperedTestPaths` non-empty).
7. `Enforce(violations, gateMode)` → one resolved `Action` + a deduped `Message`.
8. Emit `flow_gate_violation` (carrying `error` = message, `status` = resolved action).
9. Route: `block` → suppress completion; `reprompt` → relaunch a turn (≤2) and suppress; `warn`/`approve` → let the turn complete.

## 2. The rules (exact semantics)

All triggers are evaluated in `checkRule` (`evaluate.go`). Signals come from `observe.go`/`oracle.go`.

### 2.1 `r-ca` — change-audit note required

| Field | Value |
|---|---|
| Trigger | `code_changed` |
| Fires when | `HasCodeChanges(diff) && !HasChangeAuditNote(diff)` |
| Required output | a `change-audit/CA-*.md` file (status `A` or `M`) |
| Action | `reprompt` (auto-remediated, ≤2 attempts) |
| `gate_mode` | enforce → reprompt; warn → downgraded to `warn` |

- **`HasCodeChanges`**: any changed file that is **not** a doc/audit file. `IsDocOrAuditFile` = path under `requirements/`, under `change-audit/`, or ending in `.md`.
- **`HasChangeAuditNote`**: any changed file whose path contains `change-audit/CA-` with status `A`/`M`.
- Common false-negative (fixed): if git collapses a new untracked directory to `change-audit/`, the note is invisible. `-uall` expands it to the individual `change-audit/CA-<id>.md` path.

### 2.2 `r-bug` — bugfix doc required

| Field | Value |
|---|---|
| Trigger | `bug_fixed` |
| Fires when | `isBugFix && !HasBugFixDoc(diff)` |
| Required output | a file under `requirements/09-BugFix/` or containing `BUG-` in its path |
| Action | `reprompt` (auto-remediated, ≤2 attempts) |
| `gate_mode` | enforce → reprompt; warn → downgraded to `warn` |

- **`isBugFix`**: `ChangeType == "bugfix"` **or** the AI's final message contains `"fixed bug"` / `"bug fix"` (case-insensitive).
- v1 relies on the final-message heuristic; `ChangeType` is reserved for a future explicit signal.

### 2.3 `r-tests` — tests must be green

| Field | Value |
|---|---|
| Trigger | `tests_failed` |
| Fires when | `tr.Tests.Ran && len(tr.Tests.Failed) > 0` |
| Required output | tests green (or an explained exception) |
| Action | `block` — **always** (not `gate_mode`-gated) |

- `Tests.Ran` is true whenever a baseline exists. `Tests.Failed` is the set of **regressed** tests from the oracle (green-in-baseline, red-now, not from a changed test file).
- **Current coverage gap (→ Task-156):** a baseline only exists when `DetectTestRunner` resolves a runner (Go/Node/Python) and `oracle.go` can parse its stdout. On Android/iOS/Java/Rust/other projects no baseline is captured → `Tests.Ran=false` → `r-tests`/`r-reg` **never fire**, so the regression guarantee is silently absent there. Task-156 makes the signal exit-code-based and honors an explicit `test_command` to close this.

### 2.4 `r-reg` — no regressions

| Field | Value |
|---|---|
| Trigger | `regression_test_broke` |
| Fires when | `tr.Tests.Ran && len(tr.Tests.Failed) > 0` |
| Required output | restore green by fixing the code, never by weakening the test |
| Action | `block` — **always** |

- **Coupled with `r-tests` in v1**: identical condition, identical detail. `Enforce` dedupes the detail so the message reads once (`Flow gate: Tests failed: TestAdd`), not twice. Both being always-block means the resolved action is `block` either way.
- The distinction the names imply (a generic failing test vs. a *previously-green* test now red) collapses in v1 because the oracle only surfaces regressions in `Failed`. Decoupling is a future enhancement (`Q-1`).
- **→ Planned (Task-155 + Task-156):** `r-reg` keeps `block`, but (a) the block is resolved through a 3-option decision card (§3), and (b) the underlying signal becomes exit-code-based so `r-reg` actually fires on non-Go/Node/Python targets (today it cannot — see §2.3 note). When the user accepts a requirement change, a per-test override (under `.flowpilot/guard/`) prevents that specific test from re-blocking/re-flagging on the next pass.

### 2.5 `r-dep` — removed referenced code

| Field | Value |
|---|---|
| Trigger | `removed_referenced_code` |
| Fires when | any changed file with status `D` whose path ends in `.go` |
| Required output | confirm the removal or update callers |
| Action | `block` |
| `gate_mode` | enforce → block; warn → downgraded to `warn` |

- v1 is intentionally coarse: it does **not** consult the call graph. Despite the name it does not yet verify remaining callers; that requires the GitNexus structure provider (`SD-17 D-5`, `Q-3`).

### 2.6 `r-task` — task doc required (Task-113)

| Field | Value |
|---|---|
| Trigger | `task_referenced` |
| Fires when | `taskIDRegex.MatchString(FinalMessage) && !HasTaskDoc(diff)` |
| Required output | a file under `requirements/08-Task/` or containing `Task-` in its path |
| Action | `reprompt` (auto-remediated, ≤2 attempts) |
| `gate_mode` | enforce → reprompt; warn → downgraded to `warn` |

- **`taskIDRegex`**: `regexp.MustCompile(`\bTask-\d+\b`)` — word-boundary anchored so `Task-113` matches but `MyTask-113` does not.
- **`HasTaskDoc`**: any file whose path contains `requirements/08-Task/` or `Task-`, **excluding** paths that contain `FORMAT-REFERENCE-` (scaffold templates must not satisfy the predicate — same guard as `HasBugFixDoc`, BUG-141).
- v1 relies on the final-message heuristic; `ChangeType == "task"` is reserved for a future explicit signal (see §2.7).

### 2.7 `r-task` / `r-bug` — v1 heuristic limitation and how `FinalMessage` is sourced

**The detection chain (exact code path):**

```
Provider stream (Claude/Codex)
  → adapter emits EventTurnCompleted{FinalMessage: raw["result"]}   // claude_event_mapper.go:135
  → interactive_service.go:1827 copies e.FinalMessage into finalizeInput.FinalMessage
  → runFlowGate(fin) puts fin.FinalMessage into TurnResult.FinalMessage  // gate_hook.go:58
  → checkRule("task_referenced") runs taskIDRegex.MatchString(tr.FinalMessage)  // evaluate.go
```

`FinalMessage` is the **complete text of the AI's last assistant turn** — the full prose the user sees at the end of the step. For Claude it comes from the `result` field of the SDK's terminal result frame. For Codex it is assembled from the last `message.completed` event.

**The gap:** both `r-task` and `r-bug` scan this text for a keyword or ID. If the AI writes a silent summary — "Done." or "Added the comment." — without mentioning `Task-NNN` or "bug fix", the rule never fires. The gate has no other signal for these rules in v1.

**Why this is acceptable for v1:** the gate is an *enforcement layer*, not a tracking layer. Its job is to catch the case where the AI explicitly acknowledges closing a tracked item but forgets the required artifact. Silent completions are a separate problem (workflow design, step prompts) outside the gate's scope.

**Future fix — `ChangeType` explicit signal:** when the runner or step definition knows the current step maps to Task-113, it will stamp `TurnResult.ChangeType = "task"` (already checked in `evaluate.go` as `tr.ChangeType == "task"`). That makes the rule fire regardless of what the AI says, removing the message-scanning dependency entirely.

### 2.8 `r-additive-tests` — pre-existing test edited (Task-260, replaces `r-tamper` warn)

| Field | Value |
|---|---|
| Trigger | `pre_existing_test_edited` |
| Fires when | `oracle.Tampered` non-empty → `TurnResult.TamperedTestPaths` non-empty (i.e. `IsTestFile(path) && status ∈ {M,D,R,C}` filtered by `test_overrides.json` `filepath.Base(path)`). Pure `A` (new test file) never fires. No `GitDiff` fallback — `TamperedTestPaths` is authoritative; `GitDiff` `M/D/R/C` filtered by an override must not re-fire. |
| Required output | no unapproved legacy test mutation; new tests only **or** recorded user approval (`test_overrides.json` per-file, Task-155). |
| Action | `reprompt` (auto-remediated, ≤2 attempts), `gate_mode`-gated like `r-ca` — not `warn`, not always-`block`. |
| `gate_mode` | enforce → reprompt; warn → downgraded to `warn` |

- Replaces the synthetic `r-tamper` warn (hardcoded `warn` append in `gate_hook.go`): the signal now owns a first-class `DefaultRules` entry so the gate reprompts with a skill-citing remediation instead of completing silently.
- Historical alias: `r-tamper` (`oracle_tamper` warn) remains documented as the pre-Task-260 name; no code emits it after Task-260.
- Chat Plan/Code init (also Task-260): `safe-fix-contract` pointer is auto-merged into `SelectedSkills` on Chat `plan`/`code` posture turns before `promptPrep` (`injectSelectedSkills` pointer, not full content). Flow plan/coder wiring is **noted in CP-58** only.

### 2.9 `r-commit` — feature-key required (Task-157)

| Field | Value |
|---|---|
| Trigger | `commit_feature_key_missing` |
| Fires when | a code-changing turn's commit(s) lack a `[feature]` bracket or use a key not in `change-audit/FEATURE-KEYS.md` |
| Required output | a commit whose `[feature]` is a registered key (register the new key first) |
| Action | `reprompt` (auto-remediated, ≤2 attempts), `gate_mode`-gated like `r-ca` |

- The `[feature]` contract is now enforced by the runner gate signal as well as the soft `git-commit-format` skill; the ledger's `feature_key` accuracy (and therefore the history-context value, `SS-14 AC-3`) depends on the AI following it. Task-157 promotes it to a runner gate signal (the runner is the source of truth, `SS-14 BR-6`) and feeds a `featurecatalog.SuggestKey` candidate into the reprompt.

## 3. Enforcement resolution & UX

`Enforce` (`enforce.go`) collapses all violations into one result:

- Per violation, the effective action is: `block` if `isAlwaysBlock(trigger)` (`tests_failed` / `regression_test_broke`); else if `gate_mode == "warn"` then `block`/`reprompt` → `warn`; else the rule's declared action.
- The result `Action` is the **highest severity** across violations (`approve` < `warn` < `reprompt` < `block`).
- The result `Message` is `"Flow gate: " + join(unique details)` — identical details are deduped (the `r-tests`/`r-reg` case).

Desktop UX (driven by the `status` field on `flow_gate_violation`):

| Resolved action | Inline timeline | Modal | Chat status |
|---|---|---|---|
| `reprompt` (`r-ca`, `r-bug` in enforce) | amber ⚠ warn card | — | stays running; a `turn_started` for the reprompt follows |
| `warn` | amber ⚠ warn card | — | settles to completed |
| `block` (`r-tests`/`r-reg`, others in enforce) | amber ⚠ warn card | **hard-stop modal** the user must acknowledge | settles to completed (composer unblocks) |

A `block` is a hard stop: there is **no** auto-reprompt. The modal explains the failing tests and that the fix must make them pass without editing the tests.

**→ Planned (Task-155):** for `r-reg` the acknowledge-only modal is replaced by a **decision card** with three choices, each driving the next turn rather than just dismissing:

| Option | Next action | Test editable? |
|---|---|---|
| Keep test + requirement → fix code (default) | reprompt: restore green by fixing the code | no (tampering still fires) |
| Suggest requirement changes | AI proposes the `SS`/`SD`/requirement change for review; **only on explicit user agreement** is a per-test override recorded, the upstream doc updated, and the test aligned | only after agreement |
| Custom | user free-text becomes the next-turn instruction | n/a |

The chosen option is recorded for audit. This realizes the `SS-14 AC-6`/`E-6` human-confirm path; it does not loosen `r-reg` severity (still always-block) and never silently rewrites a test.

## 4. Baseline lifecycle (why capture before the turn)

The oracle can only call a test "regressed" if it was green **before** the change. Therefore the baseline must reflect pre-change state:

- `ensureBaseline(cwd)` runs once, **before the first turn executes** (`interactive_service.go`, ahead of `sendTurnWithRetry`). If `test_baseline.json` already exists it is a no-op.
- The gate **only loads** the baseline; it never captures one. (An earlier lazy-capture-in-gate bug captured the *already-broken* state as the baseline, so the broken test was recorded as never-green and no regression was ever detected — E2E-8 failure.)
- `DetectTestRunner` finds the runner at the repo root or, for monorepos, the best nested runner (e.g. a nested `go.mod` under `apps/local-runner/`); `TestDir` records where to run it. The baseline stores `{GreenTests, TestCmd, TestDir}`.
- The baseline is local-only (`.flowpilot/guard/`), machine-specific, never Drive-synced (`SD-17 §5.1`).

**→ Planned (Task-156):** the baseline additionally stores the `head_sha` + dirty-tree marker it was captured at, plus `test_command`/`test_dir`/`result_format`. `ensureBaseline` refreshes it when HEAD/working-tree differs from the stored marker (still never lazily inside the gate's failing state), runs per chat rather than per turn, and captures async so it never blocks finalize. This keeps the "captured from a known-good state" invariant while fixing staleness (`Q-2`) and replacing the strict capture-once rule (`D-4`).

## 5. Performance concern (deferred — revisit)

> **This is the explicit "come back later" item.**

`RunOracle` executes the project's **entire** test suite on **every turn** (capture once at start, then run on each gate evaluation). For small suites this is fine; for large or slow suites it adds the full suite runtime to every single turn, which is not acceptable as a default.

Why it is this way in v1:

- It is the cheapest *correct* regression signal — no code graph, no per-test selection logic.
- It is provider-agnostic and deterministic.

Why it must change later (options, not yet chosen — `Q-1`):

- **Scope to changed packages** — run only the packages touched by the diff (cheap, language-aware; misses cross-package regressions).
- **Affected-tests-only via GitNexus** — run only tests reachable from changed symbols (precise; depends on the structure provider being present and fresh).
- **Result caching / incremental** — reuse the last green result for untouched packages.
- **Opt-in / budgeted** — a `gate_mode` or per-project setting that disables per-turn full-suite runs, or runs them only on commit / on demand.
- **Async / non-blocking** — run the suite out of band and surface a late regression card instead of gating the turn synchronously.

Until one of these lands, treat per-turn full-suite execution as a known cost, document it for users with large suites, and prefer a fast nested runner (`TestDir`) over a slow root suite where possible.

**→ Resolution in progress (Task-156):** the chosen v2 direction is (a) exit-code signal against an explicit `test_command` for universality, (b) HEAD-keyed baseline refresh to avoid re-running an unchanged tree, (c) scope-to-changed-packages where the ecosystem allows, and (d) async/non-blocking capture. Task-156 settles `Q-1` (scope) and `Q-2` (refresh trigger).

## 6. Traceability

- `SD-17` Flow Gate rules table / `§6.3` → §2 (exact per-rule contract).
- `SD-17 §7.2` oracle rule → §2.3/§2.4/§2.8, §4.
- `SD-17 D-12` (pre-existing tests always block) → `D-2`, §2.3/§2.4.
- `SS-14 AC-6` (oracle integrity) → §2.4, §3 (no test-weakening), §2.8.
- `SS-14 AC-11` (force required outputs) → §2, §3.
- New: regression performance trade-off → §5 (`D-6`, `Q-1`).
- Planned hardening: `Task-155` (regression decision card) → `D-3`, §2.4, §3; `Task-156` (polyglot signal + baseline cost) → `D-4`, `D-6`, §2.3, §2.4, §4, §5, `Q-1`, `Q-2`; `Task-157` (feature-key gate) → §2.9, `Q-4`.
