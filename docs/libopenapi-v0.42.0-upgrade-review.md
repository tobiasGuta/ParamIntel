# libopenapi v0.42.0 upgrade review (draft)

This is a **review candidate**, not permission to merge. Production stays on v0.25.0 until the draft PR is separately approved.

## Proposed change

- Upgrade `github.com/pb33f/libopenapi` from `v0.25.0` to `v0.42.0`.
- Adapt `BuildV3Model()` error handling from the old `[]error` return to `error`.
- Carry forward focused parsing/evidence regression fixtures and bounded synthetic resource checks.
- No HTTP transport, request mutation, candidate scoring, evidence classification, or network behavior is intentionally changed.

## Observed grounds for the candidate

The previous isolated compatibility matrix on `test/libopenapi-compat-20261101` passed both versions and a byte-for-byte comparison of the expanded schema-intelligence report snapshots:

- Matrix: https://github.com/tobiasGuta/ParamIntel/actions/runs/41320760144
- Baseline Go CI: https://github.com/tobiasGuta/ParamIntel/actions/runs/41320760150

The v0.25.0 parser rejected an OpenAPI 3.1 spec containing a literal `$ref` field inside example data. The v0.42.0 parser, with the required API adapter, accepted that document and maintained the expected candidate set. Both versions preserved response-only candidate paths, types, status/media selection, read-only/required/nullable properties, component reference provenance, and conservative handling of ambiguous compositions and arrays on synthetic fixtures.

v0.42.0 rejected a deliberately alias-amplified YAML input that v0.25.0 accepted. Cyclic reference fixtures completed without a hang. The larger synthetic document (3 MiB) stayed below 256 MiB peak RSS in both versions in one GitHub-hosted Linux run. These are **fixture observations**, not comprehensive security or performance guarantees.

## Existing operational boundaries

- `schemaintel.Parse` explicitly disables remote and external-file references.
- `MaxDepth` and `MaxNodes` bound the **schema walker after model construction**, not the YAML parser or the full `BuildV3Model()` process.
- The synthetic 256 MiB peak-RSS ceiling and 8-second per-fixture deadline are **test-harness checks**. They are not enforced at runtime for operator-supplied OpenAPI documents.
- The tested subprocess resource harness relies on Linux `/proc` and `syscall.Getrusage`.
- Unknown or complex schema constructs should not silently become newly admitted parameters; the full report snapshot comparison and existing regression suite serve as alarms.

## Review gates before merging

1. CI is green on the PR head across supported Go versions, race/vet/build targets, and the new regression fixtures.
2. Inspect `go.mod` and `go.sum` for unintended dependency changes; reproduce with `go mod tidy`.
3. Validate several representative real-world OpenAPI documents (no credentials or customer data) and compare complete `Report` outputs with the pinned release.
4. Decide whether untrusted local specs require an explicit input-size cap or parser isolation before broadening use. This is **not** solved by the dependency bump itself.
5. Do not enable excessive YAML aliases as a workaround and do not merge without explicit operator approval.

## Outcome

Design candidate. No emergency fix indicated from current evidence. No modifications to `main` or live targets.

## Real public OpenAPI compatibility (November 1, 2026)

A read-only GitHub Actions differential test pinned five public API definitions from
`APIs-guru/openapi-directory` at commit
`f04b8d0bcd39c52e1cf3ad7a5fe744709832ae49`.
Each source is downloaded over HTTPS from that exact commit, restricted to 6 MiB,
and SHA-256 verified in the draft PR's public-spec compatibility workflow. **No API
requests were sent to 1Password, Ably, Spotify, Trello, or Stripe.**

| Document | Size | v0.25.0 | v0.42.0 |
|---|---:|---|---|
| 1Password Events | 17,339 bytes | Parsed | Parsed |
| Ably Platform | 44,030 bytes | Parsed | Parsed |
| Spotify | 289,324 bytes | Rejected | Rejected |
| Trello | 550,934 bytes | Parsed | Parsed |
| Stripe | 3,726,556 bytes | Rejected | Rejected |

- Accepted documents: 3/5.
- Selected deterministic operations examined: 29.
- Successfully analyzed JSON request/response operation combinations: 16.
- For the three accepted documents, compared full `Report` snapshots including
  operation/path selection, request and response property descriptors, candidates,
  placements, types, source references, and skipped/rejected properties.
- The generated snapshots were **byte-for-byte identical** across parser versions
  in the isolated review run:
  https://github.com/tobiasGuta/ParamIntel/actions/runs/41323538299
- The PR now contains a repeatable, pinned, checksum-verified test and explicit
  expectations for both parser-rejection classes.

**Interpretation:** The Spotify document references `../policies.yaml`;
external-file reference resolution is intentionally disabled. Both versions reject it.
The Stripe document has a circular-reference build failure in both versions. This
is an existing compatibility limitation. These two specifications did **not**
produce schema-intelligence candidate reports and must not be counted as
successful evidence-equality cases. A future change that accepts either document
should receive a separate semantic/evidence review.

## Decision: production input and parser-resource safety

**Threat model:** `-openapi` currently reads a local operator-selected document
using `os.ReadFile` without a pre-read size guard. `schemaintel.Parse` rejects
only empty input before parsing. The schema walker applies `MaxDepth` and
`MaxNodes` **after** libopenapi constructs the document model. Neither
prevents a high-complexity YAML document from exhausting CPU or RAM during
model construction. `GOMEMLIMIT` is a soft Go heap target, not an operating
system memory cap.

**Disposition: DESIGN CANDIDATE for a separate hardening change.** This is an
existing condition, not a new dependency regression. For local, operator-reviewed
input with external/remote references disabled, it need not be bundled into this
small dependency upgrade. For ingestion of untrusted, automatically obtained,
or third-party-submitted documents, enforce resource limits **before enabling that
workflow**. A candidate design should include:

1. A bounded file read **before** `os.ReadFile`-sized allocations, plus
   `schemaintel.Parse` admission checks for other callers. Consider a
   configurable, documented 16 MiB *default* document ceiling, not a global
   unchangeable constant. This accommodates the 3.7 MiB Stripe input and
   many larger real-world specs, but size alone does not bound expansion.
   Establish the final default using additional large-spec profiling.
2. A clear fail-closed error for oversized documents with the configured
   byte ceiling, and unit/CLI tests for exact-boundary inputs. No silent
   truncation and no implicit downloads of external references.
3. An isolated parsing subprocess for genuinely untrusted input, with an
   enforceable wall-clock timeout and OS memory/cgroup limits; terminate
   and discard partial results on timeout, crash, or resource exceedance.
4. Keep the parsed-candidate evidence contract independent of source
   complexity: no fallback to partial schemas, guessed fields, or reduced
   verification when limits are hit.
5. Validate the actual default and override policy against representative
   public specifications and the existing synthetic alias/depth/cycle corpus.

**Merge policy:** Keep PR #42 draft until its own CI and public-spec matrix are
green and the operator explicitly accepts the existing local-spec trust
assumption. If ParamIntel is to accept arbitrary untrusted OpenAPI documents,
treat the separate resource-boundary change as a **blocking prerequisite**.

