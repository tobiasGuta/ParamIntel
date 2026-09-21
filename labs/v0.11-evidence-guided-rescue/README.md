# v0.11 Evidence-Guided Rescue acceptance lab

This localhost-only lab validates ParamIntel v0.11 against known ground truth through the real CLI.

It covers four separate questions:

1. Can application-derived context move a useful candidate ahead of weaker wordlist noise?
2. When evidence is equal, can the scheduler use deterministic screening cost to spend a tight budget more effectively?
3. Does the zero-signal case report request cost honestly and stop once the remaining budget cannot verify another finding?
4. Does Gemini remain a bounded semantic-value proposer while ParamIntel performs the actual verification?

The lab listens only on `127.0.0.1:41782`.

## Fast path — automated deterministic benchmark

From the repository root:

```powershell
pwsh .\labs\v0.11-evidence-guided-rescue\run-benchmark.ps1
```

The runner:

- builds the v0.11 lab and ParamIntel into `.paramintel\v0.11-lab`;
- starts the localhost lab automatically;
- runs scenarios A, B, C, and the no-AI control for D;
- validates the JSON report against `expected.json`;
- stops the lab process when finished;
- leaves the generated reports under `.paramintel\v0.11-lab`.

Expected final summary:

```text
=== v0.11 dedicated lab result ===
Deterministic scenarios: PASS
Gemini scenario: skipped (use -WithGemini when desired)
```

To include the Gemini semantic-value scenario after `GEMINI_API_KEY` is already set:

```powershell
pwsh .\labs\v0.11-evidence-guided-rescue\run-benchmark.ps1 -WithGemini
```

If Gemini does not produce the application-specific value, that part is reported as inconclusive rather than being treated as proof.

---

## 1. Start the lab

From the repository root:

```powershell
go run .\labs\v0.11-evidence-guided-rescue
```

Expected:

```text
ParamIntel v0.11 evidence-guided rescue lab listening on http://127.0.0.1:41782
  context ranking:  http://127.0.0.1:41782/items
  cost tie-breaker: http://127.0.0.1:41782/search
  zero-signal:      http://127.0.0.1:41782/no-signal
  Gemini value:     http://127.0.0.1:41782/projects
```

Leave this terminal open.

## 2. Build ParamIntel from the v0.11 branch

In a second PowerShell terminal:

```powershell
git switch feat/v0.11-evidence-guided-rescue
git pull
go build -trimpath -o paramintel.exe .\cmd\paramintel
```

The commands below write one JSON report per scenario so the `value_aware` metrics can be compared directly.

---

## Scenario A — application context beats weaker candidates

Ground truth:

```text
parameter: format
value:     json
budget:    8
```

The wordlist intentionally begins with weaker candidates. The baseline response contains:

```json
"supported_formats": ["json", "csv"]
```

Run:

```powershell
.\paramintel.exe `
  -request .\labs\v0.11-evidence-guided-rescue\request-items.txt `
  -wordlist .\labs\v0.11-evidence-guided-rescue\wordlist-items.txt `
  -scheme http `
  -baseline 3 `
  -trials 3 `
  -chunk 64 `
  -characterize=false `
  -value-aware `
  -value-aware-budget 8 `
  -verbose `
  -output .\labs\v0.11-evidence-guided-rescue\items-findings.json
```

Expected acceptance:

- `format` is attempted before `debug` / `admin`;
- verbose ordering shows contextual relevance for `format`;
- `format=json` is verified;
- discovery mode is `value_aware`;
- candidate changes 3/3;
- random-name control changes 0/3;
- `value_aware.requests_used` is 8;
- `value_aware.verified_requests` is 8;
- remaining eligible candidates are deferred after the budget is consumed.

---

## Scenario B — cheaper equal-evidence candidate wins

Ground truth:

```text
parameter: sort
value:     asc
budget:    8
```

Both `debug` and `sort` are local semantic-profile candidates.

Current deterministic screening cost:

```text
debug -> 4 values
sort  -> 2 values
```

Run:

```powershell
.\paramintel.exe `
  -request .\labs\v0.11-evidence-guided-rescue\request-search.txt `
  -wordlist .\labs\v0.11-evidence-guided-rescue\wordlist-search.txt `
  -scheme http `
  -baseline 3 `
  -trials 3 `
  -chunk 64 `
  -characterize=false `
  -value-aware `
  -value-aware-budget 8 `
  -verbose `
  -output .\labs\v0.11-evidence-guided-rescue\search-findings.json
```

Expected acceptance:

- `sort` runs before `debug` despite appearing later in the wordlist;
- `sort=asc` verifies;
- request cost is 8;
- no AI call is required.

---

## Scenario C — 17-candidate zero-signal control

Ground truth:

```text
verified parameters: 0
budget:              64
```

Run:

```powershell
.\paramintel.exe `
  -request .\labs\v0.11-evidence-guided-rescue\request-no-signal.txt `
  -wordlist .\labs\v0.11-evidence-guided-rescue\wordlist-no-signal.txt `
  -scheme http `
  -baseline 3 `
  -trials 3 `
  -chunk 64 `
  -characterize=false `
  -value-aware `
  -value-aware-budget 64 `
  -verbose `
  -output .\labs\v0.11-evidence-guided-rescue\no-signal-findings.json
```

Expected acceptance for the current v0.11 branch:

- zero verified parameters;
- 17 rescue-eligible semantic candidates;
- 57 semantic-rescue requests used;
- 57 miss-cost requests;
- the final audit outcome may be `verification_budget_insufficient`;
- 7 requests remain unused because they cannot complete screen + control + three paired verification trials for another new finding.

For comparison, the frozen legacy/static evaluation recorded 60 requests on this same semantic-profile mix.

This is intentionally a modest saving. The scheduler does not truncate semantic profiles merely to make the request count look better.

---

## Scenario D — Gemini application-specific value

Ground truth:

```text
parameter: visibility
value:     internal
```

`visibility` intentionally has no built-in semantic profile.

### Control: no AI Value Advisor

```powershell
.\paramintel.exe `
  -request .\labs\v0.11-evidence-guided-rescue\request-projects.txt `
  -wordlist .\labs\v0.11-evidence-guided-rescue\wordlist-projects.txt `
  -scheme http `
  -baseline 3 `
  -trials 3 `
  -characterize=false `
  -value-aware `
  -value-aware-budget 32 `
  -verbose `
  -output .\labs\v0.11-evidence-guided-rescue\projects-control.json
```

Expected: no verified `visibility` finding.

### Gemini-enabled run

Set the key in the shell without placing it in the command:

```powershell
$env:GEMINI_API_KEY = Read-Host "Gemini API key" -MaskInput
```

Then:

```powershell
.\paramintel.exe `
  -request .\labs\v0.11-evidence-guided-rescue\request-projects.txt `
  -wordlist .\labs\v0.11-evidence-guided-rescue\wordlist-projects.txt `
  -scheme http `
  -baseline 3 `
  -trials 3 `
  -characterize=false `
  -value-aware `
  -value-aware-budget 32 `
  -ai-value-advisor `
  -ai-provider gemini `
  -ai-value-budget 4 `
  -ai-value-candidate-budget 1 `
  -verbose `
  -output .\labs\v0.11-evidence-guided-rescue\projects-ai.json
```

Expected successful path:

```text
baseline/context structure
        ↓
visibility gets contextual relevance
        ↓
Gemini proposes "internal"
        ↓
ParamIntel tests it
        ↓
paired random-name control stays unchanged
        ↓
3/3 repeated verification succeeds
```

The finding should record:

```json
"discovery_mode": "ai_value_aware"
```

The Gemini suggestion itself is never evidence.

Clear the key afterward if desired:

```powershell
Remove-Item Env:GEMINI_API_KEY
```

---

## Acceptance summary

The dedicated lab passes when:

| Scenario | Expected |
| --- | --- |
| A — context ranking | `format=json` verified in 8 rescue requests |
| B — cost tie-break | `sort=asc` verified in 8 rescue requests |
| C — zero signal | 0 findings, 57 miss requests |
| D — no-AI control | no `visibility` finding |
| D — Gemini | `visibility=internal` independently verified |

Do not change verification thresholds or semantic profiles to make a scenario pass. If observed numbers differ, inspect the report and update the implementation or documented expectation only after understanding why.
