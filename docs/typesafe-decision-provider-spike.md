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


## End-to-end HybridPlanner benchmark

The synthetic benchmark reconstructs hybrid behavior for comparison. The spike also includes a direct runner for the actual `decision.HybridPlanner` implementation:

```powershell
go run .\cmd\decision-hybrid-benchmark `
  -manifest .\labs\typesafe-decision-provider\benchmark-holdout-v2.json `
  -runs 5
```

This runner reports:

- applied expected-action rate;
- modal case accuracy;
- deterministic decision count;
- provider decision count;
- fail-closed decision count;
- per-case provider/deterministic/fail-closed rates;
- mean latency.

The v2 holdout result from the comparison benchmark is frozen at:

```text
Jev/hybrid per-run expected-action rate: 87/90 = 96.67%
Jev/hybrid modal case accuracy:          17/18 = 94.44%
STOP-on-abstain baseline:                4/18 = 22.22%
numeric gate interventions:              0
```

The only modal miss was `plan -> enum_profile`, where Jev chose STOP in 3/5 runs.

This is sufficient to continue evaluating Jev as the residual semantic planner. It is not yet sufficient to wire the provider into ParamIntel's production scan path. The remaining graduation work is end-to-end HybridPlanner verification and sanitized real-flow evaluation.


## Real-flow residual shadow capture

Schema v3 captures sanitized **decision-needed residual states immediately before semantic/value-aware rescue**.

This supersedes two earlier capture populations. Schema v1 captured already-verified findings before characterization. Schema v2 moved to the residual lifecycle but briefly captured candidates before checking whether they could actually reach semantic rescue. Schema v3 captures only decision-relevant residuals that can reach the current semantic-rescue path, plus explicit noisy/control-changed residuals that deterministic logic already stops. Earlier schemas must not be mixed with schema-v3 data.

The v2 capture point matches the benchmark lifecycle:

```text
normal deterministic discovery
        |
        +-- verified finding -----------------> normal finding path; no shadow case
        |
        +-- noisy/ambiguous generic result ---> residual shadow state; deterministic STOP is measurable
        |
        +-- clean miss ------------------------> residual shadow state
                                                   |
                                                   +-- current value-aware rescue continues unchanged
```

The capture path is passive. It does not call TypeSafe, choose an action, alter candidate ordering, alter request execution, or change finding confidence.

Enable it with normal value-aware scanning:

```powershell
go run .\cmd\paramintel `
  -request .\request.txt `
  -scheme https `
  -value-aware `
  -decision-shadow-capture .\.paramintel\decision-shadow.jsonl `
  -decision-shadow-budget 8 `
  -verbose
```

`-decision-shadow-budget` is an evaluation cap. The stored remaining budget is the lower of:

- the real remaining semantic request budget at that decision point; and
- the configured shadow budget.

This prevents the shadow dataset from pretending more request authority than either the live scan or the future bounded planner should have.

Each schema-v3 JSONL record contains only:

- candidate name;
- candidate location;
- discovery mode/value kind when already known;
- candidate/control trial counts when a direct generic verification exists;
- deterministic confidence;
- evidence kinds;
- sanitized evidence paths;
- candidate-source type/path when available;
- bounded remaining request budget.

The capture deliberately excludes:

- target URL / hostname;
- raw HTTP requests or responses;
- headers;
- cookies;
- authorization values;
- request/response bodies;
- discovery probe values;
- evidence `before` / `after` values;
- candidate-source free-text reasons;
- schema references;
- AI prompts or provider responses.

The recommended path `.paramintel/decision-shadow.jsonl` is ignored by Git. Capture write failures print a warning and do not change scan behavior.

### Dataset freezing and independent labeling

Freeze and deduplicate captured states without calling Jev:

```powershell
go run .\cmd\decision-shadow-freeze `
  -input .\.paramintel\decision-shadow.jsonl `
  -output .\.paramintel\decision-shadow-dataset.json
```

The freezer:

- accepts only the current shadow schema;
- recomputes every deterministic state ID and rejects tampering;
- deduplicates identical states by ID;
- sorts cases deterministically;
- leaves every `expected_action` blank.

Fill the labels **before** any Jev replay. Labels must use exactly one action from the fixed catalog:

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

Validate labels without an API key or provider call:

```powershell
go run .\cmd\decision-shadow-replay `
  -dataset .\.paramintel\decision-shadow-dataset.json `
  -validate-only
```

The validator refuses blank labels, unknown actions, duplicate IDs, changed state IDs, or mismatched dataset counts.

Only after labels are frozen should Jev be evaluated:

```powershell
go run .\cmd\decision-shadow-replay `
  -dataset .\.paramintel\decision-shadow-dataset.json `
  -runs 5 `
  -output .\.paramintel\decision-shadow-replay.json
```

Replay uses the actual `HybridPlanner` and reports:

- per-run expected-action agreement;
- modal case accuracy;
- deterministic decision count;
- provider decision count;
- fail-closed decision count;
- mean latency;
- TypeSafe input/output usage.

### Real-flow evaluation target

Do not force a fixed positive-finding quota from one live target.

The useful population is residual decision states, including clean misses and deterministic STOP cases, not only verified findings. Collect them opportunistically from normal authorized ParamIntel use, deduplicate them, freeze the dataset, and label it before provider replay.

Do not tune the deterministic heuristic or Jev action catalog against a frozen evaluation dataset. If a frozen set is used to change routing logic, demote it to development evidence and create a fresh holdout.
