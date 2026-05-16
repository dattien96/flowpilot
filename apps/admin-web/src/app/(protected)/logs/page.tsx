import { createGatewayBundle } from "@/data/repository/factory";
import { ListAiCallLogsUseCase } from "@/domain/usecase/logs/list-ai-call-logs-usecase";
import { StatCard } from "@/presentation/components/dashboard/stat-card";

export default async function LogsPage({
  searchParams,
}: {
  searchParams: Promise<{ status?: string; provider?: string }>;
}) {
  const params = await searchParams;
  const status =
    params.status === "success" || params.status === "failed"
      ? params.status
      : undefined;
  const provider = params.provider || undefined;
  const gateways = await createGatewayBundle();
  const useCase = new ListAiCallLogsUseCase(gateways.workflowGateway);
  const [logs, summary] = await Promise.all([
    useCase.execute({ status, provider }),
    useCase.summarize({ status, provider }),
  ]);

  return (
    <div className="space-y-6">
      <header>
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          Logs
        </p>
        <h1 className="mt-3 text-4xl font-semibold tracking-tight">Mock AI call telemetry</h1>
      </header>
      <form className="grid gap-3 rounded-[1.6rem] border border-border bg-background/70 p-5 lg:grid-cols-3">
        <select
          className="rounded-2xl border border-border bg-card px-4 py-3"
          defaultValue={status ?? ""}
          name="status"
        >
          <option value="">All statuses</option>
          <option value="success">success</option>
          <option value="failed">failed</option>
        </select>
        <input
          className="rounded-2xl border border-border bg-card px-4 py-3"
          defaultValue={provider ?? ""}
          name="provider"
          placeholder="Provider filter"
        />
        <button className="rounded-full bg-accent px-4 py-3 text-sm font-semibold text-accent-foreground">
          Apply filters
        </button>
      </form>
      <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <StatCard label="Calls" value={summary.totalCalls} hint="Filtered AI calls." />
        <StatCard
          label="Tokens"
          value={summary.totalInputTokens + summary.totalOutputTokens}
          hint="Input and output tokens."
        />
        <StatCard
          label="Cost"
          value={`$${summary.totalCostEstimate.toFixed(4)}`}
          hint="Estimated mock spend."
        />
        <StatCard
          label="Latency"
          value={`${summary.averageLatencyMs}ms`}
          hint={`${summary.failedCalls} failed calls.`}
        />
      </section>
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
