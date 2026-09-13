# ParamIntel v0.9 OpenAPI parser spike

This directory is a **research-only nested Go module**. It does not participate in ParamIntel's production build and must not be treated as v0.9 implementation code.

## Question

Can ParamIntel consume a hunter-supplied local OpenAPI document as a deterministic JSON-candidate source while preserving the current Go 1.23 baseline and refusing external-reference I/O?

## Candidate parser under test

- `github.com/getkin/kin-openapi v0.132.0`
- nested module declares `go 1.23`
- parser external refs are explicitly disabled

Why this version: current `kin-openapi` releases require newer Go toolchains, while v0.132.0 declares Go 1.22.5 and is therefore compatible with ParamIntel's current Go 1.23 baseline.

## What the spike probes

1. OpenAPI 3.0.3 basic load + validation.
2. OpenAPI 3.1.0 basic load + validation, including JSON Schema multi-type syntax.
3. OpenAPI 3.2.1 basic parse/validation behavior. This is observational only: accepting a simple 3.2.1 document does **not** prove full 3.2.1 semantic support.
4. Exact path/method traversal to a JSON request schema and a 200 JSON response schema.
5. Internal `$ref` resolution.
6. `allOf` preservation/resolution.
7. `oneOf` preservation/resolution.
8. Cyclic internal refs load without unbounded parser recursion; any future ParamIntel schema walker must still carry its own visited-set/depth budget.
9. External `$ref` is rejected while `IsExternalRefsAllowed=false`.
10. Ambiguous templated paths are observed explicitly. ParamIntel must own the future ambiguity policy instead of trusting parser path selection.

## What this spike deliberately does not do

- no `-openapi` CLI flag;
- no `internal/schemaintel` package;
- no candidate generation;
- no changes to confidence or evidence;
- no automatic schema download;
- no external `$ref` loading;
- no enum/default/example probing;
- no array insertion;
- no changes to v0.8 scaffold authorization;
- no production dependency added to ParamIntel's root `go.mod`.

## Run locally

From this directory:

```powershell
go mod tidy
go test -v ./...
```

The verbose output emits `SPIKE_RESULT` lines for behavior that is intentionally observational rather than a release guarantee.

## Decision gate

The parser is acceptable for a first v0.9 slice only if all of the following remain true:

- Go 1.23 can build and test the spike;
- OAS 3.0 and 3.1 required fixtures pass;
- internal refs and the schema structures needed for request/response comparison are navigable;
- external refs fail closed without any network/file fetch;
- cyclic refs remain bounded at parser load time;
- parser behavior is not mistaken for operation-matching policy;
- any 3.2.1 support claim is limited to what the fixture actually proves.

If this gate passes, the next step is still **design**, not broad implementation: define a small `schemaintel` interface and candidate/provenance model, then add production behavior in separate slices.
