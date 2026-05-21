import type { Integration } from "@/domain/model/entity/integration";
import type { LocalRunnerHealth } from "@/domain/model/entity/local-runner";

export function buildMcpServerStatus(integrations: Integration[]) {
  const total = integrations.length;
  const connected = integrations.filter((integration) => integration.status === "connected").length;

  return {
    connected,
    total,
    label: `${connected}/${total}`,
  };
}

export function buildRunnerStatus(health: LocalRunnerHealth | null) {
  const online = health?.status === "online";

  return {
    online,
    label: online ? "Online" : "Offline",
    toneClass: online ? "bg-success" : "bg-danger",
    textClass: online ? "text-success" : "text-danger",
  };
}
