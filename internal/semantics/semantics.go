package semantics

import (
	"regexp"
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

type TypeGuess struct {
	Name       string
	Confidence float64
	Evidence   string
}

var typePatterns = []struct {
	re       *regexp.Regexp
	typeName string
}{
	{regexp.MustCompile(`(?i)(must|should|expected|required|needs? to)\s+(?:be\s+)?(?:an?\s+)?(?:integer|int)\b|invalid\s+(?:integer|int)\b`), "integer"},
	{regexp.MustCompile(`(?i)(must|should|expected|required|needs? to)\s+(?:be\s+)?(?:a\s+)?(?:boolean|bool)\b|invalid\s+(?:boolean|bool)\b`), "boolean"},
	{regexp.MustCompile(`(?i)(must|should|expected|required|needs? to)\s+(?:be\s+)?(?:a\s+)?number\b|invalid\s+number\b`), "number"},
	{regexp.MustCompile(`(?i)(must|should|expected|required|needs? to)\s+(?:be\s+)?(?:a\s+)?string\b|invalid\s+string\b`), "string"},
	{regexp.MustCompile(`(?i)(must|should|expected|required|needs? to)\s+(?:be\s+)?(?:a\s+)?uuid\b|invalid\s+uuid\b`), "uuid"},
	{regexp.MustCompile(`(?i)(must|should|expected|required|needs? to)\s+(?:be\s+)?(?:an?\s+)?array\b|invalid\s+array\b`), "array"},
	{regexp.MustCompile(`(?i)(must|should|expected|required|needs? to)\s+(?:be\s+)?(?:an?\s+)?object\b|invalid\s+object\b`), "object"},
}

func InferFromResponse(body []byte) TypeGuess {
	text := string(body)
	for _, p := range typePatterns {
		if p.re.MatchString(text) {
			return TypeGuess{Name: p.typeName, Confidence: .98, Evidence: "server validation response"}
		}
	}
	return TypeGuess{}
}

func GuessFromName(name string) TypeGuess {
	n := strings.ToLower(name)
	if isBooleanName(n) {
		return TypeGuess{Name: "boolean", Confidence: .55, Evidence: "parameter-name heuristic"}
	}
	if isIntegerName(n) {
		return TypeGuess{Name: "integer", Confidence: .55, Evidence: "parameter-name heuristic"}
	}
	return TypeGuess{}
}

func ProfileValues(name, location string) []model.ProbeValue {
	n := strings.ToLower(name)
	jsonTyped := location == model.LocationJSON
	switch {
	case isBooleanName(n):
		if jsonTyped {
			return []model.ProbeValue{model.BoolValue(true), model.BoolValue(false), model.StringValue("true"), model.StringValue("false")}
		}
		return []model.ProbeValue{model.StringValue("true"), model.StringValue("false"), model.StringValue("1"), model.StringValue("0")}
	case isIntegerName(n):
		if jsonTyped {
			return []model.ProbeValue{model.IntegerValue(0), model.IntegerValue(1), model.IntegerValue(10), model.IntegerValue(-1)}
		}
		return []model.ProbeValue{model.StringValue("0"), model.StringValue("1"), model.StringValue("10"), model.StringValue("-1")}
	case n == "format":
		return stringValues("json", "xml", "html")
	case n == "sort" || n == "order":
		return stringValues("asc", "desc")
	case n == "include" || n == "fields" || n == "expand":
		return stringValues("all", "true", "*")
	default:
		return nil
	}
}

func isBooleanName(n string) bool {
	if strings.HasPrefix(n, "is_") || strings.HasPrefix(n, "has_") || strings.HasPrefix(n, "can_") || strings.HasPrefix(n, "allow_") || strings.HasPrefix(n, "enable_") || strings.HasPrefix(n, "show_") {
		return true
	}
	for _, exact := range []string{"admin", "debug", "internal", "preview", "verbose", "test", "include_deleted", "deleted", "enabled", "disabled"} {
		if n == exact {
			return true
		}
	}
	return false
}

func isIntegerName(n string) bool {
	for _, exact := range []string{"limit", "offset", "page", "page_size", "size", "count", "max", "min"} {
		if n == exact {
			return true
		}
	}
	return strings.HasSuffix(n, "_count") || strings.HasSuffix(n, "_limit") || strings.HasSuffix(n, "_offset") || strings.HasSuffix(n, "_size")
}

func stringValues(values ...string) []model.ProbeValue {
	out := make([]model.ProbeValue, 0, len(values))
	for _, v := range values {
		out = append(out, model.StringValue(v))
	}
	return out
}
