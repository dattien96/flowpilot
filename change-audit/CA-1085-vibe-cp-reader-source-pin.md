# CA-1085: cp_reader prompt names the run's pinned CP file

Date: 2026-10-01
Refs: user-reported — armed vibe-tasks Start flow picked a CP, fence passed,
flow launched, but the spawned agent asked the user for the requirement:
"có thực sự là runner đã receiver CP file"

## Symptom

Live run-2232 / child run-2258 (14:44): user armed `vibe-tasks` with a CP
picked in the desktop dropdown, chatted "hi", then Start flow. The forward
fence passed and `cp_reader` spawned `vibe-intake` — whose prompt (verified
verbatim in the ACP log, 1306 bytes) contained only the generic
`vibe-intake.md` agent definition ("Read the raw requirement file or pasted
idea… produce SS drafts and a sprint plan") plus the "hi" transcript. No CP
path, no binding. The agent globbed `requirements/`, found "no obvious
intake file", and called `ask_user` for the requirement it was already
given.

## Root cause

Pack-data gap, not a transport gap. The runner DID receive the CP:

- `rs.sourceDocID` stamped from the armed create / forward turn; the CP
  fence (`validateVibeCpIngestSource`) would have 422'd the forward
  otherwise — the flow launched, so validation passed.
- At launch commit `rs.vibeLockedCP = rs.sourceDocID` for CP-sourced flows
  (interactive_service.go), so `vibeResolvedSlicerSource` already returns
  the pinned path before `cp_reader` spawns.

But `cp_reader` in both `vibe-tasks.yaml` and `vibe-cp-ingest.yaml`
declared no `promptTemplate` and no `artifactBindings` — so
`appendResolvedVibeTemplatedInputs` (BUG-469's resolved-source seam) found
zero `pathTemplate` slots and emitted nothing, and `appendStaticNodePrompt`
had no template to append. Sibling nodes (`task_plan_reader`,
`task_slicer`) already carry the `cp_md` input binding + templates; the
entry reader was the one node left out.

## Fix

Pack data only; no runner-code change.

- New `prompts/vibe-cp-read.md` (manifest-listed): tells the intake agent
  it is `cp_reader` of a CP-sourced flow — read the pinned file named in
  the resolved `cp_md` binding, report the SS-13 contract shape for
  `cp_validator`, minimally fix contract gaps on bounce-back, write
  nothing else, fail plainly if the pin is missing.
- `cp_reader` in `vibe-tasks.yaml` + `vibe-cp-ingest.yaml`: added
  `promptTemplate: prompts/vibe-cp-read.md` and the `cp_md`
  file_artifact INPUT binding (`pathTemplate:
  requirements/07-Coding-Plan/**/CP-*.md`, matching the fence's accepted
  shape).

With the binding declared, `appendResolvedVibeTemplatedInputs` appends
"Bound input artifacts — resolved for this run: `cp_md`: `<exact path>`"
to the entry spawn prompt — the exact mechanism that healed the slicer in
BUG-469. Mirror seeding needs no code change:
`SeedBuiltinHarnessArtifactBindings` walks `record.Definition.Nodes`
bindings; `cp_md` maps to seeded instance `…0003`; both flows are already
in `builtinHarnessArtifactFlowIDs`, and `recordFromWorkflowRow` restores
pack-declared bindings on stale mirrors (BUG-469).

`vibe-ingest` untouched — its `ingest_reader` legitimately receives a raw
idea, not a pinned CP.

## Tests

`ca1085_vibe_cp_reader_source_pin_test.go` (reproduce-first red → green):

- `TestCA1085_CpReaderEntryPromptNamesPinnedCP` — composes the entry
  prompt exactly like flow_executor (`composeFlowNodeAgentPrompt` +
  `appendResolvedVibeTemplatedInputs`) for both flows' `cp_reader`;
  failed before (prompt was bare "hi"), now contains the pinned path.
- `TestCA1085_CpReaderDeclaresCpMdInputAndPromptTemplate` — asserts the
  `cp_md` file_artifact input binding with `pathTemplate` and a
  promptTemplate that resolves via `LoadBuiltinPrompt`.

All BUG-469 tests still pass; `go vet` clean; full runner suite in
verification.
