# ParamIntel v0.9.0 — Local OpenAPI Candidate Intelligence

ParamIntel v0.9.0 adds **local OpenAPI candidate intelligence** while preserving ParamIntel's core evidence model: schema metadata may shape hypotheses, but only live application behavior, repeated verification, paired random-name controls, and existing confidence/evidence rules may produce a finding.

## Highlights

- new `-openapi` flag accepts a local OpenAPI 3.x document;
- deterministic operation matching prefers exact concrete paths and rejects ambiguous templates;
- request schema selection follows the captured request Content-Type;
- response schema selection follows the stable baseline response status and Content-Type;
- response-only JSON properties become high-signal `openapi_response_only_json_property` hypotheses;
- only candidates whose JSON parent already exists in the captured request are activated;
- declared types, `readOnly`, `writeOnly`, `required`, schema refs, reason, and source path are preserved as provenance rather than treated as evidence;
- a single unambiguous `boolean` declaration may use JSON `true` as the candidate/control probe;
- a single unambiguous `integer` declaration may use JSON `1`;
- schema-typed findings record `discovery_mode: schema_typed`, the value, and the value kind;
- candidate and random-name control always receive the exact same typed value;
- OpenAPI-derived one-level scaffold descriptors remain passive in v0.9;
- remote references and external file references remain disabled;
- schema `enum`, `default`, `example`, `examples`, and `const` values are not consumed as probes;
- unions such as `[boolean, null]` remain conservative and do not authorize a typed representative.

## Core release rule

> OpenAPI may tell ParamIntel what is worth testing, where it may belong, and—in a narrow scalar case—which JSON type to use. Only live application behavior, repeated trials, paired random-name controls, and existing confidence/evidence rules may produce a finding.

Schema metadata is hypothesis input, not proof of runtime behavior.

## Example

Suppose the selected request schema contains:

```text
$.profile.name
```

while the selected response schema additionally contains:

```text
$.profile.beta_access
```

ParamIntel can prioritize:

```text
$.profile.beta_access
source: openapi_response_only_json_property
placement: existing_parent
```

If the schema declares exactly `boolean`, ParamIntel tests:

```json
candidate: {"profile":{"beta_access":true}}
control:   {"profile":{"zz_pi_random":true}}
```

Only the parameter name differs.

## Acceptance proof

The automated and manual v0.9 acceptance suites prove:

- a real existing-parent OpenAPI candidate can be confirmed at 3/3 candidate changes versus 0/3 control changes;
- generic unknown-field behavior is rejected when the random-name control reproduces it;
- OpenAPI one-level scaffold descriptors remain withheld even when a direct manual request proves the nested field is live;
- a strict boolean field can be discovered from the first pass using JSON `true`, with 3/3 candidate changes and 0/3 control changes;
- generic boolean behavior is rejected at 3/3 candidate changes versus 3/3 control changes;
- a strict integer field can be discovered using JSON `1`, with 3/3 candidate changes and 0/3 control changes;
- a `[boolean, null]` declaration receives no schema-typed probe and produces zero findings even though a direct boolean request proves the endpoint reacts.

## Deliberate boundaries

v0.9 is not an OpenAPI endpoint fuzzer or generic request generator. It does not add:

- broad endpoint enumeration;
- automatic exploitation;
- OpenAPI-derived scaffolding;
- array insertion;
- multi-level JSON synthesis;
- schema value spraying;
- remote specification fetching;
- confidence bonuses from schema declarations.

The v0.8 `-json-scaffold` path remains available only for deterministic `-context-response` candidates with explicit user opt-in.

## Documentation and labs

```text
docs/v0.9-local-openapi-candidate-intelligence.md
docs/v0.9-slice2-openapi-candidate-bridge.md
docs/v0.9-slice3-schema-typed-probes.md
labs/v0.9-openapi-intelligence/README.md
labs/v0.9-schema-typed-probes/README.md
```

These files document the parser boundary, operation/schema matching, candidate activation, typed-probe rule, and reproducible localhost acceptance procedures.
