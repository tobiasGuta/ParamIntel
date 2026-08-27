# ParamIntel v0.1.1

ParamIntel is a small, evidence-oriented HTTP query-parameter discovery engine for authorized web security testing and bug bounty work.

Its v0.1.1 goal is deliberately narrow: **discover candidate query parameters, verify that they are actually processed differently from random unknown parameters, and export the evidence that supports the conclusion.**

## Why this exists

Traditional parameter discovery often answers only: `did the response change?`

ParamIntel v0.1.1 adds several checks around that signal:

- imports a real raw HTTP request, preserving authentication headers, cookies, existing query data, method, and body;
- builds a multi-request baseline before probing;
- learns stable JSON paths so per-request values such as request IDs can be ignored;
- uses batch-and-narrow discovery instead of one request per candidate;
- individually verifies surviving parameters;
- runs a random-name negative control beside every verification trial;
- repeats verification for reproducibility;
- produces a confidence score and structured JSON evidence.

## v0.1.1 scope

Implemented:

- raw HTTP request parsing;
- GET/query-string candidate insertion;
- configurable baseline samples;
- batched divide-and-conquer discovery;
- semantic JSON comparison;
- fallback stable-body / body-length comparison for non-JSON responses;
- individual verification;
- random unknown-parameter controls;
- reproducibility trials;
- confidence scoring with `high` / `medium` / `low` labels;
- optional verbose candidate acceptance/rejection diagnostics;
- built-in small bug-hunting candidate set;
- optional custom wordlist;
- JSON report output;
- unit and integration tests.

Not implemented yet:

- insertion into form bodies;
- insertion into JSON/XML bodies;
- nested JSON candidates;
- parameter-specific value profiles/type inference;
- JS/OpenAPI/historical candidate sources;
- Burp extension UI;
- pairwise parameter relationship testing.

Those are intentionally deferred so v0.1 stays testable and trustworthy.

## Build

Requires Go 1.23+.

```bash
go build -o paramintel ./cmd/paramintel
```

## Test

```bash
go test ./...
go test -race ./...
```

## Usage

Save an authorized request from Burp or another proxy as `request.txt`:

```http
GET /api/users?existing=1 HTTP/1.1
Host: target.example
Authorization: Bearer REDACTED_TOKEN
Cookie: session=REDACTED

```

Run:

```bash
./paramintel \
  -request request.txt \
  -wordlist params.txt \
  -baseline 3 \
  -trials 3 \
  -chunk 64 \
  -output result.json
```

Add `-verbose` when you want to see why candidates were accepted or rejected:

```bash
./paramintel -request request.txt -wordlist params.txt -verbose -output result.json
```

A rejected candidate can look like:

```text
[-] debug
    candidate: changed 3/3
    control:   changed 3/3
    confidence: 4% LOW
    rejected: random negative control reproduced candidate behavior
```

For a local HTTP target whose raw request contains only a relative path:

```bash
./paramintel -request request.txt -scheme http
```

## Example output

```json
{
  "target": "https://target.example/api/users?existing=1",
  "method": "GET",
  "baseline": {
    "samples": 3,
    "stable_json_paths": 9,
    "body_len_min": 184,
    "body_len_max": 191
  },
  "parameters": [
    {
      "name": "organization_id",
      "location": "query",
      "confidence": 1.00,
      "confidence_label": "high",
      "candidate_changed": 3,
      "candidate_trials": 3,
      "random_control_changed": 0,
      "random_control_trials": 3,
      "evidence": [
        {
          "kind": "json_path_added",
          "path": "$.organization.visible",
          "after": "b:true"
        }
      ]
    }
  ]
}
```

## Detection model

1. Send several untouched baseline requests.
2. For JSON responses, flatten the response into semantic paths and retain only values stable across all baseline samples.
3. Test candidate names in groups.
4. If a group produces a meaningful difference, split it recursively until individual names remain.
5. For each surviving name, run repeated candidate probes and paired probes using a random parameter name.
6. Score candidates according to reproducibility, negative-control behavior, and evidence diversity.

A reported parameter is still a **research lead**, not proof of a vulnerability. Validate authorization, business logic, and impact manually within program scope.

## Safety

Use ParamIntel only against systems you own or are explicitly authorized to test. Respect program scope, request-rate limits, and destructive-action restrictions. ParamIntel v0.1.1 performs HTTP requests and mutates query strings; it does not attempt exploitation.

## Project layout

```text
cmd/paramintel/       CLI
internal/baseline/   baseline collection + request sending
internal/candidates/ candidate loading
internal/compare/    semantic response comparison
internal/confidence/ confidence scoring
internal/discovery/  batching, narrowing, verification, controls
internal/httpraw/    raw HTTP request parser
internal/model/      shared result types
```
