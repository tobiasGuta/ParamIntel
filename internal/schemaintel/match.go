package schemaintel

import (
	"errors"
	"fmt"
	"mime"
	"regexp"
	"strconv"
	"strings"

	v3high "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
)

var (
	ErrOperationNotFound  = errors.New("openapi operation not found")
	ErrAmbiguousOperation = errors.New("ambiguous openapi operation")
	ErrMediaTypeNotFound  = errors.New("openapi media type not found")
	ErrNonJSONMediaType   = errors.New("selected media type is not JSON")
	ErrResponseNotFound   = errors.New("openapi response not found")
)

func matchOperation(doc *Document, method, requestPath string) (*v3high.Operation, OperationMatch, error) {
	if doc == nil || doc.model == nil || doc.model.Model.Paths == nil || doc.model.Model.Paths.PathItems == nil {
		return nil, OperationMatch{}, ErrOperationNotFound
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" || requestPath == "" {
		return nil, OperationMatch{}, ErrOperationNotFound
	}

	// A concrete path is authoritative when it exactly matches the captured path.
	if item := doc.model.Model.Paths.PathItems.GetOrZero(requestPath); item != nil {
		op := operationForMethod(item, method)
		if op == nil {
			return nil, OperationMatch{}, fmt.Errorf("%w: %s %s", ErrOperationNotFound, method, requestPath)
		}
		return op, OperationMatch{SpecPath: requestPath, Method: method}, nil
	}

	type candidate struct {
		path string
		op   *v3high.Operation
	}
	var matches []candidate
	for specPath, item := range doc.model.Model.Paths.PathItems.FromOldest() {
		if item == nil || !templatePathMatches(specPath, requestPath) {
			continue
		}
		if op := operationForMethod(item, method); op != nil {
			matches = append(matches, candidate{path: specPath, op: op})
		}
	}
	if len(matches) == 0 {
		return nil, OperationMatch{}, fmt.Errorf("%w: %s %s", ErrOperationNotFound, method, requestPath)
	}
	if len(matches) > 1 {
		paths := make([]string, 0, len(matches))
		for _, match := range matches {
			paths = append(paths, match.path)
		}
		return nil, OperationMatch{}, fmt.Errorf("%w: %s %s matches %s", ErrAmbiguousOperation, method, requestPath, strings.Join(paths, ", "))
	}
	return matches[0].op, OperationMatch{SpecPath: matches[0].path, Method: method}, nil
}

func operationForMethod(item *v3high.PathItem, method string) *v3high.Operation {
	if item == nil {
		return nil
	}
	switch strings.ToUpper(method) {
	case "GET":
		return item.Get
	case "PUT":
		return item.Put
	case "POST":
		return item.Post
	case "DELETE":
		return item.Delete
	case "OPTIONS":
		return item.Options
	case "HEAD":
		return item.Head
	case "PATCH":
		return item.Patch
	case "TRACE":
		return item.Trace
	case "QUERY":
		// RFC 10008 — The HTTP QUERY Method. Requires libopenapi ≥ v0.38.7
		// which adds PathItem.Query *Operation for OpenAPI 3.2+ documents.
		// The method-authorization gate in cmd/paramintel/main.go classifies
		// QUERY as state-changing (default: requires -allow-state-changing) until
		// a separate policy review explicitly reclassifies it as safe.
		return item.Query
	default:
		return nil
	}
}

func templatePathMatches(specPath, requestPath string) bool {
	if specPath == requestPath {
		return true
	}
	var pattern strings.Builder
	pattern.WriteByte('^')
	for i := 0; i < len(specPath); {
		if specPath[i] != '{' {
			j := strings.IndexByte(specPath[i:], '{')
			if j < 0 {
				pattern.WriteString(regexp.QuoteMeta(specPath[i:]))
				break
			}
			j += i
			pattern.WriteString(regexp.QuoteMeta(specPath[i:j]))
			i = j
			continue
		}
		end := strings.IndexByte(specPath[i+1:], '}')
		if end < 0 {
			return false
		}
		end += i + 1
		name := specPath[i+1 : end]
		if name == "" || strings.ContainsAny(name, "{} /") {
			return false
		}
		pattern.WriteString(`[^/]+`)
		i = end + 1
	}
	pattern.WriteByte('$')
	rx, err := regexp.Compile(pattern.String())
	return err == nil && rx.MatchString(requestPath)
}

func selectMediaType(content *orderedmap.Map[string, *v3high.MediaType], actual string) (*v3high.MediaType, string, error) {
	actual = normalizeMediaType(actual)
	if !isJSONMediaType(actual) {
		return nil, "", fmt.Errorf("%w: %q", ErrNonJSONMediaType, actual)
	}
	if content == nil {
		return nil, "", fmt.Errorf("%w: %s", ErrMediaTypeNotFound, actual)
	}

	bestScore := -1
	var bestKey string
	var best *v3high.MediaType
	for key, media := range content.FromOldest() {
		score := mediaMatchScore(normalizeMediaType(key), actual)
		if score > bestScore {
			bestScore = score
			bestKey = key
			best = media
		}
	}
	if bestScore < 0 || best == nil {
		return nil, "", fmt.Errorf("%w: %s", ErrMediaTypeNotFound, actual)
	}
	return best, bestKey, nil
}

func normalizeMediaType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, _, err := mime.ParseMediaType(value); err == nil {
		return strings.ToLower(parsed)
	}
	if semi := strings.IndexByte(value, ';'); semi >= 0 {
		value = value[:semi]
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func isJSONMediaType(value string) bool {
	value = normalizeMediaType(value)
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 {
		return false
	}
	subtype := parts[1]
	return subtype == "json" || strings.HasSuffix(subtype, "+json")
}

func mediaMatchScore(pattern, actual string) int {
	if pattern == actual {
		return 100
	}
	p := strings.SplitN(pattern, "/", 2)
	a := strings.SplitN(actual, "/", 2)
	if len(p) != 2 || len(a) != 2 {
		return -1
	}
	if p[0] == "*" && p[1] == "*" {
		return 10
	}
	if p[0] == a[0] && p[1] == "*" {
		return 50
	}
	return -1
}

func selectResponse(responses *v3high.Responses, status int) (*v3high.Response, string, error) {
	if responses == nil {
		return nil, "", ErrResponseNotFound
	}
	exact := strconv.Itoa(status)
	if response := responses.Codes.GetOrZero(exact); response != nil {
		return response, exact, nil
	}
	rangeKey := fmt.Sprintf("%dXX", status/100)
	for key, response := range responses.Codes.FromOldest() {
		if strings.EqualFold(key, rangeKey) && response != nil {
			return response, key, nil
		}
	}
	if responses.Default != nil {
		return responses.Default, "default", nil
	}
	return nil, "", fmt.Errorf("%w: %d", ErrResponseNotFound, status)
}
