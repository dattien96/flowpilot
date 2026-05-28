import { useEffect, useRef, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { Play, Zap } from "lucide-react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { UserFavorite } from "@/domain/model/entity/user-favorite";
import type { Project } from "@/domain/model/entity/project";
import { StartWorkflowRunUseCase } from "@/domain/usecase/workflow-engine/start-workflow-run-usecase";

export function DashboardFavorites() {
  const navigate = useNavigate();
  const gatewayBundle = useRef(createGatewayBundle());
  const startWorkflowRunUseCase = useRef(new StartWorkflowRunUseCase(gatewayBundle.current.workflowEngineGateway));

  const [favorites, setFavorites] = useState<UserFavorite[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [workflowNames, setWorkflowNames] = useState<Record<string, string>>({});
  const [stepNames, setStepNames] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);

  const [projectSelections, setProjectSelections] = useState<Record<string, string>>({});

  // Prompt modal state
  const [promptModalOpen, setPromptModalOpen] = useState(false);
  const [selectedFav, setSelectedFav] = useState<UserFavorite | null>(null);
  const [promptText, setPromptText] = useState("");
  const [running, setRunning] = useState(false);

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      try {
        const [favs, projs, wfs, steps] = await Promise.all([
          gatewayBundle.current.userFavoriteGateway.listFavorites(),
          gatewayBundle.current.projectGateway.listProjects(),
          gatewayBundle.current.workflowEngineGateway.listWorkflows(),
          gatewayBundle.current.workflowEngineGateway.listStepDefinitions(),
        ]);

        setFavorites(favs);
        setProjects(projs);

        const wNames: Record<string, string> = {};
        for (const w of wfs) wNames[w.id] = w.name;
        setWorkflowNames(wNames);

        const sNames: Record<string, string> = {};
        for (const s of steps) sNames[s.stepType] = s.name;
        setStepNames(sNames);

        // Initialize project selections
        const selections: Record<string, string> = {};
        for (const fav of favs) {
          if (fav.defaultProjectId) {
            selections[fav.id] = fav.defaultProjectId;
          } else if (projs.length > 0) {
            const w = wfs.find(w => w.id === fav.targetId);
            if (fav.targetType === 'workflow' && w?.projectId) {
              selections[fav.id] = w.projectId;
            } else {
              selections[fav.id] = projs[0].id;
            }
          }
        }
        setProjectSelections(selections);
      } catch (e) {
        console.error("Failed to load quick run data", e);
      } finally {
        setLoading(false);
      }
    };

    void load();
  }, []);

  const openPromptModal = (fav: UserFavorite) => {
    const projectId = projectSelections[fav.id];
    if (!projectId) {
      alert("Please select a project first");
      return;
    }

    setSelectedFav(fav);
    setPromptText("");
    setPromptModalOpen(true);
  };

  const handleRun = async () => {
    if (!selectedFav) return;
    const fav = selectedFav;
    const projectId = projectSelections[fav.id];

    if (!projectId) return;

    setRunning(true);
    try {
      // Save the default project id selection for this favorite
      await gatewayBundle.current.userFavoriteGateway.updateFavoriteProject(fav.targetType, fav.targetId, projectId).catch(console.error);

      if (fav.targetType === "workflow") {
        const run = await startWorkflowRunUseCase.current.execute({
          projectId,
          startMode: "workflow-definition",
          workflowId: fav.targetId,
          beginPrompt: promptText.trim() || "Quick run from Dashboard",
        });
        void navigate({ to: `/workflow-runs/${run.id}` });
      } else if (fav.targetType === "step") {
        const run = await startWorkflowRunUseCase.current.execute({
          projectId,
          startMode: "single-step",
          stepType: fav.targetId,
          beginPrompt: promptText.trim() || "Quick run from Dashboard",
        });
        void navigate({ to: `/workflow-runs/${run.id}` });
      }
      setPromptModalOpen(false);
    } catch (e) {
      console.error("Run failed", e);
      alert(e instanceof Error ? e.message : "Run failed");
    } finally {
      setRunning(false);
    }
  };

  const renderFavoriteCard = (fav: UserFavorite) => (
    <div key={fav.id} className="group flex flex-col p-4 rounded-xl border border-border/50 bg-background/50 hover:bg-muted/30 transition-all hover:border-primary/20">
      <div className="flex items-center justify-between gap-2 mb-4">
        <div className="flex items-center gap-2 min-w-0">
          <span className={`text-[10px] shrink-0 font-bold uppercase tracking-wider px-2 py-0.5 rounded-full border ${fav.targetType === 'workflow'
              ? 'bg-violet-500/10 text-violet-500 border-violet-500/20'
              : 'bg-cyan-500/10 text-cyan-500 border-cyan-500/20'
            }`}>
            {fav.targetType}
          </span>
          <h4 className="font-semibold text-foreground truncate">
            {fav.targetType === 'workflow' ? workflowNames[fav.targetId] || fav.targetId : stepNames[fav.targetId] || fav.targetId}
          </h4>
        </div>
      </div>

      <div className="flex items-center gap-3 mt-auto pt-2 border-t border-border/30">
        <select
          className="text-sm flex-1 min-w-0 rounded-lg border border-border/80 bg-background px-3 py-1.5 focus:outline-none focus:ring-1 focus:ring-primary/40 cursor-pointer truncate"
          value={projectSelections[fav.id] || ""}
          onChange={(e) => setProjectSelections(prev => ({ ...prev, [fav.id]: e.target.value }))}
        >
          <option value="" disabled>Select Project</option>
          {projects.map(p => (
            <option key={p.id} value={p.id}>{p.name}</option>
          ))}
        </select>

        <Button
          onClick={() => openPromptModal(fav)}
          disabled={!projectSelections[fav.id]}
          size="sm"
          className="rounded-lg shadow-sm shrink-0"
        >
          <Play className="w-3.5 h-2.5 mr-1" />
          Run
        </Button>
      </div>
    </div>
  );

  const workflowFavs = favorites.filter(f => f.targetType === 'workflow');
  const stepFavs = favorites.filter(f => f.targetType === 'step');

  return (
    <div className="rounded-[1.5rem] border border-border bg-background/60 p-6 md:col-span-3">
      <div className="flex items-center gap-2 mb-6">
        <Zap className="w-5 h-5 text-primary" />
        <h2 className="text-xl font-semibold">Quick Run Favorites</h2>
      </div>

      {loading ? (
        <div className="text-muted-foreground p-4">Loading favorites...</div>
      ) : favorites.length === 0 ? (
        <div className="text-center text-muted-foreground p-8 flex flex-col items-center justify-center space-y-4 border border-dashed border-border/50 rounded-xl">
          <div className="bg-muted/50 p-4 rounded-full">
            <Zap className="w-8 h-8 text-muted-foreground/50" />
          </div>
          <div>
            <p className="font-medium text-foreground">No favorites yet</p>
            <p className="text-sm">Star workflows or steps to access them here quickly.</p>
          </div>
        </div>
      ) : (
        <div className="space-y-8">
          {workflowFavs.length > 0 && (
            <div className="space-y-4">
              <h3 className="text-sm font-semibold uppercase tracking-widest text-muted-foreground flex items-center gap-2">
                Workflows
                <span className="bg-muted text-muted-foreground rounded-full px-2 py-0.5 text-xs">{workflowFavs.length}</span>
              </h3>
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                {workflowFavs.map(renderFavoriteCard)}
              </div>
            </div>
          )}

          {stepFavs.length > 0 && (
            <div className="space-y-4">
              <h3 className="text-sm font-semibold uppercase tracking-widest text-muted-foreground flex items-center gap-2">
                Individual Steps
                <span className="bg-muted text-muted-foreground rounded-full px-2 py-0.5 text-xs">{stepFavs.length}</span>
              </h3>
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                {stepFavs.map(renderFavoriteCard)}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Prompt Modal */}
      <Dialog open={promptModalOpen} onOpenChange={setPromptModalOpen}>
        <DialogContent className="max-w-xl bg-card border-border/60 shadow-xl overflow-hidden rounded-[1.5rem]">
          <DialogHeader>
            <DialogTitle className="text-xl flex items-center gap-2">
              <Zap className="w-5 h-5 text-primary" />
              Start Quick Run
            </DialogTitle>
            <DialogDescription>
              {selectedFav && (
                <span>
                  Running {selectedFav.targetType === 'workflow' ? 'workflow' : 'step'}:{" "}
                  <strong className="text-foreground">
                    {selectedFav.targetType === 'workflow'
                      ? workflowNames[selectedFav.targetId] || selectedFav.targetId
                      : stepNames[selectedFav.targetId] || selectedFav.targetId}
                  </strong>
                </span>
              )}
            </DialogDescription>
          </DialogHeader>

          <div className="py-4">
            <label className="block text-sm font-medium text-foreground mb-2">Prompt (Instructions / Input)</label>
            <textarea
              className="w-full h-32 rounded-xl border border-border/60 bg-background/50 p-4 text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 font-mono resize-none"
              placeholder="What do you want this flow to do? (Optional)"
              value={promptText}
              onChange={(e) => setPromptText(e.target.value)}
              disabled={running}
            />
          </div>

          <DialogFooter>
            <Button variant="ghost" onClick={() => setPromptModalOpen(false)} disabled={running}>
              Cancel
            </Button>
            <Button onClick={handleRun} disabled={running || !promptText.trim()} className="gap-2 rounded-xl min-w-[120px]">
              {running ? (
                <span className="animate-pulse">Starting...</span>
              ) : (
                <>
                  <Play className="w-4 h-4" />
                  Start Run
                </>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
