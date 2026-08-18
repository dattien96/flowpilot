package app

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

// writeTestPNG writes a valid 1x1 PNG to path and returns it.
func writeTestPNG(t *testing.T, path string) {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

// CA-541: the composer freeze. Bracketed paste (KeyRunes+Paste, delivered by
// Windows Terminal on Ctrl+V) used to route EVERY keystroke through the system
// clipboard (native clipboard.Read or PowerShell GetText). A locked clipboard
// blocked forever, so typed characters never appeared while F2/F4 (handled in
// earlier switch cases) still worked. Plain paste text now inserts directly;
// the clipboard is only consulted when the paste is empty (image-only) or is a
// copied image file path. Provider-agnostic logic, parameterized over all three
// providers.
func TestBracketedPaste_PlainTextNeverReadsClipboard(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:9")
			m.provider = pk
			m.sessionLoading = false
			m.authPhase = AuthNone
			m.inputValue = "hi "

			m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("multi-line\npaste body"), Paste: true})
			am := m2.(*AppModel)
			if am.inputValue != "hi multi-line\npaste body" {
				t.Fatalf("%s: multi-line paste must insert directly, got %q", pk, am.inputValue)
			}
		})
	}
}

// A single printable rune with the stuck paste flag must still insert — this is
// the exact stuck-paste scenario where every keystroke arrives Paste:true.
func TestBracketedPaste_SingleRuneStuckPasteInserts(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:9")
			m.provider = pk
			m.sessionLoading = false
			m.authPhase = AuthNone
			m.inputValue = ""

			for _, r := range "hello" {
				m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Paste: true})
				m = m2.(*AppModel)
			}
			if m.inputValue != "hello" {
				t.Fatalf("%s: stuck-paste keystrokes must insert chars, got %q", pk, m.inputValue)
			}
		})
	}
}

// An empty/whitespace bracketed paste can only be an image-only clipboard
// (no text to deliver) — that is the one case that still reads the clipboard.
func TestBracketedPaste_EmptyRoutesClipboard(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:9")
			m.provider = pk
			m.sessionLoading = false
			m.authPhase = AuthNone
			m.inputValue = "pre"

			m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: nil, Paste: true})
			am := m2.(*AppModel)
			if am.inputValue != "pre" {
				t.Fatalf("%s: empty paste must not insert; input=%q", pk, am.inputValue)
			}
			if cmd == nil {
				t.Fatalf("%s: empty paste must consult the clipboard (image-only), got no cmd", pk)
			}
		})
	}
}

func TestBracketedPaste_ImagePathAttachesFromDisk(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			png := filepath.Join(t.TempDir(), "clip.png")
			writeTestPNG(t, png)
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:9")
			m.provider = pk
			m.sessionLoading = false
			m.authPhase = AuthNone
			m.inputValue = "pre"

			m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(png), Paste: true})
			am := m2.(*AppModel)
			if am.inputValue != "pre" {
				t.Fatalf("%s: image path paste must not insert path text; input=%q", pk, am.inputValue)
			}
			if cmd == nil {
				t.Fatalf("%s: image path paste must attach, got no cmd", pk)
			}
			msg := cmd()
			pm, ok := msg.(ClipboardPasteMsg)
			if !ok {
				t.Fatalf("%s: msg type %T", pk, msg)
			}
			if pm.Attachment == nil {
				t.Fatalf("%s: expected attachment for %s, got %+v", pk, png, pm)
			}
			if pm.Attachment.OriginalName != "clip.png" {
				t.Fatalf("%s: attachment name=%q want clip.png", pk, pm.Attachment.OriginalName)
			}
		})
	}
}

// cmdAttachImagePath reads the file straight from disk — it must succeed even
// when the system clipboard is empty/garbage, i.e. it never depends on it.
func TestCmdAttachImagePath_ReadsFromDiskNoClipboard(t *testing.T) {
	png := filepath.Join(t.TempDir(), "disk.png")
	writeTestPNG(t, png)
	m := New(config.ChatConfig{Provider: "claude"}, "http://127.0.0.1:9")
	m.provider = "claude"
	cmd := m.cmdAttachImagePath(png)
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	msg := cmd()
	pm, ok := msg.(ClipboardPasteMsg)
	if !ok {
		t.Fatalf("msg type %T", msg)
	}
	if pm.Attachment == nil || pm.Err != "" {
		t.Fatalf("expected clean attachment, got %+v", pm)
	}
}

// A non-image path pasted as text (e.g. a code file path or URL) must insert
// as plain text, not attempt an attachment.
func TestBracketedPaste_NonImagePathInsertsAsText(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.provider = "codex"
	m.sessionLoading = false
	m.authPhase = AuthNone
	m.inputValue = ""

	// Real file, wrong extension — must NOT attach, must insert text.
	p := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(p), Paste: true})
	am := m2.(*AppModel)
	if am.inputValue != p {
		t.Fatalf("non-image path must insert as text, got %q", am.inputValue)
	}
}

// Explicit Ctrl+V and Alt+V still go through the clipboard (image attach first,
// text fallback) — the freeze fix must not change the explicit-paste contract.
func TestExplicitPaste_StillDispatchesClipboardCmd(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.provider = "codex"
	m.sessionLoading = false
	m.authPhase = AuthNone

	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyCtrlV},
		{Type: tea.KeyRunes, Runes: []rune{'v'}, Alt: true},
	} {
		_, cmd := m.handleKey(k)
		if cmd == nil {
			t.Fatalf("explicit paste %q must dispatch clipboard cmd", k.String())
		}
	}
}

// imagePathFromClipboardText guards the attach branch: only single-line paths
// with an image extension that exist on disk are treated as images.
func TestImagePathFromClipboardText_Guards(t *testing.T) {
	png := filepath.Join(t.TempDir(), "a.png")
	writeTestPNG(t, png)
	if got := imagePathFromClipboardText(png); got != png {
		t.Fatalf("existing png path must be detected, got %q", got)
	}
	if got := imagePathFromClipboardText(filepath.Join(t.TempDir(), "nope.png")); got != "" {
		t.Fatalf("non-existent path must not attach, got %q", got)
	}
	if got := imagePathFromClipboardText("hello.png\nsecond line"); got != "" {
		t.Fatalf("multi-line must not be treated as a path, got %q", got)
	}
	if got := imagePathFromClipboardText(strings.ReplaceAll(png, ".png", ".txt")); got != "" {
		t.Fatalf("non-image extension must not attach, got %q", got)
	}
}
