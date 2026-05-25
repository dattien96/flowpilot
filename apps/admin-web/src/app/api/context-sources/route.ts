import { redirect } from "next/navigation";
import { NextResponse } from "next/server";
import { z } from "zod";

import { assertAdminApiSession } from "@/data/auth/session";
import { createGatewayBundle } from "@/data/repository/factory";
import { CreateContextSourceUseCase } from "@/domain/usecase/context-sources/create-context-source-usecase";

const createContextSourceSchema = z.object({
  projectId: z.string().min(1),
  type: z.enum(["manual_text", "url", "api_note", "file"]),
  title: z.string().min(3),
  rawContent: z.string().min(1),
});

export async function GET() {
  const auth = await assertAdminApiSession();
  if (!auth.ok) return auth.response;

  const gateways = await createGatewayBundle();
  const contexts = await gateways.contextSourceGateway.listContextSources();
  return NextResponse.json({ contexts });
}

export async function POST(request: Request) {
  const auth = await assertAdminApiSession();
  if (!auth.ok) return auth.response;

  const contentType = request.headers.get("content-type") ?? "";
  const input = contentType.includes("application/json")
    ? await request.json()
    : Object.fromEntries((await request.formData()).entries());
  const payload = createContextSourceSchema.parse(input);
  const gateways = await createGatewayBundle();
  const contextSource = await new CreateContextSourceUseCase(
    gateways.contextSourceGateway,
    gateways.projectGateway,
  ).execute(payload);

  if (contentType.includes("application/json")) {
    return NextResponse.json(contextSource, { status: 201 });
  }

  redirect("/context-sources");
}
