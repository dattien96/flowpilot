---
name: flow-harness-contract
description: Harness discipline for every coding flow — harness tier selection, frozen scope with plan-loop-before-freeze, reproduce-first bugfix with locked red test, contract-first TDD with signature lock, machine-checkable done verdicts via submit_review_outcome, doc intent ceiling, drift self-correction ladder, and session resume checkpoints. Use PROACTIVELY on every task, bugfix, or feature implementation.
version: 7
---

# flow-harness-contract

Umbrella rule card for executing work the way a gated engineering flow does it,
even when no tool is enforcing the gates. Companion to **safe-fix-contract**
(test safety), **context-discipline** (feature history + change contract),
**phase-doc** (document contract), **audit-logging** (ledger). Where rules
overlap, the stricter one wins.

Canonical sources (FlowPilot engine): `CP-58` (bug/task/cp harness family),
`CP-64` (reproduce-first gate), `CP-67` (contract-first scaffold + signature
lock), `CP-61` (done verdict gate), `CP-55` (flow-first preflight → canonical
acceptance), and the builtin flow packs `flows/bug-harness.yaml`,
`flows/bug-plan-harness.yaml`, `flows/task-harness.yaml`.

---

## 0. Harness tier selection (CP-58)

Pick the smallest tier that fits — never run a heavier harness for a light job:

| Tier               | When                                                 | Flow shape                                                                                                                                               | Cap                          | Plan artifact                                         |
| ------------------ | ---------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------- | ----------------------------------------------------- |
| `bug-harness`      | Clear hotfix, repro obvious, no investigation needed | 9 steps: `plan(scout)→freeze→context→reproduce_test→implement→validate→reviewer→synthesis→audit`, 1 code loop                                            | 3                            | none (contract only)                                  |
| `bug-plan-harness` | Bug needing investigation + a BUG doc before any fix | 13 nodes: scout→context→`plan_writer`→`plan_reviewer`→`plan_synthesis` (**plan loop**)→freeze→reproduce_test→implement→validate→reviewer→synthesis→audit | 5 (shared across both loops) | `requirements/09-BugFix/todo/BUG-{{idx}}-{{slug}}.md` |
| `task-harness`     | Feature slice needing HLD/LLD                        | Same shape as bug-plan-harness but plan writes `requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md`                                                      | 5                            | Task doc                                              |
| `cp-harness`       | Multi-task initiative (slice-only default)           | scout→context→cp_plan_writer→cp_reviewer→cp_synthesis→task_splitter→audit; NO freeze/coding (per-Task coding runs task-harness)                          | —                            | `requirements/07-Coding-Plan/todo/CP-*.md`            |

Both loops in bug-plan/task-harness share one round budget (`cap`, `onCap:
escalate`, `extendBy: 2`); the plan loop resets when the plan approves.
`acceptance_nodes` always include the synthesis hubs + validate + audit — a
"done" path that bypasses any of them is invalid.

## 0a. 3-Layer defense for feature_key (never trust the Scout)

1. **Scout** (`preflight_contract_plan`, read-only, cheap model tier) only
   proposes `candidate_feature_keys` + `candidate_paths` — a hypothesis, never
   a decision.
2. **plan_writer** (highest-reasoning tier, DOC WRITER ONLY: no source edits,
   no commands, no change-audit writes) MUST verify candidates against
   `FEATURE-KEYS.md` and the symptom, and **override** the wrong key in
   §Metadata; uses read/grep tools to fetch missing code before writing.
3. **plan_reviewer** (read-only) re-checks feature_key + DeclaredPaths; wrong
   mapping = `changes_requested`. Only after plan approval does
   `contract.freeze` lock scope — **freeze happens after the plan loop, never
   before it**.

## 1. Scope: declare → freeze → amend

1. Before any code-mutating edit, emit the change-contract block (see
   `context-discipline`): `feature_key`, one-line intent, and the file list you
   expect to touch.
2. Once implementation starts, the declared file list is **frozen**. An edit
   outside it is drift — stop, restate the contract with the expanded scope
   (`files: + <new paths>`), then continue. Never silently widen scope.
3. A contract inferred _after_ edits (from the diff) is not a preflight
   contract. Declare first; the diff validates the declaration, it does not
   create it.
4. The canonical truth for a feature is its **latest ledger entry + governing
   spec doc** — not the raw commit churn. Build on it; never re-implement or
   silently undo prior settled work.

## 2. Bugfix = reproduce-first

1. Before touching production code for a bug, write **one complete failing
   test** that reproduces it.
2. The test must fail by **assertion failure** (`expected X, got Y`). A compile
   error or panic does not count as reproduction — fix the test until it
   compiles and fails on the assertion.
3. The reproducer must **run the suite itself** before finishing: suite does
   not compile → turn rejected; suite passes → turn rejected (bug not
   reproduced); new test fails on an assertion → accepted (CP-64 gate
   mechanics — in this step the suite is SUPPOSED to be red; this overrides
   any "keep the suite green" default persona).
4. Once the red test is confirmed, that test file is **locked read-only**: fix
   production code only. Never reshape, weaken, skip, or delete the
   reproducing test to fit your fix (`r-reproduce` + `r-additive-tests`).
   The gate is **always on** — the old degrade flag was retired by CP-67; the
   only rollback is a code revert.
5. Flaky reproduce is retried at most 2 times (CP-64 R-1) before failing the
   gate.
6. If no failing test can be produced, state that explicitly and ask the user —
   do not proceed on a guessed fix.

## 3. New feature = contract-first scaffold TDD (CP-67)

1. Before implementation: produce compile-clean **stubs** (bodies =
   `not implemented` / zero-sentinels) plus a **complete test suite** that
   compiles and runs RED at runtime.
2. The scaffold turn is gated by `r-scaffold-red` with **three anti-smuggling
   signals** — (a) suite all-green means you implemented logic inside the
   "scaffold" → rewrite as stubs; (b) compile failure → rejected; (c) static
   stub-body whitelist: an AST walk flags any symbol body outside the stub
   whitelist (TODO/throw not-implemented/zero-sentinel) even when tests are
   red — "wrote a body with wrong logic so tests still fail" is a violation.
   Only ≥1 red test is required (structural compile-only tests may legitimately
   pass); the static signal catches body smuggling regardless of test color.
3. Once the suite exists, all public signatures are **locked**: the runner
   snapshots a canonical AST `SignatureHash` (signature-only, bodies stripped).
   Implementation fills bodies only — never renames, re-types, or adds/removes
   a signature.
4. If a signature proves inadequate mid-implementation: do not edit it inline.
   **Accumulate & batch** — keep implementing everything else, then submit ONE
   batched renegotiation request (via `submit_coder_outcome`) at end of turn.
   Negotiation is hub-and-spoke through the Main Agent (never peer-to-peer with
   the test author); budget `cap: 5`, then escalate (CP-67 P-4/P-5).

## 4. "Done" requires a machine-checkable verdict

1. Never declare done in prose. A done claim needs a **verdict**: each
   acceptance criterion → pass/fail + evidence (test name, `file:line`, command
   output). In a FlowPilot run the reviewer records this via the
   **`submit_review_outcome`** tool (per-AC verdicts with `file:line`
   citations, T1 transport schema) — a missing verdict means the review never
   happened, and the audit node must treat it as blocked.
2. Review/fix loops are **bounded** (bug-harness `cap: 3`; bug-plan/task-harness
   `cap: 5` shared across plan + code loops; `onCap: escalate`, `extendBy: 2`,
   `extendMax: 2`). At the cap with open findings: escalate to the user with
   options (extend / accept / stop). Never silently stop, never retry
   unbounded.
3. A green suite reached by weakening, skipping, or tampering with tests is a
   violation — not a pass (see `safe-fix-contract` R1).
4. A check that cannot run (no baseline, broken env, missing tool) must be
   reported as **blind/unverified** — never reported as pass. Fail closed.
   The engine encodes this as `gate_blind`: a missing or red-at-capture
   baseline is a first-class block, never a silent pass (CP-53 D-1).
5. Any test-override acceptance goes into a **waiver ledger** with reason +
   expiry (`.flowpilot/settings/`); an expired waiver re-arms the regression
   gate. A waived regression must never exist silently (CP-53 D-5).

## 4a. Green ≠ done — the spec-confirmation chain

Passing tests NEVER authorize `done` by themselves — tests themselves can be
wrong vs the spec. The chain that must hold (CP-62 M-4/D-1, CP-61, CP-53 D-2,
CP-55 §3.10, CP-47):

1. The reviewer submits **per-AC verdicts** (`{ac_id, verdict: pass|fail|
blocked, evidence: [{path, line, excerpt}]}`) — one row per acceptance
   criterion taken from the locked artifact's AC list, with file:line evidence.
2. **AC coverage is enforced at review submission** (bridge layer): a verdict
   missing any AC is rejected with a named error naming the missing ACs.
3. The hub (`plan_synthesis` / `synthesis` / `cp_synthesis`) may NOT take the
   `done` edge without a recorded PASS verdict — missing/FAIL/escalate verdict
   routes continue/escalate/ask_user instead (CP-61 fail-closed backstop).
   LLM prose "done" (the "Ralph Wiggum loop") is never sufficient.
4. New production behavior requires a **new test derived from an AC**
   (`r-newtest`) — adding tests, never editing old ones.
5. Spec conformance outlives the run: governing SS/SD/CP doc hashes feed the
   Canonical Head (`intent_signature` = hash of intent, not code); doc drift →
   `r-spec-drift`, out-of-contract code change → `r-code-drift`; reconcile or
   revert, never silently redefine (CP-43).
6. Canonical acceptance is terminal: finalizing the pending canonical head
   must happen exactly once BEFORE the run publishes `done` — if finalization
   fails, `done` is NOT published (CP-55 §3.10).

## 5. Documents: hard ceiling on intent

1. Code shows **what** the system does; it can never show **why** the business
   wants it. Never fabricate business intent, user stories, or acceptance
   criteria in spec-level documents — scaffold the skeleton and mark
   `TODO: human intent needed`.
2. Spec/requirement-level content becomes authoritative **only after explicit
   human confirmation** (a lock/preview step). Do not derive downstream plans
   from unconfirmed specs.
3. Every `Task-*`/`BUG-*` document must carry a `## Definition of Done`
   section with checkboxes. Marking a doc done with unchecked items requires an
   explicit written justification — silence is a violation.
4. Never tick a checkbox on behalf of anyone; `[x]` is a deliberate act.

## 6. Drift: self-correction ladder

Watch for these signals in your own work (the engine detects the first two
automatically and records them in `.flowpilot/workflow_drift_events.json`):

| Signal                                                | Meaning                           |
| ----------------------------------------------------- | --------------------------------- |
| `zero_delta_progress` — turn produced no code delta   | Stall — re-plan, don't re-run     |
| `repeated_test_failure` — same test failing ≥ 2 times | You are guessing, not diagnosing  |
| Repeated apology/retry loops                          | Wrong approach, not wrong attempt |
| Edits drifting outside declared scope                 | Scope breach — amend or revert    |

Ladder, in order — **never skip to a harsher step, never hard-rollback**
(machine actions in parentheses). Drift score is a telemetry aggregate over
gate signals — repeated same-test failure +30, out-of-scope edits +35,
apology loop +25, zero-delta progress +20; score decays after a successful
correction (CP-23):

1. Score 0–29: healthy — nothing fires.
2. Score 30–59: a corrective note is injected into your next prompt
   (`inject_system_note`) urging a strategy change.
3. Score 60–79: context is narrowed — next turn repacks at halved budget
   (`narrow_context`). Stay on the failing seam.
4. Score ≥80: pause for human (`pause_for_human`) — dev mode blocks the run
   (`BlockReason: "drift"`, single idempotent `drift_pause_required` event, a
   flow child parks its parent hub); vibe mode NEVER pauses for drift — the
   owner debate owns remediation.

## 7. Mistakes become lessons — only with approval

1. A **repeated** mistake (same failure class twice) → draft a lesson card:
   symptom, root cause, one-line rule.
2. Propose the card to the user. Only a **human-approved** lesson becomes a
   durable rule or skill file.
3. Never auto-create permanent rule/skill docs from a single incident — that
   produces knowledge garbage.

## 8. Session checkpoint & resume

1. **Inside a FlowPilot run**, the canonical resume artifacts are the engine's
   own: `.flowpilot/ledger/chat_summary.ndjson` (latest run/turn summary —
   read it FIRST), `.flowpilot/ledger/feature_history.ndjson` (latest entry
   per feature), the frozen contract, and `sprint_handoff.v1` for
   multi-sprint work. Never reconstruct state from chat scroll.
2. Outside FlowPilot (plain session), maintain a `STATE.md` checkpoint:
   current phase, what's done, what's next, the frozen contract, open
   findings; update it at every pause point and before ending the session.
3. A fresh session must read the checkpoint (or the engine summaries) first.
4. If the checkpoint and the working tree disagree, trust the tree and say so;
   the checkpoint is a guide, not a rewrite license.

## 9. Machine gates & `.flowpilot` artifact map

Every soft rule above has a hard gate behind it (`gate_mode: enforce`). Know
the rule ids so a block is diagnosable, and know what artifact each step owes:

| Gate rule                         | Enforces                                                                          | Skill section                    |
| --------------------------------- | --------------------------------------------------------------------------------- | -------------------------------- |
| `r-tests`, `r-reg`                | suite green + no regression vs baseline (Oracle Rule)                             | safe-fix R1                      |
| `r-reproduce`                     | red assertion test exists before production edits; green/compile-error = rejected | §2                               |
| `r-signature-lock`                | public signature hash unchanged during implement                                  | §3                               |
| `r-additive-tests`                | no pre-existing test file edited                                                  | safe-fix R1                      |
| `r-newtest`                       | new production code ships new tests                                               | safe-fix R3                      |
| `r-contract`, `r-scope`           | change contract declared; edits stay in declared paths                            | §1                               |
| `r-spec-drift`, `r-code-drift`    | code/spec divergence vs canonical docs                                            | §1, §5                           |
| `r-dod-present`, `r-dod-complete` | DoD section exists; all boxes checked or justified                                | §5                               |
| `r-ca`, `r-fk`                    | machine-parseable CA note; verified feature_key in commits                        | audit-logging, git-commit-format |
| `r-requirement`                   | test signatures match locked requirements (autonomous modes)                      | §5                               |

Artifact map (what a completed run leaves behind):

- Scout/plan/review verdicts → `submit_review_outcome` → recorded in
  `.flowpilot/ledger/chat_summary.ndjson`.
- `contract.freeze` → `.flowpilot/contracts/frozen_contracts.ndjson` +
  `frozen_contract_events.ndjson` (versioned; scope amendments supersede).
- Gate outcomes → `.flowpilot/gate-metrics.ndjson` (actions `block` /
  `gate_blind`; a gate that cannot run is `gate_blind`, never pass).
- Drift → `.flowpilot/workflow_drift_events.json` (score, signals, action).
- `audit` node → `change-audit/CA-*.md` (ledger block) +
  `.flowpilot/ledger/feature_history.ndjson` append + `chat_summary.ndjson` +
  `canonical-pending/` staging + `knowledge/` refresh (auto-distilled, never
  hand-edit). A stale ledger vs HEAD raises `ledger-needs-update` — resync
  before the next run, do not ignore it.

## 10. Orchestration invariants (CP-36/42/53/62/65)

1. **Hub-only coordination**: children never talk to each other; the main
   agent (hub) routes. Reviewer cohort results arrive as one consolidated join
   note; the hub reinvokes once per join — never on the first member.
2. **Schema-first routing (T0–T3)**: every agent output that routes state must
   go through a declared tool face (`submit_review_outcome`,
   `submit_coder_outcome`, …). T0 deterministic Go wins over T1 transport
   schema; T2 allows exactly one corrective reprompt; T3 fails closed to
   park/escalate — never interpret prose, never a silent `done`.
3. **Node isolation is enforcement, not instruction**: postures
   `read_only | verdict_only | standard` declared in flow YAML are silent-deny
   at the bridge. Reviewers may run read-only commands (git diff/log/ls/rg)
   but `rm`, redirects, `tee`, `git push` are denied.
4. **Gate precedence** (pinned): `r-requirement` > drift > owner-debate >
   `r-dod`; the budget packer never cuts contract/verdict-evidence sections.
5. **Cap escalation routes to tournament**: when a review loop exhausts its
   cap, the engine may spawn a multi-candidate tournament in isolated
   worktrees; the arbiter is deterministic Go —
   `0.5·testPassRate + 0.3·lspCleanliness + 0.2·blastRadius` — and scores
   **0** to any candidate that breaks existing tests, whatever its own pass
   rate. Tie/no-auto-pick → human decision card; retry ≤ 2, merge conflict →
   human immediately (CP-65).
6. **Mistakes become lessons only on repeat + approval**: single incident →
   drift event; pattern repeated ≥2 → lesson candidate; only human approval
   promotes it to a durable skill/rule (CP-23 D-4).
