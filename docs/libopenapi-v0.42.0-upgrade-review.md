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
