# Task-018: Auto Switch Account On Provider Usage Limit

## Metadata

- Document ID: `Task-018`
- Title: `Auto Switch Account On Provider Usage Limit`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- Child Documents: [BUG-090: Cross-Provider Chat Parity Gaps](../../09-BugFix/done/BUG-090-Cross-Provider-Chat-Parity-Gaps.md)
- Related Documents: [Task-036: Desktop Provider Accounts Sidebar](../done/Task-036-Desktop-Provider-Accounts-Sidebar.md), [Task-071: Cross-Account Chat Resume Definition of Done Checklist](./Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md), [BUG-056: Claude Usage Limit Retried As Recoverable](../../09-BugFix/done/BUG-056-Claude-Usage-Limit-Retried-As-Recoverable.md), [BUG-085: Codex Live Chat Splits Provider Session On Account Switch](../../09-BugFix/done/BUG-085-Codex-Live-Chat-Splits-Provider-Session-On-Account-Switch.md), [BUG-090: Cross-Provider Chat Parity Gaps](../../09-BugFix/done/BUG-090-Cross-Provider-Chat-Parity-Gaps.md)
- Replaces: `—`
- Tags: `desktop, local-runner, provider-accounts, codex, claude, quota, retry, chat`

## AI Quick View

### Summary

- Add a desktop-led recovery flow that reacts to provider usage-limit failures by finding another valid account for the same provider, asking the user for confirmation, switching accounts, and retrying the pending chat turn.
- Codex gets full auto-pick support in this task because the current account summary API already exposes numeric `remaining_5h_percent` and `remaining_7d_percent`.
- Claude uses a deterministic connected-account fallback when numeric quota windows are unavailable; it must not pretend to rank Claude accounts with Codex-style precision.
- Retry after switch must reuse the original pending prompt payload, not a synthetic `"Try again"` message.

### Current Ask

- Define one implementation slice for automatic same-provider account switching after a usage-limit failure in desktop chat.

### Key Decisions

- `T-1` Trigger auto-switch only from terminal provider usage-limit failures, not from generic auth, transport, or resume errors.
- `T-2` Run the orchestration in the desktop app by reusing the existing provider-account list and activate-account APIs; do not move this decision into the runner for MVP.
- `T-3` Candidate ranking for Codex is `remaining5h` first, then `remaining7d`, because the 5-hour window is the nearer blocking constraint.
- `T-4` Post-switch retry must resend the original prompt, model, reasoning, attachments, and skills payload already captured for the failed turn.
- `T-5` Claude may automatically suggest the first connected, untried account by slot order when both quota windows are unavailable; numeric best-account ranking remains out of scope until trustworthy telemetry exists.

### Constraints

- Do not auto-switch across providers.
- Do not silently switch accounts without explicit user confirmation.
- Do not auto-retry workflow-step runs in this slice; scope is desktop chat only.
- Do not regress the existing Codex cross-account resume flow described in `SD-14`.
- Do not guess Claude quota percentages from free-form text.

### Open Questions

- `Q-1` Should Claude later expose a runner-normalized candidate score once better quota telemetry exists?
- `Q-2` Should the desktop persist a short-lived "already failed on this account for this turn" cache to prevent user-confirmed switch loops?

### Source Refs

- `apps/local-runner/internal/cli/provider_account_api.go`
- `apps/local-runner/internal/cli/provider_account_terminal.go`
- `apps/local-runner/internal/runner/interactive_service.go`
- `apps/local-runner/internal/runner/claude_usage.go`
- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/types/contract.ts`
- [Task-036](../done/Task-036-Desktop-Provider-Accounts-Sidebar.md)
- [BUG-056](../../09-BugFix/done/BUG-056-Claude-Usage-Limit-Retried-As-Recoverable.md)
- [BUG-085](../../09-BugFix/done/BUG-085-Codex-Live-Chat-Splits-Provider-Session-On-Account-Switch.md)
- [SD-14](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)

## 1. Goal

When a desktop chat turn fails because the current provider account has reached usage quota, FlowPilot should help the user continue with another connected account of the same provider instead of making the user switch accounts manually and resend the prompt by hand.

The intended outcome for this task is:

1. detect a quota/usage-limit failure from the current provider turn,
2. inspect same-provider connected accounts,
3. choose the best valid replacement account when the provider telemetry is trustworthy,
4. ask the user to confirm the switch,
5. activate the chosen account,
6. show a loading state while the switch/resume preparation completes,
7. show an in-chat notice that the pending job can continue,
8. resend the original failed turn payload automatically after the user confirms the retry action.

## 2. Parent Links

- coding plan:
- tech design: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- system spec: [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md)
- specific upstream ids: `Task-036`, `BUG-056`, `BUG-085`

## 3. Trigger

The desktop already knows how to:

- list enriched provider accounts with usage metadata,
- activate a selected provider account,
- classify provider usage-limit failures as terminal,
- continue Codex chats across same-provider account switches.

What is still missing is the operator workflow on top of those pieces. Today, when quota is exhausted, the user must read the failure, manually inspect accounts, manually switch to another account, reopen or continue the chat, and then resend the prompt. That is slow, error-prone, and breaks the continuity FlowPilot already established for Codex cross-account resume.

## 4. Exact Change

- `T-1` Add a desktop-side quota-recovery controller for normal chat runs.
  - The controller listens for non-recoverable `turn_failed` events whose message matches provider usage-limit semantics already normalized by the runner.
  - It must not trigger for auth failures, `provider_account_changed`, session-file conflicts, or transport errors.

- `T-2` Keep the decision loop in the desktop store for MVP.
  - Reuse `listProviderAccounts()` to fetch candidate accounts.
  - Reuse `/provider-accounts/activate` to switch.
  - Reuse existing `sendPrompt` / `sendTurn` flow after the switch.
  - Do not add a runner-owned "auto switch now" endpoint in this slice unless implementation friction proves it necessary.

- `T-3` Limit automatic best-candidate ranking to Codex in this task.
  - Valid Codex candidate rules:
    - same `providerKey` as the failed run,
    - `authStatus == "connected"`,
    - `isActive == false`,
    - different account id from the failed account,
    - `remaining5hPercent != nil` and `remaining7dPercent != nil`,
    - `remaining5hPercent > 0`,
    - `remaining7dPercent > 0`.
  - Reject candidates with missing numeric quota telemetry for the auto-pick path.

- `T-4` Rank Codex candidates using near-window-first logic.
  - Primary sort: higher `remaining5hPercent`.
  - Secondary sort: higher `remaining7dPercent`.
  - Tertiary sort: lower `slotIndex` for deterministic stability.
  - This preserves the user requirement that an account with better 5h headroom can outrank one with better 7d headroom, as long as both windows remain above zero.

- `T-5` Add guard conditions beyond raw quota values.
  - Skip the current active account.
  - Skip any account already attempted for the same failed turn during the current recovery flow.
  - Skip accounts whose auth status is no longer connected after refresh.
  - Skip accounts from another provider even if they have better quota.

- `T-6` Add a Claude-specific fallback rule.
  - Claude usage-limit detection uses the same confirmation and original-turn retry flow as Codex.
  - When both numeric quota windows are unavailable, choose the first connected, inactive, untried Claude account by `slotIndex`.
  - Do not label that fallback as the "best quota" account because Claude telemetry does not support that claim.

- `T-7` Add a confirmation popup before any switch.
  - Popup content should include:
    - provider name,
    - current exhausted account label,
    - proposed replacement account label,
    - short reason such as `Current account hit usage limit`.
  - Actions:
    - `Switch and retry`
    - `Cancel`
  - No automatic silent switch is allowed.

- `T-8` Add an explicit loading state during switch and preparation.
  - While the desktop calls activate-account and waits for the account change to settle, the chat UI should show a temporary busy state.
  - For Codex, this loading state covers both account activation and any resume/copy work that happens lazily on the next turn per `SD-14`.

- `T-9` Add an in-chat recovery notice after a confirmed switch.
  - Show a system/UI component in the chat area stating that the account was switched and the pending job can continue.
  - The component should make the retry action explicit so the user understands why a new turn is about to be sent.
  - Suggested message shape:
    - `Switched to Codex account <label>. Continue the pending job?`

- `T-10` Retry with the original failed turn payload.
  - Reuse the saved prompt text from `lastTurnInput`.
  - Reuse selected skills, model, reasoning effort, YOLO mode, and attachments.
  - Do not replace the payload with a synthetic `"Try again"` message because that changes conversation meaning and weakens transcript accuracy.

- `T-11` Scope the first implementation to chat runs only.
  - `normal_chat` is in scope.
  - workflow-run step execution is out of scope.
  - This avoids mixing interactive chat recovery with workflow retry semantics.

- `T-12` Preserve existing resume behavior after switch.
  - Codex retry must continue to rely on the existing same-provider cross-account preparation from `SD-14` and `BUG-085`.
  - This task must not introduce a second custom session-copy path in the desktop layer.

- `T-13` Add focused tests for ranking and recovery orchestration.
  - Desktop/store tests:
    - usage-limit failure on Codex with one better candidate opens confirm flow,
    - higher `remaining5hPercent` wins even when the competing account has higher `remaining7dPercent`,
    - zero `remaining5hPercent` or zero `remaining7dPercent` disqualifies a candidate,
    - cancel leaves the run failed and does not switch,
    - confirm activates the new account and resends the original turn payload,
    - same failed turn does not loop back onto an already-attempted exhausted account.
  - Runner regression expectation:
    - existing `BUG-085` Codex cross-account resume tests continue to pass because the retry after switch depends on that path.

## 5. Touched Areas

- files:
  - `apps/desktop-flowpilot/src/state/store.ts`
  - `apps/desktop-flowpilot/src/state/store.test.ts`
  - `apps/desktop-flowpilot/src/types/contract.ts`
  - `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`
  - `apps/desktop-flowpilot/src/client/MockRunnerClient.ts`
  - `apps/desktop-flowpilot/src/components/Navigator.tsx`
  - `apps/local-runner/internal/cli/provider_account_api.go`
  - `apps/local-runner/internal/runner/interactive_service.go`
  - `apps/local-runner/internal/runner/cross_account_resume_test.go`
- modules:
  - `desktop-flowpilot`
  - `local-runner`
  - provider account selection and chat recovery
- routes:
  - `GET /client/provider-accounts`
  - `POST /provider-accounts/activate`
  - existing chat run stream / turn routes
- tables:
  - none expected

## 6. Acceptance Check

- A Codex chat that fails with a usage-limit message can offer a switch to another connected Codex account when a valid candidate exists.
- Candidate selection follows `remaining5h` first, then `remaining7d`, with all required windows above zero.
- Confirming the popup activates the chosen account and the desktop retries the original failed turn payload, not a placeholder prompt.
- Canceling the popup leaves the failed turn visible and does not switch accounts.
- The chat UI shows a clear loading and post-switch recovery notice.
- Existing Codex same-provider resume behavior still works after the automatic switch flow.
- Claude usage-limit failures can offer a deterministic connected-account fallback without entering a fake Codex-style quota-ranking path.
- Focused desktop/store tests pass for ranking, cancel, confirm, and payload-preservation behavior.

## 7. Out of Scope

- Full workflow-step auto-switch and auto-retry.
- Cross-provider failover, for example Claude -> Codex.
- Background silent switching without user confirmation.
- Inventing synthetic Claude 5h/7d percentages from free-form metadata.
- New persistent database schema for recovery state.
- Solving multi-PC continuation; that remains separate from this task.

## 8. Completion Notes

- result: implemented for desktop normal-chat runs.
- follow-ups:
  - Claude numeric best-account ranking remains a follow-up once runner telemetry exposes reliable quota windows.
  - If the desktop implementation becomes too stateful, consider a follow-up to move candidate evaluation into a dedicated runner endpoint.
- upstream docs updated:
  - `Task-018`
