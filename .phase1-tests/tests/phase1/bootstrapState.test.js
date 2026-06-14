"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const bootstrapState_1 = require("../../apps/desktop-flowpilot/src/app/bootstrapState");
function bootstrap(overrides) {
    return {
        runtimeStatus: {
            mode: "config",
            configured: true,
            apiUrl: "https://proj.supabase.co",
            anonKey: "anon",
            edgeFunctionUrl: "https://proj.supabase.co/functions/v1",
            hasServiceRoleKey: true,
            projectRef: "proj",
            runnerReachable: true,
            envAvailable: false,
            savedConfigAvailable: true,
            edgeFunctionsReady: true,
            lastError: null,
            ...(overrides.runtimeStatus ?? {}),
        },
        session: null,
        ...overrides,
    };
}
(0, node_test_1.default)("bootstrap resolves authenticated chat when session exists and runtime is healthy", () => {
    const result = (0, bootstrapState_1.resolveDesktopBootstrapState)(bootstrap({
        session: { userId: "user-1", email: "a@example.com" },
    }));
    strict_1.default.deepEqual(result, {
        issue: null,
        view: "authenticated-chat",
        preferredSettingsSection: "supabase",
    });
});
(0, node_test_1.default)("bootstrap resolves supabase invalid state explicitly", () => {
    const result = (0, bootstrapState_1.resolveDesktopBootstrapState)(bootstrap({
        runtimeStatus: {
            mode: "config",
            configured: false,
            apiUrl: null,
            anonKey: null,
            edgeFunctionUrl: null,
            hasServiceRoleKey: false,
            projectRef: null,
            runnerReachable: true,
            envAvailable: false,
            savedConfigAvailable: false,
            edgeFunctionsReady: false,
            lastError: "Saved Supabase config is incomplete.",
        },
    }));
    strict_1.default.deepEqual(result, {
        issue: "supabase-invalid",
        view: "settings",
        preferredSettingsSection: "supabase",
    });
});
(0, node_test_1.default)("bootstrap resolves runner offline to runner settings", () => {
    const result = (0, bootstrapState_1.resolveDesktopBootstrapState)(bootstrap({
        runtimeStatus: {
            mode: "config",
            configured: true,
            apiUrl: "https://proj.supabase.co",
            anonKey: "anon",
            edgeFunctionUrl: "https://proj.supabase.co/functions/v1",
            hasServiceRoleKey: true,
            projectRef: "proj",
            runnerReachable: false,
            envAvailable: false,
            savedConfigAvailable: true,
            edgeFunctionsReady: true,
            lastError: "Unable to reach local runner.",
        },
        session: { userId: "user-1", email: "a@example.com" },
    }));
    strict_1.default.deepEqual(result, {
        issue: "runner-offline",
        view: "settings",
        preferredSettingsSection: "runner",
    });
});
