import { createDemoGatewayBundle } from "@/data/repository/demo/demo-gateway-bundle";
import { hasSupabaseEnv } from "@/lib/env/app-env";

export async function createGatewayBundle() {
  if (hasSupabaseEnv()) {
    return createDemoGatewayBundle();
  }

  return createDemoGatewayBundle();
}
