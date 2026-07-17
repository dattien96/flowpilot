# SS-17: Dispatch Uncertainty And Repair Operator Contract

## Metadata

- Document ID: `SS-17`
- Title: `Dispatch Uncertainty And Repair Operator Contract`
- Phase: `system_spec`
- Status: `approved`
- Owner: `FlowPilot`
- Reviewers: `Codex review (plan round), user`
- Created: `2026-07-16`
- Last Updated: `2026-07-16`
- Parent Documents: [SS-16 Agent Flow Engine](./SS-16-Agent-Flow-Engine.md), [SS-11 Workflow With Session](./SS-11-Workflow-With_Session.md)
- Child Documents: [SD-24 Durable Turn Dispatch](../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md)
- Related Documents: [BUG-288](../09-BugFix/inprogress/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md), [CP-51](../07-Coding-Plan/todo/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md)
- Replaces: `None`
- Tags: `agent-flow-engine, durable-turn, uncertain, repair-required, operator`
- Feature Keys: `agent-flow-engine`

## AI Quick View

### Summary

- When the runner cannot automatically prove whether a provider received a turn (`uncertain`) or cannot safely load persisted run state (`repair_required`), the run must **hold** and surface a resolvable card to the local operator — never silently retry, never silently drop, never resume with partial state.
- This spec defines the product behavior: what is surfaced, which operator actions exist, their idempotency, audit, and how automated dispatch is blocked until resolution.
- It is the business authority SD-24/CP-51 trace to for `uncertain`/`repair_required` behavior (previously undefined in any SS).

### Current Ask

- Approved on 2026-07-16. SD-24 and CP-51 may now treat this contract as governing product behavior for `uncertain` / `repair_required`.

### Key Decisions

- `BR-1` Hold-and-surface: no auto-terminalize of `uncertain` on a timer; resolution is explicit.
- `BR-2` Retry never mutates history: "retry" creates a **new** dispatch attempt linked to its predecessor; the uncertain record itself only moves to a terminal state via proof or operator decision.

### Constraints

- FlowPilot local-runner is a single-operator desktop tool: the "operator" is the local user; no multi-role authorization matrix is needed (recorded, not assumed, see `BR-6`).
- Must work identically for local-file and Supabase backends.

### Open Questions

- `Q-1` Should a long-held `uncertain` run emit periodic reminders (notification) beyond the card? (UX polish; not blocking.)

### Source Refs

- SS-16 `AC-7`/`BR-6` (bounded + stoppable), SS-11 (session resume). Codex plan-review finding #8 (2026-07-16).

## 1. Goal

Define the user-facing behavior for two failure classes that automation cannot resolve alone:

- **`uncertain` dispatch** — the runner cannot prove whether the provider received/ran a turn (crash between send and durable receipt; provider unqueryable).
- **`repair_required` run** — persisted run state cannot be loaded safely (corrupt/version-mismatched runtime blob).

## 2. Problem

BUG-288 Rounds 1–20 showed that when durability breaks, the runner either guesses (duplicate/lost turns, zero-valued state) or wedges silently. There is no defined product behavior for "the system does not know" — so every engineering fix invents ad-hoc behavior, which the next review round flags again.

## 3. Scope

- Surfacing, operator actions, idempotency, audit, and dispatch-blocking for `uncertain` and `repair_required`.
- Applies to Chat Mode and Flow Mode runs, both storage backends.

## 4. Non-Goals

- The dispatch state machine itself (SD-24).
- Multi-user roles/permissions (single local operator).
- Automatic repair of corrupt blobs (a future enhancement; manual path first).

## 5. Primary Use Cases

- `UC-1` Runner crashes after sending a prompt but before the provider's acceptance is durable → on restart the run holds as `uncertain`; the user inspects and resolves.
- `UC-2` Supabase `session_runtime` for a run is corrupt → the run resumes as `repair_required`, does not run turns, and offers repair/abandon.
- `UC-3` User pressed Stop moments before a crash; recovery finds an uncertain record with a cancel request → resolution defaults to confirm-cancelled.

## 6. Acceptance Criteria

- `AC-1` **Surfaced, identifiable, explained.** An `uncertain` dispatch or `repair_required` run is visible in the UI (and API) with: run/turn identity, when it happened, why automation cannot resolve it, and the exact dispatch envelope summary (prompt reference, provider, model) for `uncertain`.
- `AC-2` **Operator actions for `uncertain`:**
  - *Inspect* — view the envelope, record states, and any provider-side evidence found.
  - *Retry as new* — dispatch a **new** attempt carrying the same immutable envelope, linked to the predecessor; the uncertain record becomes `terminal_cancelled(superseded)`. Never re-sends the old record in place.
  - *Mark completed / Mark failed* — operator asserts the provider outcome (e.g. verified in provider UI); record becomes terminal with `resolved_by=operator`.
  - *Confirm cancelled* — operator confirms that a Stop/cancel won; record becomes `terminal_cancelled(confirmed_cancelled)` and the owning outer intent is cleared. This is the default action when `CancelRequested` or an ancestor Stop fence is present.
  - *Abandon* — record becomes `terminal_cancelled(abandoned)`; the owning outer intent is cleared.
  - Resolution has a closed settlement rule: *Mark completed* and *Mark failed* preserve the record's immutable `settle_owed` decision and start `settle_pending` when it is true; *Abandon* writes `terminal_cancelled(abandoned)` with `settle_owed=false`, so it deliberately performs no gate/graph/finalizer work; *Retry as new* writes the predecessor as `terminal_cancelled(superseded)` with `settle_owed=false` and only the successor may settle. The terminal state, owner-intent mutation, `SettlePhase`, audit, and (for retry) successor are one atomic, idempotent resolution commit.
- `AC-3` **Operator actions for `repair_required`:** *Inspect* (raw quarantined blob reference), *Retry load* (after external fix), *Abandon run*. Normal snapshot writes must not overwrite the quarantined evidence before resolution.
- `AC-4` **Blocking.** While a run has an unresolved `uncertain` dispatch or is `repair_required`: no automated dispatch (resume/reprompt/restart/flow-advance) fires for that run; Stop remains available; other runs are unaffected.
- `AC-5` **Idempotent + audited.** Every resolution action is idempotent (double-click safe: second submit is a no-op returning the first result) and appends an audit record: action, actor (local operator), timestamp, prior state, resulting state.
- `AC-6` **No silent transitions.** The only ways out of `uncertain`/`repair_required` are (a) automated reconciliation that finds durable proof, or (b) an explicit operator action per AC-2/AC-3. Timers alone never resolve them.

## 7. Business Rules

- `BR-1` Hold-and-surface; never guess (see Key Decisions).
- `BR-2` Retry-as-new only; dispatch history is append-only and forward-only.
- `BR-3` A cancel request observed on an uncertain record biases the default action to *confirm cancelled*; retry-as-new then requires explicit override.
- `BR-4` Resolution actions and reconciliation share one idempotent path (same state machine transitions), so an operator action and a late automated proof cannot double-apply.
- `BR-5` Evidence preservation: quarantined corrupt blobs and superseded uncertain records remain readable for audit until the run is deleted.
- `BR-6` Authorization = local operator of this runner instance (single-user tool); recorded in the audit trail as such.
- `BR-7` Terminal proof boundary: a post-send transport error, timeout, process loss, or missing receipt is **not** terminal evidence. Only provider-backed terminal proof may terminalize a sent record; the sole exception is a cancellation proven before `send_started`, which uses the pre-send cancellation transaction. All other cases retain the intent and reconcile or hold as `uncertain`.
- `BR-8` Parent Stop fence: a child created from a parent release inherits the parent run identity and Stop generation. The child may begin sending only if that parent fence is still live in the same durable compare-and-set; a parent Stop that wins prevents the child send and resolves it through the pre-send cancellation path.

## 8. Edge Cases

- Operator marks *completed* while automated reconciliation simultaneously proves *failed* → first durable transition wins (CAS); the loser surfaces as an audit note, not a second transition (`AC-5`/`BR-4`).
- `uncertain` on a child run of a stopped parent → resolution limited to confirm-cancelled/abandon (parent stop generation wins, consistent with BUG-288 P1-04); the child cannot start a successor until a new, explicitly authorized parent release mints a fresh fence.
- Retry-as-new when the outer intent was meanwhile superseded by a newer user prompt → retry is rejected with an explanation (envelope hash mismatch vs. live intent), offering abandon instead.

## 9. Dependencies

- SD-24 dispatch record/state machine (mechanism); CP-51 Task-250 (recovery + resolution wiring), Task-253 (`repair_required` source), desktop UI card surface (reuses the existing approval/question card pattern).

## 10. Open Questions

- `Q-1` Reminder cadence for long-held `uncertain` (UX polish, non-blocking).

## 11. Definition of Done

- The behaviors in `AC-1..AC-6` exist and are covered by tests (unit for state/idempotency; one UI/API smoke for surfacing).
- SD-24 traces its `uncertain`/`repair_required` design to this spec; CP-51 ledger includes rows verifying `AC-4` (blocking), `AC-5` (idempotent resolution), the terminal-proof boundary, parent Stop fence, and the closed settlement outcomes above.
