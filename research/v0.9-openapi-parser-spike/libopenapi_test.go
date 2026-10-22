package openapispike

import (
	"errors"
	"net/http"
	"testing"

	libopenapi "github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel"
	v3high "github.com/pb33f/libopenapi/datamodel/high/v3"
)

func buildLibV3(spec string) (*libopenapi.DocumentModel[v3high.Document], []error, error) {
	cfg := datamodel.NewDocumentConfiguration()
	cfg.AllowFileReferences = false
	cfg.AllowRemoteReferences = false
	doc, err := libopenapi.NewDocumentWithConfiguration([]byte(spec), cfg)
	if err != nil {
		return nil, nil, err
	}
	model, errs := doc.BuildV3Model()
	return model, errs, nil
}

func TestLibOpenAPIMustSupportOpenAPI30And31(t *testing.T) {
	cases := []struct {
		name    string
		version string
		spec    string
	}{
		{name: "3.0.3", version: "3.0.3", spec: spec30},
		{name: "3.1.0", version: "3.1.0", spec: spec31},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model, errs, err := buildLibV3(tc.spec)
			if err != nil {
				t.Fatalf("document parse failed: %v", err)
			}
			if len(errs) > 0 {
				t.Fatalf("model build errors: %v", errs)
			}
			if model == nil || model.Model.Version != tc.version {
				t.Fatalf("model/version mismatch: model=%v version=%q", model != nil, model.Model.Version)
			}
		})
	}

	// This is the 3.1 feature kin-openapi v0.135.0 rejected in the same spike.
	model, errs, err := buildLibV3(spec31)
	if err != nil || len(errs) > 0 || model == nil {
		t.Fatalf("3.1 union fixture failed: err=%v build=%v", err, errs)
	}
	item := model.Model.Paths.PathItems.GetOrZero("/profile")
	if item == nil || item.Patch == nil || item.Patch.RequestBody == nil {
		t.Fatal("PATCH /profile request body missing")
	}
	media := item.Patch.RequestBody.Content.GetOrZero("application/json")
	if media == nil || media.Schema == nil {
		t.Fatal("3.1 request JSON schema missing")
	}
	schema := media.Schema.Schema()
	if schema == nil {
		t.Fatalf("3.1 request schema build failed: %v", media.Schema.GetBuildError())
	}
	nickname := schema.Properties.GetOrZero("nickname")
	if nickname == nil || nickname.Schema() == nil {
		t.Fatal("nickname schema missing")
	}
	gotTypes := nickname.Schema().Type
	if !containsString(gotTypes, "string") || !containsString(gotTypes, "null") {
		t.Fatalf("3.1 union types=%v want string+null", gotTypes)
	}
	t.Logf("SPIKE_RESULT parser=libopenapi-v0.25.0 oas=3.1 union_type=accepted types=%v", gotTypes)
}

func TestLibOpenAPIOperationSchemaTraversal(t *testing.T) {
	model, errs, err := buildLibV3(spec30)
	if err != nil || len(errs) > 0 || model == nil {
		t.Fatalf("build failed: err=%v build=%v", err, errs)
	}
	item := model.Model.Paths.PathItems.GetOrZero("/users/{id}")
	if item == nil || item.Patch == nil {
		t.Fatal("PATCH /users/{id} missing")
	}
	op := item.Patch
	requestMedia := op.RequestBody.Content.GetOrZero("application/json")
	if requestMedia == nil || requestMedia.Schema == nil {
		t.Fatal("request media/schema missing")
	}
	requestSchema := requestMedia.Schema.Schema()
	if requestSchema == nil || requestSchema.Properties.GetOrZero("display_name") == nil {
		t.Fatal("request property display_name was not navigable")
	}

	response := op.Responses.FindResponseByCode(200)
	if response == nil {
		t.Fatal("200 response missing")
	}
	responseMedia := response.Content.GetOrZero("application/json")
	if responseMedia == nil || responseMedia.Schema == nil {
		t.Fatal("response media/schema missing")
	}
	responseSchema := responseMedia.Schema.Schema()
	if responseSchema == nil {
		t.Fatalf("response schema build failed: %v", responseMedia.Schema.GetBuildError())
	}
	if len(responseSchema.AllOf) != 2 {
		t.Fatalf("allOf branches=%d want=2", len(responseSchema.AllOf))
	}
	for i, branch := range responseSchema.AllOf {
		if branch == nil || branch.Schema() == nil {
			t.Fatalf("allOf branch %d did not resolve: %v", i, branch.GetBuildError())
		}
	}
}

func TestLibOpenAPIOneOfIsPreserved(t *testing.T) {
	model, errs, err := buildLibV3(specOneOf)
	if err != nil || len(errs) > 0 || model == nil {
		t.Fatalf("build failed: err=%v build=%v", err, errs)
	}
	choiceProxy := model.Model.Components.Schemas.GetOrZero("Choice")
	if choiceProxy == nil {
		t.Fatal("Choice component missing")
	}
	choice := choiceProxy.Schema()
	if choice == nil {
		t.Fatalf("Choice schema build failed: %v", choiceProxy.GetBuildError())
	}
	if len(choice.OneOf) != 2 {
		t.Fatalf("oneOf branches=%d want=2", len(choice.OneOf))
	}
	for i, branch := range choice.OneOf {
		if branch == nil || branch.Schema() == nil {
			t.Fatalf("oneOf branch %d did not resolve", i)
		}
	}
}

func TestLibOpenAPIInternalCycleIsBounded(t *testing.T) {
	cfg := datamodel.NewDocumentConfiguration()
	cfg.AllowFileReferences = false
	cfg.AllowRemoteReferences = false
	doc, err := libopenapi.NewDocumentWithConfiguration([]byte(specCycle), cfg)
	if err != nil {
		t.Fatalf("document creation failed: %v", err)
	}
	model, errs := doc.BuildV3Model()
	if model == nil && len(errs) == 0 {
		t.Fatal("cycle produced neither a model nor a bounded diagnostic")
	}
	t.Logf("SPIKE_RESULT parser=libopenapi-v0.25.0 internal_ref_cycle model=%v diagnostics=%d", model != nil, len(errs))
}

func TestLibOpenAPIExternalReferenceDoesNotFetch(t *testing.T) {
	called := false
	cfg := datamodel.NewDocumentConfiguration()
	cfg.AllowFileReferences = false
	cfg.AllowRemoteReferences = false
	cfg.RemoteURLHandler = func(string) (*http.Response, error) {
		called = true
		return nil, errors.New("remote handler must not be invoked")
	}
	doc, err := libopenapi.NewDocumentWithConfiguration([]byte(specExternalRef), cfg)
	if err != nil {
		t.Fatalf("document creation failed: %v", err)
	}
	model, errs := doc.BuildV3Model()
	if called {
		t.Fatal("remote URL handler was invoked while remote refs were disabled")
	}
	t.Logf("SPIKE_RESULT parser=libopenapi-v0.25.0 external_ref remote_fetch=false model=%v diagnostics=%d", model != nil, len(errs))
}

func TestLibOpenAPI321CompatibilityProbe(t *testing.T) {
	model, errs, err := buildLibV3(spec321)
	if err != nil {
		t.Logf("SPIKE_RESULT parser=libopenapi-v0.25.0 oas=3.2.1 document_parse=rejected error=%q", err)
		return
	}
	if model == nil || len(errs) > 0 {
		t.Logf("SPIKE_RESULT parser=libopenapi-v0.25.0 oas=3.2.1 model_build model=%v diagnostics=%v full_semantic_support=unproven", model != nil, errs)
		return
	}
	t.Logf("SPIKE_RESULT parser=libopenapi-v0.25.0 oas=3.2.1 basic_parse_build=accepted version=%s full_semantic_support=unproven", model.Model.Version)
}

func TestLibOpenAPIAmbiguousTemplatesRemainParamIntelPolicy(t *testing.T) {
	model, errs, err := buildLibV3(specAmbiguousPaths)
	if err != nil {
		t.Logf("SPIKE_RESULT parser=libopenapi-v0.25.0 ambiguous_templates document_parse=rejected error=%q", err)
		return
	}
	t.Logf("SPIKE_RESULT parser=libopenapi-v0.25.0 ambiguous_templates model=%v diagnostics=%d paramintel_matcher_must_reject_ambiguity=true", model != nil, len(errs))
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
