"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const node_fs_1 = __importDefault(require("node:fs"));
const node_path_1 = __importDefault(require("node:path"));
const timelinePath = node_path_1.default.join(process.cwd(), "apps/desktop-flowpilot/src/components/Timeline.tsx");
const timelineSource = node_fs_1.default.readFileSync(timelinePath, "utf8");
(0, node_test_1.default)("Timeline keeps prompt skill summary collapsed to the first two selections", () => {
    strict_1.default.match(timelineSource, /const preview = skills\.slice\(0,\s*2\)\.map\(\(name\) => `\/\$\{name\}`\)\.join\(", "\);/);
    strict_1.default.match(timelineSource, /const remainder = skills\.length - 2;/);
    strict_1.default.match(timelineSource, /remainder > 0 \? ` \+\$\{remainder\}` : ""/);
});
(0, node_test_1.default)("Timeline still renders the full selected skill list when the summary is expanded", () => {
    strict_1.default.match(timelineSource, /skills\.map\(\(name\) => \(\s*<div key=\{name\} className="prompt-skill-chip">/s);
    strict_1.default.match(timelineSource, /<span className="prompt-skill-name">\/\{name\}<\/span>/);
});
