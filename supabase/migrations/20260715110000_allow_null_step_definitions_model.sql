-- Allow step_definitions.model to be null for inline-scope node behaviors
-- (context.produce, command.validate, artifact.audit_draft, telegram.notify,
-- flow.control, user.confirm, hub.inline, ...). Only agent.delegate (and a
-- step with no behavior_id set at all, i.e. a plain non-flow catalog step)
-- ever spawns a provider turn and actually consumes this field --
-- resolveFlowNodeModel (flow_executor.go) already returns "" for every other
-- behavior at runtime, so this column being NOT NULL only forced the authoring
-- UI to fabricate a meaningless model choice for steps that never use one.
alter table step_definitions
  alter column model drop not null;
