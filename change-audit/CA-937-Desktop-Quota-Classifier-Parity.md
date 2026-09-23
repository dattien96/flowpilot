# CA-937 — BUG-375: desktop quota classifier synced to runner token set

## Summary

`isUsageLimitMessage` was a stale 6-token subset of the runner's 22-token
`isProviderUsageLimitError`. Grok 402 ("Payment Required") and OpenCode
billing copy ("no payment method") surfaced correctly by the runner still
missed the desktop's account-switch branch, so quota failures degraded to a
generic failed run with no recovery surface — the same silent-death
silhouette reported under CP-81.

Fix: exported the helper and expanded it to the full runner token set with a
parity comment. Pure string predicate — provider-agnostic, single call site
(`consumeStream` `turn_failed` branch), so one classifier covers all
providers.

## Verified

- 2 new tests green (`usageLimitClassifier.test.ts`): 22 runner tokens + 2
  live wire copies match; 6 healthy-error negatives stay clean.
- Pairs with BUG-374 (runner now surfaces the `error.data` detail this
  classifier needs to see).

## Files

- `src/state/store.ts`,
  `src/state/usageLimitClassifier.test.ts` (new),
  `requirements/09-BugFix/todo/BUG-375-…md` (new)

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-375
change_type: bugfix
