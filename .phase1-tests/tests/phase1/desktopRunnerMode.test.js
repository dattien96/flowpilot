"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const node_fs_1 = require("node:fs");
const node_path_1 = require("node:path");
const createRunnerClient_1 = require("../../apps/desktop-flowpilot/src/client/createRunnerClient");
const MockRunnerClient_1 = require("../../apps/desktop-flowpilot/src/client/MockRunnerClient");
const HttpWsRunnerClient_1 = require("../../apps/desktop-flowpilot/src/client/HttpWsRunnerClient");
const config_1 = require("../../apps/desktop-flowpilot/src/config");
function withRunnerEnv(env, run) {
    const candidate = globalThis;
    const previousViteEnv = candidate.__FLOWPILOT_VITE_ENV__;
    const previousRunnerUrl = process.env.VITE_RUNNER_URL;
    const previousUseRunner = process.env.VITE_USE_RUNNER;
    candidate.__FLOWPILOT_VITE_ENV__ = env;
    if (env.VITE_RUNNER_URL === undefined)
        delete process.env.VITE_RUNNER_URL;
    else
        process.env.VITE_RUNNER_URL = env.VITE_RUNNER_URL;
    if (env.VITE_USE_RUNNER === undefined)
        delete process.env.VITE_USE_RUNNER;
    else
        process.env.VITE_USE_RUNNER = env.VITE_USE_RUNNER;
    try {
        run();
    }
    finally {
        if (previousViteEnv === undefined)
            delete candidate.__FLOWPILOT_VITE_ENV__;
        else
            candidate.__FLOWPILOT_VITE_ENV__ = previousViteEnv;
        if (previousRunnerUrl === undefined)
            delete process.env.VITE_RUNNER_URL;
        else
            process.env.VITE_RUNNER_URL = previousRunnerUrl;
        if (previousUseRunner === undefined)
            delete process.env.VITE_USE_RUNNER;
        else
            process.env.VITE_USE_RUNNER = previousUseRunner;
    }
}
(0, node_test_1.default)("resolveRunnerUrlFromSources prefers renderer Vite env over process env", () => {
    const url = (0, createRunnerClient_1.resolveRunnerUrlFromSources)({ VITE_RUNNER_URL: "http://renderer-runner:4000" }, { VITE_RUNNER_URL: "http://process-runner:5000", VITE_USE_RUNNER: "true" });
    strict_1.default.equal(url, "http://renderer-runner:4000");
});
(0, node_test_1.default)("resolveRunnerUrlFromSources falls back to process env when renderer env is missing", () => {
    const url = (0, createRunnerClient_1.resolveRunnerUrlFromSources)({}, { VITE_USE_RUNNER: "true" });
    strict_1.default.equal(url, config_1.RUNNER_URL);
});
(0, node_test_1.default)("createRunnerClient uses HttpWsRunnerClient when the renderer Vite env enables the runner", () => {
    withRunnerEnv({ VITE_USE_RUNNER: "true", VITE_RUNNER_URL: undefined }, () => {
        const client = (0, createRunnerClient_1.createRunnerClient)();
        strict_1.default.ok(client instanceof HttpWsRunnerClient_1.HttpWsRunnerClient);
        strict_1.default.equal((0, createRunnerClient_1.runnerModeLabel)(), `runner ${config_1.RUNNER_URL}`);
    });
});
(0, node_test_1.default)("createRunnerClient uses MockRunnerClient when neither env source enables the runner", () => {
    withRunnerEnv({ VITE_USE_RUNNER: undefined, VITE_RUNNER_URL: undefined }, () => {
        const client = (0, createRunnerClient_1.createRunnerClient)();
        strict_1.default.ok(client instanceof MockRunnerClient_1.MockRunnerClient);
        strict_1.default.equal((0, createRunnerClient_1.runnerModeLabel)(), "mock");
    });
});
(0, node_test_1.default)("desktop main publishes Vite env before importing App and store modules", () => {
    const mainSource = (0, node_fs_1.readFileSync)((0, node_path_1.resolve)(__dirname, "../../../apps/desktop-flowpilot/src/main.tsx"), "utf8");
    strict_1.default.equal(mainSource.includes('import { App } from "@/App"'), false);
    strict_1.default.ok(mainSource.indexOf("__FLOWPILOT_VITE_ENV__") < mainSource.indexOf('import("@/App")'));
});
