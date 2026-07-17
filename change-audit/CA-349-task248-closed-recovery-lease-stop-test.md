# CA-349: Task-248 closed — backend scope reconciled + recovery lease/Stop atomicity test

## Scope

Final Task-248 cleanup: reconcile §5/§6 to the real backend (memory + local NDJSON — Task-258 dropped Supabase for both dispatch and sessions), and close the one real remaining test gap.

## Changes

- `requirements/08-Task/inprogress/Task-248-...md` → `done/`: §5 Touched Areas and §6 Acceptance (V-2/V-6/V-7b) rewritten from "both backends / Supabase / real-PG" to memory + local NDJSON, citing Task-258's DROP migration and Task-253's confirmed zero-production-constructors finding.
- `apps/local-runner/internal/runner/dispatch_record_test.go` — new `TestRecoveryCommitGuards_LeaseAndStopAreAtomic`: proves `CASRecoveryAdvance` enforces the recovery lease (exact `ClaimOwner` + unexpired store-clock TTL, via a deterministic fake clock on `memoryDispatchStore.now`) AND own/parent Stop authority in the same locked transaction — wrong owner, expired lease, and an effective Stop (with an otherwise-valid fresh lease) each independently block the advance with zero state change.
- Confirmed `TestSessionUpsertGuarded_InterleavedClearCannotResurrect` (the other originally-missing test) is moot — it targeted a Supabase RPC dropped with the rest of Supabase dispatch; not rebuilt.
- CP-51 ledger rows `V2A`, `SB` (owned by Task-248 alone) flipped to ✅ — confirmed real tests exist (`TestCreatePrepared_AtomicV2Activation`, `TestActivation_SurvivesCompaction`, `TestSessionState_CarriesNoDispatchRecords`). `SW`/`TA` (co-owned with Task-255) left ☐.

## Verification

- `go build ./...`, `go vet ./internal/runner` clean.
- New test passes; full `go test ./internal/runner/...`: 18 failures, all the confirmed pre-existing/environment-dependent baseline — zero regressions across this entire session's changes (CA-340 through CA-349).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Close Task-248 — reconcile doc to the real memory+local-NDJSON backend, add the missing recovery lease+Stop atomicity test
# --->8---
