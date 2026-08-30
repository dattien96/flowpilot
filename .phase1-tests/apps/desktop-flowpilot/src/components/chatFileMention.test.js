"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const chatFileMention_1 = require("./chatFileMention");
(0, node_test_1.default)("findActiveAt reads @query at the start and after a space", () => {
    strict_1.default.deepEqual((0, chatFileMention_1.findActiveAt)("@ChatIn", 7), { index: 0, query: "ChatIn" });
    strict_1.default.deepEqual((0, chatFileMention_1.findActiveAt)("see @apps/foo", 13), { index: 4, query: "apps/foo" });
});
(0, node_test_1.default)("findActiveAt ignores email-style @ and stops at a space", () => {
    strict_1.default.equal((0, chatFileMention_1.findActiveAt)("a@b.ts", 6), null);
    strict_1.default.equal((0, chatFileMention_1.findActiveAt)("@foo bar", 8), null);
});
(0, node_test_1.default)("isAgentAtMention keeps start-of-prompt @coder as agent routing", () => {
    strict_1.default.equal((0, chatFileMention_1.isAgentAtMention)({ index: 0, query: "coder" }, ["coder"]), true);
    strict_1.default.equal((0, chatFileMention_1.isAgentAtMention)({ index: 0, query: "apps/foo" }, ["coder"]), false);
    strict_1.default.equal((0, chatFileMention_1.isAgentAtMention)({ index: 4, query: "coder" }, ["coder"]), false);
});
(0, node_test_1.default)("filterWorkspaceFiles matches substring and caps", () => {
    const paths = [
        "apps/desktop-flowpilot/src/components/ChatInput.tsx",
        "apps/local-runner/internal/runner/workspace_files.go",
    ];
    strict_1.default.deepEqual((0, chatFileMention_1.filterWorkspaceFiles)(paths, "ChatInput"), [paths[0]]);
    strict_1.default.equal((0, chatFileMention_1.filterWorkspaceFiles)(paths, "", 1).length, 1);
});
(0, node_test_1.default)("insertAtMention replaces @query with the path like a skill name", () => {
    const got = (0, chatFileMention_1.insertAtMention)("see @ChatIn please", { index: 4, query: "ChatIn" }, 11, "apps/foo/ChatInput.tsx");
    strict_1.default.equal(got.text, "see apps/foo/ChatInput.tsx please");
    strict_1.default.equal(got.cursor, 4 + "apps/foo/ChatInput.tsx".length);
});
