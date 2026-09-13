package discovery

import (
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

const openAPIResponseOnlySource = "openapi_response_only_json_property"

// schemaTypedProbeKind returns a narrowly supported JSON probe kind only when
// an OpenAPI-derived candidate has one unambiguous scalar declaration. Schema
// metadata influences the probe value only; it never changes confidence or
// bypasses candidate/control verification.
func schemaTypedProbeKind(candidate model.Candidate) (string, bool) {
	if candidate.Location != model.LocationJSON {
		return "", false
	}

	var selected string
	matched := false
	for _, source := range candidate.Sources {
		if source.Source != openAPIResponseOnlySource {
			continue
		}
		matched = true
		if len(source.DeclaredTypes) != 1 {
			return "", false
		}
		kind := strings.ToLower(strings.TrimSpace(source.DeclaredTypes[0]))
		switch kind {
		case "boolean", "integer":
		default:
			return "", false
		}
		if selected != "" && selected != kind {
			return "", false
		}
		selected = kind
	}
	if !matched || selected == "" {
		return "", false
	}
	return selected, true
}

func schemaTypedProbeValue(candidate model.Candidate) (model.ProbeValue, bool) {
	kind, ok := schemaTypedProbeKind(candidate)
	if !ok {
		return model.ProbeValue{}, false
	}
	switch kind {
	case "boolean":
		return model.BoolValue(true), true
	case "integer":
		return model.IntegerValue(1), true
	default:
		return model.ProbeValue{}, false
	}
}
