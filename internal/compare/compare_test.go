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
	if !hasDifference(got, "json_value_changed", "$.user.role") || !hasDifference(got, "json_path_added", "$.internal") {
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

func TestStableHTMLStructureBecomesEvidence(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}
	a := Snapshot(200, headers, []byte("<html><body><main><p>request-a</p></main></body></html>"))
	b := Snapshot(200, headers, []byte("<html><body><main><p>request-b</p></main></body></html>"))
	p := BuildBaseline([]model.Snapshot{a, b})
	if !p.HTMLStructureStable {
		t.Fatal("baseline HTML structure should be stable")
	}

	probe := Snapshot(200, headers, []byte("<html><body><main><div>request-c</div></main></body></html>"))
	got := AgainstBaseline(p, probe)
	if !hasDifference(got, "html_structure_changed", "") {
		t.Fatalf("missing structural evidence: %+v", got.Differences)
	}
}

func TestDynamicHTMLTextWithStableStructureRemainsNoise(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"text/html"}}
	a := Snapshot(200, headers, []byte("<html><body><p>request-1111</p></body></html>"))
	b := Snapshot(200, headers, []byte("<html><body><p>request-2222</p></body></html>"))
	p := BuildBaseline([]model.Snapshot{a, b})

	probe := Snapshot(200, headers, []byte("<html><body><p>request-3333</p></body></html>"))
	got := AgainstBaseline(p, probe)
	if got.Meaningful {
		t.Fatalf("dynamic text with unchanged structure should remain noise: %+v", got.Differences)
	}
}

func TestStableHTMLLossBecomesEvidenceWithoutContentType(t *testing.T) {
	a := Snapshot(200, nil, []byte("<html><body><p>request-a</p></body></html>"))
	b := Snapshot(200, nil, []byte("<html><body><p>request-b</p></body></html>"))
	p := BuildBaseline([]model.Snapshot{a, b})
	if !p.HTMLStructureStable {
		t.Fatal("expected sniffed HTML baseline to be structurally stable")
	}

	probe := Snapshot(200, nil, []byte("plain response with roughly similar size here"))
	got := AgainstBaseline(p, probe)
	if !hasDifference(got, "html_structure_changed", "") {
		t.Fatalf("loss of stable HTML class should be evidence: %+v", got.Differences)
	}
}

func TestResponseHeaderAddedCanBeSoleEvidence(t *testing.T) {
	baseHeaders := http.Header{"Content-Type": []string{"text/plain"}}
	a := Snapshot(200, baseHeaders, []byte("same body"))
	b := Snapshot(200, baseHeaders, []byte("same body"))
	p := BuildBaseline([]model.Snapshot{a, b})

	probeHeaders := baseHeaders.Clone()
	probeHeaders.Set("X-Debug-Mode", "enabled")
	got := AgainstBaseline(p, Snapshot(200, probeHeaders, []byte("same body")))
	if !hasDifference(got, "header_added", "X-Debug-Mode") {
		t.Fatalf("missing header-added evidence: %+v", got.Differences)
	}
}

func TestStableResponseHeaderValueChangeIsEvidenceWithoutLeakingValue(t *testing.T) {
	headers := http.Header{
		"Content-Type":   []string{"text/plain"},
		"X-Feature-Mode": []string{"normal"},
	}
	p := BuildBaseline([]model.Snapshot{
		Snapshot(200, headers, []byte("same body")),
		Snapshot(200, headers, []byte("same body")),
	})

	probeHeaders := headers.Clone()
	probeHeaders.Set("X-Feature-Mode", "admin-secret-mode")
	got := AgainstBaseline(p, Snapshot(200, probeHeaders, []byte("same body")))
	if !hasDifference(got, "header_value_changed", "X-Feature-Mode") {
		t.Fatalf("missing header-value evidence: %+v", got.Differences)
	}
	for _, d := range got.Differences {
		if d.Path == "X-Feature-Mode" && (d.Before != "" || d.After != "") {
			t.Fatalf("raw or hashed header values must not be emitted: %+v", d)
		}
	}
}

func TestIgnoredNoisyHeaderNeverBecomesEvidence(t *testing.T) {
	aHeaders := http.Header{"Content-Type": []string{"text/plain"}, "X-Request-Id": []string{"a"}}
	bHeaders := http.Header{"Content-Type": []string{"text/plain"}, "X-Request-Id": []string{"b"}}
	p := BuildBaseline([]model.Snapshot{
		Snapshot(200, aHeaders, []byte("same body")),
		Snapshot(200, bHeaders, []byte("same body")),
	})

	probeHeaders := http.Header{"Content-Type": []string{"text/plain"}, "X-Request-Id": []string{"c"}}
	got := AgainstBaseline(p, Snapshot(200, probeHeaders, []byte("same body")))
	if got.Meaningful {
		t.Fatalf("ignored request-id churn became evidence: %+v", got.Differences)
	}
}

func TestStableContentTypeChangeIsEvidence(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}}
	p := BuildBaseline([]model.Snapshot{
		Snapshot(200, headers, []byte("same")),
		Snapshot(200, headers, []byte("same")),
	})

	probeHeaders := http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}
	got := AgainstBaseline(p, Snapshot(200, probeHeaders, []byte("same")))
	if !hasDifference(got, "content_type", "") {
		t.Fatalf("missing content-type evidence: %+v", got.Differences)
	}
}

func TestTextMetricOutlierIsEvidenceForDynamicPlainText(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"text/plain"}}
	a := Snapshot(200, headers, []byte("request a\nstatus normal"))
	b := Snapshot(200, headers, []byte("request b\nstatus normal now"))
	p := BuildBaseline([]model.Snapshot{a, b})

	probe := Snapshot(200, headers, []byte("one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen"))
	got := AgainstBaseline(p, probe)
	if !hasDifference(got, "word_count", "") {
		t.Fatalf("missing text-metric evidence: %+v", got.Differences)
	}
}

func TestJSONKeepsSemanticComparisonInsteadOfTextMetricNoise(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"application/json"}}
	a := Snapshot(200, headers, []byte(`{"request_id":"a","role":"member"}`))
	b := Snapshot(200, headers, []byte(`{"request_id":"longer-request-b","role":"member"}`))
	p := BuildBaseline([]model.Snapshot{a, b})

	probe := Snapshot(200, headers, []byte(`{"request_id":"extremely-long-dynamic-request-identifier","role":"member"}`))
	got := AgainstBaseline(p, probe)
	if got.Meaningful {
		t.Fatalf("JSON dynamic value should not become text-metric evidence: %+v", got.Differences)
	}
}

func hasDifference(c model.Comparison, kind, path string) bool {
	for _, d := range c.Differences {
		if d.Kind == kind && (path == "" || d.Path == path) {
			return true
		}
	}
	return false
}
