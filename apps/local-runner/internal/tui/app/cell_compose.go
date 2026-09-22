package app

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
)

// composeCellBuf paints the chat pane into the left chatW cells and the right
// sidebar into the right sideW cells of one terminal cell buffer, then renders
// the grid. Placement is per-cell and clipped by rect — a styled You row can
// never push the sidebar into the chat column because the sidebar cells are
// written at fixed x >= chatW (CA-599). Ghostty never sees a concatenated
// "chat+side" line.
func composeCellBuf(chat, side string, fullW, chatW, sideW, h int) string {
	if fullW < 1 {
		fullW = 1
	}
	if chatW < 1 {
		chatW = 1
	}
	if sideW < 1 {
		sideW = 1
	}
	// Buffer is one column narrower than the terminal (CA-584) so macOS
	// autowrap never triggers; chat+side cells exactly fill it.
	bufW := safeTermWidth(fullW)
	if side != "" && chatW+sideW > bufW {
		bufW = chatW + sideW
	}
	buf := cellbuf.NewBuffer(bufW, h)
	canvas := cellbuf.NewCell(' ')
	canvas.Style.Bg = ansi.HexColor(colorCanvas)
	cellbuf.Fill(buf, canvas)
	if chat != "" {
		cellbuf.SetContentRect(buf, chat, cellbuf.Rect(0, 0, chatW, h))
	}
	if side != "" {
		cellbuf.SetContentRect(buf, side, cellbuf.Rect(chatW, 0, sideW, h))
	}
	return renderGrid(buf)
}

// renderGrid serializes the buffer row by row without trimming trailing space
// cells — cellbuf.Render trims them, which removes the gutter between the chat
// column and the sidebar and shifts the sidebar one column left (the recurring
// "1-col bleed"). Every row is exactly the buffer width.
func renderGrid(buf *cellbuf.Buffer) string {
	var sb strings.Builder
	var pen cellbuf.Style
	h, w := buf.Height(), buf.Width()
	for y := 0; y < h; y++ {
		if y > 0 {
			sb.WriteByte('\n')
		}
		lineW := 0
		for x := 0; x < w; x++ {
			c := buf.Cell(x, y)
			if c == nil || c.Width == 0 || c.Rune == 0 {
				// Blank or continuation cell of a wide glyph: emit a space
				// (same style as pen so the bg fills the row).
				if pen.Empty() {
					sb.WriteByte(' ')
				} else {
					sb.WriteString(pen.Sequence())
					sb.WriteByte(' ')
				}
				lineW++
				continue
			}
			if !c.Style.Equal(&pen) {
				sb.WriteString(c.Style.DiffSequence(pen))
				pen = c.Style
			}
			sb.WriteRune(c.Rune)
			lineW += c.Width
			// Continuation cells of a wide rune occupy width without runes;
			// advance past them.
			for k := 1; k < c.Width; k++ {
				x++
			}
		}
		if !pen.Empty() {
			sb.WriteString(ansi.ResetStyle)
			pen = cellbuf.Style{}
		}
		_ = lineW
	}
	return sb.String()
}
