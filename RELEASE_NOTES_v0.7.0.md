# ParamIntel v0.7.0 — Evidence Fidelity

ParamIntel v0.7.0 improves the fidelity of deterministic response comparison, especially for dynamic non-JSON responses.

## Highlights

- learns stable response features during baseline collection;
- detects normalized Content-Type changes;
- detects eligible response-header additions, removals, and value changes without emitting raw custom-header values;
- detects stable HTML structural changes, including same-size response changes;
- adds conservative line-count and word-count evidence for non-JSON text;
- preserves JSON path semantics as the authoritative JSON comparison model;
- keeps candidate/control verification and confidence scoring unchanged;
- includes end-to-end CLI acceptance and a reproducible localhost evidence-fidelity lab.

## Core release rule

> Only response features that prove stable during baseline collection may become new evidence. Candidate-specific behavior must still survive ParamIntel's paired random-name negative control.

## Acceptance proof

The v0.7 acceptance suite proves:

- same-size HTML structure changes can be confirmed;
- header-only candidate behavior can be confirmed;
- rotating request-ID noise stays outside evidence;
- generic unknown-parameter behavior is rejected by the paired random-name control;
- existing JSON semantic discovery remains intact;
- raw custom response-header values are not copied into findings evidence.

See `docs/v0.7-evidence-fidelity.md` and `labs/v0.7-evidence-fidelity/README.md` for the full design and reproduction steps.
