"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.createDesktopAdminSupabaseClient = createDesktopAdminSupabaseClient;
const supabase_js_1 = require("@supabase/supabase-js");
const desktopBridgeHttp_1 = require("./desktopBridgeHttp");
const AUTH_SESSION_STORAGE_KEY = "flowpilot.desktop.supabase-auth-session";
async function loadBrowserPersistedAuthSession() {
    if (typeof window === "undefined" || !window.localStorage) {
        return null;
    }
    try {
        const raw = window.localStorage.getItem(AUTH_SESSION_STORAGE_KEY);
        return raw ? JSON.parse(raw) : null;
    }
    catch {
        return null;
    }
}
async function loadPersistedAuthSession(clientKey) {
    const candidate = globalThis;
    const persisted = candidate.window?.flowpilot?.loadAuthSession
        ? await candidate.window.flowpilot.loadAuthSession()
        : await loadBrowserPersistedAuthSession();
    if (!persisted || persisted.clientKey !== clientKey) {
        return null;
    }
    return persisted;
}
function clientKeyFor(apiUrl, anonKey) {
    return `${apiUrl}::${anonKey}`;
}
async function createDesktopAdminSupabaseClient(apiUrl, anonKey) {
    const persisted = await loadPersistedAuthSession(clientKeyFor(apiUrl, anonKey));
    if (!persisted?.accessToken) {
        return (0, supabase_js_1.createClient)(apiUrl, anonKey, {
            global: { fetch: desktopBridgeHttp_1.desktopBridgeFetch },
        });
    }
    return (0, supabase_js_1.createClient)(apiUrl, anonKey, {
        accessToken: async () => persisted.accessToken,
        global: { fetch: desktopBridgeHttp_1.desktopBridgeFetch },
    });
}
