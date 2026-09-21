param(
    [switch]$WithGemini
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path ".\go.mod")) {
    throw "Run this script from the ParamIntel repository root."
}

$labDir = ".\labs\v0.11-evidence-guided-rescue"
$outDir = ".\.paramintel\v0.11-lab"
New-Item -ItemType Directory -Force -Path $outDir | Out-Null

$paramIntelExe = Join-Path $outDir "paramintel.exe"
$labExe = Join-Path $outDir "v0.11-lab.exe"

Write-Host "[*] Building ParamIntel and the v0.11 lab"
go build -trimpath -o $paramIntelExe .\cmd\paramintel
if ($LASTEXITCODE -ne 0) { throw "ParamIntel build failed." }

go build -trimpath -o $labExe .\labs\v0.11-evidence-guided-rescue
if ($LASTEXITCODE -ne 0) { throw "Lab build failed." }

$paramIntelExe = (Resolve-Path $paramIntelExe).Path
$labExe = (Resolve-Path $labExe).Path

$expected = Get-Content (Join-Path $labDir "expected.json") -Raw | ConvertFrom-Json
$scenario = @{}
foreach ($entry in $expected.scenarios) {
    $scenario[$entry.id] = $entry
}

function Assert-Equal {
    param(
        [Parameter(Mandatory=$true)]$Actual,
        [Parameter(Mandatory=$true)]$Expected,
        [Parameter(Mandatory=$true)][string]$Label
    )
    if ($Actual -ne $Expected) {
        throw "${Label}: got '$Actual', want '$Expected'"
    }
}

function Invoke-Scenario {
    param(
        [Parameter(Mandatory=$true)][string]$Name,
        [Parameter(Mandatory=$true)][string]$Request,
        [Parameter(Mandatory=$true)][string]$Wordlist,
        [Parameter(Mandatory=$true)][int]$Budget,
        [Parameter(Mandatory=$true)][string]$Output,
        [string[]]$ExtraArgs = @()
    )

    Write-Host ""
    Write-Host "[*] $Name"

    $args = @(
        "-request", $Request,
        "-wordlist", $Wordlist,
        "-scheme", "http",
        "-baseline", "3",
        "-trials", "3",
        "-chunk", "64",
        "-characterize=false",
        "-value-aware",
        "-value-aware-budget", "$Budget",
        "-output", $Output
    ) + $ExtraArgs

    & $paramIntelExe @args
    if ($LASTEXITCODE -ne 0) {
        throw "$Name failed with exit code $LASTEXITCODE"
    }

    return Get-Content $Output -Raw | ConvertFrom-Json
}

$lab = $null
try {
    Write-Host "[*] Starting localhost lab on 127.0.0.1:41782"
    $lab = Start-Process -FilePath $labExe -PassThru -WindowStyle Hidden

    $ready = $false
    for ($i = 0; $i -lt 20; $i++) {
        try {
            Invoke-RestMethod -Uri "http://127.0.0.1:41782/no-signal" -TimeoutSec 1 | Out-Null
            $ready = $true
            break
        } catch {
            Start-Sleep -Milliseconds 250
        }
    }
    if (-not $ready) {
        throw "Lab did not become ready on 127.0.0.1:41782."
    }

    $a = $scenario["context-ranking"]
    $aParams = @{
        Name = "Scenario A - context ranking"
        Request = Join-Path $labDir $a.request
        Wordlist = Join-Path $labDir $a.wordlist
        Budget = [int]$a.budget
        Output = Join-Path $outDir "context-ranking.json"
    }
    $aReport = Invoke-Scenario @aParams
    Assert-Equal $aReport.parameters.Count $a.expected_verified_parameters "Scenario A verified parameter count"
    Assert-Equal $aReport.parameters[0].name $a.parameter "Scenario A parameter"
    Assert-Equal $aReport.parameters[0].discovery_value $a.value "Scenario A discovery value"
    Assert-Equal $aReport.value_aware.requests_used $a.expected_rescue_requests "Scenario A rescue request count"
    Assert-Equal $aReport.value_aware.verified_requests $a.expected_rescue_requests "Scenario A verified request cost"
    Write-Host "[PASS] Scenario A: $($a.parameter)=$($a.value), $($aReport.value_aware.requests_used) rescue requests"

    $b = $scenario["cost-tie-breaker"]
    $bParams = @{
        Name = "Scenario B - cost tie-breaker"
        Request = Join-Path $labDir $b.request
        Wordlist = Join-Path $labDir $b.wordlist
        Budget = [int]$b.budget
        Output = Join-Path $outDir "cost-tie-breaker.json"
    }
    $bReport = Invoke-Scenario @bParams
    Assert-Equal $bReport.parameters.Count $b.expected_verified_parameters "Scenario B verified parameter count"
    Assert-Equal $bReport.parameters[0].name $b.parameter "Scenario B parameter"
    Assert-Equal $bReport.parameters[0].discovery_value $b.value "Scenario B discovery value"
    Assert-Equal $bReport.value_aware.requests_used $b.expected_rescue_requests "Scenario B rescue request count"
    Write-Host "[PASS] Scenario B: $($b.parameter)=$($b.value), $($bReport.value_aware.requests_used) rescue requests"

    $c = $scenario["zero-signal"]
    $cParams = @{
        Name = "Scenario C - zero signal"
        Request = Join-Path $labDir $c.request
        Wordlist = Join-Path $labDir $c.wordlist
        Budget = [int]$c.budget
        Output = Join-Path $outDir "zero-signal.json"
    }
    $cReport = Invoke-Scenario @cParams
    Assert-Equal $cReport.parameters.Count $c.expected_verified_parameters "Scenario C verified parameter count"
    Assert-Equal $cReport.value_aware.requests_used $c.expected_rescue_requests "Scenario C rescue request count"
    Assert-Equal $cReport.value_aware.miss_requests $c.expected_miss_requests "Scenario C miss request cost"
    Write-Host "[PASS] Scenario C: 0 findings, $($cReport.value_aware.requests_used) miss requests"

    $d = $scenario["gemini-semantic-value"]
    $dControlParams = @{
        Name = "Scenario D control - Gemini disabled"
        Request = Join-Path $labDir $d.request
        Wordlist = Join-Path $labDir $d.wordlist
        Budget = [int]$d.budget
        Output = Join-Path $outDir "gemini-control.json"
    }
    $dControl = Invoke-Scenario @dControlParams
    Assert-Equal $dControl.parameters.Count $d.control_expected_verified_parameters "Scenario D control verified parameter count"
    Write-Host "[PASS] Scenario D control: visibility is not proven without semantic-value AI"

    if ($WithGemini) {
        if ([string]::IsNullOrWhiteSpace($env:GEMINI_API_KEY)) {
            Write-Warning "GEMINI_API_KEY is not set. Skipping Gemini scenario."
        } else {
            $dAIParams = @{
                Name = "Scenario D - Gemini semantic value"
                Request = Join-Path $labDir $d.request
                Wordlist = Join-Path $labDir $d.wordlist
                Budget = [int]$d.budget
                Output = Join-Path $outDir "gemini-ai.json"
                ExtraArgs = @(
                    "-ai-value-advisor",
                    "-ai-provider", "gemini",
                    "-ai-value-budget", "4",
                    "-ai-value-candidate-budget", "1",
                    "-verbose"
                )
            }
            $dAI = Invoke-Scenario @dAIParams
            $visibility = @($dAI.parameters | Where-Object { $_.name -eq $d.parameter })
            if ($visibility.Count -eq 1 -and $visibility[0].discovery_mode -eq $d.ai_expected_discovery_mode) {
                Write-Host "[PASS] Scenario D Gemini: visibility=internal independently verified"
            } else {
                Write-Warning "Scenario D Gemini was inconclusive. The model suggestion is not treated as evidence."
                if ($null -ne $dAI.ai_value_advisor) {
                    Write-Host ("    provider/model: {0}/{1}" -f $dAI.ai_value_advisor.provider, $dAI.ai_value_advisor.model)
                    Write-Host ("    candidate queries: {0}" -f $dAI.ai_value_advisor.candidate_queries)
                    Write-Host ("    suggested/accepted values: {0}/{1}" -f $dAI.ai_value_advisor.suggested_values, $dAI.ai_value_advisor.accepted_values)
                    Write-Host ("    verified parameters: {0}" -f $dAI.ai_value_advisor.verified_parameters)
                }
                if ($null -ne $dAI.value_aware) {
                    Write-Host ("    rescue requests: {0}/{1}" -f $dAI.value_aware.requests_used, $dAI.value_aware.budget)
                    foreach ($audit in $dAI.value_aware.candidate_audit) {
                        if ($audit.ai_queried) {
                            Write-Host ("    AI candidate audit: {0} tier={1} relevance={2} ai_values={3} requests={4} outcome={5}" -f $audit.name, $audit.evidence_tier, $audit.context_relevance, $audit.ai_values, $audit.requests_used, $audit.outcome)
                        }
                    }
                }
            }
        }
    }

    Write-Host ""
    Write-Host "=== v0.11 dedicated lab result ==="
    Write-Host "Deterministic scenarios: PASS"
    if ($WithGemini) {
        Write-Host "Gemini scenario: see status above"
    } else {
        Write-Host "Gemini scenario: skipped (use -WithGemini when desired)"
    }
    Write-Host "Reports: $outDir"
}
finally {
    if ($null -ne $lab -and -not $lab.HasExited) {
        Stop-Process -Id $lab.Id -Force
    }
}
