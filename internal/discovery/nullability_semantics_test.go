package discovery

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
	"github.com/tobiasGuta/ParamIntel/internal/schemaintel"
)

func TestOpenAPINullabilityTypedProbeMatrix(t *testing.T) {
	tests := []struct {
		name           string
		version        string
		propertySchema string
		wantCandidate  bool
		wantProbe      bool
		wantKind       string
		wantRaw        string
	}{
		{
			name:           "A OAS 3.0 nullable boolean",
			version:        "3.0.3",
			propertySchema: "type: boolean\nnullable: true",
			wantCandidate:  true,
			wantProbe:      true,
			wantKind:       "boolean",
			wantRaw:        "true",
		},
		{
			name:           "B OAS 3.1 type union",
			version:        "3.1.0",
			propertySchema: "type: [boolean, \"null\"]",
			wantCandidate:  true,
			wantProbe:      false,
		},
		{
			name:           "C OAS 3.1 legacy nullable keyword",
			version:        "3.1.0",
			propertySchema: "type: boolean\nnullable: true",
			wantCandidate:  true,
			wantProbe:      true,
			wantKind:       "boolean",
			wantRaw:        "true",
		},
		{
			name:           "D OAS 3.1 anyOf boolean null",
			version:        "3.1.0",
			propertySchema: "anyOf:\n  - type: boolean\n  - type: \"null\"",
			wantCandidate:  false,
			wantProbe:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := schemaintel.Parse([]byte(discoveryNullabilitySpec(tt.version, tt.propertySchema)))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			report, err := schemaintel.Analyze(
				doc,
				model.RequestTemplate{
					Method: http.MethodPost,
					URL:    "https://api.example.test/probe",
					Headers: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: []byte(`{}`),
				},
				model.BaselineProfile{
					Samples:           3,
					StatusCode:        http.StatusOK,
					StatusStable:      true,
					IsJSON:            true,
					ContentType:       "application/json",
					ContentTypeStable: true,
				},
				schemaintel.DefaultConfig(),
			)
			if err != nil {
				t.Fatalf("Analyze() error = %v", err)
			}

			candidates := schemaintel.ExistingParentCandidates(report)
			if got := len(candidates) > 0; got != tt.wantCandidate {
				t.Fatalf("candidate present=%t want=%t candidates=%+v skipped=%+v", got, tt.wantCandidate, candidates, report.Skipped)
			}
			if !tt.wantCandidate {
				t.Logf("version=%s candidate=false typedProbe=false skipped=%+v", doc.Version(), report.Skipped)
				return
			}
			if len(candidates) != 1 {
				t.Fatalf("candidates=%d want=1: %+v", len(candidates), candidates)
			}

			kind, kindOK := schemaTypedProbeKind(candidates[0])
			value, valueOK := schemaTypedProbeValue(candidates[0])
			if kindOK != tt.wantProbe || valueOK != tt.wantProbe {
				t.Fatalf("typed probe kindOK=%t valueOK=%t want=%t candidate=%+v", kindOK, valueOK, tt.wantProbe, candidates[0])
			}
			if tt.wantProbe {
				if kind != tt.wantKind || value.Kind != tt.wantKind || value.Raw != tt.wantRaw {
					t.Fatalf("kind=%q value=%+v want kind=%q raw=%q", kind, value, tt.wantKind, tt.wantRaw)
				}
			} else if kind != "" || value.Kind != "" || value.Raw != "" {
				t.Fatalf("unexpected rejected probe kind=%q value=%+v", kind, value)
			}

			t.Logf("version=%s DeclaredTypes=%v typedProbe=%t kind=%q value=%q",
				doc.Version(), candidates[0].Sources[0].DeclaredTypes, tt.wantProbe, kind, value.Raw)
		})
	}
}

func discoveryNullabilitySpec(version, propertySchema string) string {
	indented := "            " + strings.ReplaceAll(propertySchema, "\n", "\n            ")
	return fmt.Sprintf(`openapi: %s
info:
  title: nullability typed-probe experiment
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
