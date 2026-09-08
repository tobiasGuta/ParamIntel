# v0.6 AI Candidate Advisor acceptance lab

This localhost-only lab tests the first AI-advisor slice without using a real target.

The endpoint exposes a response-side field named `include_archived`, but ParamIntel's built-in candidate list does not contain that exact query parameter. The server changes behavior whenever the query parameter is present, regardless of its value. That keeps this lab focused on the v0.6 first-slice question:

> Can Gemini propose a useful candidate name that normal ParamIntel candidate acquisition misses, while ParamIntel's existing verifier remains responsible for the evidence?

The lab also verifies the normal v0.6 context flow: ParamIntel reuses one response already collected during baseline, sanitizes its structure locally, and sends only that sanitized structure to the advisor. No separate AI response fixture is required for the default path.

The lab does **not** test AI-generated semantic values yet.

## 1. Start the lab

From the repository root:

```powershell
go run .\labs\v0.6-ai-advisor
```

Expected:

```text
ParamIntel v0.6 AI advisor lab listening on http://127.0.0.1:41780/projects
```

Leave that terminal open.

## 2. Build ParamIntel from the v0.6 branch

In another PowerShell terminal:

```powershell
go build -trimpath -o paramintel.exe .\cmd\paramintel
.\paramintel.exe -version
```

Expected version:

```text
ParamIntel v0.6.0-dev
```

## 3. Control run without AI

```powershell
.\paramintel.exe `
  -request .\labs\v0.6-ai-advisor\request.txt `
  -scheme http `
  -baseline 3 `
  -trials 3 `
  -chunk 8 `
  -characterize=false `
  -value-aware=false `
  -verbose `
  -output .\labs\v0.6-ai-advisor\control-findings.json
```

Expected result:

```text
0 parameters
```

The control establishes that the existing built-in candidate list does not discover `include_archived` on its own.

## 4. Set the Gemini API key without placing it directly in the command

PowerShell 7:

```powershell
$env:GEMINI_API_KEY = Read-Host "Gemini API key" -MaskInput
```

Do not commit the key or put it in a request/response fixture.

## 5. AI-enabled run using automatic baseline context

```powershell
.\paramintel.exe `
  -request .\labs\v0.6-ai-advisor\request.txt `
  -scheme http `
  -ai-advisor `
  -ai-provider gemini `
  -ai-candidate-budget 8 `
  -baseline 3 `
  -trials 3 `
  -chunk 8 `
  -characterize=false `
  -value-aware=false `
  -verbose `
  -output .\labs\v0.6-ai-advisor\ai-findings.json
```

The verbose output should include:

```text
context source: baseline_response
```

The JSON report should likewise contain:

```json
"context_source": "baseline_response"
```

A successful acceptance should show the advisor executing and ParamIntel independently confirming `include_archived`.

The finding should retain provenance similar to:

```json
{
  "name": "include_archived",
  "location": "query",
  "candidate_sources": [
    {
      "source": "ai_semantic_hypothesis",
      "priority": 90,
      "reason": "..."
    }
  ],
  "candidate_changed": 3,
  "candidate_trials": 3,
  "random_control_changed": 0,
  "random_control_trials": 3
}
```

The exact AI priority/reason wording is not the acceptance criterion. The important evidence is:

```text
baseline response already collected
        ↓
local sanitizer keeps structure only
        ↓
AI proposes candidate
        ↓
local output gate accepts candidate
        ↓
ParamIntel candidate trials change
        ↓
paired random-name controls do not change
```

### Optional explicit AI context override

When you intentionally want Gemini to reason from a different related response, use:

```text
-ai-context-response response.txt
```

That file replaces the automatic baseline response only for AI structural context. It still passes through the same local sanitizer. The report records `context_source: ai_context_response`.

## 6. Clear the key when finished

```powershell
Remove-Item Env:GEMINI_API_KEY
```

## Acceptance verdict

Record **PASS** only if all of these are true:

- control run reports zero parameters;
- AI advisor reports `context source: baseline_response` without requiring `-ai-context-response`;
- AI advisor reports that it accepted at least one candidate hypothesis;
- `include_archived` is independently confirmed by ParamIntel;
- candidate trials are reproducibly changed;
- paired random-name controls remain unchanged;
- the finding provenance says `ai_semantic_hypothesis`;
- no API key or raw sensitive value appears in the findings file.

If Gemini suggests plausible candidates but `include_archived` is not confirmed, record the run as **inconclusive**, not as an AI success. The model's suggestion by itself is never evidence.
