package schemaintel

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

var (
	ErrUnstableBaseline = errors.New("schema intelligence requires a stable baseline status and content type")
	ErrRequestJSON      = errors.New("captured request body must be a JSON object")
)

func Analyze(doc *Document, tmpl model.RequestTemplate, baseline model.BaselineProfile, cfg Config) (Report, error) {
	if doc == nil || doc.model == nil {
		return Report{}, fmt.Errorf("openapi document is nil")
	}
	if !baseline.StatusStable || !baseline.ContentTypeStable {
		return Report{}, ErrUnstableBaseline
	}

	u, err := url.Parse(tmpl.URL)
	if err != nil {
		return Report{}, fmt.Errorf("parse captured request URL: %w", err)
	}
	requestPath := u.Path
	if requestPath == "" {
		requestPath = "/"
	}
	op, operation, err := matchOperation(doc, tmpl.Method, requestPath)
	if err != nil {
		return Report{}, err
	}
	if op.RequestBody == nil {
		return Report{}, fmt.Errorf("%w: operation has no request body", ErrMediaTypeNotFound)
	}

	requestMedia, requestMediaKey, err := selectMediaType(op.RequestBody.Content, tmpl.Headers.Get("Content-Type"))
	if err != nil {
		return Report{}, fmt.Errorf("select request schema: %w", err)
	}
	if requestMedia.Schema == nil {
		return Report{}, fmt.Errorf("select request schema: media type %s has no schema", requestMediaKey)
	}

	response, responseStatusKey, err := selectResponse(op.Responses, baseline.StatusCode)
	if err != nil {
		return Report{}, err
	}
	responseMedia, responseMediaKey, err := selectMediaType(response.Content, baseline.ContentType)
	if err != nil {
		return Report{}, fmt.Errorf("select response schema: %w", err)
	}
	if responseMedia.Schema == nil {
		return Report{}, fmt.Errorf("select response schema: media type %s has no schema", responseMediaKey)
	}

	requestProperties, requestSkipped, err := flattenSchema(requestMedia.Schema, cfg)
	if err != nil {
		return Report{}, fmt.Errorf("walk request schema: %w", err)
	}
	responseProperties, responseSkipped, err := flattenSchema(responseMedia.Schema, cfg)
	if err != nil {
		return Report{}, fmt.Errorf("walk response schema: %w", err)
	}

	actualProperties, actualObjects, err := collectActualJSON(tmpl.Body)
	if err != nil {
		return Report{}, err
	}

	report := Report{
		OpenAPIVersion:     doc.Version(),
		Operation:          operation,
		RequestMediaType:   requestMediaKey,
		ResponseStatusKey:  responseStatusKey,
		ResponseMediaType:  responseMediaKey,
		RequestProperties:  sortedProperties(requestProperties),
		ResponseProperties: sortedProperties(responseProperties),
		Skipped:            append(append([]SkippedDescriptor(nil), requestSkipped...), responseSkipped...),
	}

	for _, property := range report.ResponseProperties {
		if _, documentedForRequest := requestProperties[property.Path]; documentedForRequest {
			continue
		}
		if actualProperties[property.Path] {
			report.Skipped = append(report.Skipped, SkippedDescriptor{Path: property.Path, Reason: "property already exists in captured request"})
			continue
		}
		if property.Ambiguous {
			report.Skipped = append(report.Skipped, SkippedDescriptor{Path: property.Path, Reason: "ambiguous oneOf/anyOf property is not a candidate"})
			continue
		}
		if property.Array {
			report.Skipped = append(report.Skipped, SkippedDescriptor{Path: property.Path, Reason: "array-valued property is outside v0.9 Slice 1"})
			continue
		}
		if property.Object {
			// Object containers are useful placement context for descendants, but
			// the current ParamIntel mutation model does not probe whole objects.
			continue
		}

		candidate := CandidateDescriptor{
			Name:          property.Name,
			Path:          property.Path,
			Parent:        property.Parent,
			DeclaredTypes: append([]string(nil), property.DeclaredTypes...),
			Nullable:      property.Nullable,
			Required:      property.Required,
			ReadOnly:      property.ReadOnly,
			WriteOnly:     property.WriteOnly,
			SchemaRef:     property.SchemaRef,
			Source:        SourceResponseOnlyJSONProperty,
			Reason:        "property is present in the selected response schema and absent from the selected request schema",
		}

		switch {
		case actualObjects[property.Parent]:
			candidate.Placement = PlacementExistingParent
		case actualProperties[property.Parent]:
			report.Skipped = append(report.Skipped, SkippedDescriptor{Path: property.Path, Reason: "candidate parent exists in captured request but is not an object"})
			continue
		default:
			parentDescriptor, parentKnown := responseProperties[property.Parent]
			ancestor := parentJSONPath(property.Parent)
			if parentKnown && parentDescriptor.Object && !parentDescriptor.Array && !parentDescriptor.Ambiguous && ancestor != "" && actualObjects[ancestor] {
				candidate.Placement = PlacementOneLevelScaffold
				candidate.JSONScaffoldParent = property.Parent
			} else {
				report.Skipped = append(report.Skipped, SkippedDescriptor{Path: property.Path, Reason: "candidate requires an unsupported or deeper missing JSON parent"})
				continue
			}
		}
		report.Candidates = append(report.Candidates, candidate)
	}

	sort.Slice(report.Candidates, func(i, j int) bool { return report.Candidates[i].Path < report.Candidates[j].Path })
	sort.SliceStable(report.Skipped, func(i, j int) bool {
		if report.Skipped[i].Path == report.Skipped[j].Path {
			return report.Skipped[i].Reason < report.Skipped[j].Reason
		}
		return report.Skipped[i].Path < report.Skipped[j].Path
	})
	return report, nil
}

func collectActualJSON(body []byte) (map[string]bool, map[string]bool, error) {
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrRequestJSON, err)
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil, nil, ErrRequestJSON
	}
	properties := make(map[string]bool)
	objects := map[string]bool{"$": true}
	var walk func(map[string]any, string)
	walk = func(current map[string]any, parent string) {
		for name, value := range current {
			path := joinJSONPath(parent, name)
			properties[path] = true
			if child, ok := value.(map[string]any); ok {
				objects[path] = true
				walk(child, path)
			}
		}
	}
	walk(obj, "$")
	return properties, objects, nil
}
