package schemaintel

const (
	SourceResponseOnlyJSONProperty = "openapi_response_only_json_property"

	PlacementExistingParent   = "existing_parent"
	PlacementOneLevelScaffold = "one_level_scaffold"
)

type Config struct {
	MaxDepth int
	MaxNodes int
}

func DefaultConfig() Config {
	return Config{MaxDepth: 8, MaxNodes: 512}
}

type OperationMatch struct {
	SpecPath string `json:"spec_path"`
	Method   string `json:"method"`
}

type PropertyDescriptor struct {
	Name          string   `json:"name"`
	Path          string   `json:"path"`
	Parent        string   `json:"parent"`
	DeclaredTypes []string `json:"declared_types,omitempty"`
	Required      bool     `json:"required,omitempty"`
	ReadOnly      bool     `json:"read_only,omitempty"`
	WriteOnly     bool     `json:"write_only,omitempty"`
	SchemaRef     string   `json:"schema_ref,omitempty"`
	Object        bool     `json:"object,omitempty"`
	Array         bool     `json:"array,omitempty"`
	Ambiguous     bool     `json:"ambiguous,omitempty"`
}

type CandidateDescriptor struct {
	Name               string   `json:"name"`
	Path               string   `json:"path"`
	Parent             string   `json:"parent"`
	DeclaredTypes      []string `json:"declared_types,omitempty"`
	Required           bool     `json:"required,omitempty"`
	ReadOnly           bool     `json:"read_only,omitempty"`
	WriteOnly          bool     `json:"write_only,omitempty"`
	SchemaRef          string   `json:"schema_ref,omitempty"`
	Source             string   `json:"source"`
	Reason             string   `json:"reason"`
	Placement          string   `json:"placement"`
	JSONScaffoldParent string   `json:"json_scaffold_parent,omitempty"`
}

type SkippedDescriptor struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type Report struct {
	OpenAPIVersion     string                `json:"openapi_version"`
	Operation          OperationMatch        `json:"operation"`
	RequestMediaType   string                `json:"request_media_type"`
	ResponseStatusKey  string                `json:"response_status_key"`
	ResponseMediaType  string                `json:"response_media_type"`
	RequestProperties  []PropertyDescriptor  `json:"request_properties,omitempty"`
	ResponseProperties []PropertyDescriptor  `json:"response_properties,omitempty"`
	Candidates         []CandidateDescriptor `json:"candidates,omitempty"`
	Skipped            []SkippedDescriptor   `json:"skipped,omitempty"`
}
