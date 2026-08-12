package app

import (
	"context"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ProjectContextMsg carries target project path + git branch for the statusline.
type ProjectContextMsg struct {
	Path   string
	Branch string
}

func gitBranchForPath(projectPath string) string {
	path := strings.TrimSpace(projectPath)
	if path == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (m *AppModel) cmdRefreshProjectContext() tea.Cmd {
	path := strings.TrimSpace(m.cfg.ProjectPath)
	if m.project != nil && strings.TrimSpace(m.project.Path) != "" {
		path = m.project.Path
	}
	return func() tea.Msg {
		return ProjectContextMsg{Path: path, Branch: gitBranchForPath(path)}
	}
}

func (m *AppModel) effectiveYolo() bool {
	if m.mode == ModeFlow || m.mode == ModeStep || m.launch.IsArmed() {
		return true
	}
	return m.yolo
}

func (m *AppModel) yoloStatusLabel() string {
	if m.mode == ModeFlow || m.mode == ModeStep || m.launch.IsArmed() {
		return "YOLO:ON(auto)"
	}
	if m.yolo {
		return "YOLO:ON"
	}
	return "YOLO:OFF"
}

func (m *AppModel) projectStatusLabel() string {
	name := ""
	path := strings.TrimSpace(m.projectPath)
	if path == "" {
		path = strings.TrimSpace(m.cfg.ProjectPath)
	}
	if m.project != nil {
		name = strings.TrimSpace(m.project.Name)
		if strings.TrimSpace(m.project.Path) != "" {
			path = m.project.Path
		}
	}
	if name == "" && path != "" {
		name = pathBase(path)
	}
	if name == "" {
		name = "(no project)"
	}
	branch := strings.TrimSpace(m.projectBranch)
	if branch == "" {
		branch = "—"
	}
	displayPath := path
	if len([]rune(displayPath)) > 48 {
		r := []rune(displayPath)
		displayPath = "…" + string(r[len(r)-45:])
	}
	if displayPath != "" && !strings.EqualFold(displayPath, name) {
		return name + "  " + displayPath + "  ·  " + branch
	}
	return name + "  ·  " + branch
}

func pathBase(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), `\`, `/`)
	p = strings.TrimRight(p, "/")
	if p == "" {
		return ""
	}
	if i := strings.LastIndex(p, "/"); i >= 0 && i+1 < len(p) {
		return p[i+1:]
	}
	return p
}
