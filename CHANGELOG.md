# Changelog

## v0.5.0

- Added evidence-integrity protection for definite rate-limit/backoff responses before they can become behavioral snapshots.
- HTTP `429 Too Many Requests` now produces a typed `BackoffError` and is never passed to the comparator as discovery evidence.
- HTTP `503 Service Unavailable` is treated as server backoff only when accompanied by a valid `Retry-After` value; ordinary 503 responses remain available to normal comparison.
- Added `Retry-After` parsing for both non-negative integer seconds and HTTP-date values while preserving malformed raw values for diagnostics.
- Added explicit fail-closed behavior during baseline construction: any classified rate-limit/backoff response invalidates the baseline and aborts the scan.
- Added discovery-phase integrity regressions proving group probes, candidate probes, random-name controls, value-aware screens, semantic controls, and characterization cannot turn known limiter behavior into confidence or findings.
- Added a real CLI regression proving a rate-limit abort exits non-zero, explains that the response was not used as discovery evidence, and does not write a normal findings report.
- Added `-delay` as a global minimum interval between outbound request starts. The default remains `0`, preserving existing scan speed unless pacing is explicitly requested.
- Request pacing is applied once at the shared HTTP transport, so baseline, group probing, verification, controls, value-aware rescue, and characterization all follow the same policy without changing discovery algorithms.
- Pacing is context-cancellable, race-safe, and based on request-start spacing rather than unconditional sleep after every response.
- Negative `-delay` values are rejected before network activity.
- Automatic retry/wait-on-`Retry-After` is intentionally not included in v0.5.0, especially for potentially state-changing methods.
- Added a reproducible Windows acceptance lab covering mid-scan 429 aborts, asymmetric candidate/control rate limiting, and `-delay` pacing.
- Release version updated to `ParamIntel v0.5.0`.
- Automated release gates are complete; external Windows acceptance remains required before the v0.5 pull request is considered ready to merge.

## v0.4.0

- Added bounded value-aware discovery for parameters whose behavior only appears for specific semantic values such as `debug=true`.
- Preserved the existing random-string batch/narrow discovery path as the first-pass detector; value-aware discovery is a rescue pass rather than a replacement.
- Added paired same-value random-name controls so semantic probes compare, for example, `debug=true` against `zz_pi_<random>=true`.
- Added repeated explicit-value verification without pooling evidence across different semantic values.
- Added `-value-aware` (default `true`) and `-value-aware-budget` (default `64`) CLI controls.
- Added a hard semantic request budget that reserves complete repeated-verification cost before starting confirmation and never exceeds the configured cap.
- Added deterministic rescue ordering and conservative eligibility: candidates with generic candidate/control activity are not reinterpreted by semantic rescue; only clean misses are eligible.
- Added finding provenance fields `discovery_mode`, `discovery_value`, and `discovery_value_kind`.
- Value-aware discovery remains independent of `-characterize`; `-characterize=false` still permits semantic discovery.
- Added typed JSON semantic discovery, including real JSON booleans and integers rather than string approximations.
- Added composition coverage showing a nested context-derived JSON candidate such as `$.options.include_deleted` can retain response-context provenance and be rescued with typed boolean `true`.
- Exact budget usage now reports as exhausted when the final allowed request consumes the remaining budget.
- Added regression coverage for value-sensitive discovery, same-value generic-noise rejection, insufficient-budget refusal, deterministic budget ordering, and context-plus-value-aware composition.
- Manual Windows acceptance confirmed the same controlled `debug=true` endpoint that v0.3 reported as `0 parameters` is found by v0.4 at 3/3 candidate changes, 0/3 random-control changes, and `1.00 HIGH` confidence.

## v0.3.0

- Added `-context-response` for deriving high-signal JSON parameter candidates from a related API response.
- Added request-versus-response JSON structural comparison so only response-only properties are prioritized.
- Added exact JSON placement hints, allowing contextual fields such as `$.chosen_discount` or `$.filters.limit` to be tested before generic wordlist placements.
- Added candidate provenance to findings, including source path, observed JSON type, and priority.
- Preserved the v0.2 batch/narrow, repeated verification, paired random-control, confidence, and characterization paths unchanged for contextual candidates.
- Added support for raw Burp-style HTTP responses and bare JSON bodies as context inputs.
- Context harvesting is structured-key based only; response text values are never tokenized into candidate names.
- Contextual nested fields are only actionable when their parent object already exists in the request. v0.3 does not synthesize missing object scaffolding or mutate arrays.
- Added regression coverage modeled on the PortSwigger mass-assignment workflow: `chosen_discount` is discovered from response context even when absent from the supplied wordlist.
- Added local Burp request/response and wordlist directories to `.gitignore` to reduce the risk of committing session-bearing research artifacts.

## v0.2.0

- Added `application/x-www-form-urlencoded` body parameter discovery.
- Added JSON body parameter discovery.
- Added nested JSON object discovery with configurable depth.
- Added typed JSON mutation for boolean and integer profile values.
- Added conservative server-response type inference.
- Added parameter-aware value profiling after a parameter is confirmed.
- Added `-locations auto|query,form,json` selection.
- Added `-json-depth` and `-characterize` controls.
- Added an explicit `-allow-state-changing` guard for repeated POST/PUT/PATCH/DELETE probing.
- Added tool version to JSON reports and `-version` CLI output.
- Empty findings now serialize as `[]` rather than `null`.
- Preserved v0.1 query discovery, negative controls, reproducibility, confidence scoring, and verbose rejection diagnostics.

## v0.1.1

- Added verbose candidate acceptance/rejection diagnostics.
- Added stable human-readable confidence labels.
- Kept confidence numeric in JSON with two decimal places.
- Added GraphQL-style query parameter and negative-control regression tests.

## v0.1.0

- Initial high-confidence GET/query parameter discovery engine.
- Multi-request baselines, semantic JSON comparison, batching/narrowing, individual verification, negative controls, confidence scoring, and JSON evidence output.
