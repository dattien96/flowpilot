import { createDemoGatewayBundle } from "@/data/repository/demo/demo-gateway-bundle";
import { createSupabaseServerClient } from "@/data/datasource/supabase/client";
import { HttpLocalRunnerGateway } from "@/data/repository/local-runner/http-local-runner-gateway";
import { createSupabaseGatewayBundle } from "@/data/repository/supabase/supabase-gateway-bundle";
import { getLocalRunnerBaseUrl, hasSupabaseEnv } from "@/lib/env/app-env";

export async function createGatewayBundle() {
  const localRunnerGateway = new HttpLocalRunnerGateway(getLocalRunnerBaseUrl());

  if (hasSupabaseEnv()) {
    return {
      ...createSupabaseGatewayBundle(createSupabaseServerClient()),
      localRunnerGateway,
    };
  }

  return {
    ...createDemoGatewayBundle(),
    localRunnerGateway,
  };
}
