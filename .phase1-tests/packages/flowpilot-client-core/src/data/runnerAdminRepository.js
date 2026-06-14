"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.CompositeIntegrationRepository = exports.CompositeProviderRepository = exports.CompositeArtifactRepository = exports.RunnerAdminRepository = void 0;
async function readJson(response) {
    if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Request failed with status ${response.status}.`);
    }
    return (await response.json());
}
class RunnerAdminRepository {
    httpClient;
    runnerBaseUrl;
    constructor(httpClient, runnerBaseUrl) {
        this.httpClient = httpClient;
        this.runnerBaseUrl = runnerBaseUrl;
    }
    async listLocalProviders() {
        const response = await this.httpClient.request(new URL("/providers", this.runnerBaseUrl), { cache: "no-store" });
        return readJson(response).catch(() => []);
    }
    async authenticateProvider(providerKey) {
        const response = await this.httpClient.request(new URL("/providers/auth", this.runnerBaseUrl), {
            method: "POST",
            headers: { "content-type": "application/json" },
            body: JSON.stringify({ providerName: providerKey }),
        });
        await readJson(response).catch(async (error) => {
            if (error instanceof SyntaxError)
                return;
            throw error;
        });
    }
    async listLocalArtifacts() {
        const response = await this.httpClient.request(new URL("/artifacts", this.runnerBaseUrl), { cache: "no-store" });
        return readJson(response).catch(() => []);
    }
    async getStorageDriver() {
        const response = await this.httpClient.request(new URL("/storage-driver", this.runnerBaseUrl), { cache: "no-store" });
        return readJson(response).catch(() => ({
            driverKey: "filesystem",
            enabled: false,
            remoteRootPath: "",
            remoteFolderName: "FlowPilot",
            lastValidatedAt: null,
            lastSyncedAt: null,
            lastError: null,
            updatedAt: null,
        }));
    }
    async saveStorageDriver(driver) {
        const response = await this.httpClient.request(new URL("/storage-driver", this.runnerBaseUrl), {
            method: "PUT",
            headers: { "content-type": "application/json" },
            body: JSON.stringify(driver),
        });
        return readJson(response);
    }
    async listMcpBackends() {
        const response = await this.httpClient.request(new URL("/mcp/backends", this.runnerBaseUrl), { cache: "no-store" });
        return readJson(response).catch(() => []);
    }
    async runMcpBackendAction(backendKey, action, projectId, integrationId) {
        const path = action === "install" ? `/mcp/backends/${backendKey}/install` : `/mcp/backends/${backendKey}/actions`;
        const response = await this.httpClient.request(new URL(path, this.runnerBaseUrl), {
            method: "POST",
            headers: { "content-type": "application/json" },
            body: JSON.stringify({ action, projectId, integrationId }),
        });
        await readJson(response).catch(async (error) => {
            if (error instanceof SyntaxError)
                return;
            throw error;
        });
    }
    async testIntegration(projectId, integrationId, providerType, fields) {
        const response = await this.httpClient.request(new URL("/integrations/connect", this.runnerBaseUrl), {
            method: "POST",
            headers: { "content-type": "application/json" },
            body: JSON.stringify({
                projectId,
                integrationId,
                providerType,
                action: "test",
                ...fields,
            }),
        });
        const payload = await readJson(response);
        return payload.message ?? null;
    }
    async validatePath(path) {
        const response = await this.httpClient.request(new URL("/directories/validate", this.runnerBaseUrl), {
            method: "POST",
            cache: "no-store",
            headers: { "content-type": "application/json" },
            body: JSON.stringify({ path }),
        });
        return readJson(response);
    }
}
exports.RunnerAdminRepository = RunnerAdminRepository;
class CompositeArtifactRepository {
    supabaseRepository;
    runnerRepository;
    constructor(supabaseRepository, runnerRepository) {
        this.supabaseRepository = supabaseRepository;
        this.runnerRepository = runnerRepository;
    }
    listDefinitions() { return this.supabaseRepository.listDefinitions(); }
    saveDefinition(definition) { return this.supabaseRepository.saveDefinition(definition); }
    listRuns(projectId) { return this.supabaseRepository.listRuns(projectId); }
    listLocalArtifacts() { return this.runnerRepository.listLocalArtifacts(); }
    getStorageDriver() { return this.runnerRepository.getStorageDriver(); }
    saveStorageDriver(driver) {
        return this.runnerRepository.saveStorageDriver(driver);
    }
}
exports.CompositeArtifactRepository = CompositeArtifactRepository;
class CompositeProviderRepository {
    supabaseRepository;
    runnerRepository;
    constructor(supabaseRepository, runnerRepository) {
        this.supabaseRepository = supabaseRepository;
        this.runnerRepository = runnerRepository;
    }
    listLocalProviders() { return this.runnerRepository.listLocalProviders(); }
    authenticateProvider(providerKey) { return this.runnerRepository.authenticateProvider(providerKey); }
    listSupportedModels() { return this.supabaseRepository.listSupportedModels(); }
    createSupportedModel(model) { return this.supabaseRepository.createSupportedModel(model); }
    updateSupportedModel(id, patch) { return this.supabaseRepository.updateSupportedModel(id, patch); }
    deleteSupportedModel(id) { return this.supabaseRepository.deleteSupportedModel(id); }
}
exports.CompositeProviderRepository = CompositeProviderRepository;
class CompositeIntegrationRepository {
    supabaseRepository;
    runnerRepository;
    constructor(supabaseRepository, runnerRepository) {
        this.supabaseRepository = supabaseRepository;
        this.runnerRepository = runnerRepository;
    }
    listIntegrations() { return this.supabaseRepository.listIntegrations(); }
    createIntegration(input) {
        return this.supabaseRepository.createIntegration(input);
    }
    updateIntegration(id, patch) { return this.supabaseRepository.updateIntegration(id, patch); }
    listLinkedIntegrations(projectId) { return this.supabaseRepository.listLinkedIntegrations(projectId); }
    setProjectIntegration(projectId, type, integrationId) {
        return this.supabaseRepository.setProjectIntegration(projectId, type, integrationId);
    }
    listMcpBackends() { return this.runnerRepository.listMcpBackends(); }
    runMcpBackendAction(backendKey, action, projectId, integrationId) {
        return this.runnerRepository.runMcpBackendAction(backendKey, action, projectId, integrationId);
    }
    testIntegration(projectId, integrationId, providerType, fields) {
        return this.runnerRepository.testIntegration(projectId, integrationId, providerType, fields);
    }
}
exports.CompositeIntegrationRepository = CompositeIntegrationRepository;
