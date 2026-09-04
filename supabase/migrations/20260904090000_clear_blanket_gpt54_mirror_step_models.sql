-- BUG-352 follow-up: clear the legacy blanket `gpt-5.4` stamped on mirrored
-- built-in step rows back to NULL (= inherit the run's own provider/model).
--
-- Why this exists: an old seed stamped `model='gpt-5.4'` across nearly every
-- mirrored `flowpilot_core_flow_pack_*` row (all behaviors: audit, context,
-- freeze, validate, synthesis, delegate — including the CP-58 harness flows).
-- That was never intentional per-node tiering; the designed default is empty
-- = inherit (Task-320 precedence: admin row > pack YAML > inherit), and the
-- blanket value actively harms: delegate rows pin children to a model the
-- account may not have (CA-616 family), and non-delegate rows tripped
-- ValidateFlowDefinition's fail-closed rule before BUG-352.
--
-- Scope is deliberately narrow: only mirrored built-in rows
-- (`flowpilot_core_flow_pack_*`) carrying exactly `gpt-5.4`. Intentional
-- tiering is preserved — rag-harness / context-coding grok-4.5 / claude-*
-- mixes, review-loop grok-4.5 synthesis, versioned `gpt-5.4-mini` pins, and
-- every non-mirror row (clones, my-*, grok-*, test rows).
--
-- Safe to re-run (idempotent): mirror sync never writes `model`
-- (upsertNodeStepDefinitions omits it, BUG-249 principle), so cleared rows
-- stay cleared and a second run matches zero rows.
update step_definitions set model = null
where step_type like 'flowpilot\_core\_flow\_pack\_%' escape '\'
  and model = 'gpt-5.4';
