# Changelog

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
