import { useMutation } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useState } from "react";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { Integration } from "@/domain/model/entity/integration";
import type {
  LocalRunnerHealth,
  LocalRunnerMcpBackend,
  LocalRunnerMcpTestResult,
  LocalRunnerMcpTestRunSummary,
} from "@/domain/model/entity/local-runner";
import type { Project } from "@/domain/model/entity/project";
import { CheckLocalRunnerHealthUseCase } from "@/domain/usecase/local-runner/check-local-runner-health-usecase";
import { ListLocalMcpBackendsUseCase } from "@/domain/usecase/local-runner/list-local-mcp-backends-usecase";
import { ListProjectsUseCase } from "@/domain/usecase/projects/list-projects-usecase";
import { formatTimestamp } from "@/features/mcp/integration-config";
import { Badge } from "@/presentation/components/ui/badge";

type JiraTestAction = {
  key: string;
  label: string;
  prompt: string;
};

const jiraTestActions: JiraTestAction[] = [
  {
    key: "jira_list_bugs",
    label: "Get a list of bugs",
    prompt: "Get a list of bug tickets in project SCRUM and summarize the top three by priority.",
  },
  {
    key: "jira_list_user_stories",
    label: "Get a list of user stories",
    prompt: "Get a list of user stories in project SCRUM and summarize their current status.",
  },
  {
    key: "jira_get_ticket_content",
    label: "Get the content of 1 ticket",
    prompt: "Get the content of Jira ticket SCRUM-1 and summarize the title, description, and status.",
  },
];

const mcpSections = [
  { key: "driver", label: "Driver", implemented: false },
  { key: "jira", label: "Jira", implemented: true },
  { key: "tele", label: "Tele", implemented: false },
  { key: "figma", label: "Figma", implemented: false },
  { key: "firebase", label: "Firebase", implemented: false },
] as const;

function resultTone(status: "success" | "failed") {
  return status === "success" ? "success" : "danger";
}

function DetailRow({ label, value }: Readonly<{ label: string; value: string }>) {
  return (
    <div className="rounded-2xl border border-border bg-card/80 p-4">
      <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <p className="mt-2 break-words text-sm font-medium">{value}</p>
    </div>
  );
}

export const Route = createFileRoute("/_authenticated/settings/mcp-servers/mcp-connect-test")({
  loader: async () => {
    const gateways = await createGatewayBundle();
    const [health, backends, projects, allIntegrations] = await Promise.all([
      new CheckLocalRunnerHealthUseCase(gateways.localRunnerGateway).execute(),
      new ListLocalMcpBackendsUseCase(gateways.localRunnerGateway).execute(),
      new ListProjectsUseCase(gateways.projectGateway).execute(),
      gateways.integrationGateway.listAllIntegrations(),
    ]);

    return { allIntegrations, backends, health, projects };
  },
  component: McpConnectTestPage,
});

export function McpConnectTestPage() {
  const { allIntegrations, backends, health, projects } = Route.useLoaderData();
  return (
    <McpConnectTestContent
      allIntegrations={allIntegrations}
      backends={backends}
      health={health}
      projects={projects}
    />
  );
}

export function McpConnectTestContent({
  allIntegrations,
  backends,
  health,
  projects,
}: {
  allIntegrations: Integration[];
  backends: LocalRunnerMcpBackend[];
  health: LocalRunnerHealth;
  projects: Project[];
}) {
  const runnerOnline = health.status === "online";
  const jiraBackend = backends.find((backend) => backend.providerType === "jira") ?? null;
  const connectedJiraIntegrations = allIntegrations.filter(
    (integration) => integration.type === "jira" && integration.status === "connected",
  );

  const [selectedProjectId, setSelectedProjectId] = useState(projects[0]?.id ?? "");
  const [selectedIntegrationId, setSelectedIntegrationId] = useState(
    connectedJiraIntegrations[0]?.id ?? "",
  );
  const [promptsByAction, setPromptsByAction] = useState<Record<string, string>>(
    Object.fromEntries(jiraTestActions.map((action) => [action.key, action.prompt])),
  );
  const [mcpTestError, setMcpTestError] = useState<string | null>(null);
  const [mcpTestResult, setMcpTestResult] = useState<LocalRunnerMcpTestResult | null>(null);
  const [mcpTestRuns, setMcpTestRuns] = useState<LocalRunnerMcpTestRunSummary[]>([]);
  const [mcpTestRunsLoading, setMcpTestRunsLoading] = useState(false);

  const jiraDisabledReason = !runnerOnline
    ? `The local runner is unreachable at ${health.baseUrl}.`
    : !jiraBackend
      ? "The Jira MCP backend is not available on this machine."
      : connectedJiraIntegrations.length === 0
        ? "No connected Jira MCP instance is available to test."
        : !selectedIntegrationId
          ? "Select a connected Jira MCP instance to run a test."
          : null;

  const runMcpTest = useMutation({
    mutationFn: async (action: JiraTestAction) => {
      if (jiraDisabledReason) {
        throw new Error(jiraDisabledReason);
      }
      const prompt = promptsByAction[action.key]?.trim() ?? "";
      if (!prompt) {
        throw new Error("Prompt is required.");
      }

      const gateways = await createGatewayBundle();
      return gateways.localRunnerGateway.runMcpTest({
        backendKey: jiraBackend?.key ?? "jira",
        providerType: "jira",
        projectId: selectedProjectId,
        integrationId: selectedIntegrationId,
        templateKey: action.key,
        allowWrite: false,
        prompt,
        timeoutMs: 60_000,
      });
    },
    onMutate: () => setMcpTestError(null),
    onSuccess: async (result) => {
      setMcpTestResult(result);
      setMcpTestRunsLoading(true);
      try {
        const gateways = await createGatewayBundle();
        const runs = await gateways.localRunnerGateway.listMcpTestRuns(
          jiraBackend?.key ?? "jira",
          selectedProjectId,
          selectedIntegrationId || undefined,
          5,
        );
        setMcpTestRuns(Array.isArray(runs) ? runs : []);
      } catch {
        setMcpTestRuns([]);
      } finally {
        setMcpTestRunsLoading(false);
      }
    },
    onError: (error) => {
      setMcpTestError(error instanceof Error ? error.message : "Unable to run MCP test.");
    },
  });

  useEffect(() => {
    if (
      selectedIntegrationId &&
      connectedJiraIntegrations.some((integration) => integration.id === selectedIntegrationId)
    ) {
      return;
    }
    setSelectedIntegrationId(connectedJiraIntegrations[0]?.id ?? "");
  }, [connectedJiraIntegrations, selectedIntegrationId]);

  useEffect(() => {
    if (!runnerOnline || !jiraBackend) {
      setMcpTestRuns([]);
      return;
    }

    let cancelled = false;

    async function loadMcpTestRuns() {
      setMcpTestRunsLoading(true);
      try {
        const gateways = await createGatewayBundle();
        const runs = await gateways.localRunnerGateway.listMcpTestRuns(
          jiraBackend.key,
          selectedProjectId,
          selectedIntegrationId || undefined,
          5,
        );
        if (!cancelled) {
          setMcpTestRuns(Array.isArray(runs) ? runs : []);
        }
      } catch {
        if (!cancelled) {
          setMcpTestRuns([]);
        }
      } finally {
        if (!cancelled) {
          setMcpTestRunsLoading(false);
        }
      }
    }

    void loadMcpTestRuns();

    return () => {
      cancelled = true;
    };
  }, [jiraBackend, runnerOnline, selectedIntegrationId, selectedProjectId]);

  return (
    <PageFrame
      description="Dedicated smoke-test route for validating MCP actions across supported providers."
      title="MCP Connect Test"
    >
      <div className="space-y-6">
        <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
                MCP Test Console
              </p>
              <h3 className="mt-3 text-2xl font-semibold tracking-tight">
                Provider-specific MCP smoke tests
              </h3>
              <p className="mt-2 max-w-3xl text-sm text-muted-foreground">
                This route groups test actions by MCP type. Jira is the first fully wired section,
                while the other sections reserve the same structure for future provider tests.
              </p>
            </div>
            <Badge tone={runnerOnline ? "success" : "danger"}>
              {runnerOnline ? "runner online" : "runner offline"}
            </Badge>
          </div>
        </section>

        {mcpSections.map((section) => (
          <details
            className="rounded-[1.6rem] border border-border bg-background/70 p-6"
            key={section.key}
            open={section.key === "jira"}
          >
            <summary className="cursor-pointer list-none">
              <div className="flex items-center justify-between gap-4">
                <div>
                  <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
                    MCP Type
                  </p>
                  <h3 className="mt-2 text-2xl font-semibold tracking-tight">{section.label}</h3>
                </div>
                <Badge tone={section.implemented ? "success" : "neutral"}>
                  {section.implemented ? "active shell" : "planned"}
                </Badge>
              </div>
            </summary>

            {section.key === "jira" ? (
              <div className="mt-6 space-y-4">
                <div className="grid gap-3 sm:grid-cols-2">
                  <label className="text-sm font-medium">
                    Test Project
                    <select
                      className="mt-2 w-full rounded-2xl border border-border bg-card px-4 py-3"
                      disabled={runMcpTest.isPending}
                      onChange={(event) => setSelectedProjectId(event.target.value)}
                      value={selectedProjectId}
                    >
                      {projects.map((project) => (
                        <option key={project.id} value={project.id}>
                          {project.name}
                        </option>
                      ))}
                    </select>
                  </label>

                  <label className="text-sm font-medium">
                    Jira MCP Instance
                    <select
                      className="mt-2 w-full rounded-2xl border border-border bg-card px-4 py-3"
                      disabled={connectedJiraIntegrations.length === 0 || runMcpTest.isPending}
                      onChange={(event) => setSelectedIntegrationId(event.target.value)}
                      value={selectedIntegrationId}
                    >
                      {connectedJiraIntegrations.length === 0 ? (
                        <option value="">No connected Jira MCP instances available</option>
                      ) : null}
                      {connectedJiraIntegrations.map((integration) => (
                        <option key={integration.id} value={integration.id}>
                          {integration.label}
                        </option>
                      ))}
                    </select>
                  </label>
                </div>

                {jiraDisabledReason ? (
                  <div className="rounded-2xl border border-dashed border-border bg-background/60 px-4 py-3 text-sm text-muted-foreground">
                    {jiraDisabledReason}
                  </div>
                ) : (
                  <div className="rounded-2xl border border-border bg-card/60 px-4 py-3 text-sm text-muted-foreground">
                    Expand a Jira action below to inspect the exact task payload before running it through the saved MCP connection.
                  </div>
                )}

                {jiraTestActions.map((action) => (
                  <details
                    className="rounded-2xl border border-border bg-card/60 p-4"
                    key={action.key}
                  >
                    <summary className="cursor-pointer list-none">
                      <div className="flex items-center justify-between gap-3">
                        <p className="text-sm font-medium">{action.label}</p>
                        <Badge tone="neutral">{action.key}</Badge>
                      </div>
                    </summary>

                    <div className="mt-4 space-y-3">
                      <label className="text-sm font-medium">
                        Calling API Test
                        <textarea
                          className="mt-2 min-h-28 w-full rounded-2xl border border-border bg-background px-4 py-3"
                          disabled={runMcpTest.isPending}
                          onChange={(event) =>
                            setPromptsByAction((current) => ({
                              ...current,
                              [action.key]: event.target.value,
                            }))
                          }
                          value={promptsByAction[action.key] ?? ""}
                        />
                      </label>

                      <div className="rounded-2xl border border-border bg-background/70 p-4">
                        <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
                          Request Preview
                        </p>
                        <pre className="mt-2 whitespace-pre-wrap text-sm">
{JSON.stringify(
  {
    mcpType: "jira",
    mcpInstanceId: selectedIntegrationId || "select-a-jira-instance",
    projectId: selectedProjectId || "select-a-project",
    templateKey: action.key,
    task: promptsByAction[action.key] ?? "",
  },
  null,
  2,
)}
                        </pre>
                      </div>

                      <div className="flex flex-wrap gap-2">
                        <Button
                          disabled={Boolean(jiraDisabledReason) || runMcpTest.isPending}
                          onClick={() => runMcpTest.mutate(action)}
                          type="button"
                        >
                          {runMcpTest.isPending ? "Running..." : "Run MCP Test"}
                        </Button>
                      </div>
                    </div>
                  </details>
                ))}

                {mcpTestError ? <p className="text-sm text-danger">{mcpTestError}</p> : null}

                <div className="grid gap-4 xl:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)]">
                  <div className="rounded-2xl border border-border bg-card/60 p-4">
                    <div className="flex items-center justify-between gap-3">
                      <p className="text-sm font-medium">Recent runs</p>
                      <Badge tone="neutral">jira</Badge>
                    </div>
                    {mcpTestRunsLoading ? (
                      <p className="mt-3 text-sm text-muted-foreground">Loading recent test runs...</p>
                    ) : mcpTestRuns.length === 0 ? (
                      <p className="mt-3 text-sm text-muted-foreground">
                        No test runs recorded yet for the selected Jira backend.
                      </p>
                    ) : (
                      <div className="mt-3 space-y-3">
                        {mcpTestRuns.map((run) => (
                          <div className="rounded-2xl border border-border bg-background/70 p-3" key={run.runId}>
                            <div className="flex items-center justify-between gap-3">
                              <p className="text-sm font-medium">{run.runId}</p>
                              <Badge tone={resultTone(run.status)}>{run.status}</Badge>
                            </div>
                            <p className="mt-2 text-sm text-muted-foreground">
                              Started {formatTimestamp(run.startedAt)}
                            </p>
                            <p className="mt-1 break-words text-xs text-muted-foreground">
                              {run.artifactDir}
                            </p>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>

                  <div className="rounded-2xl border border-border bg-card/60 p-4">
                    <div className="flex items-center justify-between gap-3">
                      <p className="text-sm font-medium">Latest result</p>
                      {mcpTestResult ? (
                        <Badge tone={resultTone(mcpTestResult.status)}>{mcpTestResult.status}</Badge>
                      ) : (
                        <Badge tone="neutral">Idle</Badge>
                      )}
                    </div>

                    {!mcpTestResult ? (
                      <p className="mt-3 text-sm text-muted-foreground">
                        Run one of the Jira actions to inspect the command, summaries, markdown output, and saved artifact paths.
                      </p>
                    ) : (
                      <div className="mt-3 space-y-3 text-sm">
                        <DetailRow label="Run ID" value={mcpTestResult.runId} />
                        <DetailRow label="Command" value={mcpTestResult.command} />
                        <DetailRow label="Started" value={formatTimestamp(mcpTestResult.startedAt)} />
                        <DetailRow label="Completed" value={formatTimestamp(mcpTestResult.completedAt)} />

                        {mcpTestResult.errorMessage ? (
                          <p className="rounded-2xl border border-danger/30 bg-danger/10 px-4 py-3 text-sm text-danger">
                            {mcpTestResult.errorMessage}
                          </p>
                        ) : null}

                        <div className="rounded-2xl border border-border bg-background/70 p-4">
                          <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
                            Stdout Summary
                          </p>
                          <pre className="mt-2 whitespace-pre-wrap text-sm">
                            {mcpTestResult.stdoutSummary || "No stdout summary returned."}
                          </pre>
                        </div>

                        <div className="rounded-2xl border border-border bg-background/70 p-4">
                          <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
                            Output Markdown
                          </p>
                          <pre className="mt-2 whitespace-pre-wrap text-sm">
                            {mcpTestResult.outputMarkdown || "No output markdown returned."}
                          </pre>
                        </div>
                      </div>
                    )}
                  </div>
                </div>
              </div>
            ) : (
              <div className="mt-6 rounded-2xl border border-dashed border-border bg-background/60 p-4 text-sm text-muted-foreground">
                The {section.label} action list shell is reserved here. Real test actions will be wired in a follow-up phase using the same collapsible structure as Jira.
              </div>
            )}
          </details>
        ))}
      </div>
    </PageFrame>
  );
}
