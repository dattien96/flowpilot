import { redirect } from "next/navigation";
import { NextResponse } from "next/server";

import { assertAdminApiSession } from "@/data/auth/session";
import { createGatewayBundle } from "@/data/repository/factory";

type RouteContext = {
  params: Promise<{
    runId: string;
  }>;
};

export async function POST(request: Request, context: RouteContext) {
  const auth = await assertAdminApiSession();
  if (!auth.ok) return auth.response;

  const { runId } = await context.params;
  const gateways = await createGatewayBundle();
  const run = await gateways.workflowGateway.getWorkflowRunById(runId);

  if (!run) {
    return NextResponse.json({ error: "Workflow run not found." }, { status: 404 });
  }

  if (run.status === "completed" || run.status === "rejected") {
    return NextResponse.json(
      { error: "Terminal workflow runs cannot be cancelled." },
      { status: 409 },
    );
  }

  await gateways.workflowGateway.updateWorkflowRun(runId, {
    status: "rejected",
    completedAt: new Date().toISOString(),
    errorSummary: "Cancelled by admin.",
  });

  if ((request.headers.get("content-type") ?? "").includes("application/json")) {
    return NextResponse.json({ ok: true });
  }

  redirect(`/workflow-runs/${runId}`);
}
