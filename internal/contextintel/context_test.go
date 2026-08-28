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
		t.Fatalf("expected nested percentage to be skipped because its parent is absent: %+v", report)
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
}

func TestHarvestDoesNotTokenizeJSONValues(t *testing.T) {
	request := []byte(`{"profile":{"name":"tobias"}}`)
	response := []byte(`{"profile":{"name":"admin debug role chosen_discount"}}`)

	report, err := HarvestJSONResponse(request, response, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Actionable) != 0 {
		t.Fatalf("value text must never become candidates: %+v", report.Actionable)
	}
}

func TestHarvestRejectsNonJSONObjectContext(t *testing.T) {
	if _, err := HarvestJSONResponse([]byte(`{"x":1}`), []byte(`[]`), 3); err == nil {
		t.Fatal("expected JSON object root error")
	}
}
