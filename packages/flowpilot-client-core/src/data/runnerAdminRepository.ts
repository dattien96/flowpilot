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
  LocalRunnerProviderModel,
  LocalRunnerStorageDriver,
  SupportedModel,
  TelegramApprovalRecord,
} from "../domain/adminModels";
import type { HttpClient } from "./http";

async function readJson<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Request failed with status ${response.status}.`);
  }
  return (await response.json()) as T;
}

// Task-213: the Go runner's Provider/ProviderModel DTOs use a mix of
// snake_case (detected_binary/detected_version/models[].display_name) and
// pre-existing camelCase (binaryPath/installHint) JSON tags. readJson casts
// the raw response with no transform, so this repository maps the wire shape
// into LocalRunnerProvider explicitly (mirroring apps/admin-web's
// local-runner-mappers.ts) instead of silently dropping detected_version/models.
interface RawLocalRunnerProviderModel {
  id: string;
  display_name: string;
  available: boolean;
  source: string;
  // Task-215: see LocalRunnerProviderModel/ProviderModel (Go).
  supported_reasoning_efforts?: string[];
  default_reasoning_effort?: string;
  context_window_tokens?: number;
  max_context_window_tokens?: number;
}

interface RawLocalRunnerProvider {
  key: string;
  label: string;
  installed: boolean;
  version: string | null;
  auth_status?: string;
  installHint: string | null;
  detected_binary?: string | null;
  detected_version?: string | null;
  models?: RawLocalRunnerProviderModel[];
}

function mapLocalRunnerProviderModel(raw: RawLocalRunnerProviderModel): LocalRunnerProviderModel {
  return {
    id: raw.id,
    displayName: raw.display_name,
    available: raw.available,
    source: raw.source,
    supportedReasoningEfforts: raw.supported_reasoning_efforts,
    defaultReasoningEffort: raw.default_reasoning_effort,
    contextWindowTokens: raw.context_window_tokens,
    maxContextWindowTokens: raw.max_context_window_tokens,
  };
}

function mapLocalRunnerProvider(raw: RawLocalRunnerProvider): LocalRunnerProvider {
  return {
    key: raw.key,
    label: raw.label,
    installed: raw.installed,
    version: raw.version,
    authStatus: raw.auth_status,
    installHint: raw.installHint,
    detectedBinary: raw.detected_binary,
    detectedVersion: raw.detected_version,
    models: raw.models?.map(mapLocalRunnerProviderModel),
  };
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
    const raw = await readJson<RawLocalRunnerProvider[]>(response).catch(() => []);
    return raw.map(mapLocalRunnerProvider);
  }

  async installLocalProvider(providerKey: string): Promise<LocalRunnerProvider[]> {
    const response = await this.httpClient.request(new URL("/providers/install", this.runnerBaseUrl), {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ providerName: providerKey }),
    });
    const payload = await readJson<{ providers: RawLocalRunnerProvider[] }>(response);
    return (payload.providers ?? []).map(mapLocalRunnerProvider);
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

  async listTelegramProxyApprovals(status?: string): Promise<TelegramApprovalRecord[]> {
    const url = new URL("/telegram-proxy-approvals", this.runnerBaseUrl);
    if (status) url.searchParams.set("status", status);
    const response = await this.httpClient.request(url, { cache: "no-store" });
    return readJson<TelegramApprovalRecord[]>(response).catch(() => []);
  }

  async decideTelegramProxyApproval(id: string, decision: "approved" | "rejected", comment?: string): Promise<TelegramApprovalRecord> {
    const response = await this.httpClient.request(new URL(`/telegram-proxy-approvals/${id}/decision`, this.runnerBaseUrl), {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ decision, comment }),
    });
    return readJson<TelegramApprovalRecord>(response);
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
    private readonly runnerRepository: Pick<IntegrationRepository, "listMcpBackends" | "runMcpBackendAction" | "testIntegration" | "listTelegramProxyApprovals" | "decideTelegramProxyApproval">,
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
  listTelegramProxyApprovals(status?: string) { return this.runnerRepository.listTelegramProxyApprovals(status); }
  decideTelegramProxyApproval(id: string, decision: "approved" | "rejected", comment?: string) {
    return this.runnerRepository.decideTelegramProxyApproval(id, decision, comment);
  }
}
