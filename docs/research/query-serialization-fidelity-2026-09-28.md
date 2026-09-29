# Query serialization fidelity spike — 2026-09-28

Status: **actionable transport/evidence defect confirmed; research branch only**.

Branch: `research/query-serialization-fidelity-20260928`

Successful evidence-capture run (research assertions intentionally fail inside a continue-on-error step):
https://github.com/tobiasGuta/ParamIntel/actions/runs/36507234695

## Scope

Test whether ParamIntel can add an ordinary query candidate without changing unrelated captured query bytes, and whether an OpenAPI 3.2.1 `in: querystring` operation is protected from generic ordinary-query discovery.

No production mutation code, evidence scoring, confidence thresholds, rate-limit handling, or OpenAPI candidate admission was changed.

The spike ran on Go 1.26.x and Go 1.27.x and produced the same behavioral result.

## Source facts

OpenAPI 3.2.1 distinguishes:

- `in: query`: ordinary named query parameters;
- `in: querystring`: the entire URL query string is one value described through `content`.

The two forms MUST NOT coexist on the same operation/path-item.

OpenAPI 3.2.1 also documents form-urlencoded behavior where a logical value of `a + b` serializes as `a+%2B+b`.

References:

- https://spec.openapis.org/oas/v3.2.1.html#parameter-object
- https://spec.openapis.org/oas/v3.2.1.html#url-percent-encoding
- https://spec.openapis.org/oas/v3.2.1.html#encoding-the-x-www-form-urlencoded-media-type

Go's `net/url.URL.Query()` parses `RawQuery` into `url.Values` and explicitly documents that malformed value pairs are silently discarded. `url.ParseQuery` considers an unescaped semicolon invalid. `url.Values.Encode()` serializes the parsed map in key-sorted URL-encoded form.

References:

- https://pkg.go.dev/net/url#URL.Query
- https://go.dev/src/net/url/url.go

## Current ParamIntel path

Current `internal/mutate.Apply` handles query mutations by:

1. parsing `out.URL`;
2. calling `u.Query()`;
3. applying `q.Set(name, value)`;
4. replacing `u.RawQuery` with `q.Encode()`.

This converts the captured raw query into a logical map and rebuilds the entire query whenever any query candidate is tested.

Baseline requests with no mutation do not take this rewrite path. Query candidate/control requests do.

That means the experimental variable can currently be:

> candidate/control name + unrelated query normalization

rather than strictly:

> candidate/control name

## Wire-fidelity experiment

A local HTTP server recorded the actual `RequestURI` received after ParamIntel's normal mutation and send path.

Probe value:

```text
logical:    a + b
serialized: a+%2B+b
```

The probe's own primitive form encoding is correct.

Strict replay invariant:

> Existing raw-query bytes must remain byte-for-byte unchanged when a new ordinary hidden parameter is appended.

### Results

| Captured raw query | Result | Observed change |
|---|---|---|
| `mode=basic&q=a+b` | PASS | unchanged; probe appended |
| `mode=basic&q=a%2Bb` | PASS | literal plus preserved |
| `mode=basic&q=a%20b` | FAIL | `%20` rewritten to `+` |
| `q=search&mode=basic` | FAIL | keys reordered |
| `flag&mode=basic` | FAIL | bare `flag` rewritten to `flag=` |
| `filter[role]=admin&mode=basic` | FAIL | brackets rewritten as `%5B...%5D` |
| `tag=a&tag=b` | PASS | duplicate array values preserved |
| percent-encoded whole-query JSON | FAIL | converted into a form key, normalized, moved, and given `=` |
| `mode=basic&sig=abc;def` | FAIL | entire `sig` pair silently disappeared |
| `mode=basic&path=%2fprivate` | FAIL | percent-triplet spelling normalized to `%2F` |
| `empty=&mode=basic` | PASS | explicit empty value preserved |

Result: **4/11 pass, 7/11 fail**, identical on Go 1.26.x and Go 1.27.x.

Not every byte difference is semantically invalid under OpenAPI. For example, both `%20` and `+` can decode to a space in form-urlencoded contexts. The defect is stricter: ParamIntel is a replay/evidence tool, so unrelated captured transport bytes should not change during a candidate-specific experiment unless the mutation model explicitly authorizes that representation change.

The semicolon case is unambiguously material: an existing captured field is removed from the outbound request.

## OpenAPI 3.2.1 querystring integration experiment

Fixture:

- bodyless `GET /search`;
- captured query is percent-encoded JSON;
- OpenAPI 3.2.1 declares exactly one `in: querystring` parameter with `content: application/json`;
- no ordinary `in: query` parameter exists.

Expected safe behavior before ParamIntel has an explicit querystring serializer:

> Do not send generic ordinary-query probes for this matched operation.

Observed behavior:

1. two baseline requests preserved the captured query exactly;
2. generic query discovery remained active;
3. built-in/query candidates were sent in ordinary name/value groups;
4. each probe reinterpreted the whole-query JSON through `url.Values` and rewrote it.

Example observed probe shape:

```text
/search?account_id=<token>&admin=<token>&...&%7B%22foo%22%3A%22a+%2B+b%22%7D=
```

Result: **FAIL on Go 1.26.x and Go 1.27.x.**

This is a real boundary failure: ParamIntel's OpenAPI layer correctly does not create active `querystring` candidates, but the generic discovery path independently enables ordinary `query` fuzzing and defeats that conservative boundary.

## Evidence-model impact

The random-name control reduces false-positive risk but does not make this harmless.

Potential consequences include:

- false negatives when a rewritten/dropped query invalidates authentication, signatures, routing, or application state before the candidate can be evaluated;
- unnecessary survivor work when normalization alone changes the response relative to the untouched baseline;
- candidate/control requests that differ in ordering because their names sort differently;
- broken mobile/API replay where request signatures or cache keys depend on raw query representation;
- inability to safely operate on whole-query formats such as JSON without a dedicated serializer;
- possible side effects from sending structurally invalid requests to endpoints that interpret the entire query as one value.

The evidence model assumes candidate/control testing changes only the intended experimental variable. Current query reconstruction violates that assumption for these cases.

## Recommendation

### URGENT FIX — ordinary query replay fidelity

Before expanding OpenAPI 3.2 query intelligence, replace whole-query parse-and-reencode behavior for ordinary hidden-parameter mutation with a raw-preserving mutation path.

Required invariant:

> Unrelated raw query segments remain byte-for-byte identical.

A minimal correction should not require a new confidence model. It should be a transport/mutation-layer correction covered by regression tests.

The implementation should explicitly define behavior for a candidate name already present in the captured query rather than accidentally converting discovery into parameter-pollution testing.

### DESIGN CANDIDATE — explicit querystring location

Do **not** map OAS 3.2 `in: querystring` into the existing `LocationQuery`.

A later passive slice can carry:

- source = OpenAPI querystring;
- media type;
- schema path/ref;
- serialization metadata;
- no active mutation until a serializer and same-representation candidate/control invariant exist.

Until then, when the matched OpenAPI operation declares `in: querystring`, ParamIntel should conservatively withhold generic ordinary-query discovery for that operation.

### No evidence-score redesign

Do not change:

- confidence thresholds;
- random-name control semantics;
- stable-401 baseline behavior;
- Retry-After classification;
- AI evidence authority;
- response comparison.

This is a transport-isolation defect, not evidence-scoring weakness.

## Research artifacts

- `internal/mutate/query_serialization_fidelity_spike_test.go`
- `cmd/paramintel/querystring_fidelity_spike_test.go`
- `.github/workflows/query-serialization-fidelity-spike.yml`

The tests are behind the `queryfidelityspike` build tag so normal ParamIntel CI remains unaffected.
