package aiadvisor

import (
	"bytes"
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/model"
	"github.com/tobiasGuta/ParamIntel/internal/mutate"
)

var (
	uuidSegment     = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	intSegment      = regexp.MustCompile(`^[0-9]+$`)
	keyPattern      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.:-]{0,79}$`)
	safePathSegment = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._~-]{0,31}$`)
)

// BuildInput creates the only structure that may leave ParamIntel for an AI
// provider. It intentionally excludes headers, cookies, authorization values,
// query/form values, JSON primitive values, hostnames, and raw response text.
func BuildInput(tmpl model.RequestTemplate, rawContext []byte, locations []string, maxDepth int) (Input, error) {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	u, err := url.Parse(tmpl.URL)
	if err != nil {
		return Input{}, err
	}

	requestObject, _ := decodeJSONObject(tmpl.Body)
	requestShape := shapeObject(requestObject, 0, maxDepth)
	responseObject, _ := decodeJSONObject(extractResponseBody(rawContext))
	responseShape := shapeObject(responseObject, 0, maxDepth)

	input := Input{
		Method:            strings.ToUpper(strings.TrimSpace(tmpl.Method)),
		Path:              sanitizePath(u.Path),
		QueryKeys:         sortedKeys(u.Query()),
		JSONParents:       mutate.JSONObjectParents(tmpl.Body, maxDepth),
		RequestJSONShape:  requestShape,
		ResponseJSONShape: responseShape,
	}
	input.FormKeys = formKeys(tmpl)
	input.ActiveLocations = activeLocations(tmpl, locations, len(input.JSONParents) > 0)
	sort.Strings(input.JSONParents)
	return input, nil
}

func activeLocations(tmpl model.RequestTemplate, locations []string, hasJSONObject bool) []string {
	if len(locations) != 0 && !(len(locations) == 1 && strings.EqualFold(locations[0], "auto")) {
		out := append([]string(nil), locations...)
		return out
	}
	out := []string{model.LocationQuery}
	contentType := strings.ToLower(tmpl.Headers.Get("Content-Type"))
	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		out = append(out, model.LocationForm)
	}
	if hasJSONObject {
		out = append(out, model.LocationJSON)
	}
	return out
}

func formKeys(tmpl model.RequestTemplate) []string {
	contentType := strings.ToLower(tmpl.Headers.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		return nil
	}
	values, err := url.ParseQuery(string(tmpl.Body))
	if err != nil {
		return nil
	}
	return sortedKeys(values)
}

func sortedKeys(values url.Values) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if !keyPattern.MatchString(key) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func decodeJSONObject(body []byte) (map[string]any, bool) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil, false
	}
	var root any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return nil, false
	}
	obj, ok := root.(map[string]any)
	return obj, ok
}

func shapeObject(obj map[string]any, depth, maxDepth int) map[string]any {
	if len(obj) == 0 || depth > maxDepth {
		return nil
	}
	out := make(map[string]any, len(obj))
	for key, value := range obj {
		if !keyPattern.MatchString(key) {
			continue
		}
		switch typed := value.(type) {
		case map[string]any:
			if depth < maxDepth {
				if child := shapeObject(typed, depth+1, maxDepth); len(child) > 0 {
					out[key] = child
					continue
				}
			}
			out[key] = "object"
		case []any:
			out[key] = "array"
		case nil:
			out[key] = "null"
		case bool:
			out[key] = "boolean"
		case string:
			out[key] = "string"
		case json.Number:
			if strings.ContainsAny(typed.String(), ".eE") {
				out[key] = "number"
			} else {
				out[key] = "integer"
			}
		default:
			out[key] = "unknown"
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func extractResponseBody(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || !bytes.HasPrefix(trimmed, []byte("HTTP/")) {
		return trimmed
	}
	if i := bytes.Index(trimmed, []byte("\r\n\r\n")); i >= 0 {
		return trimmed[i+4:]
	}
	if i := bytes.Index(trimmed, []byte("\n\n")); i >= 0 {
		return trimmed[i+2:]
	}
	return nil
}

func sanitizePath(path string) string {
	if path == "" {
		return "/"
	}
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if segment == "" {
			continue
		}
		decoded, err := url.PathUnescape(segment)
		if err == nil {
			segment = decoded
		}
		switch {
		case intSegment.MatchString(segment):
			segments[i] = "{int}"
		case uuidSegment.MatchString(segment):
			segments[i] = "{uuid}"
		case strings.Contains(segment, "@"):
			segments[i] = "{value}"
		case !safePathSegment.MatchString(segment):
			segments[i] = "{value}"
		default:
			segments[i] = segment
		}
	}
	return strings.Join(segments, "/")
}
