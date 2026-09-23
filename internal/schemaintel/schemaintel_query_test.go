package schemaintel

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

const oas321QuerySpec = `openapi: 3.2.1
info:
  title: OAS 3.2.1 QUERY Test
  version: 1.0.0
paths:
  /search:
    query:
      operationId: searchQuery
      summary: Search query operation
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                filter:
                  type: string
                sort:
                  type: string
                pagination:
                  type: object
                  properties:
                    page:
                      type: integer
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  filter:
                    type: string
                  results:
                    type: array
                    items:
                      type: object
                  total:
                    type: integer
                  debug:
                    type: string
                    readOnly: true
                  pagination:
                    type: object
                    properties:
                      page:
                        type: integer
                      cursor:
                        type: string
`

func TestAnalyze_OAS321_QueryOperationSelection_And_CandidateGeneration(t *testing.T) {
	doc, err := Parse([]byte(oas321QuerySpec))
	if err != nil {
		t.Fatalf("Parse OAS 3.2.1 spec: %v", err)
	}
	if doc.Version() != "3.2.1" {
		t.Fatalf("doc.Version()=%q want 3.2.1", doc.Version())
	}

	tmpl := model.RequestTemplate{
		Method: "QUERY",
		URL:    "https://api.example.test/search",
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: []byte(`{"filter":"active","sort":"created_at","pagination":{"page":1}}`),
	}

	report, err := Analyze(doc, tmpl, stableJSONBaseline(200, "application/json"), DefaultConfig())
	if err != nil {
		t.Fatalf("Analyze(QUERY): %v", err)
	}

	// 1. Operation selection
	if report.Operation.Method != "QUERY" {
		t.Fatalf("Operation.Method=%q want QUERY", report.Operation.Method)
	}
	if report.Operation.SpecPath != "/search" {
		t.Fatalf("Operation.SpecPath=%q want /search", report.Operation.SpecPath)
	}
	if report.RequestMediaType != "application/json" {
		t.Fatalf("RequestMediaType=%q want application/json", report.RequestMediaType)
	}

	// 2. Request schema extraction: request properties must be documented
	reqProps := make(map[string]bool)
	for _, p := range report.RequestProperties {
		reqProps[p.Path] = true
	}
	if !reqProps["$.filter"] || !reqProps["$.sort"] || !reqProps["$.pagination.page"] {
		t.Fatalf("missing request properties: %+v", report.RequestProperties)
	}

	// 3. Response-only candidates generation & request property exclusion
	candidatesByPath := make(map[string]CandidateDescriptor)
	for _, c := range report.Candidates {
		candidatesByPath[c.Path] = c
	}

	// $.filter is in request schema -> must NOT be a candidate
	if _, ok := candidatesByPath["$.filter"]; ok {
		t.Fatalf("$.filter documented in request schema must not be candidate")
	}
	// $.sort is in request schema -> must NOT be a candidate
	if _, ok := candidatesByPath["$.sort"]; ok {
		t.Fatalf("$.sort documented in request schema must not be candidate")
	}
	// $.pagination.page is in request schema -> must NOT be a candidate
	if _, ok := candidatesByPath["$.pagination.page"]; ok {
		t.Fatalf("$.pagination.page documented in request schema must not be candidate")
	}

	// $.results is an array -> must be skipped, not candidate
	if _, ok := candidatesByPath["$.results"]; ok {
		t.Fatalf("$.results is an array and must not be a candidate")
	}
	if !skipContains(report.Skipped, "$.results", "array-valued property") {
		t.Fatalf("missing array skip for $.results: %+v", report.Skipped)
	}

	// $.total is response-only integer -> must be candidate with existing parent ($)
	total, ok := candidatesByPath["$.total"]
	if !ok {
		t.Fatalf("$.total candidate missing: %+v", report.Candidates)
	}
	if total.Placement != PlacementExistingParent {
		t.Fatalf("$.total placement=%q want %q", total.Placement, PlacementExistingParent)
	}

	// $.debug is response-only readOnly -> candidate per current policy, retaining ReadOnly=true
	debug, ok := candidatesByPath["$.debug"]
	if !ok {
		t.Fatalf("$.debug candidate missing: %+v", report.Candidates)
	}
	if !debug.ReadOnly {
		t.Fatalf("$.debug ReadOnly=%v want true", debug.ReadOnly)
	}

	// $.pagination.cursor is nested response-only in existing parent $.pagination -> candidate
	cursor, ok := candidatesByPath["$.pagination.cursor"]
	if !ok {
		t.Fatalf("$.pagination.cursor candidate missing: %+v", report.Candidates)
	}
	if cursor.Placement != PlacementExistingParent {
		t.Fatalf("$.pagination.cursor placement=%q want %q", cursor.Placement, PlacementExistingParent)
	}
}

func TestAnalyze_OAS321_Query_MissingAndIncompatibleContentType(t *testing.T) {
	doc, err := Parse([]byte(oas321QuerySpec))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	baseline := stableJSONBaseline(200, "application/json")

	// Missing Content-Type
	tmplNoCT := model.RequestTemplate{
		Method:  "QUERY",
		URL:     "https://api.example.test/search",
		Headers: http.Header{},
		Body:    []byte(`{"filter":"test"}`),
	}
	_, err = Analyze(doc, tmplNoCT, baseline, DefaultConfig())
	if !errors.Is(err, ErrNonJSONMediaType) {
		t.Fatalf("missing CT error=%v want ErrNonJSONMediaType", err)
	}

	// Incompatible Content-Type: text/plain
	tmplText := model.RequestTemplate{
		Method:  "QUERY",
		URL:     "https://api.example.test/search",
		Headers: http.Header{"Content-Type": []string{"text/plain"}},
		Body:    []byte(`{"filter":"test"}`),
	}
	_, err = Analyze(doc, tmplText, baseline, DefaultConfig())
	if !errors.Is(err, ErrNonJSONMediaType) {
		t.Fatalf("text/plain error=%v want ErrNonJSONMediaType", err)
	}

	// Incompatible Content-Type: application/xml
	tmplXML := model.RequestTemplate{
		Method:  "QUERY",
		URL:     "https://api.example.test/search",
		Headers: http.Header{"Content-Type": []string{"application/xml"}},
		Body:    []byte(`<search><filter>test</filter></search>`),
	}
	_, err = Analyze(doc, tmplXML, baseline, DefaultConfig())
	if !errors.Is(err, ErrNonJSONMediaType) {
		t.Fatalf("application/xml error=%v want ErrNonJSONMediaType", err)
	}

	// Structured syntax JSON: application/vnd.api+json (passes isJSONMediaType, but not in fixture schema)
	tmplVnd := model.RequestTemplate{
		Method:  "QUERY",
		URL:     "https://api.example.test/search",
		Headers: http.Header{"Content-Type": []string{"application/vnd.api+json"}},
		Body:    []byte(`{"filter":"test"}`),
	}
	_, err = Analyze(doc, tmplVnd, baseline, DefaultConfig())
	if !errors.Is(err, ErrMediaTypeNotFound) {
		t.Fatalf("vnd.api+json error=%v want ErrMediaTypeNotFound", err)
	}
}

const oas321QueryRefSpec = `openapi: 3.2.1
info:
  title: OAS 3.2.1 Internal Ref Test
  version: 1.0.0
paths:
  /search:
    query:
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/SearchRequest'
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/SearchResponse'
components:
  schemas:
    SearchRequest:
      type: object
      properties:
        query_text:
          type: string
    SearchResponse:
      type: object
      properties:
        query_text:
          type: string
        total_hits:
          type: integer
`

func TestAnalyze_OAS321_Query_InternalReferences(t *testing.T) {
	doc, err := Parse([]byte(oas321QueryRefSpec))
	if err != nil {
		t.Fatalf("Parse with internal refs: %v", err)
	}

	tmpl := model.RequestTemplate{
		Method: "QUERY",
		URL:    "https://api.example.test/search",
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: []byte(`{"query_text":"antigravity"}`),
	}

	report, err := Analyze(doc, tmpl, stableJSONBaseline(200, "application/json"), DefaultConfig())
	if err != nil {
		t.Fatalf("Analyze with internal refs: %v", err)
	}

	if len(report.Candidates) != 1 {
		t.Fatalf("candidates=%+v want 1 candidate", report.Candidates)
	}
	if report.Candidates[0].Path != "$.total_hits" {
		t.Fatalf("candidate=%s want $.total_hits", report.Candidates[0].Path)
	}
}

const oas321DeepNestedSpec = `openapi: 3.2.1
info:
  title: Deep Nested Test
  version: 1.0.0
paths:
  /deep:
    query:
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                l1:
                  type: object
                  properties:
                    l2:
                      type: object
                      properties:
                        l3:
                          type: object
                          properties:
                            l4:
                              type: string
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  l1:
                    type: object
                    properties:
                      l2:
                        type: object
                        properties:
                          l3:
                            type: object
                            properties:
                              l4:
                                type: string
                              resp_l4:
                                type: string
`

func TestAnalyze_OAS321_Query_DepthLimits(t *testing.T) {
	doc, err := Parse([]byte(oas321DeepNestedSpec))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	tmpl := model.RequestTemplate{
		Method: "QUERY",
		URL:    "https://api.example.test/deep",
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: []byte(`{"l1":{"l2":{"l3":{"l4":"val"}}}}`),
	}

	// With MaxDepth: 2, l3 and l4 are beyond max depth
	cfg := Config{MaxDepth: 2}
	report, err := Analyze(doc, tmpl, stableJSONBaseline(200, "application/json"), cfg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	for _, c := range report.Candidates {
		if strings.HasPrefix(c.Path, "$.l1.l2.l3") {
			t.Fatalf("candidate %s exceeded MaxDepth=2", c.Path)
		}
	}
	if !skipContains(report.Skipped, "$.l1.l2.l3", "schema depth limit reached") {
		t.Fatalf("expected skip for schema depth limit reached, got: %+v", report.Skipped)
	}
}

func TestParse_RemoteAndExternalReferencesRemainDisabled(t *testing.T) {
	specs := []struct {
		name string
		yaml string
	}{
		{
			name: "remote_https",
			yaml: `openapi: 3.2.1
info: {title: remote, version: "1"}
paths:
  /test:
    query:
      requestBody:
        content:
          application/json:
            schema:
              $ref: 'https://example.invalid/remote.yaml#/components/schemas/Query'
      responses:
        '200': {description: ok}
`,
		},
		{
			name: "external_file",
			yaml: `openapi: 3.2.1
info: {title: file, version: "1"}
paths:
  /test:
    query:
      requestBody:
        content:
          application/json:
            schema:
              $ref: './local_file.yaml#/components/schemas/Query'
      responses:
        '200': {description: ok}
`,
		},
	}

	for _, tc := range specs {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil {
				t.Fatalf("%s: external/remote reference unexpectedly succeeded", tc.name)
			}
			if !strings.Contains(err.Error(), "build openapi model") {
				t.Fatalf("%s: unexpected error message: %v", tc.name, err)
			}
		})
	}
}

func TestAnalyze_OASVersionCompatibility_30_31_32(t *testing.T) {
	// OAS 3.0.3 (POST)
	oas30 := `openapi: 3.0.3
info: {title: oas30, version: "1"}
paths:
  /resource:
    post:
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties: {in_req: {type: string}}
      responses:
        '200':
          content:
            application/json:
              schema:
                type: object
                properties:
                  in_req: {type: string}
                  resp_only: {type: integer}
`
	// OAS 3.1.0 (POST)
	oas31 := `openapi: 3.1.0
info: {title: oas31, version: "1"}
paths:
  /resource:
    post:
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties: {in_req: {type: string}}
      responses:
        '200':
          content:
            application/json:
              schema:
                type: object
                properties:
                  in_req: {type: string}
                  resp_only: {type: integer}
`
	// OAS 3.2.1 (QUERY)
	oas32 := `openapi: 3.2.1
info: {title: oas32, version: "1"}
paths:
  /resource:
    query:
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties: {in_req: {type: string}}
      responses:
        '200':
          content:
            application/json:
              schema:
                type: object
                properties:
                  in_req: {type: string}
                  resp_only: {type: integer}
`

	tests := []struct {
		ver    string
		method string
		spec   string
	}{
		{"3.0.3", "POST", oas30},
		{"3.1.0", "POST", oas31},
		{"3.2.1", "QUERY", oas32},
	}

	for _, tt := range tests {
		t.Run("OAS_"+tt.ver+"_"+tt.method, func(t *testing.T) {
			doc, err := Parse([]byte(tt.spec))
			if err != nil {
				t.Fatalf("Parse %s: %v", tt.ver, err)
			}
			tmpl := model.RequestTemplate{
				Method:  tt.method,
				URL:     "https://api.example.test/resource",
				Headers: http.Header{"Content-Type": []string{"application/json"}},
				Body:    []byte(`{"in_req":"val"}`),
			}
			report, err := Analyze(doc, tmpl, stableJSONBaseline(200, "application/json"), DefaultConfig())
			if err != nil {
				t.Fatalf("Analyze %s: %v", tt.ver, err)
			}
			if report.Operation.Method != tt.method {
				t.Fatalf("method=%s want %s", report.Operation.Method, tt.method)
			}
			if len(report.Candidates) != 1 || report.Candidates[0].Path != "$.resp_only" {
				t.Fatalf("candidates=%+v want $.resp_only", report.Candidates)
			}
		})
	}
}
