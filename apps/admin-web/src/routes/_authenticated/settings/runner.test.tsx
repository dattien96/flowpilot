import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { RunnerContent } from "./runner";

describe("Runner page", () => {
  it("shows the system-wide runner health summary", () => {
    render(
      <RunnerContent
        health={{
          status: "online",
          runnerVersion: "0.1.0",
          cwd: "/workspace",
          os: "darwin",
          startedAt: "2026-05-19T08:00:00.000Z",
          baseUrl: "http://127.0.0.1:4317",
          errorMessage: null,
        }}
      />,
    );

    expect(screen.getByText("Runner")).toBeInTheDocument();
    expect(screen.getByText("Local runner control plane")).toBeInTheDocument();
    expect(screen.getByText("http://127.0.0.1:4317")).toBeInTheDocument();
  });
});
