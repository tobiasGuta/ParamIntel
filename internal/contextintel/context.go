package contextintel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

const (
	responseOnlyPriority = 100
	scaffoldablePriority = 90
)

type Report struct {
	ObservedProperties int
	Actionable         []model.Candidate
	Scaffoldable       []model.Candidate
	SkippedExisting    int
	SkippedNoParent    int
}

// HarvestJSONResponse derives exact JSON candidate placements from a related
// API response. Response-only properties whose parent already exists in the
// request remain immediately actionable. v0.8 additionally classifies a
// narrow set of one-level-missing parents as scaffoldable metadata, but this
// function never authorizes mutation or creates request objects itself.
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
	sort.Slice(properties, func(i, j int) bool { return properties[i].Path < properties[j].Path })

	report := Report{ObservedProperties: len(properties)}
	actionableSeen := map[string]struct{}{}
	scaffoldableSeen := map[string]struct{}{}
	for _, p := range properties {
		if _, ok := requestProperties[p.Path]; ok {
			report.SkippedExisting++
			continue
		}
		if _, ok := requestObjects[p.Parent]; !ok {
			// Preserve the existing accounting: until the caller explicitly opts
			// into a future scaffold-capable discovery path, this property is still
			// skipped from active testing.
			report.SkippedNoParent++
			if !oneLevelScaffoldableParent(p.Parent, requestObjects, requestProperties) {
				continue
			}
			key := p.Parent + "|" + p.Name
			if _, ok := scaffoldableSeen[key]; ok {
				continue
			}
			scaffoldableSeen[key] = struct{}{}
			report.Scaffoldable = append(report.Scaffoldable, model.Candidate{
				Name:               p.Name,
				Location:           model.LocationJSON,
				JSONParent:         p.Parent,
				JSONScaffoldParent: p.Parent,
				Sources: []model.CandidateSource{{
					Source:       "context_response_scaffoldable_json_property",
					Path:         p.Path,
					ObservedType: p.ObservedType,
					Priority:     scaffoldablePriority,
					Reason:       "immediate parent object is response-derived and absent from request",
				}},
			})
			continue
		}
		key := p.Parent + "|" + p.Name
		if _, ok := actionableSeen[key]; ok {
			continue
		}
		actionableSeen[key] = struct{}{}
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

// oneLevelScaffoldableParent returns true only when the missing parent path
// itself does not already exist as a scalar/null/array property and its direct
// parent is an object already present in the captured request. This represents
// exactly one missing object level without replacing existing request data.
func oneLevelScaffoldableParent(path string, requestObjects, requestProperties map[string]struct{}) bool {
	if path == "" || path == "$" {
		return false
	}
	if _, exists := requestProperties[path]; exists {
		return false
	}
	directParent := parentPath(path)
	if directParent == "" {
		return false
	}
	_, ok := requestObjects[directParent]
	return ok
}

func parentPath(path string) string {
	if path == "" || path == "$" || !strings.HasPrefix(path, "$.") {
		return ""
	}
	i := strings.LastIndex(path, ".")
	if i <= 0 {
		return ""
	}
	return path[:i]
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
