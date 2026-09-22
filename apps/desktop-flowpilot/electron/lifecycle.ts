import { app, BrowserWindow, dialog, ipcMain, Notification } from "electron";

import {
  createDesktopLifecycle,
  type DesktopLifecycle,
  type LifecycleSnapshot,
} from "../src/lifecycle/desktopLifecycle";

// CP-81 Task-418: Electron adapter for the Desktop lifecycle owner. The pure
// decision logic lives in src/lifecycle/desktopLifecycle.ts (unit-tested);
// this file only wires real Electron ports: native dialog, Notification,
// app.quit, and the renderer-facing IPC bridge.

export interface WiredDesktopLifecycle {
  lifecycle: DesktopLifecycle;
  /** before-quit handler — prevents default while a quit decision is in flight. */
  onBeforeQuit: (event: { preventDefault: () => void }) => void;
}

export function wireDesktopLifecycle(runnerURL: string): WiredDesktopLifecycle {
  const lifecycle = createDesktopLifecycle({
    runnerURL,
    clientInstanceId: `desktop-${process.pid}`,
    pid: process.pid,
    label: `Desktop ${process.pid}`,
    quit: () => app.quit(),
    notify: async (title, body) => {
      try {
        new Notification({ title, body }).show();
      } catch (err) {
        console.error("[lifecycle] notification failed:", err);
      }
    },
    chooseClose: async (snap: LifecycleSnapshot) => {
      const others = (snap.clients ?? []).filter(
        (c) => c.leaseId !== lifecycle.state().leaseId,
      );
      const work = snap.workload?.items ?? [];
      const detailLines = [
        ...others.map((c) => `Client: ${c.label ?? c.kind ?? c.leaseId}`),
        ...work.map((w) => `Work: ${w.kind}${w.runId ? ` ${w.runId}` : ""}`),
      ];
      const target = BrowserWindow.getAllWindows()[0] ?? null;
      const opts = {
        type: "warning" as const,
        title: "FlowPilot",
        message: "The runner is shared or has active work.",
        detail: detailLines.join("\n") || "Other clients or work are active.",
        buttons: ["Cancel", "Close Desktop only", "Turn off FlowPilot"],
        cancelId: 0,
        defaultId: 1,
        noLink: true,
      };
      const res = target
        ? await dialog.showMessageBox(target, opts)
        : await dialog.showMessageBox(opts);
      if (res.response === 2) return "turn_off";
      if (res.response === 1) return "close_only";
      return "cancel";
    },
    onStatus: (status) => {
      for (const w of BrowserWindow.getAllWindows()) {
        w.webContents.send("lifecycle:status", status);
      }
    },
  });

  ipcMain.handle("lifecycle:snapshot", async () => lifecycle.getSnapshot());
  ipcMain.handle("lifecycle:requestClose", async () => lifecycle.requestQuit("ipc"));
  ipcMain.handle("lifecycle:shutdown", async () => lifecycle.requestGlobalShutdown("ipc"));
  ipcMain.handle("lifecycle:restart", async () => lifecycle.requestPlannedRestart("ipc"));
  ipcMain.handle("lifecycle:state", async () => lifecycle.state());

  return {
    lifecycle,
    onBeforeQuit: (event) => {
      // Re-entry guard (T-2/constraint): while requestQuit is deciding — or
      // after it approved the quit — before-quit must not loop. The guard
      // stays set through the app.quit() re-fire, so the second pass returns
      // immediately without preventDefault.
      if (lifecycle.quitInFlight()) return;
      event.preventDefault();
      void lifecycle.requestQuit("app");
    },
  };
}
