import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";

const forbiddenPatterns = [
  /from\s+["']react["']/,
  /from\s+["']electron["']/,
  /from\s+["']@supabase\/supabase-js["']/,
  /from\s+["']react-router/,
  /\bwindow\b/,
  /\bdocument\b/,
  /\blocalStorage\b/,
];

function collectTypeScriptFiles(dir: string, files: string[] = []): string[] {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      collectTypeScriptFiles(fullPath, files);
    } else if (entry.isFile() && entry.name.endsWith(".ts")) {
      files.push(fullPath);
    }
  }
  return files;
}

test("client-core domain files do not depend on presentation/browser/electron imports", () => {
  const domainDir = path.resolve(process.cwd(), "../../packages/flowpilot-client-core/src/domain");
  const files = collectTypeScriptFiles(domainDir);
  assert.ok(files.length > 0, "expected domain files to exist");

  for (const file of files) {
    const content = fs.readFileSync(file, "utf8");
    for (const pattern of forbiddenPatterns) {
      assert.equal(
        pattern.test(content),
        false,
        `forbidden dependency ${pattern} found in ${path.relative(process.cwd(), file)}`,
      );
    }
  }
});
