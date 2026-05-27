import { mapProvider, type RawProvider } from "./local-runner-mappers";
import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";
import type {
  LocalRunnerArtifact,
  LocalRunnerBackupResult,
  LocalRunnerDirectorySelection,
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
  LocalRunnerAiSessionStartRequest,
  LocalRunnerAiSessionHandle,
  LocalRunnerAiSessionMessageRequest,
} from "@/domain/model/entity/local-runner";

type HealthResponse = Omit<LocalRunnerHealth, "baseUrl" | "errorMessage">;

export class LocalRunnerError extends Error {
  code?: string;
  details?: string;

  constructor(message: string, code?: string, details?: string) {
    super(message);
    this.name = "LocalRunnerError";
    this.code = code;
    this.details = details;
  }
}

async function readJson<T>(baseUrl: string, path: string): Promise<T> {
  const response = await fetch(new URL(path, baseUrl), {
    cache: "no-store",
  });

  if (!response.ok) {
    throw new Error(`Local runner request failed: ${response.status} ${response.statusText}`);
  }

  return (await response.json()) as T;
}

async function fetchWithTimeout(
  input: URL,
  init: RequestInit,
  {
    timeoutMs,
    timeoutMessage,
  }: {
    timeoutMs: number;
    timeoutMessage: string;
  },
) {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(timeoutMessage), timeoutMs);

  try {
    return await fetch(input, {
      ...init,
      signal: controller.signal,
    });
  } catch (error) {
    if (controller.signal.aborted || (error instanceof Error && error.name === "AbortError")) {
      throw new Error(timeoutMessage);
    }

    throw error;
  } finally {
    clearTimeout(timeoutId);
  }
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

  async pickDirectory() {
    let response: Response;
    try {
      response = await fetch(new URL("/directories/pick", this.baseUrl), {
        method: "POST",
        cache: "no-store",
        headers: {
          "content-type": "application/json",
        },
        body: JSON.stringify({}),
      });
    } catch (error) {
      throw new Error(
        `Local runner is unreachable at ${this.baseUrl}. Start it with 'just runner-dev' or 'just dev' and try again.`,
      );
    }

    if (!response.ok) {
      throw new Error(`Directory picker failed: ${response.status} ${response.statusText}`);
    }

    return (await response.json()) as LocalRunnerDirectorySelection;
  }

  async listProviders() {
    try {
      const rawProviders = await readJson<RawProvider[]>(this.baseUrl, "/providers");
      return rawProviders.map(mapProvider);
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

  async startSession(request: LocalRunnerAiSessionStartRequest) {
    const response = await fetch(new URL("/sessions/start", this.baseUrl), {
      method: "POST",
      cache: "no-store",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify(request),
    });

    if (!response.ok) {
      throw new Error(`Local runner start session failed: ${response.status} ${response.statusText}`);
    }

    return (await response.json()) as LocalRunnerAiSessionHandle;
  }

  async sendMessage(request: LocalRunnerAiSessionMessageRequest) {
    const response = await fetch(new URL("/sessions/message", this.baseUrl), {
      method: "POST",
      cache: "no-store",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify(request),
    });

    if (!response.ok) {
      let code: string | undefined;
      let details: string | undefined;
      let message = `Local runner session message failed: ${response.status} ${response.statusText}`;
      try {
        const errorBody = await response.json();
        if (errorBody && typeof errorBody === "object" && errorBody.code) {
          code = errorBody.code;
          if (errorBody.message) message = errorBody.message;
          if (errorBody.details) details = errorBody.details;
        }
      } catch (e) {
        // Fallback to text or generic message
      }
      throw new LocalRunnerError(message, code, details);
    }

    return (await response.json()) as LocalRunnerPromptExecutionResult;
  }

  async closeSession(session: LocalRunnerAiSessionHandle) {
    const response = await fetch(new URL("/sessions/close", this.baseUrl), {
      method: "POST",
      cache: "no-store",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify(session),
    });

    if (!response.ok) {
      throw new Error(`Local runner close session failed: ${response.status} ${response.statusText}`);
    }
  }

  async authenticateProvider(providerName: string) {
    const response = await fetch(new URL("/providers/auth", this.baseUrl), {
      method: "POST",
      cache: "no-store",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify({ providerName }),
    });

    if (!response.ok) {
      throw new Error(`Provider authentication trigger failed: ${response.status} ${response.statusText}`);
    }
  }

  async readFile(path: string): Promise<string> {
    const url = new URL("/files/read", this.baseUrl);
    url.searchParams.set("path", path);
    const response = await fetchWithTimeout(
      url,
      {
        method: "GET",
        cache: "no-store",
      },
      {
        timeoutMs: 8000,
        timeoutMessage: `Timed out reading file from local runner at ${this.baseUrl}.`,
      },
    );

    if (!response.ok) {
      throw new Error(`File read failed: ${response.status} ${response.statusText}`);
    }

    const json = await response.json() as { content: string };
    return json.content;
  }
}
