# Changelog

## v0.11.0

- Added evidence-guided semantic-rescue scheduling so bounded request budget is allocated by deterministic evidence tier, candidate-source priority, local contextual relevance, lower screening cost, then stable existing order.
- Preserved the verifier and trust boundary: scheduler priority, OpenAPI metadata, context structure, and AI hypotheses still cannot produce a finding without live repeated candidate/control evidence.
- Added deterministic local rescue relevance from the baseline response or explicit `-context-response`; this ranking path requires no provider call.
- Added per-candidate rescue audit fields for evidence tier, source priority, contextual relevance, deterministic value count, actual AI query status, admitted AI values, budget before/after, requests used, outcome, and verified discovery mode.
- Added top-level value-aware accounting for used requests, verified/miss/budget-exhausted cost, eligible candidates, attempted candidates, deferred candidates, and verified parameters.
- Added the verification-feasibility floor: ParamIntel does not start a new semantic value when the remaining request budget cannot complete candidate screening, the paired random-name control, and configured repeated verification.
- With three trials, the minimum actionable tail budget is eight requests; the frozen 17-candidate zero-signal case drops from 60 to 57 rescue requests without losing a still-verifiable finding.
- Fixed AI semantic-value audit reporting so `ai_queried` reflects an actual provider query rather than merely invoking an advisor whose candidate-query budget is already exhausted.
- Added body-less OpenAPI operation support: GET-style operations without request bodies can still contribute operation and response-schema intelligence, while response-only JSON properties remain informational unless the captured request has a JSON object body.
- Added dedicated v0.11 acceptance coverage for context ranking, screening-cost tie-breaking, zero-signal behavior, Gemini semantic values, full CLI built-ins, fixture correctness, and request accounting.
- Realistic OWASP crAPI acceptance confirmed the 57/64 verification floor on an authenticated endpoint, context-driven Gemini prioritization of `role` without a false finding, conservative body-less OpenAPI handling, and negative-control rejection of plausible OpenAPI coupon fields when generic unknown JSON keys reproduced the same behavior.
- PortSwigger Web Security Academy mass-assignment acceptance confirmed positive recall: response-derived `$.chosen_discount` and one-level-scaffolded `$.chosen_discount.percentage` both verified at 3/3 candidate changes versus 0/3 random-name control changes.
- Updated the CLI/report version and release-facing documentation to `ParamIntel v0.11.0`.

## v0.10.0

- Added the optional AI Semantic Value Advisor for known candidate parameters whose behavior depends on application-specific values that generic probing and deterministic semantic profiles do not cover.
- Kept AI outside the evidence boundary: model output remains a bounded hypothesis source, while only live application behavior, repeated candidate/control trials, and the existing confidence model can produce a finding.
- Added `-ai-value-advisor`, `-ai-value-budget`, and `-ai-value-candidate-budget` CLI controls. The existing `-value-aware-budget` remains the hard target-request budget and cannot be bypassed by AI.
- Reused the provider-neutral AI layer with Gemini as the first Semantic Value Advisor implementation and preserved local API-key handling through `GEMINI_API_KEY`.
- Added local admission rules for AI values: query/form values are normalized to bounded semantic strings, JSON values are limited to string/boolean/integer/null, and duplicates, deterministic values, malformed typed values, unsupported kinds, overlong values, and payload-like syntax are rejected locally.
- Added bounded enum-like semantic hints from structurally relevant response fields such as `available_visibilities`, while continuing to exclude raw response text and arbitrary primitive response values from provider input.
- Added local structural relevance scoring so scarce AI candidate-query budget is spent on candidates supported by observed application structure before unrelated generic candidates.
- Added `ai_value_advisor` report metadata and `discovery_mode: ai_value_aware` provenance for findings verified through AI-suggested values.
- Added the reproducible `labs/semantic-value-advisor` localhost acceptance lab.
- Manual A/B acceptance confirmed the control run reported zero parameters, while the AI-enabled run proposed `visibility=internal`; ParamIntel independently verified it at 3/3 candidate changes versus 0/3 same-value random-name control changes with 1.00 HIGH confidence.
- Preserved deterministic value-aware discovery ordering: if an existing semantic profile such as `debug=true` verifies the candidate, the AI Semantic Value Advisor is never called.
- Preserved OpenAPI authority boundaries, JSON evidence semantics, rate-limit/backoff integrity, controlled JSON scaffolding, state-changing-method authorization, and the existing confidence model.
- Updated the CLI/report version and release-facing documentation to `ParamIntel v0.10.0`.

## v0.9.3

- Moved ParamIntel's supported Go baseline from Go 1.23 to Go 1.26.
- CI now validates the full release gate on Go 1.26.x and Go 1.27.x.
- Added an HTTP/2 transport regression proving JSON response semantics remain intact through the ParamIntel send/snapshot boundary.
- Added a regression proving HTTP 200 responses carrying `Retry-After` remain ordinary responses and are not classified as rate-limit/backoff evidence.
- Preserved `github.com/pb33f/libopenapi v0.25.0`; the Go runtime migration intentionally does not include an OpenAPI dependency upgrade.
- The migration was validated first with an isolated Go 1.23 / 1.26 / 1.27 compatibility matrix before changing the supported minimum.

## v0.9.2

- Fixed inconsistent schema-typed probe eligibility for nullable OpenAPI scalar properties.
- Preserved OpenAPI `nullable` metadata through schema property descriptors, candidate descriptors, and finding provenance.
- Schema-typed boolean/integer shortcuts now require an unambiguous **non-nullable** scalar declaration; nullable scalars fall back to the normal generic verification path.
- OpenAPI 3.0 `type: boolean` plus `nullable: true` and mixed/legacy OpenAPI 3.1 documents using the same form no longer receive a schema-typed `true` shortcut merely because `schema.Type` contains one scalar.
- Preserved existing conservative handling for OpenAPI 3.1 `type: [boolean, null]` unions and `anyOf`/`oneOf` ambiguity.
- Did not add `null` as a probe value, reinterpret malformed schemas, expand OpenAPI authority, or change confidence/evidence rules.
- Added A–D regression coverage for OpenAPI 3.0 nullable, OpenAPI 3.1 type unions, mixed 3.1 legacy nullable, and `anyOf(boolean,null)`, plus explicit non-nullable boolean/integer typed-probe regressions.
- Updated the CLI/report version and release-facing documentation to `ParamIntel v0.9.2`.

## v0.9.1

- Fixed replay of Burp-captured mobile requests that explicitly advertise `Accept-Encoding` values such as `gzip, deflate, br`.
- ParamIntel now removes the captured `Accept-Encoding` header immediately before dispatch so Go's configured HTTP transport can negotiate and transparently decode supported response compression.
- Preserved all other captured request headers, request bodies, authorization context, mutation semantics, pacing, redirect policy, and evidence rules.
- Added regression coverage that starts with a captured `Accept-Encoding: gzip, deflate, br` request, serves gzipped JSON, and proves ParamIntel receives decoded JSON with semantic paths intact.
- Real-world authorized acceptance reproduced the original failure at HTTP 200 with a 943-byte encoded body and `0` stable JSON paths, then confirmed the fix on the unchanged request at HTTP 200 with a 5101-byte decoded body and `203` stable JSON paths.
- Updated the CLI/report version and release-facing documentation to `ParamIntel v0.9.1`.

## v0.9.0

- Added local OpenAPI candidate intelligence through a new `-openapi` CLI flag for OpenAPI 3.x documents.
- Added a production OpenAPI parser based on `github.com/pb33f/libopenapi v0.25.0`, chosen after a parser spike under the Go 1.23 baseline.
- Kept OpenAPI loading local-only: remote references, external file references, automatic specification discovery, BaseURL, and BasePath resolution remain disabled.
- Added deterministic operation selection from the captured request method/path, including exact-path preference and fail-closed rejection of ambiguous templated matches.
- Added request-schema selection from captured request Content-Type and response-schema selection from the stable baseline using exact status code, status class, then `default` fallback.
- Added bounded schema walking for object properties, internal refs, `allOf`, declared types, `readOnly`, `writeOnly`, and `required` metadata.
- Kept arrays out of candidate insertion and skipped `oneOf`/`anyOf` subtrees as ambiguous in this release.
- Added the `openapi_response_only_json_property` candidate source for properties present in the selected response schema but absent from the selected request schema.
- Added OpenAPI placement classification for `existing_parent` and passive `one_level_scaffold` descriptors.
- Activated only `existing_parent` OpenAPI candidates. OpenAPI-derived scaffold descriptors remain withheld and cannot use v0.8 scaffold authority.
- Preserved OpenAPI provenance on findings, including schema path, declared types, `readOnly`, `writeOnly`, `required`, schema reference, reason, and priority metadata.
- Added schema-typed probing for OpenAPI response-only JSON candidates with exactly one supported scalar declaration: `boolean -> true` and `integer -> 1`.
- Isolated schema-typed candidates in first-pass grouping so a type-correct scalar is used before narrowing and repeated verification.
- Preserved paired random-name control symmetry for typed probes: candidate and control receive the exact same typed value and value kind; only the leaf name changes.
- Added `discovery_mode: schema_typed`, `discovery_value`, and `discovery_value_kind` audit output for findings confirmed through the typed OpenAPI path.
- Kept unions and ambiguous/multi-type declarations conservative. Declarations such as `[boolean, null]` do not authorize a schema-typed representative.
- Did not consume schema `enum`, `default`, `example`, `examples`, `const`, or format-derived values.
- Kept schema metadata outside confidence scoring: OpenAPI may influence candidate acquisition and a narrow probe type, but only live application behavior, repeated trials, paired controls, and existing evidence rules can produce a finding.
- Added end-to-end CLI tests proving strict boolean OpenAPI candidates survive the first pass, generic boolean behavior is rejected by the typed random control, and OpenAPI scaffold descriptors remain inactive.
- Added unit coverage for parser/ref boundaries, exact-vs-template path matching, ambiguous template rejection, response status fallback, schema walking, placement classification, typed-probe admission, and typed-group isolation.
- Added `labs/v0.9-openapi-intelligence` manual acceptance covering real existing-parent behavior, generic-noise rejection, and withheld OpenAPI scaffolding.
- Added `labs/v0.9-schema-typed-probes` manual acceptance covering strict boolean, generic boolean noise, strict integer, and union fallback behavior.
- Manual Windows acceptance confirmed boolean `true` and integer `1` findings at 3/3 candidate changes versus 0/3 control changes, generic boolean behavior at 3/3 versus 3/3 was rejected, and `[boolean, null]` remained untyped with zero findings even though a direct boolean request proved the endpoint was live.
- Preserved v0.5 rate-limit evidence integrity, v0.6 AI authority boundaries, v0.7 evidence fidelity, v0.8 controlled context-response scaffolding, state-changing-method authorization, and the existing confidence model.
- Updated the CLI/report version and release-facing documentation to `ParamIntel v0.9.0`.

## v0.8.0

- Added deeper structured JSON discovery through controlled one-level parent scaffolding for deterministic response-derived candidates.
- Context intelligence now distinguishes immediately actionable JSON candidates from `Scaffoldable` candidates whose immediate parent is absent but whose direct ancestor already exists as an object in the captured request.
- Added explicit scaffold metadata (`JSONScaffoldParent`) and `RequiresJSONScaffold()` without making metadata itself sufficient to authorize mutation.
- Added an independent per-mutation `AllowJSONScaffold` gate so object creation must be explicitly authorized at the active mutation boundary.
- Added a strict one-level scaffold mutator that creates only the final missing object component when its direct parent already exists as an object.
- The scaffold path never replaces an existing object, `null`, scalar, or array, and it rejects paths that would require two or more missing object levels.
- Added `-json-scaffold` as an explicit opt-in. It fails closed unless `-context-response` is also supplied, keeping missing-parent paths tied to deterministic response structure rather than generic guessing.
- Generic wordlist candidates and AI Candidate Advisor hypotheses cannot acquire missing-parent scaffold authority in v0.8.
- Scaffold candidates are isolated into one-candidate initial groups rather than being bulk-batched with ordinary contextual or dictionary probes.
- Centralized discovery-side mutation construction so generic probing, repeated verification, paired controls, value-aware rescue, and characterization all apply the same scaffold authorization rule.
- Preserved paired random-name control symmetry: candidate and control receive the exact same scaffold and probe value, and only the leaf parameter name changes.
- Preserved the existing conservative confidence/control model. If behavior is caused by the new parent object or by any child under it, the paired random-name control reproduces the signal and the candidate is rejected.
- Added end-to-end discovery tests for a real response-derived nested field, scaffold-disabled behavior, shared-parent-noise rejection, and scaffold-group isolation.
- Added real CLI acceptance proving `$.profile.settings.beta_access` can be confirmed behind one missing parent while generic scaffold behavior is suppressed by the paired control.
- Added the reproducible `labs/v0.8-json-scaffolding` localhost lab with candidate-specific and shared-parent-noise endpoints, raw request fixtures, a deterministic context-response fixture, and PowerShell reproduction steps.
- Preserved the existing state-changing-method authorization gate, global pacing, known rate-limit/backoff fail-closed handling, v0.6 AI authority boundary, v0.7 evidence-fidelity model, and JSON path comparator semantics.
- Added `docs/v0.8-controlled-json-scaffolding.md` and release-facing documentation describing the exact authorization layers, exclusions, candidate/control symmetry, and acceptance boundary.
- Updated the CLI/report version and release-facing documentation to `ParamIntel v0.8.0`.

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
