// CP-81 Task-418: Desktop lifecycle owner — the Electron-main-side counterpart
// of the TUI lease participant (SD-28 D-2/D-5, §6.3). This module is pure TS
// with injected ports so it is unit-testable without Electron; electron/
// lifecycle.ts wires real dialog/Notification/app.quit into it.
//
// Contract mirrors internal/runner/lifecycle_api.go:
//   register → heartbeat(5s) → release on ordinary close
//   destructive actions are two-phase: 409 lifecycle_confirmation_required
//   carries a single-use confirmToken bound to the inventory revision.

export interface LifecycleClientLease {
  leaseId: string;
  clientInstanceId?: string;
  kind?: string;
  pid?: number;
  label?: string;
  projectPath?: string;
}

export interface LifecycleWorkloadItem {
  kind: string;
  runId?: string;
  projectId?: string;
  providerKey?: string;
  cancellable?: boolean;
  detail?: string;
}

export interface LifecycleRestartInfo {
  restartId: string;
  requestedAt?: string;
  deadline?: string;
}

export interface LifecycleSnapshot {
  runnerInstanceId?: string;
  generation?: number;
  protocolVersion?: number;
  buildId?: string;
  lifecycleMode?: string;
  phase?: string;
  clients?: LifecycleClientLease[];
  workload?: { items?: LifecycleWorkloadItem[] };
  idleDeadline?: string;
  restart?: LifecycleRestartInfo;
  updatePending?: boolean;
  inventoryRevision?: number;
  serverNow?: string;
}

// RunnerLifecycleState is the §6 introspection surface (token never leaves
// Electron main).
export interface RunnerLifecycleState {
  leaseId?: string;
  runnerInstanceId?: string;
  generation?: number;
  phase?: string;
  reconnectDeadlineMs?: number;
}

// LifecycleStatusEvent is what the renderer sees via IPC — no token material.
export interface LifecycleStatusEvent {
  connected: boolean;
  phase?: string;
  sharedClients: number;
  activeWork: number;
  idleDeadlineMs?: number;
  updatePending: boolean;
  reconnecting: boolean;
}

export type LifecycleCloseChoice = "cancel" | "close_only" | "turn_off";
export type LifecycleQuitSource = "window" | "app" | "ipc";

export interface LifecycleActionResult {
  ok: boolean;
  code?: string;
  restartId?: string;
  confirmationRequired?: boolean;
}

// LifecycleError mirrors the runner's {error:{code,message}} envelope; on 409
// it also carries the confirmToken + snapshot for the fenced replay.
export class LifecycleError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public confirmToken?: string,
    public snapshot?: LifecycleSnapshot,
  ) {
    super(message);
    this.name = "LifecycleError";
  }
}

export interface DesktopLifecyclePorts {
  runnerURL: string;
  /** Injected for tests; defaults to globalThis.fetch. */
  fetchFn?: typeof fetch;
  clientInstanceId: string;
  pid: number;
  label: string;
  projectPath?: string;
  /** Override heartbeat cadence (ms); otherwise the server value is used. */
  heartbeatMs?: number;
  /** Override register-retry cadence (ms); default REGISTER_RETRY_MS. */
  registerRetryMs?: number;
  /** Reconnect budget for planned restarts; default 60_000 (SD-28). */
  reconnectGraceMs?: number;
  /** /health poll cadence during reconnect; default 500ms. */
  reconnectPollMs?: number;
  now?: () => number;
  setTimeoutFn?: typeof setTimeout;
  clearTimeoutFn?: typeof clearTimeout;
  /** Native three-choice close dialog; must return one of the frozen labels' choice. */
  chooseClose?: (snap: LifecycleSnapshot) => Promise<LifecycleCloseChoice>;
  /** Native notice for unplanned runner loss. */
  notify?: (title: string, body: string) => void | Promise<void>;
  /** app.quit() — called after release/turn-off completes. */
  quit: () => void;
  /** Renderer status push (webContents.send in the Electron adapter). */
  onStatus?: (status: LifecycleStatusEvent) => void;
}

interface Lease {
  leaseId: string;
  token: string;
  runnerInstanceId: string;
  generation: number;
  heartbeatMs: number;
}

export interface DesktopLifecycle {
  start(): Promise<void>;
  stop(): void;
  state(): RunnerLifecycleState;
  getSnapshot(): Promise<LifecycleSnapshot | null>;
  requestQuit(source: LifecycleQuitSource): Promise<"cancelled" | "closed">;
  requestGlobalShutdown(source: string): Promise<LifecycleActionResult>;
  requestPlannedRestart(source: string): Promise<LifecycleActionResult>;
  heartbeatOnce(): Promise<void>;
  releaseLease(): Promise<void>;
  /** True while a quit flow is in flight (before-quit re-entry guard). */
  quitInFlight(): boolean;
}

const DEFAULT_HEARTBEAT_MS = 5_000;
const DEFAULT_RECONNECT_GRACE_MS = 60_000;
const DEFAULT_RECONNECT_POLL_MS = 500;
const CALL_TIMEOUT_MS = 4_000;
// Registration races runner boot (supervisor spawns the desktop while `go run`
// is still compiling): keep retrying so the app never stays unmanaged while a
// client-managed/supervised runner would idle-exit under it.
const REGISTER_RETRY_MS = 2_000;

export function createDesktopLifecycle(ports: DesktopLifecyclePorts): DesktopLifecycle {
  const fetchFn = ports.fetchFn ?? globalThis.fetch.bind(globalThis);
  const now = ports.now ?? (() => Date.now());
  const setT = ports.setTimeoutFn ?? setTimeout;
  const clearT = ports.clearTimeoutFn ?? clearTimeout;
  const base = ports.runnerURL.replace(/\/+$/, "");

  let lease: Lease | null = null;
  let phase: string | undefined;
  let heartbeatTimer: ReturnType<typeof setTimeout> | null = null;
  let registerRetryTimer: ReturnType<typeof setTimeout> | null = null;
  let reconnectDeadlineMs: number | null = null;
  let reconnecting = false;
  let quitting = false;
  let lastSnapshot: LifecycleSnapshot | null = null;

  function emitStatus(): void {
    ports.onStatus?.({
      connected: lease !== null || reconnecting,
      phase,
      sharedClients: lastSnapshot?.clients?.length ?? 0,
      activeWork: lastSnapshot?.workload?.items?.length ?? 0,
      idleDeadlineMs: lastSnapshot?.idleDeadline ? Date.parse(lastSnapshot.idleDeadline) : undefined,
      updatePending: lastSnapshot?.updatePending ?? false,
      reconnecting,
    });
  }

  async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
    const resp = await fetchFn(base + path, {
      method,
      headers: {
        Accept: "application/json",
        ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
        "X-Client": "desktop",
      },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(CALL_TIMEOUT_MS),
    });
    const text = await resp.text();
    const data = text ? JSON.parse(text) : undefined;
    if (resp.status === 409) {
      const err = data?.error ?? {};
      throw new LifecycleError(
        409,
        err.code ?? "conflict",
        err.message ?? resp.statusText,
        err.confirmToken,
        data?.snapshot,
      );
    }
    if (!resp.ok) {
      const err = data?.error ?? {};
      throw new LifecycleError(resp.status, err.code ?? `http_${resp.status}`, err.message ?? resp.statusText);
    }
    return data as T;
  }

  async function register(): Promise<boolean> {
    try {
      const res = await call<{
        leaseId: string;
        leaseToken: string;
        runnerInstanceId: string;
        generation: number;
        heartbeatIntervalMs?: number;
        snapshot?: LifecycleSnapshot;
      }>("POST", "/system/clients/register", {
        kind: "desktop",
        clientInstanceId: ports.clientInstanceId,
        pid: ports.pid,
        label: ports.label,
        projectPath: ports.projectPath,
      });
      lease = {
        leaseId: res.leaseId,
        token: res.leaseToken,
        runnerInstanceId: res.runnerInstanceId,
        generation: res.generation,
        heartbeatMs: ports.heartbeatMs ?? res.heartbeatIntervalMs ?? DEFAULT_HEARTBEAT_MS,
      };
      if (res.snapshot) {
        lastSnapshot = res.snapshot;
        phase = res.snapshot.phase;
      }
      emitStatus();
      return true;
    } catch {
      // Runner predates lifecycle or is gone — stay attached unmanaged.
      lease = null;
      emitStatus();
      return false;
    }
  }

  // ensureRegistered is the single entry point for acquiring a lease: on
  // failure it re-arms a retry (2s cadence) until a lease is held or the app
  // quits, so a boot-time race or a refused re-register can never leave the
  // desktop permanently invisible to the runner's idle accounting.
  async function ensureRegistered(): Promise<void> {
    if (lease || quitting) return;
    const ok = await register();
    if (ok) {
      scheduleHeartbeat();
    } else {
      scheduleRegisterRetry();
    }
  }

  function scheduleRegisterRetry(): void {
    if (registerRetryTimer || lease || quitting) return;
    registerRetryTimer = setT(() => {
      registerRetryTimer = null;
      void ensureRegistered();
    }, ports.registerRetryMs ?? REGISTER_RETRY_MS);
  }

  function scheduleHeartbeat(): void {
    if (heartbeatTimer || !lease) return;
    const tick = (): void => {
      heartbeatTimer = null;
      void heartbeatOnce().finally(() => {
        if (lease && !reconnecting && !quitting) {
          heartbeatTimer = setT(tick, lease.heartbeatMs);
        }
      });
    };
    heartbeatTimer = setT(tick, lease.heartbeatMs);
  }

  async function heartbeatOnce(): Promise<void> {
    if (!lease || reconnecting) return;
    const current = lease;
    try {
      const snap = await call<LifecycleSnapshot>(
        "POST",
        `/system/clients/${current.leaseId}/heartbeat`,
        {
          leaseToken: current.token,
          runnerInstanceId: current.runnerInstanceId,
          generation: current.generation,
        },
      );
      lastSnapshot = snap;
      phase = snap.phase;
      if (snap.phase === "draining_restart" && snap.restart && !reconnecting) {
        enterReconnect(snap.restart.deadline);
      }
      emitStatus();
    } catch (err) {
      if (
        err instanceof LifecycleError &&
        (err.code === "lease_unknown" || err.code === "stale_generation" || err.code === "invalid_lease_token")
      ) {
        // Lease expired or generation rotated — rejoin under a fresh lease.
        // If the re-register itself fails (runner mid-restart), the retry
        // timer keeps rejoining instead of staying unmanaged forever.
        lease = null;
        if (!(await register())) scheduleRegisterRetry();
        return;
      }
      if (phase === "draining_restart") {
        enterReconnect(lastSnapshot?.restart?.deadline);
        return;
      }
      await runnerLost("runner unreachable");
    }
  }

  function enterReconnect(deadlineISO?: string): void {
    reconnecting = true;
    reconnectDeadlineMs = deadlineISO ? Date.parse(deadlineISO) : now() + DEFAULT_RECONNECT_GRACE_MS;
    emitStatus();
    void pollReconnect();
  }

  async function pollReconnect(): Promise<void> {
    while (reconnecting && !quitting) {
      if (reconnectDeadlineMs !== null && now() > reconnectDeadlineMs) {
        await runnerLost("runner restart exceeded reconnect deadline");
        return;
      }
      try {
        const h = await call<{ runnerInstanceId?: string }>("GET", "/health");
        if (h.runnerInstanceId && h.runnerInstanceId !== lease?.runnerInstanceId) {
          // New instance is up — re-register under the new generation.
          reconnecting = false;
          reconnectDeadlineMs = null;
          lease = null;
          if (await register()) {
            if (!heartbeatTimer) scheduleHeartbeat();
          } else {
            scheduleRegisterRetry();
          }
          emitStatus();
          return;
        }
      } catch {
        // still down — keep polling
      }
      await new Promise((r) => setT(r, ports.reconnectPollMs ?? DEFAULT_RECONNECT_POLL_MS));
    }
  }

  async function runnerLost(reason: string): Promise<void> {
    if (heartbeatTimer) {
      clearT(heartbeatTimer);
      heartbeatTimer = null;
    }
    if (registerRetryTimer) {
      clearT(registerRetryTimer);
      registerRetryTimer = null;
    }
    reconnecting = false;
    reconnectDeadlineMs = null;
    quitting = true;
    emitStatus();
    await ports.notify?.("FlowPilot", "Runner stopped unexpectedly; FlowPilot will close");
    ports.quit();
  }

  async function releaseLease(): Promise<void> {
    const current = lease;
    lease = null;
    if (heartbeatTimer) {
      clearT(heartbeatTimer);
      heartbeatTimer = null;
    }
    if (!current) return;
    try {
      await call("POST", `/system/clients/${current.leaseId}/release`, {
        leaseToken: current.token,
        runnerInstanceId: current.runnerInstanceId,
        generation: current.generation,
      });
    } catch (err) {
      // lease_unknown on an already-gone lease is success; transport loss is
      // covered by TTL expiry server-side.
      if (!(err instanceof LifecycleError && err.code === "lease_unknown")) {
        // swallow — release is best-effort on the close path
      }
    }
    emitStatus();
  }

  async function fencedAction(path: string, reason: string): Promise<LifecycleActionResult> {
    const body: Record<string, unknown> = {
      reason,
      ...(lease
        ? {
            requesterLeaseId: lease.leaseId,
            leaseToken: lease.token,
            expectedInstanceId: lease.runnerInstanceId,
          }
        : {}),
    };
    try {
      const res = await call<{ status?: string; restartId?: string }>("POST", path, body);
      return { ok: true, restartId: res?.restartId };
    } catch (err) {
      if (err instanceof LifecycleError && err.code === "lifecycle_confirmation_required" && err.confirmToken) {
        const res = await call<{ status?: string; restartId?: string }>("POST", path, {
          ...body,
          confirm: true,
          confirmToken: err.confirmToken,
        });
        return { ok: true, restartId: res?.restartId };
      }
      if (err instanceof LifecycleError) {
        return { ok: false, code: err.code, confirmationRequired: err.code === "lifecycle_confirmation_required" };
      }
      throw err;
    }
  }

  return {
    async start(): Promise<void> {
      // Exactly one Desktop lease per app process (T-1): renderer reloads and
      // window recreation re-run start() but must not mint a second lease.
      if (lease || quitting) return;
      await ensureRegistered();
    },

    stop(): void {
      if (heartbeatTimer) {
        clearT(heartbeatTimer);
        heartbeatTimer = null;
      }
      if (registerRetryTimer) {
        clearT(registerRetryTimer);
        registerRetryTimer = null;
      }
    },

    state(): RunnerLifecycleState {
      return {
        leaseId: lease?.leaseId,
        runnerInstanceId: lease?.runnerInstanceId,
        generation: lease?.generation,
        phase,
        reconnectDeadlineMs: reconnectDeadlineMs ?? undefined,
      };
    },

    async getSnapshot(): Promise<LifecycleSnapshot | null> {
      try {
        lastSnapshot = await call<LifecycleSnapshot>("GET", "/system/lifecycle");
        phase = lastSnapshot.phase;
        emitStatus();
        return lastSnapshot;
      } catch {
        return null;
      }
    },

    async requestQuit(_source: LifecycleQuitSource): Promise<"cancelled" | "closed"> {
      if (quitting) return "closed";
      quitting = true;
      let snap: LifecycleSnapshot | null = null;
      try {
        snap = await call<LifecycleSnapshot>("GET", "/system/lifecycle");
      } catch {
        // unmanaged/gone — release best-effort and quit
      }
      const others = (snap?.clients ?? []).filter((c) => c.leaseId !== lease?.leaseId);
      const work = snap?.workload?.items?.length ?? 0;
      if (!snap || (others.length === 0 && work === 0)) {
        await releaseLease();
        ports.quit();
        return "closed";
      }
      const choice = (await ports.chooseClose?.(snap)) ?? "cancel";
      if (choice === "cancel") {
        quitting = false;
        return "cancelled";
      }
      if (choice === "turn_off") {
        await fencedAction("/system/shutdown", "user_exit").catch(() => {});
      } else {
        await releaseLease();
      }
      ports.quit();
      return "closed";
    },

    async requestGlobalShutdown(_source: string): Promise<LifecycleActionResult> {
      return fencedAction("/system/shutdown", "desktop_request");
    },

    async requestPlannedRestart(_source: string): Promise<LifecycleActionResult> {
      const res = await fencedAction("/system/restart", "desktop_request");
      if (res.ok) {
        phase = "draining_restart";
        enterReconnect(lastSnapshot?.restart?.deadline);
      }
      return res;
    },

    heartbeatOnce,
    releaseLease,
    quitInFlight: () => quitting,
  };
}
