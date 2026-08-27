# ParamIntel v0.2.0

ParamIntel is an evidence-oriented HTTP parameter discovery and behavioral-analysis tool for authorized web security testing and bug bounty research.

Instead of treating any response difference as a valid parameter, ParamIntel tries to answer a stronger question:

> **Does this specific parameter produce reproducible application behavior that a random unknown parameter does not?**

v0.2 extends the validated v0.1 query engine to form bodies and JSON objects while preserving the same negative-control and reproducibility model.

## What v0.2 adds

- GET/query-string discovery;
- `application/x-www-form-urlencoded` body discovery;
- root JSON property discovery;
- nested JSON object property discovery;
- semantic JSON response comparison;
- paired random-name negative controls;
- repeated verification trials;
- confidence scoring and labels;
- conservative type inference from server validation responses;
- parameter-aware value profiling after discovery;
- verbose acceptance/rejection diagnostics;
- JSON evidence export;
- explicit opt-in before repeatedly probing potentially state-changing methods.

### JSON boundary

v0.2 traverses JSON **objects** only. It intentionally does not mutate arrays yet. Array mutation can mean several different things (modify index 0, every element, append an object, etc.), so ParamIntel does not guess.

## Detection model

```text
raw authenticated request
        |
        v
multi-request baseline
        |
        v
candidate placement generation
(query / form / JSON object paths)
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
confirmed parameter
        |
        v
optional type/value characterization
```

A reported parameter is a **research lead**, not proof of a vulnerability. Authorization, business-logic impact, and exploitability still require manual validation within program scope.

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

In auto mode, JSON requests are tested in both the query string and JSON object locations.

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

`auto` always includes query discovery, then adds form or JSON body discovery according to the request `Content-Type`.

## Verbose diagnostics

Accepted candidate:

```text
[+] $.filters.limit (json)
    candidate: changed 3/3
    control:   changed 0/3
    confidence: 100% HIGH
    accepted: confidence meets 60% threshold
    inferred type: integer (98%; server validation response)
```

Rejected candidate:

```text
[-] debug (query)
    candidate: changed 3/3
    control:   changed 3/3
    confidence: 4% LOW
    rejected: random negative control reproduced candidate behavior
```

## Example JSON result

```json
{
  "version": "0.2.0",
  "target": "https://target.example/api/search",
  "method": "POST",
  "baseline": {
    "samples": 3,
    "stable_json_paths": 3,
    "body_len_min": 12,
    "body_len_max": 12
  },
  "parameters": [
    {
      "name": "limit",
      "location": "json",
      "json_path": "$.filters.limit",
      "confidence": 1.00,
      "confidence_label": "high",
      "candidate_changed": 3,
      "candidate_trials": 3,
      "random_control_changed": 0,
      "random_control_trials": 3,
      "inferred_type": "integer",
      "type_confidence": 0.98,
      "type_evidence": "server validation response",
      "value_profile": [
        {
          "value": "1",
          "value_kind": "integer",
          "status": 200,
          "classification": "behavioral_change"
        }
      ]
    }
  ]
}
```

## Project layout

```text
cmd/paramintel/       CLI and safety boundary
internal/baseline/   baseline collection and request sending
internal/candidates/ candidate wordlists
internal/compare/    semantic response comparison
internal/confidence/ confidence scoring
internal/discovery/  candidate placement, narrowing, verification, controls
internal/httpraw/    raw HTTP request parser
internal/model/      shared evidence/result types
internal/mutate/     query/form/JSON mutation engine
internal/semantics/  type inference and value profiles
```

## Scope and responsible use

Use ParamIntel only on systems you own or are explicitly authorized to test. Respect bug bounty scope, rate limits, forbidden actions, and data-handling rules. ParamIntel discovers and characterizes inputs; it does not automatically claim that a discovered parameter is vulnerable.
