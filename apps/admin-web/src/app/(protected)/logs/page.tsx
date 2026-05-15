import { createGatewayBundle } from "@/data/repository/factory";
import { ListAiCallLogsUseCase } from "@/domain/usecase/logs/list-ai-call-logs-usecase";

export default async function LogsPage() {
  const gateways = await createGatewayBundle();
  const logs = await new ListAiCallLogsUseCase(gateways.workflowGateway).execute();

  return (
    <div className="space-y-6">
      <header>
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          Logs
        </p>
        <h1 className="mt-3 text-4xl font-semibold tracking-tight">Mock AI call telemetry</h1>
      </header>
      <div className="overflow-hidden rounded-[1.6rem] border border-border bg-background/70">
        <table className="w-full text-left text-sm">
          <thead className="bg-muted/60 text-muted-foreground">
            <tr>
              <th className="px-4 py-3">Provider</th>
              <th className="px-4 py-3">Model</th>
              <th className="px-4 py-3">Tokens</th>
              <th className="px-4 py-3">Cost</th>
              <th className="px-4 py-3">Latency</th>
            </tr>
          </thead>
          <tbody>
            {logs.map((log) => (
              <tr key={log.id} className="border-t border-border/80">
                <td className="px-4 py-3">{log.provider}</td>
                <td className="px-4 py-3">{log.model}</td>
                <td className="px-4 py-3">
                  {log.inputTokens}/{log.outputTokens}
                </td>
                <td className="px-4 py-3">${log.costEstimate.toFixed(4)}</td>
                <td className="px-4 py-3">{log.latencyMs}ms</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
