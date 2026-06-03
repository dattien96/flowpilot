package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type artifactRunEligibilityRow struct {
	ID            string `json:"id"`
	ProjectID     string `json:"project_id"`
	WorkflowRunID string `json:"workflow_run_id"`
}

type workflowRunEligibilityRow struct {
	ID string `json:"id"`
}

func (r *Runner) ensureArtifactSyncEligibility(ctx context.Context, artifact ArtifactDetail) error {
	if strings.TrimSpace(artifact.SourceKind) == artifactSourcePromptExecution {
		return nil
	}

	config, err := readSupabaseArtifactConfig()
	if err != nil {
		return err
	}

	artifactRun, err := fetchArtifactRunEligibility(ctx, config, artifact.ArtifactID)
	if err != nil {
		return err
	}

	if strings.TrimSpace(artifactRun.WorkflowRunID) == "" {
		return errors.New("artifact sync requires artifact_runs.workflow_run_id")
	}
	if artifact.WorkflowRunID != "" && artifactRun.WorkflowRunID != artifact.WorkflowRunID {
		return fmt.Errorf(
			"artifact sync rejected: artifact_runs.workflow_run_id=%s does not match local workflow_run_id=%s",
			artifactRun.WorkflowRunID,
			artifact.WorkflowRunID,
		)
	}
	if artifact.ProjectID != "" && strings.TrimSpace(artifactRun.ProjectID) != "" && artifactRun.ProjectID != artifact.ProjectID {
		return fmt.Errorf(
			"artifact sync rejected: artifact_runs.project_id=%s does not match local project_id=%s",
			artifactRun.ProjectID,
			artifact.ProjectID,
		)
	}

	exists, err := fetchWorkflowRunEligibility(
		ctx,
		config,
		artifactRun.WorkflowRunID,
		firstNonEmptyString(artifactRun.ProjectID, artifact.ProjectID),
	)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf(
			"artifact sync rejected: workflow_runs row %s no longer exists",
			artifactRun.WorkflowRunID,
		)
	}

	return nil
}

func fetchArtifactRunEligibility(
	ctx context.Context,
	config supabaseArtifactConfig,
	artifactRunID string,
) (artifactRunEligibilityRow, error) {
	query := url.Values{}
	query.Set("select", "id,project_id,workflow_run_id")
	query.Set("id", "eq."+strings.TrimSpace(artifactRunID))
	query.Set("limit", "1")

	statusCode, responseBody, err := httpRequestFn(ctx, http.MethodGet, config.baseURL+"/rest/v1/artifact_runs?"+query.Encode(), map[string]string{
		"Authorization": "Bearer " + config.serviceKey,
		"apikey":        config.serviceKey,
		"Accept":        "application/json",
	}, nil)
	if err != nil {
		return artifactRunEligibilityRow{}, err
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return artifactRunEligibilityRow{}, fmt.Errorf(
			"artifact_runs lookup failed: %d %s",
			statusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	var rows []artifactRunEligibilityRow
	if err := json.Unmarshal(responseBody, &rows); err != nil {
		return artifactRunEligibilityRow{}, err
	}
	if len(rows) == 0 {
		return artifactRunEligibilityRow{}, fmt.Errorf(
			"artifact sync rejected: artifact_runs row %s no longer exists",
			artifactRunID,
		)
	}

	return rows[0], nil
}

func fetchWorkflowRunEligibility(
	ctx context.Context,
	config supabaseArtifactConfig,
	workflowRunID string,
	projectID string,
) (bool, error) {
	query := url.Values{}
	query.Set("select", "id")
	query.Set("id", "eq."+strings.TrimSpace(workflowRunID))
	if strings.TrimSpace(projectID) != "" {
		query.Set("project_id", "eq."+strings.TrimSpace(projectID))
	}
	query.Set("limit", "1")

	statusCode, responseBody, err := httpRequestFn(ctx, http.MethodGet, config.baseURL+"/rest/v1/workflow_runs?"+query.Encode(), map[string]string{
		"Authorization": "Bearer " + config.serviceKey,
		"apikey":        config.serviceKey,
		"Accept":        "application/json",
	}, nil)
	if err != nil {
		return false, err
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return false, fmt.Errorf(
			"workflow_runs lookup failed: %d %s",
			statusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	var rows []workflowRunEligibilityRow
	if err := json.Unmarshal(responseBody, &rows); err != nil {
		return false, err
	}

	return len(rows) > 0, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
