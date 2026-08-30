package app

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"golang.design/x/clipboard"
)

// clipboardPSTimeout bounds every PowerShell clipboard read so a locked
// clipboard can never block the TUI forever (CA-541: composer freeze).
const clipboardPSTimeout = 3 * time.Second

// clipboardNativeTimeout bounds every native golang.design/x/clipboard read
// so Alt+V cannot hang the TUI even when the native API deadlocks (CA-615:
// log 10:05:40 — gõ tay ok, Alt+V treo cả process). Windows paste already
// avoids native entirely; Darwin/Linux keep a short fallback bounded here.
const clipboardNativeTimeout = 2 * time.Second

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
	return m.cmdClipboardPasteWithFallback("")
}

// cmdAttachImagePath attaches a copied image *file* whose path was pasted as
// text (Explorer "Copy" of an image, or a bracketed paste whose runes are a
// path). It reads the file directly from disk — no clipboard round-trip, so a
// locked/hung system clipboard can never freeze the composer (CA-541).
func (m *AppModel) cmdAttachImagePath(path string) tea.Cmd {
	provider := m.provider
	modelSupports := m.chatSupportsImages()
	return func() tea.Msg {
		atts, err := client.ValidateAttachmentsForModel([]string{path}, provider, modelSupports)
		if err != nil {
			return ClipboardPasteMsg{Err: err.Error()}
		}
		if len(atts) == 0 {
			return ClipboardPasteMsg{Err: "could not attach image path from paste"}
		}
		return ClipboardPasteMsg{Attachment: &atts[0]}
	}
}

// cmdClipboardPasteWithFallback reads the system clipboard. Text wins so a
// message copied from another app pastes straight into the composer even when
// the clipboard also carries an image format (rich-text apps put both). A
// clipboard image is attached only when there is no text; explicit image paste
// stays on Alt+V / ctrl+shift+v / /image paste. When neither image nor text is
// present, fallbackText from bracketed paste (Windows Terminal Ctrl+V) is
// inserted so text paste still works if the native clipboard read fails.
func (m *AppModel) cmdClipboardPasteWithFallback(fallbackText string) tea.Cmd {
	provider := m.provider
	modelSupports := m.chatSupportsImages()
	unsupportedReason := m.imagesUnsupportedReason()
	pending := len(m.pendingAttach)
	return func() tea.Msg {
		text := readClipboardText()
		if path := imagePathFromClipboardText(text); path != "" {
			atts, err := client.ValidateAttachmentsForModel([]string{path}, provider, modelSupports)
			if err != nil {
				return ClipboardPasteMsg{Err: err.Error()}
			}
			if len(atts) == 0 {
				return ClipboardPasteMsg{Err: "could not attach image path from clipboard"}
			}
			return ClipboardPasteMsg{Attachment: &atts[0]}
		}
		if strings.TrimSpace(text) != "" {
			return ClipboardPasteMsg{Text: text, NoImage: true}
		}
		if path := imagePathFromClipboardText(fallbackText); path != "" {
			atts, err := client.ValidateAttachmentsForModel([]string{path}, provider, modelSupports)
			if err != nil {
				return ClipboardPasteMsg{Err: err.Error()}
			}
			if len(atts) == 0 {
				return ClipboardPasteMsg{Err: "could not attach image path from paste"}
			}
			return ClipboardPasteMsg{Attachment: &atts[0]}
		}
		if strings.TrimSpace(fallbackText) != "" {
			return ClipboardPasteMsg{Text: fallbackText, NoImage: true}
		}
		img, src, imgErr := readClipboardImageBytes()
		if len(img) > 0 {
			if !client.SupportsImages(provider) && !modelSupports {
				return ClipboardPasteMsg{Err: unsupportedReason}
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
		err := "clipboard has no text and no image — copy a message or image, then Ctrl+V / Alt+V or /image paste"
		if imgErr != "" {
			err = imgErr + " — " + err
		}
		return ClipboardPasteMsg{NoImage: true, Err: err}
	}
}

// readWithTimeout bounds a blocking clipboard.Read so it cannot hang the TUI
// forever. It is testable without touching the real system clipboard (CA-615).
func readWithTimeout[T any](d time.Duration, fn func() T) (T, bool) {
	ch := make(chan T, 1)
	go func() { ch <- fn() }()
	select {
	case v := <-ch:
		return v, true
	case <-time.After(d):
		var zero T
		return zero, false
	}
}

// readClipboardImageBytes tries native clipboard image, then Windows CF_HDROP /
// System.Windows.Forms fallbacks (Explorer "Copy" of a file is not FmtImage).
// Windows paste is PS-only so native Read never runs there (CA-615 deadlock).
func readClipboardImageBytes() (data []byte, fileName string, note string) {
	if runtime.GOOS != "windows" {
		if initClipboard() {
			if img, ok := readWithTimeout(clipboardNativeTimeout, func() []byte { return clipboard.Read(clipboard.FmtImage) }); ok && len(img) > 0 {
				return img, "clipboard.png", ""
			} else if !ok && note == "" {
				note = "clipboard image read timed out"
			}
		} else if note == "" {
			note = "native clipboard init failed"
		}
	} else if note == "" {
		// Windows path is PS-only; native Read is skipped to avoid deadlock.
		note = ""
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
	if runtime.GOOS == "windows" {
		if t, err := readWindowsClipboardTextPS(); err == nil {
			if strings.TrimSpace(t) != "" {
				return t
			}
			return ""
		}
		return ""
	}
	if initClipboard() {
		if t, ok := readWithTimeout(clipboardNativeTimeout, func() string { return string(clipboard.Read(clipboard.FmtText)) }); ok && strings.TrimSpace(t) != "" {
			return t
		}
	}
	if t, err := readWindowsClipboardTextPS(); err == nil && strings.TrimSpace(t) != "" {
		return t
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
	out, err := runClipboardPS(script)
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
	out, err := runClipboardPS(script)
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
	out, err := runClipboardPS(script)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// runClipboardPS runs a PowerShell clipboard script with a hard timeout so a
// clipboard locked by another process returns an error instead of hanging the
// TUI (CA-541). Returns exec.ErrDeadlineExceeded wrapped in the exec error.
func runClipboardPS(script string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), clipboardPSTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	applyClipboardSysProcAttr(cmd)
	return cmd.Output()
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

func writeClipboardText(s string) error {
	// NUL/control bytes truncate the write: on Windows the text is encoded to
	// UTF-16 and a leading 0x0000 becomes the string terminator, so the
	// clipboard reads back EMPTY even though the app "copied" the full text
	// (run-117747: a prompt pasted with a leading \u0000 copied as nothing).
	s = sanitizePasteText(s)
	if initClipboard() {
		clipboard.Write(clipboard.FmtText, []byte(s))
		return nil
	}
	if runtime.GOOS == "windows" {
		return writeWindowsClipboardTextPS(s)
	}
	return fmt.Errorf("clipboard unavailable")
}

func writeWindowsClipboardTextPS(s string) error {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", "Set-Clipboard -Value $input")
	cmd.Stdin = strings.NewReader(s)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("Set-Clipboard: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func imagePathFromClipboardText(text string) string {
	p := strings.TrimSpace(text)
	p = strings.Trim(p, `"'`)
	if p == "" {
		return ""
	}
	if strings.ContainsAny(p, "\r\n") {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(p))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return ""
	}
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

func formatPendingAttachments(atts []client.PromptAttachment) string {
	if len(atts) == 0 {
		return "No pending images. Use Alt+V or /image paste, or /image <path>."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Pending images (%d/6):\n", len(atts)))
	for i, a := range atts {
		sb.WriteString(fmt.Sprintf("  %d. %s  %s  %dx%d  %d bytes\n",
			i+1, a.OriginalName, a.MimeType, a.Width, a.Height, a.SizeBytes))
	}
	sb.WriteString("Manage: click [N img] · Remove: /image rm <n> or click [x] · Open: /image open <n> · Clear: /image clear")
	return sb.String()
}
