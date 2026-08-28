# ParamIntel v0.3.0

ParamIntel is an evidence-oriented HTTP parameter discovery and behavioral-analysis tool for authorized web security testing and bug bounty research.

Instead of treating any response difference as a valid parameter, ParamIntel tries to answer a stronger question:

> **Does this specific parameter produce reproducible application behavior that a random unknown parameter does not?**

v0.3 improves the part that happens before verification: finding application-specific parameter names worth testing. It can compare a JSON request with a related JSON API response, prioritize response-only properties, and then pass those exact candidates through the same v0.2 verification and negative-control model.

## What v0.3 adds

- related-response candidate intelligence with `-context-response`;
- structured JSON property harvesting instead of arbitrary response-word scraping;
- request-versus-response structural comparison;
- exact contextual JSON placements such as `$.chosen_discount` and `$.filters.limit`;
- contextual candidates tested before generic wordlist placements;
- candidate provenance in JSON findings;
- observed JSON type metadata from the context response;
- raw Burp-style HTTP response or bare JSON context input;
- regression coverage for discovering a mass-assignment-style field that is absent from the supplied wordlist.

v0.3 preserves:

- query-string discovery;
- `application/x-www-form-urlencoded` body discovery;
- root and nested JSON object discovery;
- multi-request baselines;
- semantic response comparison;
- batch-and-recursive narrowing;
- paired random-name negative controls;
- repeated verification trials;
- confidence scoring;
- conservative type inference and optional value characterization;
- explicit opt-in for repeated potentially state-changing requests.

### Context-intelligence boundary

Context harvesting intentionally stays narrow:

- only JSON property **keys** become contextual candidates;
- string values in a response are never split into candidate words;
- a nested response-only property is actionable only when its parent JSON object already exists in the request;
- arrays are not traversed for insertion targets;
- v0.3 does not synthesize missing nested object scaffolding;
- a context-derived property is still only a candidate until it passes the normal ParamIntel verifier.

This avoids turning a related API response into a large, noisy wordlist.

## Detection model

```text
raw authenticated request
        |
        +-------------------------------+
        |                               |
        v                               v
multi-request baseline        optional related JSON response
                                        |
                                        v
                              request/response structural diff
                                        |
                                        v
                              response-only exact candidates
                                        |
        +-------------------------------+
        |
        v
candidate placement generation
(context candidates first, then generic query/form/JSON candidates)
        |
        v
batch + recursive narrowing
        |
        v
individual candidate verification
        |
        +--> random unknown parameter control
        |
        v
reproducibility + confidence
        |
        v
confirmed parameter + provenance
        |
        v
optional type/value characterization
```

A reported parameter is a **research lead**, not proof of a vulnerability. Authorization, business-logic impact, and exploitability still require manual validation within program scope.

## Why contextual candidates matter

A generic wordlist can only test names already known to the researcher. APIs often expose additional object properties in related responses that are absent from the normal write request.

For example, suppose the normal checkout request contains:

```json
{
  "chosen_products": [
    {
      "product_id": "1",
      "quantity": 1
    }
  ]
}
```

and a related API response contains:

```json
{
  "chosen_products": [],
  "chosen_discount": {
    "percentage": 0
  }
}
```

v0.3 can derive this exact actionable candidate without requiring `chosen_discount` in a wordlist:

```text
$.chosen_discount
```

The nested response field:

```text
$.chosen_discount.percentage
```

is observed but is not automatically inserted because `$.chosen_discount` does not yet exist as an object in the request. ParamIntel does not fabricate that structure in v0.3.

## Context-response workflow

Save the authorized request you want to probe:

```http
POST /api/checkout HTTP/1.1
Host: target.example
Cookie: session=REDACTED
Content-Type: text/plain;charset=UTF-8

{"chosen_products":[{"product_id":"1","quantity":1}]}
```

Save a related response from Burp as `checkout-response.txt`:

```http
HTTP/2 200 OK
Content-Type: application/json

{"chosen_products":[],"chosen_discount":{"percentage":0}}
```

Run:

```powershell
.\paramintel.exe `
  -request .\burprequests\checkout.txt `
  -context-response .\burpresponses\checkout-response.txt `
  -locations json `
  -baseline 5 `
  -trials 3 `
  -chunk 4 `
  -allow-state-changing `
  -verbose `
  -output .\findings.json
```

A contextual finding can look like:

```json
{
  "name": "chosen_discount",
  "location": "json",
  "json_path": "$.chosen_discount",
  "candidate_sources": [
    {
      "source": "context_response_only_json_property",
      "path": "$.chosen_discount",
      "observed_type": "object",
      "priority": 100
    }
  ],
  "confidence": 1.00,
  "confidence_label": "high",
  "candidate_changed": 3,
  "candidate_trials": 3,
  "random_control_changed": 0,
  "random_control_trials": 3
}
```

The response observation explains **why the candidate was tested**. The confidence still comes from active verification, not from merely seeing the property in a response.

## Safety guard for state-changing methods

GET, HEAD, and OPTIONS can run normally.

POST, PUT, PATCH, DELETE, and other methods require explicit acknowledgement because ParamIntel replays the supplied request many times:

```bash
paramintel -request request.txt -allow-state-changing
```

Use this only after confirming that repeated requests are authorized and will not cause unwanted side effects.

## Build

Requires Go 1.23+.

Linux/macOS:

```bash
go build -trimpath -o paramintel ./cmd/paramintel
```

Windows PowerShell:

```powershell
go build -trimpath -o paramintel.exe .\cmd\paramintel
```

Cross-build Windows amd64:

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o paramintel-windows-amd64.exe ./cmd/paramintel
```

## Test

```bash
go test ./...
go test -race ./...
go vet ./...
```

Or:

```bash
make check
```

## Query parameter discovery

Save an authorized request from Burp:

```http
GET /api/users HTTP/1.1
Host: target.example
Authorization: Bearer REDACTED
Cookie: session=REDACTED

```

Run:

```bash
./paramintel \
  -request request.txt \
  -wordlist params.txt \
  -baseline 5 \
  -trials 3 \
  -chunk 32 \
  -verbose \
  -output findings.json
```

## Form-body discovery

Example request:

```http
POST /search HTTP/1.1
Host: target.example
Content-Type: application/x-www-form-urlencoded
Authorization: Bearer REDACTED

q=test&mode=basic
```

Run:

```bash
./paramintel \
  -request request.txt \
  -locations form \
  -allow-state-changing \
  -verbose \
  -output findings.json
```

`-locations auto` is the default. For form requests, auto mode tests query parameters and form-body parameters.

## JSON and nested JSON discovery

Example:

```http
POST /api/search HTTP/1.1
Host: target.example
Content-Type: application/json
Authorization: Bearer REDACTED

{"filters":{"status":"active"}}
```

With default `-json-depth 3`, ParamIntel can test candidate properties at:

```text
$
$.filters
```

A finding can therefore be represented as:

```json
{
  "name": "limit",
  "location": "json",
  "json_path": "$.filters.limit"
}
```

Run:

```bash
./paramintel \
  -request request.txt \
  -locations json \
  -json-depth 3 \
  -allow-state-changing \
  -verbose \
  -output findings.json
```

In auto mode, ParamIntel always tests query candidates. It adds form discovery for form-encoded requests and JSON discovery whenever the captured body is a parseable JSON object, even when the application uses a non-JSON MIME type such as `text/plain`.

## Type inference

ParamIntel is intentionally conservative. High-confidence type inference comes from application responses such as:

```json
{"error":"limit must be an integer"}
```

which can produce:

```json
{
  "inferred_type": "integer",
  "type_confidence": 0.98,
  "type_evidence": "server validation response"
}
```

Parameter-name heuristics can provide weaker hints for obvious names such as `limit` or `include_deleted`; those are marked with lower confidence.

The observed type from `-context-response` is provenance, not proof that the write endpoint accepts that type.

## Value profiles

Characterization happens **only after discovery and negative-control verification**.

Examples:

- boolean-like parameters: `true`, `false`, `1`, `0`;
- integer-like parameters: `0`, `1`, `10`, `-1`;
- `format`: `json`, `xml`, `html`;
- `sort` / `order`: `asc`, `desc`;
- `include` / `fields` / `expand`: small representation values.

For JSON parameters, boolean and integer values are sent as real JSON types rather than strings.

Disable characterization when you only want discovery:

```bash
./paramintel -request request.txt -characterize=false
```

## Locations

```text
-locations auto
-locations query
-locations form
-locations json
-locations query,json
```

## Verbose diagnostics

A context-derived accepted candidate can look like:

```text
[+] $.chosen_discount (json)
    source: context_response_only_json_property $.chosen_discount (observed object)
    candidate: changed 3/3
    control:   changed 0/3
    confidence: 100% HIGH
    accepted: confidence meets 60% threshold
```

Rejected generic candidate:

```text
[-] debug (query)
    candidate: changed 3/3
    control:   changed 3/3
    confidence: 4% LOW
    rejected: random negative control reproduced candidate behavior
```

## Project layout

```text
cmd/paramintel/        CLI and safety boundary
internal/baseline/    baseline collection and request sending
internal/candidates/  generic candidate wordlists
internal/compare/     semantic response comparison
internal/confidence/  confidence scoring
internal/contextintel/ structured request/response candidate intelligence
internal/discovery/   placement, narrowing, verification, controls
internal/httpraw/     raw HTTP request parser
internal/model/       shared evidence/result types
internal/mutate/      query/form/JSON mutation engine
internal/semantics/   type inference and value profiles
```

## Scope and responsible use

Use ParamIntel only on systems you own or are explicitly authorized to test. Respect bug bounty scope, rate limits, forbidden actions, and data-handling rules. ParamIntel discovers and characterizes inputs; it does not automatically claim that a discovered parameter is vulnerable.

Raw Burp requests and responses can contain session cookies, authorization headers, identifiers, and target data. Keep local research artifacts out of source control. The default `.gitignore` excludes `burprequests/`, `burpresponses/`, and `wordlists/` for this reason.
