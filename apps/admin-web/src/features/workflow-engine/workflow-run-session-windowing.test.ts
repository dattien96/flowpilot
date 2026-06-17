import { describe, expect, it } from "vitest";

import {
  getNextPromptGroupVisibleCount,
  getTotalStepPromptGroupCount,
  getVisiblePromptGroupSlice,
  getVisibleStepSessionGroupSlice,
  SESSION_PROMPT_GROUP_PAGE_SIZE,
} from "./workflow-run-session-windowing";

describe("workflow-run-session-windowing", () => {
  it("keeps the most recent prompt groups visible for long sessions", () => {
    const promptGroups = Array.from({ length: 10 }, (_, index) => `prompt-${index + 1}`);

    expect(
      getVisiblePromptGroupSlice(promptGroups, SESSION_PROMPT_GROUP_PAGE_SIZE),
    ).toEqual([
      "prompt-5",
      "prompt-6",
      "prompt-7",
      "prompt-8",
      "prompt-9",
      "prompt-10",
    ]);
  });

  it("returns every prompt group when the visible count reaches the total", () => {
    const promptGroups = ["prompt-1", "prompt-2", "prompt-3"];

    expect(
      getVisiblePromptGroupSlice(promptGroups, SESSION_PROMPT_GROUP_PAGE_SIZE),
    ).toEqual(promptGroups);
  });

  it("loads earlier prompt groups one page at a time without exceeding the total", () => {
    expect(
      getNextPromptGroupVisibleCount({
        currentCount: SESSION_PROMPT_GROUP_PAGE_SIZE,
        totalCount: 14,
      }),
    ).toBe(12);

    expect(
      getNextPromptGroupVisibleCount({
        currentCount: 12,
        totalCount: 14,
      }),
    ).toBe(14);
  });

  describe("getTotalStepPromptGroupCount", () => {
    it("sums prompt groups across all session groups", () => {
      const sessionGroups = [
        { promptGroups: ["a", "b"] },
        { promptGroups: ["c"] },
        { promptGroups: ["d", "e", "f"] },
      ];
      expect(getTotalStepPromptGroupCount(sessionGroups)).toBe(6);
    });

    it("returns zero for an empty session group list", () => {
      expect(getTotalStepPromptGroupCount([])).toBe(0);
    });
  });

  describe("getVisibleStepSessionGroupSlice", () => {
    it("shows the last N prompt groups across sessions when total exceeds the visible count", () => {
      const sessionGroups = [
        { key: "s1", promptGroups: ["pg-1", "pg-2"] },
        { key: "s2", promptGroups: ["pg-3"] },
        { key: "s3", promptGroups: ["pg-4"] },
        { key: "s4", promptGroups: ["pg-5"] },
        { key: "s5", promptGroups: ["pg-6"] },
        { key: "s6", promptGroups: ["pg-7"] },
        { key: "s7", promptGroups: ["pg-8"] },
      ];

      const result = getVisibleStepSessionGroupSlice(sessionGroups, SESSION_PROMPT_GROUP_PAGE_SIZE);

      expect(result.flatMap((g) => g.promptGroups)).toEqual([
        "pg-3", "pg-4", "pg-5", "pg-6", "pg-7", "pg-8",
      ]);
    });

    it("returns all session groups when total is within the visible count", () => {
      const sessionGroups = [
        { key: "s1", promptGroups: ["pg-1"] },
        { key: "s2", promptGroups: ["pg-2"] },
      ];
      expect(getVisibleStepSessionGroupSlice(sessionGroups, SESSION_PROMPT_GROUP_PAGE_SIZE)).toEqual(sessionGroups);
    });

    it("splits a multi-prompt session when it straddles the visibility boundary", () => {
      const sessionGroups = [
        { key: "s1", promptGroups: ["pg-1", "pg-2", "pg-3", "pg-4", "pg-5"] },
        { key: "s2", promptGroups: ["pg-6", "pg-7", "pg-8"] },
      ];

      const result = getVisibleStepSessionGroupSlice(sessionGroups, SESSION_PROMPT_GROUP_PAGE_SIZE);

      expect(result.flatMap((g) => g.promptGroups)).toEqual([
        "pg-3", "pg-4", "pg-5", "pg-6", "pg-7", "pg-8",
      ]);
    });

    it("returns an empty array for an empty session group list", () => {
      expect(getVisibleStepSessionGroupSlice([], SESSION_PROMPT_GROUP_PAGE_SIZE)).toEqual([]);
    });
  });
});
