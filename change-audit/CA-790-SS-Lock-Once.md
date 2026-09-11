# CA-790 — SS review loops before lock; lock is once

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: validator continue loops converter before lock; ss_lock/cp_lock have no continue back-edge; seal on lock clears pending hub reinvoke and refuses second park
# --->8---

## Why

Operator contract: loop only at review (converter ↔ validator) until OK; lock once; never failed→review→lock again.

Reviewer FAIL: the only continue back-edge was ss_lock→ss_converter, so validator changes_requested (and post-lock hub reinvoke) re-entered at the lock.

## Change

- YAML: `ss_validator --continue--> ss_converter`; remove `ss_lock --continue`. Same for cp_validator/cp_lock.
- `resumeVibeLock` seals (`vibeSSSealed`/`vibeCPSealed`), clears `pendingHubReinvoke` and `activeHubNodeID`.
- `parkVibeLock` no-ops after seal.
- `maybeAutoReinvokeHubWithNote` skips when the validator hub is sealed.
- Continue reset does not PENDING lock nodes.
- Writer failure does not revive the validator hub.

## Tests

`ca790_vibe_lock_once_edges_test.go`, `ca790_ss_lock_once_test.go`. Old tests untouched.

## Providers

Agnostic: topology + lock seal, no `providerKey` branch.

## Will not undo

CA-788 terminal-done claim. CA-789 writer not a review cohort. CA-777 first lock park.
