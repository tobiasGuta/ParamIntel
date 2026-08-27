package mutate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

// Apply returns a copy of tmpl with all mutations applied. Mutations must be
// from the same logical target group when they touch JSON: the engine groups
// JSON candidates by parent object before batching.
func Apply(tmpl model.RequestTemplate, mutations []model.Mutation) (model.RequestTemplate, error) {
	out := model.RequestTemplate{
		Method:  tmpl.Method,
		URL:     tmpl.URL,
		Headers: tmpl.Headers.Clone(),
		Body:    append([]byte(nil), tmpl.Body...),
	}
	if out.Headers == nil {
		out.Headers = make(http.Header)
	}

	query := map[string]string{}
	form := map[string]string{}
	var jsonMutations []model.Mutation
	for _, m := range mutations {
		switch m.Candidate.Location {
		case model.LocationQuery:
			query[m.Candidate.Name] = m.Value.Raw
		case model.LocationForm:
			form[m.Candidate.Name] = m.Value.Raw
		case model.LocationJSON:
			jsonMutations = append(jsonMutations, m)
		default:
			return model.RequestTemplate{}, fmt.Errorf("unsupported mutation location %q", m.Candidate.Location)
		}
	}

	if len(query) > 0 {
		u, err := url.Parse(out.URL)
		if err != nil {
			return model.RequestTemplate{}, err
		}
		q := u.Query()
		for k, v := range query {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		out.URL = u.String()
	}

	if len(form) > 0 {
		contentType := strings.ToLower(out.Headers.Get("Content-Type"))
		if !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
			return model.RequestTemplate{}, fmt.Errorf("form mutation requires application/x-www-form-urlencoded content type")
		}
		values, err := url.ParseQuery(string(out.Body))
		if err != nil {
			return model.RequestTemplate{}, fmt.Errorf("parse form body: %w", err)
		}
		for k, v := range form {
			values.Set(k, v)
		}
		out.Body = []byte(values.Encode())
	}

	if len(jsonMutations) > 0 {
		// Some real applications send JSON syntax under generic content types
		// such as text/plain. JSON discovery therefore follows the body
		// structure rather than requiring a JSON MIME type. The original header
		// is preserved so replay semantics remain faithful to the captured
		// request.
		var root any
		dec := json.NewDecoder(bytes.NewReader(out.Body))
		dec.UseNumber()
		if err := dec.Decode(&root); err != nil {
			return model.RequestTemplate{}, fmt.Errorf("json mutation requires a parseable JSON body: %w", err)
		}
		obj, ok := root.(map[string]any)
		if !ok {
			return model.RequestTemplate{}, fmt.Errorf("json discovery currently requires an object root")
		}
		for _, m := range jsonMutations {
			parent, err := resolveObject(obj, m.Candidate.JSONParent)
			if err != nil {
				return model.RequestTemplate{}, err
			}
			parent[m.Candidate.Name] = jsonValue(m.Value)
		}
		body, err := json.Marshal(obj)
		if err != nil {
			return model.RequestTemplate{}, err
		}
		out.Body = body
	}

	out.Headers.Del("Content-Length")
	return out, nil
}

func resolveObject(root map[string]any, path string) (map[string]any, error) {
	if path == "" || path == "$" {
		return root, nil
	}
	if !strings.HasPrefix(path, "$.") {
		return nil, fmt.Errorf("invalid JSON parent path %q", path)
	}
	cur := root
	for _, part := range strings.Split(strings.TrimPrefix(path, "$."), ".") {
		next, ok := cur[part].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("JSON parent %q is not an object", path)
		}
		cur = next
	}
	return cur, nil
}

func jsonValue(v model.ProbeValue) any {
	switch v.Kind {
	case "boolean":
		b, _ := strconv.ParseBool(v.Raw)
		return b
	case "integer":
		i, _ := strconv.Atoi(v.Raw)
		return i
	case "null":
		return nil
	default:
		return v.Raw
	}
}

// JSONObjectParents returns object paths suitable for safe nested insertion.
// Arrays are intentionally not traversed in v0.2 to avoid ambiguous mutation
// semantics and combinatorial expansion.
func JSONObjectParents(body []byte, maxDepth int) []string {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	var root any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return nil
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil
	}
	out := []string{"$"}
	walkParents(obj, "$", 0, maxDepth, &out)
	return out
}

func walkParents(obj map[string]any, path string, depth, maxDepth int, out *[]string) {
	if depth >= maxDepth {
		return
	}
	for k, v := range obj {
		child, ok := v.(map[string]any)
		if !ok {
			continue
		}
		childPath := path + "." + k
		*out = append(*out, childPath)
		walkParents(child, childPath, depth+1, maxDepth, out)
	}
}
