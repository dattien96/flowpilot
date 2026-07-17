# CA-337: SP-05 Kill-Review Closed-Claim Contract

## Summary

Add system principle SP-05 defining terminating (kill) reviews with frozen claims, probe DoD, finding inventory, coverage boundary, and post-kill bug ingress — so plan/code reviews can end without BUG-288-style open acceptance loops. Add `kill-review` agent skill and `requirements/reviews/` home for `KR-*` reports.

## Files

- `requirements/04-System-Principle/SP-05-Kill-Review-Closed-Claim-Contract.md` (new)
- `.agents/skills/kill-review/SKILL.md` (new)
- `requirements/reviews/README.md` (new)

## Notes

- Complements SS-15 (product review-until-clean) by constraining how "clean" is defined.
- Reference implementation of strong closed claim: CP-51 §10.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: SP-05
change_type: docs
summary: Add SP-05 kill-review closed-claim contract, skill, and KR report home
# --->8---
