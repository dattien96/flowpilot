package runner

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocateSessionFile returns the provider-owned session file for a resume handle.
func LocateSessionFile(providerKey ProviderKey, accountHome, sessionID, cwd string) (string, bool) {
	if strings.TrimSpace(accountHome) == "" || strings.TrimSpace(sessionID) == "" {
		return "", false
	}
	switch providerKey {
	case ProviderKeyCodex:
		root := filepath.Join(accountHome, "sessions")
		var matches []string
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, "rollout-") && strings.HasSuffix(name, "-"+sessionID+".jsonl") {
				matches = append(matches, path)
			}
			return nil
		})
		if len(matches) == 0 {
			return "", false
		}
		return newestPath(matches), true
	case ProviderKeyClaude:
		root := filepath.Join(accountHome, ".claude", "projects")
		target := sessionID + ".jsonl"
		var found string
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if d.Name() == target {
				found = path
				return fs.SkipAll
			}
			return nil
		})
		return found, found != ""
	default:
		return "", false
	}
}

// RelocateSessionFile copies a provider session file into the target account home.
func RelocateSessionFile(providerKey ProviderKey, srcPath, targetHome, sessionID, cwd string) (string, error) {
	if strings.TrimSpace(srcPath) == "" || strings.TrimSpace(targetHome) == "" {
		return "", errors.New("source path and target home are required")
	}
	dstPath, err := relocationTargetPath(providerKey, srcPath, targetHome, sessionID, cwd)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(dstPath); err == nil {
		return "", errors.New("destination session file already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return dstPath, nil
}

func RestoreSessionFile(providerKey ProviderKey, targetHome, relativePath, sessionID, cwd string, body []byte) (string, error) {
	dstPath, err := restoreTargetPath(providerKey, targetHome, relativePath, sessionID, cwd)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dstPath, body, 0o644); err != nil {
		return "", err
	}
	return dstPath, nil
}

func DiscoverCodexRolloutSessionID(accountHome, cwd string) (string, bool) {
	root := filepath.Join(accountHome, "sessions")
	bestID := ""
	bestTime := time.Time{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasPrefix(d.Name(), "rollout-") || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		meta, ok := readCodexRolloutMeta(path)
		if !ok || meta.Cwd != cwd || meta.ID == "" {
			return nil
		}
		when := meta.Timestamp
		if when.IsZero() {
			if info, statErr := d.Info(); statErr == nil {
				when = info.ModTime()
			}
		}
		if bestID == "" || when.After(bestTime) {
			bestID = meta.ID
			bestTime = when
		}
		return nil
	})
	return bestID, bestID != ""
}

type codexRolloutMeta struct {
	ID        string
	Cwd       string
	Timestamp time.Time
}

func readCodexRolloutMeta(path string) (codexRolloutMeta, bool) {
	f, err := os.Open(path)
	if err != nil {
		return codexRolloutMeta{}, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return codexRolloutMeta{}, false
	}
	var raw map[string]any
	if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
		return codexRolloutMeta{}, false
	}
	payload, _ := raw["payload"].(map[string]any)
	if payload == nil {
		payload = raw
	}
	id, _ := payload["id"].(string)
	cwd, _ := payload["cwd"].(string)
	ts, _ := payload["timestamp"].(string)
	var parsed time.Time
	if ts != "" {
		parsed, _ = time.Parse(time.RFC3339Nano, ts)
	}
	return codexRolloutMeta{ID: id, Cwd: cwd, Timestamp: parsed}, true
}

func relocationTargetPath(providerKey ProviderKey, srcPath, targetHome, sessionID, cwd string) (string, error) {
	switch providerKey {
	case ProviderKeyCodex:
		parts := strings.Split(filepath.ToSlash(srcPath), "/sessions/")
		if len(parts) == 2 {
			return filepath.Join(targetHome, "sessions", filepath.FromSlash(parts[1])), nil
		}
		name := filepath.Base(srcPath)
		if sessionID == "" {
			sessionID = strings.TrimSuffix(strings.TrimPrefix(name, "rollout-"), ".jsonl")
		}
		now := time.Now().UTC()
		return filepath.Join(
			targetHome,
			"sessions",
			now.Format("2006"),
			now.Format("01"),
			now.Format("02"),
			name,
		), nil
	case ProviderKeyClaude:
		hashDir := filepath.Base(filepath.Dir(srcPath))
		return filepath.Join(targetHome, ".claude", "projects", hashDir, sessionID+".jsonl"), nil
	default:
		return "", errors.New("unsupported provider session relocation")
	}
}

func restoreTargetPath(providerKey ProviderKey, targetHome, relativePath, sessionID, cwd string) (string, error) {
	clean := filepath.Clean(strings.ReplaceAll(strings.TrimSpace(relativePath), "\\", "/"))
	clean = strings.TrimPrefix(clean, "./")
	if clean == "." || clean == "" {
		return "", errors.New("relative session path is required")
	}
	if strings.HasPrefix(clean, "../") || clean == ".." {
		return "", errors.New("relative session path escapes target home")
	}
	switch providerKey {
	case ProviderKeyCodex:
		if clean != "sessions" && !strings.HasPrefix(clean, "sessions/") {
			return "", errors.New("invalid codex restore path")
		}
	case ProviderKeyClaude:
		if clean != ".claude" && !strings.HasPrefix(clean, ".claude/") {
			return "", errors.New("invalid claude restore path")
		}
	default:
		return "", errors.New("unsupported provider session relocation")
	}
	return filepath.Join(targetHome, filepath.FromSlash(clean)), nil
}

func newestPath(paths []string) string {
	best := ""
	bestTime := time.Time{}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestTime) {
			best = path
			bestTime = info.ModTime()
		}
	}
	return best
}
