# ParamIntel Semantic Value Advisor

## Goal

The Semantic Value Advisor is an optional second-stage rescue layer for parameters whose names are already known but whose behavior depends on an application-specific value.

It does not discover findings, assign confidence, or judge whether a response is vulnerable.

The governing rule is:

> AI may suggest which ordinary semantic values are worth testing. Only ParamIntel's deterministic candidate/control verifier may decide whether the value produces candidate-specific behavior.

## Execution order

The advisor is intentionally not in the hot enumeration loop.

```text
candidate name
    |
    v
generic ParamIntel verification
    |
    | clean miss
    v
deterministic semantic profile
    |
    | no verified behavior
    v
AI Semantic Value Advisor
    |
    v
local value validation
    |
    v
one-shot candidate screen
    |
    v
same-value random-name control
    |
    v
repeated candidate/control verification
    |
    v
normal ParamIntel confidence
```

If a deterministic semantic value already verifies the candidate, the AI advisor is never called.

## CLI

The advisor is disabled by default.

```powershell
.\paramintel.exe `
  -request .\request.txt `
  -ai-value-advisor `
  -ai-provider gemini `
  -ai-value-budget 4 `
  -ai-value-candidate-budget 8 `
  -value-aware-budget 96 `
  -verbose
```

The provider and API-key settings are shared with the existing AI Candidate Advisor. The Candidate Advisor does not need to be enabled in order to use the Semantic Value Advisor.

### Budgets

- `-ai-value-budget` controls the maximum number of locally admitted AI value hypotheses per candidate.
- `-ai-value-candidate-budget` controls how many clean-miss candidates may be submitted to the provider.
- `-value-aware-budget` remains the hard target-request budget for semantic rescue. AI cannot bypass it.

The Semantic Value Advisor requires value-aware discovery to remain enabled and requires a positive value-aware request budget.

## Input boundary

The provider receives the same sanitized application structure used by the Candidate Advisor plus:

- candidate name;
- candidate location;
- JSON parent path when relevant;
- deterministic semantic values already covered locally, so the provider can avoid repeating them.

The provider does not receive:

- Authorization headers;
- cookies;
- hostnames;
- query or form values from the captured request;
- primitive JSON values from the captured request;
- raw response text;
- credentials or tokens.

## Allowed output

For query and form candidates, all admitted values are normalized to strings.

For JSON candidates, the first slice accepts only:

- string;
- boolean;
- integer;
- null.

The local admission gate rejects:

- unsupported value kinds;
- invalid typed values;
- values longer than 80 runes;
- duplicates;
- values already covered by deterministic profiles;
- suggestions beyond the configured value budget.

The provider instruction explicitly excludes exploit payload generation. The feature is for ordinary application-domain semantics such as:

- lifecycle states;
- visibility modes;
- feature modes;
- enum-like values;
- booleans;
- bounded integers.

## Evidence boundary

A value suggested by AI is only a hypothesis.

For each admitted value ParamIntel still requires:

1. a meaningful candidate response;
2. a same-value random-name control that does not reproduce the behavior;
3. complete repeated candidate/control verification;
4. the normal minimum-confidence threshold.

Candidate and control always receive the exact same value and value kind. Only the parameter name changes.

A verified AI-value finding records:

```json
{
  "discovery_mode": "ai_value_aware",
  "discovery_value": "internal",
  "discovery_value_kind": "string"
}
```

AI priority or rationale never contributes to confidence.

## Reporting

The scan report includes an `ai_value_advisor` summary containing:

- provider;
- model;
- input policy;
- context source;
- number of candidate queries;
- suggested values;
- locally accepted values;
- number of parameters ultimately verified through AI semantic values.

These counts describe hypothesis generation and deterministic outcomes; they do not convert model suggestions into evidence.

## First-slice acceptance case

The core acceptance test uses a parameter named `visibility` that has no built-in ParamIntel semantic profile.

A generic random value produces no behavior. The AI value source proposes `internal`. ParamIntel then proves the behavior independently with repeated candidate/control testing and records `ai_value_aware` provenance.

A separate regression proves that when the built-in deterministic `debug=true` profile succeeds, the AI Value Advisor is never called.
