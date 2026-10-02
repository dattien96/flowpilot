# CA-1137 — BUG-588: premature RUNNING stamp on continue back-edge

## What changed

`apps/local-runner/internal/runner/interactive_service.go`:

- The `continue` round-reset stamps the re-entry node RUNNING before child
  dispatch (Task-304/BUG-286 contract — kept). When dispatch then matched no
  child and spawned none (`continue_backedge_no_child`), the stamp lied:
  live run-139670 showed coder RUNNING for ~3 min while the only live leg
  was tdd.
- New `hasLiveSiblingLeg` + a revert in the no-child tail: when another
  sibling leg is in flight, the premature stamp falls back to PENDING. The
  sibling's own advance re-spawns and re-stamps the node for real.
- When NO sibling leg is in flight the stamp is kept (leg may still be
  provisioning) — the existing oracle tests are preserved.

## Tests

`bug588_premature_running_test.go` — red-first: live sibling leg → revert
to PENDING; no sibling → stamp preserved.
