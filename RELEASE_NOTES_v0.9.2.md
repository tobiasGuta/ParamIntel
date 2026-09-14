# ParamIntel v0.9.2 — OpenAPI Nullability Consistency

ParamIntel v0.9.2 is a focused correctness release for OpenAPI schema-typed probing.

## Fix

ParamIntel v0.9 introduced a narrow schema-typed shortcut for response-only JSON candidates whose OpenAPI schema declared exactly one supported scalar type:

```text
boolean -> true
integer -> 1
```

`libopenapi` exposes legacy OpenAPI `nullable` metadata separately from `schema.Type`. ParamIntel previously preserved only the declared type list. This meant an OpenAPI 3.0 schema such as:

```yaml
type: boolean
nullable: true
```

could appear to ParamIntel as a single unambiguous boolean and receive the typed `true` shortcut, while the semantically equivalent OpenAPI 3.1 representation:

```yaml
type: [boolean, "null"]
```

correctly remained multi-type and did not receive a typed shortcut.

v0.9.2 preserves `nullable` through schema descriptors and candidate provenance, and requires schema-typed boolean/integer shortcuts to be non-nullable.

## Regression matrix

The release covers four representations:

```text
A. OAS 3.0.3
   type: boolean
   nullable: true
   -> nullable provenance preserved; no schema-typed shortcut

B. OAS 3.1.0
   type: [boolean, "null"]
   -> existing multi-type rejection preserved

C. OAS 3.1.0
   type: boolean
   nullable: true
   -> nullable provenance preserved; no schema-typed shortcut

D. OAS 3.1.0
   anyOf:
     - type: boolean
     - type: "null"
   -> existing ambiguity withholding preserved
```

Non-nullable `boolean` and `integer` declarations continue to use the existing `true` and `1` typed probes.

## Evidence boundary

This patch does **not**:

- add `null` as a new probe value;
- reinterpret malformed or mixed-version OpenAPI documents;
- expand OpenAPI candidate authority;
- change candidate/control verification;
- change confidence scoring or evidence rules;
- activate OpenAPI-derived scaffolding.

Schema metadata remains hypothesis/probe-selection input only. Findings still require live application behavior, repeated verification, paired random-name controls, and the normal ParamIntel evidence model.

## Origin

The issue was surfaced by ParamIntel Watch from a real-world OpenAPI 3.0/3.1 compatibility pattern, then confirmed with an isolated A–D experiment before any production behavior changed.
