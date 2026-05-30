import { NextResponse } from "next/server";
import { z } from "zod";
import { createGatewayBundle } from "@/data/repository/factory";

const importSchema = z.object({
  providerKey: z.enum(["codex", "claude", "gemini"]),
});

export async function POST(request: Request) {
  try {
    const { providerKey } = importSchema.parse(await request.json());
    const gateways = await createGatewayBundle();
    const providers = await gateways.localRunnerGateway.listProviders();
    const targetProvider = providers.find((p) => p.key === providerKey);

    if (!targetProvider) {
      return NextResponse.json(
        { error: `Provider "${providerKey}" not found in runner.` },
        { status: 404 }
      );
    }

    const detectedModels = (targetProvider.models ?? []).filter((m) => m.available);
    const detectionMethod = providerKey === "codex"
      ? "codex_debug_models"
      : providerKey === "gemini"
        ? "gemini_bundle_registry"
        : "claude_binary_registry";

    return NextResponse.json({
      providerKey,
      detectedAt: new Date().toISOString(),
      detectedCliVersion: targetProvider.version || "unknown",
      detectionMethod,
      models: detectedModels.map((model) => ({
        modelId: model.id,
        displayName: model.displayName || model.id,
        source: model.source,
      })),
      warnings: [],
    });
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Provider models import failed.",
      },
      { status: 400 }
    );
  }
}
