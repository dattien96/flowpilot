import { describe, expect, it } from "vitest";

import {
  getNextPromptGroupVisibleCount,
  getVisiblePromptGroupSlice,
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
});
