const { spawn, execSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const net = require('net');
const http = require('http');
const os = require('os');

const defaultRootDir = path.resolve(__dirname, '..');
let rootDir = defaultRootDir;

function parseDotenvValue(rawValue) {
  let value = rawValue.trim();
  const quote = value[0];
  if ((quote === '"' || quote === "'") && value[value.length - 1] === quote) {
    value = value.slice(1, -1);
    if (quote === '"') {
      value = value.replace(/\\n/g, '\n').replace(/\\r/g, '\r').replace(/\\t/g, '\t');
    }
    return value;
  }
  return value.replace(/\s+#.*$/, '').trim();
}

function loadEnvFile(envFile) {
  if (!envFile) return;
  const envPath = path.resolve(rootDir, envFile);
  if (!fs.existsSync(envPath)) {
    console.warn(`[Supervisor] Env file ${path.relative(rootDir, envPath)} not found; using shell env/defaults.`);
    return;
  }
  const content = fs.readFileSync(envPath, 'utf8');
  for (const line of content.split(/\r?\n/)) {
    const match = line.match(/^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)?\s*$/);
    if (!match) continue;
    const [, key, rawValue = ''] = match;
    process.env[key] = parseDotenvValue(rawValue);
  }
}

// Parse arguments
let webPort = '';
let runnerPort = '';
let desktopPort = '';
let restartExisting = false;
let withDesktop = false;
let withWeb = true;
let envFile = '';
let rootDirArg = '';
let desktopPath = 'apps/desktop-flowpilot';
const args = process.argv.slice(2);
const webPortProvidedByArgs = args.includes('--web-port');
for (let i = 0; i < args.length; i++) {
  if (args[i] === '--root-dir' && args[i + 1]) {
    rootDirArg = args[i + 1];
    i++;
  } else if (args[i] === '--env-file' && args[i + 1]) {
    envFile = args[i + 1];
    i++;
  } else if (args[i] === '--web-port' && args[i + 1]) {
    webPort = args[i + 1];
    i++;
  } else if (args[i] === '--runner-port' && args[i + 1]) {
    runnerPort = args[i + 1];
    i++;
  } else if (args[i] === '--desktop-port' && args[i + 1]) {
    desktopPort = args[i + 1];
    i++;
  } else if (args[i] === '--restart-existing') {
    restartExisting = true;
  } else if (args[i] === '--with-desktop') {
    withDesktop = true;
  } else if (args[i] === '--without-web') {
    withWeb = false;
  } else if (args[i] === '--desktop-path' && args[i + 1]) {
    desktopPath = args[i + 1];
    i++;
  }
}

if (rootDirArg) {
  rootDir = path.resolve(rootDirArg);
}

loadEnvFile(envFile);
const webPortProvidedByEnv = Boolean(process.env.FLOWPILOT_ADMIN_WEB_PORT);
const webPortWasExplicit = webPortProvidedByArgs || webPortProvidedByEnv;
webPort = webPort || process.env.FLOWPILOT_ADMIN_WEB_PORT || '3002';
runnerPort = runnerPort || process.env.FLOWPILOT_RUNNER_PORT || '4317';
desktopPort = desktopPort || process.env.FLOWPILOT_DESKTOP_PORT || '';

const flowpilotDir = path.join(rootDir, '.flowpilot');
const metadataPath = path.join(flowpilotDir, 'supervisor.json');
// `let` so tests can relocate the control file without touching the real
// workspace (Task-419 _internals seam).
let controlPath = path.join(flowpilotDir, 'supervisor.cmd');
const runnerUrl = process.env.FLOWPILOT_RUNNER_URL || `http://127.0.0.1:${runnerPort}`;
const googleDriveRedirectUri =
  process.env.GOOGLE_DRIVE_REDIRECT_URI ||
  `${runnerUrl.replace(/\/+$/, '')}/artifact-storage/google-drive/oauth/callback`;
const goCacheDir = process.env.GOCACHE || path.join(os.tmpdir(), 'flowpilot-go-cache');
const librePort = process.env.FLOWPILOT_LIBRETRANSLATE_PORT || '5001';

let webProcess = null;
let runnerProcess = null;
let desktopProcess = null;
let libreProcess = null;
let hintDesktopPid = null;
let isExiting = false;
let isRestarting = false;
// CP-81 Task-419: set when a valid fenced restart-runner command has been
// consumed — the runner self-exits after its drain, so its exit is EXPECTED
// and must respawn only the runner, not the stack.
let plannedRunnerRestart = false;
// Cached runnerInstanceId learned from /health while the runner was alive —
// used to fence stale supervisor.cmd records after the writer has exited.
let currentRunnerInstanceId = null;

function ensureDirectoryExists(dir) {
  if (!fs.existsSync(dir)) {
    fs.mkdirSync(dir, { recursive: true });
  }
}

function isPortInUse(port) {
  return new Promise((resolve) => {
    if (getPidUsingPort(port)) {
      resolve(true);
      return;
    }
    const server = net.createServer();
    server.once('error', (err) => {
      if (err.code === 'EADDRINUSE') {
        resolve(true);
      } else {
        resolve(false);
      }
    });
    server.once('listening', () => {
      server.close();
      resolve(false);
    });
    server.listen(port, '127.0.0.1');
  });
}

function isPidAlive(pid) {
  if (!pid) return false;
  try {
    process.kill(pid, 0);
    return true;
  } catch (e) {
    return false;
  }
}

function getPidUsingPort(port) {
  try {
    if (process.platform === 'win32') {
      const output = execSync('netstat -ano', { encoding: 'utf8' });
      const lines = output.split('\n');
      for (const line of lines) {
        if (line.includes(`:${port}`) && line.includes('LISTENING')) {
          const parts = line.trim().split(/\s+/);
          const pid = parseInt(parts[parts.length - 1], 10);
          if (pid && pid > 0) return pid;
        }
      }
    } else {
      try {
        const output = execSync(`lsof -t -i tcp:${port}`, {
          encoding: 'utf8',
          stdio: ['ignore', 'pipe', 'ignore'],
        }).trim();
        const pid = parseInt(output, 10);
        if (pid && pid > 0) return pid;
      } catch (e) {
        // Fallback for Linux using ss or netstat if lsof is missing
        try {
          const output = execSync(`ss -lptn 'sport = :${port}'`, {
            encoding: 'utf8',
            stdio: ['ignore', 'pipe', 'ignore'],
          });
          const match = output.match(/pid=(\d+)/);
          if (match && match[1]) return parseInt(match[1], 10);
        } catch (err) {}
      }
    }
  } catch (e) {}
  return null;
}

function verifyProcessOwner(pid, type) {
  if (!pid) return false;
  try {
    let cmd = '';
    if (process.platform === 'win32') {
      cmd = execSync(`wmic process where processid=${pid} get commandline`, { encoding: 'utf8' }).toLowerCase();
    } else {
      try {
        cmd = execSync(`ps -p ${pid} -o command=`, { encoding: 'utf8' }).toLowerCase();
      } catch (pe) {
        try {
          cmd = fs.readFileSync(`/proc/${pid}/cmdline`, 'utf8').toLowerCase();
        } catch (fe) {}
      }
    }
    if (type === 'web') {
      const adminWebDir = path.join(rootDir, 'apps', 'admin-web').toLowerCase();
      return cmd.includes(adminWebDir) || cmd.includes('apps/admin-web') || cmd.includes('admin-web');
    } else if (type === 'runner') {
      return cmd.includes('flowpilot') || cmd.includes('go') || cmd.includes('runner') || cmd.includes('serve');
    }
  } catch (e) {}
  return false;
}

function createManagedProcessRef(pid, detached = false) {
  return {
    pid,
    detached,
    exitCode: null,
    killed: false,
  };
}

function signalManagedProcess(child, signal) {
  if (!child || !child.pid) return;
  try {
    if (process.platform !== 'win32' && child.detached) {
      process.kill(-child.pid, signal);
      return;
    }
    if (process.platform === 'win32') {
      // BUG-240: on Windows, `go run` compiles to a temp binary and launches
      // it as a genuinely separate child process (no POSIX exec()-replace
      // semantics), so `child.pid` here is only the go.exe/cmd.exe wrapper —
      // never the actual compiled server process running underneath it.
      // process.kill(pid, signal) targets that single PID only and does not
      // propagate to the descendant, and Node's Windows SIGINT emulation has
      // no real graceful-shutdown semantics to preserve anyway (it just
      // terminates the targeted process). Go straight to a tree-kill so the
      // wrapper and its compiled child both die together, instead of
      // signaling only the wrapper and relying on a liveness poll (see
      // cleanupAndExit) that can read the wrapper as "dead" while the real
      // server binary is still orphaned and running.
      killProcessTree(child);
      return;
    }
    process.kill(child.pid, signal);
  } catch (e) {}
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function getAdminWebUrl() {
  return process.env.VITE_ADMIN_WEB_URL || `http://localhost:${webPort}`;
}

async function findAvailablePort(startPort, host = '127.0.0.1') {
  let candidate = startPort;
  while (await isPortInUse(candidate, host)) {
    candidate += 1;
  }
  return candidate;
}

function isLibreTranslateInstalled() {
  return resolveLibreTranslateCommand() !== null;
}

function resolveLibreTranslateCommand() {
  try {
    const cmd = process.platform === 'win32' ? 'where libretranslate' : 'which libretranslate';
    execSync(cmd, { stdio: 'ignore' });
    return { command: 'libretranslate', args: [] };
  } catch (e) {}

  const homeCandidates = [];
  if (process.env.HOME) {
    homeCandidates.push(process.env.HOME);
  }
  if (process.env.USERPROFILE && !homeCandidates.includes(process.env.USERPROFILE)) {
    homeCandidates.push(process.env.USERPROFILE);
  }
  if (process.env.USER) {
    const userHome = path.join('/Users', process.env.USER);
    if (!homeCandidates.includes(userHome)) {
      homeCandidates.push(userHome);
    }
  }

  for (const home of homeCandidates) {
    const pythonRoot = path.join(home, 'Library', 'Python');
    try {
      const versionDirs = fs
        .readdirSync(pythonRoot, { withFileTypes: true })
        .filter((entry) => entry.isDirectory())
        .map((entry) => entry.name)
        .sort()
        .reverse();
      for (const version of versionDirs) {
        const candidate = path.join(pythonRoot, version, 'bin', 'libretranslate');
        if (fs.existsSync(candidate)) {
          return { command: candidate, args: [] };
        }
      }
    } catch (scanErr) {}

    const localBinCandidate = path.join(home, '.local', 'bin', 'libretranslate');
    if (fs.existsSync(localBinCandidate)) {
      return { command: localBinCandidate, args: [] };
    }
  }

  for (const pair of [
    { pip: 'pip', python: 'python' },
    { pip: 'pip3', python: 'python3' },
  ]) {
    try {
      const output = execSync(`${pair.pip} show libretranslate`, {
        encoding: 'utf8',
        stdio: ['ignore', 'pipe', 'ignore'],
      });
      if (output.includes('Name: libretranslate')) {
        return { command: pair.python, args: ['-m', 'libretranslate'] };
      }
    } catch (pipErr) {}
  }

  return null;
}

async function stopManagedProcess(child, label) {
  if (!child || !child.pid) return;

  console.log(`[Supervisor] Stopping existing ${label} process ${child.pid}...`);
  signalManagedProcess(child, 'SIGINT');

  const deadline = Date.now() + 3000;
  while (Date.now() < deadline) {
    if (!isPidAlive(child.pid)) {
      return;
    }
    await sleep(150);
  }

  console.log(`[Supervisor] Force-killing existing ${label} process ${child.pid}...`);
  killProcessTree(child);
  await sleep(300);
}

async function startServices() {
  ensureDirectoryExists(flowpilotDir);

  // Read old metadata as a hint
  let hintWebPid = null;
  let hintRunnerPid = null;
  if (fs.existsSync(metadataPath)) {
    try {
      const oldMeta = JSON.parse(fs.readFileSync(metadataPath, 'utf8'));
      if (oldMeta.webPid) hintWebPid = oldMeta.webPid;
      if (oldMeta.runnerPid) hintRunnerPid = oldMeta.runnerPid;
      if (oldMeta.desktopPid) hintDesktopPid = oldMeta.desktopPid;
    } catch (e) {}
  }

  const webInUse = withWeb ? await isPortInUse(parseInt(webPort, 10)) : false;
  const runnerInUse = await isPortInUse(parseInt(runnerPort, 10));

  // Resolve actual PIDs if in use and verify their ownership to prevent PID reuse issues
  let adoptedWebPid = null;
  if (withWeb && webInUse) {
    if (hintWebPid && isPidAlive(hintWebPid) && verifyProcessOwner(hintWebPid, 'web')) {
      adoptedWebPid = hintWebPid;
    } else {
      const portPid = getPidUsingPort(parseInt(webPort, 10));
      if (portPid && verifyProcessOwner(portPid, 'web')) {
        adoptedWebPid = portPid;
      }
    }
  }

  let adoptedRunnerPid = null;
  if (runnerInUse) {
    if (hintRunnerPid && isPidAlive(hintRunnerPid) && verifyProcessOwner(hintRunnerPid, 'runner')) {
      adoptedRunnerPid = hintRunnerPid;
    } else {
      const portPid = getPidUsingPort(parseInt(runnerPort, 10));
      if (portPid && verifyProcessOwner(portPid, 'runner')) {
        adoptedRunnerPid = portPid;
      }
    }
  }

  if (withWeb && webInUse && !adoptedWebPid) {
    if (webPortWasExplicit) {
      throw new Error(
        `Port ${webPort} is already in use by a process that does not look like the FlowPilot web app. Stop that process or choose another port.`,
      );
    }

    const fallbackWebPort = await findAvailablePort(parseInt(webPort, 10) + 1);
    console.log(
      `[Supervisor] Admin web port ${webPort} is busy. Falling back to ${fallbackWebPort}.`,
    );
    webPort = String(fallbackWebPort);
    return startServices();
  }

  if (runnerInUse && !adoptedRunnerPid) {
    throw new Error(
      `Port ${runnerPort} is already in use by a process that does not look like the FlowPilot local runner. Stop that process or choose another port.`,
    );
  }

  if (restartExisting && ((withWeb && adoptedWebPid) || adoptedRunnerPid)) {
    console.log('[Supervisor] Restarting existing owned dev processes so env and code changes take effect...');
    await stopManagedProcess(adoptedRunnerPid ? createManagedProcessRef(adoptedRunnerPid, false) : null, 'runner');
    if (withWeb) {
      await stopManagedProcess(adoptedWebPid ? createManagedProcessRef(adoptedWebPid, false) : null, 'web');
    }
    return startServicesFresh();
  }

  const allManagedServicesRunning = withWeb ? webInUse && runnerInUse : runnerInUse;
  if (allManagedServicesRunning) {
    console.log(
      withWeb
        ? '[Supervisor] Both services are already running. Watching for control commands...'
        : '[Supervisor] Runner service is already running. Watching for control commands...',
    );

    if (withWeb && adoptedWebPid) {
      webProcess = createManagedProcessRef(adoptedWebPid, false);
    }
    if (adoptedRunnerPid) {
      runnerProcess = createManagedProcessRef(adoptedRunnerPid, false);
      // CP-81: learn the adopted runner's instance ID so fenced commands are
      // validated against this generation, and clear any command file left by
      // a dead generation (T-6 startup hygiene).
      void refreshRunnerInstanceId();
    }
    clearStaleSupervisorCommand();

    const metadata = {
      supervisorPid: process.pid,
      webPid: adoptedWebPid,
      runnerPid: adoptedRunnerPid,
      desktopPid: hintDesktopPid,
      controlPath: controlPath,
    };
    fs.writeFileSync(metadataPath, JSON.stringify(metadata, null, 2));
    return;
  }

  return startServicesFresh({ webInUse, runnerInUse, adoptedWebPid, adoptedRunnerPid });
}

function attachExitHandlers(child, label) {
  child.on('exit', (code) => {
    if (!isExiting && !isRestarting) {
      console.log(`[Supervisor] ${label} process exited with code ${code}. Exiting...`);
      cleanupAndExit();
    }
  });
}

// ── CP-81 Task-419: lifecycle-aware runner ownership ─────────────────────────
// The supervisor is a controller, not a user client: it never registers a
// lease and never blocks idle shutdown. It launches the runner in supervised
// mode, honors only fenced commands matching the live runnerInstanceId, and
// distinguishes the runner's planned self-exit (drain → fenced restart cmd)
// from unexpected death (no ghost respawn — the stack exits, clients see the
// unplanned-loss path).

// startRunnerProcess spawns the runner child in supervised lifecycle mode.
// Extracted so planned restarts can respawn ONLY the runner while web/desktop
// stay up and their clients reconnect to the new generation.
function startRunnerProcess() {
  const runnerCmd = process.platform === 'win32' ? 'go.exe' : 'go';
  const hasCodexAppServerFlag = Object.prototype.hasOwnProperty.call(
    process.env,
    'FLOWPILOT_CODEX_APPSERVER',
  );
  const runnerEnv = {
    ...process.env,
    GOCACHE: goCacheDir,
    FLOWPILOT_RUNNER_PORT: runnerPort,
    FLOWPILOT_RUNNER_URL: runnerUrl,
    GOOGLE_DRIVE_REDIRECT_URI: googleDriveRedirectUri,
    FLOWPILOT_CODEX_APPSERVER: hasCodexAppServerFlag
      ? process.env.FLOWPILOT_CODEX_APPSERVER
      : '1',
    // CP-81: supervised mode — the runner exposes the lifecycle API but never
    // idle-exits; shutdown/restart authority is coordinated via fenced
    // supervisor.cmd records. Both spellings are set: the runner reads
    // FLOWPILOT_LIFECYCLE_MODE (root.go resolveLifecycleMode); the *_RUNNER_*
    // name documents the supervisor contract.
    FLOWPILOT_LIFECYCLE_MODE: 'supervised',
    FLOWPILOT_RUNNER_LIFECYCLE_MODE: 'supervised',
    SUPERVISOR_PID: String(process.pid),
    FLOWPILOT_SUPERVISOR_PID: String(process.pid),
  };
  runnerProcess = spawn(runnerCmd, ['run', './cmd/flowpilot', 'runner', 'serve', '--port', runnerPort], {
    cwd: path.join(rootDir, 'apps', 'local-runner'),
    shell: true,
    stdio: 'inherit',
    detached: process.platform !== 'win32',
    env: runnerEnv,
  });
  runnerProcess.detached = process.platform !== 'win32';
  currentRunnerInstanceId = null;
  attachRunnerExitHandler(runnerProcess);
}

// attachRunnerExitHandler splits runner exit into planned (a fenced restart
// command was consumed → respawn only the runner) and unexpected death.
function attachRunnerExitHandler(child) {
  child.on('exit', (code, signal) => {
    if (isExiting) return;
    if (plannedRunnerRestart || isRestarting) {
      // Expected during a planned restart — respawn happens in
      // handlePlannedRunnerRestart, not here.
      return;
    }
    handleRunnerExitUnexpected(code, signal);
  });
}

// handleRunnerExitUnexpected is the frozen no-ghost-restart policy (T-5): log
// the death and tear the stack down — never silently spawn a hidden new
// generation behind clients that still believe the old runner is alive.
function handleRunnerExitUnexpected(code, signal) {
  console.log(
    `[Supervisor] Runner exited unexpectedly (code=${code} signal=${signal} pid=${runnerProcess && runnerProcess.pid}). ` +
      'No silent respawn — shutting down stack so clients see the loss.',
  );
  cleanupAndExit();
}

// fetchRunnerInstanceId asks the live runner for its instance identity.
// Returns null when the runner is down (normal mid-drain).
function fetchRunnerInstanceId() {
  return new Promise((resolve) => {
    const req = http.request(
      {
        hostname: '127.0.0.1',
        port: parseInt(runnerPort, 10),
        path: '/health',
        method: 'GET',
        timeout: 2000,
      },
      (res) => {
        let raw = '';
        res.on('data', (chunk) => (raw += chunk));
        res.on('end', () => {
          try {
            const body = JSON.parse(raw);
            resolve(typeof body.runnerInstanceId === 'string' ? body.runnerInstanceId : null);
          } catch (e) {
            resolve(null);
          }
        });
      },
    );
    req.on('error', () => resolve(null));
    req.on('timeout', () => {
      req.destroy();
      resolve(null);
    });
    req.end();
  });
}

// refreshRunnerInstanceId learns the current generation's instance ID for
// command fencing; called after spawn once /health is reachable.
async function refreshRunnerInstanceId() {
  const id = await fetchRunnerInstanceId();
  if (id) currentRunnerInstanceId = id;
  return currentRunnerInstanceId;
}

// readSupervisorCommand parses the control file. Two shapes are accepted:
//   - legacy plain text ("shutdown"|"restart") from unmanaged/legacy runners
//   - fenced JSON {action, runnerInstanceId, restartId, requestedAt,
//     expiresAt, requester} written by the CP-81 drain path
function readSupervisorCommand() {
  if (!fs.existsSync(controlPath)) return null;
  let raw;
  try {
    raw = fs.readFileSync(controlPath, 'utf8').trim();
  } catch (e) {
    return null; // transient read lock — retry next tick
  }
  if (!raw) return null;
  if (raw.startsWith('{')) {
    try {
      const cmd = JSON.parse(raw);
      if (typeof cmd === 'object' && cmd !== null && typeof cmd.action === 'string') {
        return cmd;
      }
    } catch (e) {
      return { action: '__invalid__' };
    }
    return null;
  }
  return { action: raw.toLowerCase(), legacy: true };
}

// validateSupervisorCommand fences a parsed record against the live (or last
// known) runner instance: stale/expired/unknown commands are never honored.
function validateSupervisorCommand(cmd, liveInstanceId) {
  if (!cmd || typeof cmd.action !== 'string') {
    return { valid: false, reason: 'empty' };
  }
  if (cmd.legacy) {
    // Pre-lifecycle writers carry no fence — honored under the old contract.
    return { valid: cmd.action === 'shutdown' || cmd.action === 'restart', reason: 'legacy' };
  }
  if (cmd.action !== 'shutdown' && cmd.action !== 'restart' && cmd.action !== 'restart-runner') {
    return { valid: false, reason: 'unknown_action' };
  }
  if (cmd.expiresAt && Date.parse(cmd.expiresAt) <= Date.now()) {
    return { valid: false, reason: 'expired' };
  }
  // Fenced records must name the generation that wrote them.
  if (!cmd.runnerInstanceId) {
    return { valid: false, reason: 'missing_instance' };
  }
  // Instance fence: the live /health value wins; the cached value covers the
  // mid-drain window where the writer has already exited.
  const expected = liveInstanceId || currentRunnerInstanceId;
  if (!expected) {
    // Cannot verify the fence at all — fail closed rather than let an old
    // record act on an unverified generation.
    return { valid: false, reason: 'unverifiable_instance' };
  }
  if (cmd.runnerInstanceId !== expected) {
    return { valid: false, reason: 'stale_instance' };
  }
  return { valid: true };
}

// clearStaleSupervisorCommand removes control files that failed validation
// (stale generation, expired, malformed) so they cannot fire later (T-6).
function clearStaleSupervisorCommand() {
  try {
    if (fs.existsSync(controlPath)) fs.unlinkSync(controlPath);
  } catch (e) {}
}

// handlePlannedRunnerRestart performs T-3: the runner already drained and is
// self-exiting; wait for the old process to die, then respawn ONLY the runner
// — web/desktop processes stay up and their clients reconnect to the new
// runner generation.
async function handlePlannedRunnerRestart(cmd) {
  plannedRunnerRestart = true;
  console.log(
    `[Supervisor] Planned runner restart (restartId=${cmd.restartId || 'n/a'} requester=${cmd.requester || 'unknown'}). ` +
      'Waiting for the old runner to exit, then respawning the runner only.',
  );
  const deadline = Date.now() + 15000;
  while (runnerProcess && runnerProcess.pid && isPidAlive(runnerProcess.pid) && Date.now() < deadline) {
    await sleep(200);
  }
  if (runnerProcess && runnerProcess.pid && isPidAlive(runnerProcess.pid)) {
    console.log('[Supervisor] Old runner did not exit in 15s — killing its tree before respawn.');
    killProcessTree(runnerProcess);
    await sleep(300);
  }
  startRunnerProcess();
  plannedRunnerRestart = false;
  void refreshRunnerInstanceId();
}

// shutdownRunnerTree is the explicit-stack-shutdown path (BUG-240 preserved):
// on Windows it tree-kills the go.exe wrapper AND the compiled runner child.
function shutdownRunnerTree(child) {
  killProcessTree(child);
}

// pollSupervisorCommand replaces the old raw-text watcher: parse, fence-check
// against the runner instance, consume valid commands, clear stale ones.
async function pollSupervisorCommand() {
  if (isExiting || plannedRunnerRestart) return;
  const cmd = readSupervisorCommand();
  if (!cmd) return;
  const liveInstanceId = await fetchRunnerInstanceId();
  const verdict = validateSupervisorCommand(cmd, liveInstanceId);
  if (!verdict.valid) {
    console.log(`[Supervisor] Ignoring control command (${verdict.reason}): ${JSON.stringify(cmd)}`);
    clearStaleSupervisorCommand();
    return;
  }
  // Consume before acting so a retried poll cannot replay it.
  clearStaleSupervisorCommand();
  if (cmd.action === 'shutdown') {
    console.log('[Supervisor] Shutdown command detected. Waiting 200ms for response flush...');
    setTimeout(() => {
      cleanupAndExit();
    }, 200);
  } else if (cmd.action === 'restart' && cmd.legacy) {
    console.log('[Supervisor] Legacy restart command detected — full stack restart.');
    setTimeout(() => {
      handleRestart();
    }, 200);
  } else if (cmd.action === 'restart' || cmd.action === 'restart-runner') {
    // Fenced runner restart (Task-419 T-3): runner-only respawn.
    setTimeout(() => {
      void handlePlannedRunnerRestart(cmd);
    }, 200);
  }
}

async function startServicesFresh(existing = {}) {
  const {
    webInUse = false,
    runnerInUse = false,
    adoptedWebPid = null,
    adoptedRunnerPid = null,
  } = existing;

  // Clear any stale command file
  if (fs.existsSync(controlPath)) {
    try {
      fs.unlinkSync(controlPath);
    } catch (e) {}
  }

  // Spawn web app if not running
  if (withWeb) {
    if (!webInUse) {
      console.log(`[Supervisor] Starting web service on port ${webPort}...`);
      const webCmd = process.platform === 'win32' ? 'npm.cmd' : 'npm';
      webProcess = spawn(webCmd, ['run', 'dev', '--', '--port', webPort], {
        cwd: path.join(rootDir, 'apps', 'admin-web'),
        shell: true,
        stdio: 'inherit',
        detached: process.platform !== 'win32',
        env: {
          ...process.env,
          FLOWPILOT_ADMIN_WEB_PORT: webPort,
          FLOWPILOT_RUNNER_PORT: runnerPort,
          FLOWPILOT_RUNNER_URL: runnerUrl,
          VITE_LOCAL_RUNNER_URL: process.env.VITE_LOCAL_RUNNER_URL || runnerUrl,
          VITE_ADMIN_WEB_URL: getAdminWebUrl(),
          GOOGLE_DRIVE_REDIRECT_URI: googleDriveRedirectUri,
        },
      });
      webProcess.detached = process.platform !== 'win32';

      attachExitHandlers(webProcess, 'Web');
    } else {
      console.log(`[Supervisor] Web service is already running on port ${webPort}, skipping start.`);
      if (adoptedWebPid) {
        webProcess = createManagedProcessRef(adoptedWebPid, false);
      }
    }
  }

  // Spawn runner if not running
  if (!runnerInUse) {
    console.log(`[Supervisor] Starting runner service on port ${runnerPort}...`);
    startRunnerProcess();
    // Learn the new generation's runnerInstanceId once /health comes up so
    // fenced commands validate against it (go run compiles — boot takes a
    // moment, so poll in the background).
    void (async () => {
      for (let i = 0; i < 60 && !isExiting; i++) {
        if (await refreshRunnerInstanceId()) return;
        await sleep(500);
      }
    })();
  } else {
    console.log(`[Supervisor] Runner service is already running on port ${runnerPort}, skipping start.`);
    if (adoptedRunnerPid) {
      runnerProcess = createManagedProcessRef(adoptedRunnerPid, false);
    }
  }

  // Spawn the desktop app (Electron + Vite). It has no fixed managed port, so we
  // kill any stale instance from a previous run by its recorded PID, then start
  // fresh. Closing the desktop window does NOT tear down web/runner.
  if (withDesktop) {
    if (hintDesktopPid && isPidAlive(hintDesktopPid)) {
      console.log(`[Supervisor] Stopping stale desktop process ${hintDesktopPid}...`);
      killProcessTree(createManagedProcessRef(hintDesktopPid, false));
    }
    // Point the desktop at the runner we just (re)started so it uses the real
    // HttpWsRunnerClient instead of the offline mock. An explicit VITE_RUNNER_URL in
    // the environment still wins (e.g. to target a remote runner).
    const desktopRunnerUrl = process.env.VITE_RUNNER_URL || runnerUrl;
    console.log(`[Supervisor] Starting desktop app (Electron + Vite) → runner ${desktopRunnerUrl}...`);
    const desktopCmd = process.platform === 'win32' ? 'npm.cmd' : 'npm';
    const desktopArgs = ['run', 'dev'];
    if (desktopPort) {
      desktopArgs.push('--', '--port', desktopPort, '--strictPort', '--host', '127.0.0.1');
    }
    const desktopEnv = { ...process.env };
    delete desktopEnv.ELECTRON_RUN_AS_NODE;
    desktopProcess = spawn(desktopCmd, desktopArgs, {
      cwd: path.join(rootDir, desktopPath),
      shell: true,
      stdio: 'inherit',
      detached: process.platform !== 'win32',
      env: {
        ...desktopEnv,
        FLOWPILOT_ADMIN_WEB_PORT: webPort,
        FLOWPILOT_RUNNER_PORT: runnerPort,
        FLOWPILOT_RUNNER_URL: runnerUrl,
        VITE_RUNNER_URL: desktopRunnerUrl,
        VITE_LOCAL_RUNNER_URL: process.env.VITE_LOCAL_RUNNER_URL || runnerUrl,
        VITE_ADMIN_WEB_URL: getAdminWebUrl(),
        GOOGLE_DRIVE_REDIRECT_URI: googleDriveRedirectUri,
      },
    });
    desktopProcess.detached = process.platform !== 'win32';
    attachDesktopExitHandler(desktopProcess);
  }

  // Spawn LibreTranslate if installed (optional — exit does not bring down the stack)
  if (isLibreTranslateInstalled()) {
    const libreInUse = await isPortInUse(librePort);
    if (libreInUse) {
      console.log(`[Supervisor] LibreTranslate already running on port ${librePort}, skipping start.`);
    } else {
      const libreCommand = resolveLibreTranslateCommand();
      if (!libreCommand) {
        console.log('[Supervisor] LibreTranslate package detected but no runnable command was resolved. Skipping translation service.');
      } else {
      console.log(`[Supervisor] Starting LibreTranslate on port ${librePort} (en + vi only)...`);
      libreProcess = spawn(
        libreCommand.command,
        [...libreCommand.args, '--load-only', 'en,vi', '--port', String(librePort)],
        {
          shell: true,
          stdio: 'inherit',
          detached: process.platform !== 'win32',
        },
      );
        libreProcess.detached = process.platform !== 'win32';
        libreProcess.on('exit', (code) => {
          if (!isExiting && !isRestarting) {
            console.log(`[Supervisor] LibreTranslate exited with code ${code}. Web + runner keep running.`);
          }
        });
      }
    }
  } else {
    console.log('[Supervisor] LibreTranslate not installed, skipping translation service. (Install via Engine Settings)');
  }

  // Write supervisor metadata
  const metadata = {
    supervisorPid: process.pid,
    webPid: webProcess ? webProcess.pid : null,
    runnerPid: runnerProcess ? runnerProcess.pid : null,
    desktopPid: desktopProcess ? desktopProcess.pid : null,
    librePid: libreProcess ? libreProcess.pid : null,
    controlPath: controlPath,
    webPort: parseInt(webPort, 10),
    runnerPort: parseInt(runnerPort, 10),
    desktopPort: desktopPort ? parseInt(desktopPort, 10) : null,
    runnerUrl,
    adminWebUrl: getAdminWebUrl(),
  };
  fs.writeFileSync(metadataPath, JSON.stringify(metadata, null, 2));
}

// The desktop window closing is a normal user action — log it but keep the
// rest of the stack running (unlike web/runner, which exit the supervisor).
function attachDesktopExitHandler(child) {
  child.on('exit', (code) => {
    if (!isExiting && !isRestarting) {
      console.log(`[Supervisor] Desktop app exited with code ${code}. Web + runner keep running.`);
    }
  });
}

function killProcessTree(child) {
  if (!child) return;
  const pid = child.pid;
  if (!pid) return;
  if (process.platform === 'win32') {
    const { execSync } = require('child_process');
    try {
      execSync(`taskkill /F /T /PID ${pid}`);
    } catch (e) {}
  } else {
    try {
      if (child.detached) {
        process.kill(-pid, 'SIGKILL');
      } else {
        process.kill(pid, 'SIGKILL');
      }
    } catch (e) {
      try {
        process.kill(pid, 'SIGKILL');
      } catch (err) {}
    }
  }
}

function triggerHTTPShutdown() {
  const req = http.request({
    hostname: '127.0.0.1',
    port: parseInt(runnerPort, 10),
    path: '/system/shutdown',
    method: 'POST'
  }, () => {});
  req.on('error', () => {});
  req.end();
}

function cleanupAndExit() {
  if (isExiting) return;
  isExiting = true;
  console.log('[Supervisor] Shutting down stack...');

  // Request runner to clean up sessions via HTTP
  triggerHTTPShutdown();

  // Signal the process groups on Unix to let them shut down gracefully
  signalManagedProcess(runnerProcess, 'SIGINT');
  signalManagedProcess(webProcess, 'SIGINT');
  signalManagedProcess(desktopProcess, 'SIGINT');
  signalManagedProcess(libreProcess, 'SIGINT');

  // Wait up to 3 seconds for processes to exit on their own before force-killing
  let checks = 0;
  const maxChecks = 15; // 15 * 200ms = 3000ms
  const interval = setInterval(() => {
    const runnerAlive = runnerProcess && runnerProcess.exitCode === null && !runnerProcess.killed && isPidAlive(runnerProcess.pid);
    const webAlive = webProcess && webProcess.exitCode === null && !webProcess.killed && isPidAlive(webProcess.pid);
    const desktopAlive = desktopProcess && desktopProcess.exitCode === null && !desktopProcess.killed && isPidAlive(desktopProcess.pid);
    const libreAlive = libreProcess && libreProcess.exitCode === null && !libreProcess.killed && isPidAlive(libreProcess.pid);

    if (!runnerAlive && !webAlive && !desktopAlive && !libreAlive) {
      clearInterval(interval);
      finishExit();
    } else {
      checks++;
      if (checks >= maxChecks) {
        console.log('[Supervisor] Grace period expired. Force-killing lingering processes...');
        clearInterval(interval);
        if (runnerAlive) killProcessTree(runnerProcess);
        if (webAlive) killProcessTree(webProcess);
        if (desktopAlive) killProcessTree(desktopProcess);
        if (libreAlive) killProcessTree(libreProcess);
        finishExit();
      }
    }
  }, 200);
}

// Test seam (Task-419): when noExit is set, finishExit records the exit
// instead of calling process.exit so tests can drive shutdown paths.
let noExit = false;
let exitedForTest = false;

function finishExit() {
  try {
    if (fs.existsSync(controlPath)) fs.unlinkSync(controlPath);
  } catch (e) {}
  try {
    if (fs.existsSync(metadataPath)) fs.unlinkSync(metadataPath);
  } catch (e) {}
  if (noExit) {
    exitedForTest = true;
    return;
  }
  process.exit(0);
}

function handleRestart() {
  if (isRestarting) return;
  isRestarting = true;
  console.log('[Supervisor] Restarting stack...');

  // Signal the process groups on Unix first
  signalManagedProcess(runnerProcess, 'SIGINT');
  signalManagedProcess(webProcess, 'SIGINT');
  signalManagedProcess(desktopProcess, 'SIGINT');
  signalManagedProcess(libreProcess, 'SIGINT');

  // Wait up to 2 seconds for processes to exit, then force-kill if still alive
  let checks = 0;
  const maxChecks = 10; // 10 * 200ms = 2000ms
  const interval = setInterval(() => {
    const runnerAlive = runnerProcess && runnerProcess.exitCode === null && !runnerProcess.killed && isPidAlive(runnerProcess.pid);
    const webAlive = webProcess && webProcess.exitCode === null && !webProcess.killed && isPidAlive(webProcess.pid);
    const desktopAlive = desktopProcess && desktopProcess.exitCode === null && !desktopProcess.killed && isPidAlive(desktopProcess.pid);
    const libreAlive = libreProcess && libreProcess.exitCode === null && !libreProcess.killed && isPidAlive(libreProcess.pid);

    if (!runnerAlive && !webAlive && !desktopAlive && !libreAlive) {
      clearInterval(interval);
      isRestarting = false;
      startServices();
    } else {
      checks++;
      if (checks >= maxChecks) {
        console.log('[Supervisor] Force-killing lingering processes for restart...');
        clearInterval(interval);
        if (runnerAlive) killProcessTree(runnerProcess);
        if (webAlive) killProcessTree(webProcess);
        if (desktopAlive) killProcessTree(desktopProcess);
        if (libreAlive) killProcessTree(libreProcess);
        setTimeout(() => {
          isRestarting = false;
          startServices();
        }, 500);
      }
    }
  }, 200);
}

// Watch control file — fenced commands only (Task-419 T-2): the poll parses,
// validates against the live/cached runnerInstanceId + expiry, consumes valid
// commands once, and removes stale ones so an old generation's record can
// never fire against a new runner.
const commandWatchInterval = setInterval(() => {
  void pollSupervisorCommand();
}, 500);

// Capture signals
function installSignalHandlers() {
  process.on('SIGINT', cleanupAndExit);
  process.on('SIGTERM', cleanupAndExit);
  // Terminal window closed (SIGHUP) must also tear down: managed children are
  // spawned detached (own process groups), so without this they outlive the
  // supervisor as unkillable-by-Ctrl+C orphans that keep holding ports and
  // dispatch.lock files.
  process.on('SIGHUP', cleanupAndExit);
}

// ── module seam (Task-419 tests) ────────────────────────────────────────────
// Required as a library in tests: no auto-start, no signal handlers, and the
// command watcher stays silent until started.
if (require.main === module) {
  installSignalHandlers();
  // T-6 startup hygiene: a control file left by a dead generation is stale by
  // definition — clear it before the first poll can act on it.
  clearStaleSupervisorCommand();
  startServices();
} else {
  clearInterval(commandWatchInterval);
}

module.exports = {
  startRunnerProcess,
  readSupervisorCommand,
  validateSupervisorCommand,
  handlePlannedRunnerRestart,
  handleRunnerExitUnexpected,
  shutdownRunnerTree,
  clearStaleSupervisorCommand,
  pollSupervisorCommand,
  fetchRunnerInstanceId,
  cleanupAndExit,
  // Test seams — the internals tests legitimately manipulate.
  _internals: {
    getRunnerProcess: () => runnerProcess,
    setRunnerProcess: (p) => {
      runnerProcess = p;
    },
    setWebProcess: (p) => {
      webProcess = p;
    },
    setDesktopProcess: (p) => {
      desktopProcess = p;
    },
    setLibreProcess: (p) => {
      libreProcess = p;
    },
    setRunnerInstanceId: (id) => {
      currentRunnerInstanceId = id;
    },
    setNoExit: (v) => {
      noExit = v;
      exitedForTest = false;
    },
    getExitedForTest: () => exitedForTest,
    installSignalHandlers,
    getPlannedRunnerRestart: () => plannedRunnerRestart,
    setControlPath: (p) => {
      controlPath = p;
    },
    setRunnerPort: (p) => {
      runnerPort = String(p);
    },
    isExitingRef: () => isExiting,
  },
};
