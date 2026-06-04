import { describe, expect, it } from "vitest";

import { formatTaskBoardStatus, parseTaskBreakdownMarkdown } from "./task-breakdown-parser";

describe("taskBreakdownParser", () => {
  it("parses headings, checklist items, and inline metadata", () => {
    const parsed = parseTaskBreakdownMarkdown(`
# Milestone 1

- [ ] Build task board | assignee: Maya | estimate: 3h | depends on: Setup
- [x] Wire schedule summary | owner: Noah | effort: 1.5h

## Milestone 2

1. Improve filters (in progress) assignee: Pia, hours: 2
`);

    expect(parsed.title).toBe("Milestone 1");
    expect(parsed.sections).toHaveLength(2);
    expect(parsed.tasks).toHaveLength(3);
    expect(parsed.tasks[0]?.status).toBe("backlog");
    expect(parsed.tasks[0]?.assignee).toBe("Maya");
    expect(parsed.tasks[0]?.estimateHours).toBe(3);
    expect(parsed.tasks[0]?.dependencies).toEqual(["Setup"]);
    expect(parsed.tasks[1]?.status).toBe("done");
    expect(parsed.tasks[2]?.status).toBe("in_progress");
    expect(formatTaskBoardStatus("in_progress")).toBe("In Progress");
  });

  it("creates a fallback task when no list items are present", () => {
    const parsed = parseTaskBreakdownMarkdown("Just a plain markdown paragraph.");

    expect(parsed.tasks).toHaveLength(1);
    expect(parsed.tasks[0]?.title).toContain("Just a plain markdown paragraph");
    expect(parsed.sections).toHaveLength(1);
  });
});
