import { describe, expect, it } from "vitest";

import { getTeamLinkDelta } from "./project-team-links";

describe("getTeamLinkDelta", () => {
  it("returns teams to link and unlink for the next selection", () => {
    const delta = getTeamLinkDelta(
      [
        {
          id: "team-1",
          name: "Platform",
          createdAt: "2026-05-18T00:00:00.000Z",
          updatedAt: "2026-05-18T00:00:00.000Z",
        },
        {
          id: "team-2",
          name: "Backend",
          createdAt: "2026-05-18T00:00:00.000Z",
          updatedAt: "2026-05-18T00:00:00.000Z",
        },
      ],
      ["team-2", "team-3"],
    );

    expect(delta).toEqual({
      toLink: ["team-3"],
      toUnlink: ["team-1"],
    });
  });

  it("deduplicates and drops blank team ids", () => {
    const delta = getTeamLinkDelta([], ["team-1", "team-1", " ", ""]);

    expect(delta).toEqual({
      toLink: ["team-1"],
      toUnlink: [],
    });
  });
});
