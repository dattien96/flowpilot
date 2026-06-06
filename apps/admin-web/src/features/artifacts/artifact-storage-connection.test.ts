import { describe, expect, it } from "vitest";

import {
  isArtifactStorageConnectionReady,
} from "./artifact-storage-connection";

describe("artifact storage connection helpers", () => {
  it("treats only connected folder-backed google drive records as ready", () => {
    expect(isArtifactStorageConnectionReady(null)).toBe(false);
    expect(
      isArtifactStorageConnectionReady({
        status: "pending",
        folderId: "folder-1",
      } as never),
    ).toBe(false);
    expect(
      isArtifactStorageConnectionReady({
        status: "connected",
        folderId: "",
      } as never),
    ).toBe(false);
    expect(
      isArtifactStorageConnectionReady({
        status: "reconnect_required",
        folderId: "folder-1",
      } as never),
    ).toBe(false);
    expect(
      isArtifactStorageConnectionReady({
        status: "connected",
        folderId: "folder-1",
      } as never),
    ).toBe(true);
  });
});
