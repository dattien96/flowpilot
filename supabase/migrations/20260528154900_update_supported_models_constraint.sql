alter table step_definitions
  drop constraint if exists step_definitions_supported_model_check;

alter table step_definitions
  add constraint step_definitions_supported_model_check
  check (
    model in (
      'auto-gemini-3',
      'auto-gemini-2.5',
      'gemini-3.1-pro-preview',
      'gemini-3-flash-preview',
      'gemini-3.1-flash-lite-preview',
      'gemini-2.5-pro',
      'gemini-2.5-flash',
      'gemini-2.5-flash-lite',
      'gemini-flash',
      'gemini-pro',
      'claude-haiku',
      'claude-sonnet',
      'claude-opus',
      'gpt-5.4-mini',
      'gpt-5.4',
      'gpt-5.5'
    )
  );
