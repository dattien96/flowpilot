"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const node_fs_1 = __importDefault(require("node:fs"));
const node_path_1 = __importDefault(require("node:path"));
function repoRoot() {
    const candidates = [process.cwd(), node_path_1.default.resolve(process.cwd(), "../.."), node_path_1.default.resolve(__dirname, "../../..")];
    for (const c of candidates) {
        if (node_fs_1.default.existsSync(node_path_1.default.join(c, "apps/desktop-flowpilot/src/components/Timeline.tsx")))
            return c;
    }
    return process.cwd();
}
const timelinePath = node_path_1.default.join(repoRoot(), "apps/desktop-flowpilot/src/components/Timeline.tsx");
const timelineSource = node_fs_1.default.readFileSync(timelinePath, "utf8");
const stylesPath = node_path_1.default.join(repoRoot(), "apps/desktop-flowpilot/src/styles.css");
const stylesSource = node_fs_1.default.readFileSync(stylesPath, "utf8");
(0, node_test_1.default)("Timeline clamps user prompt bubbles to 4 lines with a '....' tail", () => {
    strict_1.default.match(timelineSource, /const PROMPT_MAX_LINES = 4;/);
    strict_1.default.match(timelineSource, /const PROMPT_ELLIPSIS = "\.\.\.\.";/);
    strict_1.default.match(timelineSource, /WebkitLineClamp: PROMPT_MAX_LINES/);
    strict_1.default.match(timelineSource, /className=\{`prompt-text \$\{clamped \? "prompt-text-clamped" : ""\}`\}/);
    strict_1.default.match(timelineSource, /clamped && <span className="prompt-ellipsis">\{PROMPT_ELLIPSIS\}<\/span>/);
});
(0, node_test_1.default)("Timeline makes a truncated prompt card click-expandable without toggling copy", () => {
    strict_1.default.match(timelineSource, /target\.closest\("\.bubble-copy"\) \|\| target\.closest\("\.prompt-skills-summary"\)\) return;/);
    strict_1.default.match(timelineSource, /className=\{`prompt-stack \$\{truncated \? "prompt-truncatable" : ""\} \$\{expanded \? "prompt-expanded" : ""\}`\}/);
    strict_1.default.match(timelineSource, /onClick=\{toggle\}/);
    strict_1.default.match(timelineSource, /setExpanded\(\(value\) => !value\)/);
    strict_1.default.match(timelineSource, /el\.scrollHeight > el\.clientHeight \+ 1/);
});
(0, node_test_1.default)("Timeline prompt clamp CSS caps the bubble and hides the ellipsis once expanded", () => {
    strict_1.default.match(stylesSource, /\.prompt-text-clamped \{/);
    strict_1.default.match(stylesSource, /-webkit-line-clamp/);
    strict_1.default.match(stylesSource, /\.prompt-ellipsis \{/);
    strict_1.default.match(stylesSource, /\.prompt-expanded \.prompt-ellipsis \{[\s\S]*display: none;/);
});
