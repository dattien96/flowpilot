// M-4 regression (CP-83 manual): one transient heartbeat transport failure
// must not kill the desktop app. The runner grants a ~15 s lease TTL (3 missed
// beats at 5 s cadence); the client now retries inside a beat before invoking
// runnerLost. Sustained failure still notifies + quits (CP-81 contract kept).
import test from "node:test";
import assert from "node:assert/strict";

import { createDesktopLifecycle, type LifecycleSnapshot } from "./desktopLifecycle";

function makeHarness(heartbeatFailures: number) {
  const snapshot: LifecycleSnapshot = {
    runnerInstanceId: "inst-1",
    generation: 1,
    protocolVersion: 1,
    lifecycleMode: "client-managed",
    phase: "ready",
    clients: [{ leaseId: "l-desk", kind: "desktop", label: "Desktop" }],
    workload: { items: [] },
    updatePending: false,
    serverNow: new Date().toISOString(),
  };
  const calls = { heartbeat: 0, register: 0 };
  let remainingFailures = heartbeatFailures;
  const fetchFn = (async (input: unknown, init?: { method?: string }) => {
    const url = typeof input === "string" ? input : (input as { url: string }).url;
    const path = new URL(url).pathname;
    const json = (status: number, body: unknown) =>
      new Response(JSON.stringify(body ?? {}), { status, headers: { "Content-Type": "application/json" } });
    if (path === "/system/clients/register") {
      calls.register++;
      return json(200, {
        leaseId: "l-desk", leaseToken: "tok-1", runnerInstanceId: "inst-1",
        generation: 1, heartbeatIntervalMs: 60000, ttlMs: 15000, snapshot,
      });
    }
    if (path.endsWith("/heartbeat")) {
      calls.heartbeat++;
      if (remainingFailures > 0) {
        remainingFailures--;
        throw new Error("connection reset by peer");
      }
      return json(200, snapshot);
    }
    return json(404, { error: { code: "not_found", message: path } });
  }) as unknown as typeof fetch;

  const harness = {
    quits: 0,
    notices: [] as Array<{ title: string; body: string }>,
    calls,
    lifecycle: undefined as unknown as ReturnType<typeof createDesktopLifecycle>,
  };
  harness.lifecycle = createDesktopLifecycle({
    runnerURL: "http://runner.test",
    fetchFn,
    clientInstanceId: "desktop-test",
    pid: 1,
    label: "Desktop",
    heartbeatMs: 60_000,
    heartbeatRetryMs: 1,
    reconnectPollMs: 1,
    setTimeoutFn: ((fn: () => void, ms?: number) => {
      const h = setTimeout(fn, ms);
      (h as unknown as { unref?: () => void }).unref?.();
      return h;
    }) as typeof setTimeout,
    notify: (title, body) => {
      harness.notices.push({ title, body });
    },
    quit: () => harness.quits++,
  });
  return harness;
}

test("transient heartbeat failure is absorbed — no quit, lease stays", async () => {
  const h = makeHarness(1); // first attempt throws, retry succeeds
  await h.lifecycle.start();
  assert.equal(h.lifecycle.state().leaseId, "l-desk");
  await h.lifecycle.heartbeatOnce();
  assert.equal(h.calls.heartbeat, 2, "beat must retry once internally");
  assert.equal(h.quits, 0, "one dropped heartbeat must not kill the app");
  assert.equal(h.notices.length, 0);
  await h.lifecycle.heartbeatOnce();
  assert.equal(h.quits, 0);
});

test("sustained heartbeat failure still quits with notice", async () => {
  const h = makeHarness(99); // every attempt fails
  await h.lifecycle.start();
  await h.lifecycle.heartbeatOnce();
  assert.equal(h.calls.heartbeat, 3, "all attempts exhausted before giving up");
  assert.equal(h.quits, 1);
  assert.match(h.notices[0]?.body ?? "", /stopped unexpectedly/i);
});

test("heartbeat failure then recovery keeps the same lease", async () => {
  const h = makeHarness(2); // fails twice, third attempt succeeds — inside one beat
  await h.lifecycle.start();
  await h.lifecycle.heartbeatOnce();
  assert.equal(h.calls.heartbeat, 3);
  assert.equal(h.quits, 0);
  assert.equal(h.lifecycle.state().leaseId, "l-desk", "lease must not rotate on transport blip");
});
