/** Task-309: frozen-contract scope drift parsing (mirrors gate_hook.go:863 / TUI parseDriftedPaths). */
export const DRIFT_PATH_MARKER = "wrote outside the frozen contract's declared paths:" as const;

export function parseDriftedPaths(gate: string): string[] | null {
  const idx = gate.indexOf(DRIFT_PATH_MARKER);
  if (idx < 0) return null;
  let rest = gate.slice(idx + DRIFT_PATH_MARKER.length).trim();
  if (rest.endsWith(".")) rest = rest.slice(0, -1).trim();
  if (!rest) return null;
  const parts = rest
    .split(",")
    .map((p) => p.trim().replace(/\.$/, "").trim())
    .filter((p) => p.length > 0);
  return parts.length > 0 ? parts : null;
}

export function awaitingUserDriftState(loopState: {
  blockReason?: string;
  gateReason?: string;
}): {
  driftedPaths: string[] | null;
  isDrift: boolean;
  stalled: boolean;
  isCap: boolean;
  isSprintBoundary: boolean;
  retryIsPrimary: boolean;
} {
  const stalled = loopState.blockReason === "member_stalled";
  const isCap = loopState.blockReason === "cap";
  // vibe_sprint_boundary (CA-1097): a finished sprint parked for continuation —
  // the action starts the next task, it is not a retry.
  const isSprintBoundary = loopState.blockReason === "vibe_sprint_boundary";
  const driftedPaths = !stalled && !isCap ? parseDriftedPaths(loopState.gateReason ?? "") : null;
  const isDrift = driftedPaths !== null && driftedPaths.length > 0;
  return {
    driftedPaths,
    isDrift,
    stalled,
    isCap,
    isSprintBoundary,
    retryIsPrimary: !isDrift,
  };
}
