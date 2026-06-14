"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const node_fs_1 = __importDefault(require("node:fs"));
const node_path_1 = __importDefault(require("node:path"));
const forbiddenPatterns = [
    /from\s+["']react["']/,
    /from\s+["']electron["']/,
    /from\s+["']@supabase\/supabase-js["']/,
    /from\s+["']react-router/,
    /\bwindow\b/,
    /\bdocument\b/,
    /\blocalStorage\b/,
];
function collectTypeScriptFiles(dir, files = []) {
    for (const entry of node_fs_1.default.readdirSync(dir, { withFileTypes: true })) {
        const fullPath = node_path_1.default.join(dir, entry.name);
        if (entry.isDirectory()) {
            collectTypeScriptFiles(fullPath, files);
        }
        else if (entry.isFile() && entry.name.endsWith(".ts")) {
            files.push(fullPath);
        }
    }
    return files;
}
(0, node_test_1.default)("client-core domain files do not depend on presentation/browser/electron imports", () => {
    const domainDir = node_path_1.default.resolve(process.cwd(), "../../packages/flowpilot-client-core/src/domain");
    const files = collectTypeScriptFiles(domainDir);
    strict_1.default.ok(files.length > 0, "expected domain files to exist");
    for (const file of files) {
        const content = node_fs_1.default.readFileSync(file, "utf8");
        for (const pattern of forbiddenPatterns) {
            strict_1.default.equal(pattern.test(content), false, `forbidden dependency ${pattern} found in ${node_path_1.default.relative(process.cwd(), file)}`);
        }
    }
});
