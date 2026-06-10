import { beforeEach, describe, expect, it, vi } from "vitest";

import { handleWorkflowSubmitGoogleDriveWriteApproval } from "./submit-google-drive-write-approval-http-handler";

const mocks = vi.hoisted(() => ({
  assertAdminApiSessionForRequest: vi.fn(),
  createGatewayBundle: vi.fn(),
  createRuntimeSupabaseAdminClient: vi.fn(),
  submitGoogleDriveWriteApprovalRuntime: vi.fn(),
}));

vi.mock("@/data/auth/session", () => ({
  assertAdminApiSessionForRequest: mocks.assertAdminApiSessionForRequest,
}));

vi.mock("@/data/repository/factory", () => ({
  createGatewayBundle: mocks.createGatewayBundle,
}));

vi.mock("@/features/workflow-engine/workflow-start-runtime", () => ({
  submitGoogleDriveWriteApprovalRuntime: mocks.submitGoogleDriveWriteApprovalRuntime,
}));

vi.mock("@/lib/supabase/runtime-config.server", () => ({
  createRuntimeSupabaseAdminClient: mocks.createRuntimeSupabaseAdminClient,
}));

describe("handleWorkflowSubmitGoogleDriveWriteApproval", () => {
  beforeEach(() => {
    mocks.assertAdminApiSessionForRequest.mockReset();
    mocks.createGatewayBundle.mockReset();
    mocks.createRuntimeSupabaseAdminClient.mockReset();
    mocks.submitGoogleDriveWriteApprovalRuntime.mockReset();

    mocks.assertAdminApiSessionForRequest.mockResolvedValue({
      ok: true,
      session: {
        mode: "supabase",
        user: {
          id: "user-1",
          email: "user@example.com",
        },
      },
    });
    mocks.createGatewayBundle.mockResolvedValue({
      localRunnerGateway: { runner: true },
    });
    mocks.createRuntimeSupabaseAdminClient.mockResolvedValue({ admin: true });
    mocks.submitGoogleDriveWriteApprovalRuntime.mockResolvedValue({
      id: "step-1",
      workflow_run_id: "run-1",
      status: "DONE",
    });
  });

  it("accepts JSON approval submissions and returns JSON", async () => {
    const request = new Request("http://localhost/api/workflow-engine/google-drive-write-approval", {
      method: "POST",
      headers: {
        "content-type": "application/json",
        authorization: "Bearer token-1",
      },
      body: JSON.stringify({
        stepId: "step-1",
        decision: "approved",
        comment: "Proceed",
      }),
    });

    const response = await handleWorkflowSubmitGoogleDriveWriteApproval(request);

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({
      id: "step-1",
      workflow_run_id: "run-1",
      status: "DONE",
    });
    expect(mocks.submitGoogleDriveWriteApprovalRuntime).toHaveBeenCalledWith({
      adminClient: { admin: true },
      localRunnerGateway: { runner: true },
      stepId: "step-1",
      decision: "approved",
      comment: "Proceed",
    });
  });

  it("accepts form submissions and redirects back to the run page", async () => {
    const form = new URLSearchParams({
      stepId: "step-1",
      decision: "rejected",
      comment: "Not allowed",
    });
    const request = new Request("http://localhost/api/workflow-engine/google-drive-write-approval", {
      method: "POST",
      headers: {
        "content-type": "application/x-www-form-urlencoded",
        referer: "http://localhost/workflow-runs/run-1",
      },
      body: form,
    });

    const response = await handleWorkflowSubmitGoogleDriveWriteApproval(request);

    expect(response.status).toBe(303);
    expect(response.headers.get("location")).toBe("http://localhost/workflow-runs/run-1");
    expect(mocks.submitGoogleDriveWriteApprovalRuntime).toHaveBeenCalledWith({
      adminClient: { admin: true },
      localRunnerGateway: { runner: true },
      stepId: "step-1",
      decision: "rejected",
      comment: "Not allowed",
    });
  });

  it("returns the auth response when the request is unauthorized", async () => {
    mocks.assertAdminApiSessionForRequest.mockResolvedValueOnce({
      ok: false,
      response: Response.json({ error: "Authentication required." }, { status: 401 }),
    });

    const response = await handleWorkflowSubmitGoogleDriveWriteApproval(
      new Request("http://localhost/api/workflow-engine/google-drive-write-approval", {
        method: "POST",
        headers: {
          "content-type": "application/json",
        },
        body: JSON.stringify({
          stepId: "step-1",
          decision: "approved",
        }),
      }),
    );

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({
      error: "Authentication required.",
    });
    expect(mocks.submitGoogleDriveWriteApprovalRuntime).not.toHaveBeenCalled();
  });
});
