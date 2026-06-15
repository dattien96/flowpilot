"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const desktopAdminSupabaseClient_1 = require("../../apps/desktop-flowpilot/src/auth/desktopAdminSupabaseClient");
(0, node_test_1.default)("createDesktopAdminSupabaseClient reuses persisted desktop auth token for settings queries", async () => {
    const originalWindow = globalThis.window;
    const persistedSession = {
        clientKey: "https://proj.supabase.co::anon",
        accessToken: "access-token",
        refreshToken: "refresh-token",
        userId: "user-1",
        email: "user@example.com",
    };
    globalThis.window = {
        flowpilot: {
            loadAuthSession: async () => persistedSession,
        },
    };
    try {
        const client = await (0, desktopAdminSupabaseClient_1.createDesktopAdminSupabaseClient)("https://proj.supabase.co", "anon");
        strict_1.default.equal(await client._getAccessToken(), "access-token");
    }
    finally {
        globalThis.window = originalWindow;
    }
});
(0, node_test_1.default)("createDesktopAdminSupabaseClient falls back to anon key when no matching desktop session exists", async () => {
    const originalWindow = globalThis.window;
    globalThis.window = {
        flowpilot: {
            loadAuthSession: async () => ({
                clientKey: "https://other.supabase.co::anon",
                accessToken: "wrong-token",
                refreshToken: "wrong-refresh",
                userId: "user-2",
            }),
        },
    };
    try {
        const client = await (0, desktopAdminSupabaseClient_1.createDesktopAdminSupabaseClient)("https://proj.supabase.co", "anon");
        strict_1.default.equal(await client._getAccessToken(), "anon");
    }
    finally {
        globalThis.window = originalWindow;
    }
});
