package schemaintel

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestOpenAPINullabilitySemanticsExperiment(t *testing.T) {
	cases := []struct {
		name    string
		version string
		schema  string
	}{
		{name: "A_oas30_nullable", version: "3.0.3", schema: "type: boolean\n          nullable: true"},
		{name: "B_oas31_type_union", version: "3.1.0", schema: "type: [boolean, \\"null\\"]"},
		{name: "C_oas31_legacy_nullable", version: "3.1.0", schema: "type: boolean\n          nullable: true"},
		{name: "D_oas31_anyof_null", version: "3.1.0", schema: "anyOf:\n            - type: boolean\n            - type: \\"null\\""},
	}

	var rows []string
	for _, tc := range cases {
		spec := nullabilityExperimentSpec(tc.version, tc.schema)
		doc, err := Parse([]byte(spec))
		if err != nil {
			rows = append(rows, fmt.Sprintf("%s | parse_error=%v", tc.name, err))
			continue
		}

		item, ok := doc.model.Model.Paths.PathItems.Get("/probe")
		if !ok || item == nil || item.Post == nil {
			rows = append(rows, fmt.Sprintf("%s | navigation_error=missing POST /probe", tc.name))
			continue
		}
		resp, ok := item.Post.Responses.Codes.Get("200")
		if !ok || resp == nil {
			rows = append(rows, fmt.Sprintf("%s | navigation_error=missing 200 response", tc.name))
			continue
		}
		media, ok := resp.Content.Get("application/json")
		if !ok || media == nil || media.Schema == nil {
			rows = append(rows, fmt.Sprintf("%s | navigation_error=missing response schema", tc.name))
			continue
		}
		root, err := media.Schema.BuildSchema()
		if err != nil || root == nil || root.Properties == nil {
			rows = append(rows, fmt.Sprintf("%s | schema_error=%v", tc.name, err))
			continue
		}
		enabledProxy, ok := root.Properties.Get("enabled")
		if !ok || enabledProxy == nil {
			rows = append(rows, fmt.Sprintf("%s | navigation_error=missing enabled property", tc.name))
			continue
		}
		enabled, err := enabledProxy.BuildSchema()
		if err != nil || enabled == nil {
			rows = append(rows, fmt.Sprintf("%s | property_error=%v", tc.name, err))
			continue
		}

		report, analyzeErr := Analyze(
			doc,
			requestTemplate(http.MethodPost, "https://api.example.test/probe", `{}`),
			stableJSONBaseline(200, "application/json"),
			DefaultConfig(),
		)
		declared := []string(nil)
		ambiguous := false
		required := false
		for _, prop := range report.ResponseProperties {
			if prop.Path == "$.enabled" {
				declared = prop.DeclaredTypes
				ambiguous = prop.Ambiguous
				required = prop.Required
				break
			}
		}
		nullable := "<nil>"
		if enabled.Nullable != nil {
			nullable = fmt.Sprintf("%t", *enabled.Nullable)
		}
		rows = append(rows, fmt.Sprintf(
			"%s | version=%s | lib_type=%v | nullable=%s | oneOf=%d | anyOf=%d | declared=%v | ambiguous=%t | required=%t | candidates=%d | analyze_error=%v",
			tc.name, doc.Version(), enabled.Type, nullable, len(enabled.OneOf), len(enabled.AnyOf), declared, ambiguous, required, len(report.Candidates), analyzeErr,
		))
	}

	t.Fatalf("OpenAPI nullability experiment matrix:\n%s", strings.Join(rows, "\n"))
}

func nullabilityExperimentSpec(version, propertySchema string) string {
	return fmt.Sprintf(`openapi: %s
info:
  title: nullability experiment
  version: "1"
paths:
  /probe:
    post:
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  enabled:
                    %s
`, version, strings.ReplaceAll(propertySchema, "\n", "\n                    "))
}
