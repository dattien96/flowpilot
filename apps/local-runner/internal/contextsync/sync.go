package contextsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

type DriveSyncer interface {
	SyncFile(ctx context.Context, localPath, remoteName string) error
}

type SyncResult struct {
	Synced  []string `json:"synced"`
	Skipped []string `json:"skipped"`
	Errors  []string `json:"errors"`
}

func SyncSharedFiles(ctx context.Context, store *EngineStore, syncer DriveSyncer) SyncResult {
	result := SyncResult{
		Synced:  []string{},
		Skipped: []string{},
		Errors:  []string{},
	}
	for _, path := range store.SharedFiles() {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			result.Skipped = append(result.Skipped, path)
			continue
		}
		if syncer == nil {
			result.Skipped = append(result.Skipped, path)
			continue
		}
		if err := syncer.SyncFile(ctx, path, filepath.Base(path)); err != nil {
			result.Errors = append(result.Errors, path+": "+err.Error())
		} else {
			result.Synced = append(result.Synced, path)
		}
	}
	return result
}

// ComputeSHA256 returns the hex SHA256 of a file's contents. Used for manifest integrity.
func ComputeSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

type ManifestEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size_bytes"`
}

// WriteManifest writes a JSON manifest of the shared files (path, sha256, size)
// to store.DotFlowpilotDir+"/manifest.json". Used as the Drive _index equivalent.
func WriteManifest(store *EngineStore) error {
	var entries []ManifestEntry
	for _, path := range store.SharedFiles() {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		hash, err := ComputeSHA256(path)
		if err != nil {
			return err
		}
		entries = append(entries, ManifestEntry{
			Path:   path,
			SHA256: hash,
			Size:   info.Size(),
		})
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(store.DotFlowpilotDir, "manifest.json"), data, 0o644)
}
