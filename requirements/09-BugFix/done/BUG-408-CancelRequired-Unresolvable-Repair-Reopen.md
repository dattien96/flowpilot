# BUG-408: `cancel_required`/`send_started`-after-stop records permanently unresolvable; repair re-opens at rev=1 every boot

## Metadata

- Document ID: `BUG-408`
- Title: `send_started records surviving stop+crash can never be terminalized — resolve/retry-as-new 409, repair abandon doesn't settle record, boot re-opens repair at rev=1 forever`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation](../../07-Coding-Plan/done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md)
- Feature Keys: `dispatch-durability`, `recovery`, `operator-surface`

## AI Quick View

### Summary

- CP51-003: `turn-1307` stayed `send_started` after stop+crash. Boot recovery correctly marks `cancel_required` + opens a repair, but `resolve`/`retry-as-new` both 409 ("requires uncertain got send_started") and `repair-resolution abandon` resolves only the RepairRecord — the DispatchRecord stays non-terminal **forever**.
- Each subsequent boot the scanner re-claims the record and `OpenRepair` overwrites the resolved repair with a fresh `RepairRecord` at **revision 1** — `repair_required`+`cancel_required` attention items recur indefinitely and resolution history is lost (`dispatch-final.ndjson` seq 171 `repair run-1302 open rev=1` after seq 168 `resolved rev=3`).

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** a dispatch record parked in `send_started` at stop/crash time can never be resolved: `resolve` and `retry-as-new` reject 409 (they require `uncertain` state); the repair abandon path terminalizes only the repair object, not the dispatch record it was opened for; and each boot re-opens a fresh repair at rev=1, resurrecting the attention item and losing prior resolution history.
- **Expected:** one of the operator surfaces terminalizes the record (e.g. abandon marks the DispatchRecord `terminal_cancelled`/quarantined), and a resolved repair is not resurrected by the boot scanner.
- **Actual:** permanent attention-list leak — the only "resolution" that exists (repair abandon) does not terminalize the record it was opened for, and boot recovery re-opens repair at rev=1 every restart.
- **Impact:** eternal `repair_required`/`cancel_required` attention entries accumulate forever on any bed that ever had a mid-`send_started` crash; operators cannot clear them; audit/resolution history is overwritten each boot.

## Reproduction

1. Stop a run mid-`send_started`, then SIGKILL the runner (turn-1307 shape on run-1302).
2. Restart → boot recovery marks `cancel_required` + opens repair (correct markers per BUG-289 A2/F-7).
3. Try `resolve` or `retry-as-new` → both 409 `requires uncertain got send_started`.
4. `repair-resolution abandon` → repair resolved rev=3, but the DispatchRecord stays non-terminal.
5. Restart again → scanner re-claims the record; `OpenRepair` writes a fresh RepairRecord at rev=1; attention items recur.

## Root cause

- Operator-state machine gap (`apps/local-runner/internal/runner/dispatch_store_memory.go`): `resolve`/`retry-as-new` gate on `state=="uncertain"` — `cancel_required`/`send_started` records have no legal transition to a terminal state.
- `repair-resolution abandon` commits only the `RepairRecord`; no path writes a terminal outcome onto the underlying `DispatchRecord`.
- Boot scanner re-claims non-terminal records unconditionally and `OpenRepair` overwrites a resolved repair (revision resets to 1) rather than detecting prior resolution.

## Evidence

- `~/fp-beds/lt-evidence/cp51/l51-6b/attention.json`, `l51-6b/attention-after-second-restart.json` — repair re-opened at rev=1 after second SIGKILL+restart.
- `~/fp-beds/lt-evidence/cp51/dispatch-final.ndjson` — seq 168 `repair run-1302 resolved rev=3` followed by seq 171 `repair run-1302 open rev=1`.
- `~/fp-beds/lt-evidence/cp51/l51-6b/resolve-send-started.json`, `retry-send-started.json` — both HTTP 409.
- `~/fp-beds/lt-evidence/cp51/RESULT.md` (BUG-LIVE-CP51-003); bed state intentionally left in place as repro.

## Severity

`medium` — no data loss, but a permanent attention leak and unrecoverable record state that grows noisier each boot; undermines the operator reconciliation surface.

## Completion Notes (implemented 2026-09-23, CA-922)

- Fix: `CommitRepairResolution` on `resolved_abandon` terminalizes every non-terminal DispatchRecord for the run (`terminal_cancelled`, `abandoned,resolved_by=operator`) through the legal edge table with attach revoke + intent clear + durable lines — the boot scanner skips terminal records so the attention leak stops. `OpenRepair` continues the revision sequence (`existing.RepairRevision+1`) instead of overwriting a resolved repair at rev=1.
- Files: `internal/runner/dispatch_store_memory.go`.
- Tests: `TestBug408_RepairAbandonTerminalizesStrandedRecords`, `TestBug408_ScannerDoesNotResurrectResolvedRepair` (stop-armed scanner scan → repair stays resolved).
