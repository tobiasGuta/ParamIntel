package contextintel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

const responseOnlyPriority = 100

type Report struct {
	ObservedProperties int
	Actionable         []model.Candidate
	SkippedExisting    int
	SkippedNoParent    int
}

// HarvestJSONResponse derives exact JSON candidate placements from a related
// API response. It only emits response-only properties whose parent object
// already exists in the request body, so v0.3 never invents missing object
// scaffolding or array mutation semantics.
func HarvestJSONResponse(requestBody, rawResponse []byte, maxDepth int) (Report, error) {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	requestRoot, err := decodeJSONObject(requestBody)
	if err != nil {
		return Report{}, fmt.Errorf("context candidate request body: %w", err)
	}
	responseBody := responseBody(rawResponse)
	responseRoot, err := decodeJSONObject(responseBody)
	if err != nil {
		return Report{}, fmt.Errorf("context response: %w", err)
	}

	requestObjects := map[string]struct{}{"$": {}}
	requestProperties := map[string]struct{}{}
	collectRequestPaths(requestRoot, "$", 0, maxDepth, requestObjects, requestProperties)

	var properties []property
	collectProperties(responseRoot, "$", 0, maxDepth, &properties)

	report := Report{ObservedProperties: len(properties)}
	seen := map[string]struct{}{}
	for _, p := range properties {
		if _, ok := requestProperties[p.Path]; ok {
			report.SkippedExisting++
			continue
		}
		if _, ok := requestObjects[p.Parent]; !ok {
			report.SkippedNoParent++
			continue
		}
		key := p.Parent + "|" + p.Name
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		report.Actionable = append(report.Actionable, model.Candidate{
			Name:       p.Name,
			Location:   model.LocationJSON,
			JSONParent: p.Parent,
			Sources: []model.CandidateSource{{
				Source:       "context_response_only_json_property",
				Path:         p.Path,
				ObservedType: p.ObservedType,
				Priority:     responseOnlyPriority,
			}},
		})
	}
	return report, nil
}

type property struct {
	Name         string
	Path         string
	Parent       string
	ObservedType string
}

func decodeJSONObject(body []byte) (map[string]any, error) {
	var root any
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(body)))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("body is not parseable JSON: %w", err)
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("JSON root must be an object")
	}
	return obj, nil
}

func responseBody(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if !bytes.HasPrefix(trimmed, []byte("HTTP/")) {
		return trimmed
	}
	if i := bytes.Index(trimmed, []byte("\r\n\r\n")); i >= 0 {
		return trimmed[i+4:]
	}
	if i := bytes.Index(trimmed, []byte("\n\n")); i >= 0 {
		return trimmed[i+2:]
	}
	return trimmed
}

func collectRequestPaths(obj map[string]any, parent string, depth, maxDepth int, objects, properties map[string]struct{}) {
	if depth > maxDepth {
		return
	}
	for name, value := range obj {
		path := join(parent, name)
		properties[path] = struct{}{}
		child, ok := value.(map[string]any)
		if ok && depth < maxDepth {
			objects[path] = struct{}{}
			collectRequestPaths(child, path, depth+1, maxDepth, objects, properties)
		}
	}
}

func collectProperties(obj map[string]any, parent string, depth, maxDepth int, out *[]property) {
	if depth > maxDepth {
		return
	}
	for name, value := range obj {
		path := join(parent, name)
		*out = append(*out, property{Name: name, Path: path, Parent: parent, ObservedType: observedType(value)})
		child, ok := value.(map[string]any)
		if ok && depth < maxDepth {
			collectProperties(child, path, depth+1, maxDepth, out)
		}
	}
}

func observedType(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case json.Number:
		if !strings.ContainsAny(x.String(), ".eE") {
			return "integer"
		}
		return "number"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		return "unknown"
	}
}

func join(parent, name string) string {
	if parent == "$" {
		return "$." + name
	}
	return parent + "." + name
}
