package schemaintel

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestOpenAPINullabilitySemanticsMatrix(t *testing.T) {
	tests := []struct {
		name              string
		version           string
		propertySchema    string
		wantLibTypes      []string
		wantNullable      *bool
		wantDeclaredTypes []string
		wantAmbiguous     bool
		wantCandidate     bool
	}{
		{name: "A OAS 3.0 nullable boolean", version: "3.0.3", propertySchema: "type: boolean\nnullable: true", wantLibTypes: []string{"boolean"}, wantNullable: boolPtrForNullabilityTest(true), wantDeclaredTypes: []string{"boolean"}, wantCandidate: true},
		{name: "B OAS 3.1 type union", version: "3.1.0", propertySchema: "type: [boolean, \"null\"]", wantLibTypes: []string{"boolean", "null"}, wantDeclaredTypes: []string{"boolean", "null"}, wantCandidate: true},
		{name: "C OAS 3.1 legacy nullable keyword", version: "3.1.0", propertySchema: "type: boolean\nnullable: true", wantLibTypes: []string{"boolean"}, wantNullable: boolPtrForNullabilityTest(true), wantDeclaredTypes: []string{"boolean"}, wantCandidate: true},
		{name: "D OAS 3.1 anyOf boolean null", version: "3.1.0", propertySchema: "anyOf:\n  - type: boolean\n  - type: \"null\"", wantAmbiguous: true, wantCandidate: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Parse([]byte(nullabilitySpec(tt.version, tt.propertySchema)))
			if err != nil { t.Fatalf("Parse() error = %v", err) }
			if doc.Version() != tt.version { t.Fatalf("version=%q want=%q", doc.Version(), tt.version) }

			libTypes, nullable, err := responsePropertyLibrarySemantics(doc, "enabled")
			if err != nil { t.Fatal(err) }
			if got := normalizedTypes(libTypes); !reflect.DeepEqual(got, tt.wantLibTypes) { t.Fatalf("libopenapi schema.Type=%v want=%v", got, tt.wantLibTypes) }
			if !sameOptionalBool(nullable, tt.wantNullable) { t.Fatalf("libopenapi schema.Nullable=%v want=%v", optionalBoolString(nullable), optionalBoolString(tt.wantNullable)) }

			report, err := Analyze(doc, requestTemplate(http.MethodPost, "https://api.example.test/probe", `{}`), stableJSONBaseline(http.StatusOK, "application/json"), DefaultConfig())
			if err != nil { t.Fatalf("Analyze() error = %v", err) }

			property, ok := propertyByPath(report.ResponseProperties, "$.enabled")
			if !ok { t.Fatalf("response property $.enabled missing: %+v", report.ResponseProperties) }
			if !reflect.DeepEqual(property.DeclaredTypes, tt.wantDeclaredTypes) { t.Fatalf("DeclaredTypes=%v want=%v", property.DeclaredTypes, tt.wantDeclaredTypes) }
			if property.Ambiguous != tt.wantAmbiguous { t.Fatalf("Ambiguous=%t want=%t", property.Ambiguous, tt.wantAmbiguous) }
			if !property.Required { t.Fatal("Required=false want=true") }

			candidatePresent := false
			for _, candidate := range report.Candidates { if candidate.Path == "$.enabled" { candidatePresent = true } }
			if candidatePresent != tt.wantCandidate { t.Fatalf("candidate present=%t want=%t candidates=%+v skipped=%+v", candidatePresent, tt.wantCandidate, report.Candidates, report.Skipped) }

			t.Logf("version=%s libopenapi.Type=%v Nullable=%s DeclaredTypes=%v Ambiguous=%t Required=%t candidate=%t", doc.Version(), normalizedTypes(libTypes), optionalBoolString(nullable), property.DeclaredTypes, property.Ambiguous, property.Required, candidatePresent)
		})
	}
}

func responsePropertyLibrarySemantics(doc *Document, propertyName string) ([]string, *bool, error) {
	op, _, err := matchOperation(doc, http.MethodPost, "/probe"); if err != nil { return nil, nil, err }
	response, _, err := selectResponse(op.Responses, http.StatusOK); if err != nil { return nil, nil, err }
	media, _, err := selectMediaType(response.Content, "application/json"); if err != nil { return nil, nil, err }
	root, err := media.Schema.BuildSchema(); if err != nil { return nil, nil, err }
	if root == nil || root.Properties == nil { return nil, nil, fmt.Errorf("response root schema has no properties") }
	for name, proxy := range root.Properties.FromOldest() {
		if name != propertyName { continue }
		schema, err := proxy.BuildSchema(); if err != nil { return nil, nil, err }
		if schema == nil { return nil, nil, fmt.Errorf("property %s returned nil schema", propertyName) }
		return append([]string(nil), schema.Type...), schema.Nullable, nil
	}
	return nil, nil, fmt.Errorf("response property %s not found", propertyName)
}

func propertyByPath(properties []PropertyDescriptor, path string) (PropertyDescriptor, bool) {
	for _, property := range properties { if property.Path == path { return property, true } }
	return PropertyDescriptor{}, false
}

func nullabilitySpec(version, propertySchema string) string {
	indent := strings.Repeat(" ", 20)
	indented := indent + strings.ReplaceAll(propertySchema, "\n", "\n"+indent)
	return fmt.Sprintf(`openapi: %s
info:
  title: nullability semantics experiment
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
                required: [enabled]
                properties:
                  enabled:
%s
`, version, indented)
}

func boolPtrForNullabilityTest(value bool) *bool { return &value }
func sameOptionalBool(got, want *bool) bool { if got == nil || want == nil { return got == nil && want == nil }; return *got == *want }
func optionalBoolString(value *bool) string { if value == nil { return "<nil>" }; return fmt.Sprintf("%t", *value) }
