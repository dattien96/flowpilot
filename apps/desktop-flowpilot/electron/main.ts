import { app, BrowserWindow, ipcMain, shell } from "electron";
import path from "node:path";

// Phase 1 (04-01 Part A): minimal Electron shell. Loads the Vite dev server in
// dev, the built renderer in prod. The only native bridge is a STUB IdeBridge
// that logs — real IDE CLI invocation (code -g / studio / xed) is Part B.

const VITE_DEV_SERVER_URL = process.env.VITE_DEV_SERVER_URL;

function createWindow(): void {
  const win = new BrowserWindow({
    width: 1320,
    height: 880,
    minWidth: 960,
    minHeight: 640,
    backgroundColor: "#0e1117",
    title: "FlowPilot Desktop (mock)",
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

// Part A stub: log the request only. Part B replaces this with real IDE CLI
// detection + invocation (VS Code `code -g file:line`, Android Studio, Xcode).
ipcMain.handle("ide:open", (_event, payload: { file: string; line?: number }) => {
  console.log("[IdeBridge stub] open in IDE:", payload);
  return { ok: true, stub: true };
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
