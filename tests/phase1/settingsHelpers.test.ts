import test from "node:test";
import assert from "node:assert/strict";

import { validateDirectoryBindingsInOrder } from "../../apps/desktop-flowpilot/src/components/settings/settingsHelpers";
import type { DirectoryRepository, DirectorySelectionResult, ProjectWorkspaceBinding } from "../../packages/flowpilot-client-core/src";

class FakeDirectoryRepository implements DirectoryRepository {
  constructor(
    private readonly results: Record<string, { usable: boolean; reason: string }>,
  ) {}

  // The phase1 light-weight fake never drives the real directory picker
  // dialog; keeping the signature aligned with the interface (Task-316
  // phase1 runner debt follow-up) without a DOM dependency.
  async pickDirectory(): Promise<DirectorySelectionResult> {
    throw new Error("pickDirectory not used in this fixture");
  }

  async validatePath(path: string) {
    const result = this.results[path];
    if (!result) {
      throw new Error(`unexpected path ${path}`);
    }
    return { path, ...result };
  }
}

function binding(localPath: string): ProjectWorkspaceBinding {
  return {
    id: localPath,
    projectId: "project-1",
    localPath,
    label: null,
    createdAt: "",
    updatedAt: "",
  };
}

test("validateDirectoryBindingsInOrder returns the first usable binding in configured order", async () => {
  const directories = new FakeDirectoryRepository({
    "/workspace/a": { usable: false, reason: "missing" },
    "/workspace/b": { usable: true, reason: "" },
    "/workspace/c": { usable: true, reason: "" },
  });

  const result = await validateDirectoryBindingsInOrder(
    [binding("/workspace/a"), binding("/workspace/b"), binding("/workspace/c")],
    directories,
  );

  assert.equal(result, "/workspace/b");
});

test("validateDirectoryBindingsInOrder reports every checked binding when none are usable", async () => {
  const directories = new FakeDirectoryRepository({
    "/workspace/a": { usable: false, reason: "missing" },
    "/workspace/b": { usable: false, reason: "not a directory" },
  });

  await assert.rejects(
    () =>
      validateDirectoryBindingsInOrder(
        [binding("/workspace/a"), binding("/workspace/b")],
        directories,
      ),
    /\/workspace\/a: missing[\s\S]*\/workspace\/b: not a directory/,
  );
});
