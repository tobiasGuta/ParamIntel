# ParamIntel v0.9 Slice 3 — Schema-Typed Probe Manual Acceptance

This localhost-only lab validates the v0.9 Slice 3 rule that OpenAPI may influence a candidate's JSON scalar **probe type**, while the existing repeated verifier and paired random-name control remain authoritative.

## Acceptance matrix

| Case | Expected result |
| --- | --- |
| strict boolean `beta_access` | schema-typed `true`, candidate 3/3, control 0/3, one finding |
| generic boolean behavior | candidate 3/3, control 3/3, zero findings |
| strict integer `access_level` | schema-typed `1`, candidate 3/3, control 0/3, one finding |
| union `[boolean, null]` | no schema-typed probe; generic string path cannot trigger behavior; zero findings |

The union case has a direct-request sanity check proving the endpoint does react to a real boolean. This distinguishes a deliberate type-admission boundary from an inert fixture.

## 1. Build ParamIntel

From the repository root:

```powershell
cd D:\Tools\ParamIntel
go build -o .\paramintel.exe .\cmd\paramintel
```

Release builds should report `ParamIntel v0.9.0` with `.\paramintel.exe -version`.

## 2. Start the lab

In a second PowerShell terminal:

```powershell
cd D:\Tools\ParamIntel
go run .\labs\v0.9-schema-typed-probes
```

Expected startup:

```text
ParamIntel v0.9 schema-typed acceptance lab listening on http://127.0.0.1:8096
endpoints: POST /boolean-real, POST /boolean-noise, POST /integer-real, POST /union
```

Leave that terminal running.

## 3. Acceptance A — strict boolean field

Run from the repository root:

```powershell
.\paramintel.exe `
  -request .\labs\v0.9-schema-typed-probes\request-boolean-real.txt `
  -openapi .\labs\v0.9-schema-typed-probes\openapi.yaml `
  -scheme http `
  -locations json `
  -allow-state-changing `
  -baseline 3 `
  -trials 3 `
  -chunk 8 `
  -characterize=false `
  -value-aware=false `
  -verbose `
  -output .\labs\v0.9-schema-typed-probes\boolean-real-findings.json
```

Required result:

```text
$.profile.beta_access
  discovery: schema-typed using "true" (boolean)
  candidate: changed 3/3
  control:   changed 0/3
  accepted
```

The output must contain exactly one finding.

Inspect it with:

```powershell
Get-Content .\labs\v0.9-schema-typed-probes\boolean-real-findings.json
```

The finding should contain:

```text
discovery_mode: schema_typed
discovery_value: true
discovery_value_kind: boolean
source: openapi_response_only_json_property
declared_types: [boolean]
```

## 4. Acceptance B — generic boolean behavior must be rejected

```powershell
.\paramintel.exe `
  -request .\labs\v0.9-schema-typed-probes\request-boolean-noise.txt `
  -openapi .\labs\v0.9-schema-typed-probes\openapi.yaml `
  -scheme http `
  -locations json `
  -allow-state-changing `
  -baseline 3 `
  -trials 3 `
  -chunk 8 `
  -characterize=false `
  -value-aware=false `
  -verbose `
  -output .\labs\v0.9-schema-typed-probes\boolean-noise-findings.json
```

Required result for `$.profile.beta_access`:

```text
candidate: changed 3/3
control:   changed 3/3
rejected: random negative control reproduced candidate behavior
```

The output must contain zero findings.

```powershell
Get-Content .\labs\v0.9-schema-typed-probes\boolean-noise-findings.json
```

## 5. Acceptance C — strict integer field

```powershell
.\paramintel.exe `
  -request .\labs\v0.9-schema-typed-probes\request-integer-real.txt `
  -openapi .\labs\v0.9-schema-typed-probes\openapi.yaml `
  -scheme http `
  -locations json `
  -allow-state-changing `
  -baseline 3 `
  -trials 3 `
  -chunk 8 `
  -characterize=false `
  -value-aware=false `
  -verbose `
  -output .\labs\v0.9-schema-typed-probes\integer-real-findings.json
```

Required result:

```text
$.profile.access_level
  discovery: schema-typed using "1" (integer)
  candidate: changed 3/3
  control:   changed 0/3
  accepted
```

The output must contain exactly one finding.

```powershell
Get-Content .\labs\v0.9-schema-typed-probes\integer-real-findings.json
```

## 6. Acceptance D — union declaration must not authorize typed probing

The OpenAPI response schema declares:

```yaml
beta_access:
  type:
    - boolean
    - 'null'
```

Slice 3 deliberately does not choose a typed representative for a union.

Run:

```powershell
.\paramintel.exe `
  -request .\labs\v0.9-schema-typed-probes\request-union.txt `
  -openapi .\labs\v0.9-schema-typed-probes\openapi.yaml `
  -scheme http `
  -locations json `
  -allow-state-changing `
  -baseline 3 `
  -trials 3 `
  -chunk 8 `
  -characterize=false `
  -value-aware=false `
  -verbose `
  -output .\labs\v0.9-schema-typed-probes\union-findings.json
```

Required result: zero findings.

```powershell
Get-Content .\labs\v0.9-schema-typed-probes\union-findings.json
```

### Union sanity check

Prove the endpoint itself reacts to the boolean value that Slice 3 intentionally refuses to infer from a union:

```powershell
$body = '{"profile":{"name":"tobias","beta_access":true}}'

Invoke-RestMethod `
  -Method Post `
  -Uri http://127.0.0.1:8096/union `
  -ContentType 'application/json' `
  -Body $body | ConvertTo-Json -Depth 5
```

Expected response contains:

```json
{
  "profile": {
    "name": "tobias",
    "beta_access": true
  }
}
```

A zero ParamIntel finding together with this direct response proves the union remained on the generic string path by policy rather than because the endpoint was inert.

## Acceptance conclusion

Slice 3 passes manual acceptance only if all four conditions hold:

1. an unambiguous boolean schema declaration enables a typed boolean discovery probe;
2. the paired random-name control receives that same boolean and rejects generic boolean behavior;
3. an unambiguous integer schema declaration enables the integer probe `1`;
4. a multi-type union receives no schema-typed probe and therefore does not bypass the conservative admission boundary.
