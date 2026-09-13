package openapispike

import (
	"context"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func loadAndValidate(spec string) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	// This is the proposed ParamIntel boundary: local document + internal refs only.
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromData([]byte(spec))
	if err != nil {
		return nil, err
	}
	if err := doc.Validate(context.Background()); err != nil {
		return doc, err
	}
	return doc, nil
}

// kin-openapi v0.135.0 is the last release we found that preserves ParamIntel's
// Go 1.23 baseline. It handles the OAS 3.0 fixture, but its validator rejects
// the ordinary OAS 3.1 union type used by spec31. This is a comparison result,
// not a production requirement we intend to weaken.
func TestKinOpenAPIBaselineAnd31UnionLimitation(t *testing.T) {
	doc, err := loadAndValidate(spec30)
	if err != nil {
		t.Fatalf("kin-openapi v0.135.0 should handle the OAS 3.0 fixture: %v", err)
	}
	if doc.OpenAPI != "3.0.3" {
		t.Fatalf("version=%q want=3.0.3", doc.OpenAPI)
	}

	_, err = loadAndValidate(spec31)
	if err == nil {
		t.Fatal("kin-openapi v0.135.0 unexpectedly accepted the OAS 3.1 union fixture; revisit parser decision")
	}
	if !strings.Contains(err.Error(), `unsupported 'type' value "null"`) {
		t.Fatalf("unexpected OAS 3.1 failure: %v", err)
	}
	t.Logf("SPIKE_RESULT parser=kin-openapi-v0.135.0 oas=3.1 union_type=rejected error=%q", err)
}

// OpenAPI 3.2.1 is deliberately observational in this spike. A basic document
// being accepted does NOT establish full 3.2.1 semantic support in the parser.
func TestOpenAPI321CompatibilityProbe(t *testing.T) {
	doc, err := loadAndValidate(spec321)
	if err != nil {
		t.Logf("SPIKE_RESULT parser=kin-openapi-v0.135.0 oas=3.2.1 basic_parse_validate=rejected error=%q", err)
		return
	}
	t.Logf("SPIKE_RESULT parser=kin-openapi-v0.135.0 oas=3.2.1 basic_parse_validate=accepted version=%s full_semantic_support=unproven", doc.OpenAPI)
}

func TestOperationSchemaTraversal(t *testing.T) {
	doc, err := loadAndValidate(spec30)
	if err != nil {
		t.Fatal(err)
	}
	item := doc.Paths.Find("/users/{id}")
	if item == nil || item.Patch == nil {
		t.Fatal("PATCH /users/{id} was not resolved")
	}
	op := item.Patch
	if op.RequestBody == nil || op.RequestBody.Value == nil {
		t.Fatal("request body missing")
	}
	reqMedia := op.RequestBody.Value.Content["application/json"]
	if reqMedia == nil || reqMedia.Schema == nil || reqMedia.Schema.Value == nil {
		t.Fatal("request JSON schema was not resolved")
	}
	if _, ok := reqMedia.Schema.Value.Properties["display_name"]; !ok {
		t.Fatal("expected request property display_name")
	}

	resp := op.Responses.Value("200")
	if resp == nil || resp.Value == nil {
		t.Fatal("200 response missing")
	}
	respMedia := resp.Value.Content["application/json"]
	if respMedia == nil || respMedia.Schema == nil || respMedia.Schema.Value == nil {
		t.Fatal("response JSON schema was not resolved")
	}
	if len(respMedia.Schema.Value.AllOf) != 2 {
		t.Fatalf("response allOf branches=%d want=2", len(respMedia.Schema.Value.AllOf))
	}
	for i, branch := range respMedia.Schema.Value.AllOf {
		if branch == nil || branch.Value == nil {
			t.Fatalf("allOf branch %d was not resolved", i)
		}
	}
}

func TestOneOfIsPreservedForExplicitPolicyDecision(t *testing.T) {
	doc, err := loadAndValidate(specOneOf)
	if err != nil {
		t.Fatal(err)
	}
	choice := doc.Components.Schemas["Choice"]
	if choice == nil || choice.Value == nil {
		t.Fatal("Choice schema missing")
	}
	if len(choice.Value.OneOf) != 2 {
		t.Fatalf("oneOf branches=%d want=2", len(choice.Value.OneOf))
	}
	for i, branch := range choice.Value.OneOf {
		if branch == nil || branch.Value == nil {
			t.Fatalf("oneOf branch %d was not resolved", i)
		}
	}
}

func TestInternalReferenceCycleLoadsWithoutUnboundedTraversal(t *testing.T) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromData([]byte(specCycle))
	if err != nil {
		t.Fatalf("cyclic internal refs should be bounded by the parser: %v", err)
	}
	if doc.Components.Schemas["A"] == nil || doc.Components.Schemas["B"] == nil {
		t.Fatal("cyclic component schemas were not retained")
	}
	// Do not recursively walk the cycle here. Production schemaintel must carry
	// its own visited-ref/depth budget even when the parser resolves pointers.
	t.Log("SPIKE_RESULT parser=kin-openapi-v0.135.0 internal_ref_cycle=loaded production_walk_requires_visited_set")
}

func TestExternalReferenceFailsClosed(t *testing.T) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	_, err := loader.LoadFromData([]byte(specExternalRef))
	if err == nil {
		t.Fatal("external $ref unexpectedly loaded while external refs are disabled")
	}
	if !strings.Contains(err.Error(), "disallowed external reference") {
		t.Fatalf("unexpected external-ref error: %v", err)
	}
	t.Logf("SPIKE_RESULT parser=kin-openapi-v0.135.0 external_ref=blocked error=%q", err)
}

// Ambiguous templated paths must be rejected by ParamIntel's future operation
// matcher regardless of whether the parser accepts the document itself.
func TestAmbiguousTemplatePathsAreNotDelegatedToParser(t *testing.T) {
	_, err := loadAndValidate(specAmbiguousPaths)
	if err != nil {
		t.Logf("SPIKE_RESULT parser=kin-openapi-v0.135.0 ambiguous_templates parser_validation=rejected error=%q", err)
		return
	}
	t.Log("SPIKE_RESULT parser=kin-openapi-v0.135.0 ambiguous_templates parser_validation=accepted paramintel_matcher_must_reject_ambiguity=true")
}

const spec30 = `openapi: 3.0.3
info:
  title: ParamIntel spike
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
                    properties:
                      role:
                        type: string
                        readOnly: true
                      settings:
                        type: object
                        properties:
                          beta_access:
                            type: boolean
components:
  schemas:
    UserUpdate:
      type: object
      properties:
        display_name:
          type: string
`

const spec31 = `openapi: 3.1.0
info:
  title: ParamIntel spike
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
                nickname:
                  type:
                    - string
                    - 'null'
      responses:
        '200':
          description: ok
components: {}
`

const spec321 = `openapi: 3.2.1
info:
  title: ParamIntel spike
  version: "1"
paths:
  /profile:
    get:
      responses:
        '200':
          description: ok
components: {}
`

const specOneOf = `openapi: 3.1.0
info:
  title: ParamIntel spike
  version: "1"
paths: {}
components:
  schemas:
    Alpha:
      type: object
      properties:
        alpha:
          type: string
    Beta:
      type: object
      properties:
        beta:
          type: boolean
    Choice:
      oneOf:
        - $ref: '#/components/schemas/Alpha'
        - $ref: '#/components/schemas/Beta'
`

const specCycle = `openapi: 3.1.0
info:
  title: ParamIntel spike
  version: "1"
paths: {}
components:
  schemas:
    A:
      type: object
      properties:
        b:
          $ref: '#/components/schemas/B'
    B:
      type: object
      properties:
        a:
          $ref: '#/components/schemas/A'
`

const specExternalRef = `openapi: 3.1.0
info:
  title: ParamIntel spike
  version: "1"
paths: {}
components:
  schemas:
    External:
      $ref: 'https://example.invalid/model.yaml#/components/schemas/External'
`

const specAmbiguousPaths = `openapi: 3.1.0
info:
  title: ParamIntel spike
  version: "1"
paths:
  /users/{id}:
    parameters:
      - name: id
        in: path
        required: true
        schema:
          type: string
    get:
      responses:
        '200':
          description: by id
  /users/{name}:
    parameters:
      - name: name
        in: path
        required: true
        schema:
          type: string
    get:
      responses:
        '200':
          description: by name
components: {}
`
