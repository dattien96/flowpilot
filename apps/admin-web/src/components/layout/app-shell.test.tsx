import { describe, expect, it } from "vitest";

import { settingsNavItems } from "@/components/layout/app-nav";

describe("AppShell", () => {
  it("includes the MCP Servers entry in the settings navigation", () => {
    expect(
      settingsNavItems.some(
        (item) => item.to === "/settings/mcp-servers" && item.label === "MCP Servers",
      ),
    ).toBe(true);
    expect(
      settingsNavItems.some((item) => item.to === "/settings/runner" && item.label === "Runner"),
    ).toBe(true);
  });
});
