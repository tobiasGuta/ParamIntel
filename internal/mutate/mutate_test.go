package mutate

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestApplyFormPreservesExistingFields(t *testing.T) {
	tmpl := model.RequestTemplate{
		Method:  "POST",
		URL:     "https://example.test/search?existing=1",
		Headers: http.Header{"Content-Type": []string{"application/x-www-form-urlencoded"}, "Content-Length": []string{"999"}},
		Body:    []byte("q=test&mode=basic"),
	}
	out, err := Apply(tmpl, []model.Mutation{{Candidate: model.Candidate{Name: "debug", Location: model.LocationForm}, Value: model.StringValue("true")}})
	if err != nil {
		t.Fatal(err)
	}
	vals, err := url.ParseQuery(string(out.Body))
	if err != nil {
		t.Fatal(err)
	}
	if vals.Get("q") != "test" || vals.Get("mode") != "basic" || vals.Get("debug") != "true" {
		t.Fatalf("form=%v", vals)
	}
	if out.Headers.Get("Content-Length") != "" {
		t.Fatalf("content-length should be removed: %q", out.Headers.Get("Content-Length"))
	}
	if out.URL != tmpl.URL {
		t.Fatalf("url changed: %q", out.URL)
	}
}

func TestApplyNestedJSONWithTypedValue(t *testing.T) {
	tmpl := model.RequestTemplate{Method: "POST", URL: "https://example.test/api", Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"query":"x","filters":{"status":"active"}}`)}
	out, err := Apply(tmpl, []model.Mutation{{Candidate: model.Candidate{Name: "limit", Location: model.LocationJSON, JSONParent: "$.filters"}, Value: model.IntegerValue(10)}})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Body, &got); err != nil {
		t.Fatal(err)
	}
	filters := got["filters"].(map[string]any)
	if filters["status"] != "active" || filters["limit"].(float64) != 10 {
		t.Fatalf("json=%s", out.Body)
	}
}

func TestApplyJSONWithTextPlainContentType(t *testing.T) {
	tmpl := model.RequestTemplate{
		Method:  "POST",
		URL:     "https://example.test/api/checkout",
		Headers: http.Header{"Content-Type": []string{"text/plain;charset=UTF-8"}},
		Body:    []byte(`{"chosen_products":[{"product_id":"1","quantity":1}]}`),
	}
	out, err := Apply(tmpl, []model.Mutation{{Candidate: model.Candidate{Name: "chosen_discount", Location: model.LocationJSON, JSONParent: "$"}, Value: model.StringValue("probe")}})
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Headers.Get("Content-Type"); got != "text/plain;charset=UTF-8" {
		t.Fatalf("content type changed: %q", got)
	}
	var body map[string]any
	if err := json.Unmarshal(out.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["chosen_discount"] != "probe" {
		t.Fatalf("json=%s", out.Body)
	}
}

func TestApplyScaffoldRequiresExplicitAuthorization(t *testing.T) {
	tmpl := model.RequestTemplate{Method: "POST", URL: "https://example.test/api", Body: []byte(`{"profile":{"name":"tobias"}}`)}
	candidate := model.Candidate{
		Name:               "beta_access",
		Location:           model.LocationJSON,
		JSONParent:         "$.profile.settings",
		JSONScaffoldParent: "$.profile.settings",
	}
	_, err := Apply(tmpl, []model.Mutation{{Candidate: candidate, Value: model.BoolValue(true)}})
	if err == nil || !strings.Contains(err.Error(), "requires explicit authorization") {
		t.Fatalf("expected explicit scaffold authorization error, got %v", err)
	}
}

func TestApplyCreatesExactlyOneMissingObjectLevel(t *testing.T) {
	tmpl := model.RequestTemplate{Method: "POST", URL: "https://example.test/api", Body: []byte(`{"profile":{"name":"tobias"}}`)}
	candidate := model.Candidate{
		Name:               "beta_access",
		Location:           model.LocationJSON,
		JSONParent:         "$.profile.settings",
		JSONScaffoldParent: "$.profile.settings",
	}
	out, err := Apply(tmpl, []model.Mutation{{Candidate: candidate, Value: model.BoolValue(true), AllowJSONScaffold: true}})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Body, &got); err != nil {
		t.Fatal(err)
	}
	profile := got["profile"].(map[string]any)
	settings := profile["settings"].(map[string]any)
	if settings["beta_access"] != true || profile["name"] != "tobias" {
		t.Fatalf("json=%s", out.Body)
	}
}

func TestApplyScaffoldNeverReplacesExistingParent(t *testing.T) {
	for _, body := range []string{
		`{"settings":null}`,
		`{"settings":"disabled"}`,
		`{"settings":[]}`,
		`{"settings":{"existing":true}}`,
	} {
		tmpl := model.RequestTemplate{Method: "POST", URL: "https://example.test/api", Body: []byte(body)}
		candidate := model.Candidate{
			Name:               "beta_access",
			Location:           model.LocationJSON,
			JSONParent:         "$.settings",
			JSONScaffoldParent: "$.settings",
		}
		_, err := Apply(tmpl, []model.Mutation{{Candidate: candidate, Value: model.BoolValue(true), AllowJSONScaffold: true}})
		if err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("existing parent must not be replaced: body=%s err=%v", body, err)
		}
	}
}

func TestApplyScaffoldRejectsTwoMissingObjectLevels(t *testing.T) {
	tmpl := model.RequestTemplate{Method: "POST", URL: "https://example.test/api", Body: []byte(`{"name":"tobias"}`)}
	candidate := model.Candidate{
		Name:               "beta_access",
		Location:           model.LocationJSON,
		JSONParent:         "$.profile.settings",
		JSONScaffoldParent: "$.profile.settings",
	}
	_, err := Apply(tmpl, []model.Mutation{{Candidate: candidate, Value: model.BoolValue(true), AllowJSONScaffold: true}})
	if err == nil || !strings.Contains(err.Error(), "cannot scaffold JSON parent") {
		t.Fatalf("expected multi-level scaffold rejection, got %v", err)
	}
}

func TestApplyScaffoldRequiresMatchingParentMetadata(t *testing.T) {
	tmpl := model.RequestTemplate{Method: "POST", URL: "https://example.test/api", Body: []byte(`{"profile":{"name":"tobias"}}`)}
	candidate := model.Candidate{
		Name:               "beta_access",
		Location:           model.LocationJSON,
		JSONParent:         "$.profile.settings",
		JSONScaffoldParent: "$.profile.preferences",
	}
	_, err := Apply(tmpl, []model.Mutation{{Candidate: candidate, Value: model.BoolValue(true), AllowJSONScaffold: true}})
	if err == nil || !strings.Contains(err.Error(), "does not match candidate parent") {
		t.Fatalf("expected scaffold metadata mismatch rejection, got %v", err)
	}
}

func TestApplyMultipleMutationsMayShareOneAuthorizedScaffold(t *testing.T) {
	tmpl := model.RequestTemplate{Method: "POST", URL: "https://example.test/api", Body: []byte(`{"profile":{"name":"tobias"}}`)}
	base := model.Candidate{Location: model.LocationJSON, JSONParent: "$.profile.settings", JSONScaffoldParent: "$.profile.settings"}
	out, err := Apply(tmpl, []model.Mutation{
		{Candidate: model.Candidate{Name: "beta_access", Location: base.Location, JSONParent: base.JSONParent, JSONScaffoldParent: base.JSONScaffoldParent}, Value: model.BoolValue(true), AllowJSONScaffold: true},
		{Candidate: model.Candidate{Name: "mode", Location: base.Location, JSONParent: base.JSONParent, JSONScaffoldParent: base.JSONScaffoldParent}, Value: model.StringValue("probe"), AllowJSONScaffold: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Body, &got); err != nil {
		t.Fatal(err)
	}
	settings := got["profile"].(map[string]any)["settings"].(map[string]any)
	if settings["beta_access"] != true || settings["mode"] != "probe" {
		t.Fatalf("json=%s", out.Body)
	}
}

func TestJSONObjectParentsFindsObjectsButNotArrays(t *testing.T) {
	parents := JSONObjectParents([]byte(`{"filters":{"nested":{"x":1}},"items":[{"hidden":true}]}`), 3)
	sort.Strings(parents)
	want := []string{"$", "$.filters", "$.filters.nested"}
	sort.Strings(want)
	if !reflect.DeepEqual(parents, want) {
		t.Fatalf("parents=%v want=%v", parents, want)
	}
}
