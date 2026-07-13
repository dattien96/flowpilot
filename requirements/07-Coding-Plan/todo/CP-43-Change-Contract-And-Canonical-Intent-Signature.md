# CP-43: Change Contract And Canonical Intent Signature (Scope-Drift Detection)

## Metadata

- Document ID: `CP-43`
- Title: `Change Contract And Canonical Intent Signature (Scope-Drift Detection)`
- Phase: `coding_plan`
- Status: `approved`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-03`
- Last Updated: `2026-07-13` (scope confirmed Change-Contract-only — context-harness refactor absorbed by CP-44; all upstream deps CP-35/36/41/42/44 + BUG-243 done; ready to implement P-1)
- Parent Documents: [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (US-3, AC-7, AC-8)
- Child Documents: [Task-184: Change Contract Capture](../../08-Task/todo/Task-184-Change-Contract-Capture.md) (P-1), [Task-185: Scope-Drift Detection](../../08-Task/todo/Task-185-Scope-Drift-Detection.md) (P-2), [Task-186: Canonical Head And Intent Signature](../../08-Task/todo/Task-186-Canonical-Head-And-Intent-Signature.md) (P-3), [Task-187: Superseding Decision Records And Retire](../../08-Task/todo/Task-187-Superseding-Decision-Records-And-Retire.md) (P-4), [Task-188: Canonical-Head Packing And Admin Visibility](../../08-Task/todo/Task-188-Canonical-Head-Packing-And-Admin.md) (P-5)
- Related Documents: [CP-44: Pluggable Context Source Registry](./CP-44-Pluggable-Context-Source-Registry.md) (**dependency** — Canonical Head is a context source cắm vào CP-44's registry; CP-44 lands first), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md) (D-11), [CP-35: Context And Regression Engine Rollout](../done/CP-35-Context-And-Regression-Engine-Rollout.md), [CP-23: Context Control & Wrong-Way Detection](./CP-23-Auto-Learn-To-Skill.md), [CP-32: UnitTest Rule](../done/CP-32-UnitTest-Rule.md), [CP-42: Flow Pack And Generic Node Behavior Refactor](../done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) (P-8 residual only, §11; P-4 typed-context-bindings re-homed to CP-44), [BUG-243: Flow Mode Validate And Audit Behaviors Disconnected From Task-170/171](../../09-BugFix/done/BUG-243-Flow-Mode-Validate-And-Audit-Behaviors-Disconnected-From-Task-170-171.md) (**un-folded 2026-07-08** → standalone, lands first; no longer CP-43 scope)
- Replaces: `None`
- Tags: `context, regression, scope-drift, change-contract, canonical-head, intent-signature, flow-gate, gitnexus, provenance`

## AI Quick View

### Summary

- Delivers `SS-14 US-3` (declare-intended-change and flag-anything-else), the one user story `CP-35` left **deferred** (`SD-17 D-11`).
- Solves the "messy churn" problem: when a feature's code goes `A → B → C → X → A` across hundreds of commits, the raw ledger is noise. The AI should read a single **Canonical Head** (current authoritative state + why the dead-ends were rejected), not replay the churn.
- Deliberately **avoids the approach that got `D-11` deferred**: no AST-normalized hashing of code bytes (noisy on rename/format/ripple, and `A`-then-`A` hashes equal but is not semantically equal). Instead it hashes **canonical intent** (governing spec + declared behavior) and does a cheap **declared-scope vs actual-touched** set diff for drift.
- Four mechanisms, all built on `CP-35`'s existing Go modules (`changeledger`, `flowgate`, `structure`): (1) a **Change Contract** the AI declares before editing; (2) **scope-drift detection** as a post-turn gate rule; (3) a per-feature **Canonical Head** carrying an `intent_signature`; (4) **superseding Decision Records** that preserve negative knowledge and let positive churn collapse.

### Current Ask

- Give every code-mutating step a declared intent contract, flag any edit outside it, maintain one canonical statement of truth per feature that survives churn, and detect when code has drifted from its governing spec — cheaply, deterministically, and auditable per `AC-7`/`AC-8`.

### Key Decisions

- `P-1` Hash **intent**, not code. `intent_signature = sha256(feature_key + sorted governing_doc_ids + their content hashes + canonical behavior statement)`. This is the "digital signature" for integrity; code bytes are never hashed.
- `P-2` Drift = a **set difference** `actual_touched \ declared_scope`, computed from the flow gate's existing `GitDiff` (file-level always; symbol-level only when `structure.Available()`). No AST, no code-graph build in v1 — this is exactly why `D-11` was deferred and this CP does not repeat it.
- `P-3` A per-`feature_key` **Canonical Head** is the authority the packer reads first; the ordered commit history from `CP-35` is packed only as secondary detail.
- `P-4` `A → B → C → A` collapses: only the current canonical state plus **rejected-alternative records** (negative knowledge, with reasons) are promoted; positive churn stays in the ledger but is not packed by default.
- `P-2` Rules land in warn mode first; only `r-scope` may block, and only when `structure.Available()` gives reliable symbol-level truth.

### Constraints

- Extend, do not break, `CP-35`'s `internal/{changeledger,flowgate,structure,contextsync}/` and the post-`finishTurn` gate hook in `interactive_service.go`; reuse `observe.go`'s `TurnResult.GitDiff` and `structure.Dependents`.
- Per-`project_id`, local-first under `<target>/.flowpilot/`; shared derived files sync via the `CP-35 P-8` `context-engine/` Drive mechanism; machine-specific data stays local.
- All contract/drift/head computation is **non-fatal and retryable** (`AC-9`); it never blocks the raw artifact save.
- Deterministic-first: only the canonical behavior statement and rejection summaries are AI-generated (cheap-tier, heuristic fallback), mirroring `SD-17`'s `chat_summary` rule.

### Open Questions

- `Q-1` **(RESOLVED, 2026-07-08)** Design-of-record needs are met **without a new `SD-17` addendum**: CP-43's intent-signature/change-contract is already covered by **[SD-21](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md)** (this CP's parent), and the context-source registry it packs onto is now **[SD-22](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md)**. The earlier "one shared SD-17 D-13 addendum" idea is superseded by the SD-21 + SD-22 pair. Owner had confirmed an SD-level record is wanted; that record is SD-21 (intent) + SD-22 (registry).
- `Q-2` **(RESOLVED → C hybrid, 2026-07-08)** Contract capture UX = **hybrid** (confirms the `P-1` default): prefer **explicit** AI declaration (`feature/intent/files` block before editing, Confidence=`declared`, via the `context-discipline`/`git-commit-format` skill pack); **fall back to inferred** from the first diff (Confidence=`inferred`) when the AI does not declare; prompt-confirm the inferred case **only in `enforce` mode**. Rejected: (A) forced-explicit-always (+1 turn on every trivial edit) and (B) inferred-only (drift baked in, weak detection). `P-1` already encodes this — no change to §4.1 needed.
- `Q-3` **(RESOLVED → symbol-level block when GitNexus present, 2026-07-08)** Owner notes GitNexus (`structure`) is effectively always available on the primary project, so `r-scope` uses **symbol-level truth and may block** there. Nuance retained for portability: CP-43 targets *any* bound project, so where `structure.Available()==false` (a project GitNexus hasn't indexed), `r-scope` **degrades to file-level warn-only** to avoid `SD-17 R-1` false-drift — never a hard block on file-level alone. So: block is gated on `structure.Available()`, which is the common case here.
- `Q-4` **(RESOLVED → per `feature_key` only, 2026-07-08)** Canonical Head granularity is **per `feature_key`** in v1; per-symbol granularity is not pursued now.

### Source Refs

- `SS-14` US-3, AC-7 (records symbols/files changed), AC-8 (stale context flagged), BR-2 (`SS-13 §7.3` no silent redefinition of upstream intent).
- `SD-17` D-3 (ordered history, newest = truth), D-11 (deferred symbol/AST/change-contract), §3.2 chat_summary (rejected approaches), §6.1 prompt-assembly seam.
- `CP-35` §4.1 `changeledger`, §4.4 `flowgate` (`observe.go`/`evaluate.go`/`enforce.go`), §4.3 `structure`, §4.2.1 history packing, §4.8 `contextsync`.

## 1. Goal

Implement `SS-14 US-3` for any bound project: before a code-mutating step, the AI declares a **Change Contract** (feature, intended files/symbols, intended behavior); after the step, the engine flags any edit outside that contract and any drift from the feature's **Canonical Head**. Maintain one canonical statement of truth per feature — carrying an `intent_signature` and the list of tried-and-rejected alternatives — so that churn (`A → B → C → X → A`) is readable as *current truth + why the dead-ends failed*, not as raw commit noise. Cover the deferred `SD-17 D-11` scope-drift without the AST-hashing cost that caused it to be deferred.

## 2. Input Documents

- `SS-14` (US-3, AC-7, AC-8, BR-2) — the acceptance criteria this CP satisfies.
- `SD-17` (D-3, D-11, §3.2, §6.1) — the design this CP activates and extends; `D-11` is the deferred decision this CP delivers a cost-safe version of.
- `CP-35` (§4.1, §4.3, §4.4, §4.2.1, §4.8) — the shipped modules this CP builds on.

## 3. Implementation Strategy

- **Overall approach:** one new runner module `internal/changecontract/` plus new `flowgate` rules; no new Supabase tables. Reuse `CP-35`'s post-turn gate hook, `GitDiff` observation, `structure` provider, and `contextsync` Drive sync. Intent is hashed; code is never hashed.
- **Sequencing logic:** `P-1` (capture the contract) → `P-2` (flag scope drift against it) → `P-3` (Canonical Head + intent signature) → `P-4` (superseding decisions / churn collapse) → `P-5` (authority-first packing + Admin visibility). `P-2` can ship warn-only immediately after `P-1`.
- **Dependencies:** requires `CP-35` `changeledger` + `flowgate` + `structure` merged (they are). **Depends on [CP-44](./CP-44-Pluggable-Context-Source-Registry.md)** — the Canonical Head is packed as a registered context source, and the declared-scope `Change Contract` is surfaced through the same context-source registry; CP-44 (substrate) lands first so `P-3`/`P-5` do not hardcode a new source that would then need re-migration. `P-2` symbol-level drift is gated on `structure.Available()`; degrades to file-level otherwise. `P-5` packing extends `CP-35 §4.2.1` **via CP-44's `feature.history`/`canonical.head` sources**, not a bespoke slot.
- **Module map (new under `apps/local-runner/internal/`):** `changecontract/` (P-1, P-3, P-4). New rules live in existing `flowgate/` (P-2). Packing extends existing `featurecatalog`/`contextresolver` slots (P-5).

## 4. Work Breakdown

### 4.1 `P-1` Change Contract capture — `internal/changecontract/contract.go`

**Goal:** before a code turn, record what the AI *intends* to change, so `P-2` can flag anything else.

**Type:**
```go
type Contract struct {
    RunID        string   `json:"run_id"`
    StepID       string   `json:"step_id"`
    FeatureKey   string   `json:"feature_key"`   // from CP-35 feature.resolve
    Intent       string   `json:"intent"`        // one-line "what this change should achieve"
    DeclaredPaths []string `json:"declared_paths"` // globs the AI intends to touch
    DeclaredSymbols []string `json:"declared_symbols,omitempty"` // when structure available
    DeclaredAt   string   `json:"declared_at"`   // RFC3339
    Confidence   string   `json:"confidence"`    // declared | inferred
}
```

**Behavior:**
- New pre-turn resolver slot `contract.declare` (priority 1, runs after `CP-35`'s `feature.resolve`). It prompts the AI — via the `git-commit-format`/`context-discipline` skill pack — to emit a small declaration block (`feature`, `intent`, `files`) before editing. Parse it into a `Contract`.
- If the AI does not declare (legacy/simple turns), fall back to `Confidence="inferred"`: after the first diff, synthesize a contract from the touched top-level paths and ask for confirmation only when `gate_mode=enforce` (`Q-2`).
- Store one contract per `(run_id, step_id)` in `.flowpilot/contracts/contracts.ndjson`, last-wins, mutex-guarded (mirror `local_file_session_store.go`).

**Acceptance:** a code step begins with a stored `Contract` carrying `feature_key`, `intent`, and `declared_paths`; a turn with no declaration produces an `inferred` contract without blocking.

### 4.2 `P-2` Scope-drift detection — `flowgate` rules `r-contract`, `r-scope`

**Goal:** flag edits outside the declared contract. This is `US-3`'s "flagged when it changes anything else."

**Detection (deterministic set diff — no AST):**
- `observe.go` already yields `TurnResult.GitDiff []ChangedFile`. Compute `actual_paths = {f.Path}`.
- `out_of_scope = actual_paths \ match(DeclaredPaths)` (glob match; exclude `requirements/`, `change-audit/`, `*.md`, and the per-project ignore set from `SS-14 E-4`).
- File-level always. **Symbol-level** only when `structure.Available()`: map changed hunks → symbols and diff against `DeclaredSymbols`; a touched symbol with `structure.Dependents.Count > 0` outside scope is high-severity.

**Rules (seed into `settings/flow-rules.json`):**
```json
[ {"id":"r-contract","trigger":"code_changed_no_contract","required_output":"declared_change_contract","action":"reprompt"},
  {"id":"r-scope","trigger":"edit_outside_declared_scope","required_output":"confirm_or_revert_out_of_scope","action":"warn"} ]
```
- `r-scope` `action` is `warn` in v1; may escalate to `block` **only** when `structure.Available()` (reliable symbol truth) — resolves `Q-3`, respects `SD-17 R-1` false-drift risk.
- `r-contract` reprompts once ("declare intended scope before editing"), bounded by `CP-35`'s `max_reprompt_attempts`.

**Wiring:** add both to `evaluate.go` `Evaluate(tr, rules)`; reuse `enforce.go` reprompt/warn/block ladder and SSE violation emission. No new hook point — same post-`finishTurn` call site as `CP-35 P-4`.

**Acceptance:** a turn that edits a file outside `declared_paths` emits an `r-scope` warn violation with the offending paths; when GitNexus is present and the out-of-scope symbol has dependents, the violation is high-severity (and blocks if configured); a turn with no contract emits `r-contract` once.

### 4.3 `P-3` Canonical Head + intent signature — `internal/changecontract/head.go`

**Goal:** one authoritative record per feature that carries the "digital signature" of its intent and survives churn. This is the answer to "read the truth, not the log."

**Type:**
```go
type CanonicalHead struct {
    FeatureKey        string            `json:"feature_key"`
    BehaviorStatement string            `json:"behavior_statement"` // canonical "what it does now"
    GoverningDocIDs   []string          `json:"governing_doc_ids"`  // SS/SD/CP ids
    GoverningDocHashes map[string]string `json:"governing_doc_hashes"` // docID -> sha256(file)
    IntentSignature   string            `json:"intent_signature"`   // see below
    HeadCommit        string            `json:"head_commit"`
    Decisions         []Decision        `json:"decisions"`          // P-4, superseding list
    UpdatedAt         string            `json:"updated_at"`
    Status            string            `json:"status"`             // current | spec_drifted | code_drifted
}
```

**Intent signature (hash intent, never code):**
```
intent_signature = sha256( canonicalJoin(
    feature_key,
    sort(governing_doc_ids),
    sort(governing_doc_hashes.values),   // sha256 of each governing SS/SD/CP markdown
    normalize(behavior_statement)
))
```

**Drift semantics (the integrity check the user asked for):**
- **Spec drift:** recompute the governing doc hashes on each relevant turn; if they differ from `GoverningDocHashes` while `BehaviorStatement` is unchanged → `Status="spec_drifted"` → the Head is stale relative to its spec; emit `r-spec-drift` (warn) asking to reconcile (`AC-8`, `BR-2`: upstream intent changed, downstream must catch up — not silently redefine).
- **Code drift:** code for the feature changed (from `GitDiff` + `changeledger`) but the change was **out of the declared contract** and **not** reflected in `BehaviorStatement`/governing docs → the implementation has diverged from canonical intent; emit `r-code-drift` (warn) → "reconcile the change into the Canonical Head or revert."
- **Match:** signature holds and the change was in-contract → Head is refreshed (`HeadCommit`, `BehaviorStatement` folded from the contract's realized `Intent`), `Status="current"`. The AI/dev then does **not** need to read history.

**Build/update:** on a gate-passing code step, `UpdateHead(featureKey, contract, diff)` folds the realized contract into the Head and recomputes the signature. First build backfills from `CP-35`'s `feature_history` (newest entry seeds `HeadCommit`/`BehaviorStatement`) + governing docs discovered via `featurecatalog.DocRefs`.

**Store:** `.flowpilot/canonical/<feature_key>.json`, one Head per feature; shared-syncable (`P-5`).

**Acceptance:** editing a governing SS/SD flips the feature Head to `spec_drifted`; an out-of-contract code change that is not reconciled flips it to `code_drifted`; an in-contract change refreshes the Head and keeps `Status=current` with a new `intent_signature`.

### 4.4 `P-4` Superseding Decision Records — churn collapse — `internal/changecontract/decisions.go`

**Goal:** preserve the *negative knowledge* (why `B/C/X` were rejected) and let positive churn collapse, so `A → B → C → X → A` reads as "A + three closed dead-ends," not five equal states.

**Type:**
```go
type Decision struct {
    Tried         string `json:"tried"`          // short description of the approach
    Outcome       string `json:"outcome"`         // adopted | rejected | reverted
    Reason        string `json:"reason"`          // why rejected/reverted (the negative knowledge)
    SupersededBy  string `json:"superseded_by"`   // commit/decision that replaced it
    SourceDocID   string `json:"source_doc_id"`   // Task-/BUG-/CP- id when present
    At            string `json:"at"`
}
```

**Sources (deterministic-first):**
- `CP-35 §3.2` `chat_summary.ndjson` already records "decisions, rejected approaches" per feature — fold those into `Decisions`.
- `changeledger` `bugfix` entries on the same feature that revert a prior feature commit → `Outcome="reverted"`, `Reason` from the `BUG-` doc summary.
- Only the `Reason`/`Tried` *text* is AI-summarized (cheap-tier, heuristic fallback); ordering and linkage are deterministic.

**Collapse rule:** the Canonical Head keeps only (a) the current canonical state and (b) the `Decisions` list. The full positive churn stays in `feature_history.ndjson` but is **not packed by default** — `P-5` packs the Head; raw history is available on demand only.

**Acceptance:** for a feature that went `A → B(rejected) → A`, the Head shows `A` current plus a `Decision{Tried:"B", Outcome:"rejected", Reason:...}`; the packed prompt does not replay B/C churn unless explicitly requested.

### 4.5 `P-5` Authority-first packing + Admin visibility

**Prompt packing (extends `CP-35 §4.2.1`):** the `feature.history` slot prepends a Canonical Head block **before** the ordered history:
```
## Canonical state of "<feature>"  (this is current truth — build on it)
- Behavior: <behavior_statement>
- Intent signature: <sig8>  [status: current | spec_drifted | code_drifted]
- Do NOT re-attempt these (already rejected):
  - B — <reason>            (BUG-060)
  - X — <reason>            (Task-140)
## Full history (secondary, oldest → newest)   ← packed only if budget allows / requested
- ...
```
The Head is a mandatory slot; raw history is optional/lower-priority in the budget packer (`CP-23`/`CP-10` §5), directly cutting the churn noise the user reported.

**Admin Web:** a "Canonical Head" panel per feature — behavior statement, signature + status chip, rejected-decisions list, and the Change Contract for the active step with in-scope/out-of-scope diff highlighting.

**Acceptance:** on a resolved code turn the prompt leads with the Canonical Head; the Admin panel shows signature status and the scope diff for the current step.

## 5. Touched Areas

- **New module:** `apps/local-runner/internal/changecontract/` (`contract.go`, `head.go`, `decisions.go`, `query.go`).
- **Extended:** `internal/flowgate/` (`rules.go` + `evaluate.go` new triggers `r-contract`/`r-scope`/`r-spec-drift`/`r-code-drift`/`r-attach-spec`/`r-retire`; `observe.go` unchanged — reuses `GitDiff`); `internal/featurecatalog` history slot (Head prepend); `internal/contextresolver` new `contract.declare` slot; `internal/contextsync` shared-file list (+ `canonical/*.json`).
- **Reused unchanged:** `interactive_service.go` post-`finishTurn` hook, `structure.Dependents`, `changeledger` history, `chat_summary`, Drive sync helpers.
- **Owned elsewhere:** skill-pack copy for the declaration prompt lives in `CP-34`/`CP-35 P-6` (`context-discipline` skill gets a "declare scope before editing" clause); Admin panels are Admin-Web work.
- **Database:** none (local-first; see §6).

## 6. Data or Migration Steps

- **No Supabase migration.** All data is local under `<target>/.flowpilot/`: `contracts/contracts.ndjson`, `canonical/<feature_key>.json`.
- **New `step_context_slots.resolver` enum value:** `contract.declare`.
- **New `settings/flow-rules.json` rules:** `r-contract`, `r-scope`, `r-spec-drift`, `r-code-drift` (all `warn` by default; `r-scope` escalates to `block` only when `structure.Available()`), plus the two lifecycle approval rules `r-attach-spec` and `r-retire` (`action: approve`, reuse `SD-16` approval-gate; see [SD-21 §6](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md#6-interfaces-and-contracts)).
- **`contextsync` shared set (+):** add `canonical/*.json` to the `context-engine/` Drive-synced set; `contracts.ndjson` stays **local-only** (ephemeral per-run intent).
- **Backfill on first bind:** build one `CanonicalHead` per `feature_key` from `feature_history` (newest = seed) + `featurecatalog.DocRefs` governing docs + `chat_summary` rejected approaches.

## 7. Validation Plan

- **Unit:** contract parse (declared vs inferred); glob scope diff incl. `SS-14 E-4` ignore set; symbol-level diff when structure present; `intent_signature` stability (same inputs → same hash; doc-content change → new hash); spec-drift/code-drift status transitions; decision folding + collapse (churn not packed); Head backfill from history.
- **Integration:** declare contract → edit in scope → `Status=current`, Head refreshed; edit out of scope → `r-scope` warn (and block when GitNexus present + dependents); edit a governing SS → `spec_drifted`; `A → B → A` history → Head shows A + rejected-B decision and the packed prompt omits B churn; no-declaration turn → `r-contract` once then inferred contract.
- **Manual:** confirm the packed prompt leads with the Canonical Head across Claude/Codex; confirm scope-diff highlighting in Admin; confirm `contracts.ndjson` never syncs to Drive while `canonical/*.json` does.
- **Failure cases:** governing doc deleted (signature recompute must not panic → `spec_drifted`, flagged); rename-only diff must **not** trip `r-scope` when GitNexus resolves it to an in-scope symbol (the `D-11` false-drift trap this CP is designed to avoid).

## 8. Rollout and Fallback

- **Rollout order:** `P-1` → `P-2` (warn) → `P-3` → `P-4` → `P-5`; flip `r-scope` to `block` per project only after warn-mode calibration and only where `structure.Available()`.
- **Fallback path:** each rule is feature-flagged in `flow-rules.json`; disabling all reverts to exactly `CP-35` behavior with no data loss (`canonical/` and `contracts/` retained locally; git remains source of truth).
- **Monitoring:** contract-vs-actual diffs, scope violations, signature status transitions, and reconciliation actions → the existing gate audit log (extends `CP-35 §8` / `CP-23` telemetry), retrievable per `AC-7`.

## 9. Risks

- `R-1` **False drift** on rename/format/ripple — the exact reason `D-11` was deferred. Mitigation: hash intent not code; scope diff is declared-vs-actual (not content hashing); symbol-level only via GitNexus; `SS-14 E-4` ignore set; `r-scope` warns before it ever blocks.
- `R-2` **Contract friction** — forcing a declaration slows simple turns. Mitigation: inferred-contract fallback; `r-contract` warn/reprompt only, never blocks; declaration folded into the existing skill pack, not a separate UI step.
- `R-3` **Stale/incorrect behavior statement** poisons the Head. Mitigation: statement is deterministic-seeded from newest history + governing docs; AI text is cheap-tier with heuristic fallback; `spec_drifted`/`code_drifted` are surfaced for human reconciliation, never auto-rewritten silently (`BR-2`).
- `R-4` **Overlap with `CP-35`/`CP-23`.** Mitigation: `CP-35` = structural *history* (what happened), `CP-23` = behavioral *runtime* drift (in-session), `CP-43` = *intent integrity* (declared vs actual vs canonical). Shared hooks, telemetry, and storage; no duplicated observation.
- `R-5` **Governing-doc discovery is wrong** (Head hashes the wrong specs). Mitigation: source `governing_doc_ids` from `featurecatalog.DocRefs`; `Q-1` may promote this into an `SD-17` addendum for a reviewed contract.

## 10. Definition of Done

- [ ] `P-1` Every code-mutating step has a stored `Contract` (declared or inferred) with `feature_key`, `intent`, `declared_paths`; no-declaration turns do not block.
- [ ] `P-2` An edit outside the declared scope raises `r-scope` (warn by default) with the offending paths; symbol-level severity applies when `structure.Available()`; `r-scope` blocks only when configured and structure is available.
- [ ] `P-3` Each feature has a `CanonicalHead` with a reproducible `intent_signature`; editing a governing spec flips it to `spec_drifted`; an unreconciled out-of-contract code change flips it to `code_drifted`; an in-contract change refreshes the Head to `current`.
- [ ] `P-4` `A → B → A` churn is represented as current-A + a rejected-B `Decision` with a reason; positive churn is not packed into the prompt by default.
- [ ] `P-5` Resolved code turns lead with the Canonical Head block (behavior + signature status + rejected list) before any raw history; Admin Web shows signature status and the step's in/out-of-scope diff.
- [ ] Satisfies `SS-14` US-3 (declare + flag), AC-7 (records symbols/files changed, auditable), AC-8 (spec-changed context flagged stale).
- [ ] All contract/head/drift computation is non-fatal and retryable (`AC-9`); raw artifact save is never blocked.
- [ ] `go build ./...` and `go test ./internal/changecontract/... ./internal/flowgate/...` pass; no regression in `CP-35` module tests.
- [ ] `contracts.ndjson` stays local-only; `canonical/*.json` syncs to `context-engine/` via the `CP-35 P-8` mechanism.

## 11. Relocated Context-Harness E2E Tests (from CP-41 §11)

> **Scope note (RESOLVED 2026-07-13, owner direction).** CP-43's scope is **only** the Change Contract / Canonical Intent Signature / scope-drift work (§1–§10 above). The context-harness *refactor* this note originally anticipated — reworking feature-key resolution, history/chat-summary injection, and the context-package lifecycle — was **absorbed by [CP-44](./CP-44-Pluggable-Context-Source-Registry.md) (Pluggable Context Source Registry, now DONE)**, which turned those into pluggable context sources. **§1 will NOT be expanded** to cover a context-harness refactor — there is no such refactor left for CP-43 to do. The CH-1..CH-4 scenarios below are therefore **inherited acceptance checks for context-harness behavior already delivered by CP-41 + CP-44**, not CP-43 deliverables — CP-43's own DOD (§10) does not depend on them. CH-3 was live-verified 2026-07-13 (during CP-41 closure); CH-1/CH-2/CH-4 are covered by CP-41/CP-44's shipped behavior and may be spot-checked opportunistically but do not gate CP-43. When CP-43 adds its `canonical.head` source it plugs into CP-44's registry exactly like `feature.history`/`chat.summary` — it does not modify their retrieval logic.
>
> **UN-FOLDED (2026-07-08, owner direction) — [BUG-243](../../09-BugFix/done/BUG-243-Flow-Mode-Validate-And-Audit-Behaviors-Disconnected-From-Task-170-171.md) is no longer CP-43 scope.** The 2026-07-06 note below folded the RAG-harness Flow-Mode **pipeline execution** gap into CP-43. On splitting the context-harness rework into [CP-44](./CP-44-Pluggable-Context-Source-Registry.md) (context-source substrate) + this CP (intent integrity), BUG-243 was recognized as **flow-engine execution plumbing for the `validate`/`audit` nodes** (`F-0` mid-flow inline dispatch + `F-1/F-2` handler wiring + `F-3` audit UI) — it touches the `validate`/`audit` nodes, not the `context` node, and is orthogonal to both CPs. It is now a **standalone bug scheduled first**, tracked entirely in its own doc. _Superseded note (2026-07-06): "folded into this CP's rework." No longer in force._
>
> **RE-HOMED (2026-07-08) — CP-42 `P-4` moved to [CP-44](./CP-44-Pluggable-Context-Source-Registry.md).** The 2026-07-07 note below folded two CP-42 §10 residual DOD items here. On the split, **`P-4` (context packages become typed artifacts with producer/consumer bindings declared by flow data, not a CP-41-only special case) is exactly CP-44's charter** and is now owned by CP-44 (see CP-44 `P-3`/`P-4` + its child Task-193/194). Only CP-42 `P-8` stays associated with CP-43's neighbourhood:
> - RAG Harness runs from a pack definition with **no `isPlanStepType`/`isCodingStepType` runtime branch** (CP-42 `P-8`; the alias-normalization shim landed, the literal step-name branches did not get removed). Note: the remaining literal branches live in `flow_context_handoff.go` and the `validate`/`audit` dispatch path — so `P-8` is most naturally closed alongside **BUG-243 `F-0`** (mid-flow inline dispatch removes the last literal step-type reason-to-branch). Track `P-8` with BUG-243, not with the intent-integrity work in §1–§10.
>
> _Superseded note (2026-07-07): "both folded here." Now: `P-4`→CP-44, `P-8`→BUG-243 neighbourhood._

### Scenario CH-1 — Feature History Injected (Verify Deterministic Retrieval)

**Setup:** A workspace whose `.flowpilot/ledger/feature_history.ndjson` has at least 2 commits for the target feature key.

**Action:** Run only the context/Plan step. Inspect the composed prompt logged to the prompt-log directory.

**Expected:**
- [ ] Prompt log file contains the `## Flow Context Package` section.
- [ ] `## Prior Work` block lists the commit summaries from the ledger.
- [ ] `## Audit note: No vector retrieval used` line is present — confirms no vector DB involved.
- [ ] `featureConfidence` is `verified` (confidence ≥ 5.0 threshold met).

### Scenario CH-2 — Unknown Feature Key Degrades Gracefully

**Setup:** A workspace with no `.flowpilot` catalog directory, or a prompt that resolves to no known feature key.

**Action:** Start the flow with a context/Plan prompt like: `"Fix a bug in some-unknown-feature-xyz"`.

**Expected:**
- [ ] Context/Plan step completes without crashing.
- [ ] `FlowContextPackage` has `featureConfidence: unresolved` and `warnings: ["feature catalog unavailable: ..."]` or `["no feature resolved ..."]`.
- [ ] Coding step still receives the package (degraded — no history block, but the sentinel is present).
- [ ] No crash, no panic, no empty prompt.

### Scenario CH-3 — No Chat Summaries (History-Only Package)

**Setup:** Workspace with feature history in `.flowpilot/ledger/feature_history.ndjson` but NO `chat_summary.ndjson`. *(This is CP-41's original `DOD-7` graceful-degradation acceptance.)*

**Action:** Run the context/Plan step for a known feature key.

**Expected:**
- [x] `FlowContextPackage` has `historyBlock` populated (commit history present).
- [x] `discussionBlock` is empty or absent — no crash due to missing chat summary ledger.
- [x] Coding step prompt includes the history block but no discussion section.
- [x] `warnings` does NOT mention chat summary as a fatal error (graceful degradation).

**Verified 2026-07-13** in `D:\working\gate-sandbox`: `chat_summary.ndjson` deleted, prompt `"Implement a small improvement to the calc-core arithmetic divide operation"`. `run-9954-flow-events.ndjson` shows `featureKey: calc-core`, `featureConfidence: verified`, `historyBlock` populated, the `chat.summary` package section has an empty `Body` and no `warnings` field at all. `run-9959-turns.ndjson` (the Coding-step prompt) contains `## Prior work on "calc-core"` with no "Prior discussion" section anywhere. No crash — the run completed through validate/audit.

### Scenario CH-4 — Plan Step Reruns → Coding Gets Fresh Context Package

**Setup:** A Flow Mode run that has already completed one context→Coding cycle.

**Action:** Rerun the context/Plan step on the same run. Then observe the Coding step's next turn.

**Expected:**
- [ ] A new `EventFlowContextPackage` is emitted with a new `packageId`.
- [ ] The Coding step's retry prompt references the **new** package ID, not the old one.
- [ ] Old package ID is no longer used in the Coding prompt after the rerun.
- [ ] `planContextPackage` cache is cleared (verify by two distinct `EventFlowContextPackage` entries).

### Relocated failure cases

| Case | How to trigger | Expected |
|------|---------------|----------|
| Missing chat summary ledger | Delete `chat_summary.ndjson` before the context/Plan step | Package degrades: `discussionBlock` empty; no crash |
| Runner restart after context-package creation | Kill runner after the context/Plan step, restart | Coding step resumes; `EventFlowContextPackage` is found in persisted events; new package NOT rebuilt |

## 12. Notes

- **Lifecycle walkthrough + who-creates/who-changes:** see [SD-21 §13 (Phụ lục A, tiếng Việt)](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md#13-phụ-lục-a--giải-thích--ví-dụ-vòng-đời-tiếng-việt) — the concrete `chat-ui` example (Change Contract vs Canonical Head, `intent_signature`, and the runner/AI/human roles) lives there and is not duplicated here.
- This CP activates the deferred `SD-17 D-11`; `SD-21` is the reviewed design of record for it.
- The guiding principle from the originating discussion: **hash intent, not bytes; read the authority (Canonical Head), not the log; preserve negative knowledge, collapse positive churn.** `A`-then-`A` is byte-identical but not semantically identical — the value of the history is the rejected `B/C/X` and their reasons, which the Head carries and the raw ledger buries.
