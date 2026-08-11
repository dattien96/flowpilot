package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"flowpilot-runner/internal/flowgate"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("gate-check", flag.ExitOnError)
	goOnly := fs.Bool("go-only", false, "run only the Go baseline check")
	tsOnly := fs.Bool("ts-only", false, "run only the TypeScript baseline check")
	bootstrap := fs.Bool("bootstrap", false, "capture/update baselines instead of checking")
	help := fs.Bool("help", false, "show usage")
	_ = fs.Parse(args)

	if *help {
		printHelp()
		return 0
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gate-check: %v\n", err)
		return 2
	}
	dotFP := filepath.Join(repoRoot, ".flowpilot")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if *bootstrap {
		return bootstrapBaselines(ctx, repoRoot, dotFP)
	}

	runGo := !*tsOnly
	runTS := !*goOnly
	failed := false

	if runGo {
		res, err := flowgate.CheckDogfoodSuite(ctx, repoRoot, dotFP, flowgate.GoBaselineFile, "go", false)
		fmt.Println(res.Message)
		if err != nil {
			failed = true
		}
	}
	if runTS {
		res, err := flowgate.CheckDogfoodSuite(ctx, repoRoot, dotFP, flowgate.TSBaselineFile, "ts", true)
		fmt.Println(res.Message)
		if err != nil {
			failed = true
		} else if res.Skipped {
			fmt.Fprintln(os.Stderr, "gate-check: ts baseline missing — warn only (D-6)")
		}
	}

	if failed {
		return 1
	}
	return 0
}

func bootstrapBaselines(ctx context.Context, repoRoot, dotFP string) int {
	goCmd := "go test ./..."
	goDir := "apps/local-runner"
	if _, err := flowgate.CaptureBaselineFile(ctx, repoRoot, dotFP, flowgate.GoBaselineFile, goCmd, goDir); err != nil {
		fmt.Fprintf(os.Stderr, "gate-check bootstrap go: %v\n", err)
		return 1
	}
	fmt.Printf("captured Go baseline → .flowpilot/guard/%s (test_dir=%s)\n", flowgate.GoBaselineFile, goDir)

	tsCmd := "npm run typecheck"
	tsDir := "apps/desktop-flowpilot"
	if _, err := flowgate.CaptureBaselineFile(ctx, repoRoot, dotFP, flowgate.TSBaselineFile, tsCmd, tsDir); err != nil {
		fmt.Fprintf(os.Stderr, "gate-check bootstrap ts (warn): %v\n", err)
		fmt.Fprintln(os.Stderr, "TS baseline not captured — Go baseline still usable (D-6)")
	} else {
		fmt.Printf("captured TS baseline → .flowpilot/guard/%s (test_dir=%s)\n", flowgate.TSBaselineFile, tsDir)
	}
	return 0
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find repo root (.git) from cwd")
		}
		dir = parent
	}
}

func printHelp() {
	fmt.Print(`gate-check — FlowPilot dogfood regression gate (CP-53 P-3)

Usage:
  gate-check [--go-only|--ts-only]
  gate-check --bootstrap
  gate-check --help

Exit codes:
  0  pass (TS missing baseline is warn-only when running both suites)
  1  regression / gate_blind / env error
  2  usage / repo root error

Baselines (D-6 independent):
  Go: .flowpilot/guard/test_baseline.json       (apps/local-runner)
  TS: .flowpilot/guard/test_baseline_ts.json    (apps/desktop-flowpilot typecheck)

Install hooks:
  ./scripts/install-gate-hooks.sh

Windows: use Git Bash or WSL; native PowerShell hook is not supported in v1.
`)
}
