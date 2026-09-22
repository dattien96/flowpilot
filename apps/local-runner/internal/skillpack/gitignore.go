package skillpack

import (
	"os"
	"path/filepath"
	"strings"
)

// gitignoreAIHeader marks the block of AI-tool ignore entries the init flow
// maintains in the target repo's .gitignore.
const gitignoreAIHeader = "# AI Rules"

// gitignoreAIEntries are the AI tool dirs/files kept out of version control.
// Install writes provider skills into several of these roots; the rest are
// common agent-tool locations that should likewise never be committed.
var gitignoreAIEntries = []string{
	".agents/",
	".claude/",
	".codex/",
	".grok/",
	".opencode/",
	".cursorrules",
	".qwen/",
}

// EnsureGitignoreAIEntries makes <targetRepoDir>/.gitignore contain the AI
// rules block: the file is created when missing, missing entries are appended
// when the file exists, and an already-complete file is left untouched
// (idempotent). Existing user content is never modified or reordered.
// Returns whether the file was written.
func EnsureGitignoreAIEntries(targetRepoDir string) (bool, error) {
	path := filepath.Join(targetRepoDir, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	existing := make(map[string]bool, len(gitignoreAIEntries))
	for _, line := range strings.Split(string(data), "\n") {
		existing[strings.TrimSpace(line)] = true
	}

	missing := make([]string, 0, len(gitignoreAIEntries))
	for _, entry := range gitignoreAIEntries {
		if !existing[entry] {
			missing = append(missing, entry)
		}
	}
	if len(missing) == 0 {
		return false, nil
	}

	var out strings.Builder
	content := string(data)
	out.WriteString(content)
	if len(content) > 0 {
		if !strings.HasSuffix(content, "\n") {
			out.WriteString("\n")
		}
		out.WriteString("\n")
	}
	if !existing[gitignoreAIHeader] {
		out.WriteString(gitignoreAIHeader + "\n")
	}
	for _, entry := range missing {
		out.WriteString(entry + "\n")
	}

	return true, os.WriteFile(path, []byte(out.String()), 0o644)
}
