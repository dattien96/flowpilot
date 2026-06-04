import { useEffect, useState, useRef } from "react";
import { getBrowserSupabaseClient } from "@/data/datasource/supabase/client";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type {
  WorkflowRun,
  WorkflowRunStep,
  WorkflowRunLog,
} from "@/domain/model/entity/workflow-engine";

export function useWorkflowRealtime(runId: string | undefined) {
  const [run, setRun] = useState<WorkflowRun | null>(null);
  const [steps, setSteps] = useState<WorkflowRunStep[]>([]);
  const [logs, setLogs] = useState<WorkflowRunLog[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);

  const gatewayBundle = useRef(createGatewayBundle());

  function getGatewayBundle() {
    return gatewayBundle.current;
  }

  const loadData = async () => {
    if (!runId) return;
    try {
      const data = await getGatewayBundle().workflowEngineGateway.getWorkflowRunDetail(runId);
      if (data) {
        setRun(data.run);
        setSteps(data.steps);
        setLogs(data.logs);
      }
    } catch (err: any) {
      console.error("Error loading run detail:", err);
      setError(err.message || "Failed to load execution run");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!runId) {
      setLoading(false);
      return;
    }
    loadData();

    // 1. Setup Supabase Realtime subscription if available
    let runSubscription: any = null;
    let stepSubscription: any = null;

    void (async () => {
      try {
        const supabase = await getBrowserSupabaseClient();
        runSubscription = supabase
        .channel(`run-realtime-${runId}`)
        .on(
          "postgres_changes",
          {
            event: "*",
            schema: "public",
            table: "workflow_runs",
            filter: `id=eq.${runId}`,
          },
          () => {
            loadData();
          }
        )
        .subscribe();

        stepSubscription = supabase
        .channel(`steps-realtime-${runId}`)
        .on(
          "postgres_changes",
          {
            event: "*",
            schema: "public",
            table: "workflow_run_steps",
            filter: `workflow_run_id=eq.${runId}`,
          },
          () => {
            loadData();
          }
        )
          .subscribe();
      } catch (realtimeErr) {
        console.warn("Supabase realtime not available. Falling back to polling.", realtimeErr);
      }
    })();

    // 2. High-frequency polling (2 seconds) as fallback or for logs
    const interval = setInterval(() => {
      // Only poll if running or pending
      if (!run || run.status === "PENDING" || run.status === "RUNNING") {
        loadData();
      }
    }, 2000);

    return () => {
      void getBrowserSupabaseClient().then((supabase) => {
        if (runSubscription) supabase.removeChannel(runSubscription);
        if (stepSubscription) supabase.removeChannel(stepSubscription);
      });
      clearInterval(interval);
    };
  }, [runId, run?.status]);

  const toggleYolo = async (enabled: boolean) => {
    if (!runId) return;
    try {
      const updatedRun = await getGatewayBundle().workflowEngineGateway.toggleYoloMode(
        runId,
        enabled
      );
      setRun(updatedRun);
    } catch (err: any) {
      alert(`YOLO toggle failed: ${err.message}`);
    }
  };

  const approveStep = async (stepId: string, approve: boolean, comment?: string) => {
    try {
      const updatedStep = await getGatewayBundle().workflowEngineGateway.submitStepApproval(
        stepId,
        approve,
        comment
      );
      // Update step state immediately
      setSteps((prev) => prev.map((s) => (s.id === stepId ? updatedStep : s)));
      // Refetch details to capture newly running state or logs
      await loadData();
    } catch (err: any) {
      alert(`Approval submission failed: ${err.message}`);
    }
  };

  return {
    run,
    steps,
    logs,
    loading,
    error,
    toggleYolo,
    approveStep,
    refetch: loadData,
  };
}
