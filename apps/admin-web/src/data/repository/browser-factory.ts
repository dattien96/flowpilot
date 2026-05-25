import { createSupabaseBrowserClient } from "@/data/datasource/supabase/client";
import { HttpLocalRunnerGateway } from "@/data/repository/local-runner/http-local-runner-gateway";
import { createDemoGatewayBundle } from "@/data/repository/demo/demo-gateway-bundle";
import { createSupabaseGatewayBundle } from "@/data/repository/supabase/supabase-gateway-bundle";
import { InMemoryAiOrchestrationGateway } from "@/data/repository/demo/in-memory-ai-orchestration-gateway";
import { SupabaseAiOrchestrationGateway } from "@/data/repository/supabase/supabase-ai-orchestration-gateway";
import { SupabaseWorkflowEngineGateway } from "@/data/repository/supabase/supabase-workflow-engine-gateway";
import { InMemoryWorkflowEngineGateway } from "@/data/repository/demo/in-memory-workflow-engine-gateway";
import { LocalFirstWorkflowGateway } from "@/data/repository/local-first/local-first-workflow-gateway";
import { getLocalRunnerBaseUrl, hasSupabaseEnv } from "@/lib/env/browser-env";

export function createGatewayBundle() {
  const localRunnerGateway = new HttpLocalRunnerGateway(
    getLocalRunnerBaseUrl(),
  );

  if (hasSupabaseEnv()) {
    const supabaseClient = createSupabaseBrowserClient();
    const bundle = createSupabaseGatewayBundle(supabaseClient);
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
      localRunnerGateway,
    };
  }

  return {
    ...createDemoGatewayBundle(),
    workflowEngineGateway: new InMemoryWorkflowEngineGateway(),
    aiOrchestrationGateway: new InMemoryAiOrchestrationGateway(),
    localRunnerGateway,
  };
}
