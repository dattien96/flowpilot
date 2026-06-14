"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const adminLogic_1 = require("../../packages/flowpilot-client-core/src/domain/adminLogic");
(0, node_test_1.default)("resolveProviderKeyForModel handles codex, gemini, auto-gemini, and claude prefixes", () => {
    strict_1.default.equal((0, adminLogic_1.resolveProviderKeyForModel)("gpt-5.4", []), "codex");
    strict_1.default.equal((0, adminLogic_1.resolveProviderKeyForModel)("gemini-2.5-pro", []), "gemini");
    strict_1.default.equal((0, adminLogic_1.resolveProviderKeyForModel)("auto-gemini-2.5-pro", []), "gemini");
    strict_1.default.equal((0, adminLogic_1.resolveProviderKeyForModel)("claude-sonnet-4", []), "claude");
});
(0, node_test_1.default)("resolveProviderKeyForModel falls back to supported model registry", () => {
    strict_1.default.equal((0, adminLogic_1.resolveProviderKeyForModel)("custom-model", [
        {
            id: "model-1",
            providerKey: "gemini",
            modelId: "custom-model",
            displayName: "Custom Model",
            isEnabled: true,
            sortOrder: 1,
            source: "manual",
            detectionMethod: null,
            detectedCliVersion: null,
            lastDetectedAt: null,
            createdAt: "",
            updatedAt: "",
        },
    ]), "gemini");
});
(0, node_test_1.default)("assertValidProjectBindings rejects empty and duplicate paths", () => {
    strict_1.default.throws(() => (0, adminLogic_1.assertValidProjectBindings)([]), /Add at least one directory binding/);
    strict_1.default.throws(() => (0, adminLogic_1.assertValidProjectBindings)([
        { localPath: "/workspace/app" },
        { localPath: "/workspace/app" },
    ]), /Directory bindings must use unique paths/);
});
(0, node_test_1.default)("assertValidProjectBindings accepts unique trimmed paths", () => {
    strict_1.default.doesNotThrow(() => (0, adminLogic_1.assertValidProjectBindings)([
        { localPath: " /workspace/app " },
        { localPath: "/workspace/api" },
    ]));
});
