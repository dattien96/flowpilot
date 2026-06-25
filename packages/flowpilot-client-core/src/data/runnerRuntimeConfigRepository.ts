import type {
  RuntimeConfigRepository,
  SupabaseConfigInput,
  SupabaseSchemaApplyInput,
  SupabaseSchemaApplyResult,
  SupabaseConfigValidation,
  SupabaseRuntimeStatus,
} from "../domain/runtime";
import type { HttpClient } from "./http";

interface SupabaseWorkspaceConfigPayload {
  apiUrl?: string;
  anonKey?: string;
  edgeFunctionUrl?: string;
  projectRef?: string;
  hasServiceRoleKey?: boolean;
  serviceRoleKey?: string;
}

export interface SupabaseWorkspaceConfigWithSecret {
  apiUrl: string;
  anonKey: string;
  serviceRoleKey: string | null;
}

function deriveProjectRef(apiUrl: string) {
  try {
    const host = new URL(apiUrl).host.toLowerCase();
    return host.replace(/\.supabase\.co$/, "").split(".")[0] || null;
  } catch {
    return null;
  }
}

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

export class RunnerRuntimeConfigRepository implements RuntimeConfigRepository {
  constructor(
    private readonly httpClient: HttpClient,
    private readonly runnerBaseUrl: string,
  ) {}

  async loadSupabaseRuntimeStatus(): Promise<SupabaseRuntimeStatus> {
    try {
      const response = await this.httpClient.request(
        new URL("/supabase-config", this.runnerBaseUrl),
        { cache: "no-store" },
      );

      if (response.ok) {
        const payload = (await response.json()) as SupabaseWorkspaceConfigPayload;
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
    } catch (error) {
      return this.demoStatus(
        false,
        error instanceof Error ? error.message : "Unable to reach local runner.",
      );
    }
  }

  async validateSupabaseConfig(
    input: SupabaseConfigInput,
  ): Promise<SupabaseConfigValidation> {
    const response = await this.httpClient.request(
      new URL("/supabase-config/validate", this.runnerBaseUrl),
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(input),
      },
    );

    if (!response.ok) {
      throw new Error(await readError(response));
    }

    return (await response.json()) as SupabaseConfigValidation;
  }

  async saveSupabaseConfig(input: SupabaseConfigInput): Promise<SupabaseRuntimeStatus> {
    const response = await this.httpClient.request(
      new URL("/supabase-config", this.runnerBaseUrl),
      {
        method: "PUT",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(input),
      },
    );

    if (!response.ok) {
      throw new Error(await readError(response));
    }

    return this.loadSupabaseRuntimeStatus();
  }

  async applySupabaseMigrations(
    input: SupabaseSchemaApplyInput,
  ): Promise<SupabaseSchemaApplyResult> {
    const response = await this.httpClient.request(
      new URL("/supabase-config/apply-migrations", this.runnerBaseUrl),
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(input),
      },
    );

    if (!response.ok) {
      throw new Error(await readError(response));
    }

    return (await response.json()) as SupabaseSchemaApplyResult;
  }

  async loadSupabaseWorkspaceConfigWithSecret(): Promise<SupabaseWorkspaceConfigWithSecret | null> {
    const response = await this.httpClient.request(
      new URL("/supabase-config?includeSecret=1", this.runnerBaseUrl),
      { cache: "no-store" },
    );

    if (response.status === 404) {
      return null;
    }
    if (!response.ok) {
      throw new Error(await readError(response));
    }

    const payload = (await response.json()) as SupabaseWorkspaceConfigPayload;
    if (!payload.apiUrl || !payload.anonKey) {
      return null;
    }
    return {
      apiUrl: payload.apiUrl,
      anonKey: payload.anonKey,
      serviceRoleKey: payload.serviceRoleKey?.trim() || null,
    };
  }

  private demoStatus(
    runnerReachable: boolean,
    lastError: string | null,
  ): SupabaseRuntimeStatus {
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
