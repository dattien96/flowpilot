"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const settingsHelpers_1 = require("../../apps/desktop-flowpilot/src/components/settings/settingsHelpers");
class FakeDirectoryRepository {
    results;
    constructor(results) {
        this.results = results;
    }
    async validatePath(path) {
        const result = this.results[path];
        if (!result) {
            throw new Error(`unexpected path ${path}`);
        }
        return { path, ...result };
    }
}
function binding(localPath) {
    return {
        id: localPath,
        projectId: "project-1",
        localPath,
        label: null,
        createdAt: "",
        updatedAt: "",
    };
}
(0, node_test_1.default)("validateDirectoryBindingsInOrder returns the first usable binding in configured order", async () => {
    const directories = new FakeDirectoryRepository({
        "/workspace/a": { usable: false, reason: "missing" },
        "/workspace/b": { usable: true, reason: "" },
        "/workspace/c": { usable: true, reason: "" },
    });
    const result = await (0, settingsHelpers_1.validateDirectoryBindingsInOrder)([binding("/workspace/a"), binding("/workspace/b"), binding("/workspace/c")], directories);
    strict_1.default.equal(result, "/workspace/b");
});
(0, node_test_1.default)("validateDirectoryBindingsInOrder reports every checked binding when none are usable", async () => {
    const directories = new FakeDirectoryRepository({
        "/workspace/a": { usable: false, reason: "missing" },
        "/workspace/b": { usable: false, reason: "not a directory" },
    });
    await strict_1.default.rejects(() => (0, settingsHelpers_1.validateDirectoryBindingsInOrder)([binding("/workspace/a"), binding("/workspace/b")], directories), /\/workspace\/a: missing[\s\S]*\/workspace\/b: not a directory/);
});
