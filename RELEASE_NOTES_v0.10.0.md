# ParamIntel v0.10.0 — AI Semantic Value Advisor

ParamIntel v0.10.0 adds a second bounded AI reasoning layer for a problem the deterministic value-aware engine cannot always solve:

> The parameter name may be correct, but what application-specific value is worth testing?

The release keeps the same evidence rule ParamIntel has used since the original AI Candidate Advisor:

> AI may propose a hypothesis. Only live application behavior, paired controls, repeated verification, and the existing confidence model may produce a finding.

## What changed

v0.10.0 adds the optional Semantic Value Advisor.

The execution order is:

```text
candidate name
    ↓
generic ParamIntel verification
    ↓ clean miss
deterministic semantic profile
    ↓ no verified behavior
AI Semantic Value Advisor
    ↓
local admission gate
    ↓
candidate screen
    ↓
same-value random-name control
    ↓
repeated candidate/control verification
    ↓
normal ParamIntel confidence
```

The model never supplies confidence and never turns its own suggestion into evidence.

## CLI

Enable the feature with:

```powershell
.\paramintel.exe `
  -request .\burprequests\request.txt `
  -ai-value-advisor `
  -ai-provider gemini `
  -ai-value-budget 4 `
  -ai-value-candidate-budget 8 `
  -value-aware-budget 96 `
  -baseline 3 `
  -trials 3 `
  -verbose
```

The feature is disabled by default.

### New controls

```text
-ai-value-advisor
-ai-value-budget
-ai-value-candidate-budget
```

`-ai-value-budget` limits locally admitted model-suggested values per candidate.

`-ai-value-candidate-budget` limits how many candidates may be submitted to the provider.

`-value-aware-budget` remains the hard target-request budget for semantic rescue. AI cannot bypass it.

## Provider input boundary

The Semantic Value Advisor reuses ParamIntel's sanitized AI context and adds only the information necessary to reason about values:

- candidate name;
- candidate location;
- JSON parent path when relevant;
- deterministic values already covered locally;
- bounded enum-like semantic hints derived from structurally relevant response fields.

Example of a useful bounded hint source:

```json
{
  "available_visibilities": [
    "public",
    "private",
    "internal"
  ]
}
```

The provider may see the safe enum-like tokens, but it does not receive arbitrary primitive response values or raw response text.

The existing privacy boundary remains in place for:

- Authorization headers;
- cookies;
- hostnames;
- captured query/form values;
- primitive JSON request values;
- raw response bodies;
- credentials and tokens.

## Local admission boundary

Provider instructions are not treated as a security boundary.

Every suggested value is checked locally before ParamIntel can send it to the target.

For query and form candidates, accepted values are bounded ordinary semantic strings.

For JSON candidates, accepted types are limited to:

```text
string
boolean
integer
null
```

Local code rejects:

- unsupported value kinds;
- malformed booleans or integers;
- duplicate suggestions;
- deterministic values ParamIntel already covered;
- overlong values;
- suggestions beyond the configured value budget;
- payload-like strings outside the semantic-token allowlist.

The Semantic Value Advisor is for ordinary application vocabulary such as lifecycle states, visibility levels, modes, roles, statuses, and other enum-like values. It is not a payload-generation feature.

## Candidate scheduling

The first end-to-end acceptance run exposed an important issue: with one AI candidate-query available, the first implementation spent that query on `account_id` simply because it appeared before the actual high-signal `visibility` candidate.

v0.10.0 fixes this with local structural relevance ranking.

Given response structure such as:

```text
available_visibilities
projects
```

ParamIntel can prioritize:

```text
visibility
```

over unrelated generic candidates before spending scarce provider calls.

This ranking is local and deterministic. It does not add confidence to a finding.

## A/B acceptance

The release includes the reproducible localhost lab:

```text
labs/semantic-value-advisor
```

The lab exposes a parameter named `visibility` whose behavior changes only for:

```text
visibility=internal
```

ParamIntel has no built-in deterministic semantic profile for `visibility`.

### Control: AI disabled

The deterministic control run produced:

```text
parameters: []
```

No parameter was verified.

### Semantic Value Advisor enabled

With one AI candidate-query allowed, ParamIntel selected:

```text
visibility (query)
local relevance: 80
semantic hints: 3
```

Gemini proposed three bounded values. ParamIntel tested them using the normal verifier and confirmed `internal`:

```text
discovery: AI semantic value using "internal" (string)
candidate: changed 3/3
control:   changed 0/3
confidence: 100% HIGH
```

The final report recorded:

```json
{
  "name": "visibility",
  "location": "query",
  "discovery_mode": "ai_value_aware",
  "discovery_value": "internal",
  "discovery_value_kind": "string",
  "confidence": 1.00,
  "candidate_changed": 3,
  "candidate_trials": 3,
  "random_control_changed": 0,
  "random_control_trials": 3
}
```

The deterministic evidence included new JSON paths under `$.internal_projects` and a new `$.mode` value.

This demonstrates the intended distinction:

```text
Gemini:
"internal is worth testing."

ParamIntel:
"I independently proved that visibility=internal causes reproducible
candidate-specific behavior that the same-value random-name control does not."
```

## Reporting

When enabled, reports may include:

```json
{
  "ai_value_advisor": {
    "provider": "gemini",
    "model": "gemini-3.5-flash-lite",
    "input_policy": "sanitized structure, candidate metadata, and bounded enum-like semantic hints",
    "context_source": "baseline_response",
    "candidate_queries": 1,
    "suggested_values": 3,
    "accepted_values": 3,
    "verified_parameters": 1
  }
}
```

A finding verified through this path records:

```text
discovery_mode: ai_value_aware
```

AI rationale or priority does not contribute to confidence.

## Deterministic-first behavior

The new advisor does not replace the existing v0.4 semantic profiles.

For a parameter such as:

```text
debug=true
```

if ParamIntel's deterministic value-aware path already verifies the behavior, the Semantic Value Advisor is not called.

This keeps model use focused on the semantic gap instead of paying an LLM to repeat logic ParamIntel already has.

## Compatibility and evidence model

v0.10.0 preserves:

- candidate/control symmetry;
- repeated verification;
- confidence scoring;
- stable JSON evidence;
- Go 1.26 / Go 1.27 release support;
- OpenAPI candidate and typed-probe authority boundaries;
- controlled JSON scaffolding;
- rate-limit/backoff evidence integrity;
- request pacing;
- state-changing-method authorization.

The AI feature expands hypothesis generation, not evidence authority.
