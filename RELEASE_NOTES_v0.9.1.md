# ParamIntel v0.9.1 — Response Decoding Reliability

ParamIntel v0.9.1 is a focused patch release for replayed HTTP requests captured from mobile applications.

## Fix

Captured requests may include an explicit header such as:

```http
Accept-Encoding: gzip, deflate, br
```

ParamIntel previously replayed that header verbatim. That could cause the Go HTTP client to return encoded response bytes instead of transparently decoded JSON.

v0.9.1 removes the captured `Accept-Encoding` header immediately before dispatch. The configured HTTP transport can then negotiate and decode supported response compression normally.

All other request headers and request mutation behavior remain unchanged.

## Regression coverage

A new automated test starts from a request advertising `gzip, deflate, br`, serves gzipped JSON, and verifies that ParamIntel receives decoded JSON with semantic paths intact.

The standard CI gate remains green for tests, vet, race detection, Linux build, and Windows cross-build.

## Acceptance proof

Before the fix, the reproduced request produced:

```text
status: 200 (stable=true)
body length: 943-943 bytes
stable JSON paths: 0
```

A manual `Accept-Encoding: identity` workaround produced:

```text
status: 200 (stable=true)
body length: 5101-5101 bytes
stable JSON paths: 203
```

After the fix, the original request was replayed unchanged with `Accept-Encoding: gzip, deflate, br` and produced:

```text
status: 200 (stable=true)
body length: 5101-5101 bytes
stable JSON paths: 203
```

This confirms the response-decoding regression is fixed.

## Scope

v0.9.1 does not add a new discovery feature or change ParamIntel's evidence model. It is a transport/replay correctness patch for reliable JSON-semantic analysis.
