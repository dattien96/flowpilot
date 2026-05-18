import { useEffect, useState, type FormEvent } from "react";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { Activity, ArrowRight, CheckCircle2, Clock3, FolderOpen, Plus } from "lucide-react";

import { AppShell } from "@/components/layout/app-shell";
import { Button } from "@/presentation/components/ui/button";
import { Badge } from "@/presentation/components/ui/badge";
import { StatCard } from "@/presentation/components/dashboard/stat-card";
import { statusTone } from "@/presentation/view-models/factories";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { GetDashboardSummaryUseCase } from "@/domain/usecase/dashboard/get-dashboard-summary-usecase";
import { ListProjectsUseCase } from "@/domain/usecase/projects/list-projects-usecase";
import { ListWorkflowRunsUseCase } from "@/domain/usecase/workflow-runs/list-workflow-runs-usecase";
import { ListFeaturesUseCase } from "@/domain/usecase/features/list-features-usecase";
import { ListWorkflowDefinitionsUseCase } from "@/domain/usecase/workflow-definitions/list-workflow-definitions-usecase";
import { ListPendingApprovalsUseCase } from "@/domain/usecase/approvals/list-pending-approvals-usecase";
import { ListOutputsUseCase } from "@/domain/usecase/outputs/list-outputs-usecase";
import { ListAiCallLogsUseCase } from "@/domain/usecase/logs/list-ai-call-logs-usecase";
import { GetStorageDriverUseCase } from "@/domain/usecase/artifacts/get-storage-driver-usecase";
import { CheckLocalRunnerHealthUseCase } from "@/domain/usecase/local-runner/check-local-runner-health-usecase";
import { ListLocalProvidersUseCase } from "@/domain/usecase/local-runner/list-local-providers-usecase";
import { ListLocalSkillsUseCase } from "@/domain/usecase/local-runner/list-local-skills-usecase";
import { ListLocalFlowsUseCase } from "@/domain/usecase/local-runner/list-local-flows-usecase";
import { ListArtifactsUseCase } from "@/domain/usecase/artifacts/list-artifacts-usecase";
import type { CreateProjectPayload } from "@/domain/model/payload/project-payload";
import { createSupabaseBrowserClient } from "@/data/supabase/client";
import { useAuth } from "@/features/auth/auth-provider";
import { hasSupabaseEnv } from "@/lib/env/browser-env";

function LoadingState() {
  return <p className="text-sm text-muted-foreground">Loading...</p>;
}

function ErrorState({ message }: { message: string }) {
  return <p className="text-sm text-danger">{message}</p>;
}

function routeTo(path: string) {
  return path as never;
}

function routeParams<T extends Record<string, string>>(params: T) {
  return params as never;
}

export function LoginPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { session } = useAuth();
  const [email, setEmail] = useState("admin@example.com");
  const [password, setPassword] = useState("password");
  const [error, setError] = useState<string | null>(null);
  const supabaseEnabled = Boolean(session?.mode === "supabase" || hasSupabaseEnv());

  useEffect(() => {
    if (session) {
      void navigate({ to: routeTo("/dashboard"), replace: true });
    }
  }, [navigate, session]);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError(null);

    if (!supabaseEnabled) {
      void navigate({ to: routeTo("/dashboard"), replace: true });
      return;
    }

    const supabase = createSupabaseBrowserClient();
    const result = await supabase.auth.signInWithPassword({ email, password });

    if (result.error) {
      setError(result.error.message);
      return;
    }

    await queryClient.invalidateQueries();
    void navigate({ to: routeTo("/dashboard"), replace: true });
  };

  return (
    <div className="noise-bg flex min-h-screen items-center justify-center px-4">
      <div className="panel-shadow w-full max-w-xl rounded-[2rem] border border-border bg-card p-8">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-muted-foreground">
          FlowPilot Access
        </p>
        <h1 className="mt-4 text-4xl font-semibold tracking-tight">
          Enter the workflow control center.
        </h1>
        <p className="mt-4 max-w-lg text-base text-muted-foreground">
          {supabaseEnabled
            ? "Sign in with a Supabase Auth user to access protected admin workflows."
            : "Supabase env is not configured, so the admin runs in demo mode for local exploration."}
        </p>
        {supabaseEnabled ? (
          <form onSubmit={handleSubmit} className="mt-8 grid gap-3">
            <input
              className="rounded-2xl border border-border bg-background px-4 py-3"
              name="email"
              onChange={(event) => setEmail(event.target.value)}
              placeholder="admin@example.com"
              required
              type="email"
              value={email}
            />
            <input
              className="rounded-2xl border border-border bg-background px-4 py-3"
              name="password"
              onChange={(event) => setPassword(event.target.value)}
              placeholder="Password"
              required
              type="password"
              value={password}
            />
            <Button type="submit">Sign in</Button>
            {error ? <ErrorState message={error} /> : null}
          </form>
        ) : (
          <div className="mt-8 flex gap-3">
            <Button onClick={() => void navigate({ to: routeTo("/dashboard"), replace: true })}>
              <span className="inline-flex items-center gap-2">
                Continue to demo
                <ArrowRight className="size-4" />
              </span>
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}

export function DashboardPage() {
  const summaryQuery = useQuery({
    queryKey: ["dashboard-summary"],
    queryFn: async () => {
      const gateways = createGatewayBundle();
      return new GetDashboardSummaryUseCase(
        gateways.projectGateway,
        gateways.featureGateway,
        gateways.contextSourceGateway,
        gateways.workflowGateway,
      ).execute();
    },
  });

  return (
    <AppShell>
      <div className="space-y-8">
        <header className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
              Operational Status
            </p>
            <h1 className="mt-3 text-4xl font-semibold tracking-tight">
              Workflow control at a glance.
            </h1>
          </div>
          <p className="max-w-xl text-sm text-muted-foreground">
            The Vite shell still speaks to the same domain layer and demo data model while the route system changes underneath it.
          </p>
        </header>

        {summaryQuery.isLoading ? <LoadingState /> : null}
        {summaryQuery.isError ? <ErrorState message="Unable to load dashboard summary." /> : null}

        {summaryQuery.data ? (
          <>
            <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
              <StatCard
                label="Running"
                value={summaryQuery.data.activeWorkflowCount}
                hint="Workflow runs progressing without a human gate."
                accent={<Activity className="size-6 text-accent" />}
              />
              <StatCard
                label="Approvals"
                value={summaryQuery.data.pendingApprovalCount}
                hint="Outputs paused for an explicit human decision."
                accent={<Clock3 className="size-6 text-warning" />}
              />
              <StatCard
                label="Outputs"
                value={summaryQuery.data.completedOutputCount}
                hint="Stored artifacts available for review and export."
                accent={<CheckCircle2 className="size-6 text-success" />}
              />
              <StatCard
                label="Projects"
                value={summaryQuery.data.projectCount}
                hint="Registered product scopes available for intake."
                accent={<FolderOpen className="size-6 text-accent" />}
              />
            </section>

            <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
              <div className="flex items-end justify-between">
                <div>
                  <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
                    Recent Runs
                  </p>
                  <h2 className="mt-3 text-2xl font-semibold tracking-tight">
                    Latest workflow activity
                  </h2>
                </div>
                <Link className="text-sm font-medium text-accent" to={routeTo("/workflow-runs")}>
                  View all runs
                </Link>
              </div>
              <div className="mt-6 space-y-3">
                {summaryQuery.data.recentRuns.length === 0 ? (
                  <p className="text-sm text-muted-foreground">No workflow runs yet.</p>
                ) : (
                  summaryQuery.data.recentRuns.map((run) => (
                    <Link
                      key={run.id}
                      className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3 transition-colors hover:bg-muted/50"
                      to={routeTo("/workflow-runs")}
                    >
                      <div>
                        <p className="font-semibold">{run.id}</p>
                        <p className="text-sm text-muted-foreground">
                          Started {new Date(run.startedAt).toLocaleString()}
                        </p>
                      </div>
                      <Badge tone={statusTone(run.status)}>{run.status}</Badge>
                    </Link>
                  ))
                )}
              </div>
            </section>
          </>
        ) : null}
      </div>
    </AppShell>
  );
}

export function ProjectsPage() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const projectsQuery = useQuery({
    queryKey: ["projects"],
    queryFn: async () => {
      const gateways = createGatewayBundle();
      return new ListProjectsUseCase(gateways.projectGateway).execute();
    },
  });

  const mutation = useMutation({
    mutationFn: async (payload: CreateProjectPayload) => {
      const gateways = createGatewayBundle();
      return gateways.projectGateway.createProject(payload);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
  });

  const [form, setForm] = useState<CreateProjectPayload>({
    name: "",
    description: "",
    platform: "android",
    repositoryUrl: "",
  });

  return (
    <AppShell>
      <div className="space-y-6">
        <header>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Projects
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">Project registry</h1>
        </header>

        <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <h2 className="text-xl font-semibold">Create project</h2>
          <form
            className="mt-4 grid gap-3 lg:grid-cols-2"
            onSubmit={(event) => {
              event.preventDefault();
              mutation.mutate(form, {
                onSuccess: async () => {
                  setForm({ name: "", description: "", platform: "android", repositoryUrl: "" });
                  await queryClient.invalidateQueries({ queryKey: ["projects"] });
                  await navigate({ to: routeTo("/projects") });
                },
              });
            }}
          >
            <input
              className="rounded-2xl border border-border bg-card px-4 py-3"
              name="name"
              onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
              placeholder="Project name"
              required
              value={form.name}
            />
            <input
              className="rounded-2xl border border-border bg-card px-4 py-3"
              name="repositoryUrl"
              onChange={(event) =>
                setForm((current) => ({ ...current, repositoryUrl: event.target.value }))
              }
              placeholder="https://github.com/org/repo"
              required
              type="url"
              value={form.repositoryUrl}
            />
            <textarea
              className="min-h-28 rounded-2xl border border-border bg-card px-4 py-3 lg:col-span-2"
              name="description"
              onChange={(event) =>
                setForm((current) => ({ ...current, description: event.target.value }))
              }
              placeholder="Short project description"
              required
              value={form.description}
            />
            <select
              className="rounded-2xl border border-border bg-card px-4 py-3"
              name="platform"
              onChange={(event) =>
                setForm((current) => ({ ...current, platform: event.target.value as CreateProjectPayload["platform"] }))
              }
              value={form.platform}
            >
              <option value="android">android</option>
              <option value="ios">ios</option>
              <option value="web">web</option>
              <option value="multi">multi</option>
            </select>
            <div className="flex items-center justify-end">
              <Button type="submit">
                <span className="inline-flex items-center gap-2">
                  <Plus className="size-4" />
                  Create project
                </span>
              </Button>
            </div>
          </form>
        </section>

        {projectsQuery.isLoading ? <LoadingState /> : null}
        {projectsQuery.isError ? <ErrorState message="Unable to load projects." /> : null}

        <div className="grid gap-4 xl:grid-cols-2">
          {projectsQuery.data?.map((project) => (
            <Link
              key={project.id}
              className="rounded-[1.6rem] border border-border bg-background/70 p-6 transition-transform hover:-translate-y-0.5"
              params={routeParams({ projectId: project.id })}
              to={routeTo("/projects/$projectId")}
            >
              <div className="flex items-start justify-between gap-4">
                <div>
                  <h2 className="text-2xl font-semibold">{project.name}</h2>
                  <p className="mt-2 text-sm text-muted-foreground">{project.description}</p>
                </div>
                <Badge>{project.platform}</Badge>
              </div>
              <p className="mt-6 font-mono text-xs uppercase tracking-[0.24em] text-muted-foreground">
                {project.repositoryUrl}
              </p>
            </Link>
          ))}
        </div>
      </div>
    </AppShell>
  );
}

export function ProjectDetailPage() {
  const params = useParams({ strict: false }) as { projectId?: string };
  const projectId = params.projectId ?? "";
  const detailQuery = useQuery({
    queryKey: ["project-detail", projectId],
    queryFn: async () => {
      const gateways = createGatewayBundle();
      const project = await gateways.projectGateway.getProjectById(projectId);
      const features = await gateways.featureGateway.listFeaturesByProject(projectId);
      const contexts = await gateways.contextSourceGateway.listContextSourcesByProject(projectId);
      return { project, features, contexts };
    },
    enabled: Boolean(projectId),
  });

  return (
    <AppShell>
      <div className="space-y-6">
        <header>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Project Detail
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">Project overview</h1>
        </header>

        {detailQuery.isLoading ? <LoadingState /> : null}
        {detailQuery.isError ? <ErrorState message="Unable to load project detail." /> : null}

        {detailQuery.data?.project ? (
          <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
            <h2 className="text-2xl font-semibold">{detailQuery.data.project.name}</h2>
            <p className="mt-2 text-sm text-muted-foreground">{detailQuery.data.project.description}</p>
            <p className="mt-4 text-xs uppercase tracking-[0.24em] text-muted-foreground">
              {detailQuery.data.project.repositoryUrl}
            </p>
          </section>
        ) : null}

        <section className="grid gap-4 xl:grid-cols-2">
          <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
            <h3 className="text-xl font-semibold">Features</h3>
            <div className="mt-4 space-y-3">
              {detailQuery.data?.features.map((feature) => (
                <div key={feature.id} className="rounded-2xl border border-border bg-card px-4 py-3">
                  <p className="font-semibold">{feature.title}</p>
                  <p className="text-sm text-muted-foreground">{feature.status}</p>
                </div>
              ))}
            </div>
          </div>
          <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
            <h3 className="text-xl font-semibold">Context Sources</h3>
            <div className="mt-4 space-y-3">
              {detailQuery.data?.contexts.map((context) => (
                <div key={context.id} className="rounded-2xl border border-border bg-card px-4 py-3">
                  <p className="font-semibold">{context.title}</p>
                  <p className="text-sm text-muted-foreground">{context.type}</p>
                </div>
              ))}
            </div>
          </div>
        </section>
      </div>
    </AppShell>
  );
}

export function FeaturesPage() {
  const featuresQuery = useQuery({
    queryKey: ["features"],
    queryFn: async () => {
      const gateways = createGatewayBundle();
      return new ListFeaturesUseCase(gateways.featureGateway).execute();
    },
  });

  return (
    <AppShell>
      <div className="space-y-6">
        <header>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Features
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">Feature registry</h1>
        </header>
        {featuresQuery.isLoading ? <LoadingState /> : null}
        {featuresQuery.data ? (
          <div className="grid gap-4 xl:grid-cols-2">
            {featuresQuery.data.map((feature) => (
              <div key={feature.id} className="rounded-[1.6rem] border border-border bg-background/70 p-6">
                <p className="font-semibold">{feature.title}</p>
                <p className="mt-2 text-sm text-muted-foreground">{feature.businessGoal}</p>
                <Badge tone={feature.status === "active" ? "success" : "neutral"}>{feature.status}</Badge>
              </div>
            ))}
          </div>
        ) : null}
      </div>
    </AppShell>
  );
}

export function WorkflowDefinitionsPage() {
  const query = useQuery({
    queryKey: ["workflow-definitions"],
    queryFn: async () => {
      const gateways = createGatewayBundle();
      return new ListWorkflowDefinitionsUseCase(gateways.workflowGateway).execute();
    },
  });

  return (
    <AppShell>
      <div className="space-y-6">
        <header>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Workflow Definitions
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">Definition registry</h1>
        </header>
        {query.isLoading ? <LoadingState /> : null}
        <div className="space-y-3">
          {query.data?.map((definition) => (
            <div key={definition.id} className="rounded-[1.6rem] border border-border bg-background/70 p-6">
              <p className="font-semibold">{definition.name}</p>
              <p className="mt-2 text-sm text-muted-foreground">{definition.description}</p>
              <p className="mt-4 text-xs uppercase tracking-[0.24em] text-muted-foreground">
                version {definition.version} · {definition.status}
              </p>
            </div>
          ))}
        </div>
      </div>
    </AppShell>
  );
}

export function WorkflowRunsPage() {
  const query = useQuery({
    queryKey: ["workflow-runs"],
    queryFn: async () => {
      const gateways = createGatewayBundle();
      return new ListWorkflowRunsUseCase(gateways.workflowGateway).execute();
    },
  });

  return (
    <AppShell>
      <div className="space-y-6">
        <header>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Workflow Runs
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">Execution timeline registry</h1>
        </header>
        {query.isLoading ? <LoadingState /> : null}
        <div className="space-y-3">
          {query.data?.map((run) => (
            <Link
              key={run.id}
              className="flex items-center justify-between rounded-[1.6rem] border border-border bg-background/70 p-5"
              to={routeTo("/workflow-runs")}
            >
              <div>
                <p className="font-semibold">{run.id}</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Started {new Date(run.startedAt).toLocaleString()}
                </p>
              </div>
              <Badge tone={statusTone(run.status)}>{run.status}</Badge>
            </Link>
          ))}
        </div>
      </div>
    </AppShell>
  );
}

export function ApprovalsPage() {
  const query = useQuery({
    queryKey: ["approvals"],
    queryFn: async () => {
      const gateways = createGatewayBundle();
      return new ListPendingApprovalsUseCase(gateways.workflowGateway).execute();
    },
  });

  return (
    <AppShell>
      <div className="space-y-6">
        <header>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Approval Center
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">Pending approval queue</h1>
        </header>
        {query.isLoading ? <LoadingState /> : null}
        <div className="space-y-3">
          {query.data?.map((item) => (
            <div key={item.approval.id} className="rounded-[1.6rem] border border-border bg-background/70 p-6">
              <p className="font-semibold">{item.feature?.title ?? item.run.id}</p>
              <p className="mt-2 text-sm text-muted-foreground">{item.step?.stepName ?? "Approval step"}</p>
              <p className="mt-4 text-sm text-muted-foreground">
                {item.output?.title ?? "No output preview available"}
              </p>
            </div>
          ))}
        </div>
      </div>
    </AppShell>
  );
}

export function OutputsPage() {
  const query = useQuery({
    queryKey: ["outputs"],
    queryFn: async () => {
      const gateways = createGatewayBundle();
      return new ListOutputsUseCase(gateways.workflowGateway).execute();
    },
  });

  return (
    <AppShell>
      <div className="space-y-6">
        <header>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Outputs
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">Output library</h1>
        </header>
        {query.isLoading ? <LoadingState /> : null}
        <div className="space-y-3">
          {query.data?.map((output) => (
            <div key={output.id} className="rounded-[1.6rem] border border-border bg-background/70 p-6">
              <p className="font-semibold">{output.title}</p>
              <p className="mt-2 text-sm text-muted-foreground">{output.outputType}</p>
            </div>
          ))}
        </div>
      </div>
    </AppShell>
  );
}

export function AiRunsPage() {
  const query = useQuery({
    queryKey: ["ai-runs"],
    queryFn: async () => {
      const gateways = createGatewayBundle();
      return new ListAiCallLogsUseCase(gateways.workflowGateway).execute();
    },
  });

  return (
    <AppShell>
      <div className="space-y-6">
        <header>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            AI Runs
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">Execution logs</h1>
        </header>
        {query.isLoading ? <LoadingState /> : null}
        <div className="space-y-3">
          {query.data?.map((log) => (
            <div key={log.id} className="rounded-[1.6rem] border border-border bg-background/70 p-6">
              <p className="font-semibold">{log.provider} / {log.model}</p>
              <p className="mt-2 text-sm text-muted-foreground">
                {log.inputTokens} input tokens - {log.outputTokens} output tokens - {log.status}
              </p>
            </div>
          ))}
        </div>
      </div>
    </AppShell>
  );
}

export function ArtifactManagementPage() {
  const query = useQuery({
    queryKey: ["artifact-management"],
    queryFn: async () => {
      const gateways = createGatewayBundle();
      return Promise.all([
        new GetStorageDriverUseCase(gateways.localRunnerGateway).execute(),
        new ListArtifactsUseCase(gateways.workflowGateway, gateways.localRunnerGateway).execute(),
      ]);
    },
  });

  return (
    <AppShell>
      <div className="space-y-6">
        <header>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Artifacts
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">Artifact management</h1>
        </header>
        {query.isLoading ? <LoadingState /> : null}
        <div className="space-y-3">
          {query.data?.[1].map((artifact) => (
            <Link
              key={artifact.artifactId}
              className="rounded-[1.6rem] border border-border bg-background/70 p-6 transition-colors hover:bg-muted/40"
              params={routeParams({ artifactId: artifact.artifactId })}
              to={routeTo("/artifacts/$artifactId")}
            >
              <p className="font-semibold">{artifact.title}</p>
              <p className="mt-2 text-sm text-muted-foreground">{artifact.localPath || artifact.remotePath || "No file path"}</p>
            </Link>
          ))}
        </div>
      </div>
    </AppShell>
  );
}

export function ArtifactMemoryPage() {
  const params = useParams({ strict: false }) as { artifactId?: string };
  const artifactId = params.artifactId ?? "";
  const query = useQuery({
    queryKey: ["artifact-memory", artifactId],
    queryFn: async () => {
      const gateways = createGatewayBundle();
      return gateways.localRunnerGateway.getArtifactById(artifactId);
    },
    enabled: Boolean(artifactId),
  });

  return (
    <AppShell>
      <div className="space-y-6">
        <header>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Artifact Memory
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">Artifact detail</h1>
        </header>
        {query.isLoading ? <LoadingState /> : null}
        {query.data ? (
          <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
            <p className="font-semibold">{query.data.title}</p>
            <p className="mt-2 text-sm text-muted-foreground">{query.data.localPath || query.data.remotePath || "No file path"}</p>
            <pre className="mt-4 overflow-auto rounded-2xl bg-card p-4 text-xs text-muted-foreground">
              {JSON.stringify(query.data, null, 2)}
            </pre>
          </div>
        ) : null}
      </div>
    </AppShell>
  );
}

export function SettingsPage() {
  const query = useQuery({
    queryKey: ["settings"],
    queryFn: async () => {
      const gateways = createGatewayBundle();
      const [health, providers, skills, flows, storageDriver] = await Promise.all([
        new CheckLocalRunnerHealthUseCase(gateways.localRunnerGateway).execute(),
        new ListLocalProvidersUseCase(gateways.localRunnerGateway).execute(),
        new ListLocalSkillsUseCase(gateways.localRunnerGateway).execute(),
        new ListLocalFlowsUseCase(gateways.localRunnerGateway).execute(),
        new GetStorageDriverUseCase(gateways.localRunnerGateway).execute(),
      ]);

      return { health, providers, skills, flows, storageDriver };
    },
  });

  return (
    <AppShell>
      <div className="space-y-8">
        <header className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
              Settings
            </p>
            <h1 className="mt-3 text-4xl font-semibold tracking-tight">Local runner boundary.</h1>
          </div>
          <p className="max-w-xl text-sm text-muted-foreground">
            The browser renders runner metadata from the local Go process but never executes shell commands directly.
          </p>
        </header>

        {query.isLoading ? <LoadingState /> : null}
        {query.data ? (
          <section className="grid gap-4 xl:grid-cols-[1.1fr_0.9fr]">
            <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
              <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
                Runner Health
              </p>
              <h2 className="mt-3 text-2xl font-semibold tracking-tight">Go Cobra local process</h2>
              <div className="mt-6 grid gap-3 sm:grid-cols-2">
                <div className="rounded-2xl border border-border bg-card/80 p-4">
                  <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">Base URL</p>
                  <p className="mt-2 break-words text-sm font-medium">{query.data.health.baseUrl}</p>
                </div>
                <div className="rounded-2xl border border-border bg-card/80 p-4">
                  <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">Version</p>
                  <p className="mt-2 break-words text-sm font-medium">{query.data.health.runnerVersion ?? "Unavailable"}</p>
                </div>
              </div>
            </div>

            <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
              <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
                Storage Driver
              </p>
              <h2 className="mt-3 text-2xl font-semibold tracking-tight">
                Artifact storage controls
              </h2>
              <pre className="mt-4 overflow-auto rounded-2xl bg-card p-4 text-xs text-muted-foreground">
                {JSON.stringify(query.data.storageDriver, null, 2)}
              </pre>
            </div>
          </section>
        ) : null}
      </div>
    </AppShell>
  );
}

export function PromptTemplatesPage() {
  return (
    <AppShell>
      <div className="space-y-6">
        <header>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Prompt Templates
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">Prompt template settings</h1>
        </header>
        <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <p className="text-sm text-muted-foreground">
            Prompt templates will be wired into the new React UI in the next implementation slice.
          </p>
        </section>
      </div>
    </AppShell>
  );
}
