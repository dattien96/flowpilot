# Use Case 01 - Web Prompt To Local Codex Execution

Date: 2026-05-15

## 1. Goal

Let a user type a prompt in the admin web, choose a provider, and run that prompt through the local runner so the app can execute `codex` on the same machine and show the result back in the UI.

This use case is intentionally narrower than full workflow orchestration:

- one prompt
- one provider
- one local runner
- one structured response
- one result view

The goal is to prove the execution boundary before wiring the full workflow graph.

## 2. Scope

### Included

- Prompt input in the web
- Provider selection
- Local runner health check
- Local provider detection
- One HTTP request from web to local runner
- One local `codex` CLI execution
- Captured stdout and stderr summary
- Structured output rendered back in the web
- Local artifact path exposure

### Excluded

- Full workflow definitions
- Approval gates
- Multi-step orchestration
- Claude and Gemini execution, except as future provider adapters
- Remote backend execution
- Automatic installation of provider CLIs
- Syncing raw local artifacts into Supabase

## 3. User Story

As a user, I want to type a prompt in the admin web and have the local runner execute it with my selected AI CLI, so I can verify the end-to-end AI control plane before expanding to workflow automation.

## 4. Final Target Flow

1. User opens the admin web.
2. Web checks local runner health.
3. Web checks provider detection.
4. User enters a prompt.
5. User selects `codex`.
6. Web submits the request to the local runner.
7. Runner assembles the prompt from the selected skill or flow context.
8. Runner invokes `codex` locally.
9. Runner captures the CLI output.
10. Runner returns a normalized response.
11. Web stores execution metadata in the app state.
12. User sees the result and local artifact links.

## 5. Domain Design

### New use cases

- `RunPromptUseCase`
- `CheckLocalRunnerHealthUseCase`
- `ListLocalProvidersUseCase`
- `ListLocalSkillsUseCase`
- `ListLocalFlowsUseCase`

### New gateway contract

- `LocalRunnerGateway`
- `PromptExecutionGateway` if the prompt execution API is separated from runner health and discovery

### Prompt execution request model

Suggested fields:

- `providerKey`
- `prompt`
- `workspacePath`
- `skillIds`
- `flowId`
- `contextSourceIds`
- `workingDirectory`
- `timeoutMs`
- `outputFormat`

### Prompt execution response model

Suggested fields:

- `status`
- `providerKey`
- `command`
- `stdoutSummary`
- `stderrSummary`
- `outputMarkdown`
- `artifactPaths`
- `startedAt`
- `completedAt`
- `exitCode`
- `errorMessage`

## 6. Data Flow

### Web side

- Presentation layer owns the input form and result viewer.
- Domain use case validates the request.
- Data layer calls the local runner over localhost HTTP.
- The web never launches shell commands directly.

### Runner side

- Local runner receives one execute request.
- Runner resolves the selected provider.
- Runner loads local markdown brain data if needed.
- Runner writes a prompt file and execution logs.
- Runner invokes the provider CLI.
- Runner normalizes the result into a stable JSON response.

## 7. Implementation Phases

### Phase 1 - Add a prompt execution contract

Goal:
Define the exact input and output shape before wiring UI or CLI calls.

Tasks:

- Add prompt execution request and response types in domain.
- Add a runner execution gateway interface.
- Decide whether prompt execution lives on the same gateway as health/discovery or on a dedicated gateway.
- Add validation rules for empty prompt, missing provider, and timeout bounds.

Done when:

- The app can describe prompt execution without referring to shell commands in UI code.

### Phase 2 - Add runner execute endpoint

Goal:
Expose a single runner endpoint that can execute one prompt with one provider.

Tasks:

- Add `POST /runs/{runId}/steps` or a simpler `POST /execute` endpoint for the first use case.
- Accept provider key, prompt, and workspace context.
- Normalize local execution output into JSON.
- Return local artifact paths and command metadata.
- Add timeout and cancellation handling.

Done when:

- The runner can accept one request and return one structured result.

### Phase 3 - Implement Codex adapter only

Goal:
Make the first provider path real before adding more providers.

Tasks:

- Implement `CodexCliProviderAdapter`.
- Resolve `codex` on PATH.
- Run the provider with a configurable command template.
- Capture stdout, stderr, exit code, and duration.
- Persist execution artifacts locally.

Done when:

- A prompt can be executed with Codex on the local machine.

### Phase 4 - Wire web form to runner

Goal:
Allow a user to test the end-to-end path from the admin web.

Tasks:

- Add a prompt input panel in Settings or a dedicated test page.
- Add provider selector bound to runner detection.
- Add a submit action that calls the local runner gateway.
- Add pending/loading/error states.
- Render the result markdown and artifact metadata.

Done when:

- A user can type a prompt in the web and see the Codex result come back.

### Phase 5 - Add persistence for execution metadata

Goal:
Keep execution history visible after refresh.

Tasks:

- Store prompt run records in Supabase.
- Store provider key, status, and timestamps.
- Store artifact references, not raw artifact content.
- Store output summary and approval placeholders if needed later.

Done when:

- The result is still visible after reload.

## 8. File Plan

### Admin web

- `apps/admin-web/src/domain/model/entity/local-runner.ts`
- `apps/admin-web/src/domain/gateway/local-runner-gateway.ts`
- `apps/admin-web/src/domain/usecase/local-runner/*`
- `apps/admin-web/src/data/repository/local-runner/http-local-runner-gateway.ts`
- `apps/admin-web/src/app/(protected)/settings/page.tsx`
- `apps/admin-web/src/app/(protected)/runner-test/page.tsx` if a dedicated page is needed

### Local runner

- `apps/local-runner/cmd/flowpilot/main.go`
- `apps/local-runner/internal/cli/root.go`
- `apps/local-runner/internal/runner/*`
- `apps/local-runner/internal/provider/codex/*`
- `apps/local-runner/internal/execution/*`

## 9. Testing Plan

### Unit tests

- prompt request validation rejects empty prompt
- provider selection rejects unsupported provider keys
- runner gateway returns offline health when the runner is unreachable
- Codex adapter builds the expected command shape
- runner normalizes stdout/stderr and exit code correctly

### Integration tests

- web can read local runner health
- web can detect providers
- web can submit a prompt to the local runner
- runner returns a successful result for a sample Codex prompt

### Manual test

1. Start the runner.
2. Start the admin web.
3. Open the runner test page.
4. Paste a short prompt.
5. Select `codex`.
6. Submit.
7. Verify the local terminal shows the provider command running.
8. Verify the web shows the returned response.

## 10. Acceptance Criteria

The first use case is complete when all of the following are true:

- user can open the web and see the runner as online
- user can see installed providers
- user can type a prompt
- user can choose Codex
- user can submit the prompt
- local runner invokes the `codex` CLI on the current machine
- web receives a structured response
- output and artifact metadata are visible after the call completes
- no provider API key is required in the web app

## 11. Risks

- Browser-side execution must stay forbidden.
- Provider CLIs may differ in flags and output format.
- Prompt output may be unstructured unless the runner normalizes it.
- Long-running prompts need timeout and cancellation.
- Local artifact paths are machine-specific and should not be treated as portable data.

## 12. Recommended Build Order

1. Add the request/response contracts.
2. Add the runner execute endpoint.
3. Implement the Codex adapter.
4. Add a small web test page or panel.
5. Persist prompt run metadata.
6. Add cancellation and timeout handling.
7. Only then add Claude and Gemini adapters.

