import test from "node:test";
import assert from "node:assert/strict";
import { PtyRegistry, resolveShell, type PtyLike, type TermSpawnRequest } from "./ptyRegistry";
import { createTermApi } from "./termBridge";

// Task-427 (CP-83): terminal pty registry + preload bridge factory. The
// registry is pure node (no Electron); the bridge is a factory over injected
// IPC primitives. New file — additive only.

class FakePty implements PtyLike {
  data: string[] = [];
  resized: [number, number][] = [];
  killed = false;
  private dataCbs: ((d: string) => void)[] = [];
  private exitCbs: ((e: { exitCode: number; signal?: number }) => void)[] = [];
  write(d: string): void {
    this.data.push(d);
  }
  resize(c: number, r: number): void {
    this.resized.push([c, r]);
  }
  kill(): void {
    this.killed = true;
    for (const cb of this.exitCbs) cb({ exitCode: 0 });
  }
  onData(cb: (d: string) => void): void {
    this.dataCbs.push(cb);
  }
  onExit(cb: (e: { exitCode: number; signal?: number }) => void): void {
    this.exitCbs.push(cb);
  }
  emitData(d: string): void {
    for (const cb of this.dataCbs) cb(d);
  }
  emitExit(code: number): void {
    for (const cb of this.exitCbs) cb({ exitCode: code });
  }
}

function makeRegistry() {
  const spawned: { pty: FakePty; req: Required<Omit<TermSpawnRequest, "shell">> & { shell: string } }[] = [];
  const events: { channel: "data" | "exit"; id: string; payload: string | number }[] = [];
  const registry = new PtyRegistry(
    (req) => {
      const pty = new FakePty();
      spawned.push({ pty, req });
      return pty;
    },
    {
      onData: (id, data) => events.push({ channel: "data", id, payload: data }),
      onExit: (id, exitCode) => events.push({ channel: "exit", id, payload: exitCode }),
    },
  );
  return { registry, spawned, events };
}

test("resolveShell honors explicit request, then SHELL, then platform default", () => {
  assert.equal(resolveShell("zsh", {}, "linux"), "zsh");
  assert.equal(resolveShell(undefined, { SHELL: "/bin/fish" }, "linux"), "/bin/fish");
  assert.equal(resolveShell(undefined, {}, "linux"), "/bin/sh");
  assert.equal(resolveShell(undefined, { COMSPEC: "C:\\Windows\\System32\\cmd.exe" }, "win32"), "C:\\Windows\\System32\\cmd.exe");
  assert.equal(resolveShell(undefined, {}, "win32"), "powershell.exe");
  assert.equal(resolveShell("  ", {}, "darwin"), "/bin/sh", "blank shell falls through");
});

test("spawn registers the pty and forwards data/exit events", () => {
  const { registry, spawned, events } = makeRegistry();
  const { id } = registry.spawn({ cwd: "C:/repo", cols: 120, rows: 30 });
  assert.ok(registry.has(id));
  assert.equal(spawned.length, 1);
  assert.equal(spawned[0].req.cwd, "C:/repo");
  assert.equal(spawned[0].req.cols, 120);

  spawned[0].pty.emitData("hello$ ");
  assert.deepEqual(events, [{ channel: "data", id, payload: "hello$ " }]);

  spawned[0].pty.emitExit(0);
  assert.deepEqual(events.at(-1), { channel: "exit", id, payload: 0 });
  assert.equal(registry.has(id), false, "exit removes the registry entry");
});

test("write/resize/kill forward to the owned pty; unknown ids are no-ops", () => {
  const { registry, spawned } = makeRegistry();
  const { id } = registry.spawn({ cwd: "C:/repo", cols: 80, rows: 24 });
  registry.write(id, "dir\n");
  registry.resize(id, 132, 40);
  assert.deepEqual(spawned[0].pty.data, ["dir\n"]);
  assert.deepEqual(spawned[0].pty.resized, [[132, 40]]);

  assert.equal(registry.kill("term-nope").ok, false, "unknown id → ok:false");
  assert.equal(registry.kill(id).ok, true);
  assert.equal(spawned[0].pty.killed, true);
  assert.equal(registry.has(id), false, "kill removes the entry");
  assert.equal(registry.kill(id).ok, false, "double kill is ok:false");
});

test("spawn failure leaves no registry entry", () => {
  const registry = new PtyRegistry(
    () => {
      throw new Error("cwd missing");
    },
    { onData: () => {}, onExit: () => {} },
  );
  assert.throws(() => registry.spawn({ cwd: "C:/missing", cols: 80, rows: 24 }), /cwd missing/);
  assert.equal(registry.size, 0);
});

test("killAll empties the registry (window destroy / app quit)", () => {
  const { registry, spawned, events } = makeRegistry();
  const a = registry.spawn({ cwd: "C:/a", cols: 80, rows: 24 });
  const b = registry.spawn({ cwd: "C:/b", cols: 80, rows: 24 });
  registry.killAll();
  assert.equal(registry.size, 0);
  assert.ok(spawned[0].pty.killed && spawned[1].pty.killed);
  // kill() emits FakePty.onExit synchronously — but registry removed the entries
  // first, so no exit events should leak for force-killed ptys.
  assert.equal(events.filter((e) => e.channel === "exit").length, 0);
  void a;
  void b;
});

test("clamps degenerate geometry instead of passing 0/NaN to the pty", () => {
  const { registry, spawned } = makeRegistry();
  registry.spawn({ cwd: "C:/repo", cols: 0, rows: Number.NaN });
  assert.equal(spawned[0].req.cols, 80, "0 cols → default 80");
  assert.equal(spawned[0].req.rows, 24, "NaN rows → default 24");
});

test("term bridge methods invoke the correct IPC channels", async () => {
  const calls: { channel: string; payload: unknown }[] = [];
  const api = createTermApi(
    async (channel, payload) => {
      calls.push({ channel, payload });
      if (channel === "term:spawn") return { id: "term-1" };
      if (channel === "term:kill") return { ok: true };
      return undefined;
    },
    () => {},
    () => {},
  );
  assert.deepEqual(await api.spawn({ cwd: "C:/r", cols: 80, rows: 24 }), { id: "term-1" });
  await api.write("term-1", "ls\n");
  await api.resize("term-1", 100, 30);
  assert.deepEqual(await api.kill("term-1"), { ok: true });
  assert.deepEqual(
    calls.map((c) => c.channel),
    ["term:spawn", "term:write", "term:resize", "term:kill"],
  );
  assert.deepEqual(calls[1].payload, { id: "term-1", data: "ls\n" });
  assert.deepEqual(calls[2].payload, { id: "term-1", cols: 100, rows: 30 });
});

test("onData/onExit subscribe and unsubscribe cleanly", () => {
  const subs = new Map<string, (e: unknown, p: never) => void>();
  const api = createTermApi(
    async () => undefined,
    (channel, listener) => subs.set(channel, listener),
    (channel, listener) => {
      if (subs.get(channel) === listener) subs.delete(channel);
    },
  );
  const data: string[] = [];
  const exits: number[] = [];
  const offData = api.onData((e) => data.push(`${e.id}:${e.data}`));
  const offExit = api.onExit((e) => exits.push(e.exitCode));
  assert.equal(subs.size, 2);
  subs.get("term:data")!(null, { id: "t1", data: "out" } as never);
  subs.get("term:exit")!(null, { id: "t1", exitCode: 3 } as never);
  assert.deepEqual(data, ["t1:out"]);
  assert.deepEqual(exits, [3]);
  offData();
  offExit();
  assert.equal(subs.size, 0, "unsubscribed listeners are removed");
});
