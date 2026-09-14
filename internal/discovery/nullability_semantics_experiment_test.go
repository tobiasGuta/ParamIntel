package discovery

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
	"github.com/tobiasGuta/ParamIntel/internal/schemaintel"
)

func TestTypedProbeNullabilityExperiment(t *testing.T) {
	cases := []struct {
		name, version, property string
	}{
		{"A_oas30_nullable", "3.0.3", "type: boolean\n          nullable: true"},
		{"B_oas31_union", "3.1.0", "type: [boolean, \\"null\\"]"},
		{"C_oas31_legacy_nullable", "3.1.0", "type: boolean\n          nullable: true"},
		{"D_oas31_anyof", "3.1.0", "anyOf:\n            - type: boolean\n            - type: \\"null\\""},
	}

	var rows []string
	for _, tc := range cases {
		doc, err := schemaintel.Parse([]byte(typedNullabilitySpec(tc.version, tc.property)))
		if err != nil {
			rows = append(rows, fmt.Sprintf("%s | parse_error=%v", tc.name, err))
			continue
		}
		report, err := schemaintel.Analyze(doc, model.RequestTemplate{
			Method: http.MethodPost,
			URL: "https://api.example.test/check",
			Headers: http.Header{"Content-Type": []string{"application/json"}},
			Body: []byte(`{}`),
		}, model.BaselineProfile{
			Samples: 3, StatusCode: 200, StatusStable: true, IsJSON: true,
			ContentType: "application/json", ContentTypeStable: true,
		}, schemaintel.DefaultConfig())
		if err != nil {
			rows = append(rows, fmt.Sprintf("%s | analyze_error=%v", tc.name, err))
			continue
		}

		candidates := schemaintel.ExistingParentCandidates(report)
		if len(candidates) == 0 {
			rows = append(rows, fmt.Sprintf("%s | candidate=false | typed=false | value=<none>", tc.name))
			continue
		}
		candidate := candidates[0]
		kind, typed := schemaTypedProbeKind(candidate)
		value, valueOK := schemaTypedProbeValue(candidate)
		rows = append(rows, fmt.Sprintf("%s | declared=%v | typed=%t | kind=%s | value_ok=%t | value_kind=%s | value_raw=%s", tc.name, candidate.Sources[0].DeclaredTypes, typed, kind, valueOK, value.Kind, value.Raw))
	}

	t.Fatalf("typed-probe nullability matrix:\n%s", strings.Join(rows, "\n"))
}

func typedNullabilitySpec(version, property string) string {
	return fmt.Sprintf(`openapi: %s
info:
  title: nullability experiment
  version: "1"
paths:
  /check:
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
`, version, strings.ReplaceAll(property, "\n", "\n                    "))
}
