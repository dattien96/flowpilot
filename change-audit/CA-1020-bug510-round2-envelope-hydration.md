# CA-1020 — BUG-510 round 2: hydrate model/yolo on the envelope

## Context

Round-2 review: the durable fallback copied only `ProviderKey`; `Model`
and `YoloMode` stayed empty even though `ProviderSessionState` carries
`ModelName`/`Yolo`.

## Changes

- `interactive_handlers.go` `workflowStepsRuntime`: the durable-session
  fallback now populates `runModel`/`runYolo` from `sess.ModelName` /
  `sess.Yolo`. Step-level values continue to come from the durable
  workflow-step rows.

## Tests

New `TestBug510_StepsRuntimeHydratesModelAndYolo` — envelope model/yolo
hydrated, step model preserved, unknown run still 404s. Green.
