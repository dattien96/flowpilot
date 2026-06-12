import type { RunnerClient } from "@/types/contract";
import { MockRunnerClient } from "./MockRunnerClient";
import { HttpWsRunnerClient } from "./HttpWsRunnerClient";
import { RUNNER_URL } from "@/config";

// Selects the transport behind the RunnerClient contract (04-01 Part B):
//   - VITE_RUNNER_URL set  -> HttpWsRunnerClient against that runner
//   - VITE_USE_RUNNER=1     -> HttpWsRunnerClient against the default RUNNER_URL
//   - otherwise            -> MockRunnerClient (offline / UI dev)
// The renderer never references a concrete client — only this factory does.
export function createRunnerClient(): RunnerClient {
  const env = import.meta.env as Record<string, string | undefined>;
  const explicit = env.VITE_RUNNER_URL;
  if (explicit) return new HttpWsRunnerClient(explicit);
  if (env.VITE_USE_RUNNER === "1" || env.VITE_USE_RUNNER === "true") {
    return new HttpWsRunnerClient(RUNNER_URL);
  }
  return new MockRunnerClient();
}
