package runner

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Grok ACP reports promptCapabilities.image=false, so there are no multimodal
// image content blocks. Path fallback (operator request / CA-483): decode
// PromptAttachment bytes under <cwd>/.tmp/images/<turn>/ and append absolute
// paths to the text prompt so Grok can open them with file tools.
//
// Distinct from Codex writeCodexImageAttachments (localImage input items under
// os.TempDir/flowpilot-attach). Grok needs workspace-adjacent paths and a
// prompt suffix, not ACP image blocks.

func grokImageAttachmentRoot(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return filepath.Join(os.TempDir(), "flowpilot-tmp", "images")
	}
	return filepath.Join(cwd, ".tmp", "images")
}

// writeGrokImagePathFallback writes attachment bytes under
// <cwd>/.tmp/images/<turnID>/ and returns absolute paths + cleanup.
// Empty attachments → nil paths, no-op cleanup.
func writeGrokImagePathFallback(cwd, turnID string, atts []PromptAttachment) (paths []string, cleanup func(), err error) {
	noop := func() {}
	if len(atts) == 0 {
		return nil, noop, nil
	}
	dir := filepath.Join(grokImageAttachmentRoot(cwd), safeTurnDirName(turnID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, noop, fmt.Errorf("grok image fallback: create dir: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(dir) }

	paths = make([]string, 0, len(atts))
	for i, att := range atts {
		raw, decErr := base64.StdEncoding.DecodeString(att.Data)
		if decErr != nil {
			cleanup()
			return nil, noop, fmt.Errorf("grok image fallback: decode %q: %w", att.OriginalName, decErr)
		}
		// Prefer original basename when it has a safe image extension; else index+mime.
		name := grokAttachmentFileName(i, att)
		path := filepath.Join(dir, name)
		if werr := os.WriteFile(path, raw, 0o600); werr != nil {
			cleanup()
			return nil, noop, fmt.Errorf("grok image fallback: write %q: %w", att.OriginalName, werr)
		}
		abs, aerr := filepath.Abs(path)
		if aerr != nil {
			abs = path
		}
		paths = append(paths, abs)
	}
	return paths, cleanup, nil
}

func grokAttachmentFileName(index int, att PromptAttachment) string {
	base := filepath.Base(strings.TrimSpace(att.OriginalName))
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		// Sanitize basename (no path seps already via Base).
		safe := strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
				return r
			default:
				return '_'
			}
		}, base)
		if safe != "" && safe != "." && safe != ".." {
			return fmt.Sprintf("%d_%s", index, safe)
		}
	}
	return fmt.Sprintf("%d%s", index, mimeExtension(att.MimeType))
}

// appendGrokImagePathsToPrompt adds a text block listing absolute image paths
// for the model to open with tools. No-op when paths is empty.
func appendGrokImagePathsToPrompt(prompt string, paths []string, originalNames []string) string {
	if len(paths) == 0 {
		return prompt
	}
	var b strings.Builder
	b.WriteString(prompt)
	if prompt != "" && !strings.HasSuffix(prompt, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("\n---\n")
	b.WriteString("[FlowPilot image attachments — path fallback]\n")
	b.WriteString("Grok ACP does not accept multimodal image blocks (promptCapabilities.image=false).\n")
	b.WriteString("Each image was written to disk for this turn. Open these files with your tools if you need their visual content:\n")
	for i, p := range paths {
		label := ""
		if i < len(originalNames) && strings.TrimSpace(originalNames[i]) != "" {
			label = " (" + strings.TrimSpace(originalNames[i]) + ")"
		}
		b.WriteString(fmt.Sprintf("%d. %s%s\n", i+1, p, label))
	}
	return b.String()
}

// sweepGrokImagePathFallback removes orphaned per-turn dirs under
// <cwd>/.tmp/images (and the process temp root) older than maxAge.
// Best-effort; safe at runner boot. cwd may be empty (temp root only).
func sweepGrokImagePathFallback(cwd string, maxAge time.Duration, now time.Time) {
	roots := []string{grokImageAttachmentRoot(cwd)}
	// Always also sweep the no-cwd temp root used when Cwd was empty.
	if alt := grokImageAttachmentRoot(""); alt != roots[0] {
		roots = append(roots, alt)
	}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
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
}
