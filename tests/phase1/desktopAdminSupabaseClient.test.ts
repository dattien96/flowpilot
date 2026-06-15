import test from "node:test";
import assert from "node:assert/strict";

import { createDesktopAdminSupabaseClient } from "../../apps/desktop-flowpilot/src/auth/desktopAdminSupabaseClient";

type PersistedAuthSession = {
  clientKey: string;
  accessToken: string;
  refreshToken: string;
  userId: string;
  email?: string | null;
};

test("createDesktopAdminSupabaseClient reuses persisted desktop auth token for settings queries", async () => {
  const originalWindow = (globalThis as { window?: unknown }).window;
  const persistedSession: PersistedAuthSession = {
    clientKey: "https://proj.supabase.co::anon",
    accessToken: "access-token",
    refreshToken: "refresh-token",
    userId: "user-1",
    email: "user@example.com",
  };

  (globalThis as { window?: unknown }).window = {
    flowpilot: {
      loadAuthSession: async () => persistedSession,
    },
  };

  try {
    const client = await createDesktopAdminSupabaseClient(
      "https://proj.supabase.co",
      "anon",
    );

    assert.equal(
      await (client as unknown as { _getAccessToken(): Promise<string | null> })._getAccessToken(),
      "access-token",
    );
  } finally {
    (globalThis as { window?: unknown }).window = originalWindow;
  }
});

test("createDesktopAdminSupabaseClient falls back to anon key when no matching desktop session exists", async () => {
  const originalWindow = (globalThis as { window?: unknown }).window;
  (globalThis as { window?: unknown }).window = {
    flowpilot: {
      loadAuthSession: async () => ({
        clientKey: "https://other.supabase.co::anon",
        accessToken: "wrong-token",
        refreshToken: "wrong-refresh",
        userId: "user-2",
      }),
    },
  };

  try {
    const client = await createDesktopAdminSupabaseClient(
      "https://proj.supabase.co",
      "anon",
    );

    assert.equal(
      await (client as unknown as { _getAccessToken(): Promise<string | null> })._getAccessToken(),
      "anon",
    );
  } finally {
    (globalThis as { window?: unknown }).window = originalWindow;
  }
});
