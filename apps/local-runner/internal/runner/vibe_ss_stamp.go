package runner

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// BUG-365: ss_lock was in-memory only, so the locked SS list stayed
// `status: draft` on disk. cp_writer read the draft and asked the operator to
// fix the SS status before writing the CP (live run-646702); the run then had
// no CP/Task and the slicer parked. Locking now freezes the list on disk.
var (
	vibeSSFrontmatterStatusDraft = regexp.MustCompile(`(?m)^status:[ \t]*"?draft"?[ \t]*$`)
	vibeSSMetadataStatusDraft    = regexp.MustCompile("(?m)^- Status:[ \t]*`?draft`?[ \t]*$")
)

// stampVibeSSApproved flips `status: draft` to `status: approved` for the
// locked SS list (SS-*.md + SPRINT-PLAN*.md). FORMAT-* references are skipped
// and prose that merely mentions draft is untouched.
func stampVibeSSApproved(cwd string) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return
	}
	dir := filepath.Join(cwd, "requirements", "05-System-Specs")
	for _, pat := range []string{
		filepath.Join(dir, "SS-*.md"),
		filepath.Join(dir, "SPRINT-PLAN*.md"),
	} {
		matches, err := filepath.Glob(pat)
		if err != nil {
			continue
		}
		for _, abs := range matches {
			if strings.HasPrefix(strings.ToUpper(filepath.Base(abs)), "FORMAT-") {
				continue
			}
			b, readErr := os.ReadFile(abs)
			if readErr != nil {
				continue
			}
			src := string(b)
			stamped := vibeSSFrontmatterStatusDraft.ReplaceAllString(src, "status: approved")
			stamped = vibeSSMetadataStatusDraft.ReplaceAllString(stamped, "- Status: `approved`")
			if stamped == src {
				continue
			}
			_ = os.WriteFile(abs, []byte(stamped), 0o644)
		}
	}
}
