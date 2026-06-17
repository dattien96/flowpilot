import { app, BrowserWindow, ipcMain, Notification, shell } from "electron";
import { execFile } from "node:child_process";
import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import path from "node:path";

// Electron shell (04-01). Loads the Vite dev server in dev, the built renderer in
// prod. The IdeBridge is the real Part B implementation: it detects an installed
// IDE CLI and opens the file at a line.

const VITE_DEV_SERVER_URL = process.env.VITE_DEV_SERVER_URL;

// Required on Windows for native notifications to appear in the Action Center.
// In dev mode the packaged app ID is not registered, so we use the executable
// path as the AUMID — Windows can always resolve it and will show the banner.
// In production the build sets its own AUMID via electron-builder.
if (process.platform === "win32") {
  app.setAppUserModelId("com.flowpilot.desktop");
}

// openInIde (04-06 / 04-01 Part B): invoke the first available IDE CLI to open a
// file at a line. VS Code `code -g file:line`, Android Studio `studio file:line`,
// Xcode `xed -l line file`. On Windows the launchers are `.cmd` shims, so we try a
// small candidate list per IDE and fall back across them. execFile (not a shell)
// avoids quoting/injection issues with the file path.
type IdeCandidate = { bin: string; args: (file: string, line?: number) => string[] };
type PersistedAuthSession = {
  clientKey: string;
  accessToken: string;
  refreshToken: string;
  userId: string;
  email?: string | null;
};
type BridgeHttpRequest = {
  url: string;
  method?: string;
  headers?: Record<string, string>;
  body?: string;
};

const IDE_CANDIDATES: IdeCandidate[] = [
  { bin: "code", args: (f, l) => ["-g", l ? `${f}:${l}` : f] },
  { bin: "code.cmd", args: (f, l) => ["-g", l ? `${f}:${l}` : f] },
  { bin: "cursor", args: (f, l) => ["-g", l ? `${f}:${l}` : f] },
  { bin: "studio", args: (f, l) => (l ? [`${f}:${l}`] : [f]) },
  { bin: "studio.sh", args: (f, l) => (l ? [`${f}:${l}`] : [f]) },
  { bin: "xed", args: (f, l) => (l ? ["-l", String(l), f] : [f]) },
];

function authSessionFilePath(): string {
  return path.join(app.getPath("userData"), "supabase-auth-session.json");
}

async function loadPersistedAuthSession(): Promise<PersistedAuthSession | null> {
  try {
    const raw = await readFile(authSessionFilePath(), "utf8");
    return JSON.parse(raw) as PersistedAuthSession;
  } catch (error) {
    if (
      typeof error === "object" &&
      error !== null &&
      "code" in error &&
      (error as { code?: string }).code === "ENOENT"
    ) {
      return null;
    }
    throw error;
  }
}

async function savePersistedAuthSession(payload: PersistedAuthSession): Promise<void> {
  await mkdir(path.dirname(authSessionFilePath()), { recursive: true });
  await writeFile(authSessionFilePath(), JSON.stringify(payload), "utf8");
}

async function clearPersistedAuthSession(): Promise<void> {
  await rm(authSessionFilePath(), { force: true });
}

// quoteWinArg wraps an argument for cmd.exe. When we spawn through a shell
// (required for `.cmd` launchers on Windows, see tryOpen) Node no longer escapes
// arguments, so we double-quote each one and escape embedded double quotes.
function quoteWinArg(arg: string): string {
  return `"${arg.replace(/"/g, '\\"')}"`;
}

function tryOpen(candidates: IdeCandidate[], file: string, line: number | undefined): Promise<boolean> {
  if (candidates.length === 0) return Promise.resolve(false);
  const [head, ...rest] = candidates;
  // On Windows the IDE launchers (`code`, `cursor`, `studio`) are `.cmd` shims.
  // Since Node 18.20 / 20.12 (CVE-2024-27980) a `.cmd`/`.bat` can only be spawned
  // through a shell — spawning it directly throws `spawn EINVAL`. So on win32 we
  // set shell:true (which also lets bare `code` resolve via PATHEXT) and quote the
  // arguments ourselves because shell:true disables Node's automatic escaping.
  const onWindows = process.platform === "win32";
  return new Promise((resolve) => {
    const next = (): void => void tryOpen(rest, file, line).then(resolve);
    try {
      const rawArgs = head.args(file, line);
      const args = onWindows ? rawArgs.map(quoteWinArg) : rawArgs;
      execFile(head.bin, args, { shell: onWindows, windowsHide: true }, (err) => {
        if (!err) {
          resolve(true);
          return;
        }
        // ENOENT (not installed) or launch failure → try the next candidate.
        next();
      });
    } catch {
      // Some Node versions throw synchronously (e.g. EINVAL) instead of via the
      // callback — keep falling through candidates rather than rejecting.
      next();
    }
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

// Real IDE open (04-01 Part B): try each known IDE CLI until one launches, then
// fall back to the OS default handler. The IDE CLIs give line targeting; the
// fallback guarantees the file still opens when no CLI is on PATH — common on
// macOS, where VS Code's `code`/`cursor` shims are an opt-in install step.
ipcMain.handle("ide:open", async (_event, payload: { file: string; line?: number }) => {
  const opened = await tryOpen(IDE_CANDIDATES, payload.file, payload.line);
  if (opened) return { ok: true };

  // Fallback: open in the OS default app (no line targeting). shell.openPath
  // returns "" on success or an error string; works on win32/darwin/linux.
  const err = await shell.openPath(payload.file);
  if (err) {
    console.warn("[IdeBridge] no IDE CLI found and openPath failed:", err, payload.file);
    return { ok: false };
  }
  return { ok: true };
});

// Open a URL in the default browser (used by the "Open Admin Web" button).
ipcMain.handle("shell:openExternal", (_event, payload: { url: string }) => {
  void shell.openExternal(payload.url);
  return { ok: true };
});

ipcMain.handle("auth-session:load", async () => loadPersistedAuthSession());
ipcMain.handle("auth-session:save", async (_event, payload: PersistedAuthSession) => {
  await savePersistedAuthSession(payload);
  return { ok: true };
});
ipcMain.handle("auth-session:clear", async () => {
  await clearPersistedAuthSession();
  return { ok: true };
});
ipcMain.handle("notification:show", (_event, payload: { title: string; body: string }) => {
  console.log("[notification] isSupported:", Notification.isSupported(), "payload:", payload);
  try {
    new Notification({ title: payload.title, body: payload.body }).show();
  } catch (err) {
    console.error("[notification] show failed:", err);
  }
  return { ok: true };
});

ipcMain.handle("http:request", async (_event, payload: BridgeHttpRequest) => {
  const response = await fetch(payload.url, {
    method: payload.method ?? "GET",
    headers: payload.headers,
    body: payload.body,
  });
  return {
    status: response.status,
    headers: Array.from(response.headers.entries()),
    body: await response.text(),
  };
});

void app.whenReady().then(createWindow);

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") app.quit();
});

app.on("activate", () => {
  if (BrowserWindow.getAllWindows().length === 0) createWindow();
});
