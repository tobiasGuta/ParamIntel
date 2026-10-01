# ParamIntel MCP: external semantic hints

This branch adds an optional local MCP surface for ParamIntel. The goal is to let an MCP-capable agent contribute semantic hypotheses without turning model output into evidence and without requiring ParamIntel to make a second AI-provider request.

The responsibility boundary stays explicit:

```text
ChatGPT / Codex
    |
    | semantic hypotheses
    v
ParamIntel MCP
    |
    v
ParamIntel
    |- deterministic candidate acquisition
    |- mutation and replay
    |- repeated verification
    |- random-name negative controls
    |- confidence and evidence
    v
verified / rejected result
```

External hints are hypotheses only. A model saying that `role=admin` looks plausible does not make it a finding.

## Build

Build the normal CLI and the MCP server from the same checkout:

```bash
go build -o ./bin/paramintel ./cmd/paramintel
go build -o ./bin/paramintel-mcp ./cmd/paramintel-mcp
```

Point the MCP adapter at that CLI binary and choose the directory from which it may read raw request files:

```bash
export PARAMINTEL_MCP_BIN="$PWD/bin/paramintel"
export PARAMINTEL_MCP_REQUEST_ROOT="$PWD"
```

Then configure your MCP client to launch:

```text
/path/to/paramintel-mcp
```

The server uses stdio. It does not open a listening network port.

## MCP tools

### `inspect_request_file`

Reads a raw HTTP request from `PARAMINTEL_MCP_REQUEST_ROOT` and returns a sanitized structural view for agent reasoning.

It returns information such as:

- HTTP method;
- sanitized path;
- active discovery locations;
- query and form key names;
- JSON parent paths;
- JSON shape/type information.

It intentionally does not return raw headers, cookies, authorization values, hostnames, query/form values, JSON primitive values, or raw response text.

A typical agent flow is to inspect first, reason about likely semantics, then call the analysis tool with a small set of hypotheses.

### `analyze_request_file`

Runs the normal ParamIntel CLI with a bounded external-hints document.

The MCP tool does not enable `-ai-advisor` or `-ai-value-advisor`. Therefore this path does not make a ParamIntel AI-provider call by itself. If a future MCP mode intentionally enables an internal provider, that should remain an explicit policy choice.

The current adapter intentionally invokes the existing CLI rather than duplicating the discovery engine. This keeps MCP optional and preserves one verification implementation while the integration is evaluated. A later refactor can expose a reusable Go engine API if the MCP experiment proves worthwhile.

## External hints

The CLI can also consume the same format directly:

```bash
paramintel \
  -request request.txt \
  -hints hints.json
```

Example:

```json
{
  "context": [
    "administrative user creation",
    "identity management"
  ],
  "candidates": [
    {
      "name": "role",
      "location": "json",
      "json_parent": "$",
      "priority": 95,
      "reason": "authorization-related user state",
      "values": [
        {
          "value": "admin",
          "kind": "string",
          "priority": 95,
          "reason": "plausible privileged role"
        },
        {
          "value": "user",
          "kind": "string",
          "priority": 70,
          "reason": "plausible ordinary role"
        }
      ]
    },
    {
      "name": "is_admin",
      "location": "json",
      "json_parent": "$",
      "priority": 90,
      "reason": "plausible privilege flag",
      "values": [
        {
          "value": "true",
          "kind": "boolean",
          "priority": 95
        },
        {
          "value": "false",
          "kind": "boolean",
          "priority": 70
        }
      ]
    }
  ]
}
```

The format is deliberately provider-agnostic. Hints can come from ChatGPT, Codex, another model, a human researcher, Reconductor, or another analysis tool.

## Admission and provenance

External candidate hints pass through the same local admission policy used for the existing AI Candidate Advisor. ParamIntel still rejects invalid names, inactive locations, nonexistent JSON parents, duplicates, candidates already present in the request, and candidates already covered by the deterministic wordlist.

Accepted external candidates record:

```text
source: external_semantic_hint
```

External value hints use the existing typed-value admission rules. They remain bounded, are deduplicated against deterministic values, and cannot bypass the normal verification pipeline.

When an external value is responsible for a verified rescue, the result records:

```text
discovery_mode: external_value_hint
semantic_source: external_hint
```

That keeps external-agent yield distinguishable from deterministic discovery and from ParamIntel's internal AI advisor.

## External-first AI behavior

If a caller supplies external value hints and the internal Semantic Value Advisor is also enabled from the CLI, ParamIntel evaluates the external values first. The internal provider is only used as a fallback when the external source has no admitted values for that candidate.

For the MCP tool in this branch, internal AI flags are not enabled at all. That is intentional: the first experiment is whether an MCP agent can supply useful priors while ParamIntel spends zero separate provider calls.

## Safety boundaries

The MCP adapter keeps several additional controls around live execution:

1. Request-file reads are confined to `PARAMINTEL_MCP_REQUEST_ROOT`. Symlinks that escape the configured root are rejected.
2. Request files are size-bounded before parsing.
3. The tool accepts structured options, not arbitrary CLI arguments, so an agent cannot inject extra ParamIntel flags.
4. POST, PUT, PATCH, DELETE, and other potentially state-changing methods keep the normal ParamIntel gate and add a second MCP gate. The caller must set `allow_state_changing=true`, and the server operator must also set:

```bash
export PARAMINTEL_MCP_ALLOW_STATE_CHANGING=1
```

5. Temporary hint files are written with mode `0600` and removed after the run.
6. The MCP server is stdio-only.

Use live analysis only on systems you are authorized to test and with request methods whose side effects you understand.

## Why keep the standalone AI advisor?

MCP is an optional integration, not a new dependency for ParamIntel's core behavior. The existing provider-backed advisors remain useful when ParamIntel is run by itself.

The intended model is:

```text
standalone ParamIntel
    -> deterministic analysis
    -> optional internal AI advisor
    -> verification

agent-driven ParamIntel
    -> external semantic hints
    -> deterministic analysis
    -> verification
    -> optional internal AI fallback only when explicitly enabled
```

This lets the project compare candidate sources empirically instead of assuming that one model path is better.
