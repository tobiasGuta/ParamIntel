package compare

import (
	"net/http"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestDynamicJSONIgnoredButStableSemanticChangeDetected(t *testing.T) {
	a := Snapshot(200, nil, []byte(`{"request_id":"a","user":{"role":"member"},"items":[1,2]}`))
	b := Snapshot(200, nil, []byte(`{"request_id":"b","user":{"role":"member"},"items":[1,2]}`))
	p := BuildBaseline([]model.Snapshot{a, b})

	dynamicOnly := Snapshot(200, nil, []byte(`{"request_id":"c","user":{"role":"member"},"items":[1,2]}`))
	if got := AgainstBaseline(p, dynamicOnly); got.Meaningful {
		t.Fatalf("dynamic-only response marked meaningful: %+v", got)
	}

	changed := Snapshot(200, nil, []byte(`{"request_id":"d","user":{"role":"admin"},"items":[1,2],"internal":true}`))
	got := AgainstBaseline(p, changed)
	if !got.Meaningful {
		t.Fatal("expected semantic change")
	}
	var roleChanged, pathAdded bool
	for _, d := range got.Differences {
		if d.Kind == "json_value_changed" && d.Path == "$.user.role" {
			roleChanged = true
		}
		if d.Kind == "json_path_added" && d.Path == "$.internal" {
			pathAdded = true
		}
	}
	if !roleChanged || !pathAdded {
		t.Fatalf("missing expected evidence: %+v", got.Differences)
	}
}

func TestStableTextBodyChange(t *testing.T) {
	a := Snapshot(200, nil, []byte("same"))
	b := Snapshot(200, nil, []byte("same"))
	p := BuildBaseline([]model.Snapshot{a, b})
	if AgainstBaseline(p, Snapshot(200, nil, []byte("same"))).Meaningful {
		t.Fatal("same body changed")
	}
	if !AgainstBaseline(p, Snapshot(200, nil, []byte("different"))).Meaningful {
		t.Fatal("different body not detected")
	}
}

func TestUnstableBaselineStatusIsIgnored(t *testing.T) {
	a := Snapshot(200, nil, []byte(`{"ok":true}`))
	b := Snapshot(204, nil, []byte(`{"ok":true}`))
	p := BuildBaseline([]model.Snapshot{a, b})
	if p.StatusStable {
		t.Fatal("status should be marked unstable")
	}
	got := AgainstBaseline(p, Snapshot(201, nil, []byte(`{"ok":true}`)))
	if got.Meaningful {
		t.Fatalf("unstable status alone should not be evidence: %+v", got)
	}
}

func TestSnapshotRecordsResponseFeaturesAndComparatorUsesStableStructure(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}
	a := Snapshot(200, headers, []byte("<html><body><main><p>request-a</p></main></body></html>"))
	b := Snapshot(200, headers, []byte("<html><body><main><p>request-b</p></main></body></html>"))

	if a.Features.ContentType != "text/html" {
		t.Fatalf("content type=%q want=text/html", a.Features.ContentType)
	}
	if !a.Features.IsHTML || a.Features.HTMLStructureHash == "" {
		t.Fatalf("missing HTML response features: %+v", a.Features)
	}
	if a.Features.HTMLElementCount != 4 {
		t.Fatalf("element count=%d want=4", a.Features.HTMLElementCount)
	}

	p := BuildBaseline([]model.Snapshot{a, b})
	if p.StableBody != "" {
		t.Fatal("test requires a dynamic baseline body")
	}
	changedStructure := Snapshot(200, headers, []byte("<html><body><main><div>request-c</div></main></body></html>"))
	got := AgainstBaseline(p, changedStructure)
	found := false
	for _, d := range got.Differences {
		if d.Kind == "html_structure_changed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("stable HTML structure change was not promoted to evidence: %+v", got.Differences)
	}
}
