package promptblock

import (
	"strings"
	"unicode/utf8"
)

// TruncateUTF8 returns s clipped to at most maxBytes without splitting a UTF-8
// code point. The bool reports whether truncation happened.
func TruncateUTF8(s string, maxBytes int) (string, bool) {
	if maxBytes <= 0 {
		return "", true
	}
	if len(s) <= maxBytes {
		return s, false
	}
	if maxBytes >= len(s) {
		return s, false
	}

	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	if cut <= 0 {
		return "", true
	}
	return s[:cut], true
}

// EscapeClosingTag makes it safe to embed text inside a tag-delimited prompt
// block by escaping the corresponding closing tag.
func EscapeClosingTag(text, tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return text
	}
	closeTag := "</" + tag + ">"
	return strings.ReplaceAll(text, closeTag, "<\\/"+tag+">")
}
