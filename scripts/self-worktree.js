const { spawnSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const rootDir = path.resolve(__dirname, '..');
const args = process.argv.slice(2);

let branchName = 'task/flowpilot-dev';
let relativePath = '../flowpilot-dev';
let dryRun = false;
let copyEnv = true;
let envFileName = '.env.dev';
let templateName = '.env.dev.example';
let existingBranchMode = false;
let sourceWorktreePath = rootDir;
let detachRef = '';
let syncRef = '';

for (let index = 0; index < args.length; index += 1) {
  const arg = args[index];
  if (arg === '--branch' && args[index + 1]) {
    branchName = args[index + 1];
    index += 1;
    continue;
  }
  if (arg === '--path' && args[index + 1]) {
    relativePath = args[index + 1];
    index += 1;
    continue;
  }
  if (arg === '--dry-run') {
    dryRun = true;
    continue;
  }
  if (arg === '--env-file' && args[index + 1]) {
    envFileName = args[index + 1];
    index += 1;
    continue;
  }
  if (arg === '--template' && args[index + 1]) {
    templateName = args[index + 1];
    index += 1;
    continue;
  }
  if (arg === '--existing-branch') {
    existingBranchMode = true;
    continue;
  }
  if (arg === '--source-worktree' && args[index + 1]) {
    sourceWorktreePath = path.resolve(rootDir, args[index + 1]);
    index += 1;
    continue;
  }
  if (arg === '--detach-at-ref' && args[index + 1]) {
    detachRef = args[index + 1];
    index += 1;
    continue;
  }
  if (arg === '--sync-ref' && args[index + 1]) {
    syncRef = args[index + 1];
    index += 1;
    continue;
  }
  if (arg === '--no-copy-env') {
    copyEnv = false;
    continue;
  }
  if (!arg.startsWith('--') && branchName === 'task/flowpilot-dev') {
    branchName = arg;
    continue;
  }
  fail(`Unknown argument: ${arg}`);
}

const targetPath = path.resolve(rootDir, relativePath);

function fail(message) {
  console.error(`[SelfWorktree] ${message}`);
  process.exit(1);
}

function runGit(gitArgs) {
  const result = spawnSync('git', gitArgs, {
    cwd: rootDir,
    encoding: 'utf8',
  });
  if (result.status !== 0) {
    fail((result.stderr || result.stdout || `git ${gitArgs.join(' ')} failed`).trim());
  }
  return result.stdout;
}

function runGitInDirectory(cwd, gitArgs) {
  const result = spawnSync('git', gitArgs, {
    cwd,
    encoding: 'utf8',
  });
  if (result.status !== 0) {
    fail((result.stderr || result.stdout || `git ${gitArgs.join(' ')} failed`).trim());
  }
  return result.stdout;
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

function ensureWorktree() {
  const worktrees = parseWorktreeList(runGit(['worktree', 'list', '--porcelain']));
  const existingByPath = worktrees.find((entry) => path.resolve(entry.path) === targetPath);
  if (existingByPath) {
    return existingByPath.path;
  }

  const existingBranch = worktrees.find((entry) => entry.branch === `refs/heads/${branchName}`);
  if (existingBranch) {
    fail(
      `Branch ${branchName} is already checked out at ${existingBranch.path}. Use that worktree or choose another branch.`,
    );
  }

  if (dryRun) {
    if (detachRef) {
      console.log(`[SelfWorktree] git worktree add --detach ${targetPath} ${detachRef}`);
    } else {
      console.log(
        `[SelfWorktree] git worktree add ${targetPath}${existingBranchMode ? ` ${branchName}` : ` -b ${branchName}`}`,
      );
    }
    return targetPath;
  }

  const gitArgs = detachRef
    ? ['worktree', 'add', '--detach', targetPath, detachRef]
    : existingBranchMode
      ? ['worktree', 'add', targetPath, branchName]
      : ['worktree', 'add', targetPath, '-b', branchName];
  runGit(gitArgs);
  return targetPath;
}

function ensureSyncedRef(worktreePath) {
  if (!syncRef) return;
  const resolvedRef = runGit(['rev-parse', syncRef]).trim();

  if (dryRun) {
    console.log(`[SelfWorktree] git -C ${worktreePath} checkout --detach ${resolvedRef}`);
    console.log(`[SelfWorktree] git -C ${worktreePath} reset --hard ${resolvedRef}`);
    return;
  }

  runGitInDirectory(worktreePath, ['checkout', '--detach', resolvedRef]);
  runGitInDirectory(worktreePath, ['reset', '--hard', resolvedRef]);
}

function ensureEnvFile(worktreePath) {
  if (!copyEnv) return;
  const targetEnvPath = path.join(worktreePath, envFileName);
  if (fs.existsSync(targetEnvPath)) return;

  const localDevEnvPath = path.join(rootDir, envFileName);
  const templatePath = path.join(rootDir, templateName);
  const sourcePath = fs.existsSync(localDevEnvPath) ? localDevEnvPath : templatePath;
  if (!fs.existsSync(sourcePath)) return;

  if (dryRun) {
    console.log(`[SelfWorktree] copy ${sourcePath} -> ${targetEnvPath}`);
    return;
  }

  fs.copyFileSync(sourcePath, targetEnvPath);
}

function ensureSharedDependencies(worktreePath) {
  const dependencyDirs = [
    'node_modules',
    path.join('apps', 'admin-web', 'node_modules'),
    path.join('apps', 'desktop-flowpilot', 'node_modules'),
  ];

  for (const relativeDir of dependencyDirs) {
    const sourceDir = path.join(sourceWorktreePath, relativeDir);
    const targetDir = path.join(worktreePath, relativeDir);
    if (!fs.existsSync(sourceDir) || fs.existsSync(targetDir)) {
      continue;
    }

    if (dryRun) {
      console.log(`[SelfWorktree] symlink ${targetDir} -> ${sourceDir}`);
      continue;
    }

    fs.mkdirSync(path.dirname(targetDir), { recursive: true });
    fs.symlinkSync(sourceDir, targetDir, 'dir');
  }
}

function ensureFlowpilotState(worktreePath) {
  const sourceFlowpilotDir = path.join(sourceWorktreePath, '.flowpilot');
  const targetFlowpilotDir = path.join(worktreePath, '.flowpilot');
  if (!fs.existsSync(sourceFlowpilotDir)) {
    return;
  }

  if (dryRun) {
    console.log(`[SelfWorktree] replace ${targetFlowpilotDir} from ${sourceFlowpilotDir}`);
    return;
  }

  fs.rmSync(targetFlowpilotDir, { recursive: true, force: true });
  fs.cpSync(sourceFlowpilotDir, targetFlowpilotDir, { recursive: true });
}

function main() {
  const worktreePath = ensureWorktree();
  ensureSyncedRef(worktreePath);
  ensureEnvFile(worktreePath);
  ensureSharedDependencies(worktreePath);
  ensureFlowpilotState(worktreePath);
  console.log(worktreePath);
}

main();
