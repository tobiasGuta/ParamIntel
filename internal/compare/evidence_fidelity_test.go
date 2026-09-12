package compare

import (
	"net/http"
	"strings"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestDynamicHTMLSameSizeStructuralChangeBecomesEvidence(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"text/html"}}
	// Baseline text changes while the tag structure remains stable, so the old
	// exact-body comparison is unavailable. All samples remain similar in size.
	a := Snapshot(200, headers, []byte(`<html><body><main><p>request-a</p></main></body></html>`))
	b := Snapshot(200, headers, []byte(`<html><body><main><p>request-b</p></main></body></html>`))
	p := BuildBaseline([]model.Snapshot{a, b})
	if p.StableBody != "" {
		t.Fatal("test requires a dynamic baseline body")
	}
	if !p.HTMLStructureStable {
		t.Fatal("test requires stable baseline HTML structure")
	}

	probe := Snapshot(200, headers, []byte(`<html><body><main><b>request-c</b></main></body></html>`))
	got := AgainstBaseline(p, probe)
	if !got.Meaningful {
		t.Fatal("same-size structural change should be meaningful")
	}
	if !hasDifference(got.Differences, "html_structure_changed", "") {
		t.Fatalf("missing HTML structure evidence: %+v", got.Differences)
	}
	if hasDifference(got.Differences, "body_length", "") {
		t.Fatalf("test must prove structure detection rather than body length: %+v", got.Differences)
	}
}

func TestHeaderOnlyBehaviorCanBeEvidence(t *testing.T) {
	baselineHeaders := http.Header{"Content-Type": []string{"text/plain"}}
	a := Snapshot(200, baselineHeaders, []byte("same body"))
	b := Snapshot(200, baselineHeaders, []byte("same body"))
	p := BuildBaseline([]model.Snapshot{a, b})

	probeHeaders := baselineHeaders.Clone()
	probeHeaders.Set("X-Debug-Mode", "enabled")
	probe := Snapshot(200, probeHeaders, []byte("same body"))
	got := AgainstBaseline(p, probe)
	if !got.Meaningful {
		t.Fatal("new eligible response header should be meaningful")
	}
	if !hasDifference(got.Differences, "header_added", "X-Debug-Mode") {
		t.Fatalf("missing header-added evidence: %+v", got.Differences)
	}
	if hasDifference(got.Differences, "body_changed", "") || hasDifference(got.Differences, "body_length", "") {
		t.Fatalf("header-only behavior should not require body evidence: %+v", got.Differences)
	}
}

func TestStableHeaderChangeAndRemovalAreEvidenceWithoutRawValues(t *testing.T) {
	headers := http.Header{
		"Content-Type":   []string{"text/plain"},
		"X-Feature-Mode": []string{"normal"},
		"Location":       []string{"/dashboard"},
	}
	p := BuildBaseline([]model.Snapshot{
		Snapshot(200, headers, []byte("body-a")),
		Snapshot(200, headers, []byte("body-b")),
	})

	probeHeaders := http.Header{
		"Content-Type":   []string{"text/plain"},
		"X-Feature-Mode": []string{"preview"},
	}
	got := AgainstBaseline(p, Snapshot(200, probeHeaders, []byte("body-c")))
	if !hasDifference(got.Differences, "header_value_changed", "X-Feature-Mode") {
		t.Fatalf("missing changed-header evidence: %+v", got.Differences)
	}
	if !hasDifference(got.Differences, "header_removed", "Location") {
		t.Fatalf("missing removed-header evidence: %+v", got.Differences)
	}
	for _, d := range got.Differences {
		if strings.HasPrefix(d.Kind, "header_") && (d.Before != "" || d.After != "") {
			t.Fatalf("header evidence must not expose raw or hashed values: %+v", d)
		}
	}
}

func TestUnstableAndIgnoredHeadersDoNotBecomeEvidence(t *testing.T) {
	aHeaders := http.Header{
		"Content-Type": []string{"text/plain"},
		"X-Mode":       []string{"a"},
		"X-Request-Id": []string{"request-a"},
	}
	bHeaders := http.Header{
		"Content-Type": []string{"text/plain"},
		"X-Mode":       []string{"b"},
		"X-Request-Id": []string{"request-b"},
	}
	p := BuildBaseline([]model.Snapshot{
		Snapshot(200, aHeaders, []byte("body-a")),
		Snapshot(200, bHeaders, []byte("body-b")),
	})

	probeHeaders := http.Header{
		"Content-Type": []string{"text/plain"},
		"X-Mode":       []string{"c"},
		"X-Request-Id": []string{"request-c"},
	}
	got := AgainstBaseline(p, Snapshot(200, probeHeaders, []byte("body-c")))
	for _, d := range got.Differences {
		if strings.HasPrefix(d.Kind, "header_") {
			t.Fatalf("unstable/ignored headers must not become evidence: %+v", got.Differences)
		}
	}
}

func TestStableContentTypeChangeIsEvidence(t *testing.T) {
	textHeaders := http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}}
	p := BuildBaseline([]model.Snapshot{
		Snapshot(200, textHeaders, []byte("alpha")),
		Snapshot(200, textHeaders, []byte("bravo")),
	})
	probeHeaders := http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}
	got := AgainstBaseline(p, Snapshot(200, probeHeaders, []byte("charl")))
	if !hasDifference(got.Differences, "content_type", "") {
		t.Fatalf("missing content-type evidence: %+v", got.Differences)
	}
	for _, d := range got.Differences {
		if d.Kind == "content_type" && (d.Before != "text/plain" || d.After != "text/html") {
			t.Fatalf("content type must use normalized media types: %+v", d)
		}
	}
}

func TestTextMetricToleranceRejectsSmallNoiseAndDetectsLargeShift(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"text/plain"}}
	a := Snapshot(200, headers, []byte("one two three\nfour five\n"))
	b := Snapshot(200, headers, []byte("one two three six\nfour five\n"))
	p := BuildBaseline([]model.Snapshot{a, b})

	withinNoise := Snapshot(200, headers, []byte("one two three seven eight\nfour five\nextra\n"))
	got := AgainstBaseline(p, withinNoise)
	if hasDifference(got.Differences, "line_count", "") || hasDifference(got.Differences, "word_count", "") {
		t.Fatalf("small metric movement should stay inside tolerance: %+v", got.Differences)
	}

	largeShift := Snapshot(200, headers, []byte("one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty\nline two\nline three\nline four\nline five\nline six\n"))
	got = AgainstBaseline(p, largeShift)
	if !hasDifference(got.Differences, "line_count", "") {
		t.Fatalf("large line-count shift not detected: %+v", got.Differences)
	}
	if !hasDifference(got.Differences, "word_count", "") {
		t.Fatalf("large word-count shift not detected: %+v", got.Differences)
	}
}

func TestJSONPathEvidenceRemainsAvailable(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"application/json"}}
	a := Snapshot(200, headers, []byte(`{"request_id":"a","role":"member"}`))
	b := Snapshot(200, headers, []byte(`{"request_id":"b","role":"member"}`))
	p := BuildBaseline([]model.Snapshot{a, b})
	got := AgainstBaseline(p, Snapshot(200, headers, []byte(`{"request_id":"c","role":"admin"}`)))
	if !hasDifference(got.Differences, "json_value_changed", "$.role") {
		t.Fatalf("existing JSON semantic evidence regressed: %+v", got.Differences)
	}
	if hasDifference(got.Differences, "word_count", "") || hasDifference(got.Differences, "line_count", "") || hasDifference(got.Differences, "html_structure_changed", "") {
		t.Fatalf("non-JSON body metrics must not replace JSON semantics: %+v", got.Differences)
	}
}

func hasDifference(diffs []model.Difference, kind, path string) bool {
	for _, d := range diffs {
		if d.Kind == kind && (path == "" || d.Path == path) {
			return true
		}
	}
	return false
}
