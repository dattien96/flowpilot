-- CP-05-05 Task-232: third built-in artifact type, `telegram.v1`, the first
-- OUTPUT-only "action" artifact (not context/file) — a bound step must send
-- a Telegram notification via the Telegram MCP tool (`send_message`), not
-- read/produce content. Verified by a message_id present in the tool-call
-- response, not filesystem existence (SD-11 §3.3 amended 2026-07-13; CP-05-05
-- D-8-equivalent semantics).
insert into artifact_types (
  id,
  version,
  category,
  producer_behavior,
  consumer_hints,
  config_schema,
  render_template,
  system_owned,
  status
) values (
  'telegram.v1',
  1,
  'notify',
  '',
  '{"promptSection": "notify", "directions": ["output"]}'::jsonb,
  '{"type": "object", "required": ["chatId"], "properties": {"chatId": {"type": "string", "description": "Telegram chat or channel id to send the notification to."}, "messageTemplate": {"type": "string", "description": "Optional message template; free-form text if omitted (CP-05-05 Q-5)."}}}'::jsonb,
  'telegram_notification',
  true,
  'active'
)
on conflict (id) do update
  set version = excluded.version,
      category = excluded.category,
      producer_behavior = excluded.producer_behavior,
      consumer_hints = excluded.consumer_hints,
      config_schema = excluded.config_schema,
      render_template = excluded.render_template,
      system_owned = excluded.system_owned,
      status = excluded.status,
      updated_at = now();
