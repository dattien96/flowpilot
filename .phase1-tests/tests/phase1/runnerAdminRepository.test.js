"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const runnerAdminRepository_1 = require("../../packages/flowpilot-client-core/src/data/runnerAdminRepository");
class RecordingHttpClient {
    responseFactory;
    requests = [];
    constructor(responseFactory) {
        this.responseFactory = responseFactory;
    }
    async request(input, init) {
        const url = String(input);
        this.requests.push({ url, init });
        return this.responseFactory(url, init);
    }
}
(0, node_test_1.default)("RunnerAdminRepository validates directory paths through runner endpoint", async () => {
    const httpClient = new RecordingHttpClient(async () => new Response(JSON.stringify({ path: "/workspace/app", usable: true, reason: "" }), {
        status: 200,
    }));
    const repository = new runnerAdminRepository_1.RunnerAdminRepository(httpClient, "http://127.0.0.1:4317");
    const result = await repository.validatePath("/workspace/app");
    strict_1.default.deepEqual(result, { path: "/workspace/app", usable: true, reason: "" });
    strict_1.default.equal(httpClient.requests[0]?.url, "http://127.0.0.1:4317/directories/validate");
    strict_1.default.equal(httpClient.requests[0]?.init?.method, "POST");
});
(0, node_test_1.default)("RunnerAdminRepository starts provider auth through runner endpoint", async () => {
    const httpClient = new RecordingHttpClient(async () => new Response("", { status: 200 }));
    const repository = new runnerAdminRepository_1.RunnerAdminRepository(httpClient, "http://127.0.0.1:4317");
    await repository.authenticateProvider("codex");
    strict_1.default.equal(httpClient.requests[0]?.url, "http://127.0.0.1:4317/providers/auth");
    strict_1.default.equal(httpClient.requests[0]?.init?.method, "POST");
});
