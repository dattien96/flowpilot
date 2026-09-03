"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const fileArtifactConfig_1 = require("./fileArtifactConfig");
(0, node_test_1.default)("parseFileArtifactStructure returns null for paths-only", () => {
    strict_1.default.equal((0, fileArtifactConfig_1.parseFileArtifactStructure)({ paths: ["a.md"] }), null);
    strict_1.default.equal((0, fileArtifactConfig_1.isFileArtifactStructureEnabled)({ paths: ["a.md"] }), false);
});
(0, node_test_1.default)("parseFileArtifactStructure reads markdown_sections", () => {
    const got = (0, fileArtifactConfig_1.parseFileArtifactStructure)({
        paths: ["docs/x.md"],
        structure: { kind: "markdown_sections", sections: ["What", "Why"] },
    });
    strict_1.default.deepEqual(got, { kind: "markdown_sections", sections: ["What", "Why"] });
    strict_1.default.equal((0, fileArtifactConfig_1.isFileArtifactStructureEnabled)({
        structure: { kind: "markdown_sections", sections: ["What"] },
    }), true);
});
(0, node_test_1.default)("structure remains enabled with empty sections while editing", () => {
    const cfg = { paths: ["a.md"], structure: { kind: "markdown_sections", sections: [] } };
    strict_1.default.equal((0, fileArtifactConfig_1.isFileArtifactStructureEnabled)(cfg), true);
    strict_1.default.deepEqual((0, fileArtifactConfig_1.parseFileArtifactStructure)(cfg), { kind: "markdown_sections", sections: [] });
});
(0, node_test_1.default)("parseFileArtifactStructure ignores format-only configs", () => {
    strict_1.default.equal((0, fileArtifactConfig_1.parseFileArtifactStructure)({ paths: ["a.md"], format: "coder_decision_memo" }), null);
});
(0, node_test_1.default)("withFileArtifactStructure disables by removing structure", () => {
    const got = (0, fileArtifactConfig_1.withFileArtifactStructure)({
        paths: ["a.md"],
        structure: { kind: "markdown_sections", sections: ["What"] },
        format: "stale",
    }, false, "What");
    strict_1.default.deepEqual(got, { paths: ["a.md"] });
    strict_1.default.equal("format" in got, false);
    strict_1.default.equal("structure" in got, false);
});
(0, node_test_1.default)("withFileArtifactStructure does not auto-fill on textarea clear", () => {
    const got = (0, fileArtifactConfig_1.withFileArtifactStructure)({ paths: ["a.md"] }, true, "  \n  ", {
        fillDefaultWhenEmpty: false,
    });
    strict_1.default.deepEqual(got.structure, { kind: "markdown_sections", sections: [] });
});
(0, node_test_1.default)("UI toggle path: enable empty then save fills defaults", () => {
    // Checkbox on → empty structure allowed in draft (no silent coding-memo inject).
    const draft = (0, fileArtifactConfig_1.withFileArtifactStructure)({ paths: ["a.md"] }, true, "", {
        fillDefaultWhenEmpty: false,
    });
    strict_1.default.equal((0, fileArtifactConfig_1.isFileArtifactStructureEnabled)(draft), true);
    strict_1.default.deepEqual((0, fileArtifactConfig_1.parseFileArtifactStructure)(draft), {
        kind: "markdown_sections",
        sections: [],
    });
    // Save → defaults become What/Why/Baseline.
    const saved = (0, fileArtifactConfig_1.normalizeFileArtifactConfigForSave)(draft);
    strict_1.default.deepEqual(saved.structure, {
        kind: "markdown_sections",
        sections: [...fileArtifactConfig_1.DEFAULT_CODING_MEMO_SECTIONS],
    });
});
(0, node_test_1.default)("withFileArtifactStructure enables with defaults when fillDefaultWhenEmpty", () => {
    const got = (0, fileArtifactConfig_1.withFileArtifactStructure)({ paths: ["a.md"] }, true, "  \n  ", {
        fillDefaultWhenEmpty: true,
    });
    strict_1.default.deepEqual(got.structure, {
        kind: "markdown_sections",
        sections: [...fileArtifactConfig_1.DEFAULT_CODING_MEMO_SECTIONS],
    });
});
(0, node_test_1.default)("normalizeFileArtifactConfigForSave fills empty structure sections", () => {
    const got = (0, fileArtifactConfig_1.normalizeFileArtifactConfigForSave)({
        paths: ["a.md"],
        structure: { kind: "markdown_sections", sections: [] },
    });
    strict_1.default.deepEqual(got.structure, {
        kind: "markdown_sections",
        sections: [...fileArtifactConfig_1.DEFAULT_CODING_MEMO_SECTIONS],
    });
});
(0, node_test_1.default)("withFileArtifactStructure parses textarea sections", () => {
    const got = (0, fileArtifactConfig_1.withFileArtifactStructure)({ paths: ["a.md"] }, true, "Summary\nCases\nResult\n");
    strict_1.default.deepEqual(got.structure, {
        kind: "markdown_sections",
        sections: ["Summary", "Cases", "Result"],
    });
});
(0, node_test_1.default)("textareaToSections dedupes case-insensitively", () => {
    strict_1.default.deepEqual((0, fileArtifactConfig_1.textareaToSections)("What\nwhat\nWhy"), ["What", "Why"]);
});
