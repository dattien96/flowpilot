// image.go — image normalization for PromptAttachment (Task-052 / CP-56).
//
// Rules:
//   - Only PNG and JPEG accepted; other formats are rejected.
//   - PNG output stays PNG; JPEG and any other accepted format outputs JPEG at q80.
//   - Longest edge is capped at 1568 px (Catmull-Rom downscale when larger).
//   - Final encoded bytes must be ≤ 2 MiB.
//   - Only "codex" and "claude" providers support image attachments.
//   - Maximum 6 attachments per turn.
//
// Skill: cli-tui (CP-56)
package client

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/jpeg" // register JPEG decoder
	"image/png"
	_ "image/png" // register PNG decoder
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
)

const (
	maxEdgePx     = 1568
	maxSizeBytes  = 2 * 1024 * 1024 // 2 MiB
	maxAttachments = 6
	jpegQuality   = 80
)

// SupportsImages reports whether providerKey accepts image attachments.
func SupportsImages(providerKey string) bool {
	switch strings.ToLower(providerKey) {
	case "codex", "claude":
		return true
	}
	return false
}

// NormalizeImage decodes imgData, scales it down if any edge exceeds 1568 px,
// re-encodes as PNG (if input was PNG) or JPEG q80 otherwise, and returns a
// PromptAttachment ready to include in TurnInput.Attachments.
//
// Returns an error if:
//   - the image cannot be decoded
//   - the format is not PNG or JPEG
//   - the normalized image exceeds 2 MiB
func NormalizeImage(imgData []byte, fileName string) (*PromptAttachment, error) {
	img, format, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, fmt.Errorf("decode image %q: %w", fileName, err)
	}

	switch format {
	case "png", "jpeg":
		// supported
	default:
		return nil, fmt.Errorf("unsupported image format %q (only png/jpeg accepted)", format)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Scale down if either edge exceeds the cap.
	if w > maxEdgePx || h > maxEdgePx {
		nw, nh := scaledDimensions(w, h, maxEdgePx)
		dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
		img = dst
		w, h = nw, nh
	}

	// Re-encode.
	var outBuf bytes.Buffer
	var mimeType string
	if format == "png" {
		mimeType = "image/png"
		if err := png.Encode(&outBuf, img); err != nil {
			return nil, fmt.Errorf("encode png: %w", err)
		}
	} else {
		mimeType = "image/jpeg"
		if err := jpeg.Encode(&outBuf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return nil, fmt.Errorf("encode jpeg: %w", err)
		}
	}

	outData := outBuf.Bytes()
	if len(outData) > maxSizeBytes {
		return nil, fmt.Errorf("image too large after normalization (%d bytes > %d bytes limit)", len(outData), maxSizeBytes)
	}

	return &PromptAttachment{
		ID:           genAttachmentID(),
		Kind:         "image",
		OriginalName: fileName,
		MimeType:     mimeType,
		Data:         base64.StdEncoding.EncodeToString(outData),
		SizeBytes:    int64(len(outData)),
		Width:        w,
		Height:       h,
	}, nil
}

// ValidateAttachments reads, normalizes, and validates up to 6 image files for
// attachment to a chat turn. Returns an error if the provider does not support
// images, too many files are given, or any image fails normalization.
func ValidateAttachments(paths []string, providerKey string) ([]PromptAttachment, error) {
	if !SupportsImages(providerKey) {
		return nil, fmt.Errorf("provider %q does not support image attachments (codex/claude only)", providerKey)
	}
	if len(paths) > maxAttachments {
		return nil, fmt.Errorf("too many images: %d provided, maximum is %d", len(paths), maxAttachments)
	}

	attachments := make([]PromptAttachment, 0, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		att, err := NormalizeImage(data, filepath.Base(p))
		if err != nil {
			return nil, fmt.Errorf("normalize %s: %w", p, err)
		}
		attachments = append(attachments, *att)
	}
	return attachments, nil
}

// scaledDimensions returns new (w, h) that fit inside a maxEdge×maxEdge box,
// preserving aspect ratio. Returns (w, h) unchanged when both are within limits.
func scaledDimensions(w, h, maxEdge int) (int, int) {
	if w <= maxEdge && h <= maxEdge {
		return w, h
	}
	if w >= h {
		return maxEdge, h * maxEdge / w
	}
	return w * maxEdge / h, maxEdge
}

func genAttachmentID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "att-" + hex.EncodeToString(b)
}
