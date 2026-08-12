package app

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"golang.design/x/clipboard"
)

var clipboardOnce sync.Once
var clipboardOK bool

func initClipboard() bool {
	clipboardOnce.Do(func() {
		if err := clipboard.Init(); err == nil {
			clipboardOK = true
		}
	})
	return clipboardOK
}

// ClipboardPasteMsg carries Ctrl+V / paste results (image and/or text).
type ClipboardPasteMsg struct {
	Attachment *client.PromptAttachment
	Text       string
	Err        string
	NoImage    bool
}

// AttachmentOpenMsg reports /image open results.
type AttachmentOpenMsg struct {
	Path string
	Err  string
}

func (m *AppModel) cmdClipboardPaste() tea.Cmd {
	provider := m.provider
	pending := len(m.pendingAttach)
	return func() tea.Msg {
		img, src, imgErr := readClipboardImageBytes()
		if len(img) > 0 {
			if !client.SupportsImages(provider) {
				return ClipboardPasteMsg{Err: client.ImagesUnsupportedReason(provider)}
			}
			if pending >= 6 {
				return ClipboardPasteMsg{Err: "Maximum 6 images per turn."}
			}
			name := "clipboard.png"
			if src != "" {
				name = src
			}
			att, err := client.NormalizeImage(img, name)
			if err != nil {
				return ClipboardPasteMsg{Err: err.Error()}
			}
			return ClipboardPasteMsg{Attachment: att}
		}
		text := readClipboardText()
		if strings.TrimSpace(text) != "" {
			msg := ClipboardPasteMsg{Text: text, NoImage: true}
			if imgErr != "" {
				msg.Err = imgErr
			}
			return msg
		}
		err := "clipboard has no image (and no text) — copy an image or image file, then Alt+V or /image paste"
		if imgErr != "" {
			err = imgErr + " — " + err
		}
		return ClipboardPasteMsg{NoImage: true, Err: err}
	}
}

// readClipboardImageBytes tries native clipboard image, then Windows CF_HDROP /
// System.Windows.Forms fallbacks (Explorer "Copy" of a file is not FmtImage).
func readClipboardImageBytes() (data []byte, fileName string, note string) {
	if initClipboard() {
		if img := clipboard.Read(clipboard.FmtImage); len(img) > 0 {
			return img, "clipboard.png", ""
		}
	} else {
		note = "native clipboard init failed"
	}
	if runtime.GOOS == "windows" {
		if img, name, err := readWindowsClipboardImagePS(); err == nil && len(img) > 0 {
			return img, name, ""
		} else if err != nil && note == "" {
			note = err.Error()
		}
		if img, name, err := readWindowsClipboardFileDropPS(); err == nil && len(img) > 0 {
			return img, name, ""
		} else if err != nil && note == "" {
			note = err.Error()
		}
	}
	return nil, "", note
}

func readClipboardText() string {
	if initClipboard() {
		if t := string(clipboard.Read(clipboard.FmtText)); strings.TrimSpace(t) != "" {
			return t
		}
	}
	if runtime.GOOS == "windows" {
		if t, err := readWindowsClipboardTextPS(); err == nil {
			return t
		}
	}
	return ""
}

func readWindowsClipboardImagePS() ([]byte, string, error) {
	script := `
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$img = [System.Windows.Forms.Clipboard]::GetImage()
if ($null -eq $img) { exit 2 }
$ms = New-Object System.IO.MemoryStream
$img.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
[Convert]::ToBase64String($ms.ToArray())
`
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return nil, "", fmt.Errorf("windows GetImage: %w", err)
	}
	b64 := strings.TrimSpace(string(out))
	if b64 == "" {
		return nil, "", fmt.Errorf("windows GetImage empty")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, "", err
	}
	return raw, "clipboard.png", nil
}

func readWindowsClipboardFileDropPS() ([]byte, string, error) {
	script := `
Add-Type -AssemblyName System.Windows.Forms
$files = [System.Windows.Forms.Clipboard]::GetFileDropList()
if ($null -eq $files -or $files.Count -lt 1) { exit 2 }
$p = $files[0]
$ext = [IO.Path]::GetExtension($p).ToLowerInvariant()
if ($ext -notin @('.png','.jpg','.jpeg')) { exit 3 }
Write-Output $p
`
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return nil, "", fmt.Errorf("windows file-drop: %w", err)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return nil, "", fmt.Errorf("windows file-drop empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	return raw, filepath.Base(path), nil
}

func readWindowsClipboardTextPS() (string, error) {
	script := `
Add-Type -AssemblyName System.Windows.Forms
$t = [System.Windows.Forms.Clipboard]::GetText()
if ($null -eq $t) { exit 2 }
Write-Output $t
`
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (m *AppModel) cmdOpenPendingAttachment(index int) tea.Cmd {
	atts := append([]client.PromptAttachment(nil), m.pendingAttach...)
	return func() tea.Msg {
		if index < 1 || index > len(atts) {
			return AttachmentOpenMsg{Err: fmt.Sprintf("image index %d out of range (1-%d)", index, len(atts))}
		}
		att := atts[index-1]
		raw, err := decodeAttachmentData(att.Data)
		if err != nil {
			return AttachmentOpenMsg{Err: err.Error()}
		}
		ext := ".png"
		if strings.Contains(att.MimeType, "jpeg") {
			ext = ".jpg"
		}
		name := strings.TrimSpace(att.OriginalName)
		if name == "" {
			name = fmt.Sprintf("attach-%d%s", index, ext)
		}
		dir := filepath.Join(os.TempDir(), "flowpilot-tui-attach")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return AttachmentOpenMsg{Err: err.Error()}
		}
		path := filepath.Join(dir, filepath.Base(name))
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			return AttachmentOpenMsg{Err: err.Error()}
		}
		if err := openPath(path); err != nil {
			return AttachmentOpenMsg{Path: path, Err: err.Error()}
		}
		return AttachmentOpenMsg{Path: path}
	}
}

func decodeAttachmentData(b64 string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(b64)
}

func openPath(path string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("cmd", "/c", "start", "", path).Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}

func formatPendingAttachments(atts []client.PromptAttachment) string {
	if len(atts) == 0 {
		return "No pending images. Use Alt+V or /image paste (codex/claude), or /image <path>."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Pending images (%d/6):\n", len(atts)))
	for i, a := range atts {
		sb.WriteString(fmt.Sprintf("  %d. %s  %s  %dx%d  %d bytes\n",
			i+1, a.OriginalName, a.MimeType, a.Width, a.Height, a.SizeBytes))
	}
	sb.WriteString("Open: /image open <n>  · Clear: /image clear")
	return sb.String()
}
