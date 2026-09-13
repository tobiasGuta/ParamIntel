# ParamIntel v0.9 OpenAPI parser spike

This directory is a **research-only nested Go module**. It does not participate in ParamIntel's production build and must not be treated as v0.9 implementation code.

## Question

Can ParamIntel consume a hunter-supplied local OpenAPI document as a deterministic JSON-candidate source while preserving the current Go 1.23 baseline and refusing external-reference I/O?

## Decision

**Preferred parser for the first production v0.9 slice: `github.com/pb33f/libopenapi v0.25.0`, pinned initially.**

The comparison found a material reason not to use the last checked Go-1.23-compatible `kin-openapi` release:

- `kin-openapi v0.135.0` declares Go 1.22.5 and therefore fits ParamIntel's Go 1.23 baseline, but its validator rejects an ordinary OpenAPI 3.1 / JSON Schema union type such as `type: [string, null]` with `unsupported 'type' value "null"`;
- `libopenapi v0.25.0` declares Go 1.23.0 and accepts the same OpenAPI 3.1 union fixture, preserving both `string` and `null` in the high-level schema model.

This does not mean `libopenapi` supports every OpenAPI 3.1 or 3.2.1 feature. It means it passed the specific compatibility surface ParamIntel needs for the next design slice better than the compatible `kin-openapi` candidate did.

## Compatibility matrix

| Probe | kin-openapi v0.135.0 | libopenapi v0.25.0 | v0.9 implication |
|---|---|---|---|
| Go 1.23 toolchain | Pass | Pass | no ParamIntel toolchain bump required |
| OpenAPI 3.0.3 fixture | Pass | Pass | supported research baseline |
| OpenAPI 3.1 `type: [string, null]` | **Rejects** | **Pass** | prefer libopenapi |
| Exact request/response schema traversal | Pass | Pass | operation-local schema intelligence is feasible |
| Internal `$ref` | Pass | Pass | internal refs are usable |
| `allOf` | Pass | Pass | composition can be inspected |
| `oneOf` | Pass | Pass | composition can be inspected, but policy remains ParamIntel's |
| Cyclic internal refs | Loads boundedly | Loads boundedly | production walker still needs visited-set/depth limits |
| External remote `$ref` with external I/O disabled | Explicitly rejects | Does not fetch; model build returns diagnostics | production must fail/skip schema intelligence conservatively |
| Ambiguous templated paths | Parser accepts document | Parser accepts document | ParamIntel must detect/reject ambiguous operation matches |
| Basic OpenAPI 3.2.1 document | Accepted | Accepted | **full 3.2.1 semantic support remains unproven** |

## Security boundary for production

If `libopenapi` is promoted into the root module, ParamIntel should use an explicit configuration equivalent to:

- `AllowFileReferences = false`;
- `AllowRemoteReferences = false`;
- no `BasePath`;
- no `BaseURL`;
- no automatic schema download;
- hunter supplies the root OpenAPI document locally;
- internal references inside that root document are permitted;
- any unresolved/external reference that affects the selected operation must cause schema intelligence to skip/fail closed rather than silently inventing a candidate.

The spike installed a sentinel remote URL handler and proved that it was **not invoked** for the external-reference fixture while remote refs were disabled.

## What the spike proves

1. ParamIntel can retain Go 1.23 while using `libopenapi v0.25.0`.
2. OpenAPI 3.0.3 and the tested OpenAPI 3.1 JSON Schema features can be represented.
3. A captured operation's request JSON schema and response JSON schema are navigable.
4. `allOf` and `oneOf` branches are available for a future bounded schema walker.
5. Internal-reference cycles do not require eager unbounded parser recursion, but ParamIntel must still track visited schemas and enforce a depth/node budget.
6. External references can be kept from causing remote I/O.
7. Neither parser should be trusted to make ParamIntel's operation-matching decision when path templates are ambiguous.
8. Both parsers accept the basic `openapi: 3.2.1` fixture, but this spike makes **no claim** that either implements the complete OpenAPI 3.2.1 semantic surface.

## What this spike deliberately does not do

- no `-openapi` CLI flag;
- no `internal/schemaintel` production package;
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
go vet ./...
go test -v ./...
go test -race ./...
```

The verbose output emits `SPIKE_RESULT` lines for behavior that is intentionally observational rather than a release guarantee.

## Recommended next slice

The next step is still **design/representation, not active scanning**.

### v0.9 Slice 1 — schema intelligence model

Define a small `internal/schemaintel` boundary and tests for:

1. loading a local OpenAPI document with all external I/O disabled;
2. exact captured-request method/path matching with explicit ambiguous-template rejection;
3. JSON request media-type selection from the captured request;
4. response schema selection from the actual baseline status and content type;
5. bounded schema flattening for object properties plus internal `$ref`/`allOf`, with explicit handling policy for `oneOf`/`anyOf`;
6. producing **candidate descriptors only** — no target requests yet;
7. provenance such as `openapi_response_only_json_property` including declared type and annotations (`readOnly`, `writeOnly`, `required`) without affecting confidence;
8. classification of one-level missing-parent candidates so v0.8 scaffolding can later be extended through an explicit OpenAPI provenance authorization gate.

Only after that representation layer is reviewed and green should a later slice admit OpenAPI-derived candidates into ParamIntel's existing deterministic verifier.
