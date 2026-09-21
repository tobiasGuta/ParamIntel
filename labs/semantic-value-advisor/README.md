# Semantic Value Advisor acceptance lab

This localhost-only lab validates the first Semantic Value Advisor slice.

The candidate name is `visibility`, but generic random values produce baseline-like behavior and ParamIntel has no built-in semantic profile for that name.

The baseline response exposes the application vocabulary:

```json
{
  "available_visibilities": ["public", "private", "internal"]
}
```

Only this request changes behavior:

```text
GET /projects?visibility=internal
```

That makes the acceptance question precise:

> Can the AI Value Advisor infer an application-specific semantic value while ParamIntel's existing same-value random-name control and repeated verifier remain responsible for the evidence?

## Run the lab

Terminal 1:

```powershell
go run .\labs\semantic-value-advisor
```

The lab listens on `127.0.0.1:41781`.

## Build ParamIntel

```powershell
go build -o paramintel.exe .\cmd\paramintel
```

## Control run: AI Value Advisor disabled

```powershell
.\paramintel.exe `
  -request .\labs\semantic-value-advisor\request.txt `
  -wordlist .\labs\semantic-value-advisor\wordlist.txt `
  -scheme http `
  -baseline 3 `
  -trials 3 `
  -characterize=false `
  -value-aware `
  -value-aware-budget 32 `
  -verbose
```

Expected: no verified `visibility` finding.

## AI run

Make sure `GEMINI_API_KEY` is set in the shell running ParamIntel.

```powershell
.\paramintel.exe `
  -request .\labs\semantic-value-advisor\request.txt `
  -wordlist .\labs\semantic-value-advisor\wordlist.txt `
  -scheme http `
  -baseline 3 `
  -trials 3 `
  -characterize=false `
  -value-aware `
  -value-aware-budget 32 `
  -ai-value-advisor `
  -ai-provider gemini `
  -ai-value-budget 4 `
  -ai-value-candidate-budget 1 `
  -verbose
```

Expected successful path:

- Gemini proposes `internal` as a value hypothesis for `visibility`;
- ParamIntel locally admits the ordinary semantic string;
- the candidate response changes;
- the same-value random-name control does not change;
- repeated verification succeeds;
- the result records `"discovery_mode": "ai_value_aware"`.

The model suggestion itself is never evidence.
