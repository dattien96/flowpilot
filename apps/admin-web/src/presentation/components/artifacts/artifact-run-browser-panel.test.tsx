// @vitest-environment jsdom
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ButtonHTMLAttributes, ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { LocalRunnerArtifact } from "@/domain/model/entity/local-runner";
import type { ArtifactRun } from "@/domain/model/entity/workflow-engine";

const { fetchMock, locationAssign, onArtifactsChanged } = vi.hoisted(() => ({
  fetchMock: vi.fn(),
  locationAssign: vi.fn(),
  onArtifactsChanged: vi.fn(),
}));

vi.mock("@/presentation/components/ui/badge", () => ({
  Badge: ({ children }: { children: ReactNode }) => <span>{children}</span>,
}));

vi.mock("@/presentation/components/ui/button", () => ({
  Button: ({ children, ...props }: ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
}));

vi.mock("@/presentation/components/ui/card", () => ({
  Card: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

import { ArtifactRunBrowserPanel } from "./artifact-run-browser-panel";

describe("ArtifactRunBrowserPanel", () => {
  const localArtifact: LocalRunnerArtifact = {
    artifactId: "artifact-local-1",
    title: "Response.md",
    sourceKind: "workflow_output",
    projectId: "project-1",
    workflowRunId: "run-1",
    workflowStepKey: "step-1",
    providerKey: "codex",
    localPath: "/tmp/Response.md",
    remotePath: "projects/project-1/runs/run-1/Response.md",
    remoteUrl: "https://example.com/Response.md",
    syncStatus: "local_only",
    createdAt: "2026-06-02T00:00:00.000Z",
    updatedAt: "2026-06-02T00:00:00.000Z",
    contentMarkdown:
      "This is a long artifact response title that should be truncated after fifty characters when shown in the card.",
    previewMarkdown:
      "This is a long artifact response title that should be truncated after fifty characters when shown in the card.",
    promptText: "Original prompt text",
    actualPromptText: "Actual prompt sent to provider",
  };

  const remoteFailedArtifact: ArtifactRun = {
    id: "artifact-remote-1",
    artifactDefinitionKey: "workflow_output",
    workflowId: "workflow-1",
    workflowRunId: "run-1",
    workflowRunStepId: "step-1",
    projectId: "project-1",
    title: "Failed remote artifact",
    localPath: "/tmp/remote-failed.md",
    remotePath: "projects/project-1/runs/run-1/remote-failed.md",
    remoteUrl: "https://example.com/remote-failed.md",
    storageProvider: "supabase",
    remoteObjectId: "object-1",
    syncStatus: "failed",
    createdAt: "2026-06-02T00:00:00.000Z",
    updatedAt: "2026-06-02T00:00:00.000Z",
  };

  const remoteUnboundArtifact: ArtifactRun = {
    id: "artifact-remote-2",
    artifactDefinitionKey: null,
    workflowId: "workflow-1",
    workflowRunId: "run-3",
    workflowRunStepId: "step-3",
    projectId: "project-1",
    title: "Recovered response",
    localPath: ".flowpilot/artifacts/project-1/run-3/step-3/.snapshots/artifact-remote-2/Response.md",
    remotePath: "project-1/runs/run-3/steps/step-3/artifact-remote-2/Response.md",
    remoteUrl: "https://example.com/recovered-response.md",
    storageProvider: "supabase",
    remoteObjectId: "object-2",
    syncStatus: "synced",
    createdAt: "2026-06-02T00:00:00.000Z",
    updatedAt: "2026-06-02T00:00:00.000Z",
  };

  const syncedLocalArtifact: LocalRunnerArtifact = {
    ...localArtifact,
    artifactId: "artifact-local-synced",
    syncStatus: "synced",
    storageProvider: "supabase",
    remoteObjectId: "object-synced-1",
    remotePath: "projects/project-1/runs/run-2/Response.md",
    workflowRunId: "run-2",
  };

  beforeEach(() => {
    fetchMock.mockReset();
    locationAssign.mockReset();
    onArtifactsChanged.mockReset();
    vi.stubGlobal("fetch", fetchMock);
    Object.defineProperty(window, "location", {
      configurable: true,
      value: {
        assign: locationAssign,
      },
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("syncs all eligible artifacts with one bulk action", async () => {
    fetchMock
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({
          syncStatus: "synced",
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn().mockResolvedValue({
          syncStatus: "synced",
        }),
      });

    render(
      <ArtifactRunBrowserPanel
        artifactRuns={[remoteFailedArtifact]}
        localArtifacts={[localArtifact]}
        onArtifactsChanged={onArtifactsChanged}
        projects={[]}
        scopeLabel="Artifacts"
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /sync artifacts/i }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2);
      expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/local-runner/artifacts/artifact-local-1/sync", {
        method: "POST",
      });
      expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/local-runner/artifacts/artifact-remote-1/sync", {
        method: "POST",
      });
      expect(onArtifactsChanged).toHaveBeenCalledTimes(1);
    });
  });

  it("redirects to login when syncing hits an auth error", async () => {
    fetchMock.mockResolvedValue({
      ok: false,
      status: 401,
      json: vi.fn().mockResolvedValue({
        error: "Authentication required.",
      }),
    });

    render(
      <ArtifactRunBrowserPanel
        artifactRuns={[]}
        localArtifacts={[localArtifact]}
        onArtifactsChanged={onArtifactsChanged}
        projects={[]}
        scopeLabel="Artifacts"
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /sync artifacts/i }));

    await waitFor(() => {
      expect(locationAssign).toHaveBeenCalledWith("/login");
    });

    expect(onArtifactsChanged).not.toHaveBeenCalled();
  });

  it("uses a 50 character content excerpt as the display title", () => {
    render(
      <ArtifactRunBrowserPanel
        artifactRuns={[]}
        localArtifacts={[localArtifact]}
        projects={[]}
        scopeLabel="Artifacts"
      />,
    );

    expect(
      screen.getByText("This is a long artifact response title that should..."),
    ).toBeInTheDocument();
  });

  it("shows actual prompt details in the artifact item", () => {
    render(
      <ArtifactRunBrowserPanel
        artifactRuns={[]}
        localArtifacts={[localArtifact]}
        projects={[]}
        scopeLabel="Artifacts"
      />,
    );

    expect(screen.getByText("actual-prompt.md")).toBeInTheDocument();
    expect(screen.getByText("prompt.md")).toBeInTheDocument();
    expect(screen.getByText(/Actual prompt:/)).toBeInTheDocument();
    expect(screen.getByText("Actual Prompt")).toBeInTheDocument();
    expect(screen.getByText("Actual prompt sent to provider")).toBeInTheDocument();
    expect(screen.getByText("Saved Prompt")).toBeInTheDocument();
    expect(screen.getByText("Original prompt text")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open Response.md" })).toHaveAttribute(
      "href",
      "/api/local-runner/artifacts/artifact-local-1/open",
    );
    expect(
      screen.getByRole("link", { name: "Open actual-prompt.md" }),
    ).toHaveAttribute(
      "href",
      "/api/local-runner/artifacts/artifact-local-1/open?file=actual-prompt",
    );
    expect(screen.getByRole("link", { name: "Open prompt.md" })).toHaveAttribute(
      "href",
      "/api/local-runner/artifacts/artifact-local-1/open?file=prompt",
    );
  });

  it("moves synced local artifacts into the remote tab even without a remote artifact row", () => {
    render(
      <ArtifactRunBrowserPanel
        artifactRuns={[]}
        localArtifacts={[syncedLocalArtifact]}
        projects={[]}
        scopeLabel="Artifacts"
      />,
    );

    expect(screen.getByRole("button", { name: "local/NotSyned (0)" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remote/Sync (1)" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /sync artifacts \(0\)/i })).toBeDisabled();
  });

  it("keeps workflow-backed local_only artifacts in the local tab", () => {
    render(
      <ArtifactRunBrowserPanel
        artifactRuns={[
          {
            ...remoteFailedArtifact,
            id: "artifact-local-1",
            syncStatus: "local_only",
            storageProvider: null,
            remoteObjectId: null,
            remotePath: "projects/project-1/runs/run-1/steps/step-1/Response.md",
          },
        ]}
        localArtifacts={[localArtifact]}
        projects={[]}
        scopeLabel="Artifacts"
      />,
    );

    expect(screen.getByRole("button", { name: "local/NotSyned (1)" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remote/Sync (0)" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /sync artifacts \(1\)/i })).toBeInTheDocument();
  });

  it("renders unbound when a synced artifact has no artifact definition key", () => {
    render(
      <ArtifactRunBrowserPanel
        artifactRuns={[remoteUnboundArtifact]}
        localArtifacts={[]}
        projects={[]}
        scopeLabel="Artifacts"
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Remote/Sync (1)" }));

    expect(screen.getByText("Recovered response")).toBeInTheDocument();
    expect(screen.getByText("unbound")).toBeInTheDocument();
  });

  it("includes remote metadata in open links for remote-only artifacts", () => {
    render(
      <ArtifactRunBrowserPanel
        artifactRuns={[remoteFailedArtifact]}
        localArtifacts={[]}
        projects={[]}
        scopeLabel="Artifacts"
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Remote/Sync (1)" }));

    expect(screen.getByRole("link", { name: "Open Response.md" })).toHaveAttribute(
      "href",
      "/api/local-runner/artifacts/artifact-remote-1/open?remotePath=projects%2Fproject-1%2Fruns%2Frun-1%2Fremote-failed.md&storageProvider=supabase&remoteObjectId=object-1&projectId=project-1",
    );
  });

  it("hydrates a remote-only generic title from remote content", async () => {
    const loadRemoteArtifactContent = vi.fn(async () =>
      "This is a synced remote response title that should be shortened for cards.",
    );

    render(
      <ArtifactRunBrowserPanel
        artifactRuns={[
          {
            ...remoteFailedArtifact,
            id: "artifact-remote-generic",
            title: "Response.md",
          },
        ]}
        localArtifacts={[]}
        loadRemoteArtifactContent={loadRemoteArtifactContent}
        projects={[]}
        scopeLabel="Artifacts"
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Remote/Sync (1)" }));

    await waitFor(() => {
      expect(
        screen.getByText("This is a synced remote response title that should..."),
      ).toBeInTheDocument();
    });

    expect(loadRemoteArtifactContent).toHaveBeenCalledWith(
      expect.objectContaining({
        id: "artifact-remote-generic",
        title: "Response.md",
      }),
    );
  });

  it("disables the sync button while background sync is already in progress", () => {
    render(
      <ArtifactRunBrowserPanel
        artifactRuns={[
          {
            ...remoteFailedArtifact,
            id: "artifact-local-1",
            syncStatus: "syncing",
            storageProvider: "supabase",
            remoteObjectId: null,
            remotePath: "",
          },
        ]}
        localArtifacts={[localArtifact]}
        projects={[]}
        scopeLabel="Artifacts"
      />,
    );

    expect(screen.getByRole("button", { name: /syncing\.\.\./i })).toBeDisabled();
  });

  it("does not poll refresh forever for local_only artifacts", async () => {
    vi.useFakeTimers();
    render(
      <ArtifactRunBrowserPanel
        artifactRuns={[]}
        localArtifacts={[localArtifact]}
        onArtifactsChanged={onArtifactsChanged}
        projects={[]}
        scopeLabel="Artifacts"
      />,
    );

    await vi.advanceTimersByTimeAsync(15_000);

    expect(onArtifactsChanged).not.toHaveBeenCalled();
    vi.useRealTimers();
  });

  it("polls refresh while an artifact is syncing", async () => {
    vi.useFakeTimers();
    render(
      <ArtifactRunBrowserPanel
        artifactRuns={[
          {
            ...remoteFailedArtifact,
            id: "artifact-local-1",
            syncStatus: "syncing",
            storageProvider: "supabase",
            remoteObjectId: null,
            remotePath: "",
          },
        ]}
        localArtifacts={[localArtifact]}
        onArtifactsChanged={onArtifactsChanged}
        projects={[]}
        scopeLabel="Artifacts"
      />,
    );

    await vi.advanceTimersByTimeAsync(15_000);

    expect(onArtifactsChanged).toHaveBeenCalledTimes(3);
    vi.useRealTimers();
  });
});
