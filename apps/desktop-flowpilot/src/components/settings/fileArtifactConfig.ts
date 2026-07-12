/**
 * Task-225: file_artifact.v1 instance config helpers for the Artifacts UI.
 * structure is optional; paths-only means existence-only gate (no section template).
 */

export const FILE_ARTIFACT_TYPE_ID = "file_artifact.v1";

export const DEFAULT_CODING_MEMO_SECTIONS = ["What", "Why", "Baseline"] as const;

export type FileArtifactStructure = {
  kind: "markdown_sections";
  sections: string[];
};

export function parseFileArtifactPaths(configJson: Record<string, unknown> | undefined): string[] {
  if (!configJson) return [];
  const raw = configJson.paths;
  if (!Array.isArray(raw)) return [];
  return raw
    .filter((item): item is string => typeof item === "string")
    .map((s) => s.trim())
    .filter(Boolean);
}

/**
 * True when config declares structure.kind markdown_sections (even if sections
 * are temporarily empty while the user edits the textarea).
 */
export function isFileArtifactStructureEnabled(
  configJson: Record<string, unknown> | undefined,
): boolean {
  if (!configJson) return false;
  const raw = configJson.structure;
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return false;
  return (raw as Record<string, unknown>).kind === "markdown_sections";
}

/**
 * Returns structure when kind is markdown_sections. sections may be empty
 * while editing; runner/gate ignore empty section lists.
 */
export function parseFileArtifactStructure(
  configJson: Record<string, unknown> | undefined,
): FileArtifactStructure | null {
  if (!isFileArtifactStructureEnabled(configJson)) return null;
  const obj = configJson!.structure as Record<string, unknown>;
  const sectionsRaw = obj.sections;
  const sections = Array.isArray(sectionsRaw)
    ? sectionsRaw
        .filter((item): item is string => typeof item === "string")
        .map((s) => s.trim())
        .filter(Boolean)
    : [];
  return { kind: "markdown_sections", sections };
}

export function sectionsToTextarea(sections: string[]): string {
  return sections.join("\n");
}

export function textareaToSections(text: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const line of text.split("\n")) {
    const s = line.trim();
    if (!s) continue;
    const key = s.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(s);
  }
  return out;
}

/**
 * Apply structure enable/sections to configJson.
 * - enabled false → remove structure (paths-only).
 * - enabled true → set structure; if sections empty and fillDefaultWhenEmpty,
 *   use coding memo defaults (What/Why/Baseline).
 * - strips discarded `format` key if present.
 */
export function withFileArtifactStructure(
  configJson: Record<string, unknown>,
  enabled: boolean,
  sectionsText: string,
  options?: { fillDefaultWhenEmpty?: boolean },
): Record<string, unknown> {
  const next: Record<string, unknown> = { ...configJson };
  delete next.format;
  if (!enabled) {
    delete next.structure;
    return next;
  }
  let sections = textareaToSections(sectionsText);
  if (sections.length === 0 && options?.fillDefaultWhenEmpty) {
    sections = [...DEFAULT_CODING_MEMO_SECTIONS];
  }
  next.structure = {
    kind: "markdown_sections",
    sections,
  } satisfies FileArtifactStructure;
  return next;
}

/**
 * Normalize config before save: if structure is enabled but sections empty,
 * fill coding-memo defaults so schema minItems is satisfied.
 */
export function normalizeFileArtifactConfigForSave(
  configJson: Record<string, unknown>,
): Record<string, unknown> {
  if (!isFileArtifactStructureEnabled(configJson)) {
    const next = { ...configJson };
    delete next.format;
    delete next.structure;
    return next;
  }
  const parsed = parseFileArtifactStructure(configJson);
  const text = sectionsToTextarea(parsed?.sections ?? []);
  return withFileArtifactStructure(configJson, true, text, { fillDefaultWhenEmpty: true });
}

/** Build initial file instance config (paths only). */
export function emptyFileArtifactConfig(): Record<string, unknown> {
  return { paths: [] as string[] };
}
