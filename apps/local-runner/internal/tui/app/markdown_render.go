package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

func renderMarkdownFresh(src string, width int, ascii bool) (out []mdLine) {
	defer func() {
		if rec := recover(); rec != nil {
			out = textsToMD(wrapText(src, width))
		}
	}()
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	reader := text.NewReader([]byte(src))
	doc := md.Parser().Parse(reader)
	w := &mdWriter{src: []byte(src), width: width, ascii: ascii, copyAt: map[int]string{}}
	_ = ast.Walk(doc, w.walk)
	w.flushPara()
	if len(w.lines) == 0 {
		return textsToMD(wrapText(src, width))
	}
	return packMDLines(w.lines, w.copyAt)
}

type listState struct {
	ordered bool
	next    int
}

type mdWriter struct {
	src     []byte
	width   int
	ascii   bool
	lines   []string
	copyAt  map[int]string
	buf     strings.Builder
	bold    int
	italic  int
	code    int
	quote   int
	lists   []listState
	itemPad string
}

func (w *mdWriter) walk(n ast.Node, entering bool) (ast.WalkStatus, error) {
	switch n.Kind() {
	case ast.KindHeading:
		return w.walkHeading(n, entering)
	case ast.KindParagraph:
		return w.walkParagraph(n, entering)
	case ast.KindList:
		return w.walkList(n, entering)
	case ast.KindListItem:
		return w.walkListItem(n, entering)
	case ast.KindBlockquote:
		return w.walkQuote(entering)
	case ast.KindFencedCodeBlock, ast.KindCodeBlock:
		return w.walkCode(n, entering)
	case ast.KindThematicBreak:
		if entering {
			w.flushPara()
			w.lines = append(w.lines, w.hrLine())
			w.blank()
		}
	case ast.KindEmphasis:
		w.walkEmphasis(n, entering)
	case ast.KindCodeSpan:
		if entering {
			w.writeCodeSpan(n)
			return ast.WalkSkipChildren, nil
		}
	case ast.KindLink, ast.KindAutoLink:
		return w.walkLink(n, entering)
	case ast.KindText:
		if entering {
			w.writeTextNode(n.(*ast.Text))
		}
	case ast.KindString:
		if entering {
			w.writeInline(string(n.(*ast.String).Value))
		}
	case east.KindStrikethrough:
		// text children still render; no extra chrome
	case east.KindTable:
		if entering {
			w.flushPara()
			w.lines = append(w.lines, w.renderTable(n)...)
			w.blank()
			return ast.WalkSkipChildren, nil
		}
	}
	return ast.WalkContinue, nil
}

func (w *mdWriter) walkHeading(n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		w.flushPara()
		return ast.WalkContinue, nil
	}
	text := w.takeBuf()
	for _, line := range wrapStyled(styleMdH.Render(text), w.width) {
		w.lines = append(w.lines, line)
	}
	w.blank()
	return ast.WalkContinue, nil
}

func (w *mdWriter) walkParagraph(_ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		return ast.WalkContinue, nil
	}
	if w.inList() {
		return ast.WalkContinue, nil
	}
	w.flushPara()
	return ast.WalkContinue, nil
}

func (w *mdWriter) walkList(n ast.Node, entering bool) (ast.WalkStatus, error) {
	list := n.(*ast.List)
	if entering {
		start := list.Start
		if start < 1 {
			start = 1
		}
		w.lists = append(w.lists, listState{ordered: list.IsOrdered(), next: start})
		return ast.WalkContinue, nil
	}
	if len(w.lists) > 0 {
		w.lists = w.lists[:len(w.lists)-1]
	}
	w.blank()
	return ast.WalkContinue, nil
}

func (w *mdWriter) walkListItem(_ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		w.itemPad = w.nextBullet()
		return ast.WalkContinue, nil
	}
	text := strings.TrimSpace(w.takeBuf())
	prefix := w.itemPad
	w.itemPad = ""
	wrapped := wrapStyled(prefix+text, w.width)
	pad := strings.Repeat(" ", len([]rune(prefix)))
	for i, line := range wrapped {
		if i > 0 && pad != "" {
			line = pad + strings.TrimLeft(stripANSI(line), " ")
		}
		w.lines = append(w.lines, line)
	}
	return ast.WalkContinue, nil
}

func (w *mdWriter) walkQuote(entering bool) (ast.WalkStatus, error) {
	if entering {
		w.quote++
		return ast.WalkContinue, nil
	}
	w.flushPara()
	w.quote--
	return ast.WalkContinue, nil
}

func (w *mdWriter) walkCode(n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	w.flushPara()
	lang := ""
	if fc, ok := n.(*ast.FencedCodeBlock); ok {
		lang = strings.TrimSpace(string(fc.Language(w.src)))
	}
	title := lang
	if title == "" {
		title = "code"
	}
	raw := w.blockText(n)
	body := boundFenceBody(raw)
	boxed := renderCodeFenceBox(strings.Split(body, "\n"), title, w.width, w.ascii)
	if w.copyAt == nil {
		w.copyAt = map[int]string{}
	}
	if len(boxed) > 0 {
		w.copyAt[len(w.lines)] = raw
	}
	w.lines = append(w.lines, boxed...)
	w.blank()
	return ast.WalkSkipChildren, nil
}

func renderCodeFenceBox(body []string, title string, width int, ascii bool) []string {
	if width < 12 {
		width = 12
	}
	chip := styleLink.Render(copyChip)
	innerW := 4
	for _, line := range body {
		if w := len([]rune(line)); w > innerW {
			innerW = w
		}
	}
	tlen := lipgloss.Width(" "+title+" ") + lipgloss.Width(chip)
	if tlen > innerW {
		innerW = tlen
	}
	boxW := innerW + 4
	if boxW > width {
		boxW = width
	}
	if boxW < 10 {
		boxW = 10
		if boxW > width {
			boxW = width
		}
	}
	padW := boxW - 2
	out := make([]string, 0, len(body)+2)
	out = append(out, strokeTopChip(title, chip, boxW, ascii))
	if len(body) == 0 {
		body = []string{""}
	}
	for _, line := range body {
		for _, wrapped := range wrapText(line, padW-1) {
			inner := padVisualANSI(" "+wrapped, padW)
			out = append(out, strokeCodeFill(inner, boxW, ascii))
		}
	}
	out = append(out, strokeBottom(boxW, ascii))
	return out
}

func packMDLines(lines []string, copyAt map[int]string) []mdLine {
	out := make([]mdLine, len(lines))
	for i, s := range lines {
		out[i] = mdLine{Text: s}
		if copyAt != nil {
			out[i].CopyCode = copyAt[i]
		}
	}
	return out
}

func strokeCodeFill(inner string, boxW int, ascii bool) string {
	_, _, _, _, _, v := boxGlyphs(ascii)
	innerW := boxW - 2
	if innerW < 4 {
		innerW = 4
	}
	return v + styleMdCodeBox.Render(padVisualANSI(inner, innerW)) + v
}

func (w *mdWriter) walkEmphasis(n ast.Node, entering bool) {
	level := 1
	if em, ok := n.(*ast.Emphasis); ok {
		level = em.Level
	}
	delta := 1
	if !entering {
		delta = -1
	}
	if level >= 2 {
		w.bold += delta
		return
	}
	w.italic += delta
}

func (w *mdWriter) walkLink(n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		dest := ""
		switch v := n.(type) {
		case *ast.Link:
			dest = string(v.Destination)
		case *ast.AutoLink:
			dest = string(v.URL(w.src))
		}
		if dest != "" && !strings.EqualFold(strings.TrimSpace(w.peekPlain()), dest) {
			w.buf.WriteString(styleMdDim.Render(" " + dest))
		}
		return ast.WalkContinue, nil
	}
	return ast.WalkContinue, nil
}

func (w *mdWriter) writeCodeSpan(n ast.Node) {
	w.buf.WriteString(styleMdCode.Render(string(n.Text(w.src))))
}

func (w *mdWriter) writeTextNode(t *ast.Text) {
	w.writeInline(string(t.Segment.Value(w.src)))
	if t.SoftLineBreak() {
		w.writeInline(" ")
	}
	if t.HardLineBreak() {
		w.flushPara()
	}
}

func (w *mdWriter) writeInline(s string) {
	st := styleMdBody
	if w.bold > 0 {
		st = st.Bold(true)
	}
	if w.italic > 0 {
		st = st.Italic(true)
	}
	if w.code > 0 {
		st = styleMdCode
	}
	w.buf.WriteString(st.Render(s))
}

func (w *mdWriter) flushPara() {
	text := strings.TrimSpace(w.takeBuf())
	if text == "" {
		return
	}
	mark := ""
	if w.quote > 0 {
		mark = "│ "
		if w.ascii {
			mark = "| "
		}
	}
	inner := w.width
	if mark != "" {
		inner -= len([]rune(mark))
		if inner < 8 {
			inner = 8
		}
	}
	for _, line := range wrapStyled(text, inner) {
		w.lines = append(w.lines, mark+line)
	}
	w.blank()
}

func (w *mdWriter) takeBuf() string {
	s := w.buf.String()
	w.buf.Reset()
	return s
}

func (w *mdWriter) peekPlain() string {
	return strings.TrimSpace(stripANSI(w.buf.String()))
}

func (w *mdWriter) blank() {
	if len(w.lines) == 0 {
		return
	}
	if strings.TrimSpace(stripANSI(w.lines[len(w.lines)-1])) == "" {
		return
	}
	w.lines = append(w.lines, "")
}

func (w *mdWriter) inList() bool { return len(w.lists) > 0 }

func (w *mdWriter) nextBullet() string {
	indent := strings.Repeat("  ", max(len(w.lists)-1, 0))
	if len(w.lists) == 0 {
		return indent + "• "
	}
	st := &w.lists[len(w.lists)-1]
	if !st.ordered {
		return indent + "• "
	}
	n := st.next
	st.next++
	return indent + fmt.Sprintf("%d. ", n)
}

func (w *mdWriter) hrLine() string {
	unit := "─"
	if w.ascii {
		unit = "-"
	}
	n := w.width
	if n > 24 {
		n = 24
	}
	if n < 3 {
		n = 3
	}
	return styleMdDim.Render(strings.Repeat(unit, n))
}

func (w *mdWriter) blockText(n ast.Node) string {
	var b strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		if seg.Stop <= len(w.src) && seg.Start >= 0 && seg.Start <= seg.Stop {
			b.Write(w.src[seg.Start:seg.Stop])
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (w *mdWriter) renderTable(n ast.Node) []string {
	var rows [][]string
	for row := n.FirstChild(); row != nil; row = row.NextSibling() {
		var cells []string
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			if cell.Kind() == east.KindTableCell {
				cells = append(cells, strings.TrimSpace(plainNodeText(cell, w.src)))
			} else {
				for nested := cell.FirstChild(); nested != nil; nested = nested.NextSibling() {
					if nested.Kind() == east.KindTableCell {
						cells = append(cells, strings.TrimSpace(plainNodeText(nested, w.src)))
					}
				}
			}
		}
		if len(cells) > 0 {
			rows = append(rows, cells)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	widths := make([]int, cols)
	for _, r := range rows {
		for i, c := range r {
			if w := len([]rune(c)); w > widths[i] {
				widths[i] = w
			}
		}
	}
	minTable := 1
	for _, cw := range widths {
		minTable += cw + 3
	}
	if minTable > w.width {
		return w.tablePipeFallback(rows)
	}
	var out []string
	for i, r := range rows {
		out = append(out, formatPipeRow(r, widths, cols))
		if i == 0 && len(rows) > 1 {
			out = append(out, formatPipeSep(widths))
		}
	}
	return out
}

func (w *mdWriter) tablePipeFallback(rows [][]string) []string {
	var out []string
	for _, r := range rows {
		out = append(out, wrapText(strings.Join(r, " | "), w.width)...)
	}
	return out
}

func formatPipeRow(cells []string, widths []int, cols int) string {
	var b strings.Builder
	b.WriteByte('|')
	for i := 0; i < cols; i++ {
		val := ""
		if i < len(cells) {
			val = cells[i]
		}
		pad := widths[i] - len([]rune(val))
		if pad < 0 {
			pad = 0
		}
		b.WriteByte(' ')
		b.WriteString(val)
		b.WriteString(strings.Repeat(" ", pad))
		b.WriteString(" |")
	}
	return b.String()
}

func formatPipeSep(widths []int) string {
	var b strings.Builder
	b.WriteByte('|')
	for _, w := range widths {
		n := w + 2
		if n < 3 {
			n = 3
		}
		b.WriteString(strings.Repeat("-", n))
		b.WriteByte('|')
	}
	return b.String()
}

func plainNodeText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := child.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(src))
		case *ast.String:
			b.Write(t.Value)
		}
		if child.Kind() == ast.KindCodeSpan {
			b.Write(child.Text(src))
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

func wrapStyled(s string, width int) []string {
	if strings.TrimSpace(stripANSI(s)) == "" {
		return []string{""}
	}
	if lipgloss.Width(s) <= width {
		return []string{s}
	}
	return wrapText(stripANSI(s), width)
}

func boundFenceBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(body, "\n")
	overLines := len(lines) > mdFenceMaxLines
	overBytes := len(body) > mdFenceMaxBytes
	if !overLines && !overBytes {
		return body
	}
	head, tail := mdFenceHeadLines, mdFenceTailLines
	if len(lines) <= head+tail+1 {
		head = len(lines) / 2
		tail = len(lines) - head
		if tail < 1 {
			tail = 1
			head = len(lines) - 1
		}
	}
	if head+tail >= len(lines) {
		return body
	}
	omitted := len(lines) - head - tail
	out := make([]string, 0, head+tail+1)
	out = append(out, lines[:head]...)
	out = append(out, fmt.Sprintf("… [%d lines truncated] …", omitted))
	out = append(out, lines[len(lines)-tail:]...)
	return strings.Join(out, "\n")
}
