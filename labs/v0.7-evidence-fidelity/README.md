# ParamIntel v0.7 Evidence Fidelity Acceptance Lab

This localhost-only lab proves the v0.7 evidence-fidelity behavior against controlled responses before release.

The lab intentionally covers four different conditions:

1. dynamic HTML whose body changes on every request but whose structure is normally stable;
2. a hidden query parameter that changes HTML structure without changing response size;
3. a hidden query parameter whose only meaningful effect is a response header;
4. an endpoint where every unknown query parameter changes the response, so ParamIntel's random-name negative control must reject the apparent signal;
5. a JSON endpoint that proves the existing stable JSON-path comparator still works after the v0.7 non-JSON evidence changes.

## Start the lab

From the repository root in PowerShell:

```powershell
go run .\labs\v0.7-evidence-fidelity
```

The lab listens on `http://127.0.0.1:8093`.

Build ParamIntel in another terminal:

```powershell
go build -o .\paramintel.exe .\cmd\paramintel
```

## Acceptance A: dynamic HTML + header-only behavior

Run:

```powershell
.\paramintel.exe `
  -request .\labs\v0.7-evidence-fidelity\request-html.txt `
  -scheme http `
  -locations query `
  -baseline 3 `
  -trials 3 `
  -chunk 8 `
  -characterize=false `
  -value-aware=false `
  -verbose `
  -output .\labs\v0.7-evidence-fidelity\html-findings.json
```

Expected result:

- exactly two findings: `preview` and `verbose`;
- `preview` is verified by `html_structure_changed`;
- `preview` is not dependent on `body_length`, `line_count`, or `word_count` evidence;
- `verbose` is verified by `header_added` for `X-Debug-Mode`;
- rotating `X-Request-ID` values do not become evidence;
- the raw value `enabled` from `X-Debug-Mode` is not copied into the report;
- both candidates should change 3/3 trials while their paired random-name controls change 0/3.

Why this matters: the `/html` body contains a fixed-width request ID that changes on every request. The `preview` parameter swaps a `<p>` element for a `<b>` element. Those tag names are the same length, so response size remains stable while HTML structure changes. `verbose` leaves the body alone and only adds a response header.

## Acceptance B: random-name negative control rejects shared noise

Run:

```powershell
.\paramintel.exe `
  -request .\labs\v0.7-evidence-fidelity\request-noise.txt `
  -scheme http `
  -locations query `
  -baseline 3 `
  -trials 3 `
  -chunk 8 `
  -characterize=false `
  -value-aware=false `
  -verbose `
  -output .\labs\v0.7-evidence-fidelity\noise-findings.json
```

Expected result:

- zero reported parameters;
- verbose diagnostics show candidates being rejected because the random negative control reproduces the behavior.

The endpoint deliberately changes its response whenever *any* query parameter is present. A scanner that only compares candidate-vs-baseline could mistake every parameter name for a real input. ParamIntel should reject them because the paired random-name control causes the same behavior.

## Acceptance C: existing JSON semantics remain authoritative

Run:

```powershell
.\paramintel.exe `
  -request .\labs\v0.7-evidence-fidelity\request-json.txt `
  -scheme http `
  -locations json `
  -baseline 3 `
  -trials 3 `
  -chunk 8 `
  -characterize=false `
  -value-aware=false `
  -allow-state-changing `
  -verbose `
  -output .\labs\v0.7-evidence-fidelity\json-findings.json
```

Expected result:

- exactly one finding: JSON candidate `role` at `$.role`;
- candidate changes 3/3 trials while its random-name control changes 0/3;
- evidence includes `json_value_changed` at `$.user.role`;
- the rotating `$.request_id` response property is ignored because it is not stable across baseline samples;
- v0.7 line-count, word-count, and HTML-structure evidence do not replace JSON path semantics.

`POST` is used only against this local acceptance server. The `-allow-state-changing` flag is still required so the lab exercises the same safety gate used for authorized real targets.

## Automated coverage

The same behavioral guarantees are exercised in the CLI test suite under `cmd/paramintel` using `httptest` servers. Run the full release gate with:

```powershell
go test ./...
go vet ./...
go test -race ./...
go build ./cmd/paramintel
$env:GOOS='windows'; $env:GOARCH='amd64'; go build ./cmd/paramintel; Remove-Item Env:GOOS, Env:GOARCH
```

The acceptance goal for v0.7 is not merely to detect more response differences. It is to prove that ParamIntel can observe subtle deterministic behavior while continuing to reject environmental or generic unknown-parameter noise.
