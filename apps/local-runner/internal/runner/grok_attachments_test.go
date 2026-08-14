package runner

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteGrokImagePathFallback_WritesUnderDotTmpImages(t *testing.T) {
	cwd := t.TempDir()
	atts := []PromptAttachment{sampleImageAttachment()}
	paths, cleanup, err := writeGrokImagePathFallback(cwd, "turn-g1", atts)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	defer cleanup()
	if len(paths) != 1 {
		t.Fatalf("paths=%d", len(paths))
	}
	if !strings.Contains(filepath.ToSlash(paths[0]), "/.tmp/images/") {
		t.Fatalf("want path under .tmp/images, got %q", paths[0])
	}
	raw, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(raw) != "PNGDATA" {
		t.Fatalf("bytes=%q", raw)
	}
	// Dir must live under cwd/.tmp/images/turn-g1
	rel, err := filepath.Rel(cwd, paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.ToSlash(rel), ".tmp/images/") {
		t.Fatalf("rel=%q", rel)
	}
}

func TestWriteGrokImagePathFallback_EmptyNoop(t *testing.T) {
	paths, cleanup, err := writeGrokImagePathFallback(t.TempDir(), "t", nil)
	if err != nil || paths != nil {
		t.Fatalf("noop got paths=%v err=%v", paths, err)
	}
	cleanup()
}

func TestWriteGrokImagePathFallback_BadBase64(t *testing.T) {
	att := sampleImageAttachment()
	att.Data = "!!!bad!!!"
	if _, _, err := writeGrokImagePathFallback(t.TempDir(), "t", []PromptAttachment{att}); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestWriteGrokImagePathFallback_CleanupRemovesDir(t *testing.T) {
	cwd := t.TempDir()
	paths, cleanup, err := writeGrokImagePathFallback(cwd, "turn-clean", []PromptAttachment{sampleImageAttachment()})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(paths[0])
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("cleanup left dir: %v", err)
	}
}

func TestAppendGrokImagePathsToPrompt(t *testing.T) {
	out := appendGrokImagePathsToPrompt("hello", []string{`C:\a\b.png`, `/tmp/c.jpg`}, []string{"shot.png", "other.jpg"})
	if !strings.Contains(out, "hello") {
		t.Fatal("missing original prompt")
	}
	if !strings.Contains(out, "path fallback") {
		t.Fatal("missing fallback banner")
	}
	if !strings.Contains(out, `C:\a\b.png`) || !strings.Contains(out, "shot.png") {
		t.Fatalf("missing path/label: %s", out)
	}
	if !strings.Contains(out, "/tmp/c.jpg") {
		t.Fatal("missing second path")
	}
	// empty paths leave prompt unchanged
	if got := appendGrokImagePathsToPrompt("x", nil, nil); got != "x" {
		t.Fatalf("empty: %q", got)
	}
}

func TestSweepGrokImagePathFallback(t *testing.T) {
	cwd := t.TempDir()
	root := grokImageAttachmentRoot(cwd)
	oldDir := filepath.Join(root, "old-turn")
	freshDir := filepath.Join(root, "fresh-turn")
	for _, d := range []string{oldDir, freshDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "x.png"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Make oldDir appear old
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldDir, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	sweepGrokImagePathFallback(cwd, time.Hour, time.Now())
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("old dir should be swept: %v", err)
	}
	if _, err := os.Stat(freshDir); err != nil {
		t.Fatalf("fresh dir should remain: %v", err)
	}
}

func TestGrokAttachmentFileName_PrefersOriginal(t *testing.T) {
	att := sampleImageAttachment()
	att.OriginalName = "my shot.png"
	name := grokAttachmentFileName(0, att)
	if !strings.HasSuffix(name, ".png") || !strings.Contains(name, "my") {
		t.Fatalf("name=%q", name)
	}
	// base64 payload size sanity for multi-attach
	_ = base64.StdEncoding.EncodeToString([]byte("x"))
}
