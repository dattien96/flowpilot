import type { RunnerClient } from "@/types/contract";
import { MockRunnerClient } from "./MockRunnerClient";
import { HttpWsRunnerClient } from "./HttpWsRunnerClient";
import { RUNNER_URL } from "@/config";

// Selects the transport behind the RunnerClient contract (04-01 Part B):
//   - VITE_RUNNER_URL set  -> HttpWsRunnerClient against that runner
//   - VITE_USE_RUNNER=1     -> HttpWsRunnerClient against the default RUNNER_URL
//   - otherwise            -> MockRunnerClient (offline / UI dev)
// The renderer never references a concrete client — only this factory does.
function resolvedRunnerUrl(): string | null {
  const env = import.meta.env as Record<string, string | undefined>;
  if (env.VITE_RUNNER_URL) return env.VITE_RUNNER_URL;
  if (env.VITE_USE_RUNNER === "1" || env.VITE_USE_RUNNER === "true") return RUNNER_URL;
  return null;
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
