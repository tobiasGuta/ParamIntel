# ParamIntel v0.11.2 — In-Scan Authentication-Drift Evidence Integrity

**Status: Release candidate; not yet tagged or published.**

This maintenance release packages the already-merged PR #47 authentication-like status-drift safeguard and its regression coverage. It does not add new candidate-generation or finding-scoring behavior.

## Why this release exists

An initially stable, explicitly expected HTTP 200 baseline can stop representing the operator's intended authenticated application state later in the same scan. A subsequent HTTP 401/403 could previously enter the ordinary response-difference comparator without first confirming that the unmodified captured request still reached the expected status.

A parameter finding should not be built on an expired session or another unexplained change to the baseline state.

## What changed

With `-expect-baseline-status` explicitly set to a successful HTTP **2xx** status, ParamIntel now checks subsequent HTTP 401 or 403 probe responses for authentication-like baseline drift.

For **GET, HEAD, and OPTIONS**:

1. A candidate/group/control/semantic/characterization probe returns HTTP 401/403.
2. ParamIntel sends **two additional unmodified captured requests** through its normal HTTP client.
3. If both clean rechecks return the configured expected status, the original probe remains eligible for ordinary candidate-specific verification and paired random-name controls.
4. If both clean rechecks return HTTP 401/403, ParamIntel stops with a typed `BaselineStatusDriftError` and does not produce a normal findings report.
5. If the rechecks are mixed or unexpected, the scan also fails closed.

For **state-changing methods**, ParamIntel aborts on the suspicious probe without issuing extra unchanged-request replays that might create side effects.

Additional clean recheck requests are limited to **12 per scan**. Exhausting this limit fails closed.

## Explicit non-goals

- HTTP 200 or a matching baseline-status expectation is **not** proof of an authenticated identity.
- Only observed HTTP 401/403 anomalies trigger this initial safeguard. A status-preserving login page, an HTTP redirect, a silent cookie/session change, or another non-401/403 application-state drift is not covered.
- A candidate-specific 401/403 is **not** automatically rejected if unchanged requests remain stable; normal repeated candidate/control verification still decides whether the difference matters.
- This release does not add credential refresh, replay retries of mutated probes, extra active probing for every response, or automatic vulnerability judgment.
- HTTP 200 with `Retry-After` remains a response observation, not automatically a rate-limit classification. Existing typed 429 and qualifying 503 backoff behavior is preserved.
- The new behavior is **opt-in** through `-expect-baseline-status` with a stable expected 2xx. Other baseline-testing modes retain their previous behavior.

## Acceptance evidence

- PR #47's GitHub Actions passed on **Go 1.26.11 and Go 1.27.4**: `go test ./...`, `go vet ./...`, race detection, Linux build, Windows cross-build, JSON evidence backend compatibility, OpenAPI untrusted-input hardening, and `govulncheck`.
- On Fedora, targeted discovery and CLI tests passed against local `httptest` fixtures.
- A separate Fedora local mock-API acceptance run produced the exact expected **200, 200, 200, 401, 401, 401** sequence and a confirmed-drift error; the user additionally verified that `lab/drift-report.json` was **not** generated.
- v0.11.2 release hardening adds targeted HTTP 403 regressions for both confirmed drift and candidate-specific behavior with stable clean rechecks. Their results must pass on the release-candidate branch before merge/tag.

## Release gate

On the candidate branch, verify:

```bash
go test ./...
go vet ./...
go test -race ./...
go build ./cmd/paramintel
go run ./cmd/paramintel -version
```

The CLI must print `ParamIntel v0.11.2`. The existing CI also checks Go version compatibility, Windows cross-builds, JSON evidence contract, vulnerability reachability, and OpenAPI hardening.

Do not claim a published GitHub release or tag until it has actually been created and verified. The next research item is status-preserving authentication drift, which remains outside the scope of v0.11.2.
