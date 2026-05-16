import Link from "next/link";

import { createGatewayBundle } from "@/data/repository/factory";
import { ListFeaturesUseCase } from "@/domain/usecase/features/list-features-usecase";
import { ListProjectsUseCase } from "@/domain/usecase/projects/list-projects-usecase";
import { Badge } from "@/presentation/components/ui/badge";
import { Button } from "@/presentation/components/ui/button";

export default async function FeaturesPage() {
  const gateways = await createGatewayBundle();
  const [features, projects] = await Promise.all([
    new ListFeaturesUseCase(gateways.featureGateway).execute(),
    new ListProjectsUseCase(gateways.projectGateway).execute(),
  ]);

  return (
    <div className="space-y-6">
      <header>
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          Features
        </p>
        <h1 className="mt-3 text-4xl font-semibold tracking-tight">Feature intake backlog</h1>
      </header>
      <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
        <h2 className="text-xl font-semibold">Create feature</h2>
        <form action="/api/features" method="post" className="mt-4 grid gap-3">
          <select
            className="rounded-2xl border border-border bg-card px-4 py-3"
            name="projectId"
            required
          >
            <option value="">Select project</option>
            {projects.map((project) => (
              <option key={project.id} value={project.id}>
                {project.name}
              </option>
            ))}
          </select>
          <div className="grid gap-3 lg:grid-cols-2">
            <input
              className="rounded-2xl border border-border bg-card px-4 py-3"
              name="title"
              placeholder="Feature title"
              required
            />
            <select
              className="rounded-2xl border border-border bg-card px-4 py-3"
              defaultValue="medium"
              name="priority"
            >
              <option value="low">low</option>
              <option value="medium">medium</option>
              <option value="high">high</option>
            </select>
          </div>
          <textarea
            className="min-h-24 rounded-2xl border border-border bg-card px-4 py-3"
            name="businessGoal"
            placeholder="Business goal"
            required
          />
          <textarea
            className="min-h-24 rounded-2xl border border-border bg-card px-4 py-3"
            name="userProblem"
            placeholder="User problem"
            required
          />
          <textarea
            className="min-h-24 rounded-2xl border border-border bg-card px-4 py-3"
            name="expectedFlow"
            placeholder="Expected user flow"
            required
          />
          <textarea
            className="min-h-24 rounded-2xl border border-border bg-card px-4 py-3"
            name="acceptanceCriteria"
            placeholder="Acceptance criteria"
            required
          />
          <div className="flex justify-end">
            <Button type="submit">Create feature</Button>
          </div>
        </form>
      </section>
      <div className="space-y-3">
        {features.map((feature) => (
          <Link
            key={feature.id}
            className="flex flex-col gap-3 rounded-[1.6rem] border border-border bg-background/70 p-5 lg:flex-row lg:items-start lg:justify-between"
            href={`/features/${feature.id}`}
          >
            <div>
              <h2 className="text-xl font-semibold">{feature.title}</h2>
              <p className="mt-2 text-sm text-muted-foreground">{feature.userProblem}</p>
            </div>
            <div className="flex gap-2">
              <Badge>{feature.priority}</Badge>
              <Badge>{feature.status}</Badge>
            </div>
          </Link>
        ))}
      </div>
    </div>
  );
}
