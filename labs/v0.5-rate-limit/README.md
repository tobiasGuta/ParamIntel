# ParamIntel v0.5 external acceptance lab

This lab exercises the release-level behavior added in v0.5.0 using the real CLI on Windows.

It provides three endpoints:

- `/mid-scan`: baseline traffic is normal, but active query probes receive `429 Retry-After: 2`;
- `/asymmetric`: a contextual nested JSON candidate changes behavior normally, while its generated `zz_pi_...` random-name control receives `429`;
- `/pace`: always returns a baseline-like response and logs request-start gaps for `-delay` validation.

## 1. Start the lab

From the ParamIntel repository root:

```powershell
go run .\labs\v0.5-rate-limit\main.go
```

Expected startup message:

```text
ParamIntel v0.5 acceptance lab listening on http://127.0.0.1:8092
```

Keep that PowerShell window open.

In a second PowerShell window, build the feature branch CLI:

```powershell
go build -o paramintel.exe .\cmd\paramintel
.\paramintel.exe -version
```

Expected:

```text
ParamIntel v0.5.0
```

## 2. Lab A — mid-scan 429 evidence integrity

Create a raw request:

```powershell
[System.IO.File]::WriteAllText(
    ".\burprequests\v05-mid-scan.txt",
    "GET http://127.0.0.1:8092/mid-scan HTTP/1.1`r`nHost: 127.0.0.1:8092`r`nAccept: application/json`r`n`r`n"
)
```

Remove any old output first:

```powershell
Remove-Item .\v05-mid-scan-findings.json -ErrorAction SilentlyContinue
```

Run:

```powershell
.\paramintel.exe `
    -request .\burprequests\v05-mid-scan.txt `
    -locations query `
    -baseline 3 `
    -trials 1 `
    -chunk 64 `
    -characterize=false `
    -value-aware=false `
    -verbose `
    -output .\v05-mid-scan-findings.json
```

Acceptance criteria:

- baseline completes normally;
- the first active query-probe 429 aborts the scan;
- the diagnostic contains `rate limit detected` and `response was not used as discovery evidence`;
- the process exits non-zero;
- `v05-mid-scan-findings.json` is not created.

Confirm the output file is absent:

```powershell
Test-Path .\v05-mid-scan-findings.json
```

Expected:

```text
False
```

## 3. Lab B — asymmetric candidate/control limiter

Create the request:

```powershell
[System.IO.File]::WriteAllText(
    ".\burprequests\v05-asymmetric.txt",
    "POST http://127.0.0.1:8092/asymmetric HTTP/1.1`r`nHost: 127.0.0.1:8092`r`nContent-Type: application/json`r`nAccept: application/json`r`n`r`n{`"options`":{`"page_size`":10},`"items`":[]}"
)
```

Create the related response that contributes one contextual nested candidate:

```powershell
[System.IO.File]::WriteAllText(
    ".\burprequests\v05-asymmetric-response.txt",
    "{`"options`":{`"page_size`":10,`"include_deleted`":false},`"items`":[]}"
)
```

Remove any old output:

```powershell
Remove-Item .\v05-asymmetric-findings.json -ErrorAction SilentlyContinue
```

Run:

```powershell
.\paramintel.exe `
    -request .\burprequests\v05-asymmetric.txt `
    -context-response .\burprequests\v05-asymmetric-response.txt `
    -locations json `
    -baseline 3 `
    -trials 1 `
    -chunk 64 `
    -characterize=false `
    -value-aware=false `
    -allow-state-changing `
    -verbose `
    -output .\v05-asymmetric-findings.json
```

Acceptance criteria:

- context intelligence reports exactly one actionable response-only candidate: `$.options.include_deleted`;
- the candidate produces a normal behavioral change;
- the generated same-placement `zz_pi_...` control receives 429;
- the verification experiment aborts rather than producing a confidence score/finding;
- the diagnostic contains `rate limit detected` and `response was not used as discovery evidence`;
- `v05-asymmetric-findings.json` is not created.

Confirm:

```powershell
Test-Path .\v05-asymmetric-findings.json
```

Expected:

```text
False
```

## 4. Lab C — global request-start pacing

Create the request:

```powershell
[System.IO.File]::WriteAllText(
    ".\burprequests\v05-pace.txt",
    "GET http://127.0.0.1:8092/pace HTTP/1.1`r`nHost: 127.0.0.1:8092`r`nAccept: application/json`r`n`r`n"
)
```

Run:

```powershell
.\paramintel.exe `
    -request .\burprequests\v05-pace.txt `
    -locations query `
    -baseline 3 `
    -trials 1 `
    -chunk 64 `
    -characterize=false `
    -value-aware=false `
    -delay 100ms `
    -verbose `
    -output .\v05-pace-findings.json
```

The ParamIntel window should report:

```text
[*] Request pacing: minimum 100ms between request starts
```

The lab-server window prints request-start gaps similar to:

```text
[pace] request 1 start
[pace] request 2 start gap=100ms
[pace] request 3 start gap=100ms
[pace] request 4 start gap=100ms
```

Small scheduler/local-network timing variation is expected. The external acceptance criterion is that consecutive observed starts remain approximately at or above the configured 100ms interval rather than being sent back-to-back.

This endpoint stays baseline-like, so a normal completed report with zero parameters is expected.

## Release acceptance status

Do not mark v0.5 external acceptance complete from automated tests alone. Record the actual Windows CLI output from all three labs in `VERIFICATION.txt` before the release PR is considered ready to merge.
