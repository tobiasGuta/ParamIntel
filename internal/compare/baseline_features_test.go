package compare

import (
	"net/http"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestBuildBaselineLearnsStableResponseFeatures(t *testing.T) {
	headersA := http.Header{
		"Content-Type":    []string{"text/html; charset=utf-8"},
		"X-Feature-Mode":  []string{"preview"},
		"X-Capabilities": []string{"export", "admin"},
		"Location":        []string{"/dashboard"},
		"X-Request-Id":    []string{"request-a"},
		"Set-Cookie":      []string{"session=secret-a"},
	}
	headersB := http.Header{
		"Content-Type":    []string{"TEXT/HTML; charset=iso-8859-1"},
		"X-Feature-Mode":  []string{"preview"},
		"X-Capabilities": []string{"admin", "export"},
		"Location":        []string{"/dashboard"},
		"X-Request-Id":    []string{"request-b"},
		"Set-Cookie":      []string{"session=secret-b"},
	}
	bodyA := []byte("<html>\n<body>\n<main>\n<p class=\"alpha\">member</p>\n</main>\n</body>\n</html>")
	bodyB := []byte("<HTML>\n<BODY>\n<MAIN>\n<P class=\"beta\">member account</P>\n</MAIN>\n</BODY>\n</HTML>")
	a := Snapshot(200, headersA, bodyA)
	b := Snapshot(200, headersB, bodyB)
	p := BuildBaseline([]model.Snapshot{a, b})

	if !p.ContentTypeStable || p.ContentType != "text/html" {
		t.Fatalf("content-type profile unstable/unexpected: stable=%t type=%q", p.ContentTypeStable, p.ContentType)
	}
	if !p.TextMetricsAvailable {
		t.Fatal("expected text metrics to be available")
	}
	if p.LineCountMin != minInt(a.Features.LineCount, b.Features.LineCount) || p.LineCountMax != maxInt(a.Features.LineCount, b.Features.LineCount) {
		t.Fatalf("line range=%d..%d samples=%d,%d", p.LineCountMin, p.LineCountMax, a.Features.LineCount, b.Features.LineCount)
	}
	if p.WordCountMin != minInt(a.Features.WordCount, b.Features.WordCount) || p.WordCountMax != maxInt(a.Features.WordCount, b.Features.WordCount) {
		t.Fatalf("word range=%d..%d samples=%d,%d", p.WordCountMin, p.WordCountMax, a.Features.WordCount, b.Features.WordCount)
	}
	if !p.HTMLAvailable || !p.HTMLStructureStable || p.HTMLStructureHash == "" {
		t.Fatalf("expected stable HTML structure: %+v", p)
	}
	if p.HTMLElementCountMin != 4 || p.HTMLElementCountMax != 4 {
		t.Fatalf("element range=%d..%d want=4..4", p.HTMLElementCountMin, p.HTMLElementCountMax)
	}

	for _, name := range []string{"X-Feature-Mode", "X-Capabilities", "Location"} {
		if p.StableHeaderHashes[name] == "" {
			t.Fatalf("stable header %s missing fingerprint", name)
		}
		if _, ok := p.SeenHeaderNames[name]; !ok {
			t.Fatalf("stable header %s missing from seen set", name)
		}
	}
	for _, name := range []string{"X-Request-Id", "Set-Cookie", "Content-Type"} {
		if _, ok := p.StableHeaderHashes[name]; ok {
			t.Fatalf("ignored header %s unexpectedly became stable evidence", name)
		}
		if _, ok := p.SeenHeaderNames[name]; ok {
			t.Fatalf("ignored header %s unexpectedly entered seen evidence set", name)
		}
	}
}

func TestBuildBaselineTracksUnstableHTMLStructureWithoutUsingIt(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"text/html"}}
	a := Snapshot(200, headers, []byte(`<html><body><main><p>member</p></main></body></html>`))
	b := Snapshot(200, headers, []byte(`<html><body><main><p>member</p><aside>debug</aside></main></body></html>`))
	p := BuildBaseline([]model.Snapshot{a, b})

	if !p.HTMLAvailable {
		t.Fatal("both samples are HTML, so HTML observations should be available")
	}
	if p.HTMLStructureStable {
		t.Fatal("different element structures must not be marked stable")
	}
	if p.HTMLElementCountMin != 4 || p.HTMLElementCountMax != 5 {
		t.Fatalf("element range=%d..%d want=4..5", p.HTMLElementCountMin, p.HTMLElementCountMax)
	}

	// Slice 2 only learns stability. The existing comparator must still use its
	// pre-v0.7 body logic until the differential-comparator slice lands.
	probe := Snapshot(200, headers, []byte(`<html><body><main><p>member</p><footer>info</footer></main></body></html>`))
	_ = AgainstBaseline(p, probe)
}

func TestBuildBaselineDisablesTextMetricsWhenResponseClassChanges(t *testing.T) {
	text := Snapshot(200, http.Header{"Content-Type": []string{"text/plain"}}, []byte("hello world"))
	binary := Snapshot(200, http.Header{"Content-Type": []string{"application/octet-stream"}}, []byte{0x00, 0x01, 0x02})
	p := BuildBaseline([]model.Snapshot{text, binary})

	if p.ContentTypeStable {
		t.Fatal("different media types must not be marked stable")
	}
	if p.TextMetricsAvailable {
		t.Fatal("mixed text/binary baseline must not expose text metrics as stable observations")
	}
	if p.LineCountMin != 0 || p.LineCountMax != 0 || p.WordCountMin != 0 || p.WordCountMax != 0 {
		t.Fatalf("disabled text metrics should be zeroed: %+v", p)
	}
	if p.HTMLAvailable {
		t.Fatal("mixed non-HTML baseline must not expose HTML observations")
	}
}

func TestBuildBaselineHeaderMustExistAndMatchAcrossAllSamples(t *testing.T) {
	a := Snapshot(200, http.Header{"X-Feature-Mode": []string{"normal"}}, []byte("a"))
	b := Snapshot(200, nil, []byte("b"))
	p := BuildBaseline([]model.Snapshot{a, b})

	if _, ok := p.StableHeaderHashes["X-Feature-Mode"]; ok {
		t.Fatal("header missing from one baseline sample must not be stable")
	}
	if _, ok := p.SeenHeaderNames["X-Feature-Mode"]; !ok {
		t.Fatal("eligible header should remain in seen-header set")
	}
}
