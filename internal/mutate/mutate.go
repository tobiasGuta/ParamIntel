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
// JSON candidates by parent object before batching. Scaffold-marked JSON
// candidates additionally require an explicit per-mutation authorization.
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

		createdScaffolds := map[string]struct{}{}
		for _, m := range jsonMutations {
			var parent map[string]any
			var err error
			if m.Candidate.RequiresJSONScaffold() {
				if !m.AllowJSONScaffold {
					return model.RequestTemplate{}, fmt.Errorf("JSON scaffold for %q requires explicit authorization", m.Candidate.JSONPath())
				}
				if m.Candidate.JSONScaffoldParent != m.Candidate.JSONParent {
					return model.RequestTemplate{}, fmt.Errorf("JSON scaffold parent %q does not match candidate parent %q", m.Candidate.JSONScaffoldParent, m.Candidate.JSONParent)
				}
				if _, alreadyCreated := createdScaffolds[m.Candidate.JSONScaffoldParent]; alreadyCreated {
					parent, err = resolveObject(obj, m.Candidate.JSONScaffoldParent)
				} else {
					parent, err = createOneLevelObject(obj, m.Candidate.JSONScaffoldParent)
					if err == nil {
						createdScaffolds[m.Candidate.JSONScaffoldParent] = struct{}{}
					}
				}
			} else {
				parent, err = resolveObject(obj, m.Candidate.JSONParent)
			}
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

// createOneLevelObject creates exactly the final object component of path. Its
// direct parent must already exist as an object and the final component must be
// completely absent. Existing objects, nulls, scalars, and arrays are never
// replaced. This independently enforces the same boundary used by context
// classification rather than trusting candidate metadata alone.
func createOneLevelObject(root map[string]any, path string) (map[string]any, error) {
	directParent, name, err := splitParentPath(path)
	if err != nil {
		return nil, err
	}
	parent, err := resolveObject(root, directParent)
	if err != nil {
		return nil, fmt.Errorf("cannot scaffold JSON parent %q: %w", path, err)
	}
	if _, exists := parent[name]; exists {
		return nil, fmt.Errorf("cannot scaffold JSON parent %q because it already exists", path)
	}
	child := map[string]any{}
	parent[name] = child
	return child, nil
}

func splitParentPath(path string) (string, string, error) {
	if path == "" || path == "$" || !strings.HasPrefix(path, "$." ) {
		return "", "", fmt.Errorf("invalid JSON scaffold path %q", path)
	}
	i := strings.LastIndex(path, ".")
	if i <= 0 || i == len(path)-1 {
		return "", "", fmt.Errorf("invalid JSON scaffold path %q", path)
	}
	return path[:i], path[i+1:], nil
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
