# BUG-631 — `extend-cap` grants are wiped: arm path reseeds `st.Cap` to policy default on every mount

- **ID:** BUG-631
- **Severity:** Medium-High — user/operator `extend-cap` decisions are
  silently discarded; cap-blocked escalations re-fire instead of
  resuming with the granted headroom
- **Status:** open
- **Found:** live run-150388, 2026-10-02/03 — loop state showed
  `extendCount:31` yet `cap:5` (policy default), i.e. ~31 granted
  extensions, zero effect

## Symptom

`policy.extendBy`/`extendMax` exists so an operator can raise a loop's
round cap when escalation fires (debate cap 5 → +2 → up to extendMax).
Observed live: repeated `extend-cap` operations incremented
`extendCount` in loop state, but the effective `cap` stayed at the
yaml `policy.cap` value — every mount/arm of the loop re-seeded
`st.Cap = record.Definition.Policy.Cap`, discarding the extension.

## Defect

The granted extension lives in a counter (`extendCount`) but the
**effective cap is recomputed from the flow definition on mount**
instead of being carried in durable loop state. Two invariants collide:

- "cap is a policy read from yaml at mount time" (correct for fresh mounts)
- "an operator's extend decision survives remount" (broken — the
  decision is counted but never applied)

Net effect: the `escalate → extend → continue` operator path is a
dead letter — the loop re-arms at the same cap and re-blocks on the
same boundary.

## Expected fix direction

- Persist the effective cap (or `capOverride`) in durable loop state at
  `extend-cap` time; mount/arm must honor `state.Cap` over
  `definition.Policy.Cap` when an override exists.
- `extendMax` enforcement should read cumulative grants, not just
  increment a counter that nothing consumes.
- Add a regression test: extend → remount → assert effective cap =
  granted cap, not yaml default.
