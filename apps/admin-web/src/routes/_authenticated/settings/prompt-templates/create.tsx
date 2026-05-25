import { useState } from "react";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { SaveAiPromptTemplateInput } from "@/domain/gateway/ai-orchestration-gateway";
import { SavePromptTemplateUseCase } from "@/domain/usecase/ai-orchestration/save-prompt-template-usecase";

export const Route = createFileRoute("/_authenticated/settings/prompt-templates/create")({
  component: CreatePromptTemplatePage,
});

function emptyTemplate(): SaveAiPromptTemplateInput {
  return {
    projectId: null,
    stepType: "tech_spec",
    name: "",
    description: "",
    inputSchema: {},
    outputSchema: {},
    templateContent: "# Task\nDescribe the generation objective here.",
    providerPreference: null,
    modelPreference: null,
    version: 1,
    status: "active",
  };
}

export function CreatePromptTemplatePage() {
  const navigate = useNavigate();
  const gatewayBundle = createGatewayBundle();
  const savePromptTemplateUseCase = new SavePromptTemplateUseCase(
    gatewayBundle.aiOrchestrationGateway
  );
  const [template, setTemplate] = useState<SaveAiPromptTemplateInput>(emptyTemplate());
  const [inputSchemaText, setInputSchemaText] = useState("{}");
  const [outputSchemaText, setOutputSchemaText] = useState("{}");
  const [saving, setSaving] = useState(false);

  const save = async () => {
    if (!template.name.trim() || !template.stepType.trim() || !template.templateContent.trim()) {
      window.alert("Name, step type, and template content are required.");
      return;
    }

    setSaving(true);
    try {
      await savePromptTemplateUseCase.execute({
        ...template,
        name: template.name.trim(),
        stepType: template.stepType.trim(),
        description: template.description.trim(),
        inputSchema: parseSchemaField(inputSchemaText, "Input schema"),
        outputSchema: parseSchemaField(outputSchemaText, "Output schema"),
      });
      await navigate({ to: "/settings/prompt-templates" });
    } catch (error) {
      window.alert(error instanceof Error ? error.message : "Unable to save prompt template.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <PageFrame
      title="Create Prompt Template"
      description="Create a reusable prompt template for a workflow step type or a project-scoped generation flow."
      actions={
        <div className="flex flex-wrap gap-2">
          <Link to="/settings/prompt-templates">
            <Button variant="secondary">Back to templates</Button>
          </Link>
          <Button disabled={saving} onClick={() => void save()}>
            {saving ? "Saving..." : "Create Template"}
          </Button>
        </div>
      }
    >
      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Prompt Registry
          </p>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight">New prompt template</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            Define scope, step type, prompt body, and structured schemas for this template.
          </p>
        </div>

        <div className="mt-6 grid gap-3 md:grid-cols-2">
          <label className="space-y-2 text-sm">
            <span className="font-medium">Name</span>
            <input
              className="w-full rounded-2xl border border-border bg-background px-4 py-3"
              onChange={(event) =>
                setTemplate((current) => ({ ...current, name: event.target.value }))
              }
              placeholder="Global Tech Spec"
              value={template.name}
            />
          </label>
          <label className="space-y-2 text-sm">
            <span className="font-medium">Step Type</span>
            <input
              className="w-full rounded-2xl border border-border bg-background px-4 py-3"
              onChange={(event) =>
                setTemplate((current) => ({ ...current, stepType: event.target.value }))
              }
              placeholder="tech_spec"
              value={template.stepType}
            />
          </label>
          <label className="space-y-2 text-sm">
            <span className="font-medium">Project Id (optional)</span>
            <input
              className="w-full rounded-2xl border border-border bg-background px-4 py-3"
              onChange={(event) =>
                setTemplate((current) => ({
                  ...current,
                  projectId: event.target.value.trim() || null,
                }))
              }
              placeholder="Leave blank for global scope"
              value={template.projectId ?? ""}
            />
          </label>
          <label className="space-y-2 text-sm">
            <span className="font-medium">Version</span>
            <input
              className="w-full rounded-2xl border border-border bg-background px-4 py-3"
              min={1}
              onChange={(event) =>
                setTemplate((current) => ({
                  ...current,
                  version: Number(event.target.value || "1"),
                }))
              }
              type="number"
              value={template.version}
            />
          </label>
          <label className="space-y-2 text-sm">
            <span className="font-medium">Provider Preference</span>
            <input
              className="w-full rounded-2xl border border-border bg-background px-4 py-3"
              onChange={(event) =>
                setTemplate((current) => ({
                  ...current,
                  providerPreference: event.target.value || null,
                }))
              }
              placeholder="claude / codex / gemini"
              value={template.providerPreference ?? ""}
            />
          </label>
          <label className="space-y-2 text-sm">
            <span className="font-medium">Model Preference</span>
            <input
              className="w-full rounded-2xl border border-border bg-background px-4 py-3"
              onChange={(event) =>
                setTemplate((current) => ({
                  ...current,
                  modelPreference: event.target.value || null,
                }))
              }
              placeholder="claude-sonnet-4"
              value={template.modelPreference ?? ""}
            />
          </label>
          <label className="space-y-2 text-sm md:col-span-2">
            <span className="font-medium">Description</span>
            <textarea
              className="min-h-24 w-full rounded-2xl border border-border bg-background px-4 py-3"
              onChange={(event) =>
                setTemplate((current) => ({ ...current, description: event.target.value }))
              }
              value={template.description}
            />
          </label>
          <label className="space-y-2 text-sm">
            <span className="font-medium">Status</span>
            <select
              className="w-full rounded-2xl border border-border bg-background px-4 py-3"
              onChange={(event) =>
                setTemplate((current) => ({
                  ...current,
                  status: event.target.value as NonNullable<SaveAiPromptTemplateInput["status"]>,
                }))
              }
              value={template.status}
            >
              <option value="active">active</option>
              <option value="archived">archived</option>
            </select>
          </label>
          <label className="space-y-2 text-sm md:col-span-2">
            <span className="font-medium">Template Content</span>
            <textarea
              className="min-h-48 w-full rounded-2xl border border-border bg-background px-4 py-3 font-mono text-sm"
              onChange={(event) =>
                setTemplate((current) => ({ ...current, templateContent: event.target.value }))
              }
              value={template.templateContent}
            />
          </label>
          <label className="space-y-2 text-sm">
            <span className="font-medium">Input Schema (JSON)</span>
            <textarea
              className="min-h-32 w-full rounded-2xl border border-border bg-background px-4 py-3 font-mono text-sm"
              onChange={(event) => setInputSchemaText(event.target.value)}
              value={inputSchemaText}
            />
          </label>
          <label className="space-y-2 text-sm">
            <span className="font-medium">Output Schema (JSON)</span>
            <textarea
              className="min-h-32 w-full rounded-2xl border border-border bg-background px-4 py-3 font-mono text-sm"
              onChange={(event) => setOutputSchemaText(event.target.value)}
              value={outputSchemaText}
            />
          </label>
        </div>
      </section>
    </PageFrame>
  );
}

function parseSchemaField(value: string, label: string) {
  try {
    const parsed = JSON.parse(value);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      throw new Error(`${label} must be a JSON object.`);
    }
    return parsed as Record<string, unknown>;
  } catch (error) {
    if (error instanceof Error && error.message.includes("must be a JSON object")) {
      throw error;
    }
    throw new Error(`${label} must be valid JSON.`);
  }
}
