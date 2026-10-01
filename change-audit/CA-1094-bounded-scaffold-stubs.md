# CA-1094: scaffold/TDD leg could rewrite pre-existing production files

Date: 2026-10-01
Refs: live production run-3362 (vibe-tasks, PrivateVault) — the `tdd` leg
rewrote existing prod files (`settings.gradle.kts`, `libs.versions.toml`,
`build.gradle.kts`, all present at base `39751f2`) because the frozen contract
bound the full `declared_paths` scope to the scaffold writer.

## Design decision (operator-approved)

Bounded stubs — keep the CP-67 contract-first design but bound it: the
scaffold may CREATE declared paths that did not exist at freeze (stub bodies
on the existing whitelist), and must NEVER modify a declared path that already
existed. Existing files belong to the coder sibling.

## Fix

1. **Freeze-time classification** (`internal/runner/scaffold_bounded_stub.go`):
   `scaffoldBoundedReadOnly(workspace, writerNode, declaredPaths)` selects
   declared paths that exist on disk; `runContractFreezeNode` and the sibling
   bind write them into the scaffold record's `ReadOnlyPaths` — the coder's
   record is untouched.
2. **Approval bridge**: `decideScaffoldPreExistingLock` denies writes and
   mutating exec commands aimed at those paths for the scaffold child —
   provider-neutral, evaluated before YOLO/auto-approval
   (`interactive_service.go`).
3. **Post-turn gate** (`internal/flowgate`): `TurnResult.
   ScaffoldPreExistingTouched` + `r-scaffold-red` extension — a Bash-bypassed
   write to a locked pre-existing path still fails the turn even under the
   red-suite/zero-red waiver.
4. **Amend preservation** (`internal/changecontract/frozen_scope.go`):
   `amendFrozenContractUnion` now carries `ReadOnlyPaths` forward — a scope
   widen can no longer silently drop the lock.
5. **Prompt** (`flow-pack/prompts/scaffold-contract-tdd.md`): "A declared file
   that already exists on disk is READ-ONLY for you … the coder owns all
   modifications to existing files."

## Verification

- Red→green: `scaffold_bounded_stub_test.go` (freeze marks pre-existing
  declared paths read-only for scaffold only; bridge denies prod write +
  mutating shell, allows read + new-file write) and
  `flowgate/scaffold_preexisting_test.go` (post-turn gate blocks touched
  pre-existing paths, fails closed under waiver).
- `go test ./internal/flowgate ./internal/changecontract` — green; runner
  bounded-stub suite green.
