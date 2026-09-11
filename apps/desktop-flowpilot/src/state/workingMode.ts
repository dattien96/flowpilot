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
    return ["vibe-ingest", "vibe-cp-ingest"];
  }
  return [...DEV_HARNESS_FIVE];
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
