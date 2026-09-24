// CP-81 Task-419: supervisor lifecycle-contract tests. The supervisor module
// is plain CJS — required fresh per test (require cache reset) so module
// state never leaks between cases. child_process.spawn and process.kill are
// patched only around the requiring/acting scope and always restored.
import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { mkdtempSync, writeFileSync, existsSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

const require2 = createRequire(__filename);
const SUPERVISOR_PATH = resolve(__dirname, "../../../scripts/supervisor.js");

interface FakeChild {
  pid: number;
  detached: boolean;
  exitCode: number | null;
  killed: boolean;
  on(ev: string, fn: (...a: unknown[]) => void): void;
  emit(ev: string, ...a: unknown[]): void;
}

let nextFakePid = 900_000;
function fakeChild(): FakeChild {
  const handlers: Record<string, Array<(...a: unknown[]) => void>> = {};
  return {
    pid: nextFakePid++,
    detached: process.platform !== "win32",
    exitCode: null,
    killed: false,
    on(ev, fn) {
      (handlers[ev] ||= []).push(fn);
    },
    emit(ev, ...a) {
      for (const fn of handlers[ev] ?? []) fn(...a);
    },
  };
}

interface SpawnRecord {
  cmd: string;
  args: string[];
  opts: { env?: Record<string, string>; cwd?: string; detached?: boolean };
  child: FakeChild;
}

interface Patched {
  supervisor: any;
  spawned: SpawnRecord[];
  kills: Array<[number, unknown]>;
  restore(): void;
}

// freshSupervisor re-requires the module with spawn/kill intercepted so tests
// observe real supervisor behavior without touching processes.
function freshSupervisor(): Patched {
  const cp = require2("node:child_process");
  const spawned: SpawnRecord[] = [];
  const kills: Array<[number, unknown]> = [];
  const origSpawn = cp.spawn;
  const origExecSync = cp.execSync;
  const origKill = process.kill;

  cp.spawn = (cmd: string, args: string[], opts: SpawnRecord["opts"]) => {
    const child = fakeChild();
    spawned.push({ cmd, args, opts, child });
    return child;
  };
  // Windows cleanup paths (signalManagedProcess/killProcessTree) run
  // `taskkill /F /T /PID <pid>` through execSync instead of process.kill
  // (BUG-240). Record each tree-kill as a SIGKILL-equivalent so the kill
  // ladder is observable on win32; non-taskkill execSync calls (netstat,
  // wmic, …) return empty output, matching the no-real-processes sandbox.
  (cp as { execSync: unknown }).execSync = (cmd: string): string => {
    const m = /taskkill \/F \/T \/PID (\d+)/.exec(String(cmd));
    if (m) kills.push([Number(m[1]), "SIGKILL"]);
    return "";
  };
  // isPidAlive/signal/kill all funnel through process.kill. Returning true
  // keeps fake children "alive" so cleanup reaches the force-kill ladder.
  (process as { kill: unknown }).kill = (pid: number, sig?: unknown): boolean => {
    kills.push([pid, sig ?? "SIGTERM"]);
    return true;
  };

  const modPath = require2.resolve(SUPERVISOR_PATH);
  delete require2.cache[modPath];
  const supervisor = require2(modPath);
  return {
    supervisor,
    spawned,
    kills,
    restore() {
      cp.spawn = origSpawn;
      (cp as { execSync: unknown }).execSync = origExecSync;
      (process as { kill: unknown }).kill = origKill;
      delete require2.cache[modPath];
    },
  };
}

// freshSupervisorDeadChildren variant: fake PIDs read as dead (ESRCH), so
// cleanup exits fast without the 3s force-kill ladder.
function freshSupervisorDeadChildren(): Patched {
  const patched = freshSupervisor();
  const origKill = (process as { kill: unknown }).kill;
  void origKill;
  (process as { kill: unknown }).kill = (pid: number, sig?: unknown): boolean => {
    if (sig === 0) {
      const err = new Error("ESRCH") as Error & { code: string };
      err.code = "ESRCH";
      throw err;
    }
    patched.kills.push([pid, sig ?? "SIGTERM"]);
    return true;
  };
  return patched;
}

function tmpControl(): string {
  const dir = mkdtempSync(join(tmpdir(), "fp-supervisor-"));
  return join(dir, "supervisor.cmd");
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

// ── §7 required tests ────────────────────────────────────────────────────────

test("TestSupervisor_StartsRunnerInSupervisedMode", () => {
  const p = freshSupervisor();
  try {
    p.supervisor._internals.setRunnerPort("4999");
    p.supervisor.startRunnerProcess();
    assert.equal(p.spawned.length, 1);
    const rec = p.spawned[0]!;
    assert.deepEqual(rec.args.slice(0, 4), ["run", "./cmd/flowpilot", "runner", "serve"]);
    const env = rec.opts.env ?? {};
    assert.equal(env.FLOWPILOT_LIFECYCLE_MODE, "supervised");
    assert.equal(env.FLOWPILOT_RUNNER_LIFECYCLE_MODE, "supervised");
    assert.ok(env.SUPERVISOR_PID, "supervisor PID must be passed for fencing metadata");
    assert.equal(env.FLOWPILOT_SUPERVISOR_PID, env.SUPERVISOR_PID);
  } finally {
    p.restore();
  }
});

test("TestSupervisor_DoesNotRegisterPermanentUserLease", () => {
  // The supervisor is a controller, not a user client (T-1/Q-1): it must
  // never hold a lease — assert the source carries no client-lease calls.
  const src = readFileSync(SUPERVISOR_PATH, "utf8");
  assert.ok(!src.includes("/system/clients/register"), "supervisor must not register a lease");
  assert.ok(!src.includes("/system/clients/"), "supervisor must not call client-lease endpoints");
});

test("TestSupervisor_RunnerPlannedRestartRestartsOnlyRunner", async () => {
  const p = freshSupervisorDeadChildren();
  try {
    const sup = p.supervisor;
    sup._internals.setNoExit(true);
    sup._internals.setRunnerPort("1"); // refused fast — falls back to cache
    sup._internals.setRunnerInstanceId("inst-1");
    sup._internals.setRunnerProcess(fakeChild());
    const ctl = tmpControl();
    sup._internals.setControlPath(ctl);
    writeFileSync(
      ctl,
      JSON.stringify({
        action: "restart",
        runnerInstanceId: "inst-1",
        restartId: "rst-9",
        requestedAt: new Date().toISOString(),
        expiresAt: new Date(Date.now() + 60_000).toISOString(),
        requester: "runner",
      }),
    );

    await sup.pollSupervisorCommand();
    assert.equal(existsSync(ctl), false, "valid command must be consumed");
    await sleep(600); // deferred 200ms + respawn

    assert.equal(p.spawned.length, 1, "planned restart must respawn exactly one process");
    assert.deepEqual(
      p.spawned[0]!.args.slice(0, 4),
      ["run", "./cmd/flowpilot", "runner", "serve"],
      "only the runner is respawned — web/desktop stay up",
    );
    assert.equal(sup._internals.getPlannedRunnerRestart(), false);
    assert.equal(sup._internals.getExitedForTest(), false);
  } finally {
    p.restore();
  }
});

test("TestSupervisor_UnexpectedRunnerExitDoesNotGhostRestart", async () => {
  const p = freshSupervisorDeadChildren();
  try {
    const sup = p.supervisor;
    sup._internals.setNoExit(true);
    sup._internals.setRunnerPort("1");
    sup._internals.setRunnerProcess(fakeChild());
    sup._internals.setControlPath(tmpControl());

    sup.handleRunnerExitUnexpected(137, null);
    await sleep(500); // 200ms poll interval → finishExit

    assert.equal(sup._internals.getExitedForTest(), true, "unexpected exit must end the stack");
    assert.equal(p.spawned.length, 0, "no silent respawn after unplanned runner death");
  } finally {
    p.restore();
  }
});

test("TestSupervisor_IgnoresStaleCommandForOldInstance", async () => {
  const p = freshSupervisorDeadChildren();
  try {
    const sup = p.supervisor;
    sup._internals.setNoExit(true);
    sup._internals.setRunnerPort("1");
    sup._internals.setRunnerInstanceId("inst-new");
    const ctl = tmpControl();
    sup._internals.setControlPath(ctl);
    writeFileSync(
      ctl,
      JSON.stringify({
        action: "shutdown",
        runnerInstanceId: "inst-old",
        requestedAt: new Date(Date.now() - 5_000).toISOString(),
        expiresAt: new Date(Date.now() + 60_000).toISOString(),
        requester: "runner",
      }),
    );

    await sup.pollSupervisorCommand();
    assert.equal(existsSync(ctl), false, "stale command must be removed");
    await sleep(400);
    assert.equal(sup._internals.getExitedForTest(), false, "stale command must not act");
  } finally {
    p.restore();
  }
});

test("TestSupervisor_ForceShutdownStopsRunnerWebDesktop", async () => {
  const p = freshSupervisor(); // children stay "alive" → exercises force-kill ladder
  try {
    const sup = p.supervisor;
    sup._internals.setNoExit(true);
    sup._internals.setRunnerPort("1");
    sup._internals.setRunnerInstanceId("inst-1");
    const runner = fakeChild();
    const web = fakeChild();
    const desktop = fakeChild();
    const libre = fakeChild();
    sup._internals.setRunnerProcess(runner);
    sup._internals.setWebProcess(web);
    sup._internals.setDesktopProcess(desktop);
    sup._internals.setLibreProcess(libre);
    const ctl = tmpControl();
    sup._internals.setControlPath(ctl);
    writeFileSync(
      ctl,
      JSON.stringify({
        action: "shutdown",
        runnerInstanceId: "inst-1",
        requestedAt: new Date().toISOString(),
        expiresAt: new Date(Date.now() + 60_000).toISOString(),
        requester: "runner",
      }),
    );

    await sup.pollSupervisorCommand();
    // 200ms defer + up to 3s grace + force kill
    await sleep(4_200);

    assert.equal(sup._internals.getExitedForTest(), true);
    const wanted = [runner.pid, web.pid, desktop.pid, libre.pid];
    for (const pid of wanted) {
      const sigs = p.kills.filter(([k]) => Math.abs(k) === pid).map(([, s]) => s);
      if (process.platform === "win32") {
        // Windows has no graceful signal ladder: both the SIGINT pass and the
        // force-kill pass collapse to `taskkill /F /T` (BUG-240), recorded as
        // SIGKILL-equivalents. Two hits prove the escalation ran end-to-end.
        const treeKills = sigs.filter((s) => s === "SIGKILL").length;
        assert.ok(treeKills >= 2, `pid ${pid} must be tree-killed on grace+force passes, got ${JSON.stringify(sigs)}`);
      } else {
        assert.ok(sigs.includes("SIGINT"), `pid ${pid} must get graceful SIGINT, got ${JSON.stringify(sigs)}`);
        assert.ok(sigs.includes("SIGKILL"), `pid ${pid} must get force SIGKILL, got ${JSON.stringify(sigs)}`);
      }
    }
  } finally {
    p.restore();
  }
});

test("TestSupervisor_WindowsTreeKillIncludesCompiledRunner", () => {
  // BUG-240 regression guard: on Windows the runner is spawned via `go run`
  // (wrapper) and the compiled binary is a separate child — the tree kill must
  // use taskkill /F /T so both die. Asserted at source level (deterministic
  // across platforms) plus behaviorally via the exported tree-kill.
  const src = readFileSync(SUPERVISOR_PATH, "utf8");
  assert.match(src, /taskkill \/F \/T \/PID/, "Windows tree kill must keep /T");
  assert.match(src, /BUG-240/, "BUG-240 rationale must stay documented");

  const p = freshSupervisor();
  try {
    const child = fakeChild();
    p.supervisor.shutdownRunnerTree(child);
    if (process.platform === "win32") {
      // execSync path — asserted by the source check above.
    } else {
      const sigs = p.kills.filter(([k]) => Math.abs(k) === child.pid).map(([, s]) => s);
      assert.ok(sigs.includes("SIGKILL"), `tree kill must SIGKILL the child, got ${JSON.stringify(sigs)}`);
    }
  } finally {
    p.restore();
  }
});

test("TestSupervisor_TerminalCloseLeavesNoOrphans", async () => {
  const p = freshSupervisor();
  try {
    const sup = p.supervisor;
    sup._internals.setNoExit(true);
    sup._internals.setRunnerPort("1");
    const runner = fakeChild();
    const web = fakeChild();
    const desktop = fakeChild();
    sup._internals.setRunnerProcess(runner);
    sup._internals.setWebProcess(web);
    sup._internals.setDesktopProcess(desktop);
    sup._internals.setControlPath(tmpControl());
    sup._internals.installSignalHandlers();

    process.emit("SIGHUP");
    await sleep(4_200); // grace ladder → force kill → finishExit

    assert.equal(sup._internals.getExitedForTest(), true);
    for (const pid of [runner.pid, web.pid, desktop.pid]) {
      const sigs = p.kills.filter(([k]) => Math.abs(k) === pid).map(([, s]) => s);
      assert.ok(sigs.length > 0, `orphan guard: pid ${pid} was never signaled`);
      assert.ok(sigs.includes("SIGKILL"), `orphan guard: pid ${pid} survived`);
    }
  } finally {
    process.removeAllListeners("SIGHUP");
    p.restore();
  }
});

test("TestSupervisor_CommandExpiryIgnored", async () => {
  const p = freshSupervisorDeadChildren();
  try {
    const sup = p.supervisor;
    sup._internals.setNoExit(true);
    sup._internals.setRunnerPort("1");
    sup._internals.setRunnerInstanceId("inst-1");
    const ctl = tmpControl();
    sup._internals.setControlPath(ctl);
    writeFileSync(
      ctl,
      JSON.stringify({
        action: "shutdown",
        runnerInstanceId: "inst-1",
        requestedAt: new Date(Date.now() - 120_000).toISOString(),
        expiresAt: new Date(Date.now() - 1_000).toISOString(), // already expired
        requester: "runner",
      }),
    );

    await sup.pollSupervisorCommand();
    assert.equal(existsSync(ctl), false, "expired command must be removed");
    await sleep(400);
    assert.equal(sup._internals.getExitedForTest(), false, "expired command must not act");
  } finally {
    p.restore();
  }
});
