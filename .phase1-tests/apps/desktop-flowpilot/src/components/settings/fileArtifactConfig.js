"use strict";
/**
 * Task-225: file_artifact.v1 instance config helpers for the Artifacts UI.
 * structure is optional; paths-only means existence-only gate (no section template).
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.DEFAULT_CODING_MEMO_SECTIONS = exports.FILE_ARTIFACT_TYPE_ID = void 0;
exports.parseFileArtifactPaths = parseFileArtifactPaths;
exports.isFileArtifactStructureEnabled = isFileArtifactStructureEnabled;
exports.parseFileArtifactStructure = parseFileArtifactStructure;
exports.sectionsToTextarea = sectionsToTextarea;
exports.textareaToSections = textareaToSections;
exports.withFileArtifactStructure = withFileArtifactStructure;
exports.normalizeFileArtifactConfigForSave = normalizeFileArtifactConfigForSave;
exports.emptyFileArtifactConfig = emptyFileArtifactConfig;
exports.FILE_ARTIFACT_TYPE_ID = "file_artifact.v1";
exports.DEFAULT_CODING_MEMO_SECTIONS = ["What", "Why", "Baseline"];
function parseFileArtifactPaths(configJson) {
    if (!configJson)
        return [];
    const raw = configJson.paths;
    if (!Array.isArray(raw))
        return [];
    return raw
        .filter((item) => typeof item === "string")
        .map((s) => s.trim())
        .filter(Boolean);
}
/**
 * True when config declares structure.kind markdown_sections (even if sections
 * are temporarily empty while the user edits the textarea).
 */
function isFileArtifactStructureEnabled(configJson) {
    if (!configJson)
        return false;
    const raw = configJson.structure;
    if (!raw || typeof raw !== "object" || Array.isArray(raw))
        return false;
    return raw.kind === "markdown_sections";
}
/**
 * Returns structure when kind is markdown_sections. sections may be empty
 * while editing; runner/gate ignore empty section lists.
 */
function parseFileArtifactStructure(configJson) {
    if (!isFileArtifactStructureEnabled(configJson))
        return null;
    const obj = configJson.structure;
    const sectionsRaw = obj.sections;
    const sections = Array.isArray(sectionsRaw)
        ? sectionsRaw
            .filter((item) => typeof item === "string")
            .map((s) => s.trim())
            .filter(Boolean)
        : [];
    return { kind: "markdown_sections", sections };
}
function sectionsToTextarea(sections) {
    return sections.join("\n");
}
function textareaToSections(text) {
    const seen = new Set();
    const out = [];
    for (const line of text.split("\n")) {
        const s = line.trim();
        if (!s)
            continue;
        const key = s.toLowerCase();
        if (seen.has(key))
            continue;
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
function withFileArtifactStructure(configJson, enabled, sectionsText, options) {
    const next = { ...configJson };
    delete next.format;
    if (!enabled) {
        delete next.structure;
        return next;
    }
    let sections = textareaToSections(sectionsText);
    if (sections.length === 0 && options?.fillDefaultWhenEmpty) {
        sections = [...exports.DEFAULT_CODING_MEMO_SECTIONS];
    }
    next.structure = {
        kind: "markdown_sections",
        sections,
    };
    return next;
}
/**
 * Normalize config before save: if structure is enabled but sections empty,
 * fill coding-memo defaults so schema minItems is satisfied.
 */
function normalizeFileArtifactConfigForSave(configJson) {
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
function emptyFileArtifactConfig() {
    return { paths: [] };
}
