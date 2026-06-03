# BUG-023: Workflow Does Not Follow Active Account

## Status

Fixed.

## Reproduction

1. Configure two provider accounts.
2. Default account A hits the provider limit.
3. Set account B as the active account.
4. Trigger a workflow step or workflow.
5. The real AI-provider thread still starts with account A instead of account B.

## Root Cause

Workflow session reuse only matched existing sessions by provider and model. It did not persist or compare the provider account used to create the thread, so a preserved or completed provider thread from account A remained eligible after the active account changed to account B.

The old local runner process does not need to still be live for this bug to happen. After a long idle gap, FlowPilot can still keep the durable `provider_session_id` in `workflow_run_sessions` and start a new local runner process that asks the provider CLI to resume that stale provider thread. The `session_dead` reconnect path also retried without carrying the selected provider account id forward.

## Fix

`apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts` now resolves the requested local provider account before session reuse decisions, stores the resolved account id and home path in `workflow_run_sessions.metadata_json`, and treats account mismatch as a session replacement condition.

When a session is replaced or freshly created, the runtime starts the local runner with the resolved active account. The reconnect path now also passes `providerAccountId` into the retry.

## Verification

Added a regression test in `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts` that verifies an old provider thread from account A is not resumed after the active account resolves to account B.

Additional review confirmed the fix covers the long-idle scenario: even if the previous local runner process is gone, the runtime no longer passes account A's persisted provider thread id into a new local runner process after account B is active.

Test command:

```bash
npm run test -- src/features/workflow-engine/workflow-start-runtime.test.ts
```

Result: 19 tests passed.
