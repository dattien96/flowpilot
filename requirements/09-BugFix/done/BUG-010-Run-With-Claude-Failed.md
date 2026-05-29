`Test - Single Claude Step` failed immediately after session creation.

Observed log:

```text
session process exited or is no longer registered
```

## Symptom

- The workflow runtime wrote `session_created`, then almost immediately logged `session_dead`.
- The persisted `providerSessionId` remained synthetic:
  - `claude_stream_session_prompt_...`
- No real Claude `session_id` was ever captured for the step.
- The predefined step `test-single-claude-step` could not complete its first turn.

## Root Causes

1. The Claude session transport assumed `claude -p` could stay alive as a long-lived stdin/stdout process.
   - Current Claude CLI behavior is one-shot in `--print` mode.
   - FlowPilot created a session handle, but the subprocess exited after serving a single print invocation.

2. The runner used an invalid Claude CLI combination during the first fix attempt.
   - It paired `--input-format stream-json` with `--output-format json`.
   - Claude requires `stream-json` output for `stream-json` input.

3. The runner passed the stored model name `claude-haiku` directly to Claude CLI in the session path.
   - The non-interactive CLI path here accepted `haiku`, not `claude-haiku`.
   - Claude returned a provider error before the first turn completed.

4. One test cycle initially hit a stale local runner process.
   - The process listening on port `4317` had been started on May 28, 2026, before the new Claude fixes were loaded.
   - That produced a false negative until the runner was restarted.

## Fix

### 1. Replace long-lived Claude subprocess sessions with virtual session state

Updated `apps/local-runner/internal/runner/sessions.go` so `claude_stream_json` sessions:

- keep session metadata in the runner `sessions` map
- do **not** keep a persistent Claude subprocess alive
- spawn a fresh `claude -p` process for each `SendMessage()` call
- reuse the returned real Claude `session_id` through `--resume` on later turns

This preserves the existing FlowPilot session contract while matching actual Claude CLI behavior.

### 2. Use the correct Claude stream-json contract

Updated the Claude command builder to use:

```text
claude -p --input-format stream-json --output-format stream-json --verbose ...
```

The runner now parses the final `result` event from the Claude output stream and extracts:

- `result`
- `session_id`
- `is_error`

instead of expecting a single plain JSON object.

### 3. Normalize Claude model names for the session transport

Added Claude model normalization in the runner session path:

- `claude-haiku` -> `haiku`
- `claude-sonnet` -> `sonnet`
- `claude-opus` -> `opus`

This aligns the session path with the existing one-shot `ExecutePrompt()` normalization logic.

### 4. Keep error semantics correct

Adjusted Claude message execution so:

- real subprocess startup / flag / provider failures surface as `provider error`
- only actual missing-session conditions surface as `session_dead`

That prevents the workflow runtime from incorrectly treating invalid Claude command startup as session-liveness loss.

## Files Changed

- `apps/local-runner/internal/runner/sessions.go`
- `apps/local-runner/internal/runner/sessions_test.go`

## Tests Added / Updated

Updated runner tests to cover:

- virtual Claude session startup without a persistent child process
- stream-json `result` event parsing
- real session-id capture after the first Claude turn
- resumed Claude follow-up turn using the same `session_id`
- model normalization from `claude-haiku` to `haiku`

## Verification

### Automated

Ran:

```text
rtk go test ./...
```

Result:

```text
60 passed in 3 packages
```

### Live Runner Verification

Restarted the local runner on port `4317`, then exercised the exact predefined step configuration:

- step type: `test-single-claude-step`
- model: `claude-haiku`
- reasoning: `low`
- prompt base: `Demo prompt. Just return text "Im ok" for me!`

Observed:

1. `POST /sessions/start`
   - returned a FlowPilot synthetic session id and process key

2. First `POST /sessions/message`
   - returned success
   - output: `I'm ok`
   - returned a real Claude `providerSessionId`

3. Second `POST /sessions/message` on the same FlowPilot session
   - returned success
   - output: `second ok`
   - reused the same real Claude `providerSessionId`

This confirmed both:

- first-turn execution works
- same-session continuation works

## Final Outcome

`Test - Single Claude Step` now runs successfully once the runner is restarted with the fixed code.

The user-facing failure was not caused by workflow state, Supabase session persistence, or the predefined step definition itself. The failing layer was the local runner Claude session transport.

## Residual Notes

- The local runner must be restarted after changes to `apps/local-runner/internal/runner/sessions.go`; otherwise old Claude behavior remains active.
- Existing workflow logs that show `session_dead` for Claude runs before the restart are expected historical failures.
- If Claude CLI changes its accepted aliases or stream-json contract again, the runner session adapter will need another compatibility pass.
