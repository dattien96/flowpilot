import { describe, it, expect, vi, beforeEach } from "vitest";
import { HttpLocalRunnerGateway } from "./http-local-runner-gateway";
import { LocalRunnerError } from "./http-local-runner-gateway";

describe("HttpLocalRunnerGateway", () => {
  let gateway: HttpLocalRunnerGateway;
  
  beforeEach(() => {
    gateway = new HttpLocalRunnerGateway("http://localhost:8080");
  });

  describe("sendMessage", () => {
    it("parses structured session_dead error into LocalRunnerError", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 400,
        statusText: "Bad Request",
        json: () => Promise.resolve({
          code: "session_dead",
          message: "session process exited or is no longer registered",
          details: "session_dead: session not found"
        })
      });
      global.fetch = mockFetch;

      const req = {
        session: { transportType: "codex_mcp", providerSessionId: "thread-old", processKey: "proc-old" },
        prompt: "hello",
        skillIds: [],
        contextSourceIds: []
      };

      try {
        await gateway.sendMessage(req);
        expect.fail("Should have thrown");
      } catch (err: any) {
        expect(err).toBeInstanceOf(LocalRunnerError);
        expect(err.code).toBe("session_dead");
        expect(err.message).toBe("session process exited or is no longer registered");
        expect(err.details).toBe("session_dead: session not found");
      }
    });

    it("parses structured mcp_write_approval_required error into LocalRunnerError", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 409,
        statusText: "Conflict",
        json: () => Promise.resolve({
          code: "mcp_write_approval_required",
          message: "manual approval is required before retrying this Google Drive write",
          details: "mcp_write_approval_required: MCP_WRITE_APPROVAL_REQUIRED: FlowPilot created approval request appr-123",
        }),
      });
      global.fetch = mockFetch;

      const req = {
        session: { transportType: "codex_mcp", providerSessionId: "thread-old", processKey: "proc-old" },
        prompt: "hello",
        skillIds: [],
        contextSourceIds: [],
      };

      await expect(gateway.sendMessage(req)).rejects.toMatchObject({
        code: "mcp_write_approval_required",
      });
    });
  });

  describe("google drive proxy approvals", () => {
    it("loads pending Google Drive proxy approvals from the runner", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve([{ id: "approval-1", toolName: "createGoogleDoc", status: "pending" }]),
      });
      global.fetch = mockFetch;

      const approvals = await gateway.listGoogleDriveProxyApprovals("run-1", "step-1", "pending");

      expect(approvals).toEqual([{ id: "approval-1", toolName: "createGoogleDoc", status: "pending" }]);
      expect(String(mockFetch.mock.calls[0][0])).toBe(
        "http://localhost:8080/google-drive-proxy-approvals?workflowRunId=run-1&workflowStepRunId=step-1&status=pending",
      );
    });

    it("submits a Google Drive proxy approval decision", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ id: "approval-1", status: "approved" }),
      });
      global.fetch = mockFetch;

      const result = await gateway.decideGoogleDriveProxyApproval("approval-1", {
        decision: "approved",
        comment: "Proceed",
      });

      expect(result).toEqual({ id: "approval-1", status: "approved" });
      expect(String(mockFetch.mock.calls[0][0])).toBe(
        "http://localhost:8080/google-drive-proxy-approvals/approval-1/decision",
      );
      expect(mockFetch.mock.calls[0][1]).toMatchObject({
        method: "POST",
      });
    });
  });

  describe("listSessions", () => {
    it("loads live sessions from the runner", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve([
            {
              transportType: "codex_mcp",
              providerSessionId: "thread-1",
              processKey: "proc-1",
              processPid: 4321,
            },
          ]),
      });
      global.fetch = mockFetch;

      const sessions = await gateway.listSessions();

      expect(sessions).toEqual([
        {
          transportType: "codex_mcp",
          providerSessionId: "thread-1",
          processKey: "proc-1",
          processPid: 4321,
        },
      ]);
      expect(mockFetch).toHaveBeenCalledTimes(1);
      expect(String(mockFetch.mock.calls[0][0])).toBe("http://localhost:8080/sessions");
    });
  });

  describe("deleteArtifactsByWorkflowRunIds", () => {
    it("posts the workflow run ids to the cleanup endpoint", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        status: 204,
        statusText: "No Content",
      });
      global.fetch = mockFetch;

      await gateway.deleteArtifactsByWorkflowRunIds(["run-1", " run-2 ", "run-1", ""]);

      expect(mockFetch).toHaveBeenCalledTimes(1);
      const [url, init] = mockFetch.mock.calls[0];
      expect(String(url)).toBe("http://localhost:8080/artifacts/delete-by-run-ids");
      expect(init).toMatchObject({
        method: "POST",
      });
      expect(JSON.parse(init.body as string)).toEqual({
        workflowRunIds: ["run-1", "run-2"],
      });
    });
  });
});
