"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.RunnerRuntimeConfigRepository = void 0;
function deriveProjectRef(apiUrl) {
    try {
        const host = new URL(apiUrl).host.toLowerCase();
        return host.replace(/\.supabase\.co$/, "").split(".")[0] || null;
    }
    catch {
        return null;
    }
}
async function readError(response) {
    const text = await response.text();
    if (!text)
        return `Request failed with status ${response.status}.`;
    try {
        const payload = JSON.parse(text);
        return payload.error || text;
    }
    catch {
        return text;
    }
}
class RunnerRuntimeConfigRepository {
    httpClient;
    runnerBaseUrl;
    constructor(httpClient, runnerBaseUrl) {
        this.httpClient = httpClient;
        this.runnerBaseUrl = runnerBaseUrl;
    }
    async loadSupabaseRuntimeStatus() {
        try {
            const response = await this.httpClient.request(new URL("/supabase-config", this.runnerBaseUrl), { cache: "no-store" });
            if (response.ok) {
                const payload = (await response.json());
                if (payload.apiUrl && payload.anonKey) {
                    return {
                        mode: "config",
                        configured: true,
                        apiUrl: payload.apiUrl,
                        anonKey: payload.anonKey,
                        edgeFunctionUrl: payload.edgeFunctionUrl ?? null,
                        hasServiceRoleKey: Boolean(payload.hasServiceRoleKey),
                        projectRef: payload.projectRef ?? deriveProjectRef(payload.apiUrl),
                        runnerReachable: true,
                        envAvailable: false,
                        savedConfigAvailable: true,
                        edgeFunctionsReady: Boolean(payload.edgeFunctionUrl),
                        lastError: null,
                    };
                }
                return this.demoStatus(true, "Saved Supabase config is incomplete.");
            }
            if (response.status === 404) {
                return this.demoStatus(true, null);
            }
            return this.demoStatus(true, await readError(response));
        }
        catch (error) {
            return this.demoStatus(false, error instanceof Error ? error.message : "Unable to reach local runner.");
        }
    }
    async validateSupabaseConfig(input) {
        const response = await this.httpClient.request(new URL("/supabase-config/validate", this.runnerBaseUrl), {
            method: "POST",
            headers: { "content-type": "application/json" },
            body: JSON.stringify(input),
        });
        if (!response.ok) {
            throw new Error(await readError(response));
        }
        return (await response.json());
    }
    async saveSupabaseConfig(input) {
        const response = await this.httpClient.request(new URL("/supabase-config", this.runnerBaseUrl), {
            method: "PUT",
            headers: { "content-type": "application/json" },
            body: JSON.stringify(input),
        });
        if (!response.ok) {
            throw new Error(await readError(response));
        }
        return this.loadSupabaseRuntimeStatus();
    }
    async loadSupabaseWorkspaceConfigWithSecret() {
        const response = await this.httpClient.request(new URL("/supabase-config?includeSecret=1", this.runnerBaseUrl), { cache: "no-store" });
        if (response.status === 404) {
            return null;
        }
        if (!response.ok) {
            throw new Error(await readError(response));
        }
        const payload = (await response.json());
        if (!payload.apiUrl || !payload.anonKey) {
            return null;
        }
        return {
            apiUrl: payload.apiUrl,
            anonKey: payload.anonKey,
            serviceRoleKey: payload.serviceRoleKey?.trim() || null,
        };
    }
    demoStatus(runnerReachable, lastError) {
        return {
            mode: "demo",
            configured: false,
            apiUrl: null,
            anonKey: null,
            edgeFunctionUrl: null,
            hasServiceRoleKey: false,
            projectRef: null,
            runnerReachable,
            envAvailable: false,
            savedConfigAvailable: false,
            edgeFunctionsReady: false,
            lastError,
        };
    }
}
exports.RunnerRuntimeConfigRepository = RunnerRuntimeConfigRepository;
