package compare

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/tobiasGuta/ParamIntel/internal/model"
	"github.com/tobiasGuta/ParamIntel/internal/responsefeatures"
)

func Snapshot(status int, headers map[string][]string, body []byte) model.Snapshot {
	paths, ok := flattenJSON(body)
	return model.Snapshot{
		StatusCode: status,
		Headers:    headers,
		Body:       append([]byte(nil), body...),
		JSONPaths:  paths,
		IsJSON:     ok,
		Features:   responsefeatures.Extract(headers, body),
	}
}

func BuildBaseline(samples []model.Snapshot) model.BaselineProfile {
	p := model.BaselineProfile{
		Samples:            len(samples),
		StableJSONPaths:    map[string]string{},
		SeenJSONPaths:      map[string]struct{}{},
		StableHeaderHashes: map[string]string{},
		SeenHeaderNames:    map[string]struct{}{},
	}
	if len(samples) == 0 {
		return p
	}

	first := samples[0]
	p.StatusCode = first.StatusCode
	p.StatusStable = true
	p.BodyLenMin, p.BodyLenMax = len(first.Body), len(first.Body)
	p.IsJSON = true

	p.ContentType = first.Features.ContentType
	p.ContentTypeStable = true

	p.TextMetricsAvailable = first.Features.IsText
	if p.TextMetricsAvailable {
		p.LineCountMin, p.LineCountMax = first.Features.LineCount, first.Features.LineCount
		p.WordCountMin, p.WordCountMax = first.Features.WordCount, first.Features.WordCount
	}

	p.HTMLAvailable = first.Features.IsHTML && first.Features.HTMLStructureHash != ""
	if p.HTMLAvailable {
		p.HTMLStructureStable = true
		p.HTMLStructureHash = first.Features.HTMLStructureHash
		p.HTMLElementCountMin, p.HTMLElementCountMax = first.Features.HTMLElementCount, first.Features.HTMLElementCount
	}

	for name, fingerprint := range responsefeatures.HeaderFingerprints(first.Headers) {
		p.StableHeaderHashes[name] = fingerprint
	}

	allSameBody := true
	firstHash := hash(first.Body)
	for _, s := range samples {
		if s.StatusCode != p.StatusCode {
			p.StatusStable = false
		}
		if len(s.Body) < p.BodyLenMin {
			p.BodyLenMin = len(s.Body)
		}
		if len(s.Body) > p.BodyLenMax {
			p.BodyLenMax = len(s.Body)
		}
		if hash(s.Body) != firstHash {
			allSameBody = false
		}
		if !s.IsJSON {
			p.IsJSON = false
		}
		for k := range s.JSONPaths {
			p.SeenJSONPaths[k] = struct{}{}
		}

		if s.Features.ContentType != p.ContentType {
			p.ContentTypeStable = false
		}

		if p.TextMetricsAvailable {
			if !s.Features.IsText {
				p.TextMetricsAvailable = false
			} else {
				p.LineCountMin = minInt(p.LineCountMin, s.Features.LineCount)
				p.LineCountMax = maxInt(p.LineCountMax, s.Features.LineCount)
				p.WordCountMin = minInt(p.WordCountMin, s.Features.WordCount)
				p.WordCountMax = maxInt(p.WordCountMax, s.Features.WordCount)
			}
		}

		if p.HTMLAvailable {
			if !s.Features.IsHTML || s.Features.HTMLStructureHash == "" {
				p.HTMLAvailable = false
				p.HTMLStructureStable = false
			} else {
				if s.Features.HTMLStructureHash != p.HTMLStructureHash {
					p.HTMLStructureStable = false
				}
				p.HTMLElementCountMin = minInt(p.HTMLElementCountMin, s.Features.HTMLElementCount)
				p.HTMLElementCountMax = maxInt(p.HTMLElementCountMax, s.Features.HTMLElementCount)
			}
		}

		headerFingerprints := responsefeatures.HeaderFingerprints(s.Headers)
		for name := range headerFingerprints {
			p.SeenHeaderNames[name] = struct{}{}
		}
		for name, expected := range p.StableHeaderHashes {
			if got, ok := headerFingerprints[name]; !ok || got != expected {
				delete(p.StableHeaderHashes, name)
			}
		}
	}

	if !p.TextMetricsAvailable {
		p.LineCountMin, p.LineCountMax = 0, 0
		p.WordCountMin, p.WordCountMax = 0, 0
	}
	if !p.HTMLAvailable {
		p.HTMLStructureStable = false
		p.HTMLStructureHash = ""
		p.HTMLElementCountMin, p.HTMLElementCountMax = 0, 0
	}
	if allSameBody {
		p.StableBody = firstHash
	}
	if p.IsJSON {
		for k, v := range first.JSONPaths {
			stable := true
			for i := 1; i < len(samples); i++ {
				if got, ok := samples[i].JSONPaths[k]; !ok || got != v {
					stable = false
					break
				}
			}
			if stable {
				p.StableJSONPaths[k] = v
			}
		}
	}
	return p
}

func AgainstBaseline(p model.BaselineProfile, s model.Snapshot) model.Comparison {
	var diffs []model.Difference
	if p.StatusStable && s.StatusCode != p.StatusCode {
		diffs = append(diffs, model.Difference{Kind: "status", Before: strconv.Itoa(p.StatusCode), After: strconv.Itoa(s.StatusCode)})
	}

	// Evidence-fidelity features are additive to the existing body/JSON model.
	// Only observations that were stable across baseline samples are eligible.
	diffs = append(diffs, compareStableResponseFeatures(p, s)...)

	if p.IsJSON && s.IsJSON {
		for path, before := range p.StableJSONPaths {
			after, ok := s.JSONPaths[path]
			if !ok {
				diffs = append(diffs, model.Difference{Kind: "json_path_removed", Path: path, Before: before})
			} else if after != before {
				diffs = append(diffs, model.Difference{Kind: "json_value_changed", Path: path, Before: before, After: after})
			}
		}
		for path, after := range s.JSONPaths {
			if _, seen := p.SeenJSONPaths[path]; !seen {
				diffs = append(diffs, model.Difference{Kind: "json_path_added", Path: path, After: after})
			}
		}
	} else if p.StableBody != "" {
		if hash(s.Body) != p.StableBody {
			diffs = append(diffs, model.Difference{Kind: "body_changed"})
		}
	} else {
		tolerance := 32
		span := p.BodyLenMax - p.BodyLenMin
		if span > tolerance {
			tolerance = span * 2
		}
		if len(s.Body) < p.BodyLenMin-tolerance || len(s.Body) > p.BodyLenMax+tolerance {
			diffs = append(diffs, model.Difference{Kind: "body_length", Before: fmt.Sprintf("%d..%d", p.BodyLenMin, p.BodyLenMax), After: strconv.Itoa(len(s.Body))})
		}
	}
	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].Kind == diffs[j].Kind {
			return diffs[i].Path < diffs[j].Path
		}
		return diffs[i].Kind < diffs[j].Kind
	})
	return model.Comparison{Meaningful: len(diffs) > 0, Differences: diffs}
}

func compareStableResponseFeatures(p model.BaselineProfile, s model.Snapshot) []model.Difference {
	var diffs []model.Difference

	if p.ContentTypeStable && s.Features.ContentType != p.ContentType {
		diffs = append(diffs, model.Difference{
			Kind:   "content_type",
			Before: p.ContentType,
			After:  s.Features.ContentType,
		})
	}

	probeHeaders := responsefeatures.HeaderFingerprints(s.Headers)
	for name, expected := range p.StableHeaderHashes {
		got, ok := probeHeaders[name]
		if !ok {
			diffs = append(diffs, model.Difference{Kind: "header_removed", Path: name})
			continue
		}
		if got != expected {
			diffs = append(diffs, model.Difference{Kind: "header_value_changed", Path: name})
		}
	}
	for name := range probeHeaders {
		if _, seen := p.SeenHeaderNames[name]; !seen {
			diffs = append(diffs, model.Difference{Kind: "header_added", Path: name})
		}
	}

	// JSON already has path-level semantic comparison. The new body metrics are
	// intentionally focused on non-JSON responses where v0.6 had less fidelity.
	if p.IsJSON {
		return diffs
	}

	if p.HTMLAvailable && p.HTMLStructureStable && s.Features.IsHTML && s.Features.HTMLStructureHash != "" && s.Features.HTMLStructureHash != p.HTMLStructureHash {
		diffs = append(diffs, model.Difference{Kind: "html_structure_changed"})
	}

	if p.TextMetricsAvailable && s.Features.IsText {
		if outsideMetricRange(s.Features.LineCount, p.LineCountMin, p.LineCountMax, 2) {
			diffs = append(diffs, model.Difference{
				Kind:   "line_count",
				Before: fmt.Sprintf("%d..%d", p.LineCountMin, p.LineCountMax),
				After:  strconv.Itoa(s.Features.LineCount),
			})
		}
		if outsideMetricRange(s.Features.WordCount, p.WordCountMin, p.WordCountMax, 8) {
			diffs = append(diffs, model.Difference{
				Kind:   "word_count",
				Before: fmt.Sprintf("%d..%d", p.WordCountMin, p.WordCountMax),
				After:  strconv.Itoa(s.Features.WordCount),
			})
		}
	}

	return diffs
}

func outsideMetricRange(value, minValue, maxValue, minimumTolerance int) bool {
	span := maxValue - minValue
	tolerance := minimumTolerance
	if span*2 > tolerance {
		tolerance = span * 2
	}
	return value < minValue-tolerance || value > maxValue+tolerance
}

func flattenJSON(body []byte) (map[string]string, bool) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, false
	}
	out := map[string]string{}
	walkJSON("$", v, out)
	return out, true
}

func walkJSON(path string, v any, out map[string]string) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out[path+".__keys"] = stringsJoin(keys, ",")
		for _, k := range keys {
			walkJSON(path+"."+k, x[k], out)
		}
	case []any:
		out[path+".__len"] = strconv.Itoa(len(x))
		for i, item := range x {
			walkJSON(fmt.Sprintf("%s[%d]", path, i), item, out)
		}
	case nil:
		out[path] = "null"
	case string:
		out[path] = "s:" + x
	case bool:
		out[path] = "b:" + strconv.FormatBool(x)
	case float64:
		out[path] = "n:" + strconv.FormatFloat(x, 'g', -1, 64)
	default:
		out[path] = fmt.Sprintf("%v", x)
	}
}

func stringsJoin(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	n := len(sep) * (len(parts) - 1)
	for _, p := range parts {
		n += len(p)
	}
	b := make([]byte, 0, n)
	for i, p := range parts {
		if i > 0 {
			b = append(b, sep...)
		}
		b = append(b, p...)
	}
	return string(b)
}

func minInt(a, b int) int {
	if b < a {
		return b
	}
	return a
}

func maxInt(a, b int) int {
	if b > a {
		return b
	}
	return a
}

func hash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
