# local-runner — Working Contract

Applies to all code under `apps/local-runner/`. These are the invariants the
runtime is built to enforce. Keep them true **in the code**, not just in prose —
the skills and docs in this repo advise, the Go code guarantees.

Detailed shipped rules live in `internal/skillpack/flow-pack/common/` (installed
into target projects). This file is for whoever modifies the runner itself.

---

## 1. Engine architecture

- The engine/coordinator is **domain-free**: no role strings, task types, or
  domain conditionals in coordinator code. Roles, prompts, and phase semantics
  live in pack/skill/graph **data**; Go enforces state, joins, caps, audit.
- **Hub-only routing**: child agents are isolated; coordination happens through
  the hub. No peer-to-peer dispatch, no bypass paths.
- **Schema-first output**: anything consumed for routing/state must come through
  the deterministic pipeline — deterministic read → tool/schema → validate +
  one reprompt → fail-closed. Never parse free-form prose to drive state.
- One generic transition signal per semantics. Declared tool faces map onto it;
  do not register a second tool with the same meaning.
- Extend existing seams and stores. Never create a parallel session, persistence,
  or registry model — add fields/tables on the existing path.

## 2. Durability & recovery (the strongest contract)

- Every state mutation must survive **kill, restart, chat switch, and device
  switch**. If it only lives in RAM, it does not exist. The durable record is
  the single source of truth.
- Delivery has **three outcomes**: terminal | safely-retryable | explicitly-
  uncertain. Never silently lose an event; never claim exactly-once.
- Corrupt or version-incompatible state → fail closed into a
  `repair_required`-style state. **Never zero-value resume.**
- Stop/send and other external effects must be **linearized** on durable CAS —
  `ctx.Err()` alone is defense-in-depth, not the guard.
- Provider session is pinned per run/leg. Switching provider = a **new leg**,
  never a migrated live session.
- Chat/history rendering reads from our own store; provider-side folders are
  per-leg engine state, not the transcript source of truth.
- Non-terminal records are never pruned; recovery acts only through
  claim + lease/Stop recheck before every provider call.

## 3. Gates

- Gate failures are **fail-closed**: unknown baseline, broken env, or unverifiable
  input is a block/warn — never a silent pass.
- Gate/context calculations are **non-fatal and retryable**: they must not block
  artifact save or corrupt a run; only the contracts that explicitly declare a
  hard block may block.
- Review/retry loops are bounded by a cap; hitting the cap escalates with a
  structured status — no silent stop, no unbounded reprompt.
- Waivers are ledgered debt with expiry; a waiver that exists only in chat does
  not exist.

## 4. Worktree isolation

- Worktree mode is **opt-in per run, default off**; disabled mode preserves the
  legacy behavior byte-for-byte.
- Merge-back is always an explicit user decision: `apply_patch` |
  `keep_branch` | `discard`. Conflicts produce a card + patch artifact, never
  a force-apply.
- Apply/merge is **serialized per repository**.
- Deleting a run/chat with a live binding requires resolving the merge decision
  first.
- Resume **revalidates the binding**; externally-deleted worktree/binding →
  `lost` + notice — never silently recreated.
- Boot GC may only prune worktrees of deleted/corrupt runs. `merge_pending` and
  resumable bindings are never GC'd.
- Parallel candidate arbitration is deterministic (test rate + diagnostics +
  blast radius); no ungrounded ranking.

## 5. Provider parity

- Any change touching an adapter, event stream, session, or gate hook must be
  verified across **Claude, Codex, and Grok** — or proven provider-agnostic with
  evidence (see `flow-pack/common/cross-provider-parity`).
- Missing capability on a provider = typed degradation, never silent masking.

## 6. House rules (all edits)

- Never edit/weaken existing tests to pass; old test fails → fix code or stop
  and report (`oracle-rule`, `additive-tests-only`, `safe-fix-contract`).
- Bugfix = reproduce-first: a red test by **assertion** before touching prod.
- Every change ends with a `change-audit/CA-NNN-*.md` entry + commit format
  `[Type][feature] ...` (`audit-logging`, `git-commit-format`).
- Declare scope before mutating (`context-discipline` change-contract block).

## 7. Package → invariant map

| Package | Protect |
|---|---|
| `internal/runner` | dispatch/recovery/session invariants — durability, three-outcome, CAS linearization, pinned provider legs |
| `internal/worktree` | isolation lifecycle — merge-back contract, binding recovery, GC eligibility |
| `internal/flowgate` | gate registration, precedence, fail-closed evaluation |
| `internal/changecontract`, `internal/changeledger` | declared-path enforcement; append-compatible ledger format |
| `internal/promptpacker`, `internal/contextsync` | context budget, dedupe, degrade-soft, deterministic sources |
| `internal/driftdetect` | wrong-way signals → correction ladder, never hard rollback |
| `internal/tournament` | deterministic arbitration; worktree as delegate |
| `internal/skillpack`, `internal/agentpack` | pack data vs Go enforcement boundary |
| `internal/reqscaffold` | doc contract, DoD checklist, intent hard-ceiling |

Run `go test -count=1 ./internal/<pkg>/...` after touching a package; run the
full suite before closing anything cross-cutting.
