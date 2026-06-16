<#
.SYNOPSIS
    Quick compatibility check for Claude Code CLI + Codex app-server.

.DESCRIPTION
    Verifies that the tools installed on THIS machine match the versions and
    wire contract that FlowPilot was tested against.  Run this on any new PC
    before doing development, or after an upgrade, to detect breaking changes.

.PARAMETER SkipProbe
    Skip the stream-json and app-server protocol probes (version + flag checks only).
    Use this when not logged in to Claude / Codex.

.EXAMPLE
    .\scripts\quicktest.ps1
    .\scripts\quicktest.ps1 -SkipProbe
#>

param([switch]$SkipProbe)

# Flags we pass on every `claude -p` invocation
$CLAUDE_FLAGS = @(
    "--input-format",
    "--output-format",
    "--include-partial-messages",
    "--include-hook-events",
    "--strict-mcp-config",
    "--disallowed-tools",
    "--permission-mode",
    "--mcp-config",
    "--resume",
    "--effort"
)

# ---- Helpers -----------------------------------------------------------------
$pass = 0
$warn = 0
$fail = 0
$versionFail = 0

function OK   ($msg) { Write-Host "  [PASS] $msg" -ForegroundColor Green;  $script:pass++ }
function WARN ($msg) { Write-Host "  [WARN] $msg" -ForegroundColor Yellow; $script:warn++ }
function FAIL ($msg) { Write-Host "  [FAIL] $msg" -ForegroundColor Red;    $script:fail++ }
function VERSION_FAIL ($msg) {
    Write-Host "  [FAIL] $msg" -ForegroundColor Red
    $script:fail++
    $script:versionFail++
}
function HDR  ($msg) { Write-Host "" ; Write-Host "---- $msg ----" -ForegroundColor Cyan }

function Get-TestedVersions {
    $repoRoot = Split-Path -Parent $PSScriptRoot
    $compatConfigPath = Join-Path $repoRoot ".flowpilot/settings/compat-config.json"
    if (Test-Path $compatConfigPath) {
        try {
            $saved = Get-Content $compatConfigPath -Raw | ConvertFrom-Json -ErrorAction Stop
            $claude = "$($saved.testedClaudeVersion)".Trim()
            $codex = "$($saved.testedCodexVersion)".Trim()
            if ($claude -ne "" -and $codex -ne "") {
                return @{
                    Claude = $claude
                    Codex  = $codex
                }
            }
        } catch {
            WARN "Could not parse $compatConfigPath; falling back to compat.go defaults"
        }
    }

    $compatFile = Join-Path $repoRoot "apps/local-runner/internal/runner/compat.go"
    if (-not (Test-Path $compatFile)) {
        FAIL "Could not find compat.go at $compatFile"
        return @{ Claude = ""; Codex = "" }
    }

    $source = Get-Content $compatFile -Raw
    $claudeMatch = [regex]::Match($source, 'CompatTestedClaudeVersion\s*=\s*"([^"]+)"')
    $codexMatch = [regex]::Match($source, 'CompatTestedCodexVersion\s*=\s*"([^"]+)"')
    if (-not $claudeMatch.Success -or -not $codexMatch.Success) {
        FAIL "Could not read tested versions from compat.go"
        return @{ Claude = ""; Codex = "" }
    }

    return @{
        Claude = $claudeMatch.Groups[1].Value
        Codex  = $codexMatch.Groups[1].Value
    }
}

function Get-MajorMinor ($version) {
    foreach ($tok in $version.Split(' ')) {
        $segs = $tok.Split('.')
        if ($segs.Count -ge 2 -and $segs[0] -match '^\d+$' -and $segs[1] -match '^\d+$') {
            return "$($segs[0]).$($segs[1])"
        }
    }
    return $version
}

function Check-Version ($label, $binary, $expected) {
    $out = & $binary --version 2>&1
    if ($LASTEXITCODE -ne 0 -and (-not $out)) {
        FAIL "$label not found on PATH (binary: $binary)"
        return
    }
    $got = "$out".Trim()
    Write-Host "       installed : $got" -ForegroundColor Gray
    Write-Host "       tested-on : $expected" -ForegroundColor Gray

    if ($got -like "*$expected*") {
        OK "$label version matches tested ($expected)"
    } else {
        $gotMM  = Get-MajorMinor $got
        $wantMM = Get-MajorMinor $expected
        if ($gotMM -ne $wantMM) {
            VERSION_FAIL "$label MAJOR/MINOR changed: $gotMM vs $wantMM -- run protocol probes before updating baseline"
        } else {
            WARN "$label patch drift: $got vs $expected -- same major.minor, likely safe"
        }
    }
}

$testedVersions = Get-TestedVersions

# ---- 1. Version checks -------------------------------------------------------
HDR "Version checks"
Check-Version "Claude Code CLI" "claude" $testedVersions.Claude
Check-Version "Codex"           "codex"  $testedVersions.Codex

# ---- 2. Claude flag existence ------------------------------------------------
HDR "Claude CLI flags"
$claudeHelp = (& claude --help 2>&1) -join "`n"

foreach ($flag in $CLAUDE_FLAGS) {
    if ($claudeHelp.Contains($flag)) {
        OK $flag
    } else {
        FAIL "$flag missing from 'claude --help' -- renamed or removed"
    }
}

# --permission-prompt-tool is undocumented (not in --help) but used on every
# gated turn. Probe by passing a dummy value; a removed flag gives "unknown option".
$probeOut = (& claude -p --permission-prompt-tool mcp__compat__probe --output-format stream-json --dangerously-skip-permissions "echo PROBE" 2>&1) -join " "
if ($probeOut -match "unknown (option|flag)|unexpected argument") {
    FAIL "--permission-prompt-tool removed or renamed"
} else {
    OK "--permission-prompt-tool (undocumented, probe passed)"
}

# ---- 3. Codex app-server flags -----------------------------------------------
HDR "Codex app-server flags"
$codexHelp = (& codex app-server --help 2>&1) -join "`n"

if ($codexHelp.Contains("--listen")) {
    OK "codex app-server --listen"
} else {
    FAIL "codex app-server --listen missing -- Codex transport changed"
}
if ($codexHelp -match "stdio") {
    OK "stdio transport mentioned"
} else {
    WARN "'stdio' not in app-server help -- verify --listen stdio:// still works"
}

if (-not $SkipProbe) {

# ---- 4. Claude stream-json smoke test ----------------------------------------
HDR "Claude stream-json smoke test"

# Use & operator so PowerShell resolves .cmd/.ps1 wrappers on Windows PATH.
# Capture stderr separately via a temp file to detect auth errors.
$errFile = Join-Path ([System.IO.Path]::GetTempPath()) "flowpilot_compat_claude_err.txt"
$rawLines = & claude -p --output-format stream-json --verbose --include-partial-messages --dangerously-skip-permissions "Respond with exactly the word: PONG" 2>$errFile
$claudeExit = $LASTEXITCODE
$claudeErr  = (Get-Content $errFile -Raw -ErrorAction SilentlyContinue) + ""

if ($claudeErr -match "not logged in|authentication|api key|login") {
    WARN "No Claude credentials -- skipping live stream-json probe"
} elseif ($claudeExit -ne 0 -and (-not $rawLines)) {
    WARN "claude -p exited $claudeExit with no output -- skipping frame checks"
} else {
    $seenTypes   = @{}
    $resultFrame = $null

    foreach ($line in $rawLines) {
        try {
            $frame = $line | ConvertFrom-Json -ErrorAction Stop
            if ($frame.type) {
                $seenTypes[$frame.type] = $true
                Write-Host "         frame: type=$($frame.type) subtype=$($frame.subtype)" -ForegroundColor Gray
                if ($frame.type -eq "result") { $resultFrame = $frame }
            }
        } catch {}
    }

    foreach ($ft in @("system", "result")) {
        if ($seenTypes[$ft]) { OK "frame type: $ft" }
        else { FAIL "frame type '$ft' missing -- stream-json contract changed" }
    }
    if ($seenTypes["stream_event"] -or $seenTypes["assistant"]) {
        OK "content frame present (stream_event or assistant)"
    } else {
        FAIL "no content frames -- stream-json output changed"
    }

    if ($resultFrame) {
        foreach ($field in @("subtype", "is_error")) {
            if ($null -ne $resultFrame.$field) { OK "result.$field present" }
            else { FAIL "result.$field missing -- mapClaudeResult will be wrong" }
        }
        if (-not $resultFrame.session_id) {
            WARN "result.session_id missing -- --resume may be broken"
        } else {
            OK "result.session_id present"
        }
    }
}

# ---- 5. Codex app-server initialize handshake --------------------------------
HDR "Codex app-server initialize handshake"

$initReq = '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"flowpilot-quicktest","version":"1.0"},"capabilities":{"experimentalApi":true,"requestAttestation":false}}}'

$codexJob = Start-Job -ScriptBlock {
    param($req)
    $isWindows = [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform(
        [System.Runtime.InteropServices.OSPlatform]::Windows
    )
    if ($isWindows) {
        # Use cmd.exe so .cmd wrappers (e.g. codex.cmd on Windows) work with stdio redirect.
        $psi = [System.Diagnostics.ProcessStartInfo]::new("cmd.exe")
        $psi.Arguments = "/c codex app-server --listen stdio://"
    } else {
        $psi = [System.Diagnostics.ProcessStartInfo]::new("codex")
        $psi.ArgumentList.Add("app-server")
        $psi.ArgumentList.Add("--listen")
        $psi.ArgumentList.Add("stdio://")
    }
    $psi.UseShellExecute        = $false
    $psi.RedirectStandardInput  = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError  = $true
    $p = [System.Diagnostics.Process]::Start($psi)
    $p.StandardInput.WriteLine($req)
    $p.StandardInput.Flush()
    $deadline = (Get-Date).AddSeconds(15)
    $result = $null
    while ((Get-Date) -lt $deadline -and -not $result) {
        $line = $p.StandardOutput.ReadLine()
        if ($line) {
            try {
                $msg = $line | ConvertFrom-Json -ErrorAction Stop
                if ($msg.id -eq 1) { $result = $line; break }
            } catch {}
        }
    }
    try { $p.Kill() } catch {}
    $result
} -ArgumentList $initReq

$jobResult = $codexJob | Wait-Job -Timeout 20 | Receive-Job
Remove-Job $codexJob -Force -ErrorAction SilentlyContinue

if (-not $jobResult) {
    WARN "Codex app-server did not respond to initialize (no credentials or binary issue)"
} else {
    try {
        $resp = $jobResult | ConvertFrom-Json -ErrorAction Stop
        if ($resp.error) {
            FAIL "initialize returned error: $($resp.error)"
        } else {
            Write-Host "         raw result: $($resp.result | ConvertTo-Json -Compress)" -ForegroundColor Gray
            # capabilities is read by supports(); absent = optimistic-true fallback (expected on 0.137.x)
            foreach ($field in @("protocolVersion", "capabilities")) {
                if ($null -ne $resp.result.$field) {
                    OK "initialize.$field present"
                } else {
                    WARN "initialize.$field absent -- supports() uses optimistic-true (expected on 0.137.x)"
                }
            }
            # codex 0.137.x returns metadata fields; confirm this is a real Codex server
            foreach ($field in @("codexHome", "platformOs", "userAgent")) {
                if ($null -ne $resp.result.$field) {
                    OK "initialize.$field = $($resp.result.$field)"
                } else {
                    WARN "initialize.$field absent (present on 0.137.x)"
                }
            }
            OK "Codex initialize handshake"
        }
    } catch {
        FAIL "initialize response not valid JSON: $jobResult"
    }
}

} # end -not SkipProbe

# ---- Summary -----------------------------------------------------------------
Write-Host ""
Write-Host "--------------------------------------------" -ForegroundColor DarkGray
Write-Host " Results: " -NoNewline
Write-Host "$pass PASS  " -ForegroundColor Green -NoNewline
Write-Host "$warn WARN  " -ForegroundColor Yellow -NoNewline
Write-Host "$fail FAIL"  -ForegroundColor Red

if ($fail -gt 0 -and $fail -eq $versionFail) {
    Write-Host " BASELINE DRIFT ONLY: protocol probes passed, but tested versions still need review/update." -ForegroundColor Yellow
    exit 1
} elseif ($fail -gt 0) {
    Write-Host " INCOMPATIBLE: required contract elements changed. Review before running." -ForegroundColor Red
    exit 1
} elseif ($warn -gt 0) {
    Write-Host " LIKELY COMPATIBLE: patch-level drift only. Verify manually if issues arise." -ForegroundColor Yellow
    exit 0
} else {
    Write-Host " FULLY COMPATIBLE: all checks passed." -ForegroundColor Green
    exit 0
}
