# ParamIntel v0.11.1 — Baseline Evidence Integrity (release candidate)

ParamIntel v0.11.1 makes completed scan reports explain whether the operator's intended HTTP baseline status matched the observed stable status.

It **does not** infer successful authentication from HTTP 200, assign confidence from HTTP status alone, or change the evidence verifier.

## Report format

A completed scan now contains `baseline.status_evidence`:

```json
{
  "first_observed_status_code": 200,
  "status_stable": true,
  "expected_status_code": 200,
  "expectation": "matched"
}
```

- `first_observed_status_code`: first baseline response's HTTP status; it is not claimed to represent all samples when stability is false.
- `status_stable`: whether all collected baseline samples had the same status.
- `expected_status_code`: operator's explicit `-expect-baseline-status` value; omitted when the option is disabled.
- `expectation`: `matched` if the explicitly expected status was stable and observed; `not_configured` when the operator did not supply an expectation.

Unstable or mismatched *explicit* expectations still terminate the scan before any candidate discovery. These failure cases **do not produce a normal findings report**. The CLI error remains authoritative for a rejected baseline.

## Local acceptance scenarios

| Baseline | Expected status | Behavior | Completed report |
| --- | --- | --- | --- |
| Stable 200 | 200 | Continue discovery | `matched`, observed 200 |
| Stable 401 | 401 | Continue discovery intentionally | `matched`, observed 401 |
| Stable 401 | Not supplied | Existing legacy behavior | `not_configured`, observed 401 |
| Stable 401 | 200 | Stop before discovery | No normal report |
| Unstable status | Explicit | Stop before discovery | No normal report |

The status audit includes no captured cookies, tokens, or response bodies. Existing baseline summary fields are unchanged.

## Not changed

No modifications to HTTP transport, redirect policy, compression, OpenAPI candidate selection, AI generation, rate-limit classification (including HTTP 200 with Retry-After), negative controls, trial counts, or confidence scoring.

## Verification

New deterministic unit and localhost CLI tests cover status-intent JSON output and fail-closed behavior. CI must pass on supported Go 1.26 and 1.27 before merge or release. This document records a proposed release, **not** a published tag.
