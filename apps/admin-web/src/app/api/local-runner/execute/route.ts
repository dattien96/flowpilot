import { NextResponse } from "next/server";
import { z } from "zod";

import { createGatewayBundle } from "@/data/repository/factory";
import { RunPromptUseCase } from "@/domain/usecase/local-runner/run-prompt-usecase";

const executeSchema = z.object({
  providerKey: z.string().min(1),
  prompt: z.string().min(1),
  skillIds: z.array(z.string()).default([]),
  flowId: z.string().nullable().optional(),
  contextSourceIds: z.array(z.string()).default([]),
  timeoutMs: z.number().int().positive().max(1_800_000).optional(),
  workingDirectory: z.string().nullable().optional(),
});

export async function POST(request: Request) {
  try {
    const payload = executeSchema.parse(await request.json());
    const gateways = await createGatewayBundle();
    const result = await new RunPromptUseCase(gateways.localRunnerGateway).execute({
      providerKey: payload.providerKey,
      prompt: payload.prompt,
      skillIds: payload.skillIds,
      flowId: payload.flowId ?? null,
      contextSourceIds: payload.contextSourceIds,
      timeoutMs: payload.timeoutMs ?? 600000,
      workingDirectory: payload.workingDirectory ?? null,
    });

    return NextResponse.json(result);
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Prompt execution failed.",
      },
      { status: 400 },
    );
  }
}
