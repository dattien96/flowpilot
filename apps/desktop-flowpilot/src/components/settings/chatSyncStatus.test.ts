import test from "node:test";
import assert from "node:assert/strict";
import { normalizeChatSyncGoogleDriveStatus } from "./ProjectsSettings";

// BUG-383: the runner emits availableAccounts with omitempty — when no Google
// Drive account exists the key is absent, and `payload.availableAccounts.length`
// threw "Cannot read properties of undefined (reading 'length')" in Settings.
test("normalizeChatSyncGoogleDriveStatus fills missing availableAccounts", () => {
  const payload = {
    connection: {},
    effectiveSource: "none",
    ready: false,
    // availableAccounts absent — wire shape when no accounts exist
  } as never;
  const normalized = normalizeChatSyncGoogleDriveStatus(payload);
  assert.deepEqual(normalized.availableAccounts, []);
});

test("normalizeChatSyncGoogleDriveStatus preserves present accounts", () => {
  const accounts = [{ accountId: "a1", accountReady: true, mcpWriteReady: false }];
  const normalized = normalizeChatSyncGoogleDriveStatus({
    connection: {},
    effectiveSource: "chat_sync",
    ready: true,
    availableAccounts: accounts,
  } as never);
  assert.deepEqual(normalized.availableAccounts, accounts);
});
