package contextintel

import "testing"

func TestHarvestResponseOnlyRootPropertyFromRawHTTP(t *testing.T) {
	request := []byte(`{"chosen_products":[{"product_id":"1","quantity":1}]}`)
	response := []byte("HTTP/2 200 OK\r\nContent-Type: application/json\r\n\r\n{\"chosen_products\":[],\"chosen_discount\":{\"percentage\":0}}")

	report, err := HarvestJSONResponse(request, response, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Actionable) != 1 {
		t.Fatalf("actionable=%+v report=%+v", report.Actionable, report)
	}
	candidate := report.Actionable[0]
	if candidate.Name != "chosen_discount" || candidate.JSONParent != "$" || candidate.JSONPath() != "$.chosen_discount" {
		t.Fatalf("candidate=%+v", candidate)
	}
	if len(candidate.Sources) != 1 || candidate.Sources[0].ObservedType != "object" || candidate.Sources[0].Path != "$.chosen_discount" {
		t.Fatalf("sources=%+v", candidate.Sources)
	}
	if report.SkippedNoParent != 1 {
		t.Fatalf("expected nested percentage to remain skipped from active testing: %+v", report)
	}
	if len(report.Scaffoldable) != 1 {
		t.Fatalf("scaffoldable=%+v report=%+v", report.Scaffoldable, report)
	}
	scaffold := report.Scaffoldable[0]
	if scaffold.JSONPath() != "$.chosen_discount.percentage" || scaffold.JSONScaffoldParent != "$.chosen_discount" || !scaffold.RequiresJSONScaffold() {
		t.Fatalf("scaffold candidate=%+v", scaffold)
	}
	if len(scaffold.Sources) != 1 || scaffold.Sources[0].Source != "context_response_scaffoldable_json_property" || scaffold.Sources[0].ObservedType != "integer" {
		t.Fatalf("scaffold sources=%+v", scaffold.Sources)
	}
}

func TestHarvestNestedPropertyWhenParentExists(t *testing.T) {
	request := []byte(`{"filters":{"status":"active"}}`)
	response := []byte(`{"filters":{"status":"active","limit":20}}`)

	report, err := HarvestJSONResponse(request, response, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Actionable) != 1 {
		t.Fatalf("actionable=%+v", report.Actionable)
	}
	candidate := report.Actionable[0]
	if candidate.JSONPath() != "$.filters.limit" || candidate.Sources[0].ObservedType != "integer" {
		t.Fatalf("candidate=%+v", candidate)
	}
	if candidate.RequiresJSONScaffold() || len(report.Scaffoldable) != 0 {
		t.Fatalf("existing parent must not require scaffolding: %+v", report)
	}
}

func TestHarvestClassifiesExactlyOneMissingObjectLevel(t *testing.T) {
	request := []byte(`{"profile":{"name":"tobias"}}`)
	response := []byte(`{"profile":{"name":"tobias","settings":{"beta_access":false}}}`)

	report, err := HarvestJSONResponse(request, response, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Scaffoldable) != 1 {
		t.Fatalf("scaffoldable=%+v report=%+v", report.Scaffoldable, report)
	}
	candidate := report.Scaffoldable[0]
	if candidate.JSONPath() != "$.profile.settings.beta_access" || candidate.JSONScaffoldParent != "$.profile.settings" {
		t.Fatalf("candidate=%+v", candidate)
	}
}

func TestHarvestDoesNotClassifyLeafBehindTwoMissingParents(t *testing.T) {
	request := []byte(`{"name":"tobias"}`)
	response := []byte(`{"name":"tobias","profile":{"settings":{"beta_access":false}}}`)

	report, err := HarvestJSONResponse(request, response, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range report.Scaffoldable {
		if candidate.JSONPath() == "$.profile.settings.beta_access" {
			t.Fatalf("two-level missing leaf must not be scaffoldable: %+v", candidate)
		}
	}
}

func TestHarvestDoesNotReplaceExistingNonObjectParent(t *testing.T) {
	for _, request := range []string{
		`{"settings":null}`,
		`{"settings":"disabled"}`,
		`{"settings":[]}`,
	} {
		report, err := HarvestJSONResponse([]byte(request), []byte(`{"settings":{"beta_access":false}}`), 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Scaffoldable) != 0 {
			t.Fatalf("existing non-object parent must never be scaffolded: request=%s report=%+v", request, report)
		}
	}
}

func TestHarvestDoesNotTokenizeJSONValues(t *testing.T) {
	request := []byte(`{"profile":{"name":"tobias"}}`)
	response := []byte(`{"profile":{"name":"admin debug role chosen_discount"}}`)

	report, err := HarvestJSONResponse(request, response, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Actionable) != 0 || len(report.Scaffoldable) != 0 {
		t.Fatalf("value text must never become candidates: %+v", report)
	}
}

func TestHarvestRejectsNonJSONObjectContext(t *testing.T) {
	if _, err := HarvestJSONResponse([]byte(`{"x":1}`), []byte(`[]`), 3); err == nil {
		t.Fatal("expected JSON object root error")
	}
}
