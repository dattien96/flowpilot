"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.createRunnerClient = createRunnerClient;
exports.isMockMode = isMockMode;
exports.runnerModeLabel = runnerModeLabel;
const MockRunnerClient_1 = require("./MockRunnerClient");
const HttpWsRunnerClient_1 = require("./HttpWsRunnerClient");
const config_1 = require("@/config");
// Selects the transport behind the RunnerClient contract (04-01 Part B):
//   - VITE_RUNNER_URL set  -> HttpWsRunnerClient against that runner
//   - VITE_USE_RUNNER=1     -> HttpWsRunnerClient against the default RUNNER_URL
//   - otherwise            -> MockRunnerClient (offline / UI dev)
// The renderer never references a concrete client — only this factory does.
function resolvedRunnerUrl() {
    const env = (typeof process !== "undefined" ? process.env : {});
    if (env.VITE_RUNNER_URL)
        return env.VITE_RUNNER_URL;
    if (env.VITE_USE_RUNNER === "1" || env.VITE_USE_RUNNER === "true")
        return config_1.RUNNER_URL;
    return null;
}
function createRunnerClient() {
    const url = resolvedRunnerUrl();
    return url ? new HttpWsRunnerClient_1.HttpWsRunnerClient(url) : new MockRunnerClient_1.MockRunnerClient();
}
/** True when the offline mock client is in use (no runner URL configured). */
function isMockMode() {
    return resolvedRunnerUrl() === null;
}
/** Header label: "mock" or "runner <url>" — so the active transport is visible. */
function runnerModeLabel() {
    const url = resolvedRunnerUrl();
    return url ? `runner ${url}` : "mock";
}
