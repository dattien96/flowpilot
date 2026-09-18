package cli

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"flowpilot-runner/internal/lsp"
)

// doctorRow is one language server's environment status.
type doctorRow struct {
	Platform    string
	Binary      string
	FoundPath   string
	Installed   bool
	InstallHint string
}

// doctorCheck probes every registered LSP server binary. lookPath is
// exec.LookPath in production and a stub in tests.
func doctorCheck(lookPath func(string) (string, error)) []doctorRow {
	reg := lsp.DefaultRegistry()
	plats := make([]string, 0, len(reg))
	for p := range reg {
		plats = append(plats, p)
	}
	sort.Strings(plats)
	rows := make([]doctorRow, 0, len(plats))
	for _, p := range plats {
		cfg := reg[p]
		row := doctorRow{Platform: p, Binary: cfg.Binary, InstallHint: cfg.InstallHint}
		if path, err := lookPath(cfg.Binary); err == nil {
			row.Installed = true
			row.FoundPath = path
		}
		rows = append(rows, row)
	}
	return rows
}

// doctorMissing counts rows without a resolvable binary.
func doctorMissing(rows []doctorRow) int {
	n := 0
	for _, r := range rows {
		if !r.Installed {
			n++
		}
	}
	return n
}

// formatDoctor renders the check as an aligned plain-text table.
func formatDoctor(rows []doctorRow) string {
	pw, bw := len("platform"), len("binary")
	for _, r := range rows {
		pw = max(pw, len(r.Platform))
		bw = max(bw, len(r.Binary))
	}
	var sb strings.Builder
	sb.WriteString("FlowPilot doctor — language servers\n")
	fmt.Fprintf(&sb, "%-*s  %-*s  %s\n", pw, "platform", bw, "binary", "status")
	for _, r := range rows {
		status := "ok " + r.FoundPath
		if !r.Installed {
			status = "MISSING  " + r.InstallHint
		}
		fmt.Fprintf(&sb, "%-*s  %-*s  %s\n", pw, r.Platform, bw, r.Binary, status)
	}
	if n := doctorMissing(rows); n > 0 {
		fmt.Fprintf(&sb, "\n%d server(s) missing — install them for live compiler diagnostics.\n", n)
	} else {
		sb.WriteString("\nAll language servers present.\n")
	}
	return sb.String()
}

func newDoctorCommand(cfg *config) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check language-server binaries for live diagnostics",
		Long: `Probes every language server in the LSP platform registry (gopls,
vtsls, pyright-langserver, rust-analyzer, clangd, kotlin-language-server)
via PATH and prints per-platform status with install hints.

Exits non-zero when any server is missing; FlowPilot keeps working without
them (diagnostics degrade to build commands).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rows := doctorCheck(exec.LookPath)
			cmd.Println(formatDoctor(rows))
			if n := doctorMissing(rows); n > 0 {
				return fmt.Errorf("flowpilot doctor: %d language server(s) missing", n)
			}
			return nil
		},
	}
}
