import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  reconcileArtifactSyncState,
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

  it("skips artifact sync candidates whose workflow_runs row no longer exists", async () => {
    const artifactRunsQuery = createThenableQuery({
      data: [
        {
          id: "artifact-1",
          project_id: "project-1",
          workflow_run_id: "run-missing",
          sync_status: "local_only",
          storage_provider: null,
          remote_path: null,
          remote_object_id: null,
          updated_at: "2026-06-03T10:00:00.000Z",
        },
      ],
      error: null,
    });
    const workflowRunsQuery = {
      in: vi.fn().mockResolvedValue({
        data: [],
        error: null,
      }),
    };
    const artifactRunsSelect = vi.fn(() => artifactRunsQuery);
    const workflowRunsSelect = vi.fn(() => workflowRunsQuery);
    const fromMock = vi.fn((table: string) => {
      if (table === "artifact_runs") {
        return { select: artifactRunsSelect };
      }
      if (table === "workflow_runs") {
        return { select: workflowRunsSelect };
      }
      throw new Error(`unexpected table ${table}`);
    });
    const fetchSpy = vi.spyOn(globalThis, "fetch");

    await reconcileArtifactSyncState({
      supabase: {
        from: fromMock,
      } as never,
      artifactStorageConnectionGateway: {
        getProjectStorageProvider: vi.fn(),
        updateArtifactRunStorageMetadata: vi.fn(),
      },
    });

    expect(fromMock).toHaveBeenCalledWith("artifact_runs");
    expect(fromMock).toHaveBeenCalledWith("workflow_runs");
    expect(workflowRunsQuery.in).toHaveBeenCalledWith("id", ["run-missing"]);
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
