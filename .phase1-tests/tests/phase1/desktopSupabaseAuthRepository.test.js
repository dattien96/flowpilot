"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const desktopSupabaseAuthRepository_1 = require("../../apps/desktop-flowpilot/src/auth/desktopSupabaseAuthRepository");
const runtimeStatus = {
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
};
class FakeRuntimeConfigRepository {
    loadSupabaseRuntimeStatus() {
        return Promise.resolve(runtimeStatus);
    }
    validateSupabaseConfig(_input) {
        throw new Error("Not implemented in test.");
    }
    saveSupabaseConfig(_input) {
        throw new Error("Not implemented in test.");
    }
}
class FakeHttpClient {
    handler;
    constructor(handler) {
        this.handler = handler;
    }
    request(input, init) {
        return this.handler(input, init);
    }
}
(0, node_test_1.default)("DesktopSupabaseAuthRepository keeps fallback session visible to bootstrap after runner login", async () => {
    const originalWindow = globalThis.window;
    let persistedSession = null;
    globalThis.window = {
        flowpilot: {
            loadAuthSession: async () => persistedSession,
            saveAuthSession: async (payload) => {
                persistedSession = payload;
                return { ok: true };
            },
            clearAuthSession: async () => {
                persistedSession = null;
                return { ok: true };
            },
        },
    };
    try {
        const repository = new desktopSupabaseAuthRepository_1.DesktopSupabaseAuthRepository(new FakeRuntimeConfigRepository(), new FakeHttpClient(async () => new Response(JSON.stringify({
            accessToken: "access-token",
            refreshToken: "refresh-token",
            userId: "user-1",
            email: "user@example.com",
        }), { status: 200 })), "http://127.0.0.1:4317");
        const authCalls = {
            signInWithPassword: [],
            setSession: [],
            getSession: 0,
        };
        const supabaseStub = {
            auth: {
                signInWithPassword: async (credentials) => {
                    authCalls.signInWithPassword.push(credentials);
                    throw new TypeError("Failed to fetch");
                },
                setSession: async (tokens) => {
                    authCalls.setSession.push(tokens);
                    return { data: { session: null }, error: null };
                },
                getSession: async () => {
                    authCalls.getSession += 1;
                    return { data: { session: null }, error: null };
                },
            },
        };
        const repositoryState = repository;
        repositoryState.client = supabaseStub;
        repositoryState.clientKey = `${runtimeStatus.apiUrl ?? ""}::${runtimeStatus.anonKey ?? ""}`;
        const loginSession = await repository.loginWithPassword("user@example.com", "secret");
        const restartedRepository = new desktopSupabaseAuthRepository_1.DesktopSupabaseAuthRepository(new FakeRuntimeConfigRepository(), new FakeHttpClient(async () => {
            throw new Error("Runner should not be called when persisted auth session can be restored.");
        }), "http://127.0.0.1:4317");
        const restartedSupabaseStub = {
            auth: {
                getSession: async () => {
                    authCalls.getSession += 1;
                    return { data: { session: null }, error: null };
                },
                setSession: async (tokens) => {
                    authCalls.setSession.push(tokens);
                    return {
                        data: {
                            session: {
                                access_token: tokens.access_token,
                                refresh_token: tokens.refresh_token,
                                user: { id: "user-1", email: "user@example.com" },
                            },
                        },
                        error: null,
                    };
                },
            },
        };
        const restartedRepositoryState = restartedRepository;
        restartedRepositoryState.client = restartedSupabaseStub;
        restartedRepositoryState.clientKey = `${runtimeStatus.apiUrl ?? ""}::${runtimeStatus.anonKey ?? ""}`;
        const bootstrapSession = await restartedRepository.getSession();
        strict_1.default.deepEqual(loginSession, {
            userId: "user-1",
            email: "user@example.com",
        });
        strict_1.default.deepEqual(bootstrapSession, loginSession);
        strict_1.default.deepEqual(authCalls.signInWithPassword, [
            { email: "user@example.com", password: "secret" },
        ]);
        strict_1.default.deepEqual(authCalls.setSession, [
            { access_token: "access-token", refresh_token: "refresh-token" },
            { access_token: "access-token", refresh_token: "refresh-token" },
        ]);
        strict_1.default.equal(authCalls.getSession, 1);
        strict_1.default.deepEqual(persistedSession, {
            clientKey: "https://proj.supabase.co::anon",
            accessToken: "access-token",
            refreshToken: "refresh-token",
            userId: "user-1",
            email: "user@example.com",
        });
    }
    finally {
        globalThis.window = originalWindow;
    }
});
