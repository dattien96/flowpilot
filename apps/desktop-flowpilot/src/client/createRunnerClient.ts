import type { RunnerClient } from "@/types/contract";
import { MockRunnerClient } from "./MockRunnerClient";
import { HttpWsRunnerClient } from "./HttpWsRunnerClient";
import { RUNNER_URL } from "../config";

type RunnerEnv = Record<string, string | undefined>;
type RunnerEnvGlobal = typeof globalThis & {
  __FLOWPILOT_VITE_ENV__?: RunnerEnv;
};

// Selects the transport behind the RunnerClient contract (04-01 Part B):
//   - VITE_RUNNER_URL set  -> HttpWsRunnerClient against that runner
//   - VITE_LOCAL_RUNNER_URL set -> HttpWsRunnerClient against that runner
//   - VITE_USE_RUNNER=1     -> HttpWsRunnerClient against the default RUNNER_URL
//   - otherwise            -> MockRunnerClient (offline / UI dev)
// The renderer never references a concrete client — only this factory does.
export function resolveRunnerUrlFromSources(viteEnv: RunnerEnv = {}, processEnv: RunnerEnv = {}): string | null {
  if (viteEnv.VITE_RUNNER_URL) return viteEnv.VITE_RUNNER_URL;
  if (viteEnv.VITE_LOCAL_RUNNER_URL) return viteEnv.VITE_LOCAL_RUNNER_URL;
  if (viteEnv.VITE_USE_RUNNER === "1" || viteEnv.VITE_USE_RUNNER === "true") return RUNNER_URL;
  if (processEnv.VITE_RUNNER_URL) return processEnv.VITE_RUNNER_URL;
  if (processEnv.VITE_LOCAL_RUNNER_URL) return processEnv.VITE_LOCAL_RUNNER_URL;
  if (processEnv.VITE_USE_RUNNER === "1" || processEnv.VITE_USE_RUNNER === "true") return RUNNER_URL;
  return null;
}

function resolvedRunnerUrl(): string | null {
  const candidate = globalThis as RunnerEnvGlobal;
  const viteEnv = candidate.__FLOWPILOT_VITE_ENV__ ?? {};
  const processEnv = (typeof process !== "undefined" ? process.env : {}) as RunnerEnv;
  return resolveRunnerUrlFromSources(viteEnv, processEnv);
}

export function createRunnerClient(): RunnerClient {
  const url = resolvedRunnerUrl();
  return url ? new HttpWsRunnerClient(url) : new MockRunnerClient();
}

/** True when the offline mock client is in use (no runner URL configured). */
export function isMockMode(): boolean {
  return resolvedRunnerUrl() === null;
}

/** Header label: "mock" or "runner <url>" — so the active transport is visible. */
export function runnerModeLabel(): string {
  const url = resolvedRunnerUrl();
  return url ? `runner ${url}` : "mock";
}
