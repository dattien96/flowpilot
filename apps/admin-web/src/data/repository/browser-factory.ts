import { createSupabaseBrowserClient } from "@/data/datasource/supabase/client";
import { HttpLocalRunnerGateway } from "@/data/repository/local-runner/http-local-runner-gateway";
import { createDemoGatewayBundle } from "@/data/repository/demo/demo-gateway-bundle";
import { createSupabaseGatewayBundle } from "@/data/repository/supabase/supabase-gateway-bundle";
import { getLocalRunnerBaseUrl, hasSupabaseEnv } from "@/lib/env/browser-env";

export function createGatewayBundle() {
  const localRunnerGateway = new HttpLocalRunnerGateway(getLocalRunnerBaseUrl());

  if (hasSupabaseEnv()) {
    return {
      ...createSupabaseGatewayBundle(createSupabaseBrowserClient()),
      localRunnerGateway,
    };
  }

  return {
    ...createDemoGatewayBundle(),
    localRunnerGateway,
  };
}

