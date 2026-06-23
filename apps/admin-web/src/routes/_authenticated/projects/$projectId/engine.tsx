import { useMutation } from "@tanstack/react-query";
import { Link, createFileRoute, useRouter } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { LocalRunnerEngineStatus } from "@/domain/model/entity/local-runner";
import { Badge } from "@/presentation/components/ui/badge";

type EngineLoaderData = {
  projectId: string;
  projectName: string;
  bindingCount: number;
  workingDirectory: string | null;
  engineStatus: LocalRunnerEngineStatus | null;
  engineError: string | null;
};

const TOOL_HINTS: Record<string, string> = {
  gitnexus: "Install via npm (`npm install -g gitnexus`) or use `npx gitnexus --version` to verify it resolves.",
  rtk: "Install via Homebrew (`brew install rtk`) or the RTK install script documented in SD-03.",
  node: "Install Node.js on this machine, then rerun engine initialization.",
  skill_pack: "Use Initialize / Re-sync to copy the bundled FlowPilot skills into .claude, .codex, and .gemini.",
};

export const Route = createFileRoute("/_authenticated/projects/$projectId/engine")({
  loader: async ({ params }): Promise<EngineLoaderData> => {
    const gateways = await createGatewayBundle();
    const [project, bindings] = await Promise.all([
      gateways.projectGateway.getProjectById(params.projectId),
      gateways.projectGateway.listProjectWorkspaceBindings(params.projectId),
    ]);

    const workingDirectory = bindings[0]?.localPath?.trim() || null;
    if (!project) {
      return {
        projectId: params.projectId,
        projectName: "",
        bindingCount: bindings.length,
        workingDirectory,
        engineStatus: null,
        engineError: null,
      };
    }

    if (!workingDirectory) {
      return {
        projectId: params.projectId,
        projectName: project.name,
        bindingCount: bindings.length,
        workingDirectory: null,
        engineStatus: null,
        engineError: null,
      };
    }

    try {
      return {
        projectId: params.projectId,
        projectName: project.name,
        bindingCount: bindings.length,
        workingDirectory,
        engineStatus: await gateways.localRunnerGateway.getEngineStatus(
          params.projectId,
          workingDirectory,
        ),
        engineError: null,
      };
    } catch (error) {
      return {
        projectId: params.projectId,
        projectName: project.name,
        bindingCount: bindings.length,
        workingDirectory,
        engineStatus: null,
        engineError: error instanceof Error ? error.message : "Unable to load engine status.",
      };
    }
  },
  component: ProjectEnginePage,
});

function toolBadgeTone(status: string) {
  switch (status) {
    case "ok":
      return "success";
    case "missing":
      return "danger";
    default:
      return "neutral";
  }
}

function providerBadgeTone(current: boolean, present: boolean) {
  if (current) return "success";
  if (present) return "neutral";
  return "danger";
}

function summarizeCapability(engineStatus: LocalRunnerEngineStatus) {
  const structure = engineStatus.capability.structureTier === "gitnexus"
    ? "GitNexus available: structure analysis can use the indexed dependency graph."
    : "GitNexus missing: structure analysis falls back to file and git heuristics.";
  const decision = engineStatus.capability.decisionTier === "full"
    ? "Specs detected: decision support can use requirements docs."
    : "No specs detected: decisions fall back to git and local repo context.";

  return { structure, decision };
}

function ProjectEnginePage() {
  const detail = Route.useLoaderData() as EngineLoaderData;
  const router = useRouter();

  const initMutation = useMutation({
    mutationFn: async () => {
      if (!detail.workingDirectory) {
        throw new Error("Add a project directory binding before initializing the engine.");
      }
      const gateways = await createGatewayBundle();
      return await gateways.localRunnerGateway.initEngine(detail.projectId, {
        workingDirectory: detail.workingDirectory,
        trigger: "manual",
      });
    },
    onSuccess: async () => {
      await router.invalidate();
    },
    onError: (error) => {
      window.alert(error instanceof Error ? error.message : "Unable to initialize the engine.");
    },
  });

  const capabilitySummary = detail.engineStatus ? summarizeCapability(detail.engineStatus) : null;
  const lastInit = initMutation.data?.lastInit ?? detail.engineStatus?.lastInit ?? null;

  return (
    <PageFrame
      description="Inspect machine-local engine readiness for this bound project and re-sync the FlowPilot setup artifacts."
      title="Engine"
    >
      <div className="space-y-6">
        <ProjectSectionNav projectId={detail.projectId} />

        <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <h2 className="text-xl font-semibold">Engine workspace</h2>
              <p className="mt-2 text-sm text-muted-foreground">
                The Engine tab operates on the primary directory binding for this machine.
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Badge>{detail.bindingCount} bindings</Badge>
              <Badge>{detail.workingDirectory ? "bound" : "unbound"}</Badge>
            </div>
          </div>

          <div className="mt-4 rounded-2xl border border-border bg-card px-4 py-3 text-sm">
            {detail.workingDirectory ? (
              <>
                <p className="font-medium">Primary binding</p>
                <p className="mt-1 break-all text-muted-foreground">{detail.workingDirectory}</p>
              </>
            ) : (
              <>
                <p className="font-medium">No directory binding yet</p>
                <p className="mt-1 text-muted-foreground">
                  Add one from{" "}
                  <Link className="underline" to="/projects/$projectId/directory-bindings" params={{ projectId: detail.projectId }}>
                    Directory Binding
                  </Link>
                  {" "}before using the engine setup flow.
                </p>
              </>
            )}
          </div>

          {detail.engineError ? (
            <p className="mt-4 text-sm text-danger">{detail.engineError}</p>
          ) : null}
        </section>

        {detail.engineStatus ? (
          <>
            <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div>
                  <h2 className="text-xl font-semibold">Tooling health</h2>
                  <p className="mt-2 text-sm text-muted-foreground">
                    These checks are machine-local and describe what the current runner can use.
                  </p>
                </div>
                <Badge>{detail.engineStatus.initialized ? "initialized" : "not initialized"}</Badge>
              </div>

              <div className="mt-5 space-y-3">
                {detail.engineStatus.tooling.map((tool) => (
                  <div
                    key={tool.tool}
                    className="rounded-2xl border border-border bg-card px-4 py-4"
                  >
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <p className="font-medium">{tool.tool}</p>
                        <p className="mt-1 text-sm text-muted-foreground">
                          Checked {new Date(tool.checkedAt).toLocaleString()}
                        </p>
                      </div>
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge tone={toolBadgeTone(tool.status)}>{tool.status}</Badge>
                        <Badge>{tool.version || "no version"}</Badge>
                      </div>
                    </div>

                    {tool.status !== "ok" ? (
                      <p className="mt-3 text-sm text-muted-foreground">
                        {TOOL_HINTS[tool.tool] ?? "Install this dependency on the local machine, then rerun engine initialization."}
                      </p>
                    ) : null}
                  </div>
                ))}
              </div>
            </section>

            <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div>
                  <h2 className="text-xl font-semibold">Capability tier</h2>
                  <p className="mt-2 text-sm text-muted-foreground">
                    This is the effective engine mode for the current binding.
                  </p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Badge>{detail.engineStatus.capability.structureTier}</Badge>
                  <Badge>{detail.engineStatus.capability.decisionTier}</Badge>
                </div>
              </div>

              <div className="mt-5 grid gap-4 md:grid-cols-2">
                <div className="rounded-2xl border border-border bg-card px-4 py-4">
                  <p className="font-medium">Structure</p>
                  <p className="mt-2 text-sm text-muted-foreground">{capabilitySummary?.structure}</p>
                </div>
                <div className="rounded-2xl border border-border bg-card px-4 py-4">
                  <p className="font-medium">Decision support</p>
                  <p className="mt-2 text-sm text-muted-foreground">{capabilitySummary?.decision}</p>
                </div>
              </div>

              <div className="mt-4 rounded-2xl border border-border bg-card px-4 py-4">
                <p className="font-medium">Detected languages</p>
                <div className="mt-3 flex flex-wrap gap-2">
                  {(detail.engineStatus.capability.languages.length > 0
                    ? detail.engineStatus.capability.languages
                    : ["none detected"]).map((language: string) => (
                    <Badge key={language}>{language}</Badge>
                  ))}
                </div>
              </div>
            </section>

            <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div>
                  <h2 className="text-xl font-semibold">Skill pack</h2>
                  <p className="mt-2 text-sm text-muted-foreground">
                    FlowPilot installs the bundled skill pack into Claude, Codex, and Gemini provider folders.
                  </p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Badge>{`pack v${detail.engineStatus.skillPack.packVersion}`}</Badge>
                  <Badge tone={detail.engineStatus.skillPack.current ? "success" : "neutral"}>
                    {detail.engineStatus.skillPack.current ? "current" : "needs sync"}
                  </Badge>
                </div>
              </div>

              <div className="mt-5 space-y-3">
                {detail.engineStatus.skillPack.skills.map((skill) => (
                  <div
                    key={skill.name}
                    className="rounded-2xl border border-border bg-card px-4 py-4"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <p className="font-medium">{skill.name}</p>
                      <div className="flex flex-wrap gap-2">
                        {skill.providers.map((provider) => (
                          <Badge
                            key={`${skill.name}-${provider.provider}`}
                            tone={providerBadgeTone(provider.current, provider.present)}
                          >
                            {provider.provider.replace(".", "")}: {provider.current ? "current" : provider.present ? "present" : "missing"}
                          </Badge>
                        ))}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </section>

            <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div>
                  <h2 className="text-xl font-semibold">Actions</h2>
                  <p className="mt-2 text-sm text-muted-foreground">
                    Initialize or re-sync tooling status, the bundled skill pack, and the local git-derived engine artifacts.
                  </p>
                </div>
                <Button
                  type="button"
                  onClick={() => initMutation.mutate()}
                  disabled={!detail.workingDirectory || initMutation.isPending}
                >
                  {initMutation.isPending ? "Initializing..." : "Initialize / Re-sync engine"}
                </Button>
              </div>

              {lastInit ? (
                <div className="mt-5 rounded-2xl border border-border bg-card px-4 py-4">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="font-medium">Last init</p>
                    <Badge tone={lastInit.status === "success" ? "success" : "neutral"}>
                      {lastInit.status}
                    </Badge>
                    {lastInit.skipped ? <Badge>{lastInit.skipReason || "skipped"}</Badge> : null}
                  </div>

                  <p className="mt-2 text-sm text-muted-foreground">
                    Attempted {new Date(lastInit.attemptedAt).toLocaleString()}
                  </p>

                  <div className="mt-4 flex flex-wrap gap-2">
                    <Badge>{`${lastInit.install.installedPaths.length} installed`}</Badge>
                    <Badge>{`${lastInit.install.skippedPaths.length} skipped`}</Badge>
                    <Badge tone={lastInit.install.errors.length === 0 ? "success" : "danger"}>
                      {`${lastInit.install.errors.length} install errors`}
                    </Badge>
                  </div>

                  <div className="mt-4 space-y-2">
                    {lastInit.steps.map((step) => (
                      <div key={step.step} className="rounded-xl border border-border px-3 py-2 text-sm">
                        <div className="flex flex-wrap items-center justify-between gap-2">
                          <span className="font-medium">{step.step}</span>
                          <Badge tone={step.outcome === "ok" ? "success" : step.outcome === "skipped" ? "neutral" : "danger"}>
                            {step.outcome}
                          </Badge>
                        </div>
                        {step.detail ? (
                          <p className="mt-1 text-muted-foreground">{step.detail}</p>
                        ) : null}
                        {step.errorMessage ? (
                          <p className="mt-1 text-danger">{step.errorMessage}</p>
                        ) : null}
                      </div>
                    ))}
                  </div>

                  {detail.engineStatus.warnings?.length ? (
                    <div className="mt-4 space-y-1 text-sm text-muted-foreground">
                      {detail.engineStatus.warnings.map((warning: string) => (
                        <p key={warning}>{warning}</p>
                      ))}
                    </div>
                  ) : null}
                </div>
              ) : null}
            </section>
          </>
        ) : null}
      </div>
    </PageFrame>
  );
}
