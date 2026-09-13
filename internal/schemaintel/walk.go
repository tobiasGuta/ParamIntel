package schemaintel

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	highbase "github.com/pb33f/libopenapi/datamodel/high/base"
)

var ErrSchemaBudgetExceeded = errors.New("openapi schema walk budget exceeded")

type walkState struct {
	cfg        Config
	nodes      int
	properties map[string]PropertyDescriptor
	skipped    []SkippedDescriptor
	refStack   map[string]bool
}

func flattenSchema(root *highbase.SchemaProxy, cfg Config) (map[string]PropertyDescriptor, []SkippedDescriptor, error) {
	if cfg.MaxDepth <= 0 {
		cfg.MaxDepth = DefaultConfig().MaxDepth
	}
	if cfg.MaxNodes <= 0 {
		cfg.MaxNodes = DefaultConfig().MaxNodes
	}
	state := &walkState{
		cfg:        cfg,
		properties: make(map[string]PropertyDescriptor),
		refStack:   make(map[string]bool),
	}
	if root == nil {
		return state.properties, state.skipped, nil
	}
	if err := state.walkProxy(root, "$", 0); err != nil {
		return nil, state.skipped, err
	}
	return state.properties, state.skipped, nil
}

func (s *walkState) walkProxy(proxy *highbase.SchemaProxy, path string, depth int) error {
	if proxy == nil {
		return nil
	}
	if depth > s.cfg.MaxDepth {
		s.skipped = append(s.skipped, SkippedDescriptor{Path: path, Reason: "schema depth limit reached"})
		return nil
	}
	s.nodes++
	if s.nodes > s.cfg.MaxNodes {
		return fmt.Errorf("%w: max nodes %d", ErrSchemaBudgetExceeded, s.cfg.MaxNodes)
	}

	ref := ""
	if proxy.IsReference() {
		ref = proxy.GetReference()
		if s.refStack[ref] {
			s.skipped = append(s.skipped, SkippedDescriptor{Path: path, Reason: "cyclic internal schema reference"})
			return nil
		}
		s.refStack[ref] = true
		defer delete(s.refStack, ref)
	}

	schema, err := proxy.BuildSchema()
	if err != nil {
		return fmt.Errorf("build schema at %s: %w", path, err)
	}
	if schema == nil {
		return fmt.Errorf("build schema at %s: no schema returned", path)
	}

	if len(schema.OneOf) > 0 || len(schema.AnyOf) > 0 {
		s.skipped = append(s.skipped, SkippedDescriptor{Path: path, Reason: "oneOf/anyOf composition is ambiguous for v0.9 Slice 1"})
		return nil
	}

	// allOf branches constrain the same JSON location, so they are walked at the
	// same path. Duplicate properties are merged conservatively below.
	for _, branch := range schema.AllOf {
		if err := s.walkProxy(branch, path, depth); err != nil {
			return err
		}
	}

	if schema.Properties == nil {
		return nil
	}
	required := make(map[string]bool, len(schema.Required))
	for _, name := range schema.Required {
		required[name] = true
	}

	for name, child := range schema.Properties.FromOldest() {
		childPath := joinJSONPath(path, name)
		desc, childSchema, err := describeProperty(name, childPath, path, child, required[name])
		if err != nil {
			return err
		}
		s.mergeProperty(desc)

		if desc.Ambiguous {
			s.skipped = append(s.skipped, SkippedDescriptor{Path: childPath, Reason: "oneOf/anyOf property is not flattened"})
			continue
		}
		if desc.Array {
			s.skipped = append(s.skipped, SkippedDescriptor{Path: childPath, Reason: "array traversal is outside v0.9 Slice 1"})
			continue
		}
		if desc.Object && childSchema != nil {
			if err := s.walkProxy(child, childPath, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func describeProperty(name, path, parent string, proxy *highbase.SchemaProxy, required bool) (PropertyDescriptor, *highbase.Schema, error) {
	desc := PropertyDescriptor{Name: name, Path: path, Parent: parent, Required: required}
	if proxy == nil {
		return desc, nil, nil
	}
	if proxy.IsReference() {
		desc.SchemaRef = proxy.GetReference()
	}
	schema, err := proxy.BuildSchema()
	if err != nil {
		return PropertyDescriptor{}, nil, fmt.Errorf("build property schema at %s: %w", path, err)
	}
	if schema == nil {
		return PropertyDescriptor{}, nil, fmt.Errorf("build property schema at %s: no schema returned", path)
	}
	desc.DeclaredTypes = normalizedTypes(schema.Type)
	desc.ReadOnly = schema.ReadOnly != nil && *schema.ReadOnly
	desc.WriteOnly = schema.WriteOnly != nil && *schema.WriteOnly
	desc.Ambiguous = len(schema.OneOf) > 0 || len(schema.AnyOf) > 0
	desc.Array = containsType(desc.DeclaredTypes, "array") || schema.Items != nil
	desc.Object = containsType(desc.DeclaredTypes, "object") || hasProperties(schema) || len(schema.AllOf) > 0
	return desc, schema, nil
}

func (s *walkState) mergeProperty(next PropertyDescriptor) {
	current, ok := s.properties[next.Path]
	if !ok {
		s.properties[next.Path] = next
		return
	}
	current.DeclaredTypes = mergeTypes(current.DeclaredTypes, next.DeclaredTypes)
	current.Required = current.Required || next.Required
	current.ReadOnly = current.ReadOnly || next.ReadOnly
	current.WriteOnly = current.WriteOnly || next.WriteOnly
	current.Object = current.Object || next.Object
	current.Array = current.Array || next.Array
	current.Ambiguous = current.Ambiguous || next.Ambiguous
	if current.SchemaRef == "" {
		current.SchemaRef = next.SchemaRef
	}
	s.properties[next.Path] = current
}

func hasProperties(schema *highbase.Schema) bool {
	return schema != nil && schema.Properties != nil && schema.Properties.First() != nil
}

func normalizedTypes(types []string) []string {
	seen := make(map[string]struct{}, len(types))
	out := make([]string, 0, len(types))
	for _, value := range types {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func mergeTypes(a, b []string) []string {
	return normalizedTypes(append(append([]string(nil), a...), b...))
}

func containsType(types []string, want string) bool {
	for _, value := range types {
		if value == want {
			return true
		}
	}
	return false
}

func joinJSONPath(parent, name string) string {
	if parent == "" || parent == "$" {
		return "$." + name
	}
	return parent + "." + name
}

func parentJSONPath(path string) string {
	if path == "" || path == "$" {
		return ""
	}
	idx := strings.LastIndex(path, ".")
	if idx < 0 {
		return ""
	}
	if idx == 1 && strings.HasPrefix(path, "$") {
		return "$"
	}
	return path[:idx]
}

func sortedProperties(properties map[string]PropertyDescriptor) []PropertyDescriptor {
	out := make([]PropertyDescriptor, 0, len(properties))
	for _, property := range properties {
		out = append(out, property)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
