import { NextResponse } from "next/server";

import { assertAdminApiSession } from "@/data/auth/session";
import { createGatewayBundle } from "@/data/repository/factory";
import { ListAiCallLogsUseCase } from "@/domain/usecase/logs/list-ai-call-logs-usecase";

export async function GET(request: Request) {
  const auth = await assertAdminApiSession();
  if (!auth.ok) return auth.response;

  const url = new URL(request.url);
  const status = url.searchParams.get("status");
  const provider = url.searchParams.get("provider") ?? undefined;
  const filters: { status?: "success" | "failed"; provider?: string } = {
    provider,
    status:
      status === "success" || status === "failed"
        ? status
        : undefined,
  };
  const gateways = await createGatewayBundle();
  const useCase = new ListAiCallLogsUseCase(gateways.workflowGateway);
  const [logs, summary] = await Promise.all([
    useCase.execute(filters),
    useCase.summarize(filters),
  ]);

  return NextResponse.json({ logs, summary });
}
