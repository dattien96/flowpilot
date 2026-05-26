"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { WorkflowRun } from "@/domain/model/entity/workflow";
import { Badge } from "@/presentation/components/ui/badge";
import { Button } from "@/presentation/components/ui/button";
import { statusTone } from "@/presentation/view-models/factories";

interface WorkflowRunsListProps {
  runs: WorkflowRun[];
  titleByRunId: Record<string, string>;
}

export function WorkflowRunsList({ runs, titleByRunId }: WorkflowRunsListProps) {
  const router = useRouter();
  const [selectedRunIds, setSelectedRunIds] = useState<string[]>([]);
  const [isDeleting, setIsDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const allSelected = runs.length > 0 && selectedRunIds.length === runs.length;
  const selectedCount = selectedRunIds.length;

  function toggleRun(runId: string) {
    setSelectedRunIds((current) =>
      current.includes(runId) ? current.filter((id) => id !== runId) : [...current, runId],
    );
  }

  function toggleAll() {
    setSelectedRunIds(allSelected ? [] : runs.map((run) => run.id));
  }

  async function deleteRuns(runIds: string[]) {
    if (runIds.length === 0 || isDeleting) {
      return;
    }

    const uniqueRunIds = [...new Set(runIds)];
    const confirmationLabel =
      uniqueRunIds.length === runs.length
        ? "all workflow runs"
        : `${uniqueRunIds.length} selected workflow run${uniqueRunIds.length === 1 ? "" : "s"}`;

    if (!window.confirm(`Delete ${confirmationLabel}? This cannot be undone.`)) {
      return;
    }

    setIsDeleting(true);
    setError(null);

    try {
      await createGatewayBundle().workflowGateway.deleteWorkflowRuns(uniqueRunIds);
      setSelectedRunIds((current) => current.filter((runId) => !uniqueRunIds.includes(runId)));
      router.refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unable to delete workflow runs.");
    } finally {
      setIsDeleting(false);
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3 rounded-[1.4rem] border border-border bg-background/70 p-4">
        <label className="flex items-center gap-3 text-sm text-muted-foreground">
          <input
            checked={allSelected}
            className="h-4 w-4"
            disabled={runs.length === 0 || isDeleting}
            onChange={toggleAll}
            type="checkbox"
          />
          <span>Select all</span>
        </label>

        <div className="ml-auto flex flex-wrap gap-2">
          <Button
            disabled={selectedCount === 0 || isDeleting}
            onClick={() => deleteRuns(selectedRunIds)}
            variant="secondary"
            className="border-destructive/25 text-destructive hover:bg-destructive/10"
          >
            {isDeleting ? "Deleting..." : `Delete selected${selectedCount > 0 ? ` (${selectedCount})` : ""}`}
          </Button>
          <Button
            disabled={runs.length === 0 || isDeleting}
            onClick={() => deleteRuns(runs.map((run) => run.id))}
            variant="secondary"
            className="border-destructive/25 text-destructive hover:bg-destructive/10"
          >
            Delete all
          </Button>
        </div>
      </div>

      {error ? <p className="text-sm text-destructive">{error}</p> : null}

      <div className="space-y-3">
        {runs.length === 0 ? (
          <p className="text-sm text-muted-foreground">No workflow runs yet.</p>
        ) : (
          runs.map((run) => {
            const selected = selectedRunIds.includes(run.id);

            return (
              <div
                key={run.id}
                className="flex items-start gap-4 rounded-[1.6rem] border border-border bg-background/70 p-5"
              >
                <label className="mt-1 flex shrink-0 items-center">
                  <input
                    checked={selected}
                    disabled={isDeleting}
                    onChange={() => toggleRun(run.id)}
                    type="checkbox"
                  />
                </label>

                <Link className="min-w-0 flex-1" href={`/workflow-runs/${run.id}`}>
                  <p className="font-semibold">{titleByRunId[run.id] ?? run.id}</p>
                  {titleByRunId[run.id] ? (
                    <p className="mt-1 break-all font-mono text-xs text-muted-foreground">
                      Run ID: {run.id}
                    </p>
                  ) : null}
                  <p className="mt-1 text-sm text-muted-foreground">
                    Started {new Date(run.startedAt).toLocaleString()}
                  </p>
                </Link>

                <div className="flex shrink-0 items-center gap-3">
                  <Badge tone={statusTone(run.status)}>{run.status}</Badge>
                  <Button
                    disabled={isDeleting}
                    onClick={() => deleteRuns([run.id])}
                    variant="secondary"
                    className="border-destructive/25 text-destructive hover:bg-destructive/10"
                  >
                    Delete
                  </Button>
                </div>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
