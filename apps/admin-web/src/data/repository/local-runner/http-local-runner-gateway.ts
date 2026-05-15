import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";
import type {
  LocalRunnerFlow,
  LocalRunnerHealth,
  LocalRunnerProvider,
  LocalRunnerPromptExecutionRequest,
  LocalRunnerPromptExecutionResult,
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
