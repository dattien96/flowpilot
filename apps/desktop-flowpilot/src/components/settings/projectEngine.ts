import { RUNNER_URL } from "@/config";

export type ProjectEngineTrigger = "manual" | "bind";

export interface ProjectEngineToolStatus {
  tool: string;
  version?: string;
  status: string;
  checkedAt: string;
}

export interface GlobalEngineToolingStatus {
  tooling: ProjectEngineToolStatus[];
}

export interface ProjectEngineCapabilityProfile {
  hasGitNexus: boolean;
  hasRTK: boolean;
  hasNode: boolean;
  hasTests: boolean;
  hasSpecs: boolean;
  structureTier: string;
  decisionTier: string;
  languages: string[];
}

export interface ProjectEngineSkillProviderStatus {
  provider: string;
  path: string;
  present: boolean;
  current: boolean;
}

export interface ProjectEngineSkillStatus {
  name: string;
  providers: ProjectEngineSkillProviderStatus[];
}

export interface ProjectEngineSkillPackState {
  packVersion: number;
  installed: boolean;
  current: boolean;
  skills: ProjectEngineSkillStatus[];
}

export interface ProjectEngineInitStepResult {
  step: string;
  outcome: string;
  detail?: string;
  errorMessage?: string;
}

export interface ProjectEngineInitResult {
  trigger: string;
  status: string;
  skipped: boolean;
  skipReason?: string;
  attemptedAt: string;
  completedAt: string;
  workingDirectory: string;
  install: {
    installedPaths: string[];
    skippedPaths: string[];
    errors: string[];
  };
  steps: ProjectEngineInitStepResult[];
}

export interface ProjectEngineStatus {
  projectId: string;
  workingDirectory: string;
  initialized: boolean;
  gateMode: string;
  tooling: ProjectEngineToolStatus[];
  capability: ProjectEngineCapabilityProfile;
  skillPack: ProjectEngineSkillPackState;
  lastInit: ProjectEngineInitResult | null;
  warnings?: string[];
}

interface RawProjectEngineToolStatus {
  tool: string;
  version?: string;
  status: string;
  checked_at: string;
}

interface RawProjectEngineCapabilityProfile {
  has_gitnexus: boolean;
  has_rtk: boolean;
  has_node: boolean;
  has_tests: boolean;
  has_specs: boolean;
  structure_tier: string;
  decision_tier: string;
  languages: string[];
}

interface RawProjectEngineSkillProviderStatus {
  provider: string;
  path: string;
  present: boolean;
  current: boolean;
}

interface RawProjectEngineSkillStatus {
  name: string;
  providers: RawProjectEngineSkillProviderStatus[];
}

interface RawProjectEngineSkillPackState {
  packVersion: number;
  installed: boolean;
  current: boolean;
  skills: RawProjectEngineSkillStatus[];
}

interface RawProjectEngineInitStepResult {
  step: string;
  outcome: string;
  detail?: string;
  errorMessage?: string;
}

interface RawProjectEngineStatus {
  projectId: string;
  workingDirectory: string;
  initialized: boolean;
  gateMode?: string;
  tooling: RawProjectEngineToolStatus[];
  capability: RawProjectEngineCapabilityProfile;
  skillPack: RawProjectEngineSkillPackState;
  lastInit?: {
    trigger: string;
    status: string;
    skipped: boolean;
    skipReason?: string;
    attemptedAt: string;
    completedAt: string;
    workingDirectory: string;
    install: {
      installedPaths: string[];
      skippedPaths: string[];
      errors: string[];
    };
    steps: RawProjectEngineInitStepResult[];
  } | null;
  warnings?: string[];
}

interface RawLibreTranslateInstallResult {
  success: boolean;
  output: string;
  error?: string;
  tooling: RawProjectEngineToolStatus[];
}

export interface LibreTranslateInstallResult {
  success: boolean;
  output: string;
  error?: string;
  tooling: ProjectEngineToolStatus[];
}

interface RawGlobalEngineToolingStatus {
  tooling: RawProjectEngineToolStatus[];
}

function mapProjectEngineStatus(raw: RawProjectEngineStatus): ProjectEngineStatus {
  return {
    projectId: raw.projectId,
    workingDirectory: raw.workingDirectory,
    initialized: raw.initialized,
    gateMode: raw.gateMode ?? "enforce",
    tooling: (raw.tooling ?? []).map((tool) => ({
      tool: tool.tool,
      version: tool.version,
      status: tool.status,
      checkedAt: tool.checked_at,
    })),
    capability: {
      hasGitNexus: raw.capability?.has_gitnexus ?? false,
      hasRTK: raw.capability?.has_rtk ?? false,
      hasNode: raw.capability?.has_node ?? false,
      hasTests: raw.capability?.has_tests ?? false,
      hasSpecs: raw.capability?.has_specs ?? false,
      structureTier: raw.capability?.structure_tier ?? "fallback",
      decisionTier: raw.capability?.decision_tier ?? "git-only",
      languages: raw.capability?.languages ?? [],
    },
    skillPack: {
      packVersion: raw.skillPack?.packVersion ?? 0,
      installed: raw.skillPack?.installed ?? false,
      current: raw.skillPack?.current ?? false,
      skills: (raw.skillPack?.skills ?? []).map((skill) => ({
        name: skill.name,
        providers: (skill.providers ?? []).map((provider) => ({
          provider: provider.provider,
          path: provider.path,
          present: provider.present,
          current: provider.current,
        })),
      })),
    },
    lastInit: raw.lastInit
      ? {
          trigger: raw.lastInit.trigger,
          status: raw.lastInit.status,
          skipped: raw.lastInit.skipped,
          skipReason: raw.lastInit.skipReason,
          attemptedAt: raw.lastInit.attemptedAt,
          completedAt: raw.lastInit.completedAt,
          workingDirectory: raw.lastInit.workingDirectory,
          install: {
            installedPaths: raw.lastInit.install?.installedPaths ?? [],
            skippedPaths: raw.lastInit.install?.skippedPaths ?? [],
            errors: raw.lastInit.install?.errors ?? [],
          },
          steps: (raw.lastInit.steps ?? []).map((step) => ({
            step: step.step,
            outcome: step.outcome,
            detail: step.detail,
            errorMessage: step.errorMessage,
          })),
        }
      : null,
    warnings: raw.warnings ?? [],
  };
}

function mapGlobalEngineToolingStatus(
  raw: RawGlobalEngineToolingStatus,
): GlobalEngineToolingStatus {
  return {
    tooling: (raw.tooling ?? []).map((tool) => ({
      tool: tool.tool,
      version: tool.version,
      status: tool.status,
      checkedAt: tool.checked_at,
    })),
  };
}

async function readProjectEngineError(response: Response): Promise<string> {
  const text = await response.text().catch(() => "");
  if (!text) {
    return `Request failed with status ${response.status}.`;
  }
  try {
    const payload = JSON.parse(text) as { error?: { message?: string } | string };
    if (typeof payload.error === "string") {
      return payload.error;
    }
    return payload.error?.message || text;
  } catch {
    return text;
  }
}

export function resolvePrimaryBindingPath(
  bindings: ReadonlyArray<{ localPath: string }>,
): string | null {
  for (const binding of bindings) {
    const path = binding.localPath.trim();
    if (path.length > 0) {
      return path;
    }
  }
  return null;
}

export function uniqueBindingPaths(
  bindings: ReadonlyArray<{ localPath: string }>,
): string[] {
  return Array.from(
    new Set(
      bindings
        .map((binding) => binding.localPath.trim())
        .filter((path) => path.length > 0),
    ),
  );
}

export function summarizeProjectEngineInit(
  result: ProjectEngineInitResult | null | undefined,
): string {
  if (!result) {
    return "No engine init has been recorded yet.";
  }
  const installed = result.install.installedPaths.length;
  const skipped = result.install.skippedPaths.length;
  const errors = result.install.errors.length;
  const state = result.skipped
    ? "Skipped"
    : result.status === "success"
      ? "Completed"
      : result.status === "partial"
        ? "Completed with warnings"
        : "Failed";
  return `${state}: ${installed} installed, ${skipped} skipped, ${errors} errors.`;
}

export function engineTone(status: string): "passed" | "warn" | "fail" {
  if (status === "ok" || status === "success") {
    return "passed";
  }
  if (
    status === "partial"
    || status === "stale"
    || status === "skipped"
    || status === "warning"
    || status === "warn"
  ) {
    return "warn";
  }
  return "fail";
}

export async function fetchGlobalEngineToolingStatus(): Promise<GlobalEngineToolingStatus> {
  const response = await fetch(
    new URL("/client/engine/tooling/status", RUNNER_URL).toString(),
    { cache: "no-store" },
  );
  if (!response.ok) {
    throw new Error(await readProjectEngineError(response));
  }
  return mapGlobalEngineToolingStatus(
    (await response.json()) as RawGlobalEngineToolingStatus,
  );
}

export async function installLibreTranslateTool(signal?: AbortSignal): Promise<LibreTranslateInstallResult> {
  const response = await fetch(
    new URL("/client/engine/tooling/install/libretranslate", RUNNER_URL).toString(),
    { method: "POST", cache: "no-store", signal },
  );
  if (!response.ok) {
    throw new Error(await readProjectEngineError(response));
  }
  const raw = (await response.json()) as RawLibreTranslateInstallResult;
  return {
    success: raw.success,
    output: raw.output,
    error: raw.error,
    tooling: (raw.tooling ?? []).map((t) => ({
      tool: t.tool,
      version: t.version,
      status: t.status,
      checkedAt: t.checked_at,
    })),
  };
}

export async function fetchProjectEngineStatus(
  projectId: string,
  workingDirectory: string,
  platform?: string,
): Promise<ProjectEngineStatus> {
  const platformSuffix = platform ? `&platform=${encodeURIComponent(platform)}` : "";
  const response = await fetch(
    new URL(
      `/client/projects/${encodeURIComponent(projectId)}/engine/status?workingDirectory=${encodeURIComponent(workingDirectory)}${platformSuffix}`,
      RUNNER_URL,
    ).toString(),
    { cache: "no-store" },
  );
  if (!response.ok) {
    throw new Error(await readProjectEngineError(response));
  }
  return mapProjectEngineStatus((await response.json()) as RawProjectEngineStatus);
}

export interface XcodeConfig {
  xcodeScheme?: string | null;
  xcodeDestination?: string | null;
}

export async function initProjectEngine(
  projectId: string,
  workingDirectory: string,
  trigger: ProjectEngineTrigger,
  platform?: string,
  xcodeConfig?: XcodeConfig,
): Promise<ProjectEngineStatus> {
  const response = await fetch(
    new URL(`/client/projects/${encodeURIComponent(projectId)}/engine/init`, RUNNER_URL).toString(),
    {
      method: "POST",
      cache: "no-store",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify({
        workingDirectory,
        trigger,
        ...(platform ? { platform } : {}),
        ...(xcodeConfig?.xcodeScheme ? { xcodeScheme: xcodeConfig.xcodeScheme } : {}),
        ...(xcodeConfig?.xcodeDestination ? { xcodeDestination: xcodeConfig.xcodeDestination } : {}),
      }),
    },
  );
  if (!response.ok) {
    throw new Error(await readProjectEngineError(response));
  }
  return mapProjectEngineStatus((await response.json()) as RawProjectEngineStatus);
}

export async function fetchProjectEngineGateMode(
  projectId: string,
  workingDirectory: string,
): Promise<string> {
  const response = await fetch(
    new URL(
      `/client/projects/${encodeURIComponent(projectId)}/engine/gate-config?workingDirectory=${encodeURIComponent(workingDirectory)}`,
      RUNNER_URL,
    ).toString(),
    { cache: "no-store" },
  );
  if (!response.ok) {
    throw new Error(await readProjectEngineError(response));
  }
  const payload = (await response.json()) as { gateMode?: string };
  return payload.gateMode ?? "enforce";
}

export async function saveProjectEngineGateMode(
  projectId: string,
  workingDirectory: string,
  gateMode: string,
): Promise<string> {
  const response = await fetch(
    new URL(
      `/client/projects/${encodeURIComponent(projectId)}/engine/gate-config`,
      RUNNER_URL,
    ).toString(),
    {
      method: "POST",
      cache: "no-store",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ workingDirectory, gate_mode: gateMode }),
    },
  );
  if (!response.ok) {
    throw new Error(await readProjectEngineError(response));
  }
  const payload = (await response.json()) as { gateMode?: string };
  return payload.gateMode ?? "enforce";
}

// fetchApprovalAllowlist returns the project's persisted "don't ask again"
// shell-command rules (BUG-246).
export async function fetchApprovalAllowlist(
  projectId: string,
  workingDirectory: string,
): Promise<string[]> {
  const response = await fetch(
    new URL(
      `/client/projects/${encodeURIComponent(projectId)}/engine/approval-allowlist?workingDirectory=${encodeURIComponent(workingDirectory)}`,
      RUNNER_URL,
    ).toString(),
    { cache: "no-store" },
  );
  if (!response.ok) {
    throw new Error(await readProjectEngineError(response));
  }
  const payload = (await response.json()) as { allow?: string[] };
  return payload.allow ?? [];
}

// removeApprovalAllowRule drops one remembered rule and returns the updated list.
export async function removeApprovalAllowRule(
  projectId: string,
  workingDirectory: string,
  rule: string,
): Promise<string[]> {
  const response = await fetch(
    new URL(
      `/client/projects/${encodeURIComponent(projectId)}/engine/approval-allowlist/remove`,
      RUNNER_URL,
    ).toString(),
    {
      method: "POST",
      cache: "no-store",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ workingDirectory, rule }),
    },
  );
  if (!response.ok) {
    throw new Error(await readProjectEngineError(response));
  }
  const payload = (await response.json()) as { allow?: string[] };
  return payload.allow ?? [];
}

export async function autoInitProjectEngine(
  projectId: string,
  bindings: ReadonlyArray<{ localPath: string }>,
  platform?: string,
  xcodeConfig?: XcodeConfig,
): Promise<void> {
  const paths = uniqueBindingPaths(bindings);
  await Promise.allSettled(
    paths.map(async (workingDirectory) => {
      try {
        await initProjectEngine(projectId, workingDirectory, "bind", platform, xcodeConfig);
      } catch (error) {
        console.warn(
          "[ProjectsSettings] bind-time engine init failed",
          projectId,
          workingDirectory,
          error,
        );
      }
    }),
  );
}
