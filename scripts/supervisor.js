const { spawn, execSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const net = require('net');
const http = require('http');

// Parse arguments
let webPort = '3002';
let runnerPort = '4317';
let restartExisting = false;
let withDesktop = false;
let desktopPath = 'apps/desktop-flowpilot';
const args = process.argv.slice(2);
for (let i = 0; i < args.length; i++) {
  if (args[i] === '--web-port' && args[i + 1]) {
    webPort = args[i + 1];
    i++;
  } else if (args[i] === '--runner-port' && args[i + 1]) {
    runnerPort = args[i + 1];
    i++;
  } else if (args[i] === '--restart-existing') {
    restartExisting = true;
  } else if (args[i] === '--with-desktop') {
    withDesktop = true;
  } else if (args[i] === '--desktop-path' && args[i + 1]) {
    desktopPath = args[i + 1];
    i++;
  }
}

const rootDir = path.resolve(__dirname, '..');
const flowpilotDir = path.join(rootDir, '.flowpilot');
const metadataPath = path.join(flowpilotDir, 'supervisor.json');
const controlPath = path.join(flowpilotDir, 'supervisor.cmd');

let webProcess = null;
let runnerProcess = null;
let desktopProcess = null;
let hintDesktopPid = null;
let isExiting = false;
let isRestarting = false;

function ensureDirectoryExists(dir) {
  if (!fs.existsSync(dir)) {
    fs.mkdirSync(dir, { recursive: true });
  }
}

function isPortInUse(port) {
  return new Promise((resolve) => {
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
        const output = execSync(`lsof -t -i tcp:${port}`, { encoding: 'utf8' }).trim();
        const pid = parseInt(output, 10);
        if (pid && pid > 0) return pid;
      } catch (e) {
        // Fallback for Linux using ss or netstat if lsof is missing
        try {
          const output = execSync(`ss -lptn 'sport = :${port}'`, { encoding: 'utf8' });
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
      return cmd.includes('node') || cmd.includes('npm') || cmd.includes('next');
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
    process.kill(child.pid, signal);
  } catch (e) {}
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
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

  const webInUse = await isPortInUse(parseInt(webPort, 10));
  const runnerInUse = await isPortInUse(parseInt(runnerPort, 10));

  // Resolve actual PIDs if in use and verify their ownership to prevent PID reuse issues
  let adoptedWebPid = null;
  if (webInUse) {
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

  if (webInUse && !adoptedWebPid) {
    throw new Error(
      `Port ${webPort} is already in use by a process that does not look like the FlowPilot web app. Stop that process or choose another port.`,
    );
  }

  if (runnerInUse && !adoptedRunnerPid) {
    throw new Error(
      `Port ${runnerPort} is already in use by a process that does not look like the FlowPilot local runner. Stop that process or choose another port.`,
    );
  }

  if (restartExisting && (adoptedWebPid || adoptedRunnerPid)) {
    console.log('[Supervisor] Restarting existing owned dev processes so env and code changes take effect...');
    await stopManagedProcess(adoptedRunnerPid ? createManagedProcessRef(adoptedRunnerPid, false) : null, 'runner');
    await stopManagedProcess(adoptedWebPid ? createManagedProcessRef(adoptedWebPid, false) : null, 'web');
    return startServicesFresh();
  }

  if (webInUse && runnerInUse) {
    console.log('[Supervisor] Both services are already running. Watching for control commands...');
    
    if (adoptedWebPid) {
      webProcess = createManagedProcessRef(adoptedWebPid, false);
    }
    if (adoptedRunnerPid) {
      runnerProcess = createManagedProcessRef(adoptedRunnerPid, false);
    }

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
  if (!webInUse) {
    console.log(`[Supervisor] Starting web service on port ${webPort}...`);
    const webCmd = process.platform === 'win32' ? 'npm.cmd' : 'npm';
    webProcess = spawn(webCmd, ['run', 'dev', '--', '--port', webPort], {
      cwd: path.join(rootDir, 'apps', 'admin-web'),
      shell: true,
      stdio: 'inherit',
      detached: process.platform !== 'win32',
    });
    webProcess.detached = process.platform !== 'win32';

    attachExitHandlers(webProcess, 'Web');
  } else {
    console.log(`[Supervisor] Web service is already running on port ${webPort}, skipping start.`);
    if (adoptedWebPid) {
      webProcess = createManagedProcessRef(adoptedWebPid, false);
    }
  }

  // Spawn runner if not running
  if (!runnerInUse) {
    console.log(`[Supervisor] Starting runner service on port ${runnerPort}...`);
    const runnerCmd = process.platform === 'win32' ? 'go.exe' : 'go';
    runnerProcess = spawn(runnerCmd, ['run', './cmd/flowpilot', 'runner', 'serve', '--port', runnerPort], {
      cwd: path.join(rootDir, 'apps', 'local-runner'),
      shell: true,
      stdio: 'inherit',
      detached: process.platform !== 'win32',
    });
    runnerProcess.detached = process.platform !== 'win32';

    attachExitHandlers(runnerProcess, 'Runner');
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
    const desktopRunnerUrl = process.env.VITE_RUNNER_URL || `http://127.0.0.1:${runnerPort}`;
    console.log(`[Supervisor] Starting desktop app (Electron + Vite) → runner ${desktopRunnerUrl}...`);
    const desktopCmd = process.platform === 'win32' ? 'npm.cmd' : 'npm';
    desktopProcess = spawn(desktopCmd, ['run', 'dev'], {
      cwd: path.join(rootDir, desktopPath),
      shell: true,
      stdio: 'inherit',
      detached: process.platform !== 'win32',
      env: { ...process.env, VITE_RUNNER_URL: desktopRunnerUrl },
    });
    desktopProcess.detached = process.platform !== 'win32';
    attachDesktopExitHandler(desktopProcess);
  }

  // Write supervisor metadata
  const metadata = {
    supervisorPid: process.pid,
    webPid: webProcess ? webProcess.pid : null,
    runnerPid: runnerProcess ? runnerProcess.pid : null,
    desktopPid: desktopProcess ? desktopProcess.pid : null,
    controlPath: controlPath,
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

  // Wait up to 3 seconds for processes to exit on their own before force-killing
  let checks = 0;
  const maxChecks = 15; // 15 * 200ms = 3000ms
  const interval = setInterval(() => {
    const runnerAlive = runnerProcess && runnerProcess.exitCode === null && !runnerProcess.killed && isPidAlive(runnerProcess.pid);
    const webAlive = webProcess && webProcess.exitCode === null && !webProcess.killed && isPidAlive(webProcess.pid);
    const desktopAlive = desktopProcess && desktopProcess.exitCode === null && !desktopProcess.killed && isPidAlive(desktopProcess.pid);

    if (!runnerAlive && !webAlive && !desktopAlive) {
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
        finishExit();
      }
    }
  }, 200);
}

function finishExit() {
  try {
    if (fs.existsSync(controlPath)) fs.unlinkSync(controlPath);
  } catch (e) {}
  try {
    if (fs.existsSync(metadataPath)) fs.unlinkSync(metadataPath);
  } catch (e) {}
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

  // Wait up to 2 seconds for processes to exit, then force-kill if still alive
  let checks = 0;
  const maxChecks = 10; // 10 * 200ms = 2000ms
  const interval = setInterval(() => {
    const runnerAlive = runnerProcess && runnerProcess.exitCode === null && !runnerProcess.killed && isPidAlive(runnerProcess.pid);
    const webAlive = webProcess && webProcess.exitCode === null && !webProcess.killed && isPidAlive(webProcess.pid);
    const desktopAlive = desktopProcess && desktopProcess.exitCode === null && !desktopProcess.killed && isPidAlive(desktopProcess.pid);

    if (!runnerAlive && !webAlive && !desktopAlive) {
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
        setTimeout(() => {
          isRestarting = false;
          startServices();
        }, 500);
      }
    }
  }, 200);
}

// Watch control file
setInterval(() => {
  if (isExiting) return;
  if (fs.existsSync(controlPath)) {
    try {
      const command = fs.readFileSync(controlPath, 'utf8').trim().toLowerCase();
      if (command === 'shutdown') {
        console.log('[Supervisor] Shutdown command detected. Waiting 200ms for response flush...');
        setTimeout(() => {
          cleanupAndExit();
        }, 200);
      } else if (command === 'restart') {
        console.log('[Supervisor] Restart command detected. Waiting 200ms for response flush...');
        setTimeout(() => {
          handleRestart();
        }, 200);
      }
    } catch (e) {
      // File might be locked temporarily, ignore
    }
  }
}, 500);

// Capture signals
process.on('SIGINT', cleanupAndExit);
process.on('SIGTERM', cleanupAndExit);

// Start
startServices();
