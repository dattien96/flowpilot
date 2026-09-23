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
