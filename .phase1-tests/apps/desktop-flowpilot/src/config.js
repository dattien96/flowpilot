"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.LIBRETRANSLATE_URL = exports.RUNNER_URL = exports.ADMIN_WEB_URL = void 0;
function currentEnv() {
    const candidate = globalThis;
    if (candidate.__FLOWPILOT_VITE_ENV__) {
        return candidate.__FLOWPILOT_VITE_ENV__;
    }
    return (typeof process !== "undefined" ? process.env : {});
}
function trimTrailingSlash(value) {
    return value.replace(/\/+$/, "");
}
exports.ADMIN_WEB_URL = trimTrailingSlash(currentEnv().VITE_ADMIN_WEB_URL ?? "http://localhost:3002");
exports.RUNNER_URL = trimTrailingSlash(currentEnv().VITE_RUNNER_URL ??
    currentEnv().VITE_LOCAL_RUNNER_URL ??
    "http://127.0.0.1:4317");
exports.LIBRETRANSLATE_URL = trimTrailingSlash(currentEnv().FLOWPILOT_LIBRETRANSLATE_URL ??
    (currentEnv().FLOWPILOT_LIBRETRANSLATE_PORT
        ? `http://127.0.0.1:${currentEnv().FLOWPILOT_LIBRETRANSLATE_PORT}`
        : "http://127.0.0.1:5001"));
