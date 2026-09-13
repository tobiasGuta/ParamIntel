# ParamIntel v0.8 Controlled JSON Scaffolding Acceptance Lab

This localhost-only lab proves the first v0.8 deeper-structured-JSON behavior without broad JSON tree invention.

The captured request contains only:

```json
{"profile":{"name":"tobias"}}
```

The related context response additionally exposes:

```json
{
  "profile": {
    "name": "tobias",
    "settings": {
      "beta_access": false
    }
  }
}
```

`$.profile.settings.beta_access` is therefore response-derived, but its immediate parent `$.profile.settings` is absent from the request. With `-json-scaffold`, ParamIntel may create exactly that one missing object level for this candidate and its paired random-name control.

## Start the lab

From the repository root in PowerShell:

```powershell
go run .\labs\v0.8-json-scaffolding
```

The lab listens on `http://127.0.0.1:8094`.

Build ParamIntel in another terminal:

```powershell
go build -o .\paramintel.exe .\cmd\paramintel
```

## Acceptance A: candidate-specific nested field

Run:

```powershell
.\paramintel.exe `
  -request .\labs\v0.8-json-scaffolding\request-real.txt `
  -scheme http `
  -locations json `
  -context-response .\labs\v0.8-json-scaffolding\context-response.json `
  -json-scaffold `
  -allow-state-changing `
  -baseline 3 `
  -trials 3 `
  -chunk 8 `
  -characterize=false `
  -value-aware=false `
  -verbose `
  -output .\labs\v0.8-json-scaffolding\real-findings.json
```

Expected result:

- exactly one finding: `$.profile.settings.beta_access`;
- the candidate changes 3/3 trials;
- the paired random-name control changes 0/3;
- the candidate source is `context_response_scaffoldable_json_property`;
- the missing `settings` object is created only because `-json-scaffold` is explicitly enabled.

The candidate request is structurally equivalent to:

```json
{"profile":{"name":"tobias","settings":{"beta_access":"<probe>"}}}
```

The paired control keeps the identical scaffold and value, changing only the leaf name:

```json
{"profile":{"name":"tobias","settings":{"zz_pi_<random>":"<probe>"}}}
```

## Acceptance B: shared parent behavior must be rejected

Run:

```powershell
.\paramintel.exe `
  -request .\labs\v0.8-json-scaffolding\request-noise.txt `
  -scheme http `
  -locations json `
  -context-response .\labs\v0.8-json-scaffolding\context-response.json `
  -json-scaffold `
  -allow-state-changing `
  -baseline 3 `
  -trials 3 `
  -chunk 8 `
  -characterize=false `
  -value-aware=false `
  -verbose `
  -output .\labs\v0.8-json-scaffolding\noise-findings.json
```

Expected result:

- zero reported parameters;
- verbose output shows the scaffold candidate changing;
- the paired random-name control reproduces that behavior and suppresses the apparent finding.

`/noise` deliberately reacts whenever *any* child exists inside the newly-created `settings` object. A scanner that compared only candidate-vs-baseline would report a false positive. ParamIntel must reject it because the random-name leaf causes the same behavior under the same scaffold.

## Acceptance C: flag-off boundary

Run the first command again **without** `-json-scaffold`.

Expected result:

- `$.profile.settings.beta_access` is not actively tested;
- no missing object is synthesized;
- no scaffold finding is reported.

Also verify that using `-json-scaffold` without `-context-response` fails before target probing:

```powershell
.\paramintel.exe `
  -request .\labs\v0.8-json-scaffolding\request-real.txt `
  -scheme http `
  -locations json `
  -json-scaffold `
  -allow-state-changing
```

Expected diagnostic:

```text
-json-scaffold requires -context-response
```

## Structural safety boundaries

Automated tests additionally prove that controlled scaffolding:

- creates exactly one missing object level;
- never replaces an existing object, `null`, scalar, or array;
- rejects paths requiring two missing object levels;
- requires scaffold metadata to match the candidate's JSON parent;
- isolates scaffold candidates from bulk discovery groups;
- preserves the same scaffold and probe value for the random-name control;
- keeps existing JSON discovery behavior unchanged when scaffolding is disabled.

## Full release gate

```powershell
go test ./...
go vet ./...
go test -race ./...
go build ./cmd/paramintel
$env:GOOS='windows'; $env:GOARCH='amd64'; go build ./cmd/paramintel; Remove-Item Env:GOOS, Env:GOARCH
```

The v0.8 acceptance goal is not simply to create missing JSON objects. It is to prove that ParamIntel can test a narrowly response-derived nested hypothesis while preserving the same deterministic candidate/control discipline used by the rest of the scanner.
