# KR-002: A1 child terminal propagation matrix

## Metadata

- Review ID: `KR-002`
- Subject: `A1 child terminal and stop-state propagation`
- Mode: `protocol-matrix`
- Reviewer: `Codex`
- Date: `2026-07-20`
- Fix Policy: `all-in-claim`
- Verdict: `pass after BUG-292`

## Claim Boundary

For an A1 flow stopped from a child-focused desktop view, every locally saved parent/child snapshot must resolve to a terminal display state consistent with the runner's stopped graph. This review does not claim that all possible provider failures or all sub-agent protocols are bug-free.

## Evidence

- `run-11679` persisted parent `cancelled` with loop `stopped`; coder `run-11684` persisted `cancelled` with `parent_stop_gen_seen=1`.
- The stale state was desktop-only: `AgentsPanel` read the cached parent snapshot while focus remained on the child.

## State × Event Matrix

| Saved child state | Stopped graph reports | Required desktop state |
| --- | --- | --- |
| `running` | `cancelled` or stale `running` | `cancelled` |
| `waiting_approval` | stale `waiting_approval` | `cancelled`; no pending approval card |
| `waiting_question` | stale `waiting_question` | `cancelled`; no pending question card |
| `cancelled` | `cancelled` | `cancelled` |
| `completed` | `completed` | `completed` |
| `failed` | `failed` | `failed` |
| parent `running` | loop `stopped` | `cancelled` |
| durable checkpoint error with embedded stopped graph | `stopped` | parent/active snapshots `cancelled` |

## Closed Findings

- `K-1` (fixed): `store.stop()` did not update `_runSnapshots`; returning to main restored a pre-stop `running` snapshot.
- `K-2` (guarded): a stopped parent is authoritative over stale active child summaries during asynchronous cancellation completion.
- `K-3` (fixed): the partial durable-failure response already carried a stopped graph but formerly bypassed snapshot reconciliation.

## Verification

- New snapshot matrix test passed.
- Focused runner stop/interrupt/A1 failure probes passed.
- Desktop TypeScript/Vite/Electron build passed.

## Residual Risk

The matrix is a targeted liveness/display claim. A manual A1 live retest is still required to verify the rebuilt desktop process receives the new bundle and the visual transition occurs end-to-end.
