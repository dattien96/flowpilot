package reqscaffold

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed scaffold-pack
var scaffoldPackFS embed.FS

// folderSpec describes one requirements subfolder to scaffold.
type folderSpec struct {
	// packDir is the path inside scaffold-pack (e.g. "scaffold-pack/08-Task").
	packDir string
	// targetDir is the relative path under requirements/ in the target repo.
	targetDir string
	// subdirs are empty child directories to create (e.g. "done", "todo").
	subdirs []string
}

var folders = []folderSpec{
	{packDir: "scaffold-pack/05-System-Specs", targetDir: "05-System-Specs"},
	{packDir: "scaffold-pack/06-System-Tech-Design", targetDir: "06-System-Tech-Design"},
	{packDir: "scaffold-pack/07-Coding-Plan", targetDir: "07-Coding-Plan",
		subdirs: []string{"todo", "inprogress", "done"}},
	{packDir: "scaffold-pack/08-Task", targetDir: "08-Task",
		subdirs: []string{"todo", "done"}},
	{packDir: "scaffold-pack/09-BugFix", targetDir: "09-BugFix",
		subdirs: []string{"todo", "done"}},
}

// ScaffoldResult summarises what Scaffold did.
type ScaffoldResult struct {
	Target    string
	Created   []string // directories and files newly written
	Skipped   []string // already-present items left untouched
	Errors    []string
}

// IsScaffolded returns true when requirements/ with at least the five expected
// subfolders is already present in targetRepoDir.
func IsScaffolded(targetRepoDir string) bool {
	for _, f := range folders {
		info, err := os.Stat(filepath.Join(targetRepoDir, "requirements", f.targetDir))
		if err != nil || !info.IsDir() {
			return false
		}
	}
	return true
}

// Scaffold creates requirements/<05-09>/ subfolders in targetRepoDir when they
// are absent and copies the embedded FORMAT-REFERENCE-*.md into each one if
// the file does not yet exist. Existing FORMAT-REFERENCE files are never
// overwritten — a project may customise them. All errors are collected rather
// than aborting early; the function never panics.
func Scaffold(targetRepoDir string) (ScaffoldResult, error) {
	result := ScaffoldResult{Target: targetRepoDir}

	reqRoot := filepath.Join(targetRepoDir, "requirements")
	if err := os.MkdirAll(reqRoot, 0o755); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("mkdir requirements/: %v", err))
		return result, errors.New("could not create requirements directory")
	}

	for _, spec := range folders {
		folderAbs := filepath.Join(reqRoot, spec.targetDir)

		// Create the subfolder itself.
		if mkErr := os.MkdirAll(folderAbs, 0o755); mkErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("mkdir %s: %v", spec.targetDir, mkErr))
			continue
		}
		result.Created = append(result.Created, folderAbs)

		// Create declared child directories (todo/, done/, inprogress/).
		for _, sub := range spec.subdirs {
			subAbs := filepath.Join(folderAbs, sub)
			if info, statErr := os.Stat(subAbs); statErr == nil && info.IsDir() {
				result.Skipped = append(result.Skipped, subAbs)
				continue
			}
			if mkErr := os.MkdirAll(subAbs, 0o755); mkErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("mkdir %s/%s: %v", spec.targetDir, sub, mkErr))
				continue
			}
			result.Created = append(result.Created, subAbs)
		}

		// Copy each embedded file from the pack into the folder — skip if present.
		entries, err := fs.ReadDir(scaffoldPackFS, spec.packDir)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("read embedded %s: %v", spec.packDir, err))
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			destFile := filepath.Join(folderAbs, entry.Name())
			if _, statErr := os.Stat(destFile); statErr == nil {
				result.Skipped = append(result.Skipped, destFile)
				continue
			}
			srcPath := spec.packDir + "/" + entry.Name()
			data, readErr := scaffoldPackFS.ReadFile(srcPath)
			if readErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("read embedded %s: %v", srcPath, readErr))
				continue
			}
			if writeErr := os.WriteFile(destFile, data, 0o644); writeErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("write %s: %v", destFile, writeErr))
				continue
			}
			result.Created = append(result.Created, destFile)
		}
	}

	return result, nil
}
