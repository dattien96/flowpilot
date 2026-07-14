package runner

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocateSessionFile returns the provider-owned session artifact for a resume
// handle. For Codex/Claude this is a single session file; for Grok it is the
// session directory at GROK_HOME/sessions/<encoded-cwd>/<session-id>/.
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
	case ProviderKeyGrok:
		// Task-210 Option B: Grok stores a full session directory (chat_history.jsonl
		// + sidecars). Synthetic FlowPilot thread-* handles are not provider artifacts.
		if !isGrokRealSessionID(sessionID) {
			return "", false
		}
		dir := grokSessionDirPath(accountHome, cwd, sessionID)
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return "", false
		}
		if _, err := os.Stat(filepath.Join(dir, "chat_history.jsonl")); err != nil {
			return "", false
		}
		return dir, true
	default:
		return "", false
	}
}

// migrateCodexReservedSpawnAgentTool repairs rollout metadata written before
// BUG-124. Codex reloads dynamic_tools from session_meta during thread/resume, so
// passing the corrected alias in resume params alone cannot override the stale,
// now-reserved spawn_agent declaration.
func migrateCodexReservedSpawnAgentTool(path string) (bool, error) {
	src, err := os.Open(path)
	if err != nil {
		return false, err
	}
	// Idempotent close: the early-return paths below rely on the defer, but the success
	// path must release the handle BEFORE renaming over `path` — Windows refuses to
	// replace a file while a handle to it is still open ("Access is denied"). (BUG-127)
	srcClosed := false
	closeSrc := func() {
		if !srcClosed {
			srcClosed = true
			_ = src.Close()
		}
	}
	defer closeSrc()

	info, err := src.Stat()
	if err != nil {
		return false, err
	}
	reader := bufio.NewReader(src)
	firstLine, readErr := reader.ReadBytes('\n')
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return false, readErr
	}
	if len(firstLine) == 0 {
		return false, nil
	}

	hadNewline := firstLine[len(firstLine)-1] == '\n'
	rawLine := bytes.TrimSuffix(firstLine, []byte{'\n'})
	var entry map[string]any
	if err := json.Unmarshal(rawLine, &entry); err != nil {
		return false, nil
	}
	if entry["type"] != "session_meta" {
		return false, nil
	}
	payload, _ := entry["payload"].(map[string]any)
	tools, _ := payload["dynamic_tools"].([]any)
	changed := false
	for _, item := range tools {
		tool, _ := item.(map[string]any)
		if tool["name"] == "spawn_agent" {
			tool["name"] = codexSpawnAgentToolName
			changed = true
		}
	}
	if !changed {
		return false, nil
	}

	migratedFirstLine, err := json.Marshal(entry)
	if err != nil {
		return false, err
	}
	if hadNewline {
		migratedFirstLine = append(migratedFirstLine, '\n')
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".bug124-*")
	if err != nil {
		return false, err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		cleanup()
		return false, err
	}
	if _, err := tmp.Write(migratedFirstLine); err != nil {
		cleanup()
		return false, err
	}
	if _, err := io.Copy(tmp, reader); err != nil {
		cleanup()
		return false, err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return false, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return false, err
	}
	// Release the source handle before renaming over it (Windows requirement). The
	// reader has already been fully drained into tmp via io.Copy above. (BUG-127)
	closeSrc()
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return false, err
	}
	return true, nil
}

// RelocateSessionFile copies a provider session artifact into the target account
// home. Codex/Claude artifacts are single files; Grok artifacts are session
// directories (full tree copy). Same-home source/destination is a successful no-op.
func RelocateSessionFile(providerKey ProviderKey, srcPath, targetHome, sessionID, cwd string) (string, error) {
	if strings.TrimSpace(srcPath) == "" || strings.TrimSpace(targetHome) == "" {
		return "", errors.New("source path and target home are required")
	}
	dstPath, err := relocationTargetPath(providerKey, srcPath, targetHome, sessionID, cwd)
	if err != nil {
		return "", err
	}
	// Same-home accounts (e.g. default account re-registered under a new ID on a
	// single-account machine) map src and dst to the same physical file.  Nothing to
	// copy; return success so the caller can rebind providerAccountID and persist.
	if filepath.Clean(dstPath) == filepath.Clean(srcPath) {
		return srcPath, nil
	}
	if providerKey == ProviderKeyGrok {
		return relocateGrokSessionDir(srcPath, dstPath)
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(dstPath); err == nil {
		same, cmpErr := sameFileContents(srcPath, dstPath)
		if cmpErr != nil {
			return "", cmpErr
		}
		if same {
			return dstPath, nil
		}
		if providerKey == ProviderKeyCodex {
			updated, updateErr := updateCodexDestinationIfSameSessionExtends(srcPath, dstPath)
			if updateErr != nil {
				return "", updateErr
			}
			if updated {
				return dstPath, nil
			}
		}
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

func relocateGrokSessionDir(srcPath, dstPath string) (string, error) {
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return "", err
	}
	if !srcInfo.IsDir() {
		return "", errors.New("grok session source must be a directory")
	}
	if _, err := os.Stat(dstPath); err == nil {
		same, cmpErr := sameDirContents(srcPath, dstPath)
		if cmpErr != nil {
			return "", cmpErr
		}
		if same {
			return dstPath, nil
		}
		return "", errors.New("destination session artifact already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := copyDirRecursive(srcPath, dstPath); err != nil {
		_ = os.RemoveAll(dstPath)
		return "", err
	}
	return dstPath, nil
}

// pathUnderRoot reports whether candidate is the root directory itself or a
// descendant of it after cleaning both sides (Task-210 path-safety for Grok).
func pathUnderRoot(root, candidate string) bool {
	rootClean := filepath.Clean(root)
	candClean := filepath.Clean(candidate)
	if rootClean == "" || candClean == "" {
		return false
	}
	sep := string(filepath.Separator)
	if candClean == rootClean {
		return true
	}
	return strings.HasPrefix(candClean, rootClean+sep)
}

func copyDirRecursive(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(srcDir, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			// Skip symlinks to avoid escaping the session tree.
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFileContents(path, target)
	})
}

func copyFileContents(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return nil
}

func sameDirContents(srcDir, dstDir string) (bool, error) {
	type fileMeta struct {
		size int64
	}
	srcFiles := map[string]fileMeta{}
	err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, relErr := filepath.Rel(srcDir, path)
		if relErr != nil {
			return relErr
		}
		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		srcFiles[filepath.ToSlash(rel)] = fileMeta{size: info.Size()}
		return nil
	})
	if err != nil {
		return false, err
	}
	dstFiles := map[string]fileMeta{}
	err = filepath.WalkDir(dstDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, relErr := filepath.Rel(dstDir, path)
		if relErr != nil {
			return relErr
		}
		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		dstFiles[filepath.ToSlash(rel)] = fileMeta{size: info.Size()}
		return nil
	})
	if err != nil {
		return false, err
	}
	if len(srcFiles) != len(dstFiles) {
		return false, nil
	}
	for rel, meta := range srcFiles {
		dstMeta, ok := dstFiles[rel]
		if !ok || dstMeta.size != meta.size {
			return false, nil
		}
		same, cmpErr := sameFileContents(filepath.Join(srcDir, filepath.FromSlash(rel)), filepath.Join(dstDir, filepath.FromSlash(rel)))
		if cmpErr != nil {
			return false, cmpErr
		}
		if !same {
			return false, nil
		}
	}
	return true, nil
}

func updateCodexDestinationIfSameSessionExtends(srcPath, dstPath string) (bool, error) {
	srcMeta, srcOK := readCodexRolloutMeta(srcPath)
	dstMeta, dstOK := readCodexRolloutMeta(dstPath)
	if !srcOK || !dstOK || srcMeta.ID == "" || srcMeta.ID != dstMeta.ID {
		return false, nil
	}
	if srcMeta.Cwd != "" && dstMeta.Cwd != "" && srcMeta.Cwd != dstMeta.Cwd {
		return false, nil
	}
	src, err := os.ReadFile(srcPath)
	if err != nil {
		return false, err
	}
	dst, err := os.ReadFile(dstPath)
	if err != nil {
		return false, err
	}
	if bytes.Equal(src, dst) {
		return true, nil
	}
	if bytes.HasPrefix(src, dst) {
		if err := os.WriteFile(dstPath, src, 0o644); err != nil {
			return false, err
		}
		return true, nil
	}
	if bytes.HasPrefix(dst, src) {
		return true, nil
	}
	return false, nil
}

func sameFileContents(pathA, pathB string) (bool, error) {
	infoA, err := os.Stat(pathA)
	if err != nil {
		return false, err
	}
	infoB, err := os.Stat(pathB)
	if err != nil {
		return false, err
	}
	if infoA.Size() != infoB.Size() {
		return false, nil
	}
	a, err := os.ReadFile(pathA)
	if err != nil {
		return false, err
	}
	b, err := os.ReadFile(pathB)
	if err != nil {
		return false, err
	}
	return string(a) == string(b), nil
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

// defaultProviderSessionHome returns the provider-owned data home even when the
// provider is not installed or authenticated yet. This lets Drive restore keep
// chat history readable while turn execution remains gated by provider auth.
func defaultProviderSessionHome(providerKey ProviderKey) (string, bool) {
	userHome := preferredUserHomeDir()
	if strings.TrimSpace(userHome) == "" {
		return "", false
	}
	switch providerKey {
	case ProviderKeyCodex:
		return filepath.Join(userHome, ".codex"), true
	case ProviderKeyClaude:
		return userHome, true
	case ProviderKeyGrok:
		return filepath.Join(userHome, ".grok"), true
	default:
		return "", false
	}
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
	case ProviderKeyGrok:
		id := strings.TrimSpace(sessionID)
		if id == "" {
			id = filepath.Base(filepath.Clean(srcPath))
		}
		if !isGrokRealSessionID(id) {
			return "", errors.New("grok relocation requires a real ACP session id")
		}
		var dst string
		if strings.TrimSpace(cwd) == "" {
			// Fall back to the encoded cwd segment from the source path:
			// .../sessions/<enc-cwd>/<session-id>
			parent := filepath.Base(filepath.Dir(filepath.Clean(srcPath)))
			if parent == "" || parent == "." || parent == "sessions" || parent == ".." || strings.ContainsAny(parent, `/\`) {
				return "", errors.New("grok relocation requires cwd")
			}
			dst = filepath.Join(targetHome, "sessions", parent, id)
		} else {
			dst = grokSessionDirPath(targetHome, cwd, id)
		}
		if !pathUnderRoot(targetHome, dst) {
			return "", errors.New("grok relocation target escapes account home")
		}
		return dst, nil
	default:
		return "", errors.New("unsupported provider session relocation")
	}
}

func restoreTargetPath(providerKey ProviderKey, targetHome, relativePath, sessionID, cwd string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.ReplaceAll(strings.TrimSpace(relativePath), "\\", "/")))
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
