import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";

const timelinePath = path.join(process.cwd(), "apps/desktop-flowpilot/src/components/Timeline.tsx");
const timelineSource = fs.readFileSync(timelinePath, "utf8");
const stylesPath = path.join(process.cwd(), "apps/desktop-flowpilot/src/styles.css");
const stylesSource = fs.readFileSync(stylesPath, "utf8");

test("Timeline clamps user prompt bubbles to 4 lines with a '....' tail", () => {
  assert.match(timelineSource, /const PROMPT_MAX_LINES = 4;/);
  assert.match(timelineSource, /const PROMPT_ELLIPSIS = "\.\.\.\.";/);
  assert.match(timelineSource, /WebkitLineClamp: PROMPT_MAX_LINES/);
  assert.match(timelineSource, /className=\{`prompt-text \$\{clamped \? "prompt-text-clamped" : ""\}`\}/);
  assert.match(timelineSource, /clamped && <span className="prompt-ellipsis">\{PROMPT_ELLIPSIS\}<\/span>/);
});

test("Timeline makes a truncated prompt card click-expandable without toggling copy", () => {
  assert.match(
    timelineSource,
    /target\.closest\("\.bubble-copy"\) \|\| target\.closest\("\.prompt-skills-summary"\)\) return;/,
  );
  assert.match(timelineSource, /className=\{`prompt-stack \$\{truncated \? "prompt-truncatable" : ""\} \$\{expanded \? "prompt-expanded" : ""\}`\}/);
  assert.match(timelineSource, /onClick=\{toggle\}/);
  assert.match(timelineSource, /setExpanded\(\(value\) => !value\)/);
  assert.match(timelineSource, /el\.scrollHeight > el\.clientHeight \+ 1/);
});

test("Timeline prompt clamp CSS caps the bubble and hides the ellipsis once expanded", () => {
  assert.match(stylesSource, /\.prompt-text-clamped \{/);
  assert.match(stylesSource, /-webkit-line-clamp/);
  assert.match(stylesSource, /\.prompt-ellipsis \{/);
  assert.match(stylesSource, /\.prompt-expanded \.prompt-ellipsis \{[\s\S]*display: none;/);
});