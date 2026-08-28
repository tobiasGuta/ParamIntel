# Changelog

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
