"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.resolveRunnerUrlFromSources = resolveRunnerUrlFromSources;
exports.getRunnerBaseUrl = getRunnerBaseUrl;
exports.createRunnerClient = createRunnerClient;
exports.isMockMode = isMockMode;
exports.runnerModeLabel = runnerModeLabel;
const MockRunnerClient_1 = require("./MockRunnerClient");
const HttpWsRunnerClient_1 = require("./HttpWsRunnerClient");
const config_1 = require("../config");
// Selects the transport behind the RunnerClient contract (04-01 Part B):
//   - VITE_RUNNER_URL set  -> HttpWsRunnerClient against that runner
//   - VITE_LOCAL_RUNNER_URL set -> HttpWsRunnerClient against that runner
//   - VITE_USE_RUNNER=1     -> HttpWsRunnerClient against the default RUNNER_URL
//   - otherwise            -> MockRunnerClient (offline / UI dev)
// The renderer never references a concrete client — only this factory does.
function resolveRunnerUrlFromSources(viteEnv = {}, processEnv = {}) {
    if (viteEnv.VITE_RUNNER_URL)
        return viteEnv.VITE_RUNNER_URL;
    if (viteEnv.VITE_LOCAL_RUNNER_URL)
        return viteEnv.VITE_LOCAL_RUNNER_URL;
    if (viteEnv.VITE_USE_RUNNER === "1" || viteEnv.VITE_USE_RUNNER === "true")
        return config_1.RUNNER_URL;
    if (processEnv.VITE_RUNNER_URL)
        return processEnv.VITE_RUNNER_URL;
    if (processEnv.VITE_LOCAL_RUNNER_URL)
        return processEnv.VITE_LOCAL_RUNNER_URL;
    if (processEnv.VITE_USE_RUNNER === "1" || processEnv.VITE_USE_RUNNER === "true")
        return config_1.RUNNER_URL;
    return null;
}
function resolvedRunnerUrl() {
    const candidate = globalThis;
    const viteEnv = candidate.__FLOWPILOT_VITE_ENV__ ?? {};
    const processEnv = (typeof process !== "undefined" ? process.env : {});
    return resolveRunnerUrlFromSources(viteEnv, processEnv);
}
function getRunnerBaseUrl() {
    return resolvedRunnerUrl();
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
