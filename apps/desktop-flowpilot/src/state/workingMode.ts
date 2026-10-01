export const WORKING_MODE_DEV = "dev" as const;
export const WORKING_MODE_VIBE = "vibe" as const;
export type WorkingMode = typeof WORKING_MODE_DEV | typeof WORKING_MODE_VIBE;

export const DEV_HARNESS_FIVE = [
  "task-harness",
  "bug-harness",
  "bug-plan-harness",
  "cp-harness",
  "context-coding-review-synthesis",
] as const;

let memoryDefault: WorkingMode = WORKING_MODE_DEV;

/** UI label "Normal" never goes on the wire. */
export function wireWorkingMode(labelOrMode: string | undefined): WorkingMode {
  const s = (labelOrMode ?? "").trim().toLowerCase();
  if (s === WORKING_MODE_VIBE) return WORKING_MODE_VIBE;
  return WORKING_MODE_DEV;
}

export function flowPickerOptions(mode: string | undefined): string[] {
  if (wireWorkingMode(mode) === WORKING_MODE_VIBE) {
    // CP-90: vibe-tasks = third entry (locked CP + existing parented tasks).
    return ["vibe-ingest", "vibe-cp-ingest", "vibe-tasks"];
  }
  return [...DEV_HARNESS_FIVE];
}

// --- Flow-tab pickers ---------------------------------------------------
// Desktop mirror of internal/workingmode's user-start family gate
// (FlowAllowedForWorkingMode with kind="user"). The runner is the enforcer;
// these helpers only keep forbidden options out of the pickers.

const PACK_PREFIX = "flowpilot-core-flow-pack/";

/** Builtin mirrors that exist as rows but are never user-startable. */
const HIDDEN_FLOW_IDS = new Set(["review-loop", "rag-harness", "cp-harness-smoke"]);
const VIBE_USER_FLOW_IDS = new Set(["vibe-ingest", "vibe-cp-ingest", "vibe-tasks"]);
const VIBE_SYSTEM_FLOW_IDS = new Set(["vibe-sprint", "vibe-owner-debate"]);

/** Strip an optional "flowpilot-core-flow-pack/" prefix (BareFlowID). */
export function bareFlowId(ref: string | undefined): string {
  const id = (ref ?? "").trim();
  if (!id) return "";
  if (id.startsWith(PACK_PREFIX)) return id.slice(PACK_PREFIX.length).trim();
  const slash = id.lastIndexOf("/");
  if (slash >= 0 && slash < id.length - 1 && id.slice(0, slash) === PACK_PREFIX.slice(0, -1)) {
    return id.slice(slash + 1);
  }
  return id;
}

function isVibeFamilyFlowId(id: string): boolean {
  return VIBE_USER_FLOW_IDS.has(id) || VIBE_SYSTEM_FLOW_IDS.has(id) || id.startsWith("vibe-");
}

/**
 * True when a user may start this flow under `mode`. `flowIdOrRef` is the
 * flow identity — a bare pack flow id, a pack-prefixed ref, or a catalog
 * (workflow row) UUID. Vibe mode allows only the two user vibe flows; dev
 * allows the harness five plus untracked catalog ids and rejects every
 * vibe-family or hidden id.
 */
export function userFlowSelectableForMode(mode: string | undefined, flowIdOrRef: string | undefined): boolean {
  const id = bareFlowId(flowIdOrRef);
  if (!id || HIDDEN_FLOW_IDS.has(id)) return false;
  if (wireWorkingMode(mode) === WORKING_MODE_VIBE) {
    return VIBE_USER_FLOW_IDS.has(id);
  }
  return !isVibeFamilyFlowId(id);
}

/**
 * Filter a workflow-select option list to the mode's startable set. Each
 * item's identity is its pack flow id when present (builtin mirrors),
 * otherwise the row id (catalog UUIDs).
 */
export function filterWorkflowsForWorkingMode<T extends { id: string; packFlowId?: string | null }>(
  workflows: T[],
  mode: string | undefined,
): T[] {
  return workflows.filter((w) => userFlowSelectableForMode(mode, w.packFlowId ?? w.id));
}

export function isCodingPlanCPPath(p: string | undefined): boolean {
  const raw = (p ?? "").trim().replace(/\\/g, "/");
  if (!raw.toLowerCase().endsWith(".md")) return false;
  const base = raw.split("/").pop() ?? "";
  if (!base.toUpperCase().startsWith("CP-")) return false;
  const lower = raw.toLowerCase();
  return lower.includes("/07-coding-plan/") || lower.startsWith("07-coding-plan/");
}

export function detectVibeEntry(pathOrPrompt: string | undefined): { flowRef: "vibe-ingest" | "vibe-cp-ingest"; sourceDocId: string } {
  const raw = (pathOrPrompt ?? "").trim();
  if (!raw) return { flowRef: "vibe-ingest", sourceDocId: "" };
  if (isCodingPlanCPPath(raw)) return { flowRef: "vibe-cp-ingest", sourceDocId: raw };
  const first = raw.split(/\s+/)[0] ?? "";
  if (isCodingPlanCPPath(first)) return { flowRef: "vibe-cp-ingest", sourceDocId: first };
  return { flowRef: "vibe-ingest", sourceDocId: raw };
}

export function workspaceRelPathFromFile(file: { name: string; path?: string }, cwd?: string): string {
  const raw = (file.path || file.name || "").replace(/\\/g, "/")
  const root = (cwd ?? "").replace(/\\/g, "/").replace(/\/+$/, "")
  if (root && raw.toLowerCase().startsWith(root.toLowerCase() + "/")) {
    return raw.slice(root.length + 1)
  }
  return raw
}

export function persistWorkingMode(mode: string | undefined): WorkingMode {
  const wired = wireWorkingMode(mode);
  memoryDefault = wired;
  try {
    if (typeof localStorage !== "undefined") {
      localStorage.setItem("flowpilot.workingMode", wired);
    }
  } catch {
    /* node tests / private mode */
  }
  return wired;
}

export function loadWorkingMode(): WorkingMode {
  try {
    if (typeof localStorage !== "undefined") {
      const raw = localStorage.getItem("flowpilot.workingMode");
      if (raw) return wireWorkingMode(raw);
    }
  } catch {
    /* ignore */
  }
  return memoryDefault;
}
