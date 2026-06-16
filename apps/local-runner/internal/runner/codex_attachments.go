package runner

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// codexAttachmentRoot is the per-runner temp root that holds image attachments
// written for Codex turns (Task-052). Codex reads images by path on the runner host,
// so each turn's attachments are decoded to files under <tmp>/flowpilot-attach/<turn>/.
func codexAttachmentRoot() string {
	return filepath.Join(os.TempDir(), "flowpilot-attach")
}

// mimeExtension maps a normalized image MIME to a file extension. Falls back to .img
// for unknown types (Codex keys on content, not extension, but a sane extension helps
// debugging and any extension-sniffing on the app-server side).
func mimeExtension(mime string) string {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".img"
	}
}

// safeTurnDirName keeps the per-turn subdir name filesystem-safe (turn ids are
// runner-generated, but never trust them as raw path segments).
func safeTurnDirName(turnID string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, turnID)
	if cleaned == "" {
		cleaned = "turn"
	}
	return cleaned
}

// writeCodexImageAttachments decodes each image attachment to a file under a per-turn
// temp dir and returns the absolute paths plus a cleanup func. With no images it is a
// no-op returning a nil slice and a no-op cleanup. The caller defers cleanup so the
// dir is removed when the turn returns (complete/fail/interrupt); see SendTurn.
func writeCodexImageAttachments(turnID string, atts []PromptAttachment) ([]string, func(), error) {
	noop := func() {}
	if len(atts) == 0 {
		return nil, noop, nil
	}

	dir := filepath.Join(codexAttachmentRoot(), safeTurnDirName(turnID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, noop, fmt.Errorf("codex attachments: create temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	paths := make([]string, 0, len(atts))
	for i, att := range atts {
		raw, err := base64.StdEncoding.DecodeString(att.Data)
		if err != nil {
			cleanup()
			return nil, noop, fmt.Errorf("codex attachments: decode %q: %w", att.OriginalName, err)
		}
		name := fmt.Sprintf("%d%s", i, mimeExtension(att.MimeType))
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			cleanup()
			return nil, noop, fmt.Errorf("codex attachments: write %q: %w", att.OriginalName, err)
		}
		paths = append(paths, path)
	}
	return paths, cleanup, nil
}

// sweepCodexImageAttachments removes orphaned per-turn attachment dirs left behind by
// a hard crash/kill (the deferred cleanup in SendTurn cannot run in that case). Best
// effort: it removes entries older than maxAge and is safe to call at runner startup.
func sweepCodexImageAttachments(maxAge time.Duration, now time.Time) {
	root := codexAttachmentRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		return // nothing to sweep (dir absent) or unreadable — ignore
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) >= maxAge {
			_ = os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
}
