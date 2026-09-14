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
		wantDeclaredTypes []string
		wantNullable      bool
		wantAmbiguous     bool
		wantCandidate     bool
	}{
		{name: "A OAS 3.0 nullable boolean", version: "3.0.3", propertySchema: "type: boolean\nnullable: true", wantDeclaredTypes: []string{"boolean"}, wantNullable: true, wantCandidate: true},
		{name: "B OAS 3.1 type union", version: "3.1.0", propertySchema: "type: [boolean, \"null\"]", wantDeclaredTypes: []string{"boolean", "null"}, wantCandidate: true},
		{name: "C OAS 3.1 legacy nullable keyword", version: "3.1.0", propertySchema: "type: boolean\nnullable: true", wantDeclaredTypes: []string{"boolean"}, wantNullable: true, wantCandidate: true},
		{name: "D OAS 3.1 anyOf boolean null", version: "3.1.0", propertySchema: "anyOf:\n  - type: boolean\n  - type: \"null\"", wantDeclaredTypes: []string{}, wantAmbiguous: true, wantCandidate: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Parse([]byte(nullabilitySpec(tt.version, tt.propertySchema)))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			report, err := Analyze(
				doc,
				requestTemplate(http.MethodPost, "https://api.example.test/probe", `{}`),
				stableJSONBaseline(http.StatusOK, "application/json"),
				DefaultConfig(),
			)
			if err != nil {
				t.Fatalf("Analyze() error = %v", err)
			}

			property, ok := propertyByPath(report.ResponseProperties, "$.enabled")
			if !ok {
				t.Fatalf("response property $.enabled missing: %+v", report.ResponseProperties)
			}
			if !reflect.DeepEqual(property.DeclaredTypes, tt.wantDeclaredTypes) {
				t.Fatalf("DeclaredTypes=%v want=%v", property.DeclaredTypes, tt.wantDeclaredTypes)
			}
			if property.Nullable != tt.wantNullable {
				t.Fatalf("Nullable=%t want=%t", property.Nullable, tt.wantNullable)
			}
			if property.Ambiguous != tt.wantAmbiguous {
				t.Fatalf("Ambiguous=%t want=%t", property.Ambiguous, tt.wantAmbiguous)
			}
			if !property.Required {
				t.Fatal("Required=false want=true")
			}

			candidatePresent := false
			for _, candidate := range report.Candidates {
				if candidate.Path != "$.enabled" {
					continue
				}
				candidatePresent = true
				if candidate.Nullable != tt.wantNullable {
					t.Fatalf("candidate Nullable=%t want=%t", candidate.Nullable, tt.wantNullable)
				}
			}
			if candidatePresent != tt.wantCandidate {
				t.Fatalf("candidate present=%t want=%t candidates=%+v skipped=%+v", candidatePresent, tt.wantCandidate, report.Candidates, report.Skipped)
			}

			active := ExistingParentCandidates(report)
			if tt.wantCandidate {
				if len(active) != 1 {
					t.Fatalf("active candidates=%d want=1: %+v", len(active), active)
				}
				if len(active[0].Sources) != 1 || active[0].Sources[0].Nullable != tt.wantNullable {
					t.Fatalf("source nullable provenance=%+v want=%t", active[0].Sources, tt.wantNullable)
				}
			} else if len(active) != 0 {
				t.Fatalf("active candidates=%+v want none", active)
			}
		})
	}
}

func propertyByPath(properties []PropertyDescriptor, path string) (PropertyDescriptor, bool) {
	for _, property := range properties {
		if property.Path == path {
			return property, true
		}
	}
	return PropertyDescriptor{}, false
}

func nullabilitySpec(version, propertySchema string) string {
	indent := strings.Repeat(" ", 20)
	indented := indent + strings.ReplaceAll(propertySchema, "\n", "\n"+indent)
	return fmt.Sprintf(`openapi: %s
info:
  title: nullability semantics regression
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
