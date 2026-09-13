package schemaintel

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func stableJSONBaseline(status int, contentType string) model.BaselineProfile {
	return model.BaselineProfile{
		Samples:           3,
		StatusCode:        status,
		StatusStable:      true,
		IsJSON:            true,
		ContentType:       contentType,
		ContentTypeStable: true,
	}
}

func requestTemplate(method, target, body string) model.RequestTemplate {
	return model.RequestTemplate{
		Method: method,
		URL:    target,
		Headers: http.Header{
			"Content-Type": []string{"application/json; charset=utf-8"},
		},
		Body: []byte(body),
	}
}

func TestAnalyzeDerivesExistingAndScaffoldableResponseOnlyCandidates(t *testing.T) {
	doc, err := Parse([]byte(primarySpec))
	if err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(
		doc,
		requestTemplate(http.MethodPatch, "https://api.example.test/users/123", `{"display_name":"Tobias","profile":{"name":"Tobias"}}`),
		stableJSONBaseline(200, "application/json"),
		DefaultConfig(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.OpenAPIVersion != "3.1.0" {
		t.Fatalf("version=%q", report.OpenAPIVersion)
	}
	if report.Operation.SpecPath != "/users/{id}" || report.Operation.Method != http.MethodPatch {
		t.Fatalf("operation=%+v", report.Operation)
	}
	if report.ResponseStatusKey != "200" {
		t.Fatalf("response status key=%q", report.ResponseStatusKey)
	}
	if len(report.Candidates) != 2 {
		t.Fatalf("candidates=%+v", report.Candidates)
	}

	byPath := make(map[string]CandidateDescriptor)
	for _, candidate := range report.Candidates {
		byPath[candidate.Path] = candidate
		if candidate.Source != SourceResponseOnlyJSONProperty {
			t.Fatalf("candidate %s source=%q", candidate.Path, candidate.Source)
		}
	}

	role, ok := byPath["$.role"]
	if !ok {
		t.Fatalf("role candidate missing: %+v", report.Candidates)
	}
	if role.Placement != PlacementExistingParent || role.JSONScaffoldParent != "" {
		t.Fatalf("role placement=%+v", role)
	}
	if !role.ReadOnly || !role.Required || len(role.DeclaredTypes) != 1 || role.DeclaredTypes[0] != "string" {
		t.Fatalf("role metadata=%+v", role)
	}

	beta, ok := byPath["$.profile.settings.beta_access"]
	if !ok {
		t.Fatalf("beta candidate missing: %+v", report.Candidates)
	}
	if beta.Placement != PlacementOneLevelScaffold || beta.JSONScaffoldParent != "$.profile.settings" {
		t.Fatalf("beta placement=%+v", beta)
	}
	if !beta.ReadOnly || len(beta.DeclaredTypes) != 1 || beta.DeclaredTypes[0] != "boolean" {
		t.Fatalf("beta metadata=%+v", beta)
	}
}

func TestAnalyzeExactConcretePathWinsOverTemplate(t *testing.T) {
	doc, err := Parse([]byte(primarySpec))
	if err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(
		doc,
		requestTemplate(http.MethodPatch, "https://api.example.test/users/me", `{"display_name":"Tobias"}`),
		stableJSONBaseline(200, "application/json"),
		DefaultConfig(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.Operation.SpecPath != "/users/me" {
		t.Fatalf("spec path=%q want concrete /users/me", report.Operation.SpecPath)
	}
	if len(report.Candidates) != 1 || report.Candidates[0].Path != "$.self_only" {
		t.Fatalf("candidates=%+v", report.Candidates)
	}
}

func TestAnalyzeRejectsAmbiguousTemplateOperation(t *testing.T) {
	doc, err := Parse([]byte(ambiguousSpec))
	if err != nil {
		t.Fatal(err)
	}
	_, err = Analyze(
		doc,
		requestTemplate(http.MethodPatch, "https://api.example.test/things/123", `{}`),
		stableJSONBaseline(200, "application/json"),
		DefaultConfig(),
	)
	if !errors.Is(err, ErrAmbiguousOperation) {
		t.Fatalf("error=%v want ErrAmbiguousOperation", err)
	}
}

func TestAnalyzeUsesResponseClassThenDefault(t *testing.T) {
	doc, err := Parse([]byte(statusSpec))
	if err != nil {
		t.Fatal(err)
	}
	tmpl := requestTemplate(http.MethodPost, "https://api.example.test/jobs", `{}`)

	report, err := Analyze(doc, tmpl, stableJSONBaseline(201, "application/json"), DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if report.ResponseStatusKey != "2XX" || len(report.Candidates) != 1 || report.Candidates[0].Path != "$.class_field" {
		t.Fatalf("2XX report=%+v", report)
	}

	report, err = Analyze(doc, tmpl, stableJSONBaseline(418, "application/json"), DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if report.ResponseStatusKey != "default" || len(report.Candidates) != 1 || report.Candidates[0].Path != "$.default_field" {
		t.Fatalf("default report=%+v", report)
	}
}

func TestAnalyzeSkipsOneOfAndDeeperMissingParents(t *testing.T) {
	doc, err := Parse([]byte(boundarySpec))
	if err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(
		doc,
		requestTemplate(http.MethodPatch, "https://api.example.test/profile", `{"profile":{"name":"Tobias"}}`),
		stableJSONBaseline(200, "application/json"),
		DefaultConfig(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Candidates) != 0 {
		t.Fatalf("unexpected candidates=%+v", report.Candidates)
	}
	if !skipContains(report.Skipped, "$.choice", "oneOf/anyOf") {
		t.Fatalf("missing oneOf skip: %+v", report.Skipped)
	}
	if !skipContains(report.Skipped, "$.profile.settings.flags.beta", "deeper missing JSON parent") {
		t.Fatalf("missing deeper-parent skip: %+v", report.Skipped)
	}
}

func TestAnalyzeSkipsWhenMissingParentPositionIsOccupiedByScalar(t *testing.T) {
	doc, err := Parse([]byte(primarySpec))
	if err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(
		doc,
		requestTemplate(http.MethodPatch, "https://api.example.test/users/123", `{"display_name":"Tobias","profile":{"name":"Tobias","settings":"off"}}`),
		stableJSONBaseline(200, "application/json"),
		DefaultConfig(),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range report.Candidates {
		if candidate.Path == "$.profile.settings.beta_access" {
			t.Fatalf("beta_access must not be admitted over scalar settings: %+v", candidate)
		}
	}
	if !skipContains(report.Skipped, "$.profile.settings.beta_access", "not an object") {
		t.Fatalf("missing scalar-parent skip: %+v", report.Skipped)
	}
}

func TestAnalyzeRequiresStableBaselineMetadata(t *testing.T) {
	doc, err := Parse([]byte(primarySpec))
	if err != nil {
		t.Fatal(err)
	}
	baseline := stableJSONBaseline(200, "application/json")
	baseline.ContentTypeStable = false
	_, err = Analyze(doc, requestTemplate(http.MethodPatch, "https://api.example.test/users/123", `{}`), baseline, DefaultConfig())
	if !errors.Is(err, ErrUnstableBaseline) {
		t.Fatalf("error=%v want ErrUnstableBaseline", err)
	}
}

func TestParseRejectsExternalReferenceWithoutFetching(t *testing.T) {
	_, err := Parse([]byte(externalRefSpec))
	if err == nil {
		t.Fatal("external reference unexpectedly accepted")
	}
	if !strings.Contains(err.Error(), "build openapi model") {
		t.Fatalf("unexpected error=%v", err)
	}
}

func TestTemplatePathMatcher(t *testing.T) {
	cases := []struct {
		spec string
		path string
		want bool
	}{
		{spec: "/users/{id}", path: "/users/123", want: true},
		{spec: "/files/{id}.json", path: "/files/a.json", want: true},
		{spec: "/users/{id}", path: "/users/a/b", want: false},
		{spec: "/users/{bad name}", path: "/users/x", want: false},
	}
	for _, tc := range cases {
		if got := templatePathMatches(tc.spec, tc.path); got != tc.want {
			t.Fatalf("templatePathMatches(%q,%q)=%v want=%v", tc.spec, tc.path, got, tc.want)
		}
	}
}

func skipContains(skipped []SkippedDescriptor, path, fragment string) bool {
	for _, item := range skipped {
		if item.Path == path && strings.Contains(item.Reason, fragment) {
			return true
		}
	}
	return false
}

const primarySpec = `openapi: 3.1.0
info:
  title: ParamIntel schemaintel test
  version: "1"
paths:
  /users/{id}:
    parameters:
      - name: id
        in: path
        required: true
        schema:
          type: string
    patch:
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/UserUpdate'
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/UserUpdate'
                  - type: object
                    required: [role]
                    properties:
                      role:
                        type: string
                        readOnly: true
                      profile:
                        type: object
                        properties:
                          settings:
                            type: object
                            properties:
                              beta_access:
                                type: boolean
                                readOnly: true
  /users/me:
    patch:
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                display_name:
                  type: string
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  display_name:
                    type: string
                  self_only:
                    type: boolean
components:
  schemas:
    UserUpdate:
      type: object
      properties:
        display_name:
          type: string
        profile:
          type: object
          properties:
            name:
              type: string
`

const ambiguousSpec = `openapi: 3.1.0
info:
  title: ambiguous
  version: "1"
paths:
  /things/{id}:
    parameters:
      - name: id
        in: path
        required: true
        schema: {type: string}
    patch:
      requestBody:
        content:
          application/json:
            schema: {type: object}
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {type: object}
  /things/{name}:
    parameters:
      - name: name
        in: path
        required: true
        schema: {type: string}
    patch:
      requestBody:
        content:
          application/json:
            schema: {type: object}
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {type: object}
components: {}
`

const statusSpec = `openapi: 3.1.0
info:
  title: statuses
  version: "1"
paths:
  /jobs:
    post:
      requestBody:
        content:
          application/json:
            schema: {type: object}
      responses:
        2XX:
          description: class
          content:
            application/json:
              schema:
                type: object
                properties:
                  class_field: {type: boolean}
        default:
          description: default
          content:
            application/json:
              schema:
                type: object
                properties:
                  default_field: {type: string}
components: {}
`

const boundarySpec = `openapi: 3.1.0
info:
  title: boundaries
  version: "1"
paths:
  /profile:
    patch:
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                profile:
                  type: object
                  properties:
                    name: {type: string}
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  profile:
                    type: object
                    properties:
                      name: {type: string}
                      settings:
                        type: object
                        properties:
                          flags:
                            type: object
                            properties:
                              beta: {type: boolean}
                  choice:
                    oneOf:
                      - type: string
                      - type: integer
components: {}
`

const externalRefSpec = `openapi: 3.1.0
info:
  title: external refs
  version: "1"
paths:
  /profile:
    patch:
      requestBody:
        content:
          application/json:
            schema:
              $ref: 'https://example.invalid/request.yaml#/components/schemas/Request'
      responses:
        '200':
          description: ok
components: {}
`
