package runner

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
)

// Task-319: live-proven ACP image block shape (opencode 1.18.25, model
// opencode/mimo-v2.5-free returned "Red" for a red 2x2 PNG sent as
// {type:"image",data:<base64>,mimeType:"image/png"}).

func TestOpencodePromptParamsWithAttachments_appendsImageBlocks(t *testing.T) {
	t.Parallel()

	atts := []PromptAttachment{
		{ID: "att-1", Kind: "image", MimeType: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("pngbytes"))},
		{ID: "att-2", Kind: "image", MimeType: "image/jpeg", Data: base64.StdEncoding.EncodeToString([]byte("jpgbytes"))},
	}
	params := opencodeACPPromptParamsWithAttachments("ses_1", "describe", atts)
	blocks, ok := params["prompt"].([]map[string]string)
	if !ok {
		t.Fatalf("prompt type = %T", params["prompt"])
	}
	if len(blocks) != 3 {
		t.Fatalf("blocks = %d, want text + 2 images", len(blocks))
	}
	if blocks[0]["type"] != "text" || blocks[0]["text"] != "describe" {
		t.Fatalf("text block = %+v", blocks[0])
	}
	if blocks[1]["type"] != "image" || blocks[1]["mimeType"] != "image/png" || !strings.Contains(blocks[1]["data"], "cG5nYnl0ZXM") {
		t.Fatalf("image block 1 = %+v", blocks[1])
	}
	if blocks[2]["type"] != "image" || blocks[2]["mimeType"] != "image/jpeg" {
		t.Fatalf("image block 2 = %+v", blocks[2])
	}
}

func TestOpencodePromptParamsWithAttachments_skipsNonImageAndEmpty(t *testing.T) {
	t.Parallel()

	atts := []PromptAttachment{
		{ID: "att-1", Kind: "file", MimeType: "text/plain", Data: "zzz"},
		{ID: "att-2", Kind: "image", MimeType: "image/png", Data: "   "},
	}
	params := opencodeACPPromptParamsWithAttachments("ses_1", "hello", atts)
	blocks := params["prompt"].([]map[string]string)
	if len(blocks) != 1 || blocks[0]["type"] != "text" {
		t.Fatalf("non-image/empty attachments must be skipped, got %+v", blocks)
	}
}

func TestOpencodeTurnImageAttachments_gatedByModelCapability(t *testing.T) {
	t.TempDir()
	cachePath := filepath.Join(t.TempDir(), "opencode_models_cache.json")
	t.Setenv("FLOWPILOT_OPENCODE_MODELS_CACHE_PATH", cachePath)

	writeOpencodeModelsCache([]ProviderModel{
		{ID: "opencode/big-pickle", Source: "opencode_models", Available: true},
		{ID: "opencode/mimo-v2.5-free", Source: "opencode_models", Available: true, InputImage: true},
	})

	atts := []PromptAttachment{{ID: "att-1", Kind: "image", MimeType: "image/png", Data: "abc"}}
	if got := opencodeTurnImageAttachments(TurnRequest{ModelName: "opencode/big-pickle", Attachments: atts}); got != nil {
		t.Fatalf("non-vision model must not receive attachments, got %+v", got)
	}
	got := opencodeTurnImageAttachments(TurnRequest{ModelName: "opencode/mimo-v2.5-free", Attachments: atts})
	if len(got) != 1 {
		t.Fatalf("vision model must receive attachments, got %+v", got)
	}
	if got := opencodeTurnImageAttachments(TurnRequest{ModelName: "opencode/unknown-model", Attachments: atts}); got != nil {
		t.Fatalf("unknown model must be conservative (nil), got %+v", got)
	}
	if got := opencodeTurnImageAttachments(TurnRequest{ModelName: "", Attachments: atts}); got != nil {
		t.Fatalf("empty model must be conservative (nil), got %+v", got)
	}
}

func TestOpencodeModelSupportsImages_caseInsensitive(t *testing.T) {
	t.TempDir()
	cachePath := filepath.Join(t.TempDir(), "opencode_models_cache.json")
	t.Setenv("FLOWPILOT_OPENCODE_MODELS_CACHE_PATH", cachePath)
	writeOpencodeModelsCache([]ProviderModel{{ID: "opencode/Mimo-V2.5-Free", Source: "opencode_models", Available: true, InputImage: true}})

	if !opencodeModelSupportsImages("opencode/mimo-v2.5-free") {
		t.Fatal("case-insensitive model lookup failed")
	}
}
