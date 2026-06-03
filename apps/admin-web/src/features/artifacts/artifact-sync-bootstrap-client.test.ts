import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  requestArtifactSyncBootstrap,
  resetArtifactSyncBootstrapClientState,
} from "@/features/artifacts/artifact-sync-bootstrap-client";
import type { LocalRunnerHealth } from "@/domain/model/entity/local-runner";

const onlineHealth = (startedAt: string): LocalRunnerHealth => ({
  status: "online",
  runnerVersion: "1.0.0",
  cwd: "C:/working/flowpilot",
  os: "windows",
  startedAt,
  baseUrl: "http://localhost:3001",
  errorMessage: null,
});

describe("artifact sync bootstrap client", () => {
  beforeEach(() => {
    resetArtifactSyncBootstrapClientState();
  });

  it("skips offline runners", async () => {
    const fetchMock = vi.fn();

    await expect(
      requestArtifactSyncBootstrap(
        {
          ...onlineHealth("2026-06-03T10:00:00.000Z"),
          status: "offline",
        },
        fetchMock as typeof fetch,
      ),
    ).resolves.toBe(false);

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("requests bootstrap only once for the same runner start", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ started: true }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    );

    await expect(
      requestArtifactSyncBootstrap(
        onlineHealth("2026-06-03T10:00:00.000Z"),
        fetchMock as typeof fetch,
      ),
    ).resolves.toBe(true);
    await expect(
      requestArtifactSyncBootstrap(
        onlineHealth("2026-06-03T10:00:00.000Z"),
        fetchMock as typeof fetch,
      ),
    ).resolves.toBe(false);
    await expect(
      requestArtifactSyncBootstrap(
        onlineHealth("2026-06-03T10:05:00.000Z"),
        fetchMock as typeof fetch,
      ),
    ).resolves.toBe(true);

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/local-runner/artifacts/bootstrap-sync",
      expect.objectContaining({
        method: "POST",
        cache: "no-store",
      }),
    );
  });

  it("deduplicates in-flight bootstrap requests for the same runner start", async () => {
    let resolveFetch: ((response: Response) => void) | null = null;
    const fetchMock = vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          resolveFetch = resolve;
        }),
    );

    const firstRequest = requestArtifactSyncBootstrap(
      onlineHealth("2026-06-03T10:00:00.000Z"),
      fetchMock as typeof fetch,
    );
    const secondRequest = requestArtifactSyncBootstrap(
      onlineHealth("2026-06-03T10:00:00.000Z"),
      fetchMock as typeof fetch,
    );

    expect(fetchMock).toHaveBeenCalledTimes(1);
    resolveFetch?.(
      new Response(JSON.stringify({ started: true }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    );

    await expect(firstRequest).resolves.toBe(true);
    await expect(secondRequest).resolves.toBe(true);
  });

  it("allows retry when bootstrap fails", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ error: "boom" }), {
          status: 500,
          headers: { "content-type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ started: true }), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      );

    await expect(
      requestArtifactSyncBootstrap(
        onlineHealth("2026-06-03T10:00:00.000Z"),
        fetchMock as typeof fetch,
      ),
    ).rejects.toThrow("boom");
    await expect(
      requestArtifactSyncBootstrap(
        onlineHealth("2026-06-03T10:00:00.000Z"),
        fetchMock as typeof fetch,
      ),
    ).resolves.toBe(true);

    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
