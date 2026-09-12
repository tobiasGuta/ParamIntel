package responsefeatures

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

// Extract derives deterministic observations from one response. These are raw
// observations only: later baseline logic decides which features are stable
// enough to become discovery evidence.
func Extract(headers http.Header, body []byte) model.ResponseFeatures {
	contentType := normalizeContentType(headers.Get("Content-Type"))
	isText := isTextual(contentType, body)
	isHTML := isHTMLContent(contentType, body)

	features := model.ResponseFeatures{
		ContentType: contentType,
		IsHTML:      isHTML,
	}
	if isText {
		features.LineCount = countLines(body)
		features.WordCount = len(strings.Fields(string(body)))
	}
	if isHTML {
		features.HTMLStructureHash, features.HTMLElementCount = htmlStructure(body)
	}
	return features
}

func normalizeContentType(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if mediaType, _, err := mime.ParseMediaType(raw); err == nil {
		return strings.ToLower(mediaType)
	}
	if i := strings.IndexByte(raw, ';'); i >= 0 {
		raw = raw[:i]
	}
	return strings.ToLower(strings.TrimSpace(raw))
}

func isTextual(contentType string, body []byte) bool {
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	switch contentType {
	case "application/json", "application/xml", "application/xhtml+xml", "application/javascript", "application/x-javascript", "application/x-www-form-urlencoded":
		return true
	}
	if strings.HasSuffix(contentType, "+json") || strings.HasSuffix(contentType, "+xml") {
		return true
	}
	if contentType != "" {
		return false
	}
	return utf8.Valid(body) && !bytes.ContainsRune(body, '\x00')
}

func isHTMLContent(contentType string, body []byte) bool {
	if contentType == "text/html" || contentType == "application/xhtml+xml" {
		return true
	}
	if contentType != "" {
		return false
	}
	trimmed := strings.ToLower(strings.TrimSpace(string(body)))
	for _, prefix := range []string{"<!doctype html", "<html", "<head", "<body"} {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func countLines(body []byte) int {
	if len(body) == 0 {
		return 0
	}
	lines := bytes.Count(body, []byte{'\n'})
	if body[len(body)-1] != '\n' {
		lines++
	}
	return lines
}

// htmlStructure hashes only normalized tag boundaries. Text, comments,
// attribute names/values, and raw script/style/textarea/title contents are
// deliberately excluded so rotating application data does not alter the
// structural fingerprint.
func htmlStructure(body []byte) (string, int) {
	lower := bytes.ToLower(body)
	h := sha256.New()
	elements := 0

	for i := 0; i < len(body); {
		rel := bytes.IndexByte(body[i:], '<')
		if rel < 0 {
			break
		}
		i += rel

		if bytes.HasPrefix(lower[i:], []byte("<!--")) {
			end := bytes.Index(lower[i+4:], []byte("-->"))
			if end < 0 {
				break
			}
			i += 4 + end + 3
			continue
		}

		tag, ok := parseTag(body, i)
		if !ok {
			i++
			continue
		}
		i = tag.end
		if tag.declaration {
			continue
		}

		if tag.closing {
			_, _ = h.Write([]byte("</" + tag.name + ">"))
			continue
		}

		_, _ = h.Write([]byte("<" + tag.name + ">"))
		elements++

		if !isRawTextElement(tag.name) {
			continue
		}
		needle := []byte("</" + tag.name)
		relClose := bytes.Index(lower[i:], needle)
		if relClose < 0 {
			continue
		}
		closeStart := i + relClose
		closeTag, ok := parseTag(body, closeStart)
		if !ok || !closeTag.closing || closeTag.name != tag.name {
			continue
		}
		_, _ = h.Write([]byte("</" + tag.name + ">"))
		i = closeTag.end
	}

	if elements == 0 {
		return "", 0
	}
	return hex.EncodeToString(h.Sum(nil)), elements
}

type parsedTag struct {
	name        string
	closing     bool
	declaration bool
	end         int
}

func parseTag(body []byte, start int) (parsedTag, bool) {
	if start < 0 || start >= len(body) || body[start] != '<' {
		return parsedTag{}, false
	}
	p := start + 1
	for p < len(body) && isSpace(body[p]) {
		p++
	}
	if p >= len(body) {
		return parsedTag{}, false
	}

	if body[p] == '!' || body[p] == '?' {
		end, ok := tagEnd(body, p+1)
		return parsedTag{declaration: true, end: end}, ok
	}

	closing := false
	if body[p] == '/' {
		closing = true
		p++
		for p < len(body) && isSpace(body[p]) {
			p++
		}
	}
	nameStart := p
	for p < len(body) && isTagNameByte(body[p]) {
		p++
	}
	if p == nameStart {
		return parsedTag{}, false
	}
	name := strings.ToLower(string(body[nameStart:p]))
	end, ok := tagEnd(body, p)
	if !ok {
		return parsedTag{}, false
	}
	return parsedTag{name: name, closing: closing, end: end}, true
}

func tagEnd(body []byte, start int) (int, bool) {
	var quote byte
	for i := start; i < len(body); i++ {
		b := body[i]
		if quote != 0 {
			if b == quote {
				quote = 0
			}
			continue
		}
		switch b {
		case '\'', '"':
			quote = b
		case '>':
			return i + 1, true
		}
	}
	return 0, false
}

func isRawTextElement(name string) bool {
	switch name {
	case "script", "style", "textarea", "title":
		return true
	default:
		return false
	}
}

func isTagNameByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == ':' || b == '-' || b == '_'
}

func isSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	default:
		return false
	}
}
