import { createGatewayBundle } from "@/data/repository/factory";
import { ListContextSourcesUseCase } from "@/domain/usecase/context-sources/list-context-sources-usecase";
import { ListProjectsUseCase } from "@/domain/usecase/projects/list-projects-usecase";
import { Button } from "@/presentation/components/ui/button";

export default async function ContextSourcesPage() {
  const gateways = await createGatewayBundle();
  const [contexts, projects] = await Promise.all([
    new ListContextSourcesUseCase(gateways.contextSourceGateway).execute(),
    new ListProjectsUseCase(gateways.projectGateway).execute(),
  ]);

  return (
    <div className="space-y-6">
      <header>
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          Context Sources
        </p>
        <h1 className="mt-3 text-4xl font-semibold tracking-tight">Attached workflow context</h1>
      </header>
      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <h2 className="text-xl font-semibold">Create context source</h2>
        <form action="/api/context-sources" method="post" className="mt-4 grid gap-3">
          <div className="grid gap-3 lg:grid-cols-2">
            <select
              className="rounded-2xl border border-border bg-card px-4 py-3"
              name="projectId"
              required
            >
              <option value="">Project</option>
              {projects.map((project) => (
                <option key={project.id} value={project.id}>
                  {project.name}
                </option>
              ))}
            </select>
            <select
              className="rounded-2xl border border-border bg-card px-4 py-3"
              defaultValue="manual_text"
              name="type"
            >
              <option value="manual_text">manual_text</option>
              <option value="url">url</option>
              <option value="api_note">api_note</option>
              <option value="file">file</option>
            </select>
          </div>
          <input
            className="rounded-2xl border border-border bg-card px-4 py-3"
            name="title"
            placeholder="Source title"
            required
          />
          <textarea
            className="min-h-28 rounded-2xl border border-border bg-card px-4 py-3"
            name="rawContent"
            placeholder="Paste notes, URLs, requirements, or API context"
            required
          />
          <div className="flex justify-end">
            <Button type="submit">Create source</Button>
          </div>
        </form>
      </section>
      <div className="space-y-3">
        {contexts.map((context) => (
          <div
            key={context.id}
            className="rounded-[1.6rem] border border-border bg-background/70 p-5"
          >
            <form action={`/api/context-sources/${context.id}`} method="post" className="grid gap-3">
              <div className="grid gap-3 lg:grid-cols-[1fr_180px]">
                <input
                  className="rounded-2xl border border-border bg-card px-4 py-3 font-semibold"
                  defaultValue={context.title}
                  name="title"
                  required
                />
                <select
                  className="rounded-2xl border border-border bg-card px-4 py-3"
                  defaultValue={context.type}
                  name="type"
                >
                  <option value="manual_text">manual_text</option>
                  <option value="url">url</option>
                  <option value="api_note">api_note</option>
                  <option value="file">file</option>
                </select>
              </div>
              <textarea
                className="min-h-24 rounded-2xl border border-border bg-card px-4 py-3 text-sm"
                defaultValue={context.rawContent}
                name="rawContent"
                required
              />
              <textarea
                className="min-h-20 rounded-2xl border border-border bg-card px-4 py-3 text-sm"
                defaultValue={context.summarizedContent ?? ""}
                name="summarizedContent"
                placeholder="Optional summary"
              />
              <div className="flex flex-wrap justify-end gap-2">
                <Button type="submit" variant="secondary">Save</Button>
                <Button name="_method" type="submit" value="DELETE" variant="ghost">
                  Archive
                </Button>
              </div>
            </form>
          </div>
        ))}
      </div>
    </div>
  );
}
