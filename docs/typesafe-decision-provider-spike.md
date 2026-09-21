# TypeSafe Jev Decision Provider Spike

This branch evaluates TypeSafe Jev as a **bounded decision provider**, not as another generative AI provider.

The experiment is intentionally isolated from ParamIntel's production scan loop.

## Hypothesis

ParamIntel already has two generative AI jobs:

- Candidate Advisor: propose candidate names and placements.
- Semantic Value Advisor: propose bounded application-specific values.

Jev is being evaluated for a different question:

> Given deterministic ParamIntel state and a fixed catalog of permitted characterization experiments, which experiment should run next, or should ParamIntel stop?

Jev returns a typed choice plus confidence/probabilities. ParamIntel remains responsible for HTTP execution, controls, evidence, and findings.

## Fixed experiment catalog

The first spike allows exactly:

```text
stop
enum_profile
boolean_profile
nullability_profile
integer_boundary_profile
empty_value_profile
case_variation_profile
related_value_profile
```

Unknown actions are rejected locally.

## Deterministic gates

The planner adds local gates around the provider:

- zero remaining request budget -> STOP without calling Jev;
- provider confidence below the local threshold -> STOP;
- unknown action -> reject the provider response;
- malformed confidence/probabilities -> reject the provider response.

Default decision threshold:

```text
0.80
```

Jev confidence is only used to gate the routing decision. It never contributes to ParamIntel vulnerability confidence.

## Input boundary

The spike sends only compact structured state:

- candidate name and location;
- discovery mode and value kind;
- candidate/control trial counts;
- existing ParamIntel confidence;
- evidence kinds and paths;
- remaining request budget.

It does not send raw requests, raw responses, authorization headers, cookies, hostnames, credentials, tokens, or arbitrary response bodies.

## API contract

The adapter talks directly to TypeSafe's documented System One API:

```text
POST https://api.typesafe.ai/v1/systemone
Authorization: Bearer TYPESAFE_API_KEY
model: jev-latest
```

It sends one typed `choice` question named `next_experiment`.

The spike uses the HTTP contract directly instead of adding an unofficial Go SDK dependency.

## Standalone runner

This command does not touch a target. It only submits a sanitized decision-state fixture to TypeSafe.

Set the key for the current PowerShell session:

```powershell
$env:TYPESAFE_API_KEY = "YOUR_TYPESAFE_API_KEY"
```

Do not commit the key or paste it into logs.

Run:

```powershell
go run .\cmd\decision-spike `
  -state .\labs\typesafe-decision-provider\state.json `
  -min-confidence 0.80
```

The output preserves both what Jev suggested and what the local gate actually applies.

Example shape:

```json
{
  "suggested_action": "related_value_profile",
  "applied_action": "related_value_profile",
  "confidence": 0.91,
  "provider": "typesafe",
  "model": "jev-latest",
  "gated": false
}
```

If the model suggests a non-STOP action below the local threshold, `suggested_action` remains visible for audit but `applied_action` becomes `stop`.

## Acceptance criteria

This spike should **not** graduate merely because the API works.

Before production integration, compare Jev against a simple deterministic baseline on 20-30 sanitized ParamIntel decision states covering:

1. already-sufficient evidence;
2. boolean-like candidates;
3. enum-like candidates;
4. numeric candidates;
5. ambiguous evidence;
6. low remaining request budget.

Measure:

- exact action agreement with an expert-approved action;
- STOP precision;
- invalid/unsafe action rate;
- latency;
- token/cost usage;
- percentage of calls gated for low confidence.

Keep the integration only if Jev materially improves routing quality at acceptable cost and latency.

## Non-goals

This spike does not:

- replace Gemini;
- add TypeSafe to `-ai-provider`;
- let Jev generate parameter names;
- let Jev generate semantic values;
- let Jev generate payloads;
- let Jev send target HTTP requests;
- change ParamIntel finding confidence;
- automatically execute the selected experiment;
- change v0.10.0 production behavior.

## Architecture

```text
Gemini / generative Provider
    -> proposes new hypotheses

Jev / DecisionProvider
    -> selects among locally permitted actions

ParamIntel deterministic core
    -> executes and proves behavior
```

The interfaces stay separate because hypothesis generation and bounded decision selection are different jobs.
