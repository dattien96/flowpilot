import test from "node:test";
import assert from "node:assert/strict";

import { resolveDesktopBootstrapState } from "../../apps/desktop-flowpilot/src/app/bootstrapState";
import type { DesktopBootstrapState } from "../../packages/flowpilot-client-core/src";

function bootstrap(overrides: Partial<DesktopBootstrapState>): DesktopBootstrapState {
  return {
    runtimeStatus: {
      mode: "config",
      configured: true,
      apiUrl: "https://proj.supabase.co",
      anonKey: "anon",
      edgeFunctionUrl: "https://proj.supabase.co/functions/v1",
      hasServiceRoleKey: true,
      projectRef: "proj",
      runnerReachable: true,
      envAvailable: false,
      savedConfigAvailable: true,
      edgeFunctionsReady: true,
      lastError: null,
      ...(overrides.runtimeStatus ?? {}),
    },
    session: null,
    ...overrides,
  };
}

test("bootstrap resolves authenticated chat when session exists and runtime is healthy", () => {
  const result = resolveDesktopBootstrapState(
    bootstrap({
      session: { userId: "user-1", email: "a@example.com" },
    }),
  );

  assert.deepEqual(result, {
    issue: null,
    view: "authenticated-chat",
    preferredSettingsSection: "supabase",
  });
});

test("bootstrap resolves supabase invalid state explicitly", () => {
  const result = resolveDesktopBootstrapState(
    bootstrap({
      runtimeStatus: {
        mode: "config",
        configured: false,
        apiUrl: null,
        anonKey: null,
        edgeFunctionUrl: null,
        hasServiceRoleKey: false,
        projectRef: null,
        runnerReachable: true,
        envAvailable: false,
        savedConfigAvailable: false,
        edgeFunctionsReady: false,
        lastError: "Saved Supabase config is incomplete.",
      },
    }),
  );

  assert.deepEqual(result, {
    issue: "supabase-invalid",
    view: "settings",
    preferredSettingsSection: "supabase",
  });
});

test("bootstrap resolves runner offline to runner settings", () => {
  const result = resolveDesktopBootstrapState(
    bootstrap({
      runtimeStatus: {
        mode: "config",
        configured: true,
        apiUrl: "https://proj.supabase.co",
        anonKey: "anon",
        edgeFunctionUrl: "https://proj.supabase.co/functions/v1",
        hasServiceRoleKey: true,
        projectRef: "proj",
        runnerReachable: false,
        envAvailable: false,
        savedConfigAvailable: true,
        edgeFunctionsReady: true,
        lastError: "Unable to reach local runner.",
      },
      session: { userId: "user-1", email: "a@example.com" },
    }),
  );

  assert.deepEqual(result, {
    issue: "runner-offline",
    view: "settings",
    preferredSettingsSection: "runner",
  });
});
