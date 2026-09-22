// CP-81 Task-418: Desktop lifecycle owner tests. Pure-TS module with injected
// ports — no Electron required (the electron/ adapter is thin wiring).
import test from "node:test";
import assert from "node:assert/strict";

import { createDesktopLifecycle, type LifecycleCloseChoice, type LifecycleSnapshot } from "./desktopLifecycle";
import { MockRunnerClient } from "../client/MockRunnerClient";
import type { RunnerClient } from "../types/contract";

type StubReq = { path: string; method: string; body?: Record<string, unknown> };
type StubRes = { status: number; body?: unknown };

interface RunnerStub {
  fetchFn: typeof fetch;
  calls: StubReq[];
  counts: Record<"register" | "heartbeat" | "release" | "shutdown" | "restart" | "health" | "lifecycle", number>;
  shutdownBodies: Array<Record<string, unknown>>;
  restartBodies: Array<Record<string, unknown>>;
  setSnapshot(snap: Partial<LifecycleSnapshot>): void;
  setHealthInstance(id: string): void;
  failHeartbeat(err?: "transport" | "lease_unknown"): void;
  shutdownFirst(status: number, confirmToken?: string): void;
  restartFirst(status: number, confirmToken?: string): void;
}

function stubRunner(): RunnerStub {
  const calls: StubReq[] = [];
  const counts = { register: 0, heartbeat: 0, release: 0, shutdown: 0, restart: 0, health: 0, lifecycle: 0 };
  const shutdownBodies: Array<Record<string, unknown>> = [];
  const restartBodies: Array<Record<string, unknown>> = [];
  let snapshot: LifecycleSnapshot = {
    runnerInstanceId: "inst-1",
    generation: 1,
    protocolVersion: 1,
    lifecycleMode: "client-managed",
    phase: "ready",
    clients: [{ leaseId: "l-desk", kind: "desktop", label: "Desktop 1" }],
    workload: { items: [] },
    updatePending: false,
    serverNow: new Date().toISOString(),
  };
  let healthInstance = "inst-1";
  let heartbeatMode: "ok" | "transport" | "lease_unknown" = "ok";
  let shutdownRule: { status: number; confirmToken?: string } | null = null;
  let restartRule: { status: number; confirmToken?: string } | null = null;

  const json = (status: number, body: unknown): Response =>
    new Response(JSON.stringify(body ?? {}), {
      status,
      headers: { "Content-Type": "application/json" },
    });

  const fetchFn = (async (input: unknown, init?: { method?: string; body?: string }) => {
    const url = typeof input === "string" ? input : (input as { url: string }).url;
    const path = new URL(url).pathname;
    const body = init?.body ? (JSON.parse(init.body) as Record<string, unknown>) : undefined;
    const req: StubReq = { path, method: init?.method ?? "GET", body };
    calls.push(req);

    if (path === "/health") {
      counts.health++;
      return json(200, { status: "ok", runnerInstanceId: healthInstance });
    }
    if (path === "/system/lifecycle") {
      counts.lifecycle++;
      return json(200, snapshot);
    }
    if (path === "/system/clients/register") {
      counts.register++;
      return json(200, {
        leaseId: "l-desk",
        leaseToken: "tok-1",
        runnerInstanceId: healthInstance,
        generation: 1,
        heartbeatIntervalMs: 60000,
        ttlMs: 15000,
        snapshot,
      });
    }
    if (path.endsWith("/heartbeat")) {
      counts.heartbeat++;
      if (heartbeatMode === "transport") throw new Error("connection refused");
      if (heartbeatMode === "lease_unknown") {
        return json(409, { error: { code: "lease_unknown", message: "unknown lease" } });
      }
      return json(200, snapshot);
    }
    if (path.endsWith("/release")) {
      counts.release++;
      return json(200, { ...snapshot, phase: "idle_grace" });
    }
    if (path === "/system/shutdown") {
      counts.shutdown++;
      shutdownBodies.push(body ?? {});
      if (shutdownRule && counts.shutdown === 1) {
        return json(shutdownRule.status, {
          error: {
            code: "lifecycle_confirmation_required",
            message: "runner is shared or busy",
            confirmToken: shutdownRule.confirmToken,
          },
          snapshot,
        });
      }
      return json(202, { status: "accepted" });
    }
    if (path === "/system/restart") {
      counts.restart++;
      restartBodies.push(body ?? {});
      if (restartRule && counts.restart === 1) {
        return json(restartRule.status, {
          error: {
            code: "lifecycle_confirmation_required",
            message: "runner is shared or busy",
            confirmToken: restartRule.confirmToken,
          },
          snapshot,
        });
      }
      return json(202, { status: "accepted", restartId: "rst-1" });
    }
    return json(404, { error: { code: "not_found", message: path } });
  }) as unknown as typeof fetch;

  return {
    fetchFn,
    calls,
    counts,
    shutdownBodies,
    restartBodies,
    setSnapshot(next) {
      snapshot = { ...snapshot, ...next };
    },
    setHealthInstance(id) {
      healthInstance = id;
    },
    failHeartbeat(err = "transport") {
      heartbeatMode = err;
    },
    shutdownFirst(status, confirmToken) {
      shutdownRule = { status, confirmToken };
    },
    restartFirst(status, confirmToken) {
      restartRule = { status, confirmToken };
    },
  };
}

interface Ports {
  lifecycle: ReturnType<typeof createDesktopLifecycle>;
  stub: RunnerStub;
  quits: number;
  notices: Array<{ title: string; body: string }>;
  statuses: Array<{ phase?: string; reconnecting: boolean; sharedClients: number; updatePending: boolean; idleDeadlineMs?: number }>;
  setCloseChoice(c: LifecycleCloseChoice): void;
}

function makePorts(stub: RunnerStub): Ports {
  const ports: Ports = {
    lifecycle: undefined as unknown as Ports["lifecycle"],
    stub,
    quits: 0,
    notices: [],
    statuses: [],
    setCloseChoice(c) {
      choice = c;
    },
  };
  let choice: LifecycleCloseChoice = "cancel";
  ports.lifecycle = createDesktopLifecycle({
    runnerURL: "http://runner.test",
    fetchFn: stub.fetchFn,
    clientInstanceId: "desktop-test",
    pid: 4242,
    label: "Desktop 4242",
    heartbeatMs: 60_000, // long — tests drive heartbeatOnce() explicitly
    reconnectPollMs: 1,
    // unref'd real timers: live reconnect polls still fire, but pending
    // heartbeat timers never keep the node:test process alive.
    setTimeoutFn: ((fn: () => void, ms?: number) => {
      const h = setTimeout(fn, ms);
      (h as unknown as { unref?: () => void }).unref?.();
      return h;
    }) as typeof setTimeout,
    chooseClose: async () => choice,
    notify: (title, body) => {
      ports.notices.push({ title, body });
    },
    quit: () => {
      ports.quits++;
    },
    onStatus: (s) => {
      ports.statuses.push(s);
    },
  });
  return ports;
}

async function waitFor(cond: () => boolean, ms = 2_000): Promise<void> {
  const start = Date.now();
  while (!cond()) {
    if (Date.now() - start > ms) throw new Error("waitFor timed out");
    await new Promise((r) => setTimeout(r, 2));
  }
}

// ── §7 required tests ────────────────────────────────────────────────────────

test("TestDesktop_RegisterLeaseOncePerApp", async () => {
  const stub = stubRunner();
  const p = makePorts(stub);
  await p.lifecycle.start();
  assert.equal(stub.counts.register, 1);
  assert.equal(p.lifecycle.state().leaseId, "l-desk");
  assert.equal(p.lifecycle.state().runnerInstanceId, "inst-1");
  assert.equal(p.lifecycle.state().generation, 1);
});

test("TestDesktop_RendererReloadDoesNotCreateSecondLease", async () => {
  const stub = stubRunner();
  const p = makePorts(stub);
  await p.lifecycle.start();
  // A renderer reload / window re-creation re-invokes start() in main — the
  // lease must stay single.
  await p.lifecycle.start();
  await p.lifecycle.start();
  assert.equal(stub.counts.register, 1);
});

test("TestDesktop_HeartbeatKeepsLeaseAlive", async () => {
  const stub = stubRunner();
  const p = makePorts(stub);
  await p.lifecycle.start();
  await p.lifecycle.heartbeatOnce();
  await p.lifecycle.heartbeatOnce();
  assert.equal(stub.counts.heartbeat, 2);
  const hb = stub.calls.filter((c) => c.path.endsWith("/heartbeat"));
  assert.equal(hb[0]?.body?.leaseToken, "tok-1");
  assert.equal(hb[0]?.body?.runnerInstanceId, "inst-1");
});

test("TestDesktop_WindowCloseShowsThreeChoices", async () => {
  const stub = stubRunner();
  stub.setSnapshot({
    clients: [
      { leaseId: "l-desk", kind: "desktop", label: "Desktop" },
      { leaseId: "l-tui", kind: "tui", label: "TUI 99" },
    ],
  });
  let dialogShown = false;
  const p = makePorts(stub);
  // Rebuild with a spy on chooseClose.
  const spy = createDesktopLifecycle({
    runnerURL: "http://runner.test",
    fetchFn: stub.fetchFn,
    clientInstanceId: "desktop-test",
    pid: 1,
    label: "Desktop",
    heartbeatMs: 60_000,
    setTimeoutFn: ((fn: () => void, ms?: number) => {
      const h = setTimeout(fn, ms);
      (h as unknown as { unref?: () => void }).unref?.();
      return h;
    }) as typeof setTimeout,
    chooseClose: async (snap) => {
      dialogShown = true;
      const others = (snap.clients ?? []).filter((c) => c.leaseId !== "l-desk");
      assert.equal(others.length, 1);
      assert.equal(others[0]?.label, "TUI 99");
      return "cancel";
    },
    quit: () => {
      p.quits++;
    },
  });
  await spy.start();
  const result = await spy.requestQuit("window");
  assert.equal(result, "cancelled");
  assert.equal(dialogShown, true);
  assert.equal(p.quits, 0);
  assert.equal(stub.counts.release, 0);
  assert.equal(stub.counts.shutdown, 0);
});

test("TestDesktop_CloseDesktopOnlyReleasesLease", async () => {
  const stub = stubRunner();
  stub.setSnapshot({
    clients: [
      { leaseId: "l-desk", kind: "desktop", label: "Desktop" },
      { leaseId: "l-tui", kind: "tui", label: "TUI 99" },
    ],
  });
  const p = makePorts(stub);
  p.setCloseChoice("close_only");
  await p.lifecycle.start();
  const result = await p.lifecycle.requestQuit("window");
  assert.equal(result, "closed");
  assert.equal(stub.counts.release, 1);
  assert.equal(stub.counts.shutdown, 0);
  assert.equal(p.quits, 1);
});

test("TestDesktop_TurnOffWarnsThenForces", async () => {
  const stub = stubRunner();
  stub.setSnapshot({
    clients: [
      { leaseId: "l-desk", kind: "desktop" },
      { leaseId: "l-tui", kind: "tui" },
    ],
  });
  stub.shutdownFirst(409, "ct-77");
  const p = makePorts(stub);
  p.setCloseChoice("turn_off");
  await p.lifecycle.start();
  const result = await p.lifecycle.requestQuit("window");
  assert.equal(result, "closed");
  assert.equal(stub.counts.shutdown, 2);
  const confirmed = stub.shutdownBodies[1];
  assert.equal(confirmed?.confirm, true);
  assert.equal(confirmed?.confirmToken, "ct-77");
  assert.equal(confirmed?.requesterLeaseId, "l-desk");
  assert.equal(confirmed?.expectedInstanceId, "inst-1");
  assert.equal(p.quits, 1);
});

test("TestDesktop_RestartUsesConfirmTokenAndReconnects", async () => {
  const stub = stubRunner();
  stub.restartFirst(409, "rt-9");
  const p = makePorts(stub);
  await p.lifecycle.start();

  // After the fenced restart is accepted the runner dies then returns with a
  // new instance id — flip the stub's /health before the poll observes it.
  void (async () => {
    await new Promise((r) => setTimeout(r, 5));
    stub.setHealthInstance("inst-2");
  })();
  const res = await p.lifecycle.requestPlannedRestart("ipc");
  assert.equal(res.ok, true);
  assert.equal(stub.counts.restart, 2);
  assert.equal(stub.restartBodies[1]?.confirmToken, "rt-9");
  assert.equal(stub.restartBodies[1]?.confirm, true);

  // Reconnect: new instance → re-register under the new generation.
  await waitFor(() => stub.counts.register === 2);
  assert.equal(p.lifecycle.state().runnerInstanceId, "inst-2");
  assert.equal(p.quits, 0, "planned restart must keep the app open");
});

test("TestDesktop_PlannedRestartKeepsAppOpen", async () => {
  const stub = stubRunner();
  const p = makePorts(stub);
  await p.lifecycle.start();

  // Heartbeat observes draining_restart → reconnect mode, no quit.
  stub.setSnapshot({
    phase: "draining_restart",
    restart: { restartId: "rst-1", deadline: new Date(Date.now() + 30_000).toISOString() },
  });
  await p.lifecycle.heartbeatOnce();
  assert.equal(p.quits, 0);
  const st = p.lifecycle.state();
  assert.equal(st.phase, "draining_restart");
  assert.ok(st.reconnectDeadlineMs !== undefined);
  const lastStatus = p.statuses[p.statuses.length - 1];
  assert.equal(lastStatus?.reconnecting, true);
});

test("TestDesktop_UnplannedRunnerLossClosesWithNotice", async () => {
  const stub = stubRunner();
  const p = makePorts(stub);
  await p.lifecycle.start();

  stub.failHeartbeat("transport");
  await p.lifecycle.heartbeatOnce();
  assert.equal(p.quits, 1);
  assert.equal(p.notices.length, 1);
  assert.match(p.notices[0]?.body ?? "", /stopped unexpectedly/i);
});

test("TestDesktop_StatusShowsIdleCountdownAndUpdatePending", async () => {
  const stub = stubRunner();
  const idleDeadline = new Date(Date.now() + 21_000).toISOString();
  stub.setSnapshot({
    phase: "idle_grace",
    idleDeadline,
    updatePending: true,
    clients: [
      { leaseId: "l-desk", kind: "desktop" },
      { leaseId: "l-tui", kind: "tui" },
    ],
  });
  const p = makePorts(stub);
  await p.lifecycle.start();
  const snap = await p.lifecycle.getSnapshot();
  assert.equal(snap?.phase, "idle_grace");
  const last = p.statuses[p.statuses.length - 1];
  assert.equal(last?.updatePending, true);
  assert.equal(last?.sharedClients, 2);
  assert.ok(last?.idleDeadlineMs !== undefined && last.idleDeadlineMs > Date.now());
});

test("TestDesktop_MockRunnerClientLifecycleParity", async () => {
  const mock = new MockRunnerClient();
  // The RunnerClient contract: optional getLifecycleSnapshot is implemented by
  // both Http and Mock transports.
  const asContract: RunnerClient = mock;
  assert.equal(typeof asContract.getLifecycleSnapshot, "function");
  const snap = (await mock.getLifecycleSnapshot()) as LifecycleSnapshot;
  assert.equal(snap.phase, "ready");
  assert.equal(snap.lifecycleMode, "client-managed");
  assert.ok(Array.isArray(snap.clients) && snap.clients.length >= 1);
  assert.equal(snap.updatePending, false);
});
