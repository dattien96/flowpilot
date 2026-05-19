import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";
import type {
  LocalRunnerArtifact,
  LocalRunnerBackupResult,
  LocalRunnerFlow,
  LocalRunnerHealth,
  LocalRunnerIntegrationConnectionRequest,
  LocalRunnerIntegrationConnectionResult,
  LocalRunnerMcpBackend,
  LocalRunnerMcpBackendActionRequest,
  LocalRunnerMcpTestRequest,
  LocalRunnerMcpTestResult,
  LocalRunnerMcpTestRunSummary,
  LocalRunnerProvider,
  LocalRunnerPromptExecutionRequest,
  LocalRunnerPromptExecutionResult,
  LocalRunnerStorageDriver,
  LocalRunnerStorageDriverRequest,
  LocalRunnerSkill,
} from "@/domain/model/entity/local-runner";

type HealthResponse = Omit<LocalRunnerHealth, "baseUrl" | "errorMessage">;

async function readJson<T>(baseUrl: string, path: string): Promise<T> {
  const response = await fetch(new URL(path, baseUrl), {
    cache: "no-store",
  });

  if (!response.ok) {
    throw new Error(`Local runner request failed: ${response.status} ${response.statusText}`);
  }

  return (await response.json()) as T;
}

function offlineHealth(baseUrl: string, errorMessage: string): LocalRunnerHealth {
  return {
    status: "offline",
    runnerVersion: null,
    cwd: null,
    os: null,
    startedAt: null,
    baseUrl,
    errorMessage,
  };
}

export class HttpLocalRunnerGateway implements LocalRunnerGateway {
  constructor(private readonly baseUrl: string) {}

  async getHealth() {
    try {
      const payload = await readJson<HealthResponse>(this.baseUrl, "/health");
      return {
        ...payload,
        baseUrl: this.baseUrl,
        errorMessage: null,
      };
    } catch (error) {
      return offlineHealth(
        this.baseUrl,
        error instanceof Error ? error.message : "Unable to reach the local runner.",
      );
    }
  }

  async listProviders() {
    try {
      return await readJson<LocalRunnerProvider[]>(this.baseUrl, "/providers");
    } catch {
      return [];
    }
  }

  async listSkills() {
    try {
      return await readJson<LocalRunnerSkill[]>(this.baseUrl, "/skills");
    } catch {
      return [];
    }
  }

  async listFlows() {
    try {
      return await readJson<LocalRunnerFlow[]>(this.baseUrl, "/flows");
    } catch {
      return [];
    }
  }

  async listArtifacts() {
    try {
      return await readJson<LocalRunnerArtifact[]>(this.baseUrl, "/artifacts");
    } catch {
      return [];
    }
  }

  async getArtifactById(artifactId: string) {
    try {
      return await readJson<LocalRunnerArtifact>(this.baseUrl, `/artifacts/${artifactId}`);
    } catch {
      return null;
    }
  }

  async getStorageDriver() {
    try {
      return await readJson<LocalRunnerStorageDriver>(this.baseUrl, "/storage-driver");
    } catch {
      return {
        driverKey: "filesystem",
        enabled: false,
        remoteRootPath: "",
        remoteFolderName: "FlowPilot",
        lastValidatedAt: null,
        lastSyncedAt: null,
        lastError: null,
        updatedAt: null,
      };
    }
  }

  async saveStorageDriver(request: LocalRunnerStorageDriverRequest) {
    const response = await fetch(new URL("/storage-driver", this.baseUrl), {
      method: "PUT",
      cache: "no-store",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify(request),
    });

    if (!response.ok) {
      throw new Error(`Storage driver save failed: ${response.status} ${response.statusText}`);
    }

    return (await response.json()) as LocalRunnerStorageDriver;
  }

  async validateStorageDriver() {
    const response = await fetch(new URL("/storage-driver", this.baseUrl), {
      method: "POST",
      cache: "no-store",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify({}),
    });

    if (!response.ok) {
      throw new Error(
        `Storage driver validation failed: ${response.status} ${response.statusText}`,
      );
    }

    return (await response.json()) as LocalRunnerStorageDriver;
  }

  async listMcpBackends() {
    try {
      return await readJson<LocalRunnerMcpBackend[]>(this.baseUrl, "/mcp-backends");
    } catch {
      return [];
    }
  }

  async installMcpBackend(backendKey: string) {
    const response = await fetch(
      new URL(`/mcp-backends/${encodeURIComponent(backendKey)}/install`, this.baseUrl),
      {
        method: "POST",
        cache: "no-store",
        headers: {
          "content-type": "application/json",
        },
        body: JSON.stringify({}),
      },
    );

    if (!response.ok) {
      throw new Error(`MCP backend install failed: ${response.status} ${response.statusText}`);
    }

    return (await response.json()) as LocalRunnerMcpBackend;
  }

  async triggerMcpBackendAction(
    backendKey: string,
    request: LocalRunnerMcpBackendActionRequest,
  ) {
    const response = await fetch(
      new URL(`/mcp-backends/${encodeURIComponent(backendKey)}/action`, this.baseUrl),
      {
        method: "POST",
        cache: "no-store",
        headers: {
          "content-type": "application/json",
        },
        body: JSON.stringify(request),
      },
    );

    if (!response.ok) {
      throw new Error(`MCP backend action failed: ${response.status} ${response.statusText}`);
    }

    return (await response.json()) as LocalRunnerMcpBackend;
  }

  async listMcpTestRuns(backendKey: string, projectId?: string, integrationId?: string, limit = 5) {
    try {
      const url = new URL("/mcp-tests", this.baseUrl);
      url.searchParams.set("backendKey", backendKey);
      if (projectId) {
        url.searchParams.set("projectId", projectId);
      }
      if (integrationId) {
        url.searchParams.set("integrationId", integrationId);
      }
      url.searchParams.set("limit", String(limit));
      return await readJson<LocalRunnerMcpTestRunSummary[]>(this.baseUrl, url.pathname + url.search);
    } catch {
      return [];
    }
  }

  async runMcpTest(request: LocalRunnerMcpTestRequest) {
    const response = await fetch(new URL("/mcp-tests", this.baseUrl), {
      method: "POST",
      cache: "no-store",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify(request),
    });

    if (!response.ok) {
      throw new Error(`MCP test failed: ${response.status} ${response.statusText}`);
    }

    return (await response.json()) as LocalRunnerMcpTestResult;
  }

  async syncArtifact(artifactId: string) {
    const response = await fetch(new URL(`/artifacts/${artifactId}/sync`, this.baseUrl), {
      method: "POST",
      cache: "no-store",
    });

    if (!response.ok) {
      throw new Error(`Artifact sync failed: ${response.status} ${response.statusText}`);
    }

    return (await response.json()) as LocalRunnerArtifact;
  }

  async createBackup(scope: string, runId: string | null) {
    const response = await fetch(new URL("/backup", this.baseUrl), {
      method: "POST",
      cache: "no-store",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify({
        scope,
        runId,
      }),
    });

    if (!response.ok) {
      throw new Error(`Artifact backup failed: ${response.status} ${response.statusText}`);
    }

    return (await response.json()) as LocalRunnerBackupResult;
  }

  async triggerIntegrationConnection(request: LocalRunnerIntegrationConnectionRequest) {
    const response = await fetch(
      new URL(`/integrations/${encodeURIComponent(request.integrationId)}/connection`, this.baseUrl),
      {
        method: "POST",
        cache: "no-store",
        headers: {
          "content-type": "application/json",
        },
        body: JSON.stringify({
          projectId: request.projectId,
          providerType: request.providerType,
          action: request.action,
          ...(request.workspaceUrl ? { workspaceUrl: request.workspaceUrl } : {}),
          ...(request.projectKey ? { projectKey: request.projectKey } : {}),
          ...(request.boardId ? { boardId: request.boardId } : {}),
          ...(request.email ? { email: request.email } : {}),
          ...(request.apiToken ? { apiToken: request.apiToken } : {}),
        }),
      },
    );

    if (!response.ok) {
      throw new Error(
        `Integration connection trigger failed: ${response.status} ${response.statusText}`,
      );
    }

    return (await response.json()) as LocalRunnerIntegrationConnectionResult;
  }

  async deleteIntegrationConnection(integrationId: string) {
    const response = await fetch(
      new URL(`/integrations/${encodeURIComponent(integrationId)}/connection`, this.baseUrl),
      {
        method: "DELETE",
        cache: "no-store",
      },
    );

    if (!response.ok) {
      throw new Error(
        `Integration connection delete failed: ${response.status} ${response.statusText}`,
      );
    }
  }

  async executePrompt(request: LocalRunnerPromptExecutionRequest) {
    const response = await fetch(new URL("/execute", this.baseUrl), {
      method: "POST",
      cache: "no-store",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify(request),
    });

    if (!response.ok) {
      throw new Error(`Local runner prompt execution failed: ${response.status} ${response.statusText}`);
    }

    return (await response.json()) as LocalRunnerPromptExecutionResult;
  }
}
