import { useQuery } from "@tanstack/react-query";
import { createGatewayBundle } from "@/data/repository/browser-factory";

export function useSupportedModels() {
  return useQuery({
    queryKey: ["supportedModels"],
    queryFn: async () => {
      const gateways = await createGatewayBundle();
      return gateways.workflowEngineGateway.listSupportedModels();
    },
    staleTime: 60 * 1000,
  });
}
