import { createSupabaseBrowserClient } from "@/data/datasource/supabase/client";
import { HttpLocalRunnerGateway } from "@/data/repository/local-runner/http-local-runner-gateway";
import { createDemoGatewayBundle } from "@/data/repository/demo/demo-gateway-bundle";
import { createSupabaseGatewayBundle } from "@/data/repository/supabase/supabase-gateway-bundle";
import { SupabaseWorkflowEngineGateway } from "@/data/repository/supabase/supabase-workflow-engine-gateway";
import { InMemoryWorkflowEngineGateway } from "@/data/repository/demo/in-memory-workflow-engine-gateway";
import { getLocalRunnerBaseUrl, hasSupabaseEnv } from "@/lib/env/browser-env";

export function createGatewayBundle() {
  const localRunnerGateway = new HttpLocalRunnerGateway(getLocalRunnerBaseUrl());

  if (hasSupabaseEnv()) {
    const supabaseClient = createSupabaseBrowserClient();
    return {
      ...createSupabaseGatewayBundle(supabaseClient),
      workflowEngineGateway: new SupabaseWorkflowEngineGateway(supabaseClient),
      localRunnerGateway,
    };
  }

  return {
    ...createDemoGatewayBundle(),
    workflowEngineGateway: new InMemoryWorkflowEngineGateway(),
    localRunnerGateway,
  };
}


