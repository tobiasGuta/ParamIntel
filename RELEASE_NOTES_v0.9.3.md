# ParamIntel v0.9.3 — Supported Go Runtime Baseline

ParamIntel v0.9.3 is a transport and build-baseline maintenance release.

## What changed

ParamIntel previously declared Go 1.23 as its minimum version and ran its authoritative CI only on the Go 1.23 line.

Go 1.23 is no longer a supported Go release line, while ParamIntel depends heavily on Go's HTTP, TLS, URL, and networking behavior for the correctness of its evidence boundary.

v0.9.3 moves the supported baseline to:

```text
minimum: Go 1.26
CI:      Go 1.26.x + Go 1.27.x
```

## Why this matters

ParamIntel's evidence path depends on the HTTP transport accurately representing application behavior:

```text
HTTP response
    ↓
Go HTTP transport
    ↓
ParamIntel snapshot
    ↓
baseline / probes / controls
    ↓
evidence
```

v0.9.1 already demonstrated that transport behavior can materially affect semantic evidence when captured `Accept-Encoding` headers prevented transparent response decoding.

Keeping ParamIntel's supported runtime on maintained Go releases is therefore part of transport and evidence reliability, not merely compiler housekeeping.

## Compatibility experiment

Before changing the minimum version, ParamIntel's full release gate was executed across:

```text
Go 1.23.x — historical reference
Go 1.26.x — supported minimum
Go 1.27.x — supported line
```

All three lanes passed:

```text
go test ./...
go vet ./...
go test -race ./...
go build -trimpath -o paramintel ./cmd/paramintel
Windows amd64 cross-build
```

The production baseline was changed only after that matrix remained green.

## New transport regressions

v0.9.3 adds explicit coverage for two transport-sensitive invariants.

### HTTP/2 semantic preservation

A TLS test server negotiates HTTP/2 and returns JSON. The regression proves ParamIntel receives and snapshots the expected semantic JSON paths intact.

### HTTP 200 + Retry-After

Some real APIs return successful HTTP 200 responses that also contain a `Retry-After` header. ParamIntel intentionally does not classify those responses as rate limiting.

The limiter policy remains:

```text
429                         -> rate limit
503 + valid Retry-After     -> server backoff
503 without valid Retry-After -> ordinary response
200 + Retry-After           -> ordinary response
```

## Dependency boundary

This release does not upgrade `github.com/pb33f/libopenapi`.

ParamIntel remains on:

```text
github.com/pb33f/libopenapi v0.25.0
```

The OpenAPI dependency upgrade will be evaluated separately against ParamIntel's existing compatibility fixtures for refs, unions, nullability, ambiguity, schema selection, and candidate provenance.

Keeping the runtime migration separate makes regressions easier to attribute.

## Evidence model

v0.9.3 does not change:

- candidate generation authority;
- candidate/control verification;
- confidence scoring;
- OpenAPI typed-probe rules;
- rate-limit classification;
- AI Candidate Advisor authority;
- JSON scaffold authority;
- discovery semantics.

This is a transport/runtime support release only.
