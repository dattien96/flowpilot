package runner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"flowpilot-runner/internal/skillpack"
)

// scaffoldAPITimeout bounds one HTTP-triggered scaffold dispatch: a 30-minute AI
// turn plus up to cap × gate-timeout of compiler verification (CP-68 recipe:
// 3 × 300s), with headroom. Long by design — the TUI client mirrors it.
const scaffoldAPITimeout = 60 * time.Minute

// ScaffoldStatusResponse is the GET .../scaffold/status body. Desktop uses
// Capable to decide whether to show the "AI Scaffold" option at all (CP-68 Q-2:
// hide it completely when unsupported).
type ScaffoldStatusResponse struct {
	ProjectID           string                    `json:"projectId"`
	Platform            string                    `json:"platform"`
	Capable             bool                      `json:"capable"`
	Recipe              *skillpack.ScaffoldRecipe `json:"recipe,omitempty"`
	MissingSkills       []string                  `json:"missingSkills,omitempty"`
	VerificationCommand string                    `json:"verificationCommand,omitempty"`
	ScaffoldStatus      *ScaffoldStatusFile       `json:"scaffoldStatus,omitempty"`
}

// ScaffoldDispatchRequestBody is the POST .../scaffold body.
type ScaffoldDispatchRequestBody struct {
	WorkingDirectory string `json:"workingDirectory"`
	Platform         string `json:"platform,omitempty"`
	ProviderKey      string `json:"providerKey,omitempty"`
	ModelName        string `json:"modelName,omitempty"`
	YoloMode         bool   `json:"yoloMode,omitempty"`
	Force            bool   `json:"force,omitempty"`
	Trigger          string `json:"trigger,omitempty"`
}

// handleScaffoldStatus serves GET /client/projects/{projectId}/scaffold/status.
// Platform comes from the query (TUI/Desktop know it) with a catalog fallback, so
// the endpoint stays usable from a bare project id.
func (s *InteractiveService) handleScaffoldStatus(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("projectId"))

	platform := strings.TrimSpace(r.URL.Query().Get("platform"))
	workspace := strings.TrimSpace(r.URL.Query().Get("workingDirectory"))
	if platform == "" || workspace == "" {
		if project, ok := s.lookupProject(projectID); ok {
			if platform == "" {
				platform = strings.TrimSpace(project.Platform)
			}
			if workspace == "" {
				workspace = strings.TrimSpace(project.Path)
			}
		}
	}

	out := ScaffoldStatusResponse{
		ProjectID: projectID,
		Platform:  platform,
		Capable:   skillpack.HasScaffoldCapability(platform),
	}
	if recipe, found, _ := skillpack.LoadScaffoldRecipe(platform); found && recipe != nil {
		out.Recipe = recipe
		out.VerificationCommand = strings.TrimSpace(recipe.VerificationGate.Command)
		if missing, ok := skillpack.VerifyRecipeSkills(recipe); !ok {
			out.MissingSkills = missing
		}
	}
	if workspace != "" {
		if statusFile, ok := LoadScaffoldStatusFile(workspace); ok {
			out.ScaffoldStatus = statusFile
		}
	}

	writeInteractiveJSON(w, http.StatusOK, out)
}

// handleDispatchScaffold serves POST /client/projects/{projectId}/scaffold.
//
// The response is 200 for every outcome the dispatcher can produce — including a
// failed compiler gate — because the status field is the contract the UI renders
// (done | skipped | error). 4xx/5xx are reserved for malformed requests and an
// unavailable runner.
func (s *InteractiveService) handleDispatchScaffold(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("projectId"))

	body := ScaffoldDispatchRequestBody{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_body", "invalid request body: "+err.Error()))
		return
	}

	platform := strings.TrimSpace(body.Platform)
	workspaceInput := strings.TrimSpace(body.WorkingDirectory)
	if platform == "" || workspaceInput == "" {
		if project, ok := s.lookupProject(projectID); ok {
			if platform == "" {
				platform = strings.TrimSpace(project.Platform)
			}
			if workspaceInput == "" {
				workspaceInput = strings.TrimSpace(project.Path)
			}
		}
	}

	dir, dirErr := s.resolveEngineWorkingDirectory(workspaceInput)
	if dirErr != nil {
		writeInteractiveError(w, dirErr)
		return
	}

	if s.runner == nil && s.scaffoldDispatcherFactory == nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadGateway, "runner_unavailable", "runner is unavailable"))
		return
	}

	result, err := s.scaffoldDispatcher().Dispatch(r.Context(), ScaffoldRequest{
		ProjectID:    projectID,
		WorkspaceDir: dir,
		Platform:     platform,
		ProviderKey:  s.resolveScaffoldProviderKey(body.ProviderKey, body.ModelName),
		ModelName:    strings.TrimSpace(body.ModelName),
		YoloMode:     body.YoloMode,
		Force:        body.Force,
		Trigger:      firstNonEmptyLineOf(strings.TrimSpace(body.Trigger), "manual"),
	})
	if err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadGateway, "scaffold_dispatch_failed", err.Error()))
		return
	}

	writeInteractiveJSON(w, http.StatusOK, result)
}

// scaffoldDispatcher returns the dispatcher to use: an injected factory wins
// (tests / future custom runtimes), otherwise the live runner backs the AI turn.
func (s *InteractiveService) scaffoldDispatcher() *ScaffoldDispatcher {
	if s.scaffoldDispatcherFactory != nil {
		return s.scaffoldDispatcherFactory()
	}
	return NewScaffoldDispatcher(s.runner)
}

// resolveScaffoldProviderKey picks the provider for a scaffold turn: the caller's
// explicit choice, else the model's provider prefix, else the registry default.
// An empty result lets ExecutePrompt report the missing-provider error itself.
func (s *InteractiveService) resolveScaffoldProviderKey(providerKey, modelName string) string {
	if key := strings.TrimSpace(providerKey); key != "" {
		return key
	}
	if model := strings.TrimSpace(modelName); model != "" {
		if key, ok := providerKeyFromModel(model); ok {
			return string(key)
		}
	}
	if s.registry != nil {
		if key, ok := s.registry.DefaultProviderKey(); ok {
			return string(key)
		}
	}
	return ""
}

// lookupProject resolves a project row by id via the catalog. Best-effort: a
// catalog that cannot list projects yields no project, never an error.
func (s *InteractiveService) lookupProject(projectID string) (Project, bool) {
	if strings.TrimSpace(projectID) == "" || s.catalog == nil {
		return Project{}, false
	}
	projects, err := s.catalog.ListProjects(context.Background())
	if err != nil {
		return Project{}, false
	}
	for _, project := range projects {
		if project.ID == projectID {
			return project, true
		}
	}
	return Project{}, false
}

// autoTriggerScaffold is the CP-68 passive Desktop trigger: after the create-project
// engine init, a scaffold-capable platform gets one background AI scaffold turn.
// It is deliberately async (project creation must not block on an AI turn plus a
// package install), and a non-capable platform is ignored as a normal skip.
func (s *InteractiveService) autoTriggerScaffold(projectID, workspaceDir, platform, modelName string) {
	if !skillpack.HasScaffoldCapability(platform) {
		log.Printf("[scaffold] project=%s platform=%q skipped (no verified recipe)", projectID, platform)
		return
	}
	if s.runner == nil && s.scaffoldDispatcherFactory == nil {
		log.Printf("[scaffold] project=%s platform=%q skipped (runner unavailable)", projectID, platform)
		return
	}

	go func() {
		// Background work must never take the whole runner process down.
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[scaffold] project=%s panicked: %v", projectID, recovered)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), scaffoldAPITimeout)
		defer cancel()

		result, err := s.scaffoldDispatcher().Dispatch(ctx, ScaffoldRequest{
			ProjectID:    projectID,
			WorkspaceDir: workspaceDir,
			Platform:     platform,
			ProviderKey:  s.resolveScaffoldProviderKey("", modelName),
			ModelName:    strings.TrimSpace(modelName),
			Trigger:      "create_project",
		})
		if err != nil {
			log.Printf("[scaffold] project=%s dispatch error: %v", projectID, err)
			return
		}
		log.Printf("[scaffold] project=%s platform=%s status=%s: %s", projectID, platform, result.Status, result.Message)
	}()
}
