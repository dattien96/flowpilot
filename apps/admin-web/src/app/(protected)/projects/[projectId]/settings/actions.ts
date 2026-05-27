"use server";

import { revalidatePath } from "next/cache";
import { createGatewayBundle } from "@/data/repository/factory";

import { z } from "zod";

const TtlSchema = z.coerce.number().int().min(1).max(10080); // max 1 week

export async function updateSessionTtlAction(projectId: string, formData: FormData) {
  const ttl = formData.get("sessionIdleTtlMinutes");
  if (!ttl) return;
  const parsed = TtlSchema.safeParse(ttl);
  if (!parsed.success) {
    throw new Error("Invalid session TTL. Must be a number between 1 and 10080.");
  }
  const gateways = await createGatewayBundle();
  await gateways.projectGateway.updateProject(projectId, {
    sessionIdleTtlMinutes: parsed.data
  });
  revalidatePath(`/projects/${projectId}/settings`);
}
