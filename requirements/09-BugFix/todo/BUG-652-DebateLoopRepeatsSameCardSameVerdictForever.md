# BUG-652 — The owner-debate loop has no same-verdict circuit breaker: an unresolved card re-spawns the same owner cohort, produces the same verdict, re-parks on the same card — observed 3+ identical rounds on one task

- **ID:** BUG-652
- **Severity:** High — burns provider quota indefinitely (each round =
  2+ owner legs + synthesis) and can outlast the human attending; the
  loop only broke when the operator repaired the underlying contract
  binding (BUG-651) out-of-band.
- **Status:** FIXED — CA-1236 (2026-10-08): per-entity verdict-signature circuit breaker in restoreVibeFlowAfterDebate parks the loop for a human on identical verdicts (durable VibeDebateVerdictSigs)

## Evidence chain (all live)

1. Task-113 TDD leg parked on a contract/scope violation → owner debate
   → verdict `flag-requirement-change` → reprompt → SAME violation
   (contract still wrong) → debate round 2 → same verdict → round 3.
2. Each round spawned fresh `owner_1`/`owner_2` legs analyzing the same
   evidence to the same conclusion; nothing tracked "this exact verdict
   on this exact card already happened N times".

## Root cause (hypothesis)

The debate mount is keyed to the parked card, but verdict history is not
fed back: no dedup on (card signature × verdict), no escalation after a
repeat count, so a fixable-by-humans-only state re-debates forever.

## Fix direction

- `F-1` Circuit breaker: same (cardSignature, verdict) pair twice →
  escalate to a human decision card instead of another cohort round.
- `F-2` Feed prior verdicts into the debate context so owners see they
  are repeating themselves and must pick a different remediation or
  escalate.

## Regression coverage

- `TestBug652_IdenticalVerdictTwiceEscalates` — 2nd identical verdict →
  human card, no 3rd cohort.
- `TestBug652_DistinctVerdictsAllowed` — different verdicts across
  rounds still permitted (no false trigger).
