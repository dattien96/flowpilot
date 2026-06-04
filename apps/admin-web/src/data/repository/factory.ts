import { createDemoGatewayBundle } from "@/data/repository/demo/demo-gateway-bundle";
import { createSupabaseServerClient } from "@/data/datasource/supabase/client";
import { HttpLocalRunnerGateway } from "@/data/repository/local-runner/http-local-runner-gateway";
import { createSupabaseGatewayBundle } from "@/data/repository/supabase/supabase-gateway-bundle";
import { InMemoryAiOrchestrationGateway } from "@/data/repository/demo/in-memory-ai-orchestration-gateway";
import { SupabaseAiOrchestrationGateway } from "@/data/repository/supabase/supabase-ai-orchestration-gateway";
import { SupabaseWorkflowEngineGateway } from "@/data/repository/supabase/supabase-workflow-engine-gateway";
import { InMemoryWorkflowEngineGateway } from "@/data/repository/demo/in-memory-workflow-engine-gateway";
import { LocalFirstWorkflowGateway } from "@/data/repository/local-first/local-first-workflow-gateway";
import { getLocalRunnerBaseUrl } from "@/lib/env/app-env";
import { createSupabaseAdminClient } from "@/lib/supabase/admin";
import { SupabaseUserFavoriteGateway } from "@/data/repository/supabase/supabase-user-favorite-gateway";
import { InMemoryUserFavoriteGateway } from "@/data/repository/demo/in-memory-user-favorite-gateway";
import { SupabaseArtifactStorageConnectionGateway } from "@/data/repository/supabase/supabase-artifact-storage-connection-gateway";
import { startArtifactSyncBootstrap } from "@/features/artifacts/artifact-auto-sync";
import { hasSupabaseRuntimeConfigOrEnvFallback } from "@/lib/supabase/runtime-config.server";

export async function createGatewayBundle() {
  const localRunnerGateway = new HttpLocalRunnerGateway(
    getLocalRunnerBaseUrl(),
  );

  if (await hasSupabaseRuntimeConfigOrEnvFallback()) {
    const supabaseClient = await createSupabaseServerClient();
    const bundle = createSupabaseGatewayBundle(supabaseClient);
    try {
      const bootstrapClient = await createSupabaseAdminClient();
      const artifactStorageConnectionGateway =
        new SupabaseArtifactStorageConnectionGateway(bootstrapClient);
      void startArtifactSyncBootstrap({
        supabase: bootstrapClient,
        artifactStorageConnectionGateway,
      });
    } catch (error) {
      console.warn(
        "Unable to initialize artifact sync bootstrap admin client:",
        error instanceof Error ? error.message : error,
      );
    }
    return {
      ...bundle,
      workflowGateway: new LocalFirstWorkflowGateway(
        bundle.workflowGateway,
        localRunnerGateway,
      ),
      workflowEngineGateway: new SupabaseWorkflowEngineGateway(supabaseClient),
      aiOrchestrationGateway: new SupabaseAiOrchestrationGateway(
        supabaseClient,
      ),
      userFavoriteGateway: new SupabaseUserFavoriteGateway(supabaseClient),
      localRunnerGateway,
    };
  }

  return {
    ...createDemoGatewayBundle(),
    workflowEngineGateway: new InMemoryWorkflowEngineGateway(),
    aiOrchestrationGateway: new InMemoryAiOrchestrationGateway(),
    userFavoriteGateway: new InMemoryUserFavoriteGateway(),
    localRunnerGateway,
  };
}
