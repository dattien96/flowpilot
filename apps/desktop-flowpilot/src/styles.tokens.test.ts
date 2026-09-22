import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";

// Task-405 (T-5): token guardrail. Asserts the :root token scales exist and
// that ALL component rules in styles.css consume them instead of hardcoded
// px spacing/radius or ms transition durations. Hairline values (<3px),
// 0/1px borders, var()/calc()/percent values, and the :root block itself are
// exempt. New rules must follow the same contract — see STYLE-TOKENS.md.

const CSS = fs.readFileSync(path.resolve(process.cwd(), "src/styles.css"), "utf8");

function rootBlock(): string {
  const m = CSS.match(/:root\s*\{([\s\S]*?)\n\}/);
  assert.ok(m, ":root block not found in styles.css");
  return m![1];
}

// All declaration bodies outside :root (at-rules like @media/@keyframes are
// skipped by the regex since their headers start with @ or a percentage).
function allRuleBodies(): string[] {
  const root = rootBlock();
  const rest = CSS.slice(CSS.indexOf(":root") + root.length);
  const bodies: string[] = [];
  const ruleRe = /(^|\n)\s*([^{}@/][^{}]*)\{([^{}]*)\}/g;
  let m: RegExpExecArray | null;
  while ((m = ruleRe.exec(rest))) {
    const sel = m[2].trim();
    if (sel.startsWith(":root") || sel.startsWith("/*")) continue;
    bodies.push(m[3]);
  }
  return bodies;
}

test("root defines spacing scale 4-48", () => {
  const root = rootBlock();
  for (const tok of [
    "--space-1", "--space-2", "--space-3", "--space-4",
    "--space-5", "--space-6", "--space-7", "--space-8",
  ]) {
    assert.ok(root.includes(`${tok}:`), `missing ${tok} in :root`);
  }
  assert.ok(root.includes("--space-1: 4px"), "--space-1 must be 4px");
  assert.ok(root.includes("--space-8: 48px"), "--space-8 must be 48px");
});

test("root defines radius/elevation/motion scales", () => {
  const root = rootBlock();
  for (const tok of [
    "--radius-sm", "--radius-md", "--radius-lg", "--radius-pill",
    "--elev-1", "--elev-2", "--elev-3",
    "--dur-fast", "--dur-med", "--ease-standard",
    "--font-size-xs", "--font-size-sm", "--font-size-md", "--font-size-lg", "--font-size-xl",
    "--line-tight", "--line-normal",
  ]) {
    assert.ok(root.includes(`${tok}:`), `missing ${tok} in :root`);
  }
});

test("all rules use tokens for spacing/radius/motion", () => {
  const bodies = allRuleBodies();
  assert.ok(bodies.length > 200, `expected rules, found ${bodies.length}`);
  const offenders: string[] = [];
  const spacingRe = /(?:^|\s|;)(?:padding|margin|gap|row-gap|column-gap)(?:-[a-z]+)?\s*:\s*[^;]*?\b([3-9]|\d{2,})px\b/;
  const radiusRe = /border-radius\s*:\s*[^;]*?\b\d+px\b/;
  const durationRe = /transition[^:]*:[^;]*?\b\d+(?:\.\d+)?m?s\b/;
  for (const body of bodies) {
    for (const decl of body.split(";")) {
      const line = decl.trim();
      if (!line) continue;
      if (spacingRe.test(line)) offenders.push(`spacing: ${line.slice(0, 80)}`);
      if (radiusRe.test(line)) offenders.push(`radius: ${line.slice(0, 80)}`);
      if (durationRe.test(line)) offenders.push(`motion: ${line.slice(0, 80)}`);
    }
  }
  assert.deepEqual(offenders.slice(0, 30), [], `${offenders.length} hardcoded px/ms in styles.css`);
});

test("chrome components carry no emoji glyphs", () => {
  const emojiRe = /[\u{1F300}-\u{1FAFF}\u{FE0F}]/u;
  for (const file of [
    "src/components/Navigator.tsx",
    "src/components/AttentionQueue.tsx",
    "src/components/ChatPosturePanel.tsx",
  ]) {
    const src = fs.readFileSync(path.resolve(process.cwd(), file), "utf8");
    const stripped = src.replace(/\/\*[\s\S]*?\*\//g, "").replace(/\/\/[^\n]*/g, "");
    assert.equal(emojiRe.test(stripped), false, `emoji glyph found in ${file}`);
  }
});
