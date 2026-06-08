package runner

import (
	"path/filepath"
	"runtime"
	"strings"
)

func canonicalPathKey(value string) string {
	cleaned := filepath.Clean(strings.TrimSpace(value))
	if cleaned == "." || cleaned == "" {
		return ""
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(cleaned)
	}
	return cleaned
}

func samePath(a, b string) bool {
	return canonicalPathKey(a) != "" && canonicalPathKey(a) == canonicalPathKey(b)
}
