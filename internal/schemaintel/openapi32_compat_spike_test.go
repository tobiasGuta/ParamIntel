package schemaintel

// This is a passive compatibility experiment. It records parser exposure and
// ParamIntel's *current* admission decisions without enabling new probe modes.
import (
    "encoding/json"
    "fmt"
    "net/http"
    "reflect"
    "sort"
    "testing"

    "github.com/tobiasGuta/ParamIntel/internal/model"
)

type compatibilityObservation struct {
    Fixture string `json:"fixture"`
    Version string `json:"version,omitempty"`
    ParseError string `json:"parse_error,omitempty"`
    AnalyzeError string `json:"analyze_error,omitempty"`
    ParameterLocations []string `json:"parameter_locations,omitempty"`
    QuerystringProperties []string `json:"querystring_properties,omitempty"`
    ItemSchemaFieldPresent bool `json:"item_schema_field_present,omitempty"`
    ItemSchemaNonNil bool `json:"item_schema_non_nil,omitempty"`
    ItemEncodingFieldPresent bool `json:"item_encoding_field_present,omitempty"`
    ItemEncodingNonNil bool `json:"item_encoding_non_nil,omitempty"`
    CandidatePaths []string `json:"candidate_paths,omitempty"`
    ActivePaths []string `json:"active_paths,omitempty"`
    Skips []string `json:"skips,omitempty"`
}

func TestOpenAPI32CompatibilitySpike(t *testing.T) {
    cases := []struct {
        name, spec, method, path, body, contentType string
        external bool
    }{
        {"oas30_json_ref", fmt.Sprintf(compatJSONSpec, "3.0.3"), http.MethodPost, "/profile", "{}", "application/json", false},
        {"oas31_json_ref", fmt.Sprintf(compatJSONSpec, "3.1.0"), http.MethodPost, "/profile", "{}", "application/json", false},
        {"oas32_json_ref", fmt.Sprintf(compatJSONSpec, "3.2.1"), http.MethodPost, "/profile", "{}", "application/json", false},
        {"oas32_querystring", compatQuerystringSpec, http.MethodGet, "/search", "", "", false},
        {"oas32_mixed_query_invalid", compatMixedQuerySpec, http.MethodGet, "/search", "", "", false},
        {"oas32_multipart_item_schema", compatMultipartSpec, http.MethodPost, "/upload", "", "multipart/form-data; boundary=example", false},
        {"oas32_union_array", compatUnionSpec, http.MethodPost, "/profile", "{}", "application/json", false},
        {"oas32_external_ref_disabled", compatExternalRefSpec, http.MethodPost, "/profile", "{}", "application/json", true},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            out := compatibilityObservation{Fixture: tc.name}
            defer func() {
                data, err := json.Marshal(out)
                if err != nil { t.Fatal(err) }
                t.Logf("SPIKE_JSON %s", data)
            }()
            doc, err := Parse([]byte(tc.spec))
            if err != nil {
                out.ParseError = err.Error()
                if tc.name == "oas30_json_ref" || tc.name == "oas31_json_ref" { t.Errorf("previously supported fixture failed: %v", err) }
                return
            }
            out.Version = doc.Version()
            if tc.external { t.Error("external ref was accepted with file and remote references disabled") }

            op, _, err := matchOperation(doc, tc.method, tc.path)
            if err != nil {
                out.AnalyzeError = err.Error()
                t.Errorf("operation selection: %v", err)
                return
            }
            for _, p := range op.Parameters {
                if p == nil { continue }
                out.ParameterLocations = append(out.ParameterLocations, p.Name+":"+p.In)
                if p.In != "querystring" || p.Content == nil { continue }
                media := p.Content.GetOrZero("application/x-www-form-urlencoded")
                if media == nil || media.Schema == nil { continue }
                props, skips, walkErr := flattenSchema(media.Schema, DefaultConfig())
                if walkErr != nil {
                    out.Skips = append(out.Skips, "querystring schema: "+walkErr.Error())
                    continue
                }
                for path := range props { out.QuerystringProperties = append(out.QuerystringProperties, path) }
                for _, skipped := range skips { out.Skips = append(out.Skips, skipped.Path+": "+skipped.Reason) }
            }
            sort.Strings(out.ParameterLocations)
            sort.Strings(out.QuerystringProperties)

            if op.RequestBody != nil && op.RequestBody.Content != nil {
                media := op.RequestBody.Content.GetOrZero("multipart/form-data")
                if media != nil {
                    value := reflect.ValueOf(media).Elem()
                    if field := value.FieldByName("ItemSchema"); field.IsValid() {
                        out.ItemSchemaFieldPresent = true
                        if field.Kind() == reflect.Pointer { out.ItemSchemaNonNil = !field.IsNil() }
                    }
                    if field := value.FieldByName("ItemEncoding"); field.IsValid() {
                        out.ItemEncodingFieldPresent = true
                        if field.Kind() == reflect.Pointer { out.ItemEncodingNonNil = !field.IsNil() }
                    }
                }
            }
            tmpl := model.RequestTemplate{
                Method: tc.method,
                URL: "https://api.example.test"+tc.path,
                Headers: http.Header{},
                Body: []byte(tc.body),
            }
            if tc.contentType != "" { tmpl.Headers.Set("Content-Type", tc.contentType) }
            report, err := Analyze(doc, tmpl, stableJSONBaseline(200, "application/json"), DefaultConfig())
            if err != nil {
                out.AnalyzeError = err.Error()
                if tc.name == "oas30_json_ref" || tc.name == "oas31_json_ref" { t.Errorf("supported Analyze failed: %v", err) }
                return
            }
            for _, candidate := range report.Candidates { out.CandidatePaths = append(out.CandidatePaths, candidate.Path) }
            for _, candidate := range ExistingParentCandidates(report) {
                out.ActivePaths = append(out.ActivePaths, candidate.JSONParent+"."+candidate.Name)
            }
            for _, skipped := range report.Skipped { out.Skips = append(out.Skips, skipped.Path+": "+skipped.Reason) }
            sort.Strings(out.CandidatePaths)
            sort.Strings(out.ActivePaths)
            sort.Strings(out.Skips)
            if tc.name == "oas30_json_ref" || tc.name == "oas31_json_ref" {
                if len(out.ActivePaths) != 1 || out.ActivePaths[0] != "$.beta_access" {
                    t.Errorf("known JSON candidate contract changed: %+v", out)
                }
            }
            if tc.name == "oas32_querystring" || tc.name == "oas32_mixed_query_invalid" || tc.name == "oas32_multipart_item_schema" {
                if len(out.ActivePaths) != 0 { t.Errorf("unsupported transport generated active JSON candidates: %+v", out) }
            }
        })
    }
}

// An internal ref is used on both sides. This fixture tests the existing
// response-only JSON candidate contract, not new 3.2 feature support.
const compatJSONSpec = `openapi: %s
info:
  title: compatibility baseline
  version: "1"
paths:
  /profile:
    post:
      requestBody:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Update'
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/Update'
                  - type: object
                    properties:
                      beta_access:
                        type: boolean
                        readOnly: true
components:
  schemas:
    Update:
      type: object
      properties:
        name:
          type: string
`

const compatQuerystringSpec = `openapi: 3.2.1
info:
  title: querystring compatibility
  version: "1"
paths:
  /search:
    get:
      parameters:
        - name: search
          in: querystring
          content:
            application/x-www-form-urlencoded:
              schema:
                type: object
                properties:
                  term: {type: string}
                  limit: {type: integer}
                  tags:
                    type: array
                    items: {type: string}
                  filter:
                    oneOf:
                      - type: string
                      - type: integer
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  admin_filter: {type: boolean}
`

// This combination is disallowed by OAS 3.2.1. Parsing success alone must
// not be confused with semantic validation.
const compatMixedQuerySpec = `openapi: 3.2.1
info:
  title: conflicting query forms
  version: "1"
paths:
  /search:
    get:
      parameters:
        - name: search
          in: querystring
          content:
            application/x-www-form-urlencoded:
              schema: {type: object}
        - name: page
          in: query
          schema: {type: integer}
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {type: object}
`

const compatMultipartSpec = `openapi: 3.2.1
info:
  title: item schema visibility
  version: "1"
paths:
  /upload:
    post:
      requestBody:
        content:
          multipart/form-data:
            itemSchema:
              type: object
              properties:
                enabled: {type: boolean}
            itemEncoding:
              enabled:
                contentType: application/json
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {type: object}
`

const compatUnionSpec = `openapi: 3.2.1
info:
  title: union and array boundary
  version: "1"
paths:
  /profile:
    post:
      requestBody:
        content:
          application/json:
            schema: {type: object}
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  safe: {type: boolean}
                  alternative:
                    anyOf:
                      - type: string
                      - type: integer
                  entries:
                    type: array
                    items:
                      type: object
                      properties:
                        secret: {type: boolean}
`

const compatExternalRefSpec = `openapi: 3.2.1
info:
  title: local-only reference boundary
  version: "1"
paths:
  /profile:
    post:
      requestBody:
        content:
          application/json:
            schema:
              $ref: './external.yaml#/components/schemas/Update'
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {type: object}
`
