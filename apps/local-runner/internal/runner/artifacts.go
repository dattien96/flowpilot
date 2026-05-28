package runner

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	artifactSourcePromptExecution = "prompt_execution"
	artifactSyncStatusLocalOnly   = "local_only"
	artifactSyncStatusSynced      = "synced"
	artifactStorageDriverKey      = "filesystem"
)

func (r *Runner) ListArtifacts() ([]ArtifactSummary, error) {
	manifests, err := r.loadArtifactDetails()
	if err != nil {
		return nil, err
	}

	summaries := make([]ArtifactSummary, 0, len(manifests))
	for _, artifact := range manifests {
		summaries = append(summaries, artifact.ArtifactSummary)
	}

	return summaries, nil
}

func (r *Runner) GetArtifact(artifactID string) (ArtifactDetail, error) {
	artifacts, err := r.loadArtifactDetails()
	if err != nil {
		return ArtifactDetail{}, err
	}

	for _, artifact := range artifacts {
		if artifact.ArtifactID == artifactID {
			return artifact, nil
		}
	}

	return ArtifactDetail{}, os.ErrNotExist
}

func (r *Runner) SavePromptArtifact(request PromptExecutionRequest, result PromptExecutionResult) (ArtifactDetail, error) {
	artifactID := fmt.Sprintf("artifact_%s", result.RunID)
	projectID := "local"
	featureID := "prompt-execution"
	stepKey := "prompt_execution"
	baseDir := r.artifactDirectory(projectID, featureID, result.RunID, stepKey, artifactID)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return ArtifactDetail{}, err
	}

	contentPath := filepath.Join(baseDir, "content.md")
	promptPath := filepath.Join(baseDir, "prompt.md")
	stdoutPath := filepath.Join(baseDir, "stdout.txt")
	stderrPath := filepath.Join(baseDir, "stderr.txt")
	commandPath := filepath.Join(baseDir, "command.txt")
	manifestPath := filepath.Join(baseDir, "manifest.json")

	checksum := checksumFor(result.OutputMarkdown)
	if checksum == "" {
		checksum = checksumFor(result.StdoutSummary + result.StderrSummary)
	}

	artifact := ArtifactDetail{
		ArtifactSummary: ArtifactSummary{
			ArtifactID:      artifactID,
			Title:           "Prompt Execution Result",
			SourceKind:      artifactSourcePromptExecution,
			ProjectID:       projectID,
			FeatureID:       featureID,
			WorkflowRunID:   result.RunID,
			WorkflowStepKey: stepKey,
			ProviderKey:     result.ProviderKey,
			LocalPath:       baseDir,
			RemotePath:      "",
			RemoteURL:       "",
			SyncStatus:      artifactSyncStatusLocalOnly,
			CreatedAt:       result.StartedAt,
			UpdatedAt:       result.CompletedAt,
			ContentMarkdown: result.OutputMarkdown,
			PreviewMarkdown: readPreview(result.OutputMarkdown),
		},
		ManifestPath: manifestPath,
		PromptPath:   promptPath,
		StdoutPath:   stdoutPath,
		StderrPath:   stderrPath,
		CommandPath:  commandPath,
		ContentPath:  contentPath,
		Checksum:     checksum,
	}

	files := map[string]string{
		contentPath: result.OutputMarkdown,
		promptPath:  request.Prompt,
		stdoutPath:  result.StdoutSummary,
		stderrPath:  result.StderrSummary,
		commandPath: result.Command,
	}

	if err := writeArtifactFiles(files); err != nil {
		return ArtifactDetail{}, err
	}

	if err := artifact.writeManifest(manifestPath); err != nil {
		return ArtifactDetail{}, err
	}

	return artifact, nil
}

func (r *Runner) GetStorageDriver() (StorageDriverConfig, error) {
	config, err := r.loadStorageDriverConfig()
	if err != nil {
		return StorageDriverConfig{}, err
	}

	return config, nil
}

func (r *Runner) SaveStorageDriver(config StorageDriverConfig) (StorageDriverConfig, error) {
	config.DriverKey = strings.TrimSpace(config.DriverKey)
	if config.DriverKey == "" {
		config.DriverKey = artifactStorageDriverKey
	}
	config.RemoteRootPath = strings.TrimSpace(config.RemoteRootPath)
	config.RemoteFolderName = strings.TrimSpace(config.RemoteFolderName)
	if config.RemoteFolderName == "" {
		config.RemoteFolderName = "FlowPilot"
	}
	config.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)

	if err := r.persistStorageDriverConfig(config); err != nil {
		return StorageDriverConfig{}, err
	}

	return config, nil
}

func (r *Runner) ValidateStorageDriver() (StorageDriverConfig, error) {
	config, err := r.loadStorageDriverConfig()
	if err != nil {
		return StorageDriverConfig{}, err
	}

	if !config.Enabled {
		config.LastError = "storage driver is disabled"
		config.LastValidatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := r.persistStorageDriverConfig(config); err != nil {
			return StorageDriverConfig{}, err
		}
		return config, nil
	}

	targetRoot := filepath.Join(config.RemoteRootPath, config.RemoteFolderName)
	if strings.TrimSpace(config.RemoteRootPath) == "" {
		config.LastError = "remote root path is required"
		config.LastValidatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		_ = r.persistStorageDriverConfig(config)
		return config, errors.New(config.LastError)
	}

	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		config.LastError = err.Error()
		config.LastValidatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		_ = r.persistStorageDriverConfig(config)
		return config, err
	}

	markerPath := filepath.Join(targetRoot, ".flowpilot-driver-check")
	if err := os.WriteFile(markerPath, []byte(time.Now().UTC().Format(time.RFC3339Nano)), 0o644); err != nil {
		config.LastError = err.Error()
		config.LastValidatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		_ = r.persistStorageDriverConfig(config)
		return config, err
	}
	_ = os.Remove(markerPath)

	config.LastError = ""
	config.LastValidatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := r.persistStorageDriverConfig(config); err != nil {
		return StorageDriverConfig{}, err
	}

	return config, nil
}

func (r *Runner) SyncArtifact(artifactID string) (ArtifactDetail, error) {
	artifact, err := r.GetArtifact(artifactID)
	if err != nil {
		return ArtifactDetail{}, err
	}

	config, err := r.loadStorageDriverConfig()
	if err != nil {
		return ArtifactDetail{}, err
	}
	if !config.Enabled {
		return ArtifactDetail{}, errors.New("storage driver is disabled")
	}
	if strings.TrimSpace(config.RemoteRootPath) == "" {
		return ArtifactDetail{}, errors.New("remote root path is required")
	}

	remoteRoot := filepath.Join(
		config.RemoteRootPath,
		config.RemoteFolderName,
		"Projects",
		safeSegment(artifact.ProjectID),
		"Features",
		safeSegment(artifact.FeatureID),
		"Runs",
		safeSegment(artifact.WorkflowRunID),
		"Steps",
		safeSegment(artifact.WorkflowStepKey),
		safeSegment(artifact.ArtifactID),
	)

	if err := copyDir(artifact.LocalPath, remoteRoot); err != nil {
		return ArtifactDetail{}, err
	}

	artifact.RemotePath = remoteRoot
	artifact.RemoteURL = "file:///" + filepath.ToSlash(remoteRoot)
	artifact.SyncStatus = artifactSyncStatusSynced
	artifact.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	manifestPath := artifact.ManifestPath
	if err := artifact.writeManifest(manifestPath); err != nil {
		return ArtifactDetail{}, err
	}

	config.LastSyncedAt = artifact.UpdatedAt
	config.LastError = ""
	if err := r.persistStorageDriverConfig(config); err != nil {
		return ArtifactDetail{}, err
	}

	return artifact, nil
}

func (r *Runner) DeleteArtifactsByWorkflowRunIDs(runIDs []string) error {
	targets := make(map[string]struct{}, len(runIDs))
	for _, runID := range runIDs {
		trimmed := strings.TrimSpace(runID)
		if trimmed == "" {
			continue
		}
		targets[trimmed] = struct{}{}
	}
	if len(targets) == 0 {
		return nil
	}

	artifactRoot := filepath.Clean(r.artifactRoot())
	info, err := os.Stat(artifactRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}

	removedPaths := make(map[string]struct{})
	walkErr := filepath.WalkDir(artifactRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() || path == artifactRoot {
			return nil
		}
		if _, ok := targets[strings.TrimSpace(entry.Name())]; !ok {
			return nil
		}
		if _, alreadyRemoved := removedPaths[path]; alreadyRemoved {
			return filepath.SkipDir
		}

		if err := os.RemoveAll(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		removedPaths[path] = struct{}{}

		if err := pruneEmptyArtifactParents(artifactRoot, filepath.Dir(path)); err != nil {
			return err
		}

		return filepath.SkipDir
	})
	if walkErr != nil {
		return walkErr
	}

	return nil
}

func pruneEmptyArtifactParents(rootPath, startPath string) error {
	root := filepath.Clean(rootPath)
	current := filepath.Clean(startPath)

	for current != root && current != "." {
		entries, err := os.ReadDir(current)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				current = filepath.Dir(current)
				continue
			}
			return err
		}
		if len(entries) > 0 {
			return nil
		}
		if err := os.Remove(current); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				current = filepath.Dir(current)
				continue
			}
			return err
		}

		next := filepath.Dir(current)
		if next == current {
			return nil
		}
		current = next
	}

	return nil
}

func (r *Runner) CreateBackup(request BackupRequest) (BackupResult, error) {
	artifacts, err := r.loadArtifactDetails()
	if err != nil {
		return BackupResult{}, err
	}

	filtered := filterArtifactsForBackup(artifacts, request)
	if len(filtered) == 0 {
		return BackupResult{}, errors.New("no artifacts available for backup")
	}

	backupDir := filepath.Join(r.workspace, ".flowpilot", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return BackupResult{}, err
	}

	archiveName := fmt.Sprintf("flowpilot_backup_%s.zip", time.Now().UTC().Format("20060102_150405"))
	backupPath := filepath.Join(backupDir, archiveName)
	file, err := os.Create(backupPath)
	if err != nil {
		return BackupResult{}, err
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	readme := fmt.Sprintf(
		"FlowPilot backup\n\nScope: %s\nArtifacts: %d\nCreatedAt: %s\n",
		scopeOrAll(request.Scope),
		len(filtered),
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err := writeZipFile(zipWriter, "README.md", []byte(readme)); err != nil {
		return BackupResult{}, err
	}

	metadataBytes, _ := json.MarshalIndent(filtered, "", "  ")
	if err := writeZipFile(zipWriter, "metadata.json", metadataBytes); err != nil {
		return BackupResult{}, err
	}

	for _, artifact := range filtered {
		err := filepath.WalkDir(artifact.LocalPath, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}

			relative, err := filepath.Rel(r.artifactRoot(), path)
			if err != nil {
				return err
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			return writeZipFile(zipWriter, filepath.ToSlash(relative), raw)
		})
		if err != nil {
			return BackupResult{}, err
		}
	}

	return BackupResult{
		BackupPath:  backupPath,
		ArchiveName: archiveName,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func (r *Runner) loadArtifactDetails() ([]ArtifactDetail, error) {
	artifactRoot := r.artifactRoot()
	info, err := os.Stat(artifactRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []ArtifactDetail{}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return []ArtifactDetail{}, nil
	}

	details := make([]ArtifactDetail, 0)
	walkErr := filepath.WalkDir(artifactRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "manifest.json" {
			return nil
		}

		detail, err := r.readArtifactDetail(path)
		if err != nil {
			return err
		}
		details = append(details, detail)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	sort.Slice(details, func(left, right int) bool {
		return details[left].UpdatedAt > details[right].UpdatedAt
	})

	return details, nil
}

func (r *Runner) readArtifactDetail(manifestPath string) (ArtifactDetail, error) {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return ArtifactDetail{}, err
	}

	var detail ArtifactDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		return ArtifactDetail{}, err
	}

	detail.ManifestPath = manifestPath
	if detail.ContentPath == "" {
		detail.ContentPath = filepath.Join(filepath.Dir(manifestPath), "content.md")
	}
	if detail.PromptPath == "" {
		detail.PromptPath = filepath.Join(filepath.Dir(manifestPath), "prompt.md")
	}
	if detail.ActualPromptPath == "" {
		detail.ActualPromptPath = filepath.Join(filepath.Dir(manifestPath), "actual-prompt.md")
	}
	if detail.StdoutPath == "" {
		detail.StdoutPath = filepath.Join(filepath.Dir(manifestPath), "stdout.txt")
	}
	if detail.StderrPath == "" {
		detail.StderrPath = filepath.Join(filepath.Dir(manifestPath), "stderr.txt")
	}
	if detail.CommandPath == "" {
		detail.CommandPath = filepath.Join(filepath.Dir(manifestPath), "command.txt")
	}
	if detail.LocalPath == "" {
		detail.LocalPath = filepath.Dir(manifestPath)
	}

	contentBytes, err := os.ReadFile(detail.ContentPath)
	if err == nil {
		detail.ContentMarkdown = strings.TrimSpace(string(contentBytes))
		detail.PreviewMarkdown = readPreview(detail.ContentMarkdown)
	}
	if promptBytes, err := os.ReadFile(detail.PromptPath); err == nil {
		detail.PromptText = strings.TrimSpace(string(promptBytes))
	}
	if promptBytes, err := os.ReadFile(detail.ActualPromptPath); err == nil {
		detail.ActualPromptText = strings.TrimSpace(string(promptBytes))
	} else {
		detail.ActualPromptText = detail.PromptText
	}
	if stdoutBytes, err := os.ReadFile(detail.StdoutPath); err == nil {
		detail.StdoutText = strings.TrimSpace(string(stdoutBytes))
	}
	if stderrBytes, err := os.ReadFile(detail.StderrPath); err == nil {
		detail.StderrText = strings.TrimSpace(string(stderrBytes))
	}
	if commandBytes, err := os.ReadFile(detail.CommandPath); err == nil {
		detail.CommandText = strings.TrimSpace(string(commandBytes))
	}

	if detail.SyncStatus == "" {
		detail.SyncStatus = artifactSyncStatusLocalOnly
	}
	if detail.Title == "" {
		detail.Title = "Artifact"
	}
	if detail.CreatedAt == "" {
		detail.CreatedAt = detail.UpdatedAt
	}

	return detail, nil
}

func (r *Runner) artifactRoot() string {
	return filepath.Join(r.workspace, ".flowpilot", "artifacts")
}

func (r *Runner) artifactDirectory(projectID, featureID, runID, stepKey, artifactID string) string {
	return filepath.Join(
		r.artifactRoot(),
		safeSegment(projectID),
		safeSegment(featureID),
		safeSegment(runID),
		safeSegment(stepKey),
		safeSegment(artifactID),
	)
}

func (detail ArtifactDetail) writeManifest(manifestPath string) error {
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(detail, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(manifestPath, raw, 0o644)
}

func (r *Runner) loadStorageDriverConfig() (StorageDriverConfig, error) {
	path := r.storageDriverConfigPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return StorageDriverConfig{
				DriverKey:        artifactStorageDriverKey,
				Enabled:          false,
				RemoteFolderName: "FlowPilot",
			}, nil
		}
		return StorageDriverConfig{}, err
	}

	var config StorageDriverConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return StorageDriverConfig{}, err
	}

	if config.DriverKey == "" {
		config.DriverKey = artifactStorageDriverKey
	}
	if config.RemoteFolderName == "" {
		config.RemoteFolderName = "FlowPilot"
	}

	return config, nil
}

func (r *Runner) persistStorageDriverConfig(config StorageDriverConfig) error {
	path := r.storageDriverConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, raw, 0o644)
}

func (r *Runner) storageDriverConfigPath() string {
	return filepath.Join(r.workspace, ".flowpilot", "settings", "storage-driver.json")
}

func filterArtifactsForBackup(artifacts []ArtifactDetail, request BackupRequest) []ArtifactDetail {
	scope := strings.TrimSpace(request.Scope)
	if scope == "" || scope == "all" {
		return artifacts
	}

	filtered := make([]ArtifactDetail, 0, len(artifacts))
	for _, artifact := range artifacts {
		if scope == "run" && request.RunID != "" && artifact.WorkflowRunID == request.RunID {
			filtered = append(filtered, artifact)
		}
	}

	return filtered
}

func writeArtifactFiles(files map[string]string) error {
	for path, contents := range files {
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func readPreview(contents string) string {
	trimmed := strings.TrimSpace(contents)
	if len(trimmed) <= 240 {
		return trimmed
	}

	return trimmed[:240] + "\n...[truncated]"
}

func safeSegment(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unassigned"
	}

	replaced := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_").Replace(trimmed)
	return replaced
}

func checksumFor(contents string) string {
	sum := sha256.Sum256([]byte(contents))
	return hex.EncodeToString(sum[:])
}

func copyDir(sourceDir, targetDir string) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}

	return filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetDir, relative)

		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, raw, 0o644)
	})
}

func writeZipFile(zipWriter *zip.Writer, name string, data []byte) error {
	fileWriter, err := zipWriter.Create(name)
	if err != nil {
		return err
	}

	_, err = fileWriter.Write(data)
	return err
}

func scopeOrAll(scope string) string {
	if strings.TrimSpace(scope) == "" {
		return "all"
	}

	return scope
}
