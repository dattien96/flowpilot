package runner

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Task-052: image attachment payload shapes + Codex temp-file lifecycle.

func sampleImageAttachment() PromptAttachment {
	return PromptAttachment{
		ID:           "att-1",
		Kind:         "image",
		OriginalName: "shot.png",
		MimeType:     "image/png",
		Data:         base64.StdEncoding.EncodeToString([]byte("PNGDATA")),
		SizeBytes:    7,
		Width:        100,
		Height:       80,
	}
}

func TestCodexTurnStartParamsAppendsImageItems(t *testing.T) {
	params := codexTurnStartParams("thread-1", "hello", nil, []string{"/tmp/a.png", "/tmp/b.webp"})
	input, ok := params["input"].([]any)
	if !ok {
		t.Fatalf("input is not a slice: %T", params["input"])
	}
	if len(input) != 3 {
		t.Fatalf("want text + 2 image items = 3, got %d", len(input))
	}
	if first := input[0].(map[string]any); first["type"] != "text" || first["text"] != "hello" {
		t.Fatalf("first item should be the text prompt, got %#v", first)
	}
	img := input[1].(map[string]any)
	if img["type"] != "localImage" || img["path"] != "/tmp/a.png" {
		t.Fatalf("second item should be the first image path item, got %#v", img)
	}
}

func TestCodexTurnStartParamsNoImagesKeepsTextOnly(t *testing.T) {
	params := codexTurnStartParams("thread-1", "hello", nil, nil)
	input := params["input"].([]any)
	if len(input) != 1 {
		t.Fatalf("no images → only the text item, got %d", len(input))
	}
}

func TestWriteCodexImageAttachmentsWritesAndCleansUp(t *testing.T) {
	atts := []PromptAttachment{sampleImageAttachment()}
	paths, cleanup, err := writeCodexImageAttachments("turn-abc", atts)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("want 1 path, got %d", len(paths))
	}
	raw, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatalf("temp file not readable: %v", err)
	}
	if string(raw) != "PNGDATA" {
		t.Fatalf("decoded bytes mismatch: %q", raw)
	}
	if filepath.Ext(paths[0]) != ".png" {
		t.Fatalf("want .png extension, got %q", paths[0])
	}
	cleanup()
	if _, err := os.Stat(filepath.Dir(paths[0])); !os.IsNotExist(err) {
		t.Fatalf("cleanup did not remove the per-turn dir: %v", err)
	}
}

func TestWriteCodexImageAttachmentsEmptyIsNoop(t *testing.T) {
	paths, cleanup, err := writeCodexImageAttachments("turn-x", nil)
	if err != nil || paths != nil {
		t.Fatalf("empty attachments should be a no-op, got paths=%v err=%v", paths, err)
	}
	cleanup() // must not panic
}

func TestWriteCodexImageAttachmentsBadBase64(t *testing.T) {
	att := sampleImageAttachment()
	att.Data = "!!!not-base64!!!"
	if _, _, err := writeCodexImageAttachments("turn-y", []PromptAttachment{att}); err == nil {
		t.Fatal("expected an error for invalid base64")
	}
}

func TestSweepCodexImageAttachments(t *testing.T) {
	root := codexAttachmentRoot()
	now := time.Now()
	oldDir := filepath.Join(root, "old-turn")
	freshDir := filepath.Join(root, "fresh-turn")
	for _, d := range []string{oldDir, freshDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	t.Cleanup(func() { _ = os.RemoveAll(oldDir); _ = os.RemoveAll(freshDir) })
	// Age the old dir well past the threshold.
	old := now.Add(-2 * time.Hour)
	if err := os.Chtimes(oldDir, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	sweepCodexImageAttachments(time.Hour, now)

	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("old dir should have been swept: %v", err)
	}
	if _, err := os.Stat(freshDir); err != nil {
		t.Fatalf("fresh dir should survive the sweep: %v", err)
	}
}

func TestClaudeWriteUserTurnWithImagesEmitsContentBlocks(t *testing.T) {
	var buf bytes.Buffer
	s := newClaudeStream(&buf)
	if err := s.writeUserTurn("describe this", []PromptAttachment{sampleImageAttachment()}); err != nil {
		t.Fatalf("writeUserTurn: %v", err)
	}
	var msg struct {
		Message struct {
			Content []map[string]any `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, buf.String())
	}
	if len(msg.Message.Content) != 2 {
		t.Fatalf("want text + image blocks = 2, got %d", len(msg.Message.Content))
	}
	if msg.Message.Content[0]["type"] != "text" {
		t.Fatalf("first block should be text, got %#v", msg.Message.Content[0])
	}
	img := msg.Message.Content[1]
	if img["type"] != "image" {
		t.Fatalf("second block should be image, got %#v", img)
	}
	src := img["source"].(map[string]any)
	if src["type"] != "base64" || src["media_type"] != "image/png" {
		t.Fatalf("image source shape wrong: %#v", src)
	}
}

func TestClaudeWriteUserTurnNoImagesUsesPlainString(t *testing.T) {
	var buf bytes.Buffer
	s := newClaudeStream(&buf)
	if err := s.writeUserTurn("just text", nil); err != nil {
		t.Fatalf("writeUserTurn: %v", err)
	}
	var msg struct {
		Message struct {
			Content any `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, isString := msg.Message.Content.(string); !isString {
		t.Fatalf("no-image content should stay a plain string, got %T", msg.Message.Content)
	}
}

func TestTurnBodyDecodesAttachments(t *testing.T) {
	payload := `{"stepId":"s1","prompt":"hi","attachments":[{"id":"a1","kind":"image","originalName":"x.webp","mimeType":"image/webp","data":"AAAA","sizeBytes":3}]}`
	var body turnBody
	if err := json.Unmarshal([]byte(payload), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Attachments) != 1 {
		t.Fatalf("want 1 attachment, got %d", len(body.Attachments))
	}
	if body.Attachments[0].MimeType != "image/webp" || body.Attachments[0].Data != "AAAA" {
		t.Fatalf("attachment decoded wrong: %#v", body.Attachments[0])
	}
}

func TestVisionCapabilityAdvertised(t *testing.T) {
	if !(&claudeAdapter{}).Capabilities().Vision {
		t.Fatal("claude adapter should advertise Vision")
	}
	if !(&codexAdapter{}).Capabilities().Vision {
		t.Fatal("codex adapter should advertise Vision")
	}
}
