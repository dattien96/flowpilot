import type { RunnerHealth, RunnerRepository } from "../domain/runner";
import type { HttpClient } from "./http";

async function readError(response: Response) {
  const text = await response.text();
  if (!text) return `Request failed with status ${response.status}.`;
  try {
    const payload = JSON.parse(text) as { error?: string };
    return payload.error || text;
  } catch {
    return text;
  }
}

interface RunnerHealthPayload {
  cwd?: string | null;
  os?: string | null;
  startedAt?: string | null;
  version?: string | null;
}

export class HttpRunnerRepository implements RunnerRepository {
  constructor(
    private readonly httpClient: HttpClient,
    private readonly runnerBaseUrl: string,
  ) {}

  async loadHealth(): Promise<RunnerHealth> {
    const response = await this.httpClient.request(
      new URL("/health", this.runnerBaseUrl),
      { cache: "no-store" },
    );
    if (!response.ok) {
      throw new Error(await readError(response));
    }

    const payload = (await response.json()) as RunnerHealthPayload;
    return {
      cwd: payload.cwd ?? null,
      os: payload.os ?? null,
      startedAt: payload.startedAt ?? null,
      version: payload.version ?? null,
    };
  }
}
