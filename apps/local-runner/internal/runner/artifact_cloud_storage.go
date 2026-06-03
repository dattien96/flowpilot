package runner

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	artifactStorageProviderSupabase    = "supabase"
	artifactStorageProviderGoogleDrive = "google_drive"
	artifactSupabaseBucketName         = "flowpilot-artifacts"
)

type artifactSyncFile struct {
	relativePath string
	bytes        []byte
}

type supabaseObjectInfoResponse struct {
	ID       string         `json:"id"`
	Metadata map[string]any `json:"metadata"`
}

type supabaseUploadResponse struct {
	ID  string `json:"Id"`
	Key string `json:"Key"`
}

type supabaseSignedURLResponse struct {
	SignedURL string `json:"signedURL"`
}

type supabaseStorageErrorResponse struct {
	StatusCode string `json:"statusCode"`
	Error      string `json:"error"`
	Message    string `json:"message"`
}

type supabaseArtifactConfig struct {
	baseURL    string
	serviceKey string
}

type googleDriveTokenResponse struct {
	AccessToken string `json:"access_token"`
}

type googleDriveFile struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	MimeType      string            `json:"mimeType"`
	WebViewLink   string            `json:"webViewLink"`
	AppProperties map[string]string `json:"appProperties"`
}

type googleDriveListResponse struct {
	Files []googleDriveFile `json:"files"`
}

const googleDriveFolderMimeType = "application/vnd.google-apps.folder"

func (r *Runner) syncArtifactToSupabase(artifact ArtifactDetail) (ArtifactDetail, error) {
	files, err := loadArtifactSyncFiles(artifact)
	if err != nil {
		_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
			StorageProvider: artifactStorageProviderSupabase,
			RemotePath:      buildArtifactCanonicalPath(artifact),
			RemoteObjectID:  artifact.RemoteObjectID,
			SyncStatus:      artifactSyncStatusFailed,
			ErrorMessage:    err.Error(),
		})
		return ArtifactDetail{}, err
	}

	sourceCreatedAt := strings.TrimSpace(artifact.CreatedAt)
	if sourceCreatedAt == "" {
		sourceCreatedAt = strings.TrimSpace(artifact.UpdatedAt)
	}

	snapshotMetadata := map[string]string{
		"artifactId":      artifact.ArtifactID,
		"projectId":       artifact.ProjectID,
		"workflowRunId":   artifact.WorkflowRunID,
		"workflowStepKey": artifact.WorkflowStepKey,
		"sourceCreatedAt": sourceCreatedAt,
		"syncRole":        "snapshot",
	}

	for _, file := range files {
		if _, err := uploadSupabaseObject(
			buildArtifactSnapshotPath(artifact, file.relativePath),
			file.bytes,
			contentTypeForArtifactPath(file.relativePath),
			withMetadata(snapshotMetadata, "relativePath", file.relativePath),
		); err != nil {
			_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
				StorageProvider: artifactStorageProviderSupabase,
				RemotePath:      buildArtifactCanonicalPath(artifact),
				RemoteObjectID:  artifact.RemoteObjectID,
				SyncStatus:      artifactSyncStatusFailed,
				ErrorMessage:    err.Error(),
			})
			return ArtifactDetail{}, err
		}
	}

	canonicalFile, err := pickArtifactCanonicalFile(artifact, files)
	if err != nil {
		_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
			StorageProvider: artifactStorageProviderSupabase,
			RemotePath:      buildArtifactCanonicalPath(artifact),
			RemoteObjectID:  artifact.RemoteObjectID,
			SyncStatus:      artifactSyncStatusFailed,
			ErrorMessage:    err.Error(),
		})
		return ArtifactDetail{}, err
	}

	canonicalPath := buildArtifactCanonicalPath(artifact)
	existingObject, err := fetchSupabaseObjectInfo(canonicalPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
			StorageProvider: artifactStorageProviderSupabase,
			RemotePath:      canonicalPath,
			RemoteObjectID:  artifact.RemoteObjectID,
			SyncStatus:      artifactSyncStatusFailed,
			ErrorMessage:    err.Error(),
		})
		return ArtifactDetail{}, err
	}

	requestCreatedAt := parseArtifactTimestamp(sourceCreatedAt)
	existingCreatedAt := readSupabaseSourceCreatedAt(existingObject.Metadata)
	remoteObjectID := strings.TrimSpace(existingObject.ID)
	if existingCreatedAt <= requestCreatedAt {
		uploadedObject, uploadErr := uploadSupabaseObject(
			canonicalPath,
			canonicalFile.bytes,
			contentTypeForArtifactPath(canonicalFile.relativePath),
			withMetadata(snapshotMetadata, "relativePath", canonicalFile.relativePath, "syncRole", "canonical"),
		)
		if uploadErr != nil {
			_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
				StorageProvider: artifactStorageProviderSupabase,
				RemotePath:      canonicalPath,
				RemoteObjectID:  remoteObjectID,
				SyncStatus:      artifactSyncStatusFailed,
				ErrorMessage:    uploadErr.Error(),
			})
			return ArtifactDetail{}, uploadErr
		}
		remoteObjectID = strings.TrimSpace(uploadedObject.ID)
	}

	if remoteObjectID == "" {
		latestObject, infoErr := fetchSupabaseObjectInfo(canonicalPath)
		if infoErr != nil {
			_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
				StorageProvider: artifactStorageProviderSupabase,
				RemotePath:      canonicalPath,
				RemoteObjectID:  remoteObjectID,
				SyncStatus:      artifactSyncStatusFailed,
				ErrorMessage:    infoErr.Error(),
			})
			return ArtifactDetail{}, infoErr
		}
		remoteObjectID = strings.TrimSpace(latestObject.ID)
	}

	return r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
		StorageProvider: artifactStorageProviderSupabase,
		RemotePath:      canonicalPath,
		RemoteObjectID:  remoteObjectID,
		SyncStatus:      artifactSyncStatusSynced,
	})
}

func (r *Runner) ResolveArtifactOpenURL(artifactID string, fileKind string) (string, error) {
	artifact, err := r.GetArtifact(artifactID)
	if err != nil {
		return "", err
	}

	storageProvider := strings.TrimSpace(strings.ToLower(artifact.StorageProvider))
	targetPath, err := resolveArtifactRemoteFilePath(artifact, fileKind)
	if err != nil {
		return "", err
	}
	switch {
	case storageProvider == artifactStorageProviderSupabase && targetPath != "":
		return createSupabaseSignedURL(targetPath)
	case storageProvider == artifactStorageProviderGoogleDrive && targetPath != "":
		return r.resolveGoogleDriveArtifactOpenURL(artifact, targetPath, fileKind)
	case strings.TrimSpace(artifact.RemoteURL) != "":
		return strings.TrimSpace(artifact.RemoteURL), nil
	default:
		return "", os.ErrNotExist
	}
}

func resolveArtifactRemoteFilePath(artifact ArtifactDetail, fileKind string) (string, error) {
	remotePath := strings.TrimSpace(artifact.RemotePath)
	if remotePath == "" {
		return "", os.ErrNotExist
	}

	switch strings.TrimSpace(strings.ToLower(fileKind)) {
	case "", "content", "response":
		return remotePath, nil
	case "actual-prompt":
		return buildArtifactSnapshotPath(artifact, "actual-prompt.md"), nil
	case "prompt":
		return buildArtifactSnapshotPath(artifact, "prompt.md"), nil
	default:
		return "", fmt.Errorf("unsupported artifact file %q", fileKind)
	}
}

func (r *Runner) resolveGoogleDriveArtifactOpenURL(
	artifact ArtifactDetail,
	targetPath string,
	fileKind string,
) (string, error) {
	projectID := strings.TrimSpace(artifact.ProjectID)
	connectionStatus, err := r.GetGoogleDriveArtifactConnectionStatus(projectID, "")
	if err != nil {
		return "", err
	}

	rootFolderID := strings.TrimSpace(connectionStatus.Connection.FolderID)
	if rootFolderID == "" {
		return "", errors.New("google drive artifact storage is not connected for this project")
	}

	creds, err := r.loadGoogleDriveCredentialByProject(projectID)
	if err != nil {
		return "", err
	}

	accessToken, err := refreshGoogleDriveAccessToken(creds.RefreshToken)
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(strings.ToLower(fileKind)) == "" ||
		strings.TrimSpace(strings.ToLower(fileKind)) == "content" ||
		strings.TrimSpace(strings.ToLower(fileKind)) == "response" {
		if remoteObjectID := strings.TrimSpace(artifact.RemoteObjectID); remoteObjectID != "" {
			return "https://drive.google.com/file/d/" + url.PathEscape(remoteObjectID) + "/view", nil
		}
	}

	file, err := findGoogleDriveFileByLogicalPath(accessToken, rootFolderID, targetPath)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(file.ID) == "" {
		return "", os.ErrNotExist
	}
	return "https://drive.google.com/file/d/" + url.PathEscape(strings.TrimSpace(file.ID)) + "/view", nil
}

func findGoogleDriveFileByLogicalPath(accessToken, rootFolderID, logicalPath string) (googleDriveFile, error) {
	segments := strings.Split(strings.Trim(strings.ReplaceAll(logicalPath, "\\", "/"), "/"), "/")
	if len(segments) == 0 {
		return googleDriveFile{}, os.ErrNotExist
	}

	parentID := strings.TrimSpace(rootFolderID)
	for _, segment := range segments[:len(segments)-1] {
		folder, err := findGoogleDriveFile(accessToken, parentID, segment)
		if err != nil {
			return googleDriveFile{}, err
		}
		if strings.TrimSpace(folder.ID) == "" {
			return googleDriveFile{}, os.ErrNotExist
		}
		parentID = strings.TrimSpace(folder.ID)
	}

	file, err := findGoogleDriveFile(accessToken, parentID, segments[len(segments)-1])
	if err != nil {
		return googleDriveFile{}, err
	}
	if strings.TrimSpace(file.ID) == "" {
		return googleDriveFile{}, os.ErrNotExist
	}
	return file, nil
}

func (r *Runner) syncArtifactToGoogleDrive(
	artifact ArtifactDetail,
	request ArtifactSyncRequest,
) (ArtifactDetail, error) {
	canonicalPath := buildArtifactCanonicalPath(artifact)
	files, err := loadArtifactSyncFiles(artifact)
	if err != nil {
		_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
			StorageProvider: artifactStorageProviderGoogleDrive,
			RemotePath:      canonicalPath,
			RemoteObjectID:  artifact.RemoteObjectID,
			SyncStatus:      artifactSyncStatusFailed,
			ErrorMessage:    err.Error(),
		})
		return ArtifactDetail{}, err
	}

	folderID := strings.TrimSpace(request.GoogleDriveFolderID)
	if folderID == "" {
		err := errors.New("google drive folder id is required")
		_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
			StorageProvider: artifactStorageProviderGoogleDrive,
			RemotePath:      canonicalPath,
			RemoteObjectID:  artifact.RemoteObjectID,
			SyncStatus:      artifactSyncStatusFailed,
			ErrorMessage:    err.Error(),
		})
		return ArtifactDetail{}, err
	}

	projectID := strings.TrimSpace(request.GoogleDriveProjectID)
	if projectID == "" {
		projectID = strings.TrimSpace(artifact.ProjectID)
	}
	creds, err := r.loadGoogleDriveCredentialByProject(projectID)
	if err != nil && strings.TrimSpace(request.GoogleDriveIntegrationID) != "" {
		creds, err = r.loadGoogleDriveCredential(request.GoogleDriveIntegrationID)
	}
	if err != nil {
		_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
			StorageProvider: artifactStorageProviderGoogleDrive,
			RemotePath:      canonicalPath,
			RemoteObjectID:  artifact.RemoteObjectID,
			SyncStatus:      artifactSyncStatusFailed,
			ErrorMessage:    err.Error(),
		})
		return ArtifactDetail{}, err
	}

	accessToken, err := refreshGoogleDriveAccessToken(creds.RefreshToken)
	if err != nil {
		_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
			StorageProvider: artifactStorageProviderGoogleDrive,
			RemotePath:      canonicalPath,
			RemoteObjectID:  artifact.RemoteObjectID,
			SyncStatus:      artifactSyncStatusFailed,
			ErrorMessage:    err.Error(),
		})
		return ArtifactDetail{}, err
	}

	sourceCreatedAt := strings.TrimSpace(artifact.CreatedAt)
	if sourceCreatedAt == "" {
		sourceCreatedAt = strings.TrimSpace(artifact.UpdatedAt)
	}

	snapshotMetadata := map[string]string{
		"artifactId":      artifact.ArtifactID,
		"projectId":       artifact.ProjectID,
		"workflowRunId":   artifact.WorkflowRunID,
		"workflowStepKey": artifact.WorkflowStepKey,
		"sourceCreatedAt": sourceCreatedAt,
		"syncRole":        "snapshot",
	}

	stepFolderID, err := ensureGoogleDriveFolderPath(
		accessToken,
		folderID,
		[]string{
			"projects",
			cloudPathSegment(artifact.ProjectID),
			"runs",
			cloudPathSegment(artifact.WorkflowRunID),
			"steps",
			cloudPathSegment(artifact.WorkflowStepKey),
		},
	)
	if err != nil {
		_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
			StorageProvider: artifactStorageProviderGoogleDrive,
			RemotePath:      canonicalPath,
			RemoteObjectID:  artifact.RemoteObjectID,
			SyncStatus:      artifactSyncStatusFailed,
			ErrorMessage:    err.Error(),
		})
		return ArtifactDetail{}, err
	}

	snapshotRootID, err := ensureGoogleDriveFolderPath(
		accessToken,
		stepFolderID,
		[]string{".snapshots", cloudPathSegment(artifact.ArtifactID)},
	)
	if err != nil {
		_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
			StorageProvider: artifactStorageProviderGoogleDrive,
			RemotePath:      canonicalPath,
			RemoteObjectID:  artifact.RemoteObjectID,
			SyncStatus:      artifactSyncStatusFailed,
			ErrorMessage:    err.Error(),
		})
		return ArtifactDetail{}, err
	}

	for _, file := range files {
		parentID, ensureErr := ensureGoogleDriveFolderPath(
			accessToken,
			snapshotRootID,
			googleDriveFolderSegments(file.relativePath),
		)
		if ensureErr != nil {
			_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
				StorageProvider: artifactStorageProviderGoogleDrive,
				RemotePath:      canonicalPath,
				RemoteObjectID:  artifact.RemoteObjectID,
				SyncStatus:      artifactSyncStatusFailed,
				ErrorMessage:    ensureErr.Error(),
			})
			return ArtifactDetail{}, ensureErr
		}

		if _, uploadErr := upsertGoogleDriveFile(
			accessToken,
			parentID,
			filepath.Base(file.relativePath),
			file.bytes,
			contentTypeForArtifactPath(file.relativePath),
			withMetadata(snapshotMetadata, "relativePath", file.relativePath),
		); uploadErr != nil {
			_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
				StorageProvider: artifactStorageProviderGoogleDrive,
				RemotePath:      canonicalPath,
				RemoteObjectID:  artifact.RemoteObjectID,
				SyncStatus:      artifactSyncStatusFailed,
				ErrorMessage:    uploadErr.Error(),
			})
			return ArtifactDetail{}, uploadErr
		}
	}

	canonicalFile, err := pickArtifactCanonicalFile(artifact, files)
	if err != nil {
		_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
			StorageProvider: artifactStorageProviderGoogleDrive,
			RemotePath:      canonicalPath,
			RemoteObjectID:  artifact.RemoteObjectID,
			SyncStatus:      artifactSyncStatusFailed,
			ErrorMessage:    err.Error(),
		})
		return ArtifactDetail{}, err
	}

	existingCanonical, err := findGoogleDriveFile(accessToken, stepFolderID, filepath.Base(canonicalPath))
	if err != nil {
		_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
			StorageProvider: artifactStorageProviderGoogleDrive,
			RemotePath:      canonicalPath,
			RemoteObjectID:  artifact.RemoteObjectID,
			SyncStatus:      artifactSyncStatusFailed,
			ErrorMessage:    err.Error(),
		})
		return ArtifactDetail{}, err
	}

	requestCreatedAt := parseArtifactTimestamp(sourceCreatedAt)
	existingCreatedAt := parseArtifactTimestamp(existingCanonical.AppProperties["sourceCreatedAt"])
	remoteObjectID := strings.TrimSpace(existingCanonical.ID)
	if existingCreatedAt <= requestCreatedAt {
		uploadedCanonical, uploadErr := upsertGoogleDriveFile(
			accessToken,
			stepFolderID,
			filepath.Base(canonicalPath),
			canonicalFile.bytes,
			contentTypeForArtifactPath(canonicalFile.relativePath),
			withMetadata(snapshotMetadata, "relativePath", canonicalFile.relativePath, "syncRole", "canonical", "remotePath", canonicalPath),
		)
		if uploadErr != nil {
			_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
				StorageProvider: artifactStorageProviderGoogleDrive,
				RemotePath:      canonicalPath,
				RemoteObjectID:  remoteObjectID,
				SyncStatus:      artifactSyncStatusFailed,
				ErrorMessage:    uploadErr.Error(),
			})
			return ArtifactDetail{}, uploadErr
		}
		remoteObjectID = strings.TrimSpace(uploadedCanonical.ID)
	}

	if remoteObjectID == "" {
		err := errors.New("google drive canonical file id was empty after sync")
		_, _ = r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
			StorageProvider: artifactStorageProviderGoogleDrive,
			RemotePath:      canonicalPath,
			RemoteObjectID:  remoteObjectID,
			SyncStatus:      artifactSyncStatusFailed,
			ErrorMessage:    err.Error(),
		})
		return ArtifactDetail{}, err
	}

	return r.SaveArtifactCloudSyncResult(artifact.ArtifactID, ArtifactCloudSyncResult{
		StorageProvider: artifactStorageProviderGoogleDrive,
		RemotePath:      canonicalPath,
		RemoteObjectID:  remoteObjectID,
		SyncStatus:      artifactSyncStatusSynced,
	})
}

func loadArtifactSyncFiles(artifact ArtifactDetail) ([]artifactSyncFile, error) {
	rootPath := filepath.Clean(artifact.LocalPath)
	info, err := os.Stat(rootPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("artifact path is not a directory: %s", rootPath)
	}

	entries, err := collectArtifactDiskFiles(rootPath)
	if err != nil {
		return nil, err
	}

	files := make([]artifactSyncFile, 0, len(entries))
	for _, entry := range entries {
		bytes, readErr := os.ReadFile(entry.absolutePath)
		if readErr != nil {
			return nil, readErr
		}
		files = append(files, artifactSyncFile{
			relativePath: entry.relativePath,
			bytes:        bytes,
		})
	}

	return files, nil
}

func buildArtifactCanonicalPath(artifact ArtifactDetail) string {
	return fmt.Sprintf(
		"projects/%s/runs/%s/steps/%s/artifacts/%s/%s",
		cloudPathSegment(artifact.ProjectID),
		cloudPathSegment(artifact.WorkflowRunID),
		cloudPathSegment(artifact.WorkflowStepKey),
		cloudPathSegment(artifact.ArtifactID),
		filepath.Base(deriveArtifactOutputFilename(artifact)),
	)
}

func buildArtifactSnapshotPath(artifact ArtifactDetail, relativePath string) string {
	return fmt.Sprintf(
		"projects/%s/runs/%s/steps/%s/.snapshots/%s/%s",
		cloudPathSegment(artifact.ProjectID),
		cloudPathSegment(artifact.WorkflowRunID),
		cloudPathSegment(artifact.WorkflowStepKey),
		cloudPathSegment(artifact.ArtifactID),
		strings.TrimLeft(relativePath, "/"),
	)
}

func deriveArtifactOutputFilename(artifact ArtifactDetail) string {
	candidate := strings.TrimSpace(artifact.ContentPath)
	if candidate == "" {
		candidate = strings.TrimSpace(artifact.Title)
	}
	if candidate == "" {
		candidate = artifact.ArtifactID + ".md"
	}
	base := filepath.Base(candidate)
	if filepath.Ext(base) == "" {
		base += ".md"
	}
	return base
}

func pickArtifactCanonicalFile(artifact ArtifactDetail, files []artifactSyncFile) (artifactSyncFile, error) {
	outputFilename := deriveArtifactOutputFilename(artifact)
	for _, file := range files {
		if file.relativePath == outputFilename {
			return file, nil
		}
	}
	for _, file := range files {
		if filepath.Base(file.relativePath) == outputFilename {
			return file, nil
		}
	}
	return artifactSyncFile{}, fmt.Errorf("output file %q was not found in the artifact directory", outputFilename)
}

func cloudPathSegment(value string) string {
	return safeSegment(value)
}

func contentTypeForArtifactPath(relativePath string) string {
	switch strings.ToLower(filepath.Ext(relativePath)) {
	case ".json":
		return "application/json; charset=utf-8"
	case ".md", ".markdown", ".txt":
		return "text/plain; charset=utf-8"
	case ".html":
		return "text/html; charset=utf-8"
	case ".csv":
		return "text/csv; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}

func withMetadata(base map[string]string, kv ...string) map[string]string {
	metadata := make(map[string]string, len(base)+len(kv)/2)
	for key, value := range base {
		metadata[key] = value
	}
	for index := 0; index+1 < len(kv); index += 2 {
		metadata[kv[index]] = kv[index+1]
	}
	return metadata
}

func parseArtifactTimestamp(value string) int64 {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	parsed, err := parseTimeMillis(value)
	if err != nil {
		return 0
	}
	return parsed
}

func parseTimeMillis(value string) (int64, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return 0, err
	}
	return parsed.UnixMilli(), nil
}

func readSupabaseSourceCreatedAt(metadata map[string]any) int64 {
	if metadata == nil {
		return 0
	}
	raw, ok := metadata["sourceCreatedAt"]
	if !ok {
		return 0
	}
	value, ok := raw.(string)
	if !ok {
		return 0
	}
	parsed, err := parseTimeMillis(value)
	if err != nil {
		return 0
	}
	return parsed
}

func uploadSupabaseObject(
	objectPath string,
	body []byte,
	contentType string,
	metadata map[string]string,
) (supabaseUploadResponse, error) {
	config, err := readSupabaseArtifactConfig()
	if err != nil {
		return supabaseUploadResponse{}, err
	}
	metadataRaw, err := json.Marshal(metadata)
	if err != nil {
		return supabaseUploadResponse{}, err
	}

	statusCode, responseBody, requestErr := httpRequestFn(
		context.Background(),
		http.MethodPost,
		fmt.Sprintf(
			"%s/storage/v1/object/%s/%s",
			config.baseURL,
			artifactSupabaseBucketName,
			encodeStorageObjectPath(objectPath),
		),
		map[string]string{
			"Authorization": "Bearer " + config.serviceKey,
			"apikey":        config.serviceKey,
			"cache-control": "max-age=3600",
			"content-type":  contentType,
			"x-metadata":    base64.StdEncoding.EncodeToString(metadataRaw),
			"x-upsert":      "true",
		},
		body,
	)
	if requestErr != nil {
		return supabaseUploadResponse{}, requestErr
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return supabaseUploadResponse{}, fmt.Errorf(
			"supabase storage upload failed: %d %s",
			statusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	var response supabaseUploadResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return supabaseUploadResponse{}, err
	}
	return response, nil
}

func fetchSupabaseObjectInfo(objectPath string) (supabaseObjectInfoResponse, error) {
	config, err := readSupabaseArtifactConfig()
	if err != nil {
		return supabaseObjectInfoResponse{}, err
	}
	statusCode, responseBody, requestErr := httpRequestFn(
		context.Background(),
		http.MethodGet,
		fmt.Sprintf(
			"%s/storage/v1/object/info/%s/%s",
			config.baseURL,
			artifactSupabaseBucketName,
			encodeStorageObjectPath(objectPath),
		),
		map[string]string{
			"Authorization": "Bearer " + config.serviceKey,
			"apikey":        config.serviceKey,
		},
		nil,
	)
	if requestErr != nil {
		return supabaseObjectInfoResponse{}, requestErr
	}
	if statusCode == http.StatusNotFound || isSupabaseObjectNotFoundResponse(statusCode, responseBody) {
		return supabaseObjectInfoResponse{}, os.ErrNotExist
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return supabaseObjectInfoResponse{}, fmt.Errorf(
			"supabase storage info failed: %d %s",
			statusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	var response supabaseObjectInfoResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return supabaseObjectInfoResponse{}, err
	}
	return response, nil
}

func isSupabaseObjectNotFoundResponse(statusCode int, responseBody []byte) bool {
	if statusCode != http.StatusBadRequest {
		return false
	}

	var payload supabaseStorageErrorResponse
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return false
	}

	return strings.TrimSpace(payload.StatusCode) == "404" &&
		strings.EqualFold(strings.TrimSpace(payload.Error), "not_found")
}

func createSupabaseSignedURL(objectPath string) (string, error) {
	config, err := readSupabaseArtifactConfig()
	if err != nil {
		return "", err
	}
	requestBody, err := json.Marshal(map[string]int{"expiresIn": 300})
	if err != nil {
		return "", err
	}

	statusCode, responseBody, requestErr := httpRequestFn(
		context.Background(),
		http.MethodPost,
		fmt.Sprintf(
			"%s/storage/v1/object/sign/%s/%s",
			config.baseURL,
			artifactSupabaseBucketName,
			encodeStorageObjectPath(objectPath),
		),
		map[string]string{
			"Authorization": "Bearer " + config.serviceKey,
			"apikey":        config.serviceKey,
			"content-type":  "application/json",
		},
		requestBody,
	)
	if requestErr != nil {
		return "", requestErr
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf(
			"supabase signed URL creation failed: %d %s",
			statusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	var response supabaseSignedURLResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.SignedURL) == "" {
		return "", errors.New("supabase signed URL response was empty")
	}

	return config.baseURL + response.SignedURL, nil
}

func readSupabaseArtifactConfig() (supabaseArtifactConfig, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("SUPABASE_API_URL")), "/")
	serviceKey := strings.TrimSpace(os.Getenv("SUPABASE_SERVICE_ROLE_KEY"))
	if baseURL == "" {
		return supabaseArtifactConfig{}, errors.New("missing required environment variable: SUPABASE_API_URL")
	}
	if serviceKey == "" {
		return supabaseArtifactConfig{}, errors.New("missing required environment variable: SUPABASE_SERVICE_ROLE_KEY")
	}
	return supabaseArtifactConfig{
		baseURL:    baseURL,
		serviceKey: serviceKey,
	}, nil
}

func encodeStorageObjectPath(objectPath string) string {
	trimmed := strings.TrimLeft(strings.TrimSpace(objectPath), "/")
	if trimmed == "" {
		return ""
	}

	parts := strings.Split(trimmed, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}

	return strings.Join(parts, "/")
}

func refreshGoogleDriveAccessToken(refreshToken string) (string, error) {
	config, err := readGoogleDriveArtifactConfig()
	if err != nil {
		return "", err
	}

	form := url.Values{}
	form.Set("client_id", config.clientID)
	form.Set("client_secret", config.clientSecret)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", strings.TrimSpace(refreshToken))

	statusCode, responseBody, requestErr := httpRequestFn(
		context.Background(),
		http.MethodPost,
		"https://oauth2.googleapis.com/token",
		map[string]string{
			"content-type": "application/x-www-form-urlencoded",
		},
		[]byte(form.Encode()),
	)
	if requestErr != nil {
		return "", requestErr
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("google drive token refresh failed: %d %s", statusCode, strings.TrimSpace(string(responseBody)))
	}

	var response googleDriveTokenResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.AccessToken) == "" {
		return "", errors.New("google drive token refresh returned an empty access token")
	}

	return strings.TrimSpace(response.AccessToken), nil
}

func ensureGoogleDriveFolderPath(accessToken, rootFolderID string, segments []string) (string, error) {
	parentID := strings.TrimSpace(rootFolderID)
	if parentID == "" {
		return "", errors.New("google drive root folder id is required")
	}

	for _, segment := range segments {
		name := strings.TrimSpace(segment)
		if name == "" {
			continue
		}

		existing, err := findGoogleDriveFile(accessToken, parentID, name)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(existing.ID) != "" {
			parentID = existing.ID
			continue
		}

		created, err := createGoogleDriveFolder(accessToken, parentID, name)
		if err != nil {
			return "", err
		}
		parentID = created.ID
	}

	return parentID, nil
}

func googleDriveFolderSegments(relativePath string) []string {
	cleaned := filepath.Dir(strings.ReplaceAll(relativePath, "\\", "/"))
	if cleaned == "." || cleaned == "/" || strings.TrimSpace(cleaned) == "" {
		return nil
	}

	parts := strings.Split(cleaned, "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		segments = append(segments, part)
	}
	return segments
}

func findGoogleDriveFile(accessToken, parentID, name string) (googleDriveFile, error) {
	query := url.Values{}
	query.Set("q", fmt.Sprintf(
		"name = '%s' and '%s' in parents and trashed = false",
		escapeGoogleDriveQueryValue(name),
		escapeGoogleDriveQueryValue(parentID),
	))
	query.Set("fields", "files(id,name,mimeType,webViewLink,appProperties)")
	query.Set("pageSize", "1")
	query.Set("includeItemsFromAllDrives", "true")
	query.Set("supportsAllDrives", "true")

	statusCode, responseBody, requestErr := httpRequestFn(
		context.Background(),
		http.MethodGet,
		"https://www.googleapis.com/drive/v3/files?"+query.Encode(),
		map[string]string{
			"Authorization": "Bearer " + accessToken,
		},
		nil,
	)
	if requestErr != nil {
		return googleDriveFile{}, requestErr
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return googleDriveFile{}, fmt.Errorf("google drive file lookup failed: %d %s", statusCode, strings.TrimSpace(string(responseBody)))
	}

	var response googleDriveListResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return googleDriveFile{}, err
	}
	if len(response.Files) == 0 {
		return googleDriveFile{}, nil
	}
	return response.Files[0], nil
}

func fetchGoogleDriveFileByID(accessToken, fileID string) (googleDriveFile, error) {
	statusCode, responseBody, requestErr := httpRequestFn(
		context.Background(),
		http.MethodGet,
		"https://www.googleapis.com/drive/v3/files/"+url.PathEscape(strings.TrimSpace(fileID))+"?fields=id,name,mimeType,webViewLink,appProperties&supportsAllDrives=true",
		map[string]string{
			"Authorization": "Bearer " + accessToken,
		},
		nil,
	)
	if requestErr != nil {
		return googleDriveFile{}, requestErr
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return googleDriveFile{}, fmt.Errorf("google drive file lookup by id failed: %d %s", statusCode, strings.TrimSpace(string(responseBody)))
	}
	var response googleDriveFile
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return googleDriveFile{}, err
	}
	return response, nil
}

func createGoogleDriveFolder(accessToken, parentID, name string) (googleDriveFile, error) {
	return createGoogleDriveMultipartFile(
		accessToken,
		http.MethodPost,
		"https://www.googleapis.com/upload/drive/v3/files?uploadType=multipart&supportsAllDrives=true&fields=id,name,mimeType,webViewLink,appProperties",
		map[string]any{
			"name":          name,
			"mimeType":      googleDriveFolderMimeType,
			"parents":       []string{parentID},
			"appProperties": map[string]string{"syncRole": "folder"},
		},
		nil,
		"application/octet-stream",
	)
}

func upsertGoogleDriveFile(
	accessToken string,
	parentID string,
	name string,
	body []byte,
	contentType string,
	appProperties map[string]string,
) (googleDriveFile, error) {
	existing, err := findGoogleDriveFile(accessToken, parentID, name)
	if err != nil {
		return googleDriveFile{}, err
	}

	metadata := map[string]any{
		"name":          name,
		"parents":       []string{parentID},
		"appProperties": appProperties,
	}

	endpoint := "https://www.googleapis.com/upload/drive/v3/files?uploadType=multipart&supportsAllDrives=true&fields=id,name,mimeType,webViewLink,appProperties"
	method := http.MethodPost
	if strings.TrimSpace(existing.ID) != "" {
		method = http.MethodPatch
		endpoint = "https://www.googleapis.com/upload/drive/v3/files/" + url.PathEscape(existing.ID) + "?uploadType=multipart&supportsAllDrives=true&fields=id,name,mimeType,webViewLink,appProperties"
		delete(metadata, "parents")
	}

	return createGoogleDriveMultipartFile(accessToken, method, endpoint, metadata, body, contentType)
}

func createGoogleDriveMultipartFile(
	accessToken string,
	method string,
	endpoint string,
	metadata map[string]any,
	body []byte,
	contentType string,
) (googleDriveFile, error) {
	requestBody, contentHeader, err := buildGoogleDriveMultipartBody(metadata, body, contentType)
	if err != nil {
		return googleDriveFile{}, err
	}

	statusCode, responseBody, requestErr := httpRequestFn(
		context.Background(),
		method,
		endpoint,
		map[string]string{
			"Authorization": "Bearer " + accessToken,
			"content-type":  contentHeader,
		},
		requestBody,
	)
	if requestErr != nil {
		return googleDriveFile{}, requestErr
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return googleDriveFile{}, fmt.Errorf("google drive upload failed: %d %s", statusCode, strings.TrimSpace(string(responseBody)))
	}

	var response googleDriveFile
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return googleDriveFile{}, err
	}
	return response, nil
}

func buildGoogleDriveMultipartBody(
	metadata map[string]any,
	body []byte,
	contentType string,
) ([]byte, string, error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	metadataPart, err := writer.CreatePart(map[string][]string{
		"Content-Type": {"application/json; charset=UTF-8"},
	})
	if err != nil {
		return nil, "", err
	}
	encodedMetadata, err := json.Marshal(metadata)
	if err != nil {
		return nil, "", err
	}
	if _, err := metadataPart.Write(encodedMetadata); err != nil {
		return nil, "", err
	}

	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	filePart, err := writer.CreatePart(map[string][]string{
		"Content-Type": {contentType},
	})
	if err != nil {
		return nil, "", err
	}
	if _, err := filePart.Write(body); err != nil {
		return nil, "", err
	}

	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func escapeGoogleDriveQueryValue(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "'", "\\'")
}
