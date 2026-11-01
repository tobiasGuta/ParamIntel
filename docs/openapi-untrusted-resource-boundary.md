# OpenAPI untrusted-input resource hardening (review candidate)

This change addresses the known gap where ParamIntel read `-openapi` using
`os.ReadFile` without a byte ceiling and constructed the libopenapi model before
the schema-walk `MaxDepth`/`MaxNodes` limits applied.

**Scope:** local OpenAPI input admission and an opt-in Linux subprocess used for
validation/analysis. No transport, candidate ranking, mutation, discovery,
baseline or evidence-scoring change is intended. No automatic ingestion,
downloading or remote reference resolution has been added.

## Default local mode

- Local OpenAPI reads require a regular file and use an explicit bounded read.
  On Linux the file is opened with `O_NONBLOCK|O_NOFOLLOW|O_CLOEXEC`;
  this rejects a final-component symlink, and prevents named pipes from
  blocking `open()` before the regular-file check. Other platforms reject
  symlinks and special files through `Lstat` pre-admission; non-Linux
  `-openapi-untrusted` remains unsupported. A parent-directory symlink
  and blocking I/O on a remote filesystem are not comprehensively sandboxed.
  The opened descriptor's `stat` check rejects a known-oversized file early;
  `LimitReader(max+1)` also detects concurrent growth instead of silently
  truncating it.
- `schemaintel.Parse` itself checks a default **16 MiB** maximum before
  libopenapi allocates/expands the schema graph. `ParseWithLimit` accepts an
  explicit bounded limit for intentional larger specs.
- `-openapi-max-bytes` can be configured from **1 through 67,108,864 bytes**
  (64 MiB maximum). A file larger than the configured size fails closed;
  no HTTP request should be issued.
- `-openapi` with no other new flags keeps the original trusted local workflow,
  but is subject to the new size admission limit.

## Explicit opt-in for untrusted local documents (Linux)

For a locally saved public/untrusted schema, operators may choose:

```sh
paramintel -request captured.txt -openapi third-party.yaml \
  -openapi-untrusted -openapi-max-bytes=16777216 \
  -openapi-parse-timeout=15s -openapi-memory-mib=768
```

- Fail closed on non-Linux platforms and Linux kernels older than 4.7.
- An internal worker process receives the already byte-bounded document through
  stdin; the parent **never calls libopenapi on this untrusted buffer**.
- The parent sends **only the method, sanitized path, Content-Type header,
  JSON property/object structure, baseline status and baseline content type**
  for analysis. Other request headers, cookies, query parameters, the
  original hostname, scalar JSON values and unused baseline detail are
  stripped. JSON property names are retained because they determine
  candidate evidence. The worker also receives **no ambient scanner
  environment variables** (including API keys or proxy credentials); its
  environment contains only resource-limit and Go runtime settings.
- The worker applies Linux `RLIMIT_DATA` to data/heap/mmap allocations and
  `RLIMIT_CPU` before decoding or parsing input. Linux 4.7+ enforces
  `RLIMIT_DATA` for `mmap`; the resource ceilings cannot be raised by the
  worker after they are applied.
- The parent enforces a **wall-clock deadline** using `exec.CommandContext`
  (default 15 seconds per worker, allowed range 1 second to 1 minute).
  Timeouts, signals, crashes and malformed/oversized responses discard all
  partial schema evidence.
- Default `RLIMIT_DATA`: 768 MiB; configurable 256 to 2048 MiB. The default
  `GOMEMLIMIT` is set to half the selected kernel data limit **as an advisory
  GC target**, not a replacement for kernel enforcement.
- Child stdout/stderr sizes are bounded. The child validates the parsed document
  **before the scanner sends any baseline HTTP requests**, then a second
  fresh worker reconstructs the schema model and produces the complete
  `schemaintel.Report` using the captured request template and baseline.
- The report is accepted only as a complete JSON result. It uses the exact
  same `schemaintel.Analyze` code, `DefaultConfig`, typed fields, candidate
  paths, skipped reasons and provenance as trusted mode.
- External file references and remote references remain disabled within
  `schemaintel.ParseWithLimit`.

**Important security boundary:** a data/mmap RLIMIT is not an exact resident
memory (`RSS`) cap or a fully isolated security sandbox; the worker process
inherits the launching account's filesystem access and network privileges.
This change contains parser CPU/memory risks but does not defend against an
arbitrary-code-execution bug in the parser or prevent a malicious library from
making network calls. Avoid automatically ingesting unknown third-party
documents until an OS-level filesystem/network sandbox and explicit input
trust policy have been independently reviewed.

## Verification gates

1. Validate file exactly at size limit and at size limit + 1; reject symlinks,
   special files (including a writerless FIFO without blocking), and invalid
   limits.
2. Reject oversize documents directly from `schemaintel.Parse` before building
   any model.
3. Compare complete CLI findings in trusted vs `-openapi-untrusted` modes
   using a local test server (no external target).
4. Verify oversized input is rejected before HTTP requests in either mode.
5. Verify invalid worker phases, mismatched size budgets, cancellation,
   output truncation, non-Linux behavior and child memory/CPU limit checks.
6. Run `go test ./...`, `go vet ./...`, race tests and Linux/Windows builds
   in GitHub Actions. The Windows binary supports trusted size-bounded local
   parsing but deliberately rejects `-openapi-untrusted`.

## Limitations and follow-ups

- The 16 MiB default is provisional; representative public large specs should
  inform policy before changing it.
- A small compressed/aliased YAML input can still be computationally expensive;
  byte admission alone is not sufficient.
- Process limits protect only the opt-in isolated path, not trusted `Parse`
  callers. Those callers are expected to pass trusted input and use the
  in-memory size admission check.
- A stronger untrusted ingestion pipeline must additionally define a
  permission-restricted worker account, no-network/filesystem sandbox,
  containment of subprocess descendants, and explicit authorization for
  automated input retrieval.
- This is a **draft hardening PR**; do not merge without operator review.
