# CA-651 — vibe-* built-in flows opened for Settings clone

## Scope
Data-only change (pack YAML). No Go/TS logic touched.
Out of scope: `selectableIn` (stays `[]` — still out of pickers),
`editable` (stays `false`), Go heal/restore logic, TS clone logic.

## Evidence
User report: a built-in vibe flow shows no Clone option on the Settings
Workflows page. Root cause chain, all verified by direct read:
- `flow-pack/flows/vibe-*.yaml` declare `builtin.cloneable: false`
  (7 flows: sprint, tasks, cp-ingest, ingest, adopt, adopt-sprint,
  owner-debate), mirrored by 7 `cloneable: false` entries in
  `flow-pack/manifest.yaml`.
- Settings hides the Clone button when `cloneable === false`
  (`apps/desktop-flowpilot/.../WorkflowsSettings.tsx:3065`).
- Both clone backends reject regardless:
  `supabaseAdminRepository.ts:689` ("not cloneable"),
  `flow_definition_resolver.go:266` (CloneBuiltin gate).
- GitNexus impact on `SyncBuiltins` returned UNKNOWN (index behind,
  receiver typing dropped 2 call sites); resolved by text search —
  the only `Cloneable` consumers are the mirror record
  (`flow_definition_resolver.go:241`), the CloneBuiltin gate, and the
  TS UI/backend above. No test pins `cloneable:false` anywhere.

## Fix
- 7 `flow-pack/flows/vibe-*.yaml`: `builtin.cloneable` `false` → `true`.
- `flow-pack/manifest.yaml`: matching 7 entries `false` → `true`
  (agreement enforced at load by `pack.go:1078` /
  `TestLoadPackFSRejectsManifestFlowMetadataDrift`, so a one-sided
  edit fails loudly instead of silently doing nothing).
- Propagation (no action needed): the YAML edit changes the raw content
  hash, so `FlowMirrorSyncService.SyncBuiltins` re-upserts each mirror
  row on next sync (hash-gated, `flow_definition_resolver.go:389`),
  flipping `workflows.cloneable` to true — the Clone button then
  appears and clones emit normal editable user copies.

## Tests
- New `internal/agentpack/vibe_flows_cloneable_test.go`
  (`TestVibeFlowsCloneable`): RED before the flip
  (`vibe-sprint must be cloneable:true`), GREEN after; pins both the
  flow-file and manifest sides for all 7 flows.
- `go test -count=1 ./internal/agentpack/...` — green.
- `go test -count=1 ./internal/runner/...` (5693 tests): 15 failures,
  all attributed, none caused by this change —
  - 12 fail identically on clean main via targeted re-run (pre-existing
    env failures: Firebase e2e, provider session/pool lifecycle,
    catalog-fake fallback, `TestFlowDefinitionStoreForUnconfiguredRunnerYieldsNil`
    — verified that test touches no pack/Cloneable surface; left
    untouched per additive-tests-only).
  - 3 (`BUG368_AbandonLetsNextSprintFreezeNewPaths`,
    `R6_LocalAheadRestoreKeepsFlowArmAndPin`,
    `Task447_ConcurrentClaimsDoNotOverbook`) PASS in isolation with
    this change applied — full-suite order/timing flakes, none
    references `Cloneable` (verified by grep over their files).
