import { beforeEach, describe, expect, it, vi } from "vitest";

import { GET } from "./route";

const mocks = vi.hoisted(() => ({
  createGatewayBundle: vi.fn(),
  getLocalRunnerBaseUrl: vi.fn(),
  getSupabaseUrl: vi.fn(),
  getRequiredEnv: vi.fn(),
  createClient: vi.fn(),
  download: vi.fn(),
}));

vi.mock("@/data/repository/factory", () => ({
  createGatewayBundle: mocks.createGatewayBundle,
}));

vi.mock("@/lib/env/app-env", () => ({
  getLocalRunnerBaseUrl: mocks.getLocalRunnerBaseUrl,
  getSupabaseUrl: mocks.getSupabaseUrl,
  getRequiredEnv: mocks.getRequiredEnv,
}));

vi.mock("@supabase/supabase-js", () => ({
  createClient: mocks.createClient,
}));

describe("artifact open route", () => {
  beforeEach(() => {
    mocks.getLocalRunnerBaseUrl.mockReturnValue("http://127.0.0.1:4317");
    mocks.getSupabaseUrl.mockReturnValue("https://example.supabase.co");
    mocks.getRequiredEnv.mockReturnValue("service-role-key");
    mocks.createClient.mockReturnValue({
      storage: {
        from: vi.fn(() => ({
          download: mocks.download,
        })),
      },
    });
    mocks.download.mockReset();
  });

  it("streams remote Supabase artifacts inline without requiring auth", async () => {
    mocks.download.mockResolvedValue({
      data: {
        type: "text/plain; charset=utf-8",
        text: async () => "# Plan",
        arrayBuffer: async () => new TextEncoder().encode("# Plan").buffer,
      },
      error: null,
    });
    mocks.createGatewayBundle.mockResolvedValue({
      localRunnerGateway: {
        getArtifactById: vi.fn().mockResolvedValue({
          artifactId: "artifact-1",
          title: "Plan",
          remotePath: "projects/project-alpha/runs/run-1/steps/plan/Plan.md",
          remoteUrl: "",
          storageProvider: "supabase",
          contentMarkdown: "# Plan",
        }),
      },
    });

    const response = await GET(new Request("http://localhost", {
      headers: { "accept": "text/html" }
    }), {
      params: Promise.resolve({ artifactId: "artifact-1" }),
    });

    expect(mocks.download).toHaveBeenCalledWith(
      "projects/project-alpha/runs/run-1/steps/plan/Plan.md",
    );
    expect(response.status).toBe(200);
    expect(response.headers.get("content-type")).toContain("text/html");
    expect(response.headers.get("content-disposition")).toContain('inline; filename="Plan.md.html"');
    expect(await response.text()).toContain("Plan");
  });

  it("streams the requested remote companion file inline for Supabase artifacts", async () => {
    mocks.download.mockResolvedValue({
      data: {
        type: "text/plain; charset=utf-8",
        text: async () => "Expanded plan prompt",
        arrayBuffer: async () => new TextEncoder().encode("Expanded plan prompt").buffer,
      },
      error: null,
    });
    mocks.createGatewayBundle.mockResolvedValue({
      localRunnerGateway: {
        getArtifactById: vi.fn().mockResolvedValue({
          artifactId: "artifact-1",
          title: "Plan",
          remotePath: "projects/project-alpha/runs/run-1/steps/plan/Plan.md",
          remoteUrl: "",
          storageProvider: "supabase",
          contentMarkdown: "# Plan",
          actualPromptText: "Expanded plan prompt",
          promptText: "Base plan prompt",
        }),
      },
    });

    const response = await GET(new Request("http://localhost?file=actual-prompt", {
      headers: { "accept": "text/html" }
    }), {
      params: Promise.resolve({ artifactId: "artifact-1" }),
    });

    expect(mocks.download).toHaveBeenCalledWith(
      "projects/project-alpha/runs/run-1/steps/plan/.snapshots/artifact-1/actual-prompt.md",
    );
    expect(response.status).toBe(200);
    expect(response.headers.get("content-type")).toContain("text/html");
    expect(await response.text()).toContain("Expanded plan prompt");
  });

  it("returns inline HTML preview when the artifact has no remote target", async () => {
    mocks.createGatewayBundle.mockResolvedValue({
      localRunnerGateway: {
        getArtifactById: vi.fn().mockResolvedValue({
          artifactId: "artifact-1",
          title: "Plan",
          remotePath: "",
          remoteUrl: "",
          storageProvider: null,
          contentMarkdown: "# Plan",
        }),
      },
    });

    const response = await GET(new Request("http://localhost", {
      headers: { "accept": "text/html" }
    }), {
      params: Promise.resolve({ artifactId: "artifact-1" }),
    });

    expect(response.status).toBe(200);
    expect(response.headers.get("content-type")).toContain("text/html");
    expect(await response.text()).toContain("Plan");
  });

  it("returns inline actual prompt HTML preview for local-only artifacts", async () => {
    mocks.createGatewayBundle.mockResolvedValue({
      localRunnerGateway: {
        getArtifactById: vi.fn().mockResolvedValue({
          artifactId: "artifact-1",
          title: "Plan",
          remotePath: "",
          remoteUrl: "",
          storageProvider: null,
          contentMarkdown: "# Plan",
          promptText: "Base prompt",
          actualPromptText: "Expanded prompt",
        }),
      },
    });

    const response = await GET(new Request("http://localhost?file=actual-prompt", {
      headers: { "accept": "text/html" }
    }), {
      params: Promise.resolve({ artifactId: "artifact-1" }),
    });

    expect(response.status).toBe(200);
    expect(response.headers.get("content-type")).toContain("text/html");
    expect(response.headers.get("content-disposition")).toContain("actual-prompt.md.html");
    expect(await response.text()).toContain("Expanded prompt");
  });

  it("streams bucket-only remote artifacts inline as HTML from Supabase", async () => {
    mocks.download.mockResolvedValue({
      data: {
        type: "text/plain; charset=utf-8",
        text: async () => "# Remote",
        arrayBuffer: async () => new TextEncoder().encode("# Remote").buffer,
      },
      error: null,
    });
    mocks.createGatewayBundle.mockResolvedValue({
      localRunnerGateway: {
        getArtifactById: vi.fn().mockResolvedValue(null),
      },
    });

    const response = await GET(
      new Request(
        "http://localhost?remotePath=projects/project-1/runs/run-1/steps/codex_test/Response.md&storageProvider=supabase",
        { headers: { "accept": "text/html" } }
      ),
      {
        params: Promise.resolve({
          artifactId: "remote:projects/project-1/runs/run-1/steps/codex_test/Response.md",
        }),
      },
    );

    expect(mocks.download).toHaveBeenCalledWith(
      "projects/project-1/runs/run-1/steps/codex_test/Response.md",
    );
    expect(response.status).toBe(200);
    expect(response.headers.get("content-type")).toContain("text/html");
    expect(await response.text()).toContain("Remote");
  });

  it("returns raw markdown text when the accept header does not include text/html", async () => {
    mocks.createGatewayBundle.mockResolvedValue({
      localRunnerGateway: {
        getArtifactById: vi.fn().mockResolvedValue({
          artifactId: "artifact-1",
          title: "Plan",
          remotePath: "",
          remoteUrl: "",
          storageProvider: null,
          contentMarkdown: "# Plan",
        }),
      },
    });

    const response = await GET(new Request("http://localhost"), {
      params: Promise.resolve({ artifactId: "artifact-1" }),
    });

    expect(response.status).toBe(200);
    expect(response.headers.get("content-type")).toContain("text/markdown");
    expect(await response.text()).toBe("# Plan");
  });
});
