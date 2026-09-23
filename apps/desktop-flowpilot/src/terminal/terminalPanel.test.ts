import test from "node:test";
import assert from "node:assert/strict";
import { resolveTerminalCwd } from "./cwd";
import { TerminalTabs } from "./termTabs";
import type { TermSpawnRequest } from "./termBridge";
import type { Project, RunHistoryItem } from "@/types/contract";

// Task-428: cwd resolution + tab lifecycle. Component is Electron/xterm-bound
// and smoke-tested live; all decision logic is pure here.

function project(id: string, path: string): Project {
  return { id, name: id, path } as unknown as Project;
}

function historyItem(runId: string, worktreePath?: string): RunHistoryItem {
  return { runId, worktreePath } as unknown as RunHistoryItem;
}

test("resolveTerminalCwd returns worktreePath for bound focused run", () => {
  const cwd = resolveTerminalCwd({
    runId: "run-1",
    runHistory: [historyItem("run-1", "/repo/.flowpilot/worktrees/run-1")],
    selectedProjectId: "p1",
    projects: [project("p1", "/repo")],
  });
  assert.equal(cwd, "/repo/.flowpilot/worktrees/run-1");
});

test("resolveTerminalCwd prefers activeWorktreePath set from RunHandle", () => {
  const cwd = resolveTerminalCwd({
    runId: "run-1",
    activeWorktreePath: "/repo/.flowpilot/worktrees/chat-9",
    runHistory: [],
    selectedProjectId: "p1",
    projects: [project("p1", "/repo")],
  });
  assert.equal(cwd, "/repo/.flowpilot/worktrees/chat-9");
});

test("resolveTerminalCwd falls back to project.path when unbound", () => {
  const cwd = resolveTerminalCwd({
    runId: "run-2",
    runHistory: [historyItem("run-2")],
    selectedProjectId: "p1",
    projects: [project("p1", "/repo")],
  });
  assert.equal(cwd, "/repo");
});

test("resolveTerminalCwd returns null with no project selected", () => {
  const cwd = resolveTerminalCwd({
    runId: null,
    runHistory: [],
    selectedProjectId: null,
    projects: [project("p1", "/repo")],
  });
  assert.equal(cwd, null);
});

interface SpawnCall extends TermSpawnRequest {
  id: string;
}

function fakeBridge(opts?: { failSpawn?: boolean }) {
  const calls: { spawns: SpawnCall[]; writes: [string, string][]; kills: string[]; resizes: [string, number, number][] } = {
    spawns: [],
    writes: [],
    kills: [],
    resizes: [],
  };
  let seq = 0;
  const bridge = {
    async spawn(req: TermSpawnRequest): Promise<{ id: string }> {
      if (opts?.failSpawn) throw new Error("spawn ENOENT: no such cwd");
      const id = `pty-${++seq}`;
      calls.spawns.push({ ...req, id });
      return { id };
    },
    async write(id: string, data: string) {
      calls.writes.push([id, data]);
    },
    async resize(id: string, cols: number, rows: number) {
      calls.resizes.push([id, cols, rows]);
    },
    async kill(id: string) {
      calls.kills.push(id);
      return { ok: true };
    },
  };
  return { bridge, calls };
}

test("open spawns pty with resolved cwd and pins it on the tab", async () => {
  const { bridge, calls } = fakeBridge();
  const tabs = new TerminalTabs(
    bridge,
    () => "/repo/.flowpilot/worktrees/run-1",
    () => {},
  );
  const id = await tabs.open(120, 30);
  assert.equal(calls.spawns.length, 1);
  assert.equal(calls.spawns[0].cwd, "/repo/.flowpilot/worktrees/run-1");
  assert.equal(calls.spawns[0].cols, 120);
  assert.equal(calls.spawns[0].rows, 30);
  const tab = tabs.tabs.find((t) => t.id === id);
  assert.ok(tab);
  assert.equal(tab.cwd, "/repo/.flowpilot/worktrees/run-1");
  assert.equal(tab.title, "run-1");
  assert.equal(tabs.activeId, id);
});

test("open rejects with no project selected — no tab created", async () => {
  const { bridge, calls } = fakeBridge();
  const tabs = new TerminalTabs(bridge, () => null, () => {});
  await assert.rejects(tabs.open(80, 24), /no project selected/);
  assert.equal(calls.spawns.length, 0);
  assert.equal(tabs.tabs.length, 0);
});

test("spawn failure propagates IPC error — no tab, no silent fallback", async () => {
  const { bridge } = fakeBridge({ failSpawn: true });
  const tabs = new TerminalTabs(bridge, () => "/gone/worktree", () => {});
  await assert.rejects(tabs.open(80, 24), /ENOENT/);
  assert.equal(tabs.tabs.length, 0);
});

test("exited pty marks tab; close kills via bridge only when running", async () => {
  const { bridge, calls } = fakeBridge();
  const tabs = new TerminalTabs(bridge, () => "/repo", () => {});
  const a = await tabs.open(80, 24);
  const b = await tabs.open(80, 24);

  tabs.markExited(a);
  assert.equal(tabs.tabs.find((t) => t.id === a)?.exited, true);

  // Exited tab closes without another kill.
  await tabs.close(a);
  assert.equal(calls.kills.filter((k) => k === a).length, 0);
  assert.equal(tabs.tabs.some((t) => t.id === a), false);
  assert.equal(tabs.activeId, b);

  // Running tab close kills through the bridge.
  await tabs.close(b);
  assert.deepEqual(calls.kills, [b]);
  assert.equal(tabs.tabs.length, 0);
  assert.equal(tabs.activeId, null);
});

test("activate switches active tab; closing active falls back to a neighbor", async () => {
  const { bridge } = fakeBridge();
  const tabs = new TerminalTabs(bridge, () => "/repo", () => {});
  const a = await tabs.open(80, 24);
  const b = await tabs.open(80, 24);
  const c = await tabs.open(80, 24);

  tabs.activate(a);
  assert.equal(tabs.activeId, a);
  await tabs.close(a);
  assert.equal(tabs.activeId, b);
  void c;
});

test("write/resize forward to the bridge untouched", async () => {
  const { bridge, calls } = fakeBridge();
  const tabs = new TerminalTabs(bridge, () => "/repo", () => {});
  const id = await tabs.open(80, 24);
  tabs.write(id, "ls -la\r");
  tabs.resize(id, 132, 40);
  await Promise.resolve();
  assert.deepEqual(calls.writes, [[id, "ls -la\r"]]);
  assert.deepEqual(calls.resizes, [[id, 132, 40]]);
});

// toggleTerminal flips the session-only dock flag (AC-4).
test("toggleTerminal flips terminalOpen", async () => {
  const { useStore } = await import("../state/store.js");
  const before = useStore.getState().terminalOpen;
  useStore.getState().toggleTerminal();
  assert.equal(useStore.getState().terminalOpen, !before);
  useStore.getState().toggleTerminal();
  assert.equal(useStore.getState().terminalOpen, before);
});
