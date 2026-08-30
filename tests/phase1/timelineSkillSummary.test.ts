import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";

function repoRoot(): string {
  const candidates = [process.cwd(), path.resolve(process.cwd(), "../.."), path.resolve(__dirname, "../../..")];
  for (const c of candidates) {
    if (fs.existsSync(path.join(c, "apps/desktop-flowpilot/src/components/Timeline.tsx"))) return c;
  }
  return process.cwd();
}
const timelinePath = path.join(repoRoot(), "apps/desktop-flowpilot/src/components/Timeline.tsx");
const timelineSource = fs.readFileSync(timelinePath, "utf8");

test("Timeline keeps prompt skill summary collapsed to the first two selections", () => {
  assert.match(
    timelineSource,
    /const preview = skills\.slice\(0,\s*2\)\.map\(\(name\) => `\/\$\{name\}`\)\.join\(", "\);/,
  );
  assert.match(
    timelineSource,
    /const remainder = skills\.length - 2;/,
  );
  assert.match(
    timelineSource,
    /remainder > 0 \? ` \+\$\{remainder\}` : ""/,
  );
});

test("Timeline still renders the full selected skill list when the summary is expanded", () => {
  assert.match(
    timelineSource,
    /skills\.map\(\(name\) => \(\s*<div key=\{name\} className="prompt-skill-chip">/s,
  );
  assert.match(
    timelineSource,
    /<span className="prompt-skill-name">\/\{name\}<\/span>/,
  );
});
