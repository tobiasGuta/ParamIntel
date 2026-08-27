package compare

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func Snapshot(status int, headers map[string][]string, body []byte) model.Snapshot {
	paths, ok := flattenJSON(body)
	return model.Snapshot{StatusCode: status, Headers: headers, Body: append([]byte(nil), body...), JSONPaths: paths, IsJSON: ok}
}

func BuildBaseline(samples []model.Snapshot) model.BaselineProfile {
	p := model.BaselineProfile{Samples: len(samples), StableJSONPaths: map[string]string{}, SeenJSONPaths: map[string]struct{}{}}
	if len(samples) == 0 {
		return p
	}
	p.StatusCode = samples[0].StatusCode
	p.StatusStable = true
	p.BodyLenMin, p.BodyLenMax = len(samples[0].Body), len(samples[0].Body)
	p.IsJSON = true
	allSameBody := true
	firstHash := hash(samples[0].Body)
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
	}
	if allSameBody {
		p.StableBody = firstHash
	}
	if p.IsJSON {
		for k, v := range samples[0].JSONPaths {
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

func hash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
