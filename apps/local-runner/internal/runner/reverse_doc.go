package runner

// Task-333 T-2 / T-3 (CP-49 P-1 / P-2): reverse-documentation evidence
// collection and deterministic draft generation.
//
// Provider-agnostic parity note: this file contains NO LLM calls. Evidence is
// collected by (1) a subprocess to the GitNexus CLI (`gitnexus query`) when it
// is available, and (2) a deterministic Go fallback — static directory scan +
// go/parser AST of exported declarations — when it is not (CP-49 graceful
// degradation). Git history contributes recent commits. Draft SD/SS content is
// synthesized by deterministic Go templating where every factual claim carries
// an explicit evidence citation (symbol / file / commit); missing evidence is
// flagged with `TODO` instead of being invented (CP-49 P-2 anti-hallucination).
//
// READ-ONLY on target source: evidence collection never writes to the scanned
// workspace; the only writes in the /standardize flow are draft documents
// under requirements/05-System-Specs/todo/ and requirements/06-System-Tech-
// Design/todo/ (see standardize_cmd.go / ss_lock_gate.go).

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// gitnexusQueryTimeout bounds a single `gitnexus query` subprocess.
	gitnexusQueryTimeout = 20 * time.Second
	// gitnexusLogTimeout bounds a `git log` subprocess.
	gitnexusLogTimeout = 10 * time.Second
	// gitnexusAnalyzeTimeout is the generous guard for the CP-49 R-2
	// auto-`gitnexus analyze` on a stale/missing index. It is only attempted
	// when the workspace is a real git repo; tests use t.TempDir() sandboxes
	// (no .git) so the command path is never blocked by analyze in tests.
	gitnexusAnalyzeTimeout = 8 * time.Minute
	// Deterministic caps so a huge workspace cannot blow up evidence size.
	revDocMaxSymbols  = 200
	revDocMaxAPIs     = 100
	revDocMaxFlows    = 50
	revDocMaxCommits  = 10
	revDocMaxFiles    = 400
	revDocMaxEndpoint = 50
)

// Evidence contains source-grounded proof collected for the reverse-doc flow
// (Task-333 Code Guide). Every list holds already-formatted, human-readable
// citation strings.
type Evidence struct {
	Symbols        []string `json:"symbols"`
	Endpoints      []string `json:"endpoints"`
	PublicAPIs     []string `json:"public_apis"`
	ExecutionFlows []string `json:"execution_flows"`
	RecentCommits  []string `json:"recent_commits"`
}

// runGitNexusCLI executes the gitnexus CLI with args inside dir. Package-level
// variable so tests can force the graceful-degradation path deterministically
// (Task-333 §10 TestStandardize_GitNexusUnavailable_FallsBackToStaticScan).
var runGitNexusCLI = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
	var cmd *exec.Cmd
	if path, err := exec.LookPath("gitnexus"); err == nil && path != "" {
		cmd = exec.CommandContext(ctx, path, args...)
	} else if npx, err := exec.LookPath("npx"); err == nil && npx != "" {
		cmd = exec.CommandContext(ctx, npx, append([]string{"--no-install", "gitnexus"}, args...)...)
	} else {
		return nil, fmt.Errorf("gitnexus CLI not available (no gitnexus/npx on PATH)")
	}
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("gitnexus %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// ssAnalyzeAttempted dedups the CP-49 R-2 auto-analyze per workspace root for
// the process lifetime (mirrors InteractiveService.gitnexusAnalyzeOnce, which
// cannot be extended without touching the shared struct).
var ssAnalyzeAttempted sync.Map

// maybeAutoAnalyzeGitNexus implements CP-49 R-2: when the workspace has no
// .gitnexus index yet, is a git repo, and the CLI is available, run
// `gitnexus analyze` with a generous timeout guard. Failures are logged and
// never fatal — the caller degrades to the static scan. No-ops in tests
// (t.TempDir() sandboxes contain no .git) so the command path never blocks.
func maybeAutoAnalyzeGitNexus(ctx context.Context, root string) {
	if _, err := os.Stat(filepath.Join(root, ".gitnexus")); err == nil {
		return // index present
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return // not a git workspace — gitnexus cannot index it
	}
	if _, loaded := ssAnalyzeAttempted.LoadOrStore(root, true); loaded {
		return // already attempted this process
	}
	actx, cancel := context.WithTimeout(ctx, gitnexusAnalyzeTimeout)
	defer cancel()
	log.Printf("[standardize] gitnexus index missing for %q — auto-running `gitnexus analyze` (CP-49 R-2)", root)
	if out, err := runGitNexusCLI(actx, root, "analyze"); err != nil {
		log.Printf("[standardize] gitnexus analyze failed (continuing with static scan): %v: %s", err, truncateRunes(string(out), 400))
	}
}

// CollectEvidence collects source evidence for scope relative to the process
// working directory (Task-333 Code Guide signature). It tries GitNexus first
// and degrades to a static directory scan + Go AST when the CLI is missing or
// fails; git log always contributes recent commits when available.
func CollectEvidence(ctx context.Context, scope StandardizeScope) (*Evidence, error) {
	return collectEvidenceIn(ctx, "", scope)
}

// collectEvidenceIn is the root-injectable core of CollectEvidence. An empty
// root means the process working directory.
func collectEvidenceIn(ctx context.Context, root string, scope StandardizeScope) (*Evidence, error) {
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("standardize: resolving workspace root: %w", err)
		}
		root = cwd
	}
	scopeDir, featureToken, err := resolveScopeDir(root, scope)
	if err != nil {
		return nil, err
	}

	maybeAutoAnalyzeGitNexus(ctx, root)

	ev := &Evidence{
		Symbols:        []string{},
		Endpoints:      []string{},
		PublicAPIs:     []string{},
		ExecutionFlows: []string{},
		RecentCommits:  []string{},
	}

	// P-1: GitNexus knowledge graph is the preferred evidence source.
	flows, graphSymbols, nexusErr := gitnexusQueryEvidence(ctx, root, featureToken, scope)
	if nexusErr != nil {
		log.Printf("[standardize] gitnexus unavailable, degrading to static scan (CP-49): %v", nexusErr)
	}
	ev.ExecutionFlows = flows
	ev.Symbols = graphSymbols

	// Graceful degradation + endpoint evidence: static directory scan and Go
	// AST always contribute what the graph does not provide.
	fileSymbols, publicAPIs, endpoints := staticScanEvidence(scopeDir, featureToken)
	if len(ev.Symbols) == 0 {
		ev.Symbols = fileSymbols
	}
	ev.PublicAPIs = publicAPIs
	ev.Endpoints = endpoints

	rel := ""
	if scope.Path != "" && scopeDir != root {
		if r, err := filepath.Rel(root, scopeDir); err == nil {
			rel = filepath.ToSlash(r)
		}
	}
	ev.RecentCommits = recentCommits(ctx, root, rel, revDocMaxCommits)
	return ev, nil
}

// resolveScopeDir maps a StandardizeScope onto a concrete directory and a
// feature token. A scope with an explicit existing directory wins; otherwise
// the scope is treated as a feature subset of the whole workspace.
func resolveScopeDir(root string, scope StandardizeScope) (string, string, error) {
	rel := filepath.ToSlash(strings.TrimSpace(scope.Path))
	feature := strings.TrimSpace(scope.FeatureName)
	if rel == "" && feature == "" {
		return root, "", nil
	}
	if rel != "" {
		dir := filepath.Join(root, filepath.FromSlash(rel))
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			token := feature
			if token == "" {
				token = strings.ToLower(filepath.Base(rel))
			}
			return dir, token, nil
		}
		// Path-like scopes (containing "/") must exist; a bare token (e.g.
		// "/standardize device") is a feature subset, not a directory.
		if strings.Contains(rel, "/") || strings.Contains(rel, string(os.PathSeparator)) {
			return "", "", fmt.Errorf("standardize: %w: %s", ErrStandardizeScopeNotFound, rel)
		}
	}
	token := strings.ToLower(feature)
	if token == "" {
		token = strings.ToLower(filepath.Base(rel))
	}
	return root, token, nil
}

// gitnexusQueryEvidence runs `gitnexus query` and extracts execution flows
// (processes) and symbol definitions from the JSON knowledge-graph output.
func gitnexusQueryEvidence(ctx context.Context, root, featureToken string, scope StandardizeScope) (flows, symbols []string, err error) {
	flows, symbols = []string{}, []string{}
	query := strings.TrimSpace(featureToken)
	if query == "" {
		query = "main execution flows"
	}
	qctx, cancel := context.WithTimeout(ctx, gitnexusQueryTimeout)
	defer cancel()
	out, qerr := runGitNexusCLI(qctx, root, "query", query, "--repo", filepath.Base(root))
	if qerr != nil {
		return flows, symbols, qerr
	}
	var payload struct {
		Processes []struct {
			Name string `json:"name"`
		} `json:"processes"`
		Definitions []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			FilePath string `json:"filePath"`
		} `json:"definitions"`
	}
	if jerr := json.Unmarshal(out, &payload); jerr != nil {
		return flows, symbols, fmt.Errorf("gitnexus query output is not parseable JSON: %w", jerr)
	}
	seenFlow := map[string]bool{}
	for _, p := range payload.Processes {
		name := strings.TrimSpace(p.Name)
		if name == "" || seenFlow[name] {
			continue
		}
		seenFlow[name] = true
		flows = append(flows, name)
		if len(flows) >= revDocMaxFlows {
			break
		}
	}
	seenSym := map[string]bool{}
	scopePrefix := ""
	if rel := filepath.ToSlash(strings.TrimSpace(scope.Path)); rel != "" {
		scopePrefix = rel
	}
	for _, d := range payload.Definitions {
		name := strings.TrimSpace(d.Name)
		if name == "" || seenSym[name] {
			continue
		}
		if scopePrefix != "" && !strings.Contains(filepath.ToSlash(d.FilePath), scopePrefix) {
			continue
		}
		seenSym[name] = true
		cite := name
		if d.FilePath != "" {
			cite = fmt.Sprintf("%s (%s)", name, filepath.ToSlash(d.FilePath))
		}
		symbols = append(symbols, cite)
		if len(symbols) >= revDocMaxSymbols {
			break
		}
	}
	return flows, symbols, nil
}

// endpointLiteralRe matches this repo's mux registration convention, e.g.
// `mux.HandleFunc("GET /client/workflow-runs", ...)` — a What-level fact.
var endpointLiteralRe = regexp.MustCompile(`"(GET|POST|PUT|DELETE|PATCH) /[A-Za-z0-9/_{}.\-]*"`)

// staticScanEvidence walks scopeDir and extracts exported Go declarations via
// go/parser plus HTTP endpoint literals. This is the CP-49 graceful
// degradation path (static directory scan + basic Go AST).
func staticScanEvidence(scopeDir, featureToken string) (symbols, publicAPIs, endpoints []string) {
	symbols, publicAPIs, endpoints = []string{}, []string{}, []string{}
	files := []string{}
	_ = filepath.WalkDir(scopeDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree — skip, never fail the command
		}
		name := d.Name()
		if d.IsDir() {
			if name == "vendor" || (strings.HasPrefix(name, ".") && path != scopeDir) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		if len(files) >= revDocMaxFiles {
			return filepath.SkipAll
		}
		if featureToken != "" {
			rel := strings.ToLower(filepath.ToSlash(path))
			if !strings.Contains(rel, featureToken) {
				return nil
			}
		}
		files = append(files, path)
		return nil
	})
	sort.Strings(files)

	seenSym := map[string]bool{}
	seenEP := map[string]bool{}
	fset := token.NewFileSet()
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		for _, m := range endpointLiteralRe.FindAllStringSubmatch(string(src), -1) {
			ep := strings.Trim(m[0], `"`)
			if ep != "" && !seenEP[ep] && len(endpoints) < revDocMaxEndpoint {
				seenEP[ep] = true
				endpoints = append(endpoints, ep)
			}
		}
		astFile, err := parser.ParseFile(fset, file, src, parser.SkipObjectResolution)
		if err != nil {
			continue // unparsable file — skip, never fail the command
		}
		pkg := astFile.Name.Name
		relFile := filepath.ToSlash(file)
		addSym := func(display string, exported bool) {
			if !exported || seenSym[display] || len(symbols) >= revDocMaxSymbols {
				return
			}
			seenSym[display] = true
			symbols = append(symbols, fmt.Sprintf("%s (%s)", display, relFile))
		}
		for _, decl := range astFile.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				recv := ""
				if decl.Recv != nil && len(decl.Recv.List) > 0 {
					recv = astTypeDisplayName(decl.Recv.List[0].Type)
				}
				exported := ast.IsExported(decl.Name.Name) || (recv != "" && ast.IsExported(recv))
				display := pkg + "." + decl.Name.Name
				if recv != "" {
					display = pkg + "." + recv + "." + decl.Name.Name
				}
				addSym(display, exported)
				if decl.Recv == nil && ast.IsExported(decl.Name.Name) && len(publicAPIs) < revDocMaxAPIs {
					publicAPIs = append(publicAPIs, fmt.Sprintf("%s (%s)", display, relFile))
				}
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					switch spec := spec.(type) {
					case *ast.TypeSpec:
						addSym(pkg+"."+spec.Name.Name, ast.IsExported(spec.Name.Name))
						if ast.IsExported(spec.Name.Name) && len(publicAPIs) < revDocMaxAPIs {
							publicAPIs = append(publicAPIs, fmt.Sprintf("%s.%s (%s)", pkg, spec.Name.Name, relFile))
						}
					case *ast.ValueSpec:
						for _, n := range spec.Names {
							addSym(pkg+"."+n.Name, ast.IsExported(n.Name))
						}
					}
				}
			}
		}
	}
	sort.Strings(publicAPIs)
	return symbols, publicAPIs, endpoints
}

// astTypeDisplayName renders an AST receiver/type expression as plain Go text
// (e.g. "*InteractiveService" or "docscan.Report").
func astTypeDisplayName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + astTypeDisplayName(t.X)
	case *ast.SelectorExpr:
		return astTypeDisplayName(t.X) + "." + t.Sel.Name
	case *ast.IndexExpr:
		return astTypeDisplayName(t.X)
	default:
		return ""
	}
}

// recentCommits reads the n most recent one-line commits touching relPath
// (or the whole repo when relPath is empty). Any git failure degrades to an
// empty list — git history evidence is optional, never fatal.
func recentCommits(ctx context.Context, root, relPath string, n int) []string {
	commits := []string{}
	gctx, cancel := context.WithTimeout(ctx, gitnexusLogTimeout)
	defer cancel()
	args := []string{"-C", root, "log", "--oneline", "-n", fmt.Sprintf("%d", n)}
	if relPath != "" && relPath != "." {
		args = append(args, "--", relPath)
	}
	out, err := exec.CommandContext(gctx, "git", args...).Output()
	if err != nil {
		return commits // not a git repo / git missing — graceful degradation
	}
	for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			commits = append(commits, ln)
		}
	}
	return commits
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ============================================================================
// Draft generation (T-2 SD / T-3 SS skeleton)
// ============================================================================

// ssLockTODO is the mandatory marker for every business-intent field the AI is
// forbidden to invent (CP-49 Hard Ceiling Rule).
const ssLockTODO = "TODO: human intent needed"

// standardizeFeatureTitle derives a deterministic TitleCase document base name
// from the scope (e.g. "features/auth" or feature "auth" -> "Auth"; the whole
// project -> "Project").
func standardizeFeatureTitle(scope StandardizeScope) string {
	token := strings.TrimSpace(scope.FeatureName)
	if token == "" {
		token = strings.TrimSpace(scope.Path)
	}
	if token == "" {
		return "Project"
	}
	token = strings.TrimPrefix(filepath.ToSlash(token), "./")
	token = strings.Trim(token, "/")
	base := token
	if i := strings.LastIndex(token, "/"); i >= 0 {
		base = token[i+1:]
	}
	base = strings.TrimSpace(strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			return r
		case r == '_' || r == ' ' || r == '.':
			return '-'
		default:
			return -1
		}
	}, base))
	if base == "" {
		return "Project"
	}
	words := strings.Split(base, "-")
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, "")
}

// standardizeScopeLabel renders the scope for document prose.
func standardizeScopeLabel(scope StandardizeScope) string {
	switch {
	case strings.TrimSpace(scope.Path) == "" && strings.TrimSpace(scope.FeatureName) == "":
		return "whole project"
	case strings.TrimSpace(scope.FeatureName) != "":
		return fmt.Sprintf("feature %q", scope.FeatureName)
	default:
		return fmt.Sprintf("scope %q", filepath.ToSlash(scope.Path))
	}
}

// draftMetadata renders the SS-13 §5.1 metadata block shared by both drafts.
func draftMetadata(docID, title, phase, parentDoc, tags string) string {
	today := time.Now().UTC().Format("2006-01-02")
	return fmt.Sprintf(`## Metadata

- Document ID: `+"`%s`"+`
- Title: `+"`%s`"+`
- Phase: `+"`%s`"+`
- Status: `+"`draft`"+`
- Owner: `+"`FlowPilot`"+`
- Reviewers: `+"`Operator`"+`
- Created: `+"`%s`"+`
- Last Updated: `+"`%s`"+`
- Parent Documents: %s
- Child Documents: `+"`None`"+`
- Related Documents: [CP-49: Reverse-Documentation](../../../07-Coding-Plan/todo/CP-49-Reverse-Documentation-And-Doc-Ingestion.md)
- Replaces: `+"`None`"+`
- Feature Keys: `+"`reverse-documentation`"+`
- Tags: `+"`%s`"+`
`, docID, title, phase, today, today, parentDoc, tags)
}

// draftAIV renders the SS-13 §5.2 AI Quick View block; intent fields stay
// TODO-flagged per CP-49.
func draftAIV(evidence *Evidence, scope StandardizeScope, intentTODO bool) string {
	var sb strings.Builder
	sb.WriteString("## AI Quick View\n\n### Summary\n\n")
	if intentTODO {
		sb.WriteString("- " + ssLockTODO + " — code only shows What, never Why (CP-49 Hard Ceiling Rule); the business summary must be written by a human at the SS-Lock gate.\n")
	} else {
		sb.WriteString(fmt.Sprintf("- As-built technical design reverse-documented from source evidence for the %s (CP-49 P-2: every claim cites its evidence).\n", standardizeScopeLabel(scope)))
	}
	sb.WriteString("\n### Current Ask\n\n")
	if intentTODO {
		sb.WriteString("- " + ssLockTODO + "\n")
	} else {
		sb.WriteString(fmt.Sprintf("- Bring the %s under the SS-13 documentation contract: draft SD below is evidence-backed; SS skeleton awaits human intent at SS-Lock.\n", standardizeScopeLabel(scope)))
	}
	sb.WriteString("\n### Key Decisions\n\n")
	if intentTODO {
		sb.WriteString("- " + ssLockTODO + "\n")
	} else {
		if len(evidence.ExecutionFlows) > 0 {
			sb.WriteString(fmt.Sprintf("- Reverse-documented architecture follows %d observed execution flow(s) from GitNexus.\n", len(evidence.ExecutionFlows)))
		} else {
			sb.WriteString("- Architecture claims are limited to statically verifiable symbols; no flow evidence was available.\n")
		}
	}
	sb.WriteString("\n### Constraints\n\n" +
		"- Read-only on project source code; only draft documents are written under `requirements/` `todo/` directories.\n" +
		"- SS business intent is never invented by AI (CP-49 Hard Ceiling Rule).\n" +
		"\n### Open Questions\n\n" +
		"- " + ssLockTODO + "\n" +
		"\n### Source Refs\n\n" +
		"- [CP-49: Reverse-Documentation](../../../07-Coding-Plan/todo/CP-49-Reverse-Documentation-And-Doc-Ingestion.md)\n" +
		"- [SS-13: AI-Followable Document Contract](../../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md)\n")
	return sb.String()
}

// evidenceBullets renders deterministic, cited evidence bullet lists.
func evidenceBullets(items []string, emptyMsg string) string {
	if len(items) == 0 {
		return "- TODO: " + emptyMsg + "\n"
	}
	var sb strings.Builder
	for _, it := range items {
		sb.WriteString("- " + it + "\n")
	}
	return sb.String()
}

// GenerateDraftSD synthesizes the as-built System Tech Design draft from
// collected evidence (Task-333 Code Guide). Deterministic: no LLM call. Every
// claim cites a symbol, file, endpoint, flow, or commit; absent evidence is
// flagged TODO (anti-hallucination, CP-49 P-2). Returns the markdown content.
func GenerateDraftSD(ctx context.Context, evidence *Evidence, scope StandardizeScope) (string, error) {
	if evidence == nil {
		return "", fmt.Errorf("standardize: evidence is nil, cannot generate a grounded SD draft")
	}
	title := standardizeFeatureTitle(scope)
	docID := "SD-" + title
	fullTitle := fmt.Sprintf("%s — As-Built Technical Design (Reverse-Doc Draft)", title)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s: %s\n\n", docID, fullTitle))
	sb.WriteString(draftMetadata(docID, fullTitle, "sd",
		"[SS-"+title+" draft](../../05-System-Specs/todo/SS-"+title+".md)",
		"reverse-doc, draft, as-built"))
	sb.WriteString("\n" + draftAIV(evidence, scope, false) + "\n")

	sb.WriteString(fmt.Sprintf("## 1. Goal\n\n- Capture the as-built technical design of the %s from verifiable source evidence, so the brownfield scope can be governed by the SS-13 document contract.\n\n", standardizeScopeLabel(scope)))
	sb.WriteString("## 2. Input Documents\n\n- CP-49 Reverse-Documentation plan; evidence collected in-process (see Traceability to Spec).\n\n")

	sb.WriteString("## 3. Architecture Decision\n\n")
	if len(evidence.ExecutionFlows) > 0 {
		sb.WriteString(fmt.Sprintf("- The scope executes as %d GitNexus-observed flow(s); the as-built architecture mirrors them:\n", len(evidence.ExecutionFlows)))
		for _, f := range evidence.ExecutionFlows {
			sb.WriteString("  - Execution flow (evidence: GitNexus knowledge graph): " + f + "\n")
		}
	} else {
		sb.WriteString("- TODO: no execution-flow evidence was collectable (GitNexus unavailable or no indexed flows); the flow-level architecture needs human verification.\n")
	}
	if len(evidence.Symbols) > 0 {
		sb.WriteString(fmt.Sprintf("- Structure is grounded in %d collected symbol(s):\n", len(evidence.Symbols)))
		for i, s := range evidence.Symbols {
			if i >= 20 {
				sb.WriteString(fmt.Sprintf("  - … and %d more symbols (evidence: evidence set truncated for readability).\n", len(evidence.Symbols)-20))
				break
			}
			sb.WriteString("  - Symbol (evidence: symbol graph/static AST): " + s + "\n")
		}
	} else {
		sb.WriteString("- TODO: no symbol evidence collected for this scope; component claims must not be made without evidence.\n")
	}
	sb.WriteString("\n")

	sb.WriteString("## 4. Component Impact\n\n")
	sb.WriteString(evidenceBullets(evidence.PublicAPIs, "no exported API evidence collected — human review required before asserting component impact"))
	sb.WriteString("\n## 5. Data Model\n\n")
	sb.WriteString("- TODO: data-model evidence is not collectable from the current evidence set; document persistence/schema claims after human review.\n\n")

	sb.WriteString("## 6. Interfaces and Contracts\n\n")
	sb.WriteString(evidenceBullets(evidence.Endpoints, "no HTTP endpoint literals found in scope — interfaces section needs human input"))
	sb.WriteString("\n## 7. Execution Flow\n\n")
	if len(evidence.ExecutionFlows) > 0 {
		for _, f := range evidence.ExecutionFlows {
			sb.WriteString("- " + f + " (evidence: GitNexus execution flows)\n")
		}
	} else {
		sb.WriteString("- TODO: no execution-flow evidence available; describe the runtime flow only after human verification.\n")
	}
	sb.WriteString("\n## 8. Failure and Edge Handling\n\n")
	if len(evidence.RecentCommits) > 0 {
		sb.WriteString("- Recent commits touching the scope may indicate known failure handling work:\n")
		for _, c := range evidence.RecentCommits {
			sb.WriteString("  - Commit (evidence: `git log --oneline`): " + c + "\n")
		}
	} else {
		sb.WriteString("- TODO: no git commit evidence available for the scope.\n")
	}
	sb.WriteString("\n## 9. Security and Operational Concerns\n\n")
	sb.WriteString("- " + ssLockTODO + " — security intent is a business decision and is never inferred from code (CP-49).\n\n")

	sb.WriteString("## 10. Risks and Trade-Offs\n\n" +
		"- Reverse-documented design reflects the code as it is, including technical debt; it is not a prescription.\n\n")
	sb.WriteString("## 11. Validation Strategy\n\n" +
		"- The draft must pass the CP-48 SS-13 conformance scan (docscan) after human review at SS-Lock.\n" +
		"- Deterministic generation: identical evidence yields a byte-identical draft (provider-agnostic, 0 LLM tokens).\n\n")
	sb.WriteString("## 12. Traceability to Spec\n\n" +
		"- Every claim above cites its evidence: symbol (symbol graph / static AST), endpoint (source literal), execution flow (GitNexus), or commit (`git log`).\n" +
		"- Sections flagged `TODO` have no collected evidence; inventing them is forbidden by CP-49 P-2.\n")
	return sb.String(), nil
}

// ssSkeletonSections are the SS canonical sections (docscan rules, SS-13 §6)
// that the skeleton pre-conforms to. Sections that carry business intent get
// the `TODO: human intent needed` marker.
var ssSkeletonSections = []string{
	"Goal",
	"Problem",
	"Scope",
	"Non-Goals",
	"User Stories or Primary Use Cases",
	"Acceptance Criteria",
	"Business Rules",
	"Edge Cases",
	"Dependencies",
	"Open Questions",
	"Definition of Done",
}

// ssIntentSections are the numbered SS sections whose content is pure business
// intent — AI may only leave the TODO marker there (CP-49 Hard Ceiling Rule).
var ssIntentSections = map[string]bool{
	"Goal":                              true,
	"Problem":                           true,
	"Non-Goals":                         true,
	"User Stories or Primary Use Cases": true,
	"Acceptance Criteria":               true,
	"Business Rules":                    true,
}

// GenerateDraftSS builds the SS skeleton (Task-333 T-3). AI never invents
// business intent: every intent-bearing section (including `## Acceptance
// Criteria`) carries `TODO: human intent needed` markers; only evidence-derived
// factual bullets (scope refs, source refs) are pre-filled.
func GenerateDraftSS(ctx context.Context, evidence *Evidence, scope StandardizeScope) (string, error) {
	title := standardizeFeatureTitle(scope)
	docID := "SS-" + title
	fullTitle := fmt.Sprintf("%s — System Spec (SS-Lock Draft)", title)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s: %s\n\n", docID, fullTitle))
	sb.WriteString(draftMetadata(docID, fullTitle, "ss",
		"[CP-49: Reverse-Documentation](../../../07-Coding-Plan/todo/CP-49-Reverse-Documentation-And-Doc-Ingestion.md)",
		"reverse-doc, draft, ss-lock"))
	sb.WriteString("\n" + draftAIV(evidence, scope, true) + "\n")

	for i, name := range ssSkeletonSections {
		sb.WriteString(fmt.Sprintf("## %d. %s\n\n", i+1, name))
		if ssIntentSections[name] {
			sb.WriteString("- " + ssLockTODO + "\n")
			if name == "Acceptance Criteria" {
				sb.WriteString("- " + ssLockTODO + " — refine the acceptance criteria and press approve at the SS-Lock gate.\n")
				sb.WriteString("- " + ssLockTODO + " — each criterion must be human-owned; AI never drafts them (CP-49 P-3).\n")
			}
		}
		switch name {
		case "Scope":
			sb.WriteString(fmt.Sprintf("- Evidence-derived scope fact: this spec governs the %s; source-level What is recorded in the paired SD draft.\n", standardizeScopeLabel(scope)))
		case "Edge Cases":
			sb.WriteString("- " + ssLockTODO + " — edge cases without code evidence must be enumerated by a human.\n")
		case "Dependencies":
			if evidence != nil && len(evidence.Endpoints) > 0 {
				sb.WriteString("- Observed inbound HTTP interfaces (evidence: source literals):\n")
				for _, ep := range evidence.Endpoints {
					sb.WriteString("  - " + ep + "\n")
				}
			} else {
				sb.WriteString("- TODO: no dependency evidence collected; list dependencies after human review.\n")
			}
		case "Open Questions":
			sb.WriteString("- " + ssLockTODO + "\n")
		case "Definition of Done":
			sb.WriteString("- [ ] SS-Lock gate approved by a human (non-bypassable, CP-49 P-3).\n" +
				"- [ ] Draft passes the CP-48 docscan conformance scan after approval.\n")
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}
