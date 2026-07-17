# CA-339: kill-review skill portable — dual copy, no SP-05 dependency

## Summary

Rewrite `kill-review` skill as self-contained Vietnamese operational guide (BAD/GOOD, protocol matrix, workflow, report skeleton). Identical copies in `.agents/skills/kill-review` and skillpack `flow-pack/common/kill-review` (version 2). Skill must not mention SP-05 so it is shareable to projects without that principle file.

## Files

- `.agents/skills/kill-review/SKILL.md`
- `apps/local-runner/internal/skillpack/flow-pack/common/kill-review/SKILL.md`
- `requirements/04-System-Principle/SP-05-Kill-Review-Closed-Claim-Contract.md` (metadata note only)
- `requirements/reviews/README.md`

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: SP-05
change_type: docs
summary: Portable kill-review skill v2 dual-synced to skillpack without SP-05 dependency
# --->8---
