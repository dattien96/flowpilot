import { createGatewayBundle } from "@/data/repository/factory";
import { CheckLocalRunnerHealthUseCase } from "@/domain/usecase/local-runner/check-local-runner-health-usecase";
import { ListLocalFlowsUseCase } from "@/domain/usecase/local-runner/list-local-flows-usecase";
import { ListLocalProvidersUseCase } from "@/domain/usecase/local-runner/list-local-providers-usecase";
import { ListLocalSkillsUseCase } from "@/domain/usecase/local-runner/list-local-skills-usecase";
import { Badge } from "@/presentation/components/ui/badge";

function DetailRow({
  label,
  value,
}: Readonly<{ label: string; value: string }>) {
  return (
    <div className="rounded-2xl border border-border bg-card/80 p-4">
      <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
        {label}
      </p>
      <p className="mt-2 break-words text-sm font-medium">{value}</p>
    </div>
  );
}

export default async function SettingsPage() {
  const gateways = await createGatewayBundle();
  const [health, providers, skills, flows] = await Promise.all([
    new CheckLocalRunnerHealthUseCase(gateways.localRunnerGateway).execute(),
    new ListLocalProvidersUseCase(gateways.localRunnerGateway).execute(),
    new ListLocalSkillsUseCase(gateways.localRunnerGateway).execute(),
    new ListLocalFlowsUseCase(gateways.localRunnerGateway).execute(),
  ]);

  const runnerTone = health.status === "online" ? "success" : "danger";

  return (
    <div className="space-y-8">
      <header className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Settings
          </p>
          <h1 className="mt-3 text-4xl font-semibold tracking-tight">
            Local runner boundary.
          </h1>
        </div>
        <p className="max-w-xl text-sm text-muted-foreground">
          The admin web reads provider, skill, and flow metadata from a local Go
          Cobra process. The browser never executes shell commands directly.
        </p>
      </header>

      <section className="grid gap-4 xl:grid-cols-[1.1fr_0.9fr]">
        <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
                Runner Health
              </p>
              <h2 className="mt-3 text-2xl font-semibold tracking-tight">
                Go Cobra local process
              </h2>
            </div>
            <Badge tone={runnerTone}>{health.status}</Badge>
          </div>

          <div className="mt-6 grid gap-3 sm:grid-cols-2">
            <DetailRow label="Base URL" value={health.baseUrl} />
            <DetailRow label="Version" value={health.runnerVersion ?? "Unavailable"} />
            <DetailRow label="Workspace" value={health.cwd ?? "Unavailable"} />
            <DetailRow label="Platform" value={health.os ?? "Unavailable"} />
          </div>

          <p className="mt-4 text-sm text-muted-foreground">
            {health.errorMessage ??
              "The runner is reachable from the Next.js server and can expose provider, skill, and flow metadata."}
          </p>
        </div>

        <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Provider Detection
          </p>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight">
            Installed AI CLIs
          </h2>
          <div className="mt-6 space-y-3">
            {providers.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                Start the local runner to detect Codex, Claude Code, and Gemini.
              </p>
            ) : (
              providers.map((provider) => (
                <div
                  key={provider.key}
                  className="rounded-2xl border border-border bg-card px-4 py-4"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <p className="font-semibold">{provider.label}</p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {provider.binaryPath ?? "Binary not found on PATH"}
                      </p>
                    </div>
                    <Badge tone={provider.installed ? "success" : "danger"}>
                      {provider.installed ? "installed" : "missing"}
                    </Badge>
                  </div>
                  <p className="mt-3 text-sm text-muted-foreground">
                    {provider.version || provider.installHint || "No version information available."}
                  </p>
                </div>
              ))
            )}
          </div>
        </div>
      </section>

      <section className="grid gap-4 xl:grid-cols-2">
        <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Skill Brain
          </p>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight">
            Local skills markdown
          </h2>
          <p className="mt-2 text-sm text-muted-foreground">
            The runner scans `.agents/skills` from the workspace root and exposes
            the discovered markdown files to the web.
          </p>
          <div className="mt-6 space-y-3">
            {skills.length === 0 ? (
              <p className="text-sm text-muted-foreground">No skills found yet.</p>
            ) : (
              skills.slice(0, 6).map((skill) => (
                <div
                  key={skill.id}
                  className="rounded-2xl border border-border bg-card px-4 py-4"
                >
                  <p className="font-semibold">{skill.name}</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {skill.filePath}
                  </p>
                  <p className="mt-3 text-sm text-muted-foreground">
                    {skill.description || "Markdown skill available for prompt assembly."}
                  </p>
                </div>
              ))
            )}
          </div>
        </div>

        <div className="rounded-[1.6rem] border border-border bg-background/70 p-6">
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Flow Brain
          </p>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight">
            Local workflow markdown
          </h2>
          <p className="mt-2 text-sm text-muted-foreground">
            The runner scans `.agents/flows` so the web can keep flow definitions
            local and versioned alongside the codebase.
          </p>
          <div className="mt-6 space-y-3">
            {flows.length === 0 ? (
              <p className="text-sm text-muted-foreground">No flows found yet.</p>
            ) : (
              flows.slice(0, 6).map((flow) => (
                <div
                  key={flow.id}
                  className="rounded-2xl border border-border bg-card px-4 py-4"
                >
                  <p className="font-semibold">{flow.name}</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {flow.filePath}
                  </p>
                  <p className="mt-3 text-sm text-muted-foreground">
                    {flow.description || "Workflow markdown available for runner execution."}
                  </p>
                </div>
              ))
            )}
          </div>
        </div>
      </section>

      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          Operating Rule
        </p>
        <h2 className="mt-3 text-2xl font-semibold tracking-tight">
          Presentation stays declarative
        </h2>
        <p className="mt-3 max-w-3xl text-muted-foreground">
          Presentation can ask the domain for runner data and render it, but it
          must not own shell execution, provider selection side effects, or
          filesystem discovery logic.
        </p>
      </section>
    </div>
  );
}
