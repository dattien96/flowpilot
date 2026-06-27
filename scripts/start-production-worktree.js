const { spawn, spawnSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const rootDir = path.resolve(__dirname, '..');
const defaultProductionWorktree = '.linked-worktrees/flowpilot-main';

function fail(message) {
  console.error(`[ProductionWorktree] ${message}`);
  process.exit(1);
}

function runGit(args) {
  const result = spawnSync('git', args, {
    cwd: rootDir,
    encoding: 'utf8',
  });
  if (result.status !== 0) {
    fail((result.stderr || result.stdout || `git ${args.join(' ')} failed`).trim());
  }
  return result.stdout;
}

function getCurrentWorktreeRoot() {
  return runGit(['rev-parse', '--show-toplevel']).trim();
}

function parseWorktreeList(output) {
  const entries = [];
  let current = null;
  for (const rawLine of output.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line) {
      if (current) {
        entries.push(current);
        current = null;
      }
      continue;
    }
    if (line.startsWith('worktree ')) {
      if (current) entries.push(current);
      current = { path: line.slice('worktree '.length), branch: '', head: '' };
      continue;
    }
    if (!current) continue;
    if (line.startsWith('branch ')) current.branch = line.slice('branch '.length);
    if (line.startsWith('HEAD ')) current.head = line.slice('HEAD '.length);
  }
  if (current) entries.push(current);
  return entries;
}

function resolvePreferredWorktree(entries, currentRoot) {
  const configured = (process.env.FLOWPILOT_PRODUCTION_WORKTREE || defaultProductionWorktree).trim();
  if (configured) {
    const targetPath = path.resolve(rootDir, configured);
    const matching = entries.find((entry) => path.resolve(entry.path) === targetPath);
    if (!matching) {
      fail(
        `FLOWPILOT_PRODUCTION_WORKTREE points to ${targetPath}, but that path is not a linked git worktree for this repo.`,
      );
    }
    if (path.resolve(matching.path) === path.resolve(currentRoot)) {
      fail('FLOWPILOT_PRODUCTION_WORKTREE points at the current worktree. Use a separate linked worktree.');
    }
    return matching;
  }

  const candidates = entries.filter((entry) => path.resolve(entry.path) !== path.resolve(currentRoot));
  const mainCandidates = candidates.filter((entry) => entry.branch === 'refs/heads/main');
  if (mainCandidates.length === 1) {
    return mainCandidates[0];
  }
  if (mainCandidates.length > 1) {
    fail(
      `Multiple main-branch worktrees found: ${mainCandidates
        .map((entry) => entry.path)
        .join(', ')}. Set FLOWPILOT_PRODUCTION_WORKTREE to choose one.`,
    );
  }

  fail(
    'No separate main-branch production worktree found. Create one with `just production-worktree` or set FLOWPILOT_PRODUCTION_WORKTREE.',
  );
}

function ensureProductionWorktree(dryRun) {
  const configuredPath = (process.env.FLOWPILOT_PRODUCTION_WORKTREE || defaultProductionWorktree).trim();
  const worktreePath = path.resolve(rootDir, configuredPath);
  const args = [
    'scripts/self-worktree.js',
    '--detach-at-ref',
    'refs/heads/main',
    '--sync-ref',
    'refs/heads/main',
    '--path',
    configuredPath,
    '--env-file',
    '.env',
    '--template',
    '.env.example',
  ];

  if (dryRun) {
    args.push('--dry-run');
  }

  const result = spawnSync('node', args, {
    cwd: rootDir,
    encoding: 'utf8',
  });
  if (result.status !== 0) {
    fail((result.stderr || result.stdout || 'Failed to prepare production worktree').trim());
  }

  const lines = (result.stdout || '')
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  const reportedPath = lines[lines.length - 1];
  return reportedPath || worktreePath;
}

function main() {
  let dryRun = false;
  const forwardedArgs = [];
  for (const arg of process.argv.slice(2)) {
    if (arg === '--dry-run') {
      dryRun = true;
      continue;
    }
    forwardedArgs.push(arg);
  }

  const currentRoot = getCurrentWorktreeRoot();
  const preparedPath = ensureProductionWorktree(dryRun);
  const entries = parseWorktreeList(runGit(['worktree', 'list', '--porcelain']));
  const target = resolvePreferredWorktree(entries, currentRoot);
  const supervisorPath = path.join(rootDir, 'scripts', 'supervisor.js');
  if (!fs.existsSync(supervisorPath)) {
    fail(`Current worktree does not contain ${supervisorPath}.`);
  }

  const args = ['scripts/supervisor.js', '--root-dir', target.path, '--env-file', '.env', ...forwardedArgs];
  console.log(`[ProductionWorktree] Using ${target.path}${target.branch ? ` (${target.branch})` : ''}`);
  if (dryRun) {
    if (path.resolve(target.path) !== path.resolve(preparedPath)) {
      console.log(`[ProductionWorktree] prepared ${preparedPath}`);
    }
    console.log(`[ProductionWorktree] node ${args.join(' ')}`);
    return;
  }
  const child = spawn('node', args, {
    cwd: rootDir,
    stdio: 'inherit',
    env: process.env,
  });

  child.on('exit', (code, signal) => {
    if (signal) {
      process.kill(process.pid, signal);
      return;
    }
    process.exit(code ?? 0);
  });
}

main();
