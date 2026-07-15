-- Migration (BUG-282): drop step_definitions.depends_on_json.
--
-- depends_on_json stored per-flow graph topology (a node's predecessor node
-- ids) on step_definitions — a de-duplicated catalog keyed by step_type and
-- reusable across flows (BUG-164). A step attached to a second flow therefore
-- dragged in the FIRST flow's node ids, which the new flow does not contain,
-- breaking entry/barrier resolution (and, before BUG-262, silently corrupting
-- the other flow on write).
--
-- Topology is authoritative on workflows.edges_json (a flow-scoped column,
-- added by 20260701090000_add_flow_engine_attrs_to_workflows.sql). The Go
-- runner now derives each node's dependency from those edges at flow-load time
-- (forwardEdgeSources in flow_executor.go), so this column is fully redundant.
-- Removing it makes a step definition genuinely flow-independent.
--
-- Introduced by 20260703160000_move_flow_node_definition_to_step_definitions.sql.
-- No data migration is needed: the equivalent information already lives on each
-- referencing workflow's edges_json, which the code reads instead.

alter table public.step_definitions
  drop column if exists depends_on_json;
