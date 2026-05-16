import { redirect } from "next/navigation";
import { NextResponse } from "next/server";
import { z } from "zod";

import { assertAdminApiSession } from "@/data/auth/session";
import { createGatewayBundle } from "@/data/repository/factory";
import { DeleteContextSourceUseCase } from "@/domain/usecase/context-sources/delete-context-source-usecase";
import { UpdateContextSourceUseCase } from "@/domain/usecase/context-sources/update-context-source-usecase";

const updateContextSourceSchema = z.object({
  title: z.string().min(3),
  type: z.enum(["manual_text", "url", "api_note", "file"]),
  rawContent: z.string().min(1),
  summarizedContent: z.string().nullable().optional(),
});

type RouteContext = {
  params: Promise<{
    contextSourceId: string;
  }>;
};

export async function PATCH(request: Request, context: RouteContext) {
  const auth = await assertAdminApiSession();
  if (!auth.ok) return auth.response;

  const { contextSourceId } = await context.params;
  const payload = updateContextSourceSchema.parse(await request.json());
  const gateways = await createGatewayBundle();
  const result = await new UpdateContextSourceUseCase(
    gateways.contextSourceGateway,
  ).execute({
    contextSourceId,
    ...payload,
  });

  return NextResponse.json(result);
}

export async function DELETE(_: Request, context: RouteContext) {
  const auth = await assertAdminApiSession();
  if (!auth.ok) return auth.response;

  const { contextSourceId } = await context.params;
  const gateways = await createGatewayBundle();
  await new DeleteContextSourceUseCase(gateways.contextSourceGateway).execute(
    contextSourceId,
  );

  return NextResponse.json({ ok: true });
}

export async function POST(request: Request, context: RouteContext) {
  const auth = await assertAdminApiSession();
  if (!auth.ok) return auth.response;

  const { contextSourceId } = await context.params;
  const formData = await request.formData();
  const method = String(formData.get("_method") ?? "").toUpperCase();
  const gateways = await createGatewayBundle();

  if (method === "DELETE") {
    await new DeleteContextSourceUseCase(gateways.contextSourceGateway).execute(
      contextSourceId,
    );
    redirect("/context-sources");
  }

  const payload = updateContextSourceSchema.parse({
    title: formData.get("title"),
    type: formData.get("type"),
    rawContent: formData.get("rawContent"),
    summarizedContent: formData.get("summarizedContent"),
  });
  await new UpdateContextSourceUseCase(gateways.contextSourceGateway).execute({
    contextSourceId,
    ...payload,
  });

  redirect("/context-sources");
}
