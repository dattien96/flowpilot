package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// Task-402 (CP-70) follow-up: the live Devin model probe the cache header
// promised but never wired. `devin models list` needs the REPL credential
// store (a different store from ACP PKCE), so the probe harvests the catalog
// ACP itself advertises: spawn a throwaway `devin acp`, run the boot
// handshake (initialize -> authenticate -> session/new), and read the
// session's "model" config option choices. The spawn + handshakes run tens
// of seconds on a cold start — far beyond the /providers 8s budget — so
// detection never calls this synchronously; warmDevinModelsCacheAsync owns it.

// devinModelsProbeTimeout bounds one warm attempt end to end.
const devinModelsProbeTimeout = 3 * time.Minute

// devinModelsProbeSessionTimeout bounds just the session/new call inside a
// probe — the catalog carrier.
const devinModelsProbeSessionTimeout = 45 * time.Second

// detectDevinModelsLive spawns a throwaway `devin acp`, harvests the session's
// model catalog, records it into the cache, and returns the ProviderModel
// list (bare catalog ids — callers prefix "devin/" via
// devinPrefixedProviderModels).
func detectDevinModelsLive(ctx context.Context) ([]ProviderModel, error) {
	ctx, cancel := context.WithTimeout(ctx, devinModelsProbeTimeout)
	defer cancel()

	cmd := commandContextFn(ctx, devinSpawnBinary(), "acp")
	cmd.Env = devinProcessEnv(nil)
	// session/new requires a cwd; probe in the temp dir so no user workspace
	// is touched and no session artifact lands in a project the user cares
	// about.
	cmd.Dir = os.TempDir()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("devin acp stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("devin acp stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("devin acp start: %w", err)
	}
	dispatcher := newDevinDispatcher(stdin, nil)
	dispatcher.start(stdout)
	defer func() {
		dispatcher.fail(fmt.Errorf("devin model probe done"))
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	return probeDevinModelCatalog(ctx, dispatcher, cmd.Dir)
}

// probeDevinModelCatalog runs initialize -> authenticate -> session/new on an
// already-wired dispatcher and returns the session's model catalog. Split from
// the spawn so tests can drive it over in-memory pipes (startFakeDevin).
func probeDevinModelCatalog(ctx context.Context, dispatcher *devinDispatcher, cwd string) ([]ProviderModel, error) {
	initCtx, initCancel := context.WithTimeout(ctx, devinInitTimeout)
	_, err := dispatcher.call(initCtx, "initialize", devinACPInitializeParams())
	initCancel()
	if err != nil {
		return nil, fmt.Errorf("devin initialize: %w", err)
	}

	authCtx, authCancel := context.WithTimeout(ctx, devinAuthTimeout)
	_, err = dispatcher.call(authCtx, "authenticate", devinACPAuthenticateParams())
	authCancel()
	if err != nil {
		return nil, fmt.Errorf("devin authenticate: %w", err)
	}

	sessCtx, sessCancel := context.WithTimeout(ctx, devinModelsProbeSessionTimeout)
	result, err := dispatcher.call(sessCtx, "session/new", devinACPSessionNewParams(cwd, nil))
	sessCancel()
	if err != nil {
		return nil, fmt.Errorf("devin session/new: %w", err)
	}

	choices := devinConfigOptionChoices(devinConfigOptionFromResult(result, "model"))
	// Task-438: new-schema CLIs expose reasoning as a session-level
	// "thought_level" option rather than effort-suffixed model ids — harvest
	// its values so the detected models carry the real reasoning capabilities.
	thoughts := devinConfigOptionChoices(devinConfigOptionFromResult(result, "thought_level"))
	levels := make([]string, 0, len(thoughts))
	for _, c := range thoughts {
		if v := strings.TrimSpace(c.Value); v != "" {
			levels = append(levels, v)
		}
	}
	models := devinCatalogChoicesToProviderModelsWithEfforts(choices, levels)
	if len(models) == 0 {
		return nil, fmt.Errorf("devin session/new returned no model options")
	}
	writeDevinModelsCache(models)
	return models, nil
}

// warmDevinModelsCacheAsync refreshes the catalog in the background so the
// next detection pass sees the live list. Guarded by devinModelsWarmInFlight
// so overlapping /providers hits spawn at most one probe.
func warmDevinModelsCacheAsync() {
	// Under `go test` the ambient `devin` binary must never spawn; the env
	// override lets a test opt in with a scripted fake (FLOWPILOT_DEVIN_BIN).
	if runningUnderGoTest() && os.Getenv("FLOWPILOT_DEVIN_BIN") == "" {
		return
	}
	if !devinModelsWarmInFlight.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer devinModelsWarmInFlight.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), devinModelsProbeTimeout)
		defer cancel()
		if _, err := detectDevinModelsLive(ctx); err != nil {
			log.Printf("[devin] model catalog probe failed: %v", err)
		}
	}()
}
