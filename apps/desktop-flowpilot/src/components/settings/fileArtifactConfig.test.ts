import test from "node:test";
import assert from "node:assert/strict";
import {
  DEFAULT_CODING_MEMO_SECTIONS,
  isFileArtifactStructureEnabled,
  normalizeFileArtifactConfigForSave,
  parseFileArtifactStructure,
  textareaToSections,
  withFileArtifactStructure,
} from "./fileArtifactConfig";

test("parseFileArtifactStructure returns null for paths-only", () => {
  assert.equal(parseFileArtifactStructure({ paths: ["a.md"] }), null);
  assert.equal(isFileArtifactStructureEnabled({ paths: ["a.md"] }), false);
});

test("parseFileArtifactStructure reads markdown_sections", () => {
  const got = parseFileArtifactStructure({
    paths: ["docs/x.md"],
    structure: { kind: "markdown_sections", sections: ["What", "Why"] },
  });
  assert.deepEqual(got, { kind: "markdown_sections", sections: ["What", "Why"] });
  assert.equal(
    isFileArtifactStructureEnabled({
      structure: { kind: "markdown_sections", sections: ["What"] },
    }),
    true,
  );
});

test("structure remains enabled with empty sections while editing", () => {
  const cfg = { paths: ["a.md"], structure: { kind: "markdown_sections", sections: [] } };
  assert.equal(isFileArtifactStructureEnabled(cfg), true);
  assert.deepEqual(parseFileArtifactStructure(cfg), { kind: "markdown_sections", sections: [] });
});

test("parseFileArtifactStructure ignores format-only configs", () => {
  assert.equal(
    parseFileArtifactStructure({ paths: ["a.md"], format: "coder_decision_memo" }),
    null,
  );
});

test("withFileArtifactStructure disables by removing structure", () => {
  const got = withFileArtifactStructure(
    {
      paths: ["a.md"],
      structure: { kind: "markdown_sections", sections: ["What"] },
      format: "stale",
    },
    false,
    "What",
  );
  assert.deepEqual(got, { paths: ["a.md"] });
  assert.equal("format" in got, false);
  assert.equal("structure" in got, false);
});

test("withFileArtifactStructure does not auto-fill on textarea clear", () => {
  const got = withFileArtifactStructure({ paths: ["a.md"] }, true, "  \n  ", {
    fillDefaultWhenEmpty: false,
  });
  assert.deepEqual(got.structure, { kind: "markdown_sections", sections: [] });
});

test("UI toggle path: enable empty then save fills defaults", () => {
  // Checkbox on → empty structure allowed in draft (no silent coding-memo inject).
  const draft = withFileArtifactStructure({ paths: ["a.md"] }, true, "", {
    fillDefaultWhenEmpty: false,
  });
  assert.equal(isFileArtifactStructureEnabled(draft), true);
  assert.deepEqual(parseFileArtifactStructure(draft), {
    kind: "markdown_sections",
    sections: [],
  });
  // Save → defaults become What/Why/Baseline.
  const saved = normalizeFileArtifactConfigForSave(draft);
  assert.deepEqual(saved.structure, {
    kind: "markdown_sections",
    sections: [...DEFAULT_CODING_MEMO_SECTIONS],
  });
});

test("withFileArtifactStructure enables with defaults when fillDefaultWhenEmpty", () => {
  const got = withFileArtifactStructure({ paths: ["a.md"] }, true, "  \n  ", {
    fillDefaultWhenEmpty: true,
  });
  assert.deepEqual(got.structure, {
    kind: "markdown_sections",
    sections: [...DEFAULT_CODING_MEMO_SECTIONS],
  });
});

test("normalizeFileArtifactConfigForSave fills empty structure sections", () => {
  const got = normalizeFileArtifactConfigForSave({
    paths: ["a.md"],
    structure: { kind: "markdown_sections", sections: [] },
  });
  assert.deepEqual(got.structure, {
    kind: "markdown_sections",
    sections: [...DEFAULT_CODING_MEMO_SECTIONS],
  });
});

test("withFileArtifactStructure parses textarea sections", () => {
  const got = withFileArtifactStructure({ paths: ["a.md"] }, true, "Summary\nCases\nResult\n");
  assert.deepEqual(got.structure, {
    kind: "markdown_sections",
    sections: ["Summary", "Cases", "Result"],
  });
});

test("textareaToSections dedupes case-insensitively", () => {
  assert.deepEqual(textareaToSections("What\nwhat\nWhy"), ["What", "Why"]);
});
