# BUG-670 — Expired Question Never Clears `pendingQuestionID` On The Owning Run

## Metadata

- Document ID: `BUG-670`
- Title: `question expiry marks the record expired but leaves run.pendingQuestionID set — leg wedged waiting_question for hours`
- Phase: `bugfix`
- Status: `FIXED — CA-1237`
- Owner: `flowpilot-runner`
- Created: `2025-01-24`
- Last Updated: `2025-01-24`
- Parent Documents: `quota-routing`, `V10R4 P0` (pending-card turn rejection)
- Related Documents: `BUG-564`, `orphan-wait heal paths`
- Tags: `questions, lifecycle, durability, severity:medium-high`

## AI Quick View

### Summary

- `expireQuestion` flips the question record to `expired` but the owning run keeps `pendingQuestionID` and `status=waiting_question`.
- `startTurn` rejects with `awaiting_user` ("resolve it before a new turn") — but the card is dead and can never be answered: catch-22 until a manual `interrupt`.
- Live: `q-802827`/`q-811596`/`q-832954` expired; owning legs sat `waiting_question` **2h40m+ / 4h+** with no expiry propagation to the run row.

### Current Ask

- Question expiry must propagate to the owning run: clear `pendingQuestionID`, transition `waiting_question → running` (or honest `expired` surface), persist, so a new turn/dispatch can re-mint a fresh card.

### Key Decisions

- `V-1` An expired question is terminal — the run must not remain accountable to a dead card.
- `V-2` Mirrored waits (sibling legs via `healMirroredQuestionWaitLocked`) must clear the same way.

### Source Refs

- live run-752742 legs stuck `waiting_question` on expired cards for hours; manual `POST /agent-loop/interrupt` cleared `pendingQuestionID` and revived them.
- `pendingQuestionID` rejection at `interactive_service.go:12062`.

## 1. Issue Summary

Quota-route questions (TTL 30min) expire while the operator is away. The expiry sweep marks the record expired but never touches the owning run: `pendingQuestionID` stays set and `status` stays `waiting_question`. `startTurn` then refuses everything with `awaiting_user` — a state the operator can no longer resolve because the card is dead. Recovery required manually interrupting each leg.

## 3. Environment and Reproduction

- environment: any run that mints a TTL question (quota route, decisions) and outlives the TTL
- reproduction: mint question → let TTL elapse → question record `expired` → `run.pendingQuestionID` still set → `startTurn` returns 409 `awaiting_user`.
- frequency: deterministic per expired question

## 4. Expected vs Actual

- expected: expiry unblocks the run — next turn can mint a fresh question or proceed
- actual: run wedged until manual interrupt

## 5. Impact

- severity: medium-high — silent multi-hour deadlock; combined with quota gates it strangles flow progress overnight

## 6. Root Cause

Two compounding gaps (verified in code):

- **Gap A — in-memory clear is not persisted.** `expireQuestion` (`interactive_service.go:10159`) clears `rs.pendingQuestionID` and flips `waiting_question → running` in memory, and persists the *question* snapshot — but never `persistProviderSession` on the run. Its sibling `clearPendingQuestion` (`:10241,10251`) DOES persist the run snapshot. So a restart/eviction before any other run-row write resurrects the stale `pendingQuestionID` + `waiting_question` from the durable session row.
- **Gap B — restore never normalizes a pointer to a non-pending record.** The rehydrate loop (`:6600-6678`) skips durable question rows whose status ≠ `pending` at `:6604` — including `expired` — so the expired-normalization at `:6637-6654` never sees them (that branch only fires for rows that were still `pending` on disk but past TTL). The `gotPendingQuestion==false` heal at `:6689` only clears when `pendingQuestionID==""` — a stale pointer to an expired record matches nothing and the run restores permanently wedged.

## 7. Fix Strategy

- `F-1` `expireQuestion`: persist the run session snapshot after clearing `pendingQuestionID`/status (mirror `clearPendingQuestion`'s `runSnap` path).
- `F-2` Rehydrate/restore: after the durable question scan, if `rs.pendingQuestionID` does not correspond to a durable `pending` record, clear it and flip `waiting_question` off (extend the `:6689` heal to cover stale-pointer, not only empty-pointer).
- `F-3` Additive tests: (a) expire while run resident → durable run row shows cleared pointer after restart; (b) session row restored with `pendingQuestionID=<expired-id>` → restore clears pointer + wait.
