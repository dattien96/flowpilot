package lsp

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// GradleError is one compiler error parsed from Gradle output.
type GradleError struct {
	// File is the workspace-relative (or absolute) path, "" when the
	// message carries no location.
	File string
	// Line is 1-based, 0 when unknown.
	Line int
	// Message is the human-readable error text.
	Message string
}

// ShouldRunGradleFallback reports whether the Gradle fallback applies: only
// for Android workspaces whose LSP diagnostics are clean (kotlin-language-
// server is known to miss R-class and Compose generated-type errors, so a
// clean LSP result is exactly when deeper validation pays off).
func ShouldRunGradleFallback(diagnostics []FileDiagnostic, platform string) bool {
	return platform == "android" && len(diagnostics) == 0
}

// gradleWrapper resolves the project wrapper script.
func gradleWrapper(workspaceRoot string) (string, error) {
	name := "gradlew"
	if runtime.GOOS == "windows" {
		name = "gradlew.bat"
	}
	p := filepath.Join(workspaceRoot, name)
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		return "", fmt.Errorf("lsp: gradlew not found in %s", workspaceRoot)
	}
	return p, nil
}

// RunGradleValidation runs compileDebugKotlin and parses compiler errors.
// A failing compilation is a normal result (parsed errors, nil error); the
// error return is reserved for infrastructure failures (missing wrapper,
// timeout/cancel, spawn errors). Callers bound the budget via ctx.
func RunGradleValidation(ctx context.Context, workspaceRoot string) ([]GradleError, error) {
	bin, err := gradleWrapper(workspaceRoot)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, bin, "compileDebugKotlin", "--console=plain")
	cmd.Dir = workspaceRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Non-zero exit with output is a failed compilation, not an
		// infrastructure failure: fall through to parsing.
		if _, ok := err.(*exec.ExitError); !ok {
			return nil, fmt.Errorf("lsp: run gradle: %w", err)
		}
	}
	var out []GradleError
	for _, line := range strings.Split(stdout.String()+"\n"+stderr.String(), "\n") {
		if e, ok := parseGradleErrorLine(strings.TrimSpace(line)); ok {
			out = append(out, e)
		}
	}
	return out, nil
}

// parseGradleErrorLine recognizes the two common Kotlin/Gradle error shapes:
//  1. e: file:///path/F.kt:12:5 Unresolved reference: R
//  2. path/F.kt:12: error: too many arguments
//  3. path/F.kt:12:5: error: too many arguments
//
// Warnings (w:) and task headers ("> Task ...") never match.
func parseGradleErrorLine(line string) (GradleError, bool) {
	if line == "" {
		return GradleError{}, false
	}
	rest := line
	isError := false
	if strings.HasPrefix(rest, "e:") {
		isError = true
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "e:"))
	} else if strings.HasPrefix(rest, "w:") {
		return GradleError{}, false
	}
	// Shape 2/3 carry an explicit "error:" marker.
	if idx := strings.Index(rest, "error:"); idx >= 0 {
		loc := strings.TrimRight(strings.TrimSpace(rest[:idx]), ":")
		msg := strings.TrimSpace(rest[idx+len("error:"):])
		file, lnum := splitFileLine(loc)
		if msg == "" {
			return GradleError{}, false
		}
		return GradleError{File: file, Line: lnum, Message: msg}, true
	}
	if !isError {
		return GradleError{}, false
	}
	// Shape 1: location and message separated by whitespace.
	fields := strings.Fields(rest)
	if len(fields) < 2 {
		return GradleError{}, false
	}
	file, lnum := splitFileLine(fields[0])
	msg := strings.TrimSpace(strings.TrimPrefix(rest, fields[0]))
	if msg == "" {
		return GradleError{}, false
	}
	return GradleError{File: file, Line: lnum, Message: msg}, true
}

// splitFileLine splits "path/F.kt:12[:5]" (or a file:// URI form) into the
// display path and 1-based line (0 when unparseable).
func splitFileLine(loc string) (string, int) {
	loc = strings.TrimPrefix(loc, "file://")
	parts := strings.Split(loc, ":")
	var nums []int
	for len(parts) > 1 {
		n, err := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1]))
		if err != nil {
			break
		}
		nums = append([]int{n}, nums...)
		parts = parts[:len(parts)-1]
	}
	file := strings.Join(parts, ":")
	// Repair a Windows drive prefix mangled by the split: "/C:/x" splits
	// into ["", "/C", "/x"], rejoining as ":/C:/x".
	if len(file) > 3 && file[0] == ':' && file[3] == ':' && isDriveLetter(file[2]) {
		file = file[2:]
	}
	line := 0
	if len(nums) > 0 {
		line = nums[0] // leftmost numeric is the line; rest is the column
	}
	return strings.TrimSpace(file), line
}

func isDriveLetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

// FormatGradleErrorsForAgent renders errors one per line as
// "file:line: error: message" (line omitted when unknown).
func FormatGradleErrorsForAgent(errors []GradleError) string {
	var sb strings.Builder
	for i, e := range errors {
		if i > 0 {
			sb.WriteString("\n")
		}
		switch {
		case e.File != "" && e.Line > 0:
			fmt.Fprintf(&sb, "%s:%d: error: %s", e.File, e.Line, e.Message)
		case e.File != "":
			fmt.Fprintf(&sb, "%s: error: %s", e.File, e.Message)
		default:
			fmt.Fprintf(&sb, "error: %s", e.Message)
		}
	}
	return sb.String()
}
