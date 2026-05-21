import { describe, expect, it } from "vitest";

import { settingsNavItems } from "@/components/layout/app-nav";
import { createSettingsNavStatuses } from "@/components/layout/app-shell";
import { buildMcpServerStatus, buildRunnerStatus } from "@/components/layout/app-shell-status";

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

  it("builds the MCP menu count from connected and total instances", () => {
    expect(
      buildMcpServerStatus([
        {
          id: "1",
          projectId: "project-a",
          type: "jira",
          label: "Jira A",
          configEncrypted: {},
          status: "connected",
          lastSyncedAt: null,
          lastError: null,
          createdAt: "",
          updatedAt: "",
        },
        {
          id: "2",
          projectId: "project-b",
          type: "google_drive",
          label: "Drive B",
          configEncrypted: {},
          status: "failed",
          lastSyncedAt: null,
          lastError: null,
          createdAt: "",
          updatedAt: "",
        },
      ]).label,
    ).toBe("1/2");
  });

  it("builds the runner menu status with online and offline labels", () => {
    expect(
      buildRunnerStatus({
        status: "online",
        runnerVersion: null,
        cwd: null,
        os: null,
        startedAt: null,
        baseUrl: "http://localhost:3001",
        errorMessage: null,
      }),
    ).toMatchObject({
      label: "Online",
      online: true,
      toneClass: "bg-success",
      textClass: "text-success",
      activeToneClass: "bg-emerald-300",
      activeTextClass: "text-emerald-50",
    });

    expect(
      buildRunnerStatus({
        status: "offline",
        runnerVersion: null,
        cwd: null,
        os: null,
        startedAt: null,
        baseUrl: "http://localhost:3001",
        errorMessage: "offline",
      }),
    ).toMatchObject({
      label: "Offline",
      online: false,
      toneClass: "bg-danger",
      textClass: "text-danger",
      activeToneClass: "bg-rose-300",
      activeTextClass: "text-rose-50",
    });
  });

  it("maps the settings menu statuses to the expected routes", () => {
    const statuses = createSettingsNavStatuses(
      [
        {
          id: "1",
          projectId: "project-a",
          type: "jira",
          label: "Jira A",
          configEncrypted: {},
          status: "connected",
          lastSyncedAt: null,
          lastError: null,
          createdAt: "",
          updatedAt: "",
        },
      ],
      {
        status: "offline",
        runnerVersion: null,
        cwd: null,
        os: null,
        startedAt: null,
        baseUrl: "http://localhost:3001",
        errorMessage: null,
      },
    );

    expect(statuses["/settings/mcp-servers"]).toMatchObject({
      kind: "count",
      label: "1/1",
    });
    expect(statuses["/settings/runner"]).toMatchObject({
      kind: "runner",
      label: "Offline",
      toneClass: "bg-danger",
      textClass: "text-danger",
      activeToneClass: "bg-rose-300",
      activeTextClass: "text-rose-50",
    });
  });
});
