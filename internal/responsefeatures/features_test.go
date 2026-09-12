package responsefeatures

import (
	"net/http"
	"testing"
)

func TestExtractNormalizesContentTypeAndCountsText(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"Text/Plain; charset=UTF-8"}}
	got := Extract(headers, []byte("one two\nthree\n"))

	if got.ContentType != "text/plain" {
		t.Fatalf("content type=%q want=text/plain", got.ContentType)
	}
	if !got.IsText {
		t.Fatal("plain text must be classified as textual")
	}
	if got.LineCount != 2 {
		t.Fatalf("line count=%d want=2", got.LineCount)
	}
	if got.WordCount != 3 {
		t.Fatalf("word count=%d want=3", got.WordCount)
	}
	if got.IsHTML {
		t.Fatal("plain text must not be classified as HTML")
	}
	if got.HTMLStructureHash != "" || got.HTMLElementCount != 0 {
		t.Fatalf("plain text unexpectedly has HTML structure: %+v", got)
	}
}

func TestHTMLStructureIgnoresTextAttributesCommentsAndRawText(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}
	bodyA := []byte(`<!doctype html>
<html><body><!-- first --><div class="alpha">hello<span data-id="1">one</span></div><script>const x = "<section>noise</section>";</script></body></html>`)
	bodyB := []byte(`<!DOCTYPE HTML>
<HTML><BODY><!-- second --><DIV class="beta" data-random="rotating">goodbye<SPAN data-id="999">two words</SPAN></DIV><SCRIPT>const y = "<aside>different</aside>";</SCRIPT></BODY></HTML>`)

	a := Extract(headers, bodyA)
	b := Extract(headers, bodyB)

	if !a.IsText || !b.IsText {
		t.Fatal("HTML must also be classified as textual")
	}
	if !a.IsHTML || !b.IsHTML {
		t.Fatalf("expected HTML classification: a=%t b=%t", a.IsHTML, b.IsHTML)
	}
	if a.HTMLElementCount != 5 || b.HTMLElementCount != 5 {
		t.Fatalf("element counts=%d,%d want=5,5", a.HTMLElementCount, b.HTMLElementCount)
	}
	if a.HTMLStructureHash == "" || b.HTMLStructureHash == "" {
		t.Fatal("expected non-empty HTML structure hashes")
	}
	if a.HTMLStructureHash != b.HTMLStructureHash {
		t.Fatalf("text/attribute/raw-text noise changed structure hash:\n%s\n%s", a.HTMLStructureHash, b.HTMLStructureHash)
	}
}

func TestHTMLStructureDetectsElementChange(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"text/html"}}
	base := Extract(headers, []byte(`<html><body><main><p>member</p></main></body></html>`))
	changed := Extract(headers, []byte(`<html><body><main><p>member</p><aside>debug</aside></main></body></html>`))

	if base.HTMLStructureHash == changed.HTMLStructureHash {
		t.Fatal("added element must change HTML structure hash")
	}
	if changed.HTMLElementCount != base.HTMLElementCount+1 {
		t.Fatalf("element count base=%d changed=%d", base.HTMLElementCount, changed.HTMLElementCount)
	}
}

func TestExtractCanConservativelySniffHTMLWithoutContentType(t *testing.T) {
	got := Extract(nil, []byte("  <!doctype html><html><body><p>hello</p></body></html>"))
	if !got.IsText || !got.IsHTML {
		t.Fatalf("expected textual HTML sniff, got %+v", got)
	}
	if got.HTMLStructureHash == "" {
		t.Fatal("expected HTML structure hash")
	}
}

func TestExplicitNonHTMLContentTypeWinsOverMarkupLookingBody(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"text/plain"}}
	got := Extract(headers, []byte(`<html><body>shown as source</body></html>`))
	if !got.IsText {
		t.Fatal("text/plain must remain textual")
	}
	if got.IsHTML {
		t.Fatal("explicit text/plain response must not be treated as HTML")
	}
	if got.HTMLStructureHash != "" {
		t.Fatal("non-HTML response must not have a structure hash")
	}
}

func TestBinaryContentDoesNotProduceTextCounts(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"application/octet-stream"}}
	got := Extract(headers, []byte{0x00, 0x01, 0x02, '\n', 'x'})
	if got.IsText {
		t.Fatal("binary response must not be classified as textual")
	}
	if got.LineCount != 0 || got.WordCount != 0 {
		t.Fatalf("binary response unexpectedly got text counts: %+v", got)
	}
}

func TestEmptyTextAndBinaryRemainDistinguishable(t *testing.T) {
	emptyText := Extract(http.Header{"Content-Type": []string{"text/plain"}}, nil)
	emptyBinary := Extract(http.Header{"Content-Type": []string{"application/octet-stream"}}, nil)
	if !emptyText.IsText || emptyBinary.IsText {
		t.Fatalf("text/binary classification collapsed: text=%+v binary=%+v", emptyText, emptyBinary)
	}
	if emptyText.LineCount != 0 || emptyBinary.LineCount != 0 {
		t.Fatal("empty bodies should both have zero counts; IsText carries the distinction")
	}
}
