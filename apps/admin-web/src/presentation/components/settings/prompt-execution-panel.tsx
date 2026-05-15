"use client";

import { useMemo, useState } from "react";

import { Button } from "@/presentation/components/ui/button";
import { Badge } from "@/presentation/components/ui/badge";

type ProviderOption = {
  key: string;
  label: string;
  installed: boolean;
};

type SkillOption = {
  id: string;
  name: string;
};

type FlowOption = {
  id: string;
  name: string;
};

type ExecutionResult = {
  status: "success" | "failed";
  runId: string;
  providerKey: string;
  command: string;
  stdoutSummary: string;
  stderrSummary: string;
  outputMarkdown: string;
  artifactPaths: string[];
  startedAt: string;
  completedAt: string;
  exitCode: number;
  errorMessage: string | null;
};

interface PromptExecutionPanelProps {
  providers: ProviderOption[];
  skills: SkillOption[];
  flows: FlowOption[];
}

export function PromptExecutionPanel({
  providers,
  skills,
  flows,
}: PromptExecutionPanelProps) {
  const defaultProvider = useMemo(
    () => providers.find((provider) => provider.key === "codex" && provider.installed) ?? providers[0] ?? null,
    [providers],
  );
  const [providerKey, setProviderKey] = useState(defaultProvider?.key ?? "");
  const [skillId, setSkillId] = useState(skills[0]?.id ?? "");
  const [flowId, setFlowId] = useState(flows[0]?.id ?? "");
  const [prompt, setPrompt] = useState("");
  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [result, setResult] = useState<ExecutionResult | null>(null);

  async function handleSubmit() {
    setLoading(true);
    setErrorMessage(null);
    setResult(null);

    try {
      const response = await fetch("/api/local-runner/execute", {
        method: "POST",
        headers: {
          "content-type": "application/json",
        },
        body: JSON.stringify({
          providerKey,
          prompt,
          skillIds: skillId ? [skillId] : [],
          flowId: flowId || null,
          contextSourceIds: [],
          timeoutMs: 600000,
          workingDirectory: null,
        }),
      });

      const data = (await response.json()) as ExecutionResult | { error?: string };

      if (!response.ok) {
        throw new Error("error" in data && data.error ? data.error : "Prompt execution failed.");
      }

      setResult(data as ExecutionResult);
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : "Prompt execution failed.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Use Case 01
          </p>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight">
            Prompt to local Codex execution
          </h2>
          <p className="mt-2 max-w-3xl text-sm text-muted-foreground">
            Type a prompt, choose the local provider, and send it through the runner
            without leaving the browser.
          </p>
        </div>
        <Badge tone={providerKey === "codex" ? "success" : "warning"}>
          {providerKey || "no provider"}
        </Badge>
      </div>

      <div className="mt-6 space-y-4">
        <div className="grid gap-4 lg:grid-cols-3">
          <label className="space-y-2">
            <span className="text-sm font-medium">Provider</span>
            <select
              className="w-full rounded-2xl border border-border bg-card px-4 py-3 text-sm outline-none"
              value={providerKey}
              onChange={(event) => setProviderKey(event.target.value)}
            >
              {providers.map((provider) => (
                <option key={provider.key} value={provider.key}>
                  {provider.label} {provider.installed ? "" : "(missing)"}
                </option>
              ))}
            </select>
          </label>

          <label className="space-y-2">
            <span className="text-sm font-medium">Skill</span>
            <select
              className="w-full rounded-2xl border border-border bg-card px-4 py-3 text-sm outline-none"
              value={skillId}
              onChange={(event) => setSkillId(event.target.value)}
            >
              <option value="">None</option>
              {skills.map((skill) => (
                <option key={skill.id} value={skill.id}>
                  {skill.name}
                </option>
              ))}
            </select>
          </label>

          <label className="space-y-2">
            <span className="text-sm font-medium">Flow</span>
            <select
              className="w-full rounded-2xl border border-border bg-card px-4 py-3 text-sm outline-none"
              value={flowId}
              onChange={(event) => setFlowId(event.target.value)}
            >
              <option value="">None</option>
              {flows.map((flow) => (
                <option key={flow.id} value={flow.id}>
                  {flow.name}
                </option>
              ))}
            </select>
          </label>
        </div>

        <label className="block space-y-2">
          <span className="text-sm font-medium">Prompt</span>
          <textarea
            className="min-h-44 w-full rounded-[1.4rem] border border-border bg-card px-4 py-4 text-sm outline-none"
            placeholder="Describe the task you want Codex to handle..."
            value={prompt}
            onChange={(event) => setPrompt(event.target.value)}
          />
        </label>

        <div className="flex items-center gap-3">
          <Button
            type="button"
            disabled={loading || !providerKey}
            onClick={() => {
              void handleSubmit();
            }}
          >
            {loading ? "Running..." : "Run prompt"}
          </Button>
          {errorMessage ? (
            <p className="text-sm text-danger">{errorMessage}</p>
          ) : (
            <p className="text-sm text-muted-foreground">
              The runner writes local artifacts under `.flowpilot/artifacts/`.
            </p>
          )}
        </div>
      </div>

      {result ? (
        <div className="mt-6 space-y-4 rounded-[1.4rem] border border-border bg-card/80 p-5">
          <div className="flex flex-wrap items-center gap-3">
            <Badge tone={result.status === "success" ? "success" : "danger"}>
              {result.status}
            </Badge>
            <p className="text-sm text-muted-foreground">Run: {result.runId}</p>
            <p className="text-sm text-muted-foreground">Exit code: {result.exitCode}</p>
          </div>

          <div className="grid gap-3 lg:grid-cols-2">
            <div>
              <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
                Command
              </p>
              <p className="mt-2 break-words rounded-2xl border border-border bg-background px-4 py-3 text-sm">
                {result.command}
              </p>
            </div>
            <div>
              <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
                Artifacts
              </p>
              <ul className="mt-2 space-y-2 rounded-2xl border border-border bg-background px-4 py-3 text-sm">
                {result.artifactPaths.map((path) => (
                  <li key={path} className="break-words">
                    {path}
                  </li>
                ))}
              </ul>
            </div>
          </div>

          <div className="grid gap-3 lg:grid-cols-2">
            <pre className="whitespace-pre-wrap rounded-2xl border border-border bg-background px-4 py-3 text-sm">
              {result.stdoutSummary || "No stdout captured."}
            </pre>
            <pre className="whitespace-pre-wrap rounded-2xl border border-border bg-background px-4 py-3 text-sm">
              {result.stderrSummary || "No stderr captured."}
            </pre>
          </div>

          {result.outputMarkdown ? (
            <pre className="whitespace-pre-wrap rounded-2xl border border-border bg-background px-4 py-3 text-sm">
              {result.outputMarkdown}
            </pre>
          ) : null}

          {result.errorMessage ? (
            <p className="text-sm text-danger">{result.errorMessage}</p>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}
