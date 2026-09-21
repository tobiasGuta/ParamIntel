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
- selected action probability below the local threshold -> STOP;
- unknown action -> reject the provider response;
- malformed confidence/probabilities -> reject the provider response.

Default decision threshold:

```text
0.80
```

Jev's overall choice confidence is retained for audit, but the local routing gate uses the selected action's probability. Neither signal contributes to ParamIntel vulnerability confidence.

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
  -min-choice-probability 0.80
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

If the model suggests a non-STOP action whose selected-label probability is below the local threshold, `suggested_action` remains visible for audit but `applied_action` becomes `stop`. Provider-selected `STOP` is never relabeled as a gate intervention.

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
- percentage of calls gated for low selected-action probability.

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


## First live observations

Two sanitized live runs against `jev-1.13.0` established useful initial behavior.

### Already-verified finding

State:

```text
visibility
candidate 3/3
control 0/3
ParamIntel confidence 1.00
remaining budget 18
```

Jev selected `stop`, with approximately:

```text
stop probability: 0.50
related_value_profile: 0.20
enum_profile: 0.18
choice confidence: 0.42
```

This is directionally sensible: ParamIntel already had strong evidence, so additional characterization may not justify spending more requests.

### Decision-needed enum case

State:

```text
status
candidate 0/3
control 0/3
observed structural clue: $.allowed_statuses
remaining budget 18
```

Jev selected:

```text
enum_profile
selected probability: 0.80
choice confidence: 0.77
stop probability: 0.11
```

This exposed an important spike-design correction: TypeSafe reports the winning label's probability separately from an overall confidence metric. The local gate now uses the selected label's probability as the action threshold, while retaining overall confidence for audit.

These two cases are encouraging but are not sufficient for production graduation.


## Stability sampling

Single Jev calls are not enough to choose a production threshold. The first enum fixture returned the same `enum_profile` decision twice, but the selected probability moved from about `0.80` to `0.77`.

Use the stability evaluator before changing the gate:

```powershell
go run .\cmd\decision-stability `
  -state .\labs\typesafe-decision-provider\state-enum.json `
  -runs 10 `
  -min-choice-probability 0.80
```

The evaluator records:

- suggested action for every run;
- applied action after the local gate;
- selected action probability;
- overall Jev choice confidence;
- gate rate;
- STOP rate;
- modal action and modal rate;
- min/mean/max selected probability;
- min/mean/max latency.

The goal is to distinguish **ranking stability** from **probability calibration**. If Jev repeatedly chooses the same correct action while the numeric probability fluctuates around a threshold, ParamIntel should not simply tune the threshold to the last observed run. The eventual gate should be selected from the benchmark distribution, not from one sample.


## Multi-case calibration fixtures

The spike now includes a small calibration set beyond the original enum case:

```text
state-enum.json       -> expert expectation: enum_profile
state-boolean.json    -> expert expectation: boolean_profile
state-integer.json    -> expert expectation: integer_boundary_profile
state-sufficient.json -> expert expectation: stop
state-ambiguous.json  -> expert expectation: stop or a strongly gated non-STOP result
```

Run each state repeatedly before changing the gate.

Example:

```powershell
go run .\cmd\decision-stability `
  -state .\labs\typesafe-decision-provider\state-boolean.json `
  -runs 10 `
  -min-choice-probability 0.80
```

The evaluator now also reports the runner-up action and the **decision margin**:

```text
decision margin = selected action probability - runner-up probability
```

This separates two very different situations:

```text
selected 0.77, runner-up 0.14 -> margin 0.63 -> strongly dominant choice

selected 0.77, runner-up 0.72 -> margin 0.05 -> genuinely ambiguous choice
```

An eventual production gate may use a calibrated combination of selected probability and decision margin, but no margin threshold should be chosen until the multi-case data is collected.
