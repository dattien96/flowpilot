import type { ReactNode } from "react";
import { useRef, useState } from "react";
import { createFileRoute, Link, Outlet, useLocation } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { SaveAiPromptTemplateInput } from "@/domain/gateway/ai-orchestration-gateway";
import type { AiPromptTemplate } from "@/domain/model/entity/ai-orchestration";
import { ListPromptTemplatesUseCase } from "@/domain/usecase/ai-orchestration/list-prompt-templates-usecase";
import { SavePromptTemplateUseCase } from "@/domain/usecase/ai-orchestration/save-prompt-template-usecase";
import { Badge } from "@/presentation/components/ui/badge";

export const Route = createFileRoute("/_authenticated/settings/prompt-templates")({
  loader: async () => {
    const gateways = await createGatewayBundle();
    const templates = await new ListPromptTemplatesUseCase(
      gateways.aiOrchestrationGateway
    ).execute();
    return { templates };
  },
  component: PromptTemplatesPage,
});

interface EditablePromptTemplate extends AiPromptTemplate {
  inputSchemaText: string;
  outputSchemaText: string;
}

export function PromptTemplatesPage() {
  const location = useLocation();
  const { templates } = Route.useLoaderData();
  const gatewayBundle = useRef(createGatewayBundle());
  const savePromptTemplateUseCase = useRef(
    new SavePromptTemplateUseCase(gatewayBundle.current.aiOrchestrationGateway)
  );

  if (location.pathname !== "/settings/prompt-templates") {
    return <Outlet />;
  }

  return (
    <PromptTemplatesContent
      initialTemplates={templates}
      onSave={(template) => savePromptTemplateUseCase.current.execute(template)}
    />
  );
}

export function PromptTemplatesContent({
  initialTemplates,
  onSave,
}: {
  initialTemplates: AiPromptTemplate[];
  onSave: (template: SaveAiPromptTemplateInput) => Promise<AiPromptTemplate>;
}) {
  const [templates, setTemplates] = useState<EditablePromptTemplate[]>(
    initialTemplates.map(toEditableTemplate)
  );
  const [savingId, setSavingId] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<string | null>(null);
  const [feedbackTone, setFeedbackTone] = useState<"danger" | "success">("success");

  const activeCount = templates.filter((template) => template.status === "active").length;
  const projectScopedCount = templates.filter((template) => template.projectId).length;
  const globalCount = templates.length - projectScopedCount;

  const updateTemplate = (id: string, patch: Partial<EditablePromptTemplate>) => {
    setTemplates((current) =>
      current.map((template) => (template.id === id ? { ...template, ...patch } : template))
    );
  };

  const saveTemplate = async (template: EditablePromptTemplate) => {
    if (!template.name.trim() || !template.stepType.trim() || !template.templateContent.trim()) {
      setFeedbackTone("danger");
      setFeedback("Name, step type, and template content are required.");
      return;
    }

    setSavingId(template.id);
    setFeedback(null);

    try {
      const saved = await onSave({
        id: template.id,
        projectId: template.projectId || null,
        stepType: template.stepType.trim(),
        name: template.name.trim(),
        description: template.description.trim(),
        inputSchema: parseSchemaField(template.inputSchemaText, "Input schema"),
        outputSchema: parseSchemaField(template.outputSchemaText, "Output schema"),
        templateContent: template.templateContent,
        providerPreference: template.providerPreference || null,
        modelPreference: template.modelPreference || null,
        version: template.version,
        status: template.status,
      });

      setTemplates((current) =>
        current.map((item) => (item.id === template.id ? toEditableTemplate(saved) : item))
      );
      setFeedbackTone("success");
      setFeedback(`Saved ${saved.name} (v${saved.version}).`);
    } catch (error) {
      setFeedbackTone("danger");
      setFeedback(
        error instanceof Error ? error.message : "Unable to save prompt template."
      );
    } finally {
      setSavingId(null);
    }
  };

  return (
    <PageFrame
      title="Prompt Templates"
      description="Step-oriented AI prompt templates with project scope, model preferences, and append-only versioning."
      actions={
        <div className="flex flex-wrap gap-2">
          <Link to="/settings/prompt-templates/create">
            <Button variant="secondary">Create Template</Button>
          </Link>
        </div>
      }
    >
      <div className="grid gap-3 md:grid-cols-3">
        <SummaryCard label="Templates" value={String(templates.length)} />
        <SummaryCard label="Active" value={String(activeCount)} />
        <SummaryCard label="Project Scoped" value={String(projectScopedCount)} />
      </div>

      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
              Prompt Registry
            </p>
            <h3 className="mt-3 text-2xl font-semibold tracking-tight">
              Versioned template catalog
            </h3>
            <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
              Global templates stay reusable across projects, while project-scoped templates
              override provider or model preferences for a specific workflow step type.
            </p>
          </div>
          <div className="rounded-2xl border border-border bg-card/80 px-4 py-3 text-right text-sm">
            <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">Scope Mix</p>
            <p className="mt-2 font-semibold">
              {globalCount} global / {projectScopedCount} project
            </p>
          </div>
        </div>

        {feedback ? (
          <div
            className={`mt-6 rounded-2xl border px-4 py-3 text-sm ${
              feedbackTone === "success"
                ? "border-success/30 bg-success/10 text-success"
                : "border-danger/30 bg-danger/10 text-danger"
            }`}
          >
            {feedback}
          </div>
        ) : null}

        <div className="mt-6 space-y-4">
          {templates.map((template) => (
            <TemplateRow
              key={template.id}
              onUpdate={updateTemplate}
              savingId={savingId}
              template={template}
              onSave={saveTemplate}
            >
            </TemplateRow>
          ))}
        </div>
      </section>
    </PageFrame>
  );
}

function SummaryCard({ label, value }: Readonly<{ label: string; value: string }>) {
  return (
    <div className="rounded-2xl border border-border bg-card/80 p-4">
      <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <p className="mt-2 text-2xl font-semibold tracking-tight">{value}</p>
    </div>
  );
}

function FormField({
  children,
  label,
}: Readonly<{ children: ReactNode; label: string }>) {
  return (
    <label className="space-y-2 text-sm">
      <span className="font-medium">{label}</span>
      {children}
    </label>
  );
}

function TemplateRow({
  onSave,
  onUpdate,
  savingId,
  template,
}: Readonly<{
  onSave: (template: EditablePromptTemplate) => Promise<void>;
  onUpdate: (id: string, patch: Partial<EditablePromptTemplate>) => void;
  savingId: string | null;
  template: EditablePromptTemplate;
}>) {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <details
      open={isOpen}
      className="rounded-[1.4rem] border border-border bg-card/80 p-5"
    >
      <summary
        className="flex cursor-pointer list-none flex-wrap items-start justify-between gap-3"
        onClick={(event) => {
          event.preventDefault();
          setIsOpen((current) => !current);
        }}
      >
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="text-lg font-semibold">{template.name}</h4>
            <Badge tone={template.status === "active" ? "success" : "warning"}>
              {template.status}
            </Badge>
            <Badge tone="neutral">{template.projectId ? "project" : "global"}</Badge>
          </div>
          <p className="mt-2 text-sm text-muted-foreground">
            Step type{" "}
            <span className="font-medium text-foreground">{template.stepType}</span>
            {" | "}Version {template.version}
            {" | "}Updated {formatTimestamp(template.updatedAt)}
          </p>
        </div>
        <span className="text-xs font-semibold uppercase tracking-[0.18em] text-muted-foreground">
          {isOpen ? "COLLAPSE" : "EXPAND"}
        </span>
      </summary>

      <div className="mt-5 grid gap-3 md:grid-cols-2">
        <FormField label="Name">
          <input
            className="w-full rounded-2xl border border-border bg-background px-4 py-3"
            onChange={(event) => onUpdate(template.id, { name: event.target.value })}
            value={template.name}
          />
        </FormField>
        <FormField label="Step Type">
          <input
            className="w-full rounded-2xl border border-border bg-background px-4 py-3"
            onChange={(event) => onUpdate(template.id, { stepType: event.target.value })}
            value={template.stepType}
          />
        </FormField>
        <FormField label="Project Id (optional)">
          <input
            className="w-full rounded-2xl border border-border bg-background px-4 py-3"
            onChange={(event) =>
              onUpdate(template.id, {
                projectId: event.target.value.trim() || null,
              })
            }
            placeholder="Leave blank for global scope"
            value={template.projectId ?? ""}
          />
        </FormField>
        <FormField label="Version">
          <input
            className="w-full rounded-2xl border border-border bg-background px-4 py-3"
            min={1}
            onChange={(event) =>
              onUpdate(template.id, {
                version: Number(event.target.value || "1"),
              })
            }
            type="number"
            value={template.version}
          />
        </FormField>
        <FormField label="Provider Preference">
          <input
            className="w-full rounded-2xl border border-border bg-background px-4 py-3"
            onChange={(event) =>
              onUpdate(template.id, {
                providerPreference: event.target.value || null,
              })
            }
            placeholder="claude / codex / gemini"
            value={template.providerPreference ?? ""}
          />
        </FormField>
        <FormField label="Model Preference">
          <input
            className="w-full rounded-2xl border border-border bg-background px-4 py-3"
            onChange={(event) =>
              onUpdate(template.id, {
                modelPreference: event.target.value || null,
              })
            }
            placeholder="claude-sonnet-4"
            value={template.modelPreference ?? ""}
          />
        </FormField>
        <FormField label="Description">
          <textarea
            className="min-h-24 w-full rounded-2xl border border-border bg-background px-4 py-3"
            onChange={(event) => onUpdate(template.id, { description: event.target.value })}
            value={template.description}
          />
        </FormField>
        <FormField label="Status">
          <select
            className="w-full rounded-2xl border border-border bg-background px-4 py-3"
            onChange={(event) =>
              onUpdate(template.id, {
                status: event.target.value as AiPromptTemplate["status"],
              })
            }
            value={template.status}
          >
            <option value="active">active</option>
            <option value="archived">archived</option>
          </select>
        </FormField>
        <FormField label="Template Content">
          <textarea
            className="min-h-48 w-full rounded-2xl border border-border bg-background px-4 py-3 font-mono text-sm"
            onChange={(event) =>
              onUpdate(template.id, { templateContent: event.target.value })
            }
            value={template.templateContent}
          />
        </FormField>
        <FormField label="Input Schema (JSON)">
          <textarea
            className="min-h-32 w-full rounded-2xl border border-border bg-background px-4 py-3 font-mono text-sm"
            onChange={(event) => onUpdate(template.id, { inputSchemaText: event.target.value })}
            value={template.inputSchemaText}
          />
        </FormField>
        <FormField label="Output Schema (JSON)">
          <textarea
            className="min-h-32 w-full rounded-2xl border border-border bg-background px-4 py-3 font-mono text-sm"
            onChange={(event) =>
              onUpdate(template.id, { outputSchemaText: event.target.value })
            }
            value={template.outputSchemaText}
          />
        </FormField>
      </div>
      <div className="mt-5 flex justify-end">
        <Button
          disabled={savingId === template.id}
          onClick={() => void onSave(template)}
          variant="secondary"
        >
          {savingId === template.id ? "Saving..." : "Save"}
        </Button>
      </div>
    </details>
  );
}

function toEditableTemplate(template: AiPromptTemplate): EditablePromptTemplate {
  return {
    ...template,
    inputSchemaText: formatJson(template.inputSchema),
    outputSchemaText: formatJson(template.outputSchema),
  };
}

function formatJson(value: Record<string, unknown>) {
  return JSON.stringify(value, null, 2);
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

function formatTimestamp(value: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }

  return parsed.toLocaleString("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
  });
}
