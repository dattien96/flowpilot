package runner

import "testing"

// Task-319: `opencode models --verbose` emits an ID line followed by a JSON
// metadata blob per model. The blobs below mirror the live 1.18.25 output
// shape (models.dev capabilities.input.image).
const opencodeVerboseFixture = `opencode/big-pickle
{
  "id": "big-pickle",
  "providerID": "opencode",
  "name": "Big Pickle",
  "capabilities": {
    "temperature": true,
    "reasoning": true,
    "attachment": false,
    "toolcall": true,
    "input": {
      "text": true,
      "audio": false,
      "image": false,
      "video": false,
      "pdf": false
    }
  },
  "limit": { "context": 200000 }
}
opencode/mimo-v2.5-free
{
  "id": "mimo-v2.5-free",
  "providerID": "opencode",
  "name": "Mimo 2.5 Free",
  "capabilities": {
    "temperature": true,
    "reasoning": true,
    "attachment": true,
    "toolcall": true,
    "input": {
      "text": true,
      "audio": false,
      "image": true,
      "video": false,
      "pdf": false
    }
  },
  "limit": { "context": 262144 }
}
opencode-go/hy3
{
  "id": "hy3",
  "providerID": "opencode-go",
  "name": "Hy3",
  "capabilities": {
    "input": { "text": true, "image": false }
  }
}
`

func TestParseOpencodeVerboseModelsOutput_capturesImageCapability(t *testing.T) {
	t.Parallel()

	models, ok := parseOpencodeVerboseModelsOutput([]byte(opencodeVerboseFixture))
	if !ok {
		t.Fatal("verbose parse returned ok=false for verbose-shaped output")
	}
	if len(models) != 3 {
		t.Fatalf("models = %d, want 3", len(models))
	}
	byID := map[string]ProviderModel{}
	for _, m := range models {
		byID[m.ID] = m
	}
	if byID["opencode/big-pickle"].InputImage {
		t.Fatalf("big-pickle InputImage = true, want false")
	}
	if !byID["opencode/mimo-v2.5-free"].InputImage {
		t.Fatalf("mimo-v2.5-free InputImage = false, want true")
	}
	if byID["opencode-go/hy3"].InputImage {
		t.Fatalf("hy3 InputImage = true, want false")
	}
	for _, m := range models {
		if m.DisplayName == "" || m.Source != "opencode_models" || !m.Available {
			t.Fatalf("model metadata lost: %+v", m)
		}
	}
}

func TestParseOpencodeVerboseModelsOutput_plainLinesFallBackToFalse(t *testing.T) {
	t.Parallel()

	// The plain `opencode models` shape (no JSON blobs) is not verbose — the
	// verbose parser must report ok=false so the dispatcher uses the plain
	// parser and the existing behavior stays unchanged.
	if _, ok := parseOpencodeVerboseModelsOutput([]byte("opencode/gpt-5.4\nopencode-go/gpt-5.4\n")); ok {
		t.Fatal("verbose parse accepted plain line output, want ok=false")
	}
	// But the dispatcher still returns models via the plain fallback.
	models, err := parseOpencodeModelsOutput([]byte("opencode/gpt-5.4\nopencode-go/gpt-5.4\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("dispatcher models = %d, want 2", len(models))
	}
	for _, m := range models {
		if m.InputImage {
			t.Fatalf("plain output must not claim InputImage: %+v", m)
		}
	}
}

func TestParseOpencodeVerboseModelsOutput_interleavedGarbageStaysNonVerbose(t *testing.T) {
	t.Parallel()

	// An ID line followed by another ID line (no JSON blob) is not the verbose
	// shape — the dispatcher must fall back instead of failing.
	out := "opencode/big-pickle\nopencode/mimo-v2.5-free\n"
	if _, ok := parseOpencodeVerboseModelsOutput([]byte(out)); ok {
		t.Fatal("verbose parse accepted ID-only output, want ok=false")
	}
}
