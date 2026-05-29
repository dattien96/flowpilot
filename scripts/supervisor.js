const { spawn } = require('child_process');
const fs = require('fs');
const path = require('path');
const net = require('net');
const http = require('http');

// Parse arguments
let webPort = '3002';
let runnerPort = '4317';
const args = process.argv.slice(2);
for (let i = 0; i < args.length; i++) {
  if (args[i] === '--web-port' && args[i + 1]) {
    webPort = args[i + 1];
    i++;
  } else if (args[i] === '--runner-port' && args[i + 1]) {
    runnerPort = args[i + 1];
    i++;
  }
}

const rootDir = path.resolve(__dirname, '..');
const flowpilotDir = path.join(rootDir, '.flowpilot');
const metadataPath = path.join(flowpilotDir, 'supervisor.json');
const controlPath = path.join(flowpilotDir, 'supervisor.cmd');

let webProcess = null;
let runnerProcess = null;
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

async function startServices() {
  ensureDirectoryExists(flowpilotDir);

  const webInUse = await isPortInUse(parseInt(webPort, 10));
  const runnerInUse = await isPortInUse(parseInt(runnerPort, 10));

  if (webInUse && runnerInUse) {
    console.log('[Supervisor] Both services are already running.');
    process.exit(0);
  }

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

    webProcess.on('exit', (code) => {
      if (!isExiting && !isRestarting) {
        console.log(`[Supervisor] Web process exited with code ${code}. Exiting...`);
        cleanupAndExit();
      }
    });
  } else {
    console.log(`[Supervisor] Web service is already running on port ${webPort}, skipping start.`);
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

    runnerProcess.on('exit', (code) => {
      if (!isExiting && !isRestarting) {
        console.log(`[Supervisor] Runner process exited with code ${code}. Exiting...`);
        cleanupAndExit();
      }
    });
  } else {
    console.log(`[Supervisor] Runner service is already running on port ${runnerPort}, skipping start.`);
  }

  // Write supervisor metadata
  const metadata = {
    supervisorPid: process.pid,
    webPid: webProcess ? webProcess.pid : null,
    runnerPid: runnerProcess ? runnerProcess.pid : null,
    controlPath: controlPath,
  };
  fs.writeFileSync(metadataPath, JSON.stringify(metadata, null, 2));
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
      // With detached: true on Unix, pid is the process group ID.
      // -pid signals the process group.
      process.kill(-pid, 'SIGKILL');
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
  if (process.platform !== 'win32') {
    if (runnerProcess && runnerProcess.pid) {
      try { process.kill(-runnerProcess.pid, 'SIGINT'); } catch (e) {}
    }
    if (webProcess && webProcess.pid) {
      try { process.kill(-webProcess.pid, 'SIGINT'); } catch (e) {}
    }
  }

  // Wait up to 3 seconds for processes to exit on their own before force-killing
  let checks = 0;
  const maxChecks = 15; // 15 * 200ms = 3000ms
  const interval = setInterval(() => {
    const runnerAlive = runnerProcess && runnerProcess.exitCode === null && !runnerProcess.killed;
    const webAlive = webProcess && webProcess.exitCode === null && !webProcess.killed;

    if (!runnerAlive && !webAlive) {
      clearInterval(interval);
      finishExit();
    } else {
      checks++;
      if (checks >= maxChecks) {
        console.log('[Supervisor] Grace period expired. Force-killing lingering processes...');
        clearInterval(interval);
        if (runnerAlive) killProcessTree(runnerProcess);
        if (webAlive) killProcessTree(webProcess);
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

  // Request runner to clean up sessions via HTTP
  triggerHTTPShutdown();

  // Signal the process groups on Unix first
  if (process.platform !== 'win32') {
    if (runnerProcess && runnerProcess.pid) {
      try { process.kill(-runnerProcess.pid, 'SIGINT'); } catch (e) {}
    }
    if (webProcess && webProcess.pid) {
      try { process.kill(-webProcess.pid, 'SIGINT'); } catch (e) {}
    }
  }

  // Wait up to 2 seconds for processes to exit, then force-kill if still alive
  let checks = 0;
  const maxChecks = 10; // 10 * 200ms = 2000ms
  const interval = setInterval(() => {
    const runnerAlive = runnerProcess && runnerProcess.exitCode === null && !runnerProcess.killed;
    const webAlive = webProcess && webProcess.exitCode === null && !webProcess.killed;

    if (!runnerAlive && !webAlive) {
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
