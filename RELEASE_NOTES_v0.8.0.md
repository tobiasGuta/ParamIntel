# ParamIntel v0.8.0 — Deeper Structured JSON Discovery

ParamIntel v0.8.0 adds **controlled one-level JSON scaffolding** for high-confidence candidates derived from a related JSON response.

## Highlights

- context intelligence can now classify response-only nested fields whose immediate parent object is absent from the captured request;
- new `-json-scaffold` opt-in admits those narrowly classified candidates only when `-context-response` is also supplied;
- the mutator can create exactly one missing object level when its direct ancestor already exists as an object;
- existing objects, `null`, scalars, and arrays are never replaced by the scaffold path;
- paths requiring two or more missing object levels remain out of scope;
- scaffold candidates are isolated from bulk first-pass groups;
- the paired random-name control receives the exact same scaffold and probe value so only the leaf name differs;
- generic behavior caused by the newly-created parent is rejected when the random-name control reproduces it;
- generic wordlist and AI candidates cannot invent missing parent paths;
- existing v0.5 rate-limit integrity, v0.6 AI boundaries, and v0.7 evidence-fidelity rules remain unchanged.

## Core release rule

> ParamIntel may create exactly one missing JSON object level only for a deterministic response-derived candidate, only when its direct ancestor already exists as a request object, and only when the user explicitly enables `-json-scaffold` together with `-context-response`.

Scaffolding is a candidate-placement capability. It does not create evidence and does not bypass deterministic verification.

## Candidate/control example

Candidate:

```json
{"profile":{"settings":{"beta_access":"probe"}}}
```

Paired control:

```json
{"profile":{"settings":{"zz_pi_random":"probe"}}}
```

The scaffold and value are identical. Only the leaf name changes.

## Acceptance proof

The v0.8 acceptance suite proves:

- a real response-derived nested field behind one missing parent can be confirmed;
- candidate-specific behavior reaches repeated verification with 3/3 candidate changes and 0/3 random-control changes in the controlled fixture;
- behavior caused by any child under the synthesized parent is rejected by the paired random-name control;
- scaffold candidates are never sent when scaffolding is disabled;
- existing non-object or already-present parent values are not replaced;
- two-level missing object invention remains rejected;
- the real CLI path requires `-context-response`, `-json-scaffold`, and the existing state-changing-method authorization gate.

See:

```text
docs/v0.8-controlled-json-scaffolding.md
labs/v0.8-json-scaffolding/README.md
```

for the full design, safety boundaries, and reproduction steps.
