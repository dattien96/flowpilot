import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { MockRunnerClient } from "../client/MockRunnerClient";
import { RunnerApiError } from "../client/HttpWsRunnerClient";
import { resetAdminUseCases } from "../clientCore";

// Polyfill localStorage for node:test
function ensureLocalStorage() {
  if (typeof (globalThis as unknown as { localStorage?: unknown }).localStorage === "undefined") {
    const store = new Map<string, string>();
    (globalThis as unknown as { localStorage: Storage }).localStorage = {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => store.set(k, v),
      removeItem: (k: string) => store.delete(k),
      clear: () => store.clear(),
      key: (i: number) => Array.from(store.keys())[i] ?? null,
      get length() { return store.size; },
    } as unknown as Storage;
  }
}

// The catalog step goes through getAdminUseCases() → runner /supabase-config →
// Supabase REST + runner /providers — all over fetch. A blanket stub keeps the
// whole admin layer returning empty lists so loadProjects can reach "ready".
function stubAdminFetch() {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (input: RequestInfo | URL) => {
    const url =
      typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    if (url.includes("/supabase-config")) {
      return new Response(
        JSON.stringify({ apiUrl: "http://127.0.0.1:9", anonKey: "anon", serviceRoleKey: "svc" }),
        { status: 200, headers: { "content-type": "application/json" } },
      );
    }
    return new Response("[]", { status: 200, headers: { "content-type": "application/json" } });
  };
  return () => {
    globalThis.fetch = originalFetch;
  };
}

function installClient(client: MockRunnerClient) {
  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatBoot: { status: "idle", step: "projects", error: null },
    runId: undefined,
    chatId: undefined,
    selectedProjectId: undefined,
    projects: [],
    providerAccounts: [],
    localProviders: [],
    supportedModels: [],
  });
}

test("loadProjects marks chatBoot ready once the workspace pipeline lands", async () => {
  ensureLocalStorage();
  resetAdminUseCases();
  const restoreFetch = stubAdminFetch();
  try {
    installClient(new MockRunnerClient());
    await useStore.getState().loadProjects();
    const boot = useStore.getState().chatBoot;
    assert.equal(boot.status, "ready");
    assert.equal(boot.error, null);
    // The data the composer gates on actually landed.
    assert.ok(useStore.getState().providerAccounts.length > 0);
    assert.ok(useStore.getState().projects.length > 0);
  } finally {
    restoreFetch();
  }
});

test("a boot-critical failure flips chatBoot to failed with the first error", async () => {
  ensureLocalStorage();
  resetAdminUseCases();
  const client = new MockRunnerClient();
  // RunnerApiError bypasses the connection-retry loop and fails fast.
  client.listProjects = async () => {
    throw new RunnerApiError(500, "server_error", "boom");
  };
  installClient(client);

  await useStore.getState().loadProjects();
  const boot = useStore.getState().chatBoot;
  assert.equal(boot.status, "failed");
  assert.match(boot.error ?? "", /projects/i);
});

test("retry after a failed boot re-locks then reaches ready", async () => {
  ensureLocalStorage();
  resetAdminUseCases();
  const client = new MockRunnerClient();
  client.listProjects = async () => {
    throw new RunnerApiError(500, "server_error", "boom");
  };
  installClient(client);
  await useStore.getState().loadProjects();
  assert.equal(useStore.getState().chatBoot.status, "failed");

  const restoreFetch = stubAdminFetch();
  try {
    resetAdminUseCases();
    const recovered = new MockRunnerClient();
    useStore.setState({
      client: recovered as unknown as ReturnType<typeof useStore.getState>["client"],
    });
    const pending = useStore.getState().loadProjects();
    // Retry immediately re-locks the gate.
    assert.equal(useStore.getState().chatBoot.status, "loading");
    await pending;
    assert.equal(useStore.getState().chatBoot.status, "ready");
  } finally {
    restoreFetch();
  }
});

test("non-critical section failures (skills) still finish ready", async () => {
  ensureLocalStorage();
  resetAdminUseCases();
  const restoreFetch = stubAdminFetch();
  try {
    const client = new MockRunnerClient();
    client.listSkills = async () => {
      throw new RunnerApiError(500, "server_error", "boom");
    };
    installClient(client);
    await useStore.getState().loadProjects();
    assert.equal(useStore.getState().chatBoot.status, "ready");
  } finally {
    restoreFetch();
  }
});

test("a second loadProjects after ready refreshes silently (no re-lock)", async () => {
  ensureLocalStorage();
  resetAdminUseCases();
  const restoreFetch = stubAdminFetch();
  try {
    installClient(new MockRunnerClient());
    await useStore.getState().loadProjects();
    assert.equal(useStore.getState().chatBoot.status, "ready");

    const pending = useStore.getState().loadProjects();
    // The gate stays open during the background refresh.
    assert.equal(useStore.getState().chatBoot.status, "ready");
    await pending;
    assert.equal(useStore.getState().chatBoot.status, "ready");
  } finally {
    restoreFetch();
  }
});
