import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  resetArtifactAutoSyncState,
  scheduleArtifactAutoSync,
} from "@/features/artifacts/artifact-auto-sync";

describe("artifact-auto-sync", () => {
  function createThenableQuery(result: { data: unknown[]; error: unknown }) {
    const query = {
      eq: vi.fn(() => query),
      order: vi.fn(() => query),
      then: (
        onFulfilled?: ((value: { data: unknown[]; error: unknown }) => unknown) | null,
        onRejected?: ((reason: unknown) => unknown) | null,
      ) =>
        Promise.resolve(result).then(
          onFulfilled ?? ((value) => value),
          onRejected ?? undefined,
        ),
    };

    return query;
  }

  beforeEach(() => {
    vi.useFakeTimers();
    resetArtifactAutoSyncState();
  });

  afterEach(() => {
    resetArtifactAutoSyncState();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("throws early when called without a valid sync context", () => {
    expect(() =>
      scheduleArtifactAutoSync(
        {
          projectId: "project-1",
          workflowRunId: "run-1",
        } as never,
        {
          projectId: "project-1",
          workflowRunId: "run-1",
        },
      ),
    ).toThrow("Artifact auto-sync requires a valid sync context.");
  });

  it("schedules a debounce timer with a valid context", () => {
    const setTimeoutSpy = vi.spyOn(globalThis, "setTimeout");

    scheduleArtifactAutoSync(
      {
        supabase: {
          from: vi.fn(),
        } as never,
        artifactStorageConnectionGateway: {
          getProjectStorageProvider: vi.fn(),
          updateArtifactRunStorageMetadata: vi.fn(),
        },
      },
      {
        projectId: "project-1",
        workflowRunId: "run-1",
      },
    );

    expect(setTimeoutSpy).toHaveBeenCalledTimes(1);
    expect(setTimeoutSpy).toHaveBeenCalledWith(expect.any(Function), 5 * 60 * 1000);
  });

  it("resets the debounce timer when the same workflow run schedules auto-sync again", async () => {
    const query = createThenableQuery({ data: [], error: null });
    const selectMock = vi.fn(() => query);
    const fromMock = vi.fn(() => ({
      select: selectMock,
    }));
    const clearTimeoutSpy = vi.spyOn(globalThis, "clearTimeout");
    const context = {
      supabase: {
        from: fromMock,
      } as never,
      artifactStorageConnectionGateway: {
        getProjectStorageProvider: vi.fn(),
        updateArtifactRunStorageMetadata: vi.fn(),
      },
    };
    const filters = {
      projectId: "project-1",
      workflowRunId: "run-1",
    };

    scheduleArtifactAutoSync(context, filters);
    await vi.advanceTimersByTimeAsync(2.5 * 60 * 1000);

    scheduleArtifactAutoSync(context, filters);
    expect(clearTimeoutSpy).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(2.5 * 60 * 1000);
    expect(fromMock).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(2.5 * 60 * 1000);
    expect(fromMock).toHaveBeenCalledTimes(1);
    expect(fromMock).toHaveBeenCalledWith("artifact_runs");
    expect(selectMock).toHaveBeenCalled();
  });
});
