import { redirect } from "next/navigation";
import { NextResponse } from "next/server";
import { assertAdminApiSession } from "@/data/auth/session";
import { createSupabaseAdminClient } from "@/lib/supabase/admin";
import { createGatewayBundle } from "@/data/repository/factory";
import { finalizeWorkflowRunSessions } from "@/features/workflow-engine/workflow-start-runtime";

export async function POST(
  request: Request,
  { params }: { params: Promise<{ runId: string }> },
) {
  const auth = await assertAdminApiSession();
  if (!auth.ok) return auth.response;

  const { runId } = await params;
  const gateways = await createGatewayBundle();
  const run = await gateways.workflowGateway.getWorkflowRunById(runId);

  if (!run) {
    return NextResponse.json({ error: "Workflow run not found." }, { status: 404 });
  }

  if (run.status === "completed" || run.status === "rejected" || run.status === "failed") {
    return NextResponse.json(
      { error: "Session cannot be terminated on a finished run." },
      { status: 409 },
    );
  }

  const adminClient = await createSupabaseAdminClient();
  await finalizeWorkflowRunSessions(adminClient, gateways.localRunnerGateway, runId);

  if ((request.headers.get("content-type") ?? "").includes("application/json")) {
    return NextResponse.json({ ok: true });
  }

  redirect(`/workflow-runs/${runId}`);
}
