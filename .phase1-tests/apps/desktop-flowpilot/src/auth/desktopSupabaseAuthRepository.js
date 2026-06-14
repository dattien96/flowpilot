"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.DesktopSupabaseAuthRepository = void 0;
const supabase_js_1 = require("@supabase/supabase-js");
const desktopBridgeHttp_1 = require("./desktopBridgeHttp");
const AUTH_SESSION_STORAGE_KEY = "flowpilot.desktop.supabase-auth-session";
function mapAuthSession(session) {
    if (!session?.user) {
        return null;
    }
    return {
        userId: session.user.id,
        email: session.user.email ?? null,
    };
}
function isFetchFailure(error) {
    if (error instanceof TypeError && /Failed to fetch/i.test(error.message)) {
        return true;
    }
    if (typeof error === "object" &&
        error !== null &&
        "message" in error &&
        typeof error.message === "string") {
        return /Failed to fetch/i.test(error.message);
    }
    return false;
}
function toSupabaseReachabilityError() {
    return new Error("Unable to reach the configured Supabase project. Check the Supabase URL, network access, and local configuration.");
}
function mapPersistedAuthSession(session) {
    return {
        userId: session.userId,
        email: session.email ?? null,
    };
}
function toPersistedAuthSession(session, clientKey) {
    if (!session?.access_token || !session.refresh_token || !session.user?.id) {
        return null;
    }
    return {
        clientKey,
        accessToken: session.access_token,
        refreshToken: session.refresh_token,
        userId: session.user.id,
        email: session.user.email ?? null,
    };
}
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
async function saveBrowserPersistedAuthSession(session) {
    if (typeof window === "undefined" || !window.localStorage) {
        return;
    }
    try {
        window.localStorage.setItem(AUTH_SESSION_STORAGE_KEY, JSON.stringify(session));
    }
    catch {
        // Ignore dev-tab storage failures and keep the in-memory session alive.
    }
}
async function clearBrowserPersistedAuthSession() {
    if (typeof window === "undefined" || !window.localStorage) {
        return;
    }
    try {
        window.localStorage.removeItem(AUTH_SESSION_STORAGE_KEY);
    }
    catch {
        // Ignore dev-tab storage failures and keep the app responsive.
    }
}
function getFlowpilotBridge() {
    const candidate = globalThis;
    return candidate.window?.flowpilot;
}
class DesktopSupabaseAuthRepository {
    runtimeConfigRepository;
    httpClient;
    runnerBaseUrl;
    client = null;
    clientKey = null;
    fallbackSession = null;
    constructor(runtimeConfigRepository, httpClient, runnerBaseUrl) {
        this.runtimeConfigRepository = runtimeConfigRepository;
        this.httpClient = httpClient;
        this.runnerBaseUrl = runnerBaseUrl;
    }
    async getSession() {
        const supabase = await this.getClient();
        if (!supabase)
            return null;
        const clientKey = this.clientKey;
        try {
            const { data, error } = await supabase.auth.getSession();
            if (error) {
                throw error;
            }
            const session = mapAuthSession(data.session);
            if (session) {
                this.fallbackSession = session;
                const persistedSession = toPersistedAuthSession(data.session, clientKey ?? "");
                if (persistedSession) {
                    await this.savePersistedAuthSession(persistedSession);
                }
                return session;
            }
            const restoredSession = await this.restorePersistedSession(supabase, clientKey);
            return restoredSession ?? this.fallbackSession;
        }
        catch (error) {
            if (isFetchFailure(error)) {
                return this.fallbackSession ?? (await this.loadPersistedSessionIdentity(clientKey));
            }
            throw error;
        }
    }
    async loginWithPassword(email, password) {
        const supabase = await this.getClient(true);
        if (!supabase) {
            throw new Error("Supabase database is not configured. Please configure this PC first.");
        }
        let session = null;
        try {
            const response = await supabase.auth.signInWithPassword({ email, password });
            if (response.error) {
                throw response.error;
            }
            session = mapAuthSession(response.data.session);
            const persistedSession = toPersistedAuthSession(response.data.session, this.clientKey ?? this.clientIdentityFromClient(supabase));
            if (persistedSession) {
                await this.savePersistedAuthSession(persistedSession);
            }
        }
        catch (error) {
            if (isFetchFailure(error)) {
                const payload = await this.loginThroughRunner(email, password);
                session = await this.persistRunnerSession(supabase, payload);
            }
            else {
                throw error;
            }
        }
        if (!session) {
            throw new Error("Supabase returned no session.");
        }
        this.fallbackSession = session;
        return session;
    }
    async logout() {
        const supabase = await this.getClient();
        if (!supabase)
            return;
        try {
            const { error } = await supabase.auth.signOut();
            if (error) {
                throw error;
            }
        }
        catch (error) {
            if (isFetchFailure(error)) {
                this.fallbackSession = null;
                await this.clearPersistedAuthSession();
                return;
            }
            throw error;
        }
        this.fallbackSession = null;
        await this.clearPersistedAuthSession();
    }
    resetSessionState() {
        this.client = null;
        this.clientKey = null;
        this.fallbackSession = null;
    }
    async getClient(requireConfigured = false) {
        const runtimeStatus = await this.runtimeConfigRepository.loadSupabaseRuntimeStatus();
        if (!runtimeStatus.configured || !runtimeStatus.apiUrl || !runtimeStatus.anonKey) {
            if (requireConfigured) {
                throw new Error("Supabase database is not configured. Please configure this PC first.");
            }
            return null;
        }
        const nextKey = this.clientIdentity(runtimeStatus);
        if (!this.client || this.clientKey !== nextKey) {
            this.client = (0, supabase_js_1.createClient)(runtimeStatus.apiUrl, runtimeStatus.anonKey, {
                global: { fetch: desktopBridgeHttp_1.desktopBridgeFetch },
            });
            this.clientKey = nextKey;
        }
        return this.client;
    }
    clientIdentity(runtimeStatus) {
        return `${runtimeStatus.apiUrl ?? ""}::${runtimeStatus.anonKey ?? ""}`;
    }
    async loginThroughRunner(email, password) {
        let response;
        try {
            response = await this.httpClient.request(new URL("/supabase-auth/login", this.runnerBaseUrl), {
                method: "POST",
                headers: { "content-type": "application/json" },
                body: JSON.stringify({ email, password }),
            });
        }
        catch (error) {
            if (isFetchFailure(error)) {
                throw new Error("Unable to reach the local runner for Supabase login fallback.");
            }
            throw error;
        }
        if (!response.ok) {
            const text = await response.text();
            throw new Error(text || "Runner-side Supabase login failed.");
        }
        const payload = (await response.json());
        return payload;
    }
    // Keep bootstrap-visible auth state even when direct renderer-side Supabase auth is unavailable.
    async persistRunnerSession(supabase, payload) {
        try {
            const { data, error } = await supabase.auth.setSession({
                access_token: payload.accessToken,
                refresh_token: payload.refreshToken,
            });
            if (error) {
                throw error;
            }
            const session = mapAuthSession(data.session);
            if (session) {
                await this.savePersistedAuthSession({
                    ...payload,
                    clientKey: this.clientKey ?? this.clientIdentityFromClient(supabase),
                });
                return session;
            }
        }
        catch {
            // Fall back to the runner-issued identity so the desktop bootstrap can continue.
        }
        await this.savePersistedAuthSession({
            ...payload,
            clientKey: this.clientKey ?? this.clientIdentityFromClient(supabase),
        });
        return {
            userId: payload.userId,
            email: payload.email ?? null,
        };
    }
    clientIdentityFromClient(_supabase) {
        return this.clientKey ?? "";
    }
    async restorePersistedSession(supabase, clientKey) {
        const persistedSession = await this.loadPersistedAuthSession(clientKey);
        if (!persistedSession) {
            return null;
        }
        const session = await this.persistRunnerSession(supabase, persistedSession);
        this.fallbackSession = session;
        return session;
    }
    async loadPersistedSessionIdentity(clientKey) {
        const persistedSession = await this.loadPersistedAuthSession(clientKey);
        return persistedSession ? mapPersistedAuthSession(persistedSession) : null;
    }
    async loadPersistedAuthSession(clientKey) {
        const bridge = getFlowpilotBridge();
        const persistedSession = bridge?.loadAuthSession
            ? await bridge.loadAuthSession()
            : await loadBrowserPersistedAuthSession();
        if (!persistedSession || (clientKey && persistedSession.clientKey !== clientKey)) {
            return null;
        }
        return persistedSession;
    }
    async savePersistedAuthSession(session) {
        const bridge = getFlowpilotBridge();
        if (bridge?.saveAuthSession) {
            await bridge.saveAuthSession(session);
            return;
        }
        await saveBrowserPersistedAuthSession(session);
    }
    async clearPersistedAuthSession() {
        const bridge = getFlowpilotBridge();
        if (bridge?.clearAuthSession) {
            await bridge.clearAuthSession();
            return;
        }
        await clearBrowserPersistedAuthSession();
    }
}
exports.DesktopSupabaseAuthRepository = DesktopSupabaseAuthRepository;
