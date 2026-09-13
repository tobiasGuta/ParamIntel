# Changelog

## v0.7.0

- Added deterministic response-feature extraction for normalized content type, text metrics, conservative HTML classification, HTML structural hashing, and eligible response-header fingerprints.
- Baseline construction now records feature stability so only observations proven stable across baseline samples can participate in the new evidence paths.
- Added `content_type` evidence for normalized Content-Type changes from a stable baseline.
- Added `header_added`, `header_removed`, and `header_value_changed` evidence for eligible stable response headers. Findings report header names only; raw custom-header values and internal fingerprints are not emitted as evidence.
- Added `html_structure_changed` evidence for stable non-JSON HTML structure changes, including same-size response changes that body-length comparison alone can miss.
- Added conservative `line_count` and `word_count` evidence for non-JSON textual responses when probes leave the learned baseline range plus tolerance.
- Preserved existing JSON path semantics as authoritative for JSON responses; v0.7 body metrics do not replace `json_path_added`, `json_path_removed`, or `json_value_changed` evidence.
- Preserved the existing status, stable-body, and body-length comparison paths. v0.7 evidence is additive rather than a replacement comparator.
- Added a conservative response-header exclusion boundary for sensitive, transport-level, cache, request-ID, correlation, and tracing headers before header evidence comparison.
- Kept candidate/control verification and confidence scoring unchanged. New evidence kinds still require repeated candidate behavior that survives paired random-name negative controls.
- Added end-to-end CLI acceptance proving same-size HTML structure detection, header-only behavior, rotating request-ID rejection, and no raw custom-header value leakage.
- Added acceptance coverage proving generic unknown-parameter behavior is rejected when the paired random-name control reproduces it.
- Added JSON regression acceptance proving stable JSON behavior remains discoverable while a rotating JSON response property stays outside evidence.
- Added the reproducible `labs/v0.7-evidence-fidelity` localhost lab with HTML, generic-noise, and JSON scenarios plus raw request fixtures and PowerShell instructions.
- Added `docs/v0.7-evidence-fidelity.md` documenting the evidence model, stability boundary, evidence kinds, privacy/noise handling, acceptance coverage, and deliberate release boundaries.
- Updated the CLI/report version and release-facing documentation to `ParamIntel v0.7.0`.

## v0.6.0

- Added the optional AI Candidate Advisor as a candidate-acquisition layer while preserving deterministic verification as the only authority for findings and confidence.
- Added a provider-neutral internal `Provider` interface with Gemini as the first adapter.
- Added `-ai-advisor`, `-ai-provider`, `-ai-model`, `-ai-api-key-env`, `-ai-context-response`, `-ai-candidate-budget`, and `-ai-timeout` CLI controls.
- Gemini now defaults to `gemini-3.5-flash-lite`, with provider-supported model overrides available through `-ai-model`.
- AI configuration and API-key validation occur locally; Gemini reads `GEMINI_API_KEY` by default and no literal API-key CLI flag is accepted.
- The normal AI path reuses one response already collected during baseline as structural context, avoiding an extra target request. `-ai-context-response` remains available as an explicit override.
- Added a local sanitizer that sends bounded application structure rather than raw captured HTTP traffic. Hostnames, Authorization/Cookie data, query/form values, JSON primitive values, raw response text, arbitrary headers, and user wordlist names are omitted from provider input.
- Added a deterministic local admission gate that rejects malformed names, inactive locations, impossible JSON parents, already-present parameters, deterministically covered names, duplicates, and suggestions beyond the configured budget.
- Added AI advisor audit/provenance data including provider/model, context source, suggestion/admission state, rejection reason, tested/verified state, and deterministic trial/confidence data.
- AI priority and reason never affect ParamIntel confidence. A finding is attributed to AI only when the exact placement retains `ai_semantic_hypothesis` provenance and independently passes ParamIntel verification.
- Added a two-minute default AI provider timeout while preserving stateless `store:false` behavior and no automatic retry for timed-out generations.
- Added a reproducible localhost v0.6 acceptance lab. The final default-model run proposed `include_archived`, which ParamIntel independently verified at 3/3 candidate changes versus 0/3 paired random-name control changes with 1.00 HIGH confidence and zero false findings in the acceptance run.
- Updated the release version to `ParamIntel v0.6.0` and refreshed release-facing README and v0.6 lab documentation.

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
- Automated release gates and external Windows acceptance are complete, including mid-scan 429 abort, asymmetric candidate/control invalidation, and observed 100ms/100ms/101ms request-start pacing with `-delay 100ms`.

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
