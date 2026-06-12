import { app, BrowserWindow, ipcMain, shell } from "electron";
import { execFile } from "node:child_process";
import path from "node:path";

// Electron shell (04-01). Loads the Vite dev server in dev, the built renderer in
// prod. The IdeBridge is the real Part B implementation: it detects an installed
// IDE CLI and opens the file at a line.

const VITE_DEV_SERVER_URL = process.env.VITE_DEV_SERVER_URL;

// openInIde (04-06 / 04-01 Part B): invoke the first available IDE CLI to open a
// file at a line. VS Code `code -g file:line`, Android Studio `studio file:line`,
// Xcode `xed -l line file`. On Windows the launchers are `.cmd` shims, so we try a
// small candidate list per IDE and fall back across them. execFile (not a shell)
// avoids quoting/injection issues with the file path.
type IdeCandidate = { bin: string; args: (file: string, line?: number) => string[] };

const IDE_CANDIDATES: IdeCandidate[] = [
  { bin: "code", args: (f, l) => ["-g", l ? `${f}:${l}` : f] },
  { bin: "code.cmd", args: (f, l) => ["-g", l ? `${f}:${l}` : f] },
  { bin: "cursor", args: (f, l) => ["-g", l ? `${f}:${l}` : f] },
  { bin: "studio", args: (f, l) => (l ? [`${f}:${l}`] : [f]) },
  { bin: "studio.sh", args: (f, l) => (l ? [`${f}:${l}`] : [f]) },
  { bin: "xed", args: (f, l) => (l ? ["-l", String(l), f] : [f]) },
];

function tryOpen(candidates: IdeCandidate[], file: string, line: number | undefined): Promise<boolean> {
  if (candidates.length === 0) return Promise.resolve(false);
  const [head, ...rest] = candidates;
  return new Promise((resolve) => {
    execFile(head.bin, head.args(file, line), (err) => {
      if (!err) {
        resolve(true);
        return;
      }
      // ENOENT (not installed) or launch failure → try the next candidate.
      void tryOpen(rest, file, line).then(resolve);
    });
  });
}

function createWindow(): void {
  const win = new BrowserWindow({
    width: 1320,
    height: 880,
    minWidth: 960,
    minHeight: 640,
    backgroundColor: "#0e1117",
    title: "FlowPilot Desktop",
    webPreferences: {
      preload: path.join(__dirname, "preload.js"),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  if (VITE_DEV_SERVER_URL) {
    void win.loadURL(VITE_DEV_SERVER_URL);
  } else {
    void win.loadFile(path.join(__dirname, "../dist/index.html"));
  }
}

// Real IDE open (04-01 Part B): try each known IDE CLI until one launches.
ipcMain.handle("ide:open", async (_event, payload: { file: string; line?: number }) => {
  const opened = await tryOpen(IDE_CANDIDATES, payload.file, payload.line);
  if (!opened) {
    console.warn("[IdeBridge] no IDE CLI found (tried code/cursor/studio/xed):", payload.file);
  }
  return { ok: opened };
});

// Open a URL in the default browser (used by the "Open Admin Web" button).
ipcMain.handle("shell:openExternal", (_event, payload: { url: string }) => {
  void shell.openExternal(payload.url);
  return { ok: true };
});

void app.whenReady().then(createWindow);

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") app.quit();
});

app.on("activate", () => {
  if (BrowserWindow.getAllWindows().length === 0) createWindow();
});
