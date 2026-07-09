import type {
  ArtifactRepository,
  DirectoryRepository,
  IntegrationRepository,
  LocalArtifactRepository,
  LocalProviderRepository,
  McpBackendRepository,
  ProviderRepository,
  StorageDriverRepository,
} from "../domain/adminRepositories";
import type {
  Integration,
  IntegrationType,
  LocalRunnerArtifact,
  LocalRunnerMcpBackend,
  LocalRunnerProvider,
  LocalRunnerStorageDriver,
  SupportedModel,
} from "../domain/adminModels";
import type { HttpClient } from "./http";

async function readJson<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Request failed with status ${response.status}.`);
  }
  return (await response.json()) as T;
}

export class RunnerAdminRepository implements
  LocalProviderRepository,
  LocalArtifactRepository,
  StorageDriverRepository,
  McpBackendRepository,
  DirectoryRepository
{
  constructor(
    private readonly httpClient: HttpClient,
    private readonly runnerBaseUrl: string,
  ) {}

  async listLocalProviders(): Promise<LocalRunnerProvider[]> {
    const response = await this.httpClient.request(new URL("/providers", this.runnerBaseUrl), { cache: "no-store" });
    return readJson<LocalRunnerProvider[]>(response).catch(() => []);
  }

  async installLocalProvider(providerKey: string): Promise<LocalRunnerProvider[]> {
    const response = await this.httpClient.request(new URL("/providers/install", this.runnerBaseUrl), {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ providerName: providerKey }),
    });
    const payload = await readJson<{ providers: LocalRunnerProvider[] }>(response);
    return payload.providers ?? [];
  }

  async authenticateProvider(providerKey: string): Promise<void> {
    const response = await this.httpClient.request(new URL("/providers/auth", this.runnerBaseUrl), {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ providerName: providerKey }),
    });
    await readJson<unknown>(response).catch(async (error) => {
      if (error instanceof SyntaxError) return;
      throw error;
    });
  }

  async listLocalArtifacts(): Promise<LocalRunnerArtifact[]> {
    const response = await this.httpClient.request(new URL("/artifacts", this.runnerBaseUrl), { cache: "no-store" });
    return readJson<LocalRunnerArtifact[]>(response).catch(() => []);
  }

  async getStorageDriver(): Promise<LocalRunnerStorageDriver> {
    const response = await this.httpClient.request(new URL("/storage-driver", this.runnerBaseUrl), { cache: "no-store" });
    return readJson<LocalRunnerStorageDriver>(response).catch(() => ({
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

  async saveStorageDriver(driver: Pick<LocalRunnerStorageDriver, "driverKey" | "enabled" | "remoteRootPath" | "remoteFolderName">): Promise<LocalRunnerStorageDriver> {
    const response = await this.httpClient.request(new URL("/storage-driver", this.runnerBaseUrl), {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(driver),
    });
    return readJson<LocalRunnerStorageDriver>(response);
  }

  async listMcpBackends(): Promise<LocalRunnerMcpBackend[]> {
    const response = await this.httpClient.request(new URL("/mcp/backends", this.runnerBaseUrl), { cache: "no-store" });
    return readJson<LocalRunnerMcpBackend[]>(response).catch(() => []);
  }

  async runMcpBackendAction(backendKey: string, action: "install" | "verify", projectId: string, integrationId?: string): Promise<void> {
    const path = action === "install" ? `/mcp/backends/${backendKey}/install` : `/mcp/backends/${backendKey}/actions`;
    const response = await this.httpClient.request(new URL(path, this.runnerBaseUrl), {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ action, projectId, integrationId }),
    });
    await readJson<unknown>(response).catch(async (error) => {
      if (error instanceof SyntaxError) return;
      throw error;
    });
  }

  async testIntegration(projectId: string, integrationId: string, providerType: string, fields?: Record<string, string | undefined>): Promise<string | null> {
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
    const payload = await readJson<{ message?: string | null }>(response);
    return payload.message ?? null;
  }

  async pickDirectory() {
    const response = await this.httpClient.request(new URL("/directories/pick", this.runnerBaseUrl), {
      method: "POST",
      cache: "no-store",
    });
    return readJson<{ path: string }>(response);
  }

  async validatePath(path: string) {
    const response = await this.httpClient.request(new URL("/directories/validate", this.runnerBaseUrl), {
      method: "POST",
      cache: "no-store",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ path }),
    });
    return readJson<{ path: string; usable: boolean; reason: string }>(response);
  }
}

export class CompositeArtifactRepository implements ArtifactRepository {
  constructor(
    private readonly supabaseRepository: Pick<ArtifactRepository, "listRuns">,
    private readonly runnerRepository: Pick<ArtifactRepository, "listLocalArtifacts" | "getStorageDriver" | "saveStorageDriver">,
  ) {}

  listRuns(projectId?: string) { return this.supabaseRepository.listRuns(projectId); }
  listLocalArtifacts() { return this.runnerRepository.listLocalArtifacts(); }
  getStorageDriver() { return this.runnerRepository.getStorageDriver(); }
  saveStorageDriver(driver: Pick<LocalRunnerStorageDriver, "driverKey" | "enabled" | "remoteRootPath" | "remoteFolderName">) {
    return this.runnerRepository.saveStorageDriver(driver);
  }
}

export class CompositeProviderRepository implements ProviderRepository {
  constructor(
    private readonly supabaseRepository: Pick<ProviderRepository, "listSupportedModels" | "createSupportedModel" | "updateSupportedModel" | "deleteSupportedModel">,
    private readonly runnerRepository: Pick<ProviderRepository, "listLocalProviders" | "installLocalProvider" | "authenticateProvider">,
  ) {}

  listLocalProviders() { return this.runnerRepository.listLocalProviders(); }
  installLocalProvider(providerKey: string) { return this.runnerRepository.installLocalProvider(providerKey); }
  authenticateProvider(providerKey: string) { return this.runnerRepository.authenticateProvider(providerKey); }
  listSupportedModels() { return this.supabaseRepository.listSupportedModels(); }
  createSupportedModel(model: Omit<SupportedModel, "id" | "createdAt" | "updatedAt">) { return this.supabaseRepository.createSupportedModel(model); }
  updateSupportedModel(id: string, patch: Partial<SupportedModel>) { return this.supabaseRepository.updateSupportedModel(id, patch); }
  deleteSupportedModel(id: string) { return this.supabaseRepository.deleteSupportedModel(id); }
}

export class CompositeIntegrationRepository implements IntegrationRepository {
  constructor(
    private readonly supabaseRepository: Pick<IntegrationRepository, "listIntegrations" | "createIntegration" | "updateIntegration" | "listLinkedIntegrations" | "setProjectIntegration">,
    private readonly runnerRepository: Pick<IntegrationRepository, "listMcpBackends" | "runMcpBackendAction" | "testIntegration">,
  ) {}

  listIntegrations() { return this.supabaseRepository.listIntegrations(); }
  createIntegration(input: { projectId: string; type: IntegrationType; label: string; configEncrypted: Record<string, unknown>; status: string; mcpTypeEnabled?: boolean }) {
    return this.supabaseRepository.createIntegration(input);
  }
  updateIntegration(id: string, patch: Partial<Integration>) { return this.supabaseRepository.updateIntegration(id, patch); }
  listLinkedIntegrations(projectId: string) { return this.supabaseRepository.listLinkedIntegrations(projectId); }
  setProjectIntegration(projectId: string, type: IntegrationType, integrationId: string | null) {
    return this.supabaseRepository.setProjectIntegration(projectId, type, integrationId);
  }
  listMcpBackends() { return this.runnerRepository.listMcpBackends(); }
  runMcpBackendAction(backendKey: string, action: "install" | "verify", projectId: string, integrationId?: string) {
    return this.runnerRepository.runMcpBackendAction(backendKey, action, projectId, integrationId);
  }
  testIntegration(projectId: string, integrationId: string, providerType: string, fields?: Record<string, string | undefined>) {
    return this.runnerRepository.testIntegration(projectId, integrationId, providerType, fields);
  }
}
