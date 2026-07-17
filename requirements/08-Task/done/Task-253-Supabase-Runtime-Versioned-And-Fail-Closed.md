# Task-253: Supabase Runtime Versioned And Fail-Closed

## Metadata

- Document ID: `Task-253`
- Title: `Supabase Runtime Versioned And Fail-Closed`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex review`
- Created: `2026-07-16`
- Last Updated: `2026-07-17`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine](../../07-Coding-Plan/inprogress/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [SD-24: Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) (§6.5), [SS-17: Dispatch Uncertainty And Repair Operator Contract](../../05-System-Specs/SS-17-Dispatch-Uncertainty-And-Repair-Operator-Contract.md) (AC-3/AC-4)
- Child Documents: `None`
- Related Documents: [BUG-288](../../09-BugFix/inprogress/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md)
- Replaces: `None`
- Tags: `agent-flow-engine, supabase, crash-recovery, fail-closed`

## AI Quick View

### Summary

- `applySessionRuntime` swallows a JSON unmarshal error and returns, leaving every recovery field (topology, gate state, cohort, pending intents) at zero-value (`supabase_workflow_store.go:134-141`), and `sessionRuntimeBlob` has **no version field** (`:60-103`). A corrupt or version-mismatched blob silently degrades a run to an empty shell.
- Add an explicit `SchemaVersion` and make decode failures / version mismatches **fail closed** (`repair_required`), never fail-open.

### Current Ask

- Version the Supabase `session_runtime` blob and make `applySessionRuntime` fail closed with a surfaced `repair_required` state.

### Key Decisions

- `T-1` Validation is three-layer (plan-review #12): (1) **version** — explicit supported-version set + migration chain; negative/unknown rejected; (2) **presence** — required fields for the claimed version must exist; (3) **semantic** — state combinations must be legal (e.g. `PendingFlowGateSettle` requires a turn id; dispatch states per SD-24 §5.2). Any layer failing ⇒ `repair_required` (SS-17 AC-3).
- `T-2` **Quarantine + no-overwrite**: the raw corrupt blob is preserved (quarantine copy) and normal snapshot writes must not overwrite it before operator resolution; automated dispatch on the run is blocked (SS-17 AC-4).
- `T-3` Co-phased with Task-248 (shared `schema_version`/`ProtocolVersion` constants + the same repair surfacing) — not an independent slice (plan-review #15).

### Constraints

- Backward compatible: existing production blobs (unversioned) must still restore when intact.
- Mirror the version onto the local store's dispatch/runtime persistence where applicable so both backends agree (coordinate with Task-248).

### Open Questions

- `Q-1` Should `repair_required` be auto-recoverable from the raw event log, or strictly operator-driven? Default: surface + hold; auto-repair is a follow-up.

### Source Refs

- CP-51 §10.1 `DOD-I5`, §6; BUG-288 R? (Supabase recovery).
- Code: `supabase_workflow_store.go:60-103` (`sessionRuntimeBlob`, no version), `:105-132` (`sessionRuntimeFromState`), `:134-141` (`applySessionRuntime` swallow).

## 1. Goal

Ensure Supabase-backed recovery never silently loses run state: the runtime blob is versioned, and any decode error or version mismatch fails closed into a surfaced `repair_required` state rather than a zero-valued resume.

## 2. Parent Links

- coding plan: CP-51 (`P-6`)
- tech design: [SD-24 Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) (§6.5 Supabase runtime contract); SD-20
- system spec: SS-14
- specific upstream ids: CP-51 `DOD-I5`, INV-4

## 3. Trigger

`applySessionRuntime(sess, raw)` does `if err := json.Unmarshal(raw, &b); err != nil { return }` (`supabase_workflow_store.go:139-141`) — on any malformed blob it returns having applied nothing, so the session keeps zero-value `ActiveFlowNodes/Edges`, `PendingFlowGateSettle`, `FlowCohortID`, `PendingResume*/Reprompt*/Restart*`, etc. `sessionRuntimeBlob` (`:60-103`) carries no version, so a shape change is indistinguishable from corruption.

## 4. Exact Change

- `T-1` Add `SchemaVersion int` to `sessionRuntimeBlob` (`json:"schema_version"`); set it in `sessionRuntimeFromState`.
- `T-2` Change `applySessionRuntime` to return an `error` (or set a `repair_required` marker on `sess`) on unmarshal failure or unknown-major version, instead of silently returning.
- `T-3` `repair_required` is a **durable `RepairRecord`**, not a boolean: every fail-closed branch calls `dispatchStore.OpenRepair(runID, reason, rawBlob, rawHash)` — create-if-absent, ONE commit writing record + `RepairRevision` + **the raw blob itself** (ledger `QB`) + snapshot-writer block + audit (SD-24 §6.7). `sess.RepairRequired` remains only as a display mirror of `GetOpenRepair`.
- `T-4` Legacy: `schema_version` absent ⇒ v0; accept only on clean decode + presence/semantic validation; any error ⇒ `OpenRepair`. Missing blob on a V2-authority run (derived from the dispatch store, not the mirror) ⇒ `OpenRepair`.
- `T-5` Update the caller(s) of `applySessionRuntime` to honor the error/record: a run with an open repair loads surfaced-blocked, never live-but-empty; its snapshot writers are rejected until a `CommitRepairResolution` resolves it.
- `T-6` Resolution path (consumed by Task-256) is **two-phase** (SD-24 §6.7 — a store transaction cannot run this Go loader): `BeginRepairResolution` (CAS-claim + audit + returns the quarantined raw) → run `applySessionRuntime` against that raw **outside** the store → `CommitRepairResolution(resolved_retry_load | failed_still_open | resolved_abandon)`. Failure keeps the record open with the attempt audited; crash between Begin/Commit expires via the attempt TTL; races serialize on the `RepairRevision`/attempt CAS.

### 4.1 Code guide (step-by-step)

> **Co-phased with Task-248** (shared version constants + the `OpenRepair`/`RepairRecord` store ops this task consumes — plan-review #4 #11; NOT independent). Sketches illustrative.

**Step 1 — version the blob ([supabase_workflow_store.go:60-103](../../../apps/local-runner/internal/runner/supabase_workflow_store.go:60)).**

```go
const sessionRuntimeSchemaVersion = 1

type sessionRuntimeBlob struct {
	SchemaVersion int `json:"schema_version"` // NEW — first field
	Label         string `json:"label,omitempty"`
	// ... existing fields ...
}
```

**Step 2 — stamp it in `sessionRuntimeFromState` ([:105](../../../apps/local-runner/internal/runner/supabase_workflow_store.go:105)).**

```go
func sessionRuntimeFromState(s ProviderSessionState) sessionRuntimeBlob {
	return sessionRuntimeBlob{ SchemaVersion: sessionRuntimeSchemaVersion, Label: s.Label, /* ... */ }
}
```

**Step 3 — fail closed in `applySessionRuntime` ([:134-141](../../../apps/local-runner/internal/runner/supabase_workflow_store.go:134)).** Change the signature to return `error` and set a repair marker instead of silently returning:

**Validation matrix (must be code, not placeholders).** v0 accepts only a clean decode of the legacy shape and applies legacy defaults; v1 requires non-empty `RunID`, `Label`, every non-empty `Pending*Prompt` to carry its matching generation and step/owner field, and `PendingFlowGateSettle` to carry the active turn/gate epoch. For both versions reject: more than one of pending resume/reprompt/restart active; `RepairRequired` combined with an active dispatch; a non-empty gate/cohort ID without its topology; and any dispatch/settle fields (they belong only in the dispatch store). `validatePresence` and `validateSemantics` return typed `ErrRuntimePresence`/`ErrRuntimeSemantic`; decode/store-read errors are retryable and do **not** OpenRepair, while those two errors atomically call OpenRepair. Decode into a temporary `sessionRuntimeBlob`, validate fully, then apply all fields in one copy operation — no partial mutation of `sess` is permitted.

```go
// Loader takes the dispatch store so every fail-closed branch can OpenRepair
// durably (plan-review #4 #3).
func applySessionRuntime(ctx context.Context, dispatch DispatchStore, sess *ProviderSessionState, raw json.RawMessage) error {
	if sess == nil {
		return nil
	}
	// V2 GUARD FIRST (plan-review #3 #5 — this check must be reachable, so the
	// old combined early-return is split): the run-level protocol marker lives
	// OUTSIDE the blob (`dispatch_protocol_version` column/field). For a V2 run,
	// a missing/null/{} runtime blob IS corruption/deletion — never "legacy".
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		// Authority = the NON-PRUNABLE activation entry via the store API — NEVER
		// the session mirror field (plan-review #5 #3).
		ver, verr := dispatch.GetRunProtocolVersion(ctx, sess.RunID)
		if verr != nil {
			return fmt.Errorf("cannot determine run protocol version: %w", verr) // retryable load failure, not repair
		}
		if ver >= 2 {
			return markRepair(ctx, dispatch, sess, raw, "runtime blob missing on a V2 run")
		}
		return nil // genuine legacy v0: nothing to apply
	}
	var b sessionRuntimeBlob
	if err := json.Unmarshal(raw, &b); err != nil {
		return markRepair(ctx, dispatch, sess, raw, "decode failed: "+err.Error()) // FAIL CLOSED + durable OpenRepair + quarantine
	}
	// (1) VERSION: explicit supported set — negative/unknown BOTH rejected.
	if !supportedRuntimeVersions[b.SchemaVersion] { // e.g. {0: true, 1: true}
		return markRepair(ctx, dispatch, sess, raw, fmt.Sprintf("unsupported schema_version %d", b.SchemaVersion))
	}
	// (2) PRESENCE: required fields for the claimed version.
	if err := b.validatePresence(); err != nil {
		return markRepair(ctx, dispatch, sess, raw, "missing required fields: "+err.Error())
	}
	// (3) SEMANTIC: legal state combinations (gate settle needs turn id;
	// dispatch states legal per SD-24 §5.2; no contradictory pending intents).
	if err := b.validateSemantics(); err != nil {
		return markRepair(ctx, dispatch, sess, raw, "semantic validation: "+err.Error())
	}
	sess.Label = b.Label
	// ... existing field copies ...
	return nil
}

// markRepair opens a DURABLE RepairRecord via the dispatch store:
// OpenRepair(runID, reason, rawBlob, rawHash) writes the record + RepairRevision
// + THE RAW BLOB ITSELF + snapshot-writer block + audit in ONE commit
// (create-if-absent: a repeat returns the existing revision). The
// sess.RepairRequired flag is only the in-memory display mirror.
// The loader therefore needs the store in scope — applySessionRuntime moves to
// a method (or takes the store): func (s *SupabaseWorkflowStore) applySessionRuntime(
//     ctx context.Context, dispatch DispatchStore, sess *ProviderSessionState, raw json.RawMessage) error
func markRepair(ctx context.Context, dispatch DispatchStore, sess *ProviderSessionState, raw json.RawMessage, reason string) error {
	// THE RAW BLOB ITSELF is persisted atomically with the RepairRecord
	// (quarantine_blob + hash in the same commit — plan-review #5 #1; ledger QB).
	// Inspect / retry-load later operate on this exact payload via GetOpenRepair.
	if _, err := dispatch.OpenRepair(ctx, sess.RunID, reason, []byte(raw), sha256Hex(raw)); err != nil {
		return fmt.Errorf("open repair: %w", err) // fail-closed even here: loader surfaces load failure
	}
	sess.RepairRequired, sess.RepairReason = true, reason // display mirror only
	return fmt.Errorf("session runtime repair_required: %s", reason)
}
```

**Step 4 — add `RepairRequired`/`RepairReason` to `ProviderSessionState`** ([workflow_store.go](../../../apps/local-runner/internal/runner/workflow_store.go)) and make the reconstruct/caller of `applySessionRuntime` honor it: a `RepairRequired` run resumes into a surfaced blocked/repair state (reuse the `IntentBlocked*` surfacing pattern) rather than a live-but-empty run. Update every `applySessionRuntime(...)` caller for the new `error` return.

### 4.2 Test skeletons (`supabase_runtime_corruption_test.go`)

```go
func TestApplySessionRuntime_CorruptBlob_RepairRequired(t *testing.T)   { /* garbage -> repair_required, not zero-value (DOD-I5); fails pre-fix */ }
func TestApplySessionRuntime_VersionUnknownOrNegative_Repair(t *testing.T) { /* v=-1, v=99 — explicit supported set */ }
func TestApplySessionRuntime_MissingRequiredFields_Repair(t *testing.T) { /* presence layer */ }
func TestApplySessionRuntime_ContradictorySemantics_Repair(t *testing.T){ /* settle without turn id; illegal dispatch state combo */ }
func TestApplySessionRuntime_PartiallyValidBlob_Repair(t *testing.T)    { /* valid JSON, half fields — no partial apply */ }
func TestApplySessionRuntime_LegacyV0Intact_Restores(t *testing.T)      {}
func TestApplySessionRuntime_MissingBlobOnV2Run_Repair(t *testing.T)    { /* run-level marker says v2; blob nil/{}/null -> repair_required, NOT legacy */ }
func TestApplySessionRuntime_MissingBlobOnV0Run_OK(t *testing.T)        { /* no marker -> genuine legacy, no repair */ }
func TestRepairRequired_QuarantinePreserved_NoSnapshotOverwrite(t *testing.T) { /* SS-17 AC-3; writer rejected while open */ }
func TestRepairRequired_BlocksAutomatedDispatch(t *testing.T)           { /* SS-17 AC-4; OP1 row */ }
func TestOpenRepair_DurableAcrossRestart(t *testing.T)                  { /* record + revision + quarantine ref survive restart; repeat OpenRepair returns same revision */ }
func TestOpenRepair_QuarantineCrashAtomicity(t *testing.T)              { /* crash mid-open: either no record or complete record+ref — never a half-open state */ }
func TestRepairResolution_RevisionRace_RetryLoadVsAbandon(t *testing.T) { /* RR row: Begin/Commit serialize on RepairRevision/attempt CAS; loser gets ErrStale */ }
func TestRepairResolution_RetryLoadFailure_StaysOpenAudited(t *testing.T) { /* Commit(failed_still_open): record open, attempt audited, idempotent resolutionID */ }
func TestRepairResolution_CrashBetweenBeginAndCommit_AttemptExpires(t *testing.T) { /* TTL supersede; no wedge */ }
func TestSessionRuntime_RoundTripV1_PreservesTopologyGateIntents(t *testing.T) {}
```

## 5. Touched Areas

- files: `supabase_workflow_store.go` (blob, `sessionRuntimeFromState`, `applySessionRuntime`, callers), `workflow_store.go` (optional `RepairRequired` field), reconstruct path in `interactive_resume.go`.
- modules: `internal/runner` Supabase persistence + recovery.
- routes: none.
- tables: Supabase `session_runtime` JSON gains `schema_version` (additive); **consumes Task-248's `repair_records` (incl. `quarantine_blob`/`quarantine_hash`) and `run_protocol_activations`** via `OpenRepair`/`GetOpenRepair`/`Begin`/`CommitRepairResolution`/`GetRunProtocolVersion` (plan-review #5 #9).

## 6. Acceptance Check

- `V-1` `supabase_runtime_corruption_test.go`: a truncated/garbage blob yields `repair_required`, not a zero-value resume (fails on current code).
- `V-2` Version-mismatch test: an unknown-major `schema_version` yields `repair_required`.
- `V-3` Legacy test: an intact unversioned (v0) blob still restores all fields.
- `V-4` Round-trip test: `sessionRuntimeFromState` → `applySessionRuntime` preserves topology/gate/cohort/intents at the current version.
- `V-5` `go build`, `go vet`, `go test ./internal/runner` clean.

## 7. Out of Scope

- Auto-repair from raw event log (follow-up).
- Local-store dispatch persistence internals (Task-248) — only coordinate the version constant.

## 8. Completion Notes

- result: **done (2026-07-17)** — `sessionRuntimeSchemaVersion`/`supportedRuntimeVersions`, `applySessionRuntimeV2` fail-closed layers (decode → version → `validatePresence` → `validateSemantics`, no partial mutation on any failure), `RepairRequired`/`RepairReason`, and store repair ops (`OpenRepair`/`BeginRepairResolution`/`CommitRepairResolution`) exist and are unit-tested (`TestApplySessionRuntime_CorruptBlob_RepairRequired`, `_VersionUnknown_Repair`, `_MissingBlobOnV2Run_Repair`, `_MissingBlobOnV0Run_OK`, `_LegacyV0Intact_Restores`, `TestOpenRepair_CreateIfAbsent_OneCommit`).
- **2026-07-17 scope correction (user):** the earlier audit flagged "`providerSessionFromDBRow` not wired to a real dispatch store" as a gap. It's real as a code fact but **has zero production impact and is not worth pursuing further**: `SupabaseWorkflowStore` has **no production constructor call anywhere** in this codebase (confirmed by repo-wide grep) — CP-51/Task-258 pivoted BOTH dispatch AND session storage to local NDJSON + Drive sync; Supabase is not the active backend for either. Wiring `providerSessionFromDBRow` to a real `DispatchStore` would harden a path nothing currently exercises. Added a minimal, harmless `SupabaseWorkflowStore.SetDispatchStore` scaffold (mirrors `InteractiveService.SetDispatchStore`) for forward-compatibility if Supabase session storage is ever reactivated, but did NOT thread it through the 3 `providerSessionFromDBRow` call sites — that would be speculative work against a dead path.
- The active backend (`localFileSessionStore`, NDJSON) has a structurally different, already-adequate corruption story: it has no nested "runtime blob" to version at all (fields are flat NDJSON columns), and a malformed line is skipped on load (degrades that one run to "not found" rather than needing a repair-marker abstraction) — consistent with the local dispatch store's own torn-tail handling (Task-248). This is not a gap for THIS task, which is explicitly scoped to the Supabase jsonb-blob versioning concern (§7 Out of Scope already excludes local-store internals).
- Also fixed in passing (CA-345): `sessionRuntimeBlob`'s encoder now writes `MarkerProvenanceRunIDs`/`Pending*ProvenanceRunID` (was previously declared+decoded but never encoded — Task-252's finding, fixed there).
- Typed `ErrRuntimePresence`/`ErrRuntimeSemantic` (vs. the current plain `fmt.Errorf`) not added — nothing consumes the type distinction (`applySessionRuntimeV2` treats both identically via `markRuntimeRepair`), so it would be a cosmetic-only change.
- Acceptance: V-1..V-4 pass (existing unit tests exercise the exact corrupt/version-mismatch/missing-blob/legacy-v0 matrix). V-5 (`go build`/`go vet`/`go test ./internal/runner` clean) verified this pass.
- upstream docs updated: task status; moved to `done/` (2026-07-17).
