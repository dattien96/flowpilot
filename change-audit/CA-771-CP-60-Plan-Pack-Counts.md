# CA-771 — CP-60/SD-24 plan counts match builtin pack + DefaultRules

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: docs
summary: Align CP-60/SD-24/Task-321/323 pack counts to 12 flows / 8 agents and keep DefaultRules at 18 (r-requirement via EnabledRulesFor)
# --->8---

## Why

Plan prose counted only the Vibe family (`7 flows` / `7 agents`) and told P-2 to append `r-requirement` to `DefaultRules()` (CA-766 said 22). Oracles: `TestLoadBuiltinPack` = 12/8, `TestDefaultRules` = 18, `r-requirement` not in that list.

## Change

Docs only. No production or test edits.

- CP-60 Goal/P-2/§6/§7/§8/§10
- SD-24 D-2 + §11 unit
- Task-321 T-2 + acceptance
- Task-323 acceptance
- KR-004 O-3 closed

## Will not undo

CA-763..CA-770. `TestDefaultRules` / `TestLoadBuiltinPack` untouched.
