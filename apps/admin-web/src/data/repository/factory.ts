import { createDemoGatewayBundle } from "@/data/repository/demo/demo-gateway-bundle";
import { HttpLocalRunnerGateway } from "@/data/repository/local-runner/http-local-runner-gateway";
import { getLocalRunnerBaseUrl, hasSupabaseEnv } from "@/lib/env/app-env";

export async function createGatewayBundle() {
  const localRunnerGateway = new HttpLocalRunnerGateway(getLocalRunnerBaseUrl());

  if (hasSupabaseEnv()) {
    return {
      ...createDemoGatewayBundle(),
      localRunnerGateway,
    };
  }

  return {
    ...createDemoGatewayBundle(),
    localRunnerGateway,
  };
}
