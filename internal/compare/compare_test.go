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

func TestSnapshotRecordsResponseFeaturesWithoutChangingComparator(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}
	body := []byte("<html><body><main><p>member</p></main></body></html>")
	s := Snapshot(200, headers, body)

	if s.Features.ContentType != "text/html" {
		t.Fatalf("content type=%q want=text/html", s.Features.ContentType)
	}
	if !s.Features.IsText || !s.Features.IsHTML || s.Features.HTMLStructureHash == "" {
		t.Fatalf("missing HTML response features: %+v", s.Features)
	}
	if s.Features.HTMLElementCount != 4 {
		t.Fatalf("element count=%d want=4", s.Features.HTMLElementCount)
	}

	p := BuildBaseline([]model.Snapshot{s, s})
	changedStructureSameLength := Snapshot(200, headers, []byte("<html><body><main><div>member</div></main></body></html>"))
	got := AgainstBaseline(p, changedStructureSameLength)
	if !got.Meaningful {
		// The baseline body is byte-stable here, so the pre-v0.7 body hash still
		// detects the change. Slice 2 must not depend on the new structure hash.
		t.Fatal("existing comparator behavior changed unexpectedly")
	}
	for _, d := range got.Differences {
		if d.Kind == "html_structure_changed" {
			t.Fatal("slice 2 must not promote HTML structure into evidence yet")
		}
	}
}

func TestBuildBaselineTracksStableHTMLFeatureRanges(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}
	a := Snapshot(200, headers, []byte("<html><body><main><p>one two</p></main></body></html>"))
	b := Snapshot(200, headers, []byte("<html>\n<body><main><p>three four five</p></main></body>\n</html>"))
	p := BuildBaseline([]model.Snapshot{a, b})

	if !p.ContentTypeStable || p.ContentType != "text/html" {
		t.Fatalf("content type baseline not stable: %+v", p)
	}
	if !p.TextClassStable || !p.IsText {
		t.Fatalf("text class baseline not stable: %+v", p)
	}
	if !p.HTMLClassStable || !p.IsHTML {
		t.Fatalf("HTML class baseline not stable: %+v", p)
	}
	if !p.HTMLStructureStable || p.HTMLStructureHash == "" {
		t.Fatalf("same HTML structure should be stable: %+v", p)
	}
	if p.HTMLElementMin != 4 || p.HTMLElementMax != 4 {
		t.Fatalf("element range=%d..%d want=4..4", p.HTMLElementMin, p.HTMLElementMax)
	}
	if p.LineCountMin != 1 || p.LineCountMax != 3 {
		t.Fatalf("line range=%d..%d want=1..3", p.LineCountMin, p.LineCountMax)
	}
	if p.WordCountMin >= p.WordCountMax {
		t.Fatalf("expected varying word-count range, got %d..%d", p.WordCountMin, p.WordCountMax)
	}
}

func TestBuildBaselineMarksUnstableFeatureClasses(t *testing.T) {
	text := Snapshot(200, http.Header{"Content-Type": []string{"text/plain"}}, []byte("hello"))
	binary := Snapshot(200, http.Header{"Content-Type": []string{"application/octet-stream"}}, []byte{0x00, 0x01})
	p := BuildBaseline([]model.Snapshot{text, binary})

	if p.ContentTypeStable {
		t.Fatal("mixed content types must be unstable")
	}
	if p.TextClassStable {
		t.Fatal("text/binary classification must be unstable")
	}
	if p.HTMLClassStable == false {
		t.Fatal("both samples are consistently non-HTML")
	}
	if p.HTMLStructureStable {
		t.Fatal("non-HTML baseline must not have stable HTML structure evidence")
	}
}

func TestBuildBaselineMarksChangedHTMLStructureUnstable(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"text/html"}}
	a := Snapshot(200, headers, []byte("<html><body><main><p>member</p></main></body></html>"))
	b := Snapshot(200, headers, []byte("<html><body><main><p>member</p><aside>rotating</aside></main></body></html>"))
	p := BuildBaseline([]model.Snapshot{a, b})

	if !p.HTMLClassStable || !p.IsHTML {
		t.Fatal("both responses should remain classified as HTML")
	}
	if p.HTMLStructureStable {
		t.Fatal("different baseline HTML structures must not become trusted evidence")
	}
	if p.HTMLElementMin != 4 || p.HTMLElementMax != 5 {
		t.Fatalf("element range=%d..%d want=4..5", p.HTMLElementMin, p.HTMLElementMax)
	}
}
