import { afterEach, describe, expect, it, vi } from "vitest";

import { HttpLocalRunnerGateway } from "./http-local-runner-gateway";

describe("HttpLocalRunnerGateway integration connection", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("posts a test request to the integration connection endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          requestStatus: "accepted",
          integrationId: "integration-1",
          integrationStatus: "pending",
          runId: null,
          message: "queued",
        }),
        { status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const gateway = new HttpLocalRunnerGateway("http://127.0.0.1:4317");
    const result = await gateway.triggerIntegrationConnection({
      projectId: "project-alpha",
      integrationId: "integration-1",
      providerType: "google_drive",
      action: "test",
    });

    const [input, init] = fetchMock.mock.calls[0] as [URL, RequestInit];
    expect(String(input)).toBe("http://127.0.0.1:4317/integrations/integration-1/connection");
    expect(init.method).toBe("POST");
    expect(init.body).toBe(
      JSON.stringify({
        projectId: "project-alpha",
        providerType: "google_drive",
        action: "test",
      }),
    );
    expect(result.requestStatus).toBe("accepted");
  });

  it("posts a retry request to the integration connection endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          requestStatus: "accepted",
          integrationId: "integration-1",
          integrationStatus: "pending",
          runId: null,
          message: null,
        }),
        { status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const gateway = new HttpLocalRunnerGateway("http://127.0.0.1:4317");
    const result = await gateway.triggerIntegrationConnection({
      projectId: "project-alpha",
      integrationId: "integration-1",
      providerType: "jira",
      action: "retry",
      workspaceUrl: "https://flowpilot899.atlassian.net",
      projectKey: "SCRUM",
    });

    const [, init] = fetchMock.mock.calls[0] as [URL, RequestInit];
    expect(init.body).toBe(
      JSON.stringify({
        projectId: "project-alpha",
        providerType: "jira",
        action: "retry",
        workspaceUrl: "https://flowpilot899.atlassian.net",
        projectKey: "SCRUM",
      }),
    );
    expect(result.integrationStatus).toBe("pending");
  });

  it("includes Jira email and api token when provided", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          requestStatus: "accepted",
          integrationId: "integration-1",
          integrationStatus: "awaiting_oauth",
          runId: "run-1",
          message: null,
        }),
        { status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const gateway = new HttpLocalRunnerGateway("http://127.0.0.1:4317");
    await gateway.triggerIntegrationConnection({
      projectId: "project-alpha",
      integrationId: "integration-1",
      providerType: "jira",
      action: "test",
      workspaceUrl: "https://flowpilot899.atlassian.net",
      projectKey: "SCRUM",
      boardId: "1",
      email: "name@company.com",
      apiToken: "secret-token",
    });

    const [, init] = fetchMock.mock.calls[0] as [URL, RequestInit];
    expect(init.body).toBe(
      JSON.stringify({
        projectId: "project-alpha",
        providerType: "jira",
        action: "test",
        workspaceUrl: "https://flowpilot899.atlassian.net",
        projectKey: "SCRUM",
        boardId: "1",
        email: "name@company.com",
        apiToken: "secret-token",
      }),
    );
  });

  it("throws a descriptive error when the runner rejects the request", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("bad request", { status: 400 }));
    vi.stubGlobal("fetch", fetchMock);

    const gateway = new HttpLocalRunnerGateway("http://127.0.0.1:4317");

    await expect(
      gateway.triggerIntegrationConnection({
        projectId: "project-alpha",
        integrationId: "integration-1",
        providerType: "firebase",
        action: "test",
      }),
    ).rejects.toThrow("Integration connection trigger failed: 400");
  });

  it("lists MCP backends from the runner", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify([
          {
            key: "google_drive",
            providerType: "google_drive",
            label: "Google Drive MCP",
            transport: "launcher",
            state: "installed",
            launcher: "npx",
            installed: true,
            binaryPath: "/usr/bin/npx",
            command: "npx -y @piotr-agier/google-drive-mcp --help",
            installHint: "Install with npx.",
            action: "verify",
            actionLabel: "Verify",
            lastCheckedAt: "2026-05-19T08:00:00.000Z",
            lastError: null,
          },
        ]),
        { status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const gateway = new HttpLocalRunnerGateway("http://127.0.0.1:4317");
    const backends = await gateway.listMcpBackends();

    expect(backends).toHaveLength(1);
    expect(backends[0]?.key).toBe("google_drive");
  });

  it("posts an MCP backend install request", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          key: "jira",
          providerType: "jira",
          label: "Atlassian MCP",
          transport: "launcher",
          state: "installed",
          launcher: "npx",
          installed: true,
          binaryPath: "/usr/bin/uvx",
          command: "npx -y mcp-remote https://mcp.atlassian.com/v1/mcp --help",
          installHint: "Install with npx.",
          action: "verify",
          actionLabel: "Verify",
          lastCheckedAt: "2026-05-19T08:00:00.000Z",
          lastError: null,
        }),
        { status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const gateway = new HttpLocalRunnerGateway("http://127.0.0.1:4317");
    const backend = await gateway.installMcpBackend("jira");

    const [input, init] = fetchMock.mock.calls[0] as [URL, RequestInit];
    expect(String(input)).toBe("http://127.0.0.1:4317/mcp-backends/jira/install");
    expect(init.method).toBe("POST");
    expect(backend.key).toBe("jira");
  });

  it("posts an MCP backend action request", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          key: "jira",
          providerType: "jira",
          label: "Atlassian MCP",
          transport: "launcher",
          state: "installed",
          launcher: "npx",
          installed: true,
          binaryPath: "/usr/bin/npx",
          command: "npx -y mcp-remote https://mcp.atlassian.com/v1/mcp --help",
          installHint: "Install with npx.",
          action: "verify",
          actionLabel: "Verify",
          lastCheckedAt: "2026-05-19T08:00:00.000Z",
          lastError: null,
        }),
        { status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const gateway = new HttpLocalRunnerGateway("http://127.0.0.1:4317");
    const backend = await gateway.triggerMcpBackendAction("jira", {
      projectId: "project-alpha",
      integrationId: "integration-1",
      action: "verify",
    });

    const [input, init] = fetchMock.mock.calls[0] as [URL, RequestInit];
    expect(String(input)).toBe("http://127.0.0.1:4317/mcp-backends/jira/action");
    expect(init.method).toBe("POST");
    expect(init.body).toBe(
      JSON.stringify({
        projectId: "project-alpha",
        integrationId: "integration-1",
        action: "verify",
      }),
    );
    expect(backend.action).toBe("verify");
  });

  it("lists MCP test runs for a backend", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify([
          {
            runId: "run-1",
            backendKey: "jira",
            providerType: "jira",
            projectId: "project-alpha",
            integrationId: "integration-1",
            status: "success",
            startedAt: "2026-05-19T08:00:00.000Z",
            completedAt: "2026-05-19T08:01:00.000Z",
            artifactDir: "C:/working/flowpilot/.flowpilot/mcp-tests/run-1",
          },
        ]),
        { status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const gateway = new HttpLocalRunnerGateway("http://127.0.0.1:4317");
    const runs = await gateway.listMcpTestRuns("jira", "project-alpha", "integration-1", 3);

    const [input, init] = fetchMock.mock.calls[0] as [URL, RequestInit];
    expect(String(input)).toBe(
      "http://127.0.0.1:4317/mcp-tests?backendKey=jira&projectId=project-alpha&integrationId=integration-1&limit=3",
    );
    expect(init.cache).toBe("no-store");
    expect(runs[0]?.runId).toBe("run-1");
  });

  it("posts an MCP test request", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          status: "success",
          runId: "run-1",
          backendKey: "jira",
          providerType: "jira",
          projectId: "project-alpha",
          integrationId: "integration-1",
          command: "npx jira-mcp test",
          stdoutSummary: "ok",
          stderrSummary: "",
          outputMarkdown: "Found 3 issues",
          artifactPaths: ["C:/working/flowpilot/.flowpilot/mcp-tests/run-1/output.md"],
          startedAt: "2026-05-19T08:00:00.000Z",
          completedAt: "2026-05-19T08:01:00.000Z",
          errorMessage: null,
        }),
        { status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const gateway = new HttpLocalRunnerGateway("http://127.0.0.1:4317");
    const result = await gateway.runMcpTest({
      backendKey: "jira",
      providerType: "jira",
      projectId: "project-alpha",
      integrationId: "integration-1",
      templateKey: "jira_find_open_bugs",
      allowWrite: false,
      prompt: "Find all open bugs in Project Alpha.",
      timeoutMs: 60000,
    });

    const [input, init] = fetchMock.mock.calls[0] as [URL, RequestInit];
    expect(String(input)).toBe("http://127.0.0.1:4317/mcp-tests");
    expect(init.method).toBe("POST");
    expect(init.body).toBe(
      JSON.stringify({
        backendKey: "jira",
        providerType: "jira",
        projectId: "project-alpha",
        integrationId: "integration-1",
        templateKey: "jira_find_open_bugs",
        allowWrite: false,
        prompt: "Find all open bugs in Project Alpha.",
        timeoutMs: 60000,
      }),
    );
    expect(result.status).toBe("success");
  });

  it("throws a descriptive error when an MCP test is rejected", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("bad request", { status: 422 }));
    vi.stubGlobal("fetch", fetchMock);

    const gateway = new HttpLocalRunnerGateway("http://127.0.0.1:4317");

    await expect(
      gateway.runMcpTest({
        backendKey: "jira",
        providerType: "jira",
        projectId: "project-alpha",
        integrationId: "integration-1",
        templateKey: "jira_find_open_bugs",
        allowWrite: false,
        prompt: "Test",
        timeoutMs: 60000,
      }),
    ).rejects.toThrow("MCP test failed: 422");
  });

  it("deletes an integration connection", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    const gateway = new HttpLocalRunnerGateway("http://127.0.0.1:4317");
    await gateway.deleteIntegrationConnection("integration-1");

    const [input, init] = fetchMock.mock.calls[0] as [URL, RequestInit];
    expect(String(input)).toBe("http://127.0.0.1:4317/integrations/integration-1/connection");
    expect(init.method).toBe("DELETE");
  });
});
