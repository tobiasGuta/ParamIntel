# Changelog

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
