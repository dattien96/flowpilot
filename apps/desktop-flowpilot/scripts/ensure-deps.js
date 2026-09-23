// Auto-install deps when node_modules is missing or manifest/lockfile changed
// since the last install — devs shouldn't need to know to run npm install.
const fs = require("fs");
const path = require("path");
const { execSync } = require("child_process");

const root = path.join(__dirname, "..");
const stamp = path.join(root, "node_modules", ".package-lock.json");

const stale =
  !fs.existsSync(path.join(root, "node_modules")) ||
  !fs.existsSync(stamp) ||
  fs.statSync(path.join(root, "package.json")).mtimeMs > fs.statSync(stamp).mtimeMs ||
  fs.statSync(path.join(root, "package-lock.json")).mtimeMs > fs.statSync(stamp).mtimeMs;

if (stale) {
  console.log("[ensure-deps] deps missing or manifest/lockfile changed — running npm install");
  execSync("npm install --no-audit --no-fund", { cwd: root, stdio: "inherit" });
}

// node-pty's darwin prebuild ships spawn-helper without the exec bit after some
// npm extractions — without it every term:spawn dies with `posix_spawnp
// failed` (BUG-385). Cheap to assert on every run, not just post-install.
const ptyPrebuilds = path.join(root, "node_modules", "node-pty", "prebuilds");
if (fs.existsSync(ptyPrebuilds)) {
  for (const arch of fs.readdirSync(ptyPrebuilds)) {
    const helper = path.join(ptyPrebuilds, arch, "spawn-helper");
    try {
      fs.accessSync(helper, fs.constants.X_OK);
    } catch {
      if (fs.existsSync(helper)) {
        fs.chmodSync(helper, 0o755);
        console.log(`[ensure-deps] restored +x on ${path.relative(root, helper)}`);
      }
    }
  }
}
