package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const orphanWorkflowRunQueryBatchSize = 100

type workflowRunArtifactDirectory struct {
	projectID string
	runID     string
	path      string
}

type ArtifactOrphanCleanupResult struct {
	CheckedRunDirectories int
	RemovedRunDirectories int
}

func (r *Runner) StartOrphanedWorkflowArtifactCleanup(ctx context.Context) {
	go func() {
		result, err := r.CleanupOrphanedWorkflowArtifactDirectories(ctx)
		if err != nil {
			log.Printf("[artifact-cleanup] skipped orphan workflow artifact cleanup: %v", err)
			return
		}
		log.Printf(
			"[artifact-cleanup] checked=%d removed=%d",
			result.CheckedRunDirectories,
			result.RemovedRunDirectories,
		)
	}()
}

func (r *Runner) CleanupOrphanedWorkflowArtifactDirectories(ctx context.Context) (ArtifactOrphanCleanupResult, error) {
	candidates, err := r.listWorkflowRunArtifactDirectories()
	if err != nil {
		return ArtifactOrphanCleanupResult{}, err
	}
	result := ArtifactOrphanCleanupResult{CheckedRunDirectories: len(candidates)}
	if len(candidates) == 0 {
		return result, nil
	}

	config, err := readSupabaseArtifactConfig()
	if err != nil {
		return result, err
	}

	byProject := make(map[string][]workflowRunArtifactDirectory)
	for _, candidate := range candidates {
		byProject[candidate.projectID] = append(byProject[candidate.projectID], candidate)
	}

	artifactRoot := filepath.Clean(r.artifactRoot())
	for projectID, projectCandidates := range byProject {
		runIDs := make([]string, 0, len(projectCandidates))
		for _, candidate := range projectCandidates {
			runIDs = append(runIDs, candidate.runID)
		}

		existingRunIDs, err := fetchExistingWorkflowRunIDs(ctx, config, projectID, runIDs)
		if err != nil {
			return result, err
		}

		for _, candidate := range projectCandidates {
			if _, exists := existingRunIDs[candidate.runID]; exists {
				continue
			}
			if err := os.RemoveAll(candidate.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return result, err
			}
			result.RemovedRunDirectories++
			if err := pruneEmptyArtifactParents(artifactRoot, filepath.Dir(candidate.path)); err != nil {
				return result, err
			}
		}
	}

	return result, nil
}

func (r *Runner) listWorkflowRunArtifactDirectories() ([]workflowRunArtifactDirectory, error) {
	artifactRoot := filepath.Clean(r.artifactRoot())
	entries, err := os.ReadDir(artifactRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []workflowRunArtifactDirectory{}, nil
		}
		return nil, err
	}

	candidates := make([]workflowRunArtifactDirectory, 0)
	for _, projectEntry := range entries {
		if !projectEntry.IsDir() || projectEntry.Name() == "local" {
			continue
		}
		projectID := strings.TrimSpace(projectEntry.Name())
		if projectID == "" {
			continue
		}

		projectPath := filepath.Join(artifactRoot, projectEntry.Name())
		runEntries, err := os.ReadDir(projectPath)
		if err != nil {
			return nil, err
		}
		for _, runEntry := range runEntries {
			if !runEntry.IsDir() {
				continue
			}
			runID := strings.TrimSpace(runEntry.Name())
			if runID == "" {
				continue
			}
			candidates = append(candidates, workflowRunArtifactDirectory{
				projectID: projectID,
				runID:     runID,
				path:      filepath.Join(projectPath, runEntry.Name()),
			})
		}
	}

	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].projectID == candidates[right].projectID {
			return candidates[left].runID < candidates[right].runID
		}
		return candidates[left].projectID < candidates[right].projectID
	})

	return candidates, nil
}

func fetchExistingWorkflowRunIDs(
	ctx context.Context,
	config supabaseArtifactConfig,
	projectID string,
	runIDs []string,
) (map[string]struct{}, error) {
	existing := make(map[string]struct{})
	uniqueRunIDs := uniqueNonEmptyValues(runIDs)
	for start := 0; start < len(uniqueRunIDs); start += orphanWorkflowRunQueryBatchSize {
		end := start + orphanWorkflowRunQueryBatchSize
		if end > len(uniqueRunIDs) {
			end = len(uniqueRunIDs)
		}

		rows, err := fetchWorkflowRunIDBatch(ctx, config, projectID, uniqueRunIDs[start:end])
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			id := strings.TrimSpace(row.ID)
			if id != "" {
				existing[id] = struct{}{}
			}
		}
	}

	return existing, nil
}

type workflowRunIDRow struct {
	ID string `json:"id"`
}

func fetchWorkflowRunIDBatch(
	ctx context.Context,
	config supabaseArtifactConfig,
	projectID string,
	runIDs []string,
) ([]workflowRunIDRow, error) {
	if len(runIDs) == 0 {
		return []workflowRunIDRow{}, nil
	}

	query := url.Values{}
	query.Set("select", "id")
	query.Set("project_id", "eq."+projectID)
	query.Set("id", "in.("+strings.Join(runIDs, ",")+")")
	endpoint := config.baseURL + "/rest/v1/workflow_runs?" + query.Encode()
	statusCode, responseBody, err := httpRequestFn(ctx, http.MethodGet, endpoint, map[string]string{
		"Authorization": "Bearer " + config.serviceKey,
		"apikey":        config.serviceKey,
		"Accept":        "application/json",
	}, nil)
	if err != nil {
		return nil, err
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("workflow_runs lookup failed: %d %s", statusCode, strings.TrimSpace(string(responseBody)))
	}

	var rows []workflowRunIDRow
	if err := json.Unmarshal(responseBody, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func uniqueNonEmptyValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		unique = append(unique, trimmed)
	}
	sort.Strings(unique)
	return unique
}
