"use server";

import { revalidatePath } from "next/cache";
import { createGatewayBundle } from "@/data/repository/factory";
import { createSupabaseServerClient } from "@/data/datasource/supabase/client";
import {
  getArtifactStorageConnection,
  isArtifactStorageConnectionReady,
} from "@/features/artifacts/artifact-storage-connection";

import { z } from "zod";

const TtlSchema = z.coerce.number().int().min(1).max(10080); // max 1 week
const ArtifactStoragePreferenceSchema = z.enum(["supabase", "google_drive"]);

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

export async function updateArtifactStoragePreferenceAction(projectId: string, formData: FormData) {
  const value = formData.get("artifactStoragePreference");
  if (!value) return;
  const parsed = ArtifactStoragePreferenceSchema.safeParse(value);
  if (!parsed.success) {
    throw new Error("Invalid artifact storage preference.");
  }
  const gateways = await createGatewayBundle();
  if (parsed.data === "google_drive") {
    const connection = await getArtifactStorageConnection(
      await createSupabaseServerClient(),
      projectId,
      "google_drive",
    );
    if (!isArtifactStorageConnectionReady(connection)) {
      throw new Error(
        "Google Drive can only be selected after this project has a connected Google Drive artifact storage folder on this runner.",
      );
    }
  }
  await gateways.projectGateway.updateProject(projectId, {
    artifactStoragePreference: parsed.data,
  });
  revalidatePath(`/projects/${projectId}/settings`);
}
