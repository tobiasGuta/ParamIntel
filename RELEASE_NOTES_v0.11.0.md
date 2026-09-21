# ParamIntel v0.11.0 — Evidence-Guided Adaptive Rescue

ParamIntel v0.11.0 improves how a bounded semantic-rescue request budget is spent.

The core rule is unchanged:

> **AI, OpenAPI, and contextual evidence may tell ParamIntel what is worth testing. Only live application behavior, repeated trials, paired random-name controls, and the existing confidence model may produce a finding.**

## What changed

v0.10 could have many value-aware candidates competing for one small request budget. v0.11 makes that allocation evidence-aware.

Rescue candidates are ordered deterministically by:

1. evidence tier;
2. candidate-source priority;
3. local contextual relevance;
4. lower deterministic screening cost;
5. stable existing order.

Direct application evidence outranks weaker hypotheses. AI candidate provenance never outranks stronger deterministic target evidence.

## Evidence tiers

- **A — direct application evidence:** actionable context-response and OpenAPI response-only candidates.
- **B — contextual / structural evidence:** scaffoldable response-derived candidates and strong local semantic matches.
- **C — AI candidate hypotheses.**
- **D — local semantic heuristics:** built-in profiles such as `debug`, `format`, `sort`, and `order`.
- **E — generic candidates.**

## Request accounting

v0.11 records per-candidate rescue provenance and cost:

- evidence tier and reason;
- source priority;
- local contextual relevance;
- deterministic value count;
- whether the AI provider was actually queried;
- admitted AI values;
- budget before and after;
- requests consumed;
- outcome;
- discovery mode when verified.

The report also summarizes total rescue cost, misses, verified cost, deferred candidates, and verified parameters.

## Verification-feasibility floor

ParamIntel no longer starts a new semantic value when the remaining request budget cannot complete:

- one candidate screen;
- one paired random-name control;
- repeated candidate verification;
- repeated control verification.

With three trials, that minimum actionable budget is eight requests.

The frozen 17-candidate zero-signal case moves from 60 rescue requests to 57 without dropping a finding that could still have been fully verified.

## OpenAPI compatibility

Body-less OpenAPI operations are now supported for response intelligence.

A GET operation without a request body can still be matched and its response schema inspected, but ParamIntel does not invent a JSON request body or convert response-only JSON fields into writable candidates when no captured JSON object body exists.

## AI audit correctness

The semantic-value audit now distinguishes an actual provider call from merely reaching the advisor wrapper after its candidate-query budget has been exhausted.

## Validation

### Dedicated v0.11 lab

The dedicated localhost acceptance lab validates:

- context relevance under a tight budget;
- lower-cost tie-breaking;
- zero-signal behavior;
- Gemini semantic values;
- the verification-feasibility floor;
- full CLI built-in ordering.

### OWASP crAPI

Realistic crAPI runs confirmed:

- 57/64 zero-signal tail behavior on an authenticated API;
- context-backed promotion of `role` before generic semantic candidates;
- Gemini value proposals without false findings;
- conservative body-less OpenAPI handling;
- OpenAPI response-only coupon hypotheses rejected when paired random-name controls reproduced the same generic behavior.

### PortSwigger Web Security Academy

The mass-assignment lab provided a positive-recall case.

From the related checkout response, ParamIntel derived and independently verified:

- `$.chosen_discount`
- `$.chosen_discount.percentage`

Both produced 3/3 candidate changes versus 0/3 random-name control changes.

This validates response-derived candidate acquisition plus controlled one-level JSON scaffolding against an external training target with known ground truth.

ParamIntel identifies the hidden parameter structure; exploitability and the business-impact value remain a separate validation step.

## Non-goals

v0.11 does not add:

- reinforcement learning;
- model-confidence scoring;
- automatic vulnerability judgment;
- aggressive semantic-profile truncation;
- looser confidence thresholds;
- weaker negative controls;
- autonomous exploitation.

The scheduler changes **what gets tested first**, not **what counts as proof**.
