import test from "node:test";
import assert from "node:assert/strict";
import { useStore, surfaceProviderLimitForRun } from "./store";
import type { ProviderAccountSummary, ProviderEventDTO } from "../types/contract";

// Task-445 (CP-87 P-1): the quota/account-switch surface triggers off the
// typed provider_limit_reached event — never by parsing turn_failed text.
// Evidence strings here deliberately do NOT contain any historical
// usage-limit token; classification is the runner's job.

function acct(id: string, active: boolean, remaining: number | null): ProviderAccountSummary {
  return {
    id,
    providerKey: "claude",
    displayName: id,
    displayLabel: id,
    homePath: `/home/${id}`,
    authStorePath: null,
    slotIndex: active ? 0 : 1,
    authStatus: "connected",
    isActive: active,
    createdAt: "2026-01-01T00:00:00Z",
    lastAuthenticatedAt: null,
    accountEmail: `${id}@example.com`,
    accountName: id,
    usageSummary: null,
    remaining5hPercent: remaining,
    remaining7dPercent: remaining,
    remaining5hResetAt: null,
    remaining7dResetAt: null,
    usageSource: remaining === null ? "unavailable" : "provider_api",
    accessTokenExpiresAt: null,
    refreshTokenExpiresAt: null,
    refreshTokenExpiryNote: null,
    usageDetailLines: [],
  };
}

function limitEvt(runId: string, msg: string): ProviderEventDTO {
  return {
    id: "e1",
    workflowRunId: runId,
    providerSessionId: "leg-1",
    providerKey: "claude",
    seq: 7,
    occurredAt: "2026-01-01T00:00:00Z",
    type: "provider_limit_reached",
    providerLimit: {
      kind: "quota_exhausted",
      providerKey: "claude",
      sanitizedMessage: msg,
      detectionSource: "stop_reason",
      confidence: "exact",
    },
  };
}

function seedChatState() {
  useStore.setState({
    runId: "run-1",
    mainRunId: "run-1",
    chatMode: "normal_chat",
    selectedProvider: "claude",
    providerAccounts: [acct("exhausted-acct", true, 0), acct("healthy-acct", false, 80)],
    pendingAccountSwitch: undefined,
    accountSwitchLoading: false,
    _accountSwitchTriedIds: [],
    attentionItems: [],
  });
}

test("provider_limit_reached opens the account-switch surface without any message parsing", () => {
  seedChatState();
  const e = limitEvt("run-1", "opaque provider wording with zero legacy tokens");
  surfaceProviderLimitForRun(e, useStore.getState, useStore.setState);
  const pending = useStore.getState().pendingAccountSwitch;
  assert.ok(pending, "typed limit event must open the account-switch modal");
  assert.equal(pending?.failedAccountId, "exhausted-acct");
  assert.equal(pending?.candidateAccount?.id, "healthy-acct");
  assert.equal(pending?.reason, "usage_limit");
});

test("turn_failed text alone no longer drives the quota surface (typed-only)", () => {
  seedChatState();
  // Legacy message text that the removed string classifier WOULD have matched.
  const e = {
    id: "e2",
    workflowRunId: "run-1",
    providerSessionId: "leg-1",
    providerKey: "claude",
    seq: 8,
    occurredAt: "2026-01-01T00:00:00Z",
    type: "turn_failed",
    error: "usage limit reached",
    recoverable: false,
  } as ProviderEventDTO;
  surfaceProviderLimitForRun(e, useStore.getState, useStore.setState);
  assert.equal(useStore.getState().pendingAccountSwitch, undefined, "untyped turn_failed must not open quota UX");
});

test("provider_limit_reached on a non-focused run lands as an inbox quota decision", () => {
  seedChatState();
  const e = limitEvt("run-other", "opaque wording");
  surfaceProviderLimitForRun(e, useStore.getState, useStore.setState);
  assert.equal(useStore.getState().pendingAccountSwitch, undefined);
  const items = useStore.getState().attentionItems;
  assert.equal(items.length, 1, "non-focused quota decision must land in the inbox");
  assert.equal(items[0].kind, "quota");
  assert.equal(items[0].decision?.quota?.candidateAccountId, "healthy-acct");
});
