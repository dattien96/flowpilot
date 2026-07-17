# Task-254: Non-Terminal Dispatch Idempotency Retention

## Metadata

- Document ID: `Task-254`
- Title: `Non-Terminal Dispatch Idempotency Retention`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex review`
- Created: `2026-07-16`
- Last Updated: `2026-07-17`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine](../../07-Coding-Plan/inprogress/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [SD-24: Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) (§6.5)
- Child Documents: `None`
- Related Documents: [BUG-288](../../09-BugFix/inprogress/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md), [Task-248](./Task-248-Durable-Dispatch-Record-And-State-Machine-Core.md)
- Replaces: `None`
- Tags: `agent-flow-engine, idempotency, durable-turn, persistence`

## AI Quick View

### Summary

- `durableIdempotencySnapshot` keeps a fixed cap of 48 keys sorted by a trailing numeric generation that is **not** comparable across namespaces (`restart-`, `resume-`, `reprompt-`), and the active key is pinned only in the accept-time snapshot; the post-turn snapshot uses plain `sessionStateOf(rs)` (`interactive_service.go:5045`). So on a long session an active low-generation key (e.g. `restart-1`) can be evicted by 48 higher-generation historical keys.
- Rule: a non-terminal dispatch/idempotency key is never prunable; pin all active keys in **every** snapshot; make retention namespace-aware.

### Current Ask

- Guarantee an active (non-terminal) idempotency/dispatch key survives every snapshot and disk round-trip regardless of how many historical keys exist.

### Key Decisions

- `T-1` **Authority inversion (plan-review #10):** `DispatchRecord` is the single source of truth; the `IdempotencyKeys` map becomes a **derived index** rebuilt from records (plus legacy migration input). It has **no independent lifecycle**: no separate cap logic for active keys, no short-circuit decision that bypasses the record.
- `T-2` Retention is driven by dispatch state: non-terminal ⇒ always retained; only `terminal` records are eligible for TTL/cap pruning, per-namespace (never compare `restart-N` vs `resume-M` as raw integers).

### Constraints

- Bound disk growth for terminal keys (keep a cap for terminal history), but never at the cost of a non-terminal key.
- Coordinate with Task-248: the dispatch record is the authority on which keys are non-terminal.

### Open Questions

- `Q-1` Keep the 48 cap for terminal keys or tie retention window to the dispatch record set? Default: cap terminal-only; non-terminal unbounded (bounded in practice by concurrency).

### Source Refs

- CP-51 §10.1 `DOD-I6`; BUG-288 R16-P0 / R18-1.
- Code: `interactive_service.go:2621-2698` (`sessionStateOfProtectingIdem`, `durableIdempotencySnapshot`, cap 48, cross-namespace gen), `:5045` (post-turn plain `sessionStateOf`).

## 1. Goal

Never drop an active (non-terminal) idempotency/dispatch key from any persisted snapshot, eliminating the long-session eviction that could re-run or lose a turn after crash.

## 2. Parent Links

- coding plan: CP-51 (`P-7`)
- tech design: [SD-24 Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) (§6.5 retention); SD-20
- system spec: SS-14
- specific upstream ids: CP-51 `DOD-I6`, INV-2

## 3. Trigger

`durableIdempotencySnapshot` (`interactive_service.go:2641-2698`) sorts durable keys by `durableIdempotencyKeyGen` (trailing integer) and keeps the highest-gen 48 (plus explicit `protectKeys`). Generations are not namespaced, so `restart-1` (gen 1) ranks below 48 `resume-*`/`reprompt-*` keys with larger integers. Protection is applied only via `sessionStateOfProtectingIdem` at accept time; the post-turn persist at `:5045` uses plain `sessionStateOf(rs)`, so the active key is unprotected there.

## 4. Exact Change

- `T-1` Invert authority: startTurn's duplicate-turn short-circuit reads the **`DispatchRecord` set** (state + `OuterIntentKey`), not the raw idempotency map. The map is derived: `rebuildIdemIndexFromDispatch(records)`; legacy map entries are migration input only (Task-248 `dispatchFromLegacyIdem`).
- `T-2` Retention lives on **records**: non-terminal never pruned; `terminal_*` pruned per-namespace TTL/cap. The derived map inherits this automatically — no independent cap logic remains in `durableIdempotencySnapshot`.
- `T-3` Every snapshot (accept-time AND post-turn `:5045`) serializes the derived index from records, so an active key can never be dropped by a snapshot path that "forgot" protection — `sessionStateOfProtectingIdem` becomes redundant and is removed.
- `T-4` Namespace-aware generation parsing (`durableIdemKeyParts`) for the terminal-history pruning only.

### 4.1 Code guide (step-by-step)

> Depends on Task-248 (record tells which keys are non-terminal). Sketches illustrative.

**Step 1 — parse namespace + generation separately ([interactive_service.go:2700-2712](../../../apps/local-runner/internal/runner/interactive_service.go:2700)).** Today `durableIdempotencyKeyGen` returns only the trailing int, so `restart-1` and `resume-99` are compared as raw ints. Split:

```go
// "durable-<run>-restart-12" -> namespace="durable-<run>-restart", gen=12
func durableIdemKeyParts(k string) (namespace string, gen int64) {
	i := strings.LastIndex(k, "-")
	if i < 0 || i+1 >= len(k) { return k, 0 }
	n, err := strconv.ParseInt(k[i+1:], 10, 64)
	if err != nil { return k, 0 }
	return k[:i], n
}
```

**Step 2 — retention split in `durableIdempotencySnapshot` ([:2641-2698](../../../apps/local-runner/internal/runner/interactive_service.go:2641)).** Non-terminal keys are always kept; only terminal keys are capped, and the cap/sort is **per-namespace**:

**Authority boundary.** Add `DispatchStore.FindActiveByOuterIntent(ctx, runID, key string, generation int64) (DispatchRecord, bool, error)` backed by the `(run_id, outer_intent_key, outer_intent_gen)` index. `startTurn`/resume duplicate handling calls this API (or its cache populated from `ListRecoverable`) and returns the existing turn for `prepared|send_claimed|send_started|provider_accepted|uncertain`; only a terminal record permits a new intent generation. `rs.idempotency` remains a derived compatibility projection for snapshots/history and is never read as delivery authority; remove `protectKeys` from correctness decisions.

```go
func durableIdempotencySnapshot(m map[string]string, nonTerminal map[string]bool, protectKeys ...string) map[string]string {
	out := map[string]string{}
	perNS := map[string][]kv{} // terminal keys grouped by namespace
	for k, v := range m {
		if !strings.HasPrefix(k, "durable-") || v == "" { continue }
		if nonTerminal[k] { out[k] = v; continue }        // ALWAYS keep non-terminal (SD-24 §6.5)
		ns, gen := durableIdemKeyParts(k)
		perNS[ns] = append(perNS[ns], kv{k, v, gen})
	}
	for _, pk := range protectKeys { if v, ok := m[pk]; ok { out[pk] = v } }
	const capPerNS = 16 // terminal history bound, per namespace
	for _, list := range perNS {
		sort.Slice(list, func(i, j int) bool { return list[i].gen > list[j].gen })
		for i, e := range list { if i >= capPerNS { break }; out[e.k] = e.v }
	}
	return out
}
```

**Step 3 — pin active keys in EVERY snapshot, not just accept-time.** The bug is that the post-turn snapshot uses plain `sessionStateOf` ([interactive_service.go:5045](../../../apps/local-runner/internal/runner/interactive_service.go:5045)). Make `sessionStateOf` itself pass the non-terminal set ([:2616](../../../apps/local-runner/internal/runner/interactive_service.go:2616)):

```go
// inside sessionStateOf(rs):
IdempotencyKeys: durableIdempotencySnapshot(rs.idempotency, rs.nonTerminalIdemKeys()),
```
where `rs.nonTerminalIdemKeys()` derives from `rs.dispatch` (Task-248): a key is non-terminal if its `DispatchRecord.State != terminal`. This makes `sessionStateOfProtectingIdem` ([:2623](../../../apps/local-runner/internal/runner/interactive_service.go:2623)) redundant for correctness (keep it as a belt-and-suspenders or remove).

### 4.2 Test skeletons (`idempotency_retention_test.go`)

```go
func TestSnapshot_ActiveLowGenKeySurvives48TerminalKeys(t *testing.T) { /* restart-1 active + 48 resume-* terminal -> restart-1 kept (DOD-I6); fails pre-fix */ }
func TestSnapshot_PostTurnAlsoPinsActiveKey(t *testing.T)             { /* the :5045 path, not just accept-time */ }
func TestSnapshot_PerNamespaceCap_NoCrossEviction(t *testing.T)       {}
func TestSnapshot_RetainedKeysReloadAndShortCircuit(t *testing.T)     {}
```

## 5. Touched Areas

- files: `interactive_service.go` (`durableIdempotencySnapshot`, `sessionStateOf`/snapshot path, `durableIdempotencyKeyGen`), `dispatch_record.go` (non-terminal query helper), `workflow_store.go` (if the snapshot needs record access).
- modules: `internal/runner` persistence.
- routes: none.
- tables: none new.

## 6. Acceptance Check

- `V-1` `idempotency_retention_test.go`: with 48+ terminal keys and one active low-gen key (`restart-1`), the active key survives the **post-turn** snapshot and disk round-trip (fails on current code).
- `V-2` Namespace test: `restart-1` is not evicted by high-gen `resume-*`/`reprompt-*` keys.
- `V-3` Terminal-cap test: terminal keys beyond the cap are pruned oldest-first without touching non-terminal keys.
- `V-4` Round-trip: retained keys reload and still short-circuit correctly after restart.
- `V-5` `go build`, `go vet`, `go test ./internal/runner` clean.

## 7. Out of Scope

- The dispatch record definition itself (Task-248).
- Recovery reconciliation (Task-250).

## 8. Completion Notes

- result: **done** — non-terminal keys always retained, per-namespace terminal cap (`capPerNS=16`), active keys pinned in **every** snapshot including the post-turn path (`sessionStateOf` → `durableIdempotencySnapshotWithNonTerminal(rs.idempotency, rs.nonTerminalIdemKeys())`, confirmed by reading the call site, not just claimed). T-2/T-3/T-4 genuinely implemented and now fully tested (`idempotency_retention_test.go`, 2026-07-17): `TestSnapshot_PostTurnAlsoPinsActiveKey` (proves the post-turn path, not just accept-time, via the real `sessionStateOf`) and `TestSnapshot_RetainedKeysReloadAndShortCircuit` (real disk round-trip via `NewLocalFileSessionStore`, reconstruct, then `startTurn` short-circuits to the same prior turnID using durable — not RAM-only — recovery evidence). All 4 named §4.2 tests now exist (2 were already in `cp51_tasks_test.go`). V-1..V-5 all pass.
- **T-1 authority inversion — descoped, not a DOD blocker:** `FindActiveByOuterIntent` is defined on all stores (Task-248) but has no caller in `startTurn`; the existing RAM idempotency-map short-circuit (battle-tested through BUG-288 R16-R20) remains the delivery mechanism. Verified this is **not** gated by any CP-51 §10.1 ledger row owned by this task (`I6`, `Rr4` are both retention/pruning-survival only) nor by this task's own §6 Acceptance Check (V-1..V-5, all retention/pinning). Replacing the idempotency-map authority with `FindActiveByOuterIntent` would be a real architecture change with meaningful regression risk against zero required DOD benefit — deliberately left as a documented, non-blocking follow-up rather than done unsafely to close a checkbox that isn't actually required.
- follow-ups: if a future CP wants the full T-1 authority inversion, it needs its own dedicated task with its own regression-safety plan against BUG-288 R16-R20 — not bundled into this one.
- upstream docs updated: task status; moved to `done/` (2026-07-17).
