package flowgate

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	goMaxPackages    = 10
	gradleMaxModules = 5
	pytestMaxDirs    = 8
	jestMaxDirs      = 8
)

// scopeTestCommand returns a test command scoped to the packages/modules covered
// by the diff. Returns "" when scoping is not possible or the fallback threshold
// is exceeded — callers must use the original testCmd in that case.
//
// testDir is the directory the test command runs in, relative to repoDir (may be
// ""). It is used to strip the module prefix from diff paths so package arguments
// are relative to the test runner's working directory. (Task-158)
func scopeTestCommand(repoDir, testDir, testCmd string, diff []ChangedFile) string {
	if len(diff) == 0 {
		return ""
	}
	switch {
	case strings.HasPrefix(testCmd, "go test"):
		return scopeGo(testDir, testCmd, diff)
	case strings.HasPrefix(testCmd, "./gradlew") || strings.HasPrefix(testCmd, "gradlew"):
		return scopeGradle(repoDir, testDir, testCmd, diff)
	case strings.HasPrefix(testCmd, "xcodebuild"):
		return scopeXcode(repoDir, testDir, testCmd, diff)
	case strings.HasPrefix(testCmd, "pytest"):
		return scopePytest(testDir, testCmd, diff)
	case strings.HasPrefix(testCmd, "jest") || strings.HasPrefix(testCmd, "npx jest"):
		return scopeJest(testDir, testCmd, diff)
	default:
		return ""
	}
}

// scopeGo maps changed .go files to package dirs and builds a scoped go test command.
func scopeGo(testDir, testCmd string, diff []ChangedFile) string {
	if goHasExplicitScope(testCmd) {
		return ""
	}

	pathPrefix := ""
	if testDir != "" {
		pathPrefix = testDir + "/"
	}

	pkgSet := make(map[string]struct{})
	for _, f := range diff {
		if !strings.HasSuffix(f.Path, ".go") {
			continue
		}
		path := f.Path
		if pathPrefix != "" {
			if !strings.HasPrefix(path, pathPrefix) {
				continue
			}
			path = strings.TrimPrefix(path, pathPrefix)
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		pkgSet[dir] = struct{}{}
	}
	if len(pkgSet) == 0 {
		return ""
	}
	if len(pkgSet) > goMaxPackages {
		log.Printf("[gate] scope threshold exceeded (%d packages), falling back to full suite", len(pkgSet))
		return ""
	}

	pkgs := make([]string, 0, len(pkgSet))
	for dir := range pkgSet {
		if dir == "." || dir == "" {
			pkgs = append(pkgs, ".")
		} else {
			pkgs = append(pkgs, "./"+dir+"/...")
		}
	}
	sort.Strings(pkgs)

	flags := goExtractFlags(testCmd)
	var cmd string
	if len(flags) > 0 {
		cmd = "go test " + strings.Join(flags, " ") + " " + strings.Join(pkgs, " ")
	} else {
		cmd = "go test " + strings.Join(pkgs, " ")
	}

	log.Printf("[gate] scoped oracle run: %q (%d packages, full would be ./...)", cmd, len(pkgs))
	return cmd
}

// goHasExplicitScope returns true when the test command already contains a specific
// package path (not the ./... wildcard), meaning the user has manually scoped it.
func goHasExplicitScope(testCmd string) bool {
	parts := strings.Fields(testCmd)
	for _, p := range parts[2:] { // skip "go" "test"
		if !strings.HasPrefix(p, "-") && p != "./..." {
			return true
		}
	}
	return false
}

// goExtractFlags returns only the flag arguments from a "go test <flags> <pkgs>" command.
func goExtractFlags(testCmd string) []string {
	parts := strings.Fields(testCmd)
	var flags []string
	for _, p := range parts[2:] {
		if strings.HasPrefix(p, "-") {
			flags = append(flags, p)
		}
	}
	return flags
}

// scopeGradle finds the Gradle modules containing changed files and builds a scoped command.
// Only scopes when the task being run is "test". Non-test tasks (lint, build, assemble, etc.)
// fall back to the full command unchanged. Extra flags (e.g. --no-daemon) are preserved.
func scopeGradle(repoDir, testDir, testCmd string, diff []ChangedFile) string {
	if gradleHasExplicitScope(testCmd) {
		return ""
	}

	task, extraFlags := gradleExtractTaskAndFlags(testCmd)
	if task != "test" {
		// Only scope "test". lint, build, assemble, etc. must run as-is.
		return ""
	}

	gradleRoot := repoDir
	pathPrefix := ""
	if testDir != "" {
		gradleRoot = filepath.Join(repoDir, filepath.FromSlash(testDir))
		pathPrefix = testDir + "/"
	}

	modSet := make(map[string]struct{})
	for _, f := range diff {
		path := f.Path
		if pathPrefix != "" {
			if !strings.HasPrefix(path, pathPrefix) {
				continue
			}
			path = strings.TrimPrefix(path, pathPrefix)
		}
		mod := findGradleModule(gradleRoot, path)
		if mod != "" {
			modSet[mod] = struct{}{}
		}
	}
	if len(modSet) == 0 {
		return ""
	}
	if len(modSet) > gradleMaxModules {
		log.Printf("[gate] scope threshold exceeded (%d modules), falling back to full suite", len(modSet))
		return ""
	}

	mods := make([]string, 0, len(modSet))
	for m := range modSet {
		mods = append(mods, m)
	}
	sort.Strings(mods)

	parts := make([]string, 0, len(mods)+len(extraFlags))
	for _, m := range mods {
		parts = append(parts, m+":test")
	}
	parts = append(parts, extraFlags...)

	wrapper := strings.Fields(testCmd)[0]
	cmd := wrapper + " " + strings.Join(parts, " ")
	log.Printf("[gate] scoped oracle run: %q (%d modules, full would be %s test)", cmd, len(mods), wrapper)
	return cmd
}

// gradleHasExplicitScope returns true when the command already specifies module tasks
// (any argument starts with ":" indicating a named module path).
func gradleHasExplicitScope(testCmd string) bool {
	for _, p := range strings.Fields(testCmd)[1:] {
		if strings.HasPrefix(p, ":") {
			return true
		}
	}
	return false
}

// gradleExtractTaskAndFlags splits the Gradle command into the task name (first
// non-flag, non-module arg) and any option flags (args starting with "-").
func gradleExtractTaskAndFlags(testCmd string) (task string, flags []string) {
	for _, p := range strings.Fields(testCmd)[1:] { // skip wrapper (./gradlew)
		switch {
		case strings.HasPrefix(p, "-"):
			flags = append(flags, p)
		case !strings.HasPrefix(p, ":") && task == "":
			task = p
		}
	}
	return
}

// findGradleModule walks up from the file's directory to find the nearest
// build.gradle[.kts] under gradleRoot, then returns the Gradle module path
// (e.g. ":feature-auth" or ":core:network"). Returns "" when none is found.
func findGradleModule(gradleRoot, filePath string) string {
	dir := filepath.ToSlash(filepath.Dir(filePath))
	for dir != "." && dir != "" {
		absDir := filepath.Join(gradleRoot, filepath.FromSlash(dir))
		for _, name := range []string{"build.gradle", "build.gradle.kts"} {
			if _, err := os.Stat(filepath.Join(absDir, name)); err == nil {
				return ":" + strings.ReplaceAll(dir, "/", ":")
			}
		}
		parent := filepath.ToSlash(filepath.Dir(dir))
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// scopeXcode applies a naming heuristic to derive test targets from changed Swift files.
// Returns "" when the heuristic produces no match or more than one unrelated target.
// testDir scopes the search root so nested iOS projects are found correctly.
func scopeXcode(repoDir, testDir, testCmd string, diff []ChangedFile) string {
	searchRoot := repoDir
	pathPrefix := ""
	if testDir != "" {
		searchRoot = filepath.Join(repoDir, filepath.FromSlash(testDir))
		pathPrefix = testDir + "/"
	}

	targets := make(map[string]struct{})
	for _, f := range diff {
		if !strings.HasSuffix(f.Path, ".swift") {
			continue
		}
		path := f.Path
		if pathPrefix != "" {
			if !strings.HasPrefix(path, pathPrefix) {
				continue
			}
			path = strings.TrimPrefix(path, pathPrefix)
		}
		parts := strings.Split(filepath.ToSlash(filepath.Dir(path)), "/")
		for i, part := range parts {
			if strings.EqualFold(part, "Sources") || strings.EqualFold(part, "Source") {
				if i+1 < len(parts) && parts[i+1] != "" {
					candidate := parts[i+1] + "Tests"
					if xcodeTargetDirExists(searchRoot, candidate) {
						targets[candidate] = struct{}{}
					}
				}
				break
			}
		}
	}

	if len(targets) != 1 {
		return ""
	}

	var target string
	for t := range targets {
		target = t
	}
	cmd := testCmd + " -only-testing:" + target
	log.Printf("[gate] scoped oracle run: %q (xcodebuild heuristic)", cmd)
	return cmd
}

// xcodeTargetDirExists does a shallow two-level directory search for a directory
// whose name matches targetName (case-insensitive).
func xcodeTargetDirExists(searchRoot, targetName string) bool {
	entries, err := os.ReadDir(searchRoot)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.EqualFold(e.Name(), targetName) {
			return true
		}
		sub, err := os.ReadDir(filepath.Join(searchRoot, e.Name()))
		if err != nil {
			continue
		}
		for _, se := range sub {
			if se.IsDir() && strings.EqualFold(se.Name(), targetName) {
				return true
			}
		}
	}
	return false
}

// scopePytest collects unique directories of changed .py files and builds a scoped pytest command.
// Falls back to "" when conftest.py is changed (global fixtures → scope unreliable).
// testDir strips the module prefix so paths are relative to the runner's working directory.
func scopePytest(testDir, testCmd string, diff []ChangedFile) string {
	pathPrefix := ""
	if testDir != "" {
		pathPrefix = testDir + "/"
	}

	dirSet := make(map[string]struct{})
	for _, f := range diff {
		if !strings.HasSuffix(f.Path, ".py") {
			continue
		}
		path := f.Path
		if pathPrefix != "" {
			if !strings.HasPrefix(path, pathPrefix) {
				continue
			}
			path = strings.TrimPrefix(path, pathPrefix)
		}
		if filepath.Base(path) == "conftest.py" {
			log.Printf("[gate] conftest.py changed, falling back to full suite")
			return ""
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		if dir != "" && dir != "." {
			dirSet[dir] = struct{}{}
		}
	}
	if len(dirSet) == 0 {
		return ""
	}
	if len(dirSet) > pytestMaxDirs {
		log.Printf("[gate] scope threshold exceeded (%d dirs), falling back to full suite", len(dirSet))
		return ""
	}

	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d+"/")
	}
	sort.Strings(dirs)

	cmd := "pytest -v " + strings.Join(dirs, " ")
	log.Printf("[gate] scoped oracle run: %q (%d dirs)", cmd, len(dirs))
	return cmd
}

// scopeJest collects unique directories of changed JS/TS files and builds a jest
// --testPathPattern command.
// testDir strips the module prefix so paths are relative to the runner's working directory.
func scopeJest(testDir, testCmd string, diff []ChangedFile) string {
	pathPrefix := ""
	if testDir != "" {
		pathPrefix = testDir + "/"
	}

	dirSet := make(map[string]struct{})
	for _, f := range diff {
		ext := filepath.Ext(f.Path)
		if ext != ".ts" && ext != ".tsx" && ext != ".js" && ext != ".jsx" {
			continue
		}
		path := f.Path
		if pathPrefix != "" {
			if !strings.HasPrefix(path, pathPrefix) {
				continue
			}
			path = strings.TrimPrefix(path, pathPrefix)
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		if dir != "" && dir != "." {
			dirSet[dir] = struct{}{}
		}
	}
	if len(dirSet) == 0 {
		return ""
	}
	if len(dirSet) > jestMaxDirs {
		log.Printf("[gate] scope threshold exceeded (%d dirs), falling back to full suite", len(dirSet))
		return ""
	}

	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	pattern := strings.Join(dirs, "|")
	baseCmd := "jest"
	if strings.HasPrefix(testCmd, "npx jest") {
		baseCmd = "npx jest"
	}
	cmd := baseCmd + " --testPathPattern=" + pattern
	log.Printf("[gate] scoped oracle run: %q (%d dirs)", cmd, len(dirs))
	return cmd
}
