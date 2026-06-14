"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const runnerRuntimeConfigRepository_1 = require("../../packages/flowpilot-client-core/src/data/runnerRuntimeConfigRepository");
const runnerRepository_1 = require("../../packages/flowpilot-client-core/src/data/runnerRepository");
class FakeHttpClient {
    handler;
    constructor(handler) {
        this.handler = handler;
    }
    request(input, init) {
        return this.handler(input, init);
    }
}
(0, node_test_1.default)("RunnerRuntimeConfigRepository returns invalid-config error from the runner", async () => {
    const repository = new runnerRuntimeConfigRepository_1.RunnerRuntimeConfigRepository(new FakeHttpClient(async () => new Response("Saved Supabase config is incomplete.", { status: 500 })), "http://127.0.0.1:4317");
    const status = await repository.loadSupabaseRuntimeStatus();
    strict_1.default.equal(status.configured, false);
    strict_1.default.equal(status.runnerReachable, true);
    strict_1.default.equal(status.lastError, "Saved Supabase config is incomplete.");
});
(0, node_test_1.default)("RunnerRuntimeConfigRepository maps saved config payload", async () => {
    const repository = new runnerRuntimeConfigRepository_1.RunnerRuntimeConfigRepository(new FakeHttpClient(async () => new Response(JSON.stringify({
        apiUrl: "https://proj.supabase.co",
        anonKey: "anon",
        edgeFunctionUrl: "https://proj.supabase.co/functions/v1",
        hasServiceRoleKey: true,
    }), { status: 200 })), "http://127.0.0.1:4317");
    const status = await repository.loadSupabaseRuntimeStatus();
    strict_1.default.equal(status.configured, true);
    strict_1.default.equal(status.projectRef, "proj");
    strict_1.default.equal(status.edgeFunctionsReady, true);
});
(0, node_test_1.default)("HttpRunnerRepository maps health payload", async () => {
    const repository = new runnerRepository_1.HttpRunnerRepository(new FakeHttpClient(async () => new Response(JSON.stringify({
        cwd: "/workspace/app",
        os: "darwin",
        startedAt: "2026-06-14T10:00:00.000Z",
        version: "1.2.3",
    }), { status: 200 })), "http://127.0.0.1:4317");
    const health = await repository.loadHealth();
    strict_1.default.deepEqual(health, {
        cwd: "/workspace/app",
        os: "darwin",
        startedAt: "2026-06-14T10:00:00.000Z",
        version: "1.2.3",
    });
});
