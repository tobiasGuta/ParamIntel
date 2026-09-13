# ParamIntel v0.9 OpenAPI Intelligence Manual Acceptance Lab

This lab manually validates the active v0.9 Slice 2 OpenAPI candidate bridge without expanding its authority.

It proves three cases:

1. a response-schema-only JSON property under an existing request object can become a normal ParamIntel candidate and be verified;
2. generic behavior caused by any unknown sibling is rejected when the paired random-name control reproduces it;
3. a response-schema-only property behind one missing JSON object is recognized by schema intelligence but remains inactive in Slice 2.

The lab listens only on `127.0.0.1:8095`.

## 1. Start the lab server

From the repository root in one PowerShell terminal:

```powershell
go run .\labs\v0.9-openapi-intelligence
```

Expected startup output includes:

```text
ParamIntel v0.9 OpenAPI acceptance lab listening on http://127.0.0.1:8095
endpoints: POST /real, POST /noise, POST /scaffold
```

Keep this terminal running.

## 2. Build ParamIntel

In a second PowerShell terminal:

```powershell
go build -o .\paramintel.exe .\cmd\paramintel
```

Release builds should report `ParamIntel v0.9.0` with `./paramintel.exe -version`.

## 3. Acceptance A — real existing-parent candidate

```powershell
.\paramintel.exe `
  -request .\labs\v0.9-openapi-intelligence\request-real.txt `
  -openapi .\labs\v0.9-openapi-intelligence\openapi.yaml `
  -scheme http `
  -locations json `
  -allow-state-changing `
  -baseline 3 `
  -trials 3 `
  -chunk 8 `
  -characterize=false `
  -value-aware=false `
  -verbose `
  -output .\labs\v0.9-openapi-intelligence\real-findings.json
```

Required result:

- `$.profile.beta_access` is tested as an OpenAPI-derived candidate;
- provenance source is `openapi_response_only_json_property`;
- the schema-declared boolean/read-only metadata is preserved in provenance;
- candidate behavior changes `3/3`;
- the paired random-name control changes `0/3`;
- exactly one parameter is reported.

The OpenAPI metadata itself must not increase confidence or create evidence. The normal ParamIntel verifier remains authoritative.

## 4. Acceptance B — generic sibling noise must be rejected

```powershell
.\paramintel.exe `
  -request .\labs\v0.9-openapi-intelligence\request-noise.txt `
  -openapi .\labs\v0.9-openapi-intelligence\openapi.yaml `
  -scheme http `
  -locations json `
  -allow-state-changing `
  -baseline 3 `
  -trials 3 `
  -chunk 8 `
  -characterize=false `
  -value-aware=false `
  -verbose `
  -output .\labs\v0.9-openapi-intelligence\noise-findings.json
```

Required result:

- `$.profile.beta_access` may change behavior against baseline;
- the paired random-name sibling reproduces the same behavior;
- candidate and control should both change `3/3` in the controlled fixture;
- the candidate is rejected;
- the output contains `0 parameters`.

This proves OpenAPI candidate selection does not bypass the negative-control invariant.

## 5. Acceptance C — OpenAPI scaffold descriptor remains withheld

```powershell
.\paramintel.exe `
  -request .\labs\v0.9-openapi-intelligence\request-scaffold.txt `
  -openapi .\labs\v0.9-openapi-intelligence\openapi.yaml `
  -scheme http `
  -locations json `
  -allow-state-changing `
  -baseline 3 `
  -trials 3 `
  -chunk 8 `
  -characterize=false `
  -value-aware=false `
  -verbose `
  -output .\labs\v0.9-openapi-intelligence\scaffold-findings.json
```

The applicable response schema contains:

```text
$.profile.settings.beta_access
```

while the captured request contains `$.profile` but not `$.profile.settings`.

Required result:

- schema intelligence classifies the nested property as requiring one missing object level;
- Slice 2 does not admit it as an active candidate;
- ParamIntel does not synthesize `$.profile.settings` from OpenAPI provenance;
- the output contains `0 parameters`.

### Optional lab sanity check

To prove the endpoint really would react if the nested field were sent manually:

```powershell
$body = '{"profile":{"name":"tobias","settings":{"beta_access":true}}}'
Invoke-RestMethod `
  -Method Post `
  -Uri http://127.0.0.1:8095/scaffold `
  -ContentType 'application/json' `
  -Body $body | ConvertTo-Json -Depth 5
```

The response should show:

```json
{
  "profile": {
    "name": "tobias",
    "settings": {
      "beta_access": true
    }
  }
}
```

That confirms a zero-finding ParamIntel result is caused by the Slice 2 admission boundary, not by an inert endpoint.

## Release interpretation

All three manual cases must pass before expanding OpenAPI authority in a later slice.

A passing Slice 2 acceptance means:

```text
OpenAPI may nominate an existing-parent JSON field
                    ↓
          normal ParamIntel candidate
                    ↓
       repeated candidate verification
                    ↓
        paired random-name control
                    ↓
       finding only from live behavior
```

OpenAPI-derived one-level scaffolding remains a separate future decision.
