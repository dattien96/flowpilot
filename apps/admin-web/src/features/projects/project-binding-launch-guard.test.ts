import { describe, expect, it, vi } from "vitest";

import {
  buildProjectBindingLaunchFailureMessage,
  ensureProjectHasUsableBinding,
} from "./project-binding-launch-guard";

vi.mock("@/lib/env/browser-env", () => ({
  getLocalRunnerBaseUrl: () => "http://127.0.0.1:4317",
}));

describe("project-binding-launch-guard", () => {
  it("formats root-cause guidance for invalid bindings", () => {
    const message = buildProjectBindingLaunchFailureMessage(
      [
        {
          path: "/workspace/missing",
          usable: false,
          reason: "path does not exist",
        },
        {
          path: "/workspace/file.txt",
          usable: false,
          reason: "path is not a directory",
        },
      ],
      "/projects/project-alpha/directory-bindings",
    );

    expect(message).toContain("Unable to start execution");
    expect(message).toContain("/workspace/missing: path does not exist");
    expect(message).toContain("/projects/project-alpha/directory-bindings");
  });

  it("fails fast when no bindings exist", async () => {
    await expect(ensureProjectHasUsableBinding("project-alpha", [])).rejects.toThrow(
      "/projects/project-alpha/directory-bindings",
    );
  });

  it("accepts the first usable binding", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            path: "/workspace/alpha",
            usable: true,
            reason: "",
          }),
          { status: 200 },
        ),
      );

    await expect(
      ensureProjectHasUsableBinding("project-alpha", [
        {
          id: "binding-1",
          projectId: "project-alpha",
          localPath: "/workspace/alpha",
          label: "Primary",
          createdAt: "2026-05-22T00:00:00.000Z",
          updatedAt: "2026-05-22T00:00:00.000Z",
        },
      ]),
    ).resolves.toBe("/workspace/alpha");

    fetchMock.mockRestore();
  });
});
