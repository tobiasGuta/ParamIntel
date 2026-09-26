# OpenAPI 3.2 compatibility spike — 2026-09-26

Status: **research only; no production dependency or discovery-policy change**.

Branch: `research/openapi-32-compat-20260926`. GitHub Actions run: https://github.com/tobiasGuta/ParamIntel/actions/runs/36277109074

## Question

Does updating `github.com/pb33f/libopenapi` from pinned v0.25.0 to v0.38.7 improve ParamIntel's *actual* schema-driven discovery, as opposed to only parsing a newer OpenAPI document?

## Method

The same eight passive fixtures were executed using Go 1.26 with the pinned dependency and the experimental dependency. The experimental runner updated `go.mod` and `go.sum` only in its checkout. A small runner-only API adaptation replaced the old `BuildV3Model() []error` handling with the new `BuildV3Model() error` handling in `internal/schemaintel/load.go`. This change was **not** committed to production code.

The experiment observed the parser's exposed parameter locations/content and multipart model metadata alongside ParamIntel's existing candidate admission decisions. It sent **no target HTTP requests**. The candidate-dependency runner also executed `go test ./... -count=1`, which passed. Neither race/vet nor the full test suite on Go 1.27 were part of this experimental runner.

## Results

| Fixture | Pinned v0.25.0 | Experimental v0.38.7 |
|---|---|---|
| OAS 3.0 JSON internal ref | `$.beta_access` active | Identical |
| OAS 3.1 JSON internal ref | `$.beta_access` active | Identical |
| OAS 3.2.1 JSON internal ref | `$.beta_access` active | Identical |
| OAS 3.2.1 `querystring` | Parser exposes `search:querystring` and schema properties; no active JSON candidates from bodyless GET | Identical |
| Invalid mixed `query` + `querystring` | Both locations accepted by parser; no active candidates | Identical |
| OAS 3.2.1 multipart `itemSchema` and `itemEncoding` | High-level media-type fields unavailable; current Analyze rejects non-JSON media | Both high-level fields exposed and non-nil; current Analyze still rejects non-JSON media |
| OAS 3.2.1 union and array | `$.safe` admitted; ambiguous and array paths withheld | Identical |
| External file reference | Parse fails closed with local-only settings | Identical |

Seven of eight fixture outputs were identical. The only observed semantic-model exposure delta was the multipart metadata. The current transport/admission path does not use it.

### Migration issue

An unmodified ParamIntel source checkout does **not** compile against v0.38.7: `BuildV3Model` now returns a single `error`, while `load.go` calls `len(buildErrs)` and `errors.Join(buildErrs...)`. The runner-only error-API adaptation fixed this compilation issue. Existing tests passed afterward, but that does not prove broad behavioral compatibility beyond the tested fixtures.

### Interpretation

1. **The old dependency already parses basic 3.2.1 documents and exposes querystring content schemas.** The immediate discovery limitation is ParamIntel's JSON-body-only candidate bridge, not inability to read these structures.
2. **Querystring needs a separate admission and serializer design.** An entire query string is one media-typed value under OpenAPI 3.2.1, not automatically a collection of safely replayable ordinary `in: query` parameters. Avoid flatten-and-send behavior until serialization, negative-control equivalence, captured request placement, and safety gates are explicit.
3. **Validation is separate from parsing.** Both library versions accepted the fixture that mixes `in: query` and `in: querystring`. OpenAPI 3.2.1 prohibits that mix. This matters if a future ParamIntel querystring bridge relies on schema declarations as trusted placement instructions.
4. **Multipart metadata is now represented by the newer parser, but remains outside ParamIntel's JSON-only active model.** It is not sufficient reason on its own for the dependency migration.
5. External references stay disabled, and unions/arrays remain withheld from active discovery. Do not relax these boundaries as part of a dependency upgrade.

## Decision

- **RECORD:** Existing v0.25.0 already preserves the tested OpenAPI 3.2.1 querystring structure.
- **DESIGN CANDIDATE:** Passive querystring descriptor extraction for same-operation, local-only, `application/x-www-form-urlencoded` schema intelligence, with a clear separate source type and *no active requests* in the first slice.
- **TEST:** Reject conflicting `in: query` and `in: querystring` declarations before any future querystring-derived candidate admission, including parameters inherited from Path Item.
- **IGNORE FOR NOW:** Immediate production upgrade to v0.38.7 based solely on this OpenAPI 3.2 feature. Consider independently when supported functionality or a relevant security/reliability fix justifies migration and the expanded regression gate passes.

Do not alter the current confidence model, baseline status policy, random-name same-value controls, JSON-body mutation authorization, external-ref policy, or Life360 retry/rate-limit classification based on this spike.

## Evidence and reproduction

- Test source: `internal/schemaintel/openapi32_compat_spike_test.go`.
- Isolated dependency comparison workflow: `.github/workflows/openapi-compat-spike.yml`.
- Successful run and downloadable logs: https://github.com/tobiasGuta/ParamIntel/actions/runs/36277109074
- OAS 3.2.1 `querystring` semantics: https://spec.openapis.org/oas/v3.2.1.html#parameter-object
- OAS 3.2.1 media-type `itemSchema` / `itemEncoding`: https://spec.openapis.org/oas/v3.2.1.html#media-type-object

This experiment is intentionally a compatibility spike, not a complete 3.2.1 validation suite or proof that a new parameter is discoverable in a live target.
