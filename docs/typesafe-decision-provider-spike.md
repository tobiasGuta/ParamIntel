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


## Jev versus deterministic baseline

The spike now includes a deliberately competent deterministic router in `internal/decision/heuristic.go`.

It handles:

- exhausted request budgets;
- already-verified high-confidence findings;
- noisy paired controls;
- explicit boolean/integer value kinds;
- common enum-like candidate names plus enum-like response structure;
- common boolean/numeric naming patterns.

This is not intended as a straw-man baseline. If it matches Jev on the harder cases, Jev does not earn a production dependency.

Run the head-to-head benchmark:

```powershell
go run .\cmd\decision-benchmark `
  -manifest .\labs\typesafe-decision-provider\benchmark.json `
  -runs 5
```

The benchmark contains the original five calibration states plus eight harder states where the semantic clue is more indirect.

It reports:

- heuristic action and correctness per case;
- Jev modal action and modal rate;
- Jev expert-label agreement rate;
- mean selected probability;
- mean selected-vs-runner-up margin;
- mean Jev latency;
- aggregate heuristic accuracy;
- aggregate Jev expert-label agreement.

The production decision should be simple:

```text
if deterministic baseline ~= Jev:
    do not add Jev to ParamIntel

if Jev materially beats deterministic baseline
and remains stable/cost-effective:
    continue toward Feedback-Guided Characterization Planner
```

The benchmark should be expanded before any production merge, but it is now sufficient to test whether Jev is providing reasoning value beyond obvious local rules.


## Gate calibration conclusion

The 13-case benchmark showed that the original absolute selected-probability gate was counterproductive for this bounded decision job.

Observed benchmark behavior:

```text
deterministic coverage:              38.46%
deterministic accuracy when decided: 100%

Jev raw expected-action rate:        ~86%
hybrid raw expected-action rate:     ~86%

0.80 probability-gated Jev:          materially worse
0.80 probability-gated hybrid:       materially worse
```

A shadow decision-margin sweep also failed to improve the raw policy. Every positive tested margin threshold reduced expected-action accuracy versus accepting any valid catalog choice.

This does **not** mean Jev probabilities are useless. They remain valuable audit/calibration data. It means the current benchmark does not support using them as an admission gate for a fixed catalog of low-risk characterization experiments.

The spike therefore now defaults to:

```text
valid catalog choice -> apply
```

Numeric probability gating is opt-in for experiments only:

```powershell
-min-choice-probability 0.80
```

A value of `0` disables numeric gating.

Safety still comes from local invariants:

- fixed action catalog;
- request-budget enforcement;
- unknown-action rejection;
- malformed-response rejection;
- deterministic HTTP execution;
- paired controls and repeated verification;
- Jev never modifies finding confidence;
- provider failures fail closed in the hybrid planner.

This conclusion is still benchmark-local. The benchmark must expand before production graduation.


## Experimental hybrid planner

The spike now includes `internal/decision.HybridPlanner`.

Policy:

```text
deterministic Decide(state)
    |
    +-- decided --> apply deterministic action; no provider call
    |
    +-- abstain --> call Jev
                       |
                       +-- valid catalog action --> apply directly
                       |
                       +-- provider unavailable/error/malformed response --> STOP
```

The hybrid planner does not use Jev probability or confidence as a production-style admission gate by default.

Probability/confidence remain audit data. The existing optional `MinChoiceProbability` path is retained only for calibration experiments; `0` disables it.

This keeps the important separation:

```text
decision model -> chooses a bounded experiment
deterministic core -> executes, controls, verifies, and scores evidence
```

A wrong Jev routing decision can spend bounded characterization budget, but it cannot create a finding or raise finding confidence by itself.


## Frozen holdout benchmark

After generic structural evidence rules raised deterministic coverage on the development benchmark, that benchmark is no longer suitable for proving incremental Jev value. The deterministic planner must remain frozen while the holdout is evaluated.

The holdout manifest is:

```text
labs/typesafe-decision-provider/benchmark-holdout.json
```

Run:

```powershell
go run .\cmd\decision-benchmark `
  -manifest .\labs\typesafe-decision-provider\benchmark-holdout.json `
  -runs 5
```

The benchmark now reports `stop_fallback_expected_rate`, which represents:

```text
deterministic decision when available
otherwise STOP
```

Compare that directly with `hybrid_expected_rate`:

```text
STOP fallback ~= hybrid:
    Jev has not demonstrated enough incremental value

hybrid materially > STOP fallback:
    Jev is adding residual semantic routing value
```

Do not add new deterministic rules based on holdout failures until the holdout result has been recorded.


## Residual holdout v2

The first frozen holdout demonstrated incremental Jev value over STOP fallback, but its failures were then used to clarify the action catalog. It is therefore development evidence from that point forward, not final proof.

A second untouched residual holdout is frozen at:

```text
labs/typesafe-decision-provider/benchmark-holdout-v2.json
```

It contains 18 cases:

- 2 enum;
- 2 related-value;
- 2 nullability;
- 2 empty-value;
- 2 case-variation;
- 2 boolean;
- 2 integer-boundary;
- 4 STOP controls.

The deterministic heuristic is intentionally unchanged and should abstain on these cases. The benchmark must compare:

```text
STOP-on-abstain baseline
vs
Jev-on-abstain hybrid
```

The runner now also reports case-level modal accuracy in addition to per-run expected-action rate.

Run:

```powershell
go run .\cmd\decision-benchmark `
  -manifest .\labs\typesafe-decision-provider\benchmark-holdout-v2.json `
  -runs 5
```

Do not alter heuristic rules or catalog semantics before recording this result.
