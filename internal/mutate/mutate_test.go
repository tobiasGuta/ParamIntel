package mutate

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"sort"
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

func TestJSONObjectParentsFindsObjectsButNotArrays(t *testing.T) {
	parents := JSONObjectParents([]byte(`{"filters":{"nested":{"x":1}},"items":[{"hidden":true}]}`), 3)
	sort.Strings(parents)
	want := []string{"$", "$.filters", "$.filters.nested"}
	sort.Strings(want)
	if !reflect.DeepEqual(parents, want) {
		t.Fatalf("parents=%v want=%v", parents, want)
	}
}
