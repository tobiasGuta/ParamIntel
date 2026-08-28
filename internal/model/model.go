package model

import (
	"net/http"
	"strconv"
)

type RequestTemplate struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
}

type Snapshot struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	JSONPaths  map[string]string
	IsJSON     bool
}

type BaselineProfile struct {
	Samples         int
	StatusCode      int
	StatusStable    bool
	StableJSONPaths map[string]string
	SeenJSONPaths   map[string]struct{}
	StableBody      string
	BodyLenMin      int
	BodyLenMax      int
	IsJSON          bool
}

type Difference struct {
	Kind   string `json:"kind"`
	Path   string `json:"path,omitempty"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

type Comparison struct {
	Meaningful  bool         `json:"meaningful"`
	Differences []Difference `json:"differences,omitempty"`
}

// ConfidenceScore keeps confidence numeric in JSON while rendering with two
// decimal places so perfect scores are emitted as 1.00 instead of a bare 1.
type ConfidenceScore float64

func (c ConfidenceScore) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatFloat(float64(c), 'f', 2, 64)), nil
}

const (
	LocationQuery = "query"
	LocationForm  = "form"
	LocationJSON  = "json"
)

type CandidateSource struct {
	Source       string `json:"source"`
	Path         string `json:"path,omitempty"`
	ObservedType string `json:"observed_type,omitempty"`
	Priority     int    `json:"priority,omitempty"`
}

type Candidate struct {
	Name       string
	Location   string
	JSONParent string
	Sources    []CandidateSource
}

func (c Candidate) JSONPath() string {
	if c.Location != LocationJSON {
		return ""
	}
	if c.JSONParent == "" || c.JSONParent == "$" {
		return "$." + c.Name
	}
	return c.JSONParent + "." + c.Name
}

type ProbeValue struct {
	Kind string
	Raw  string
}

func StringValue(v string) ProbeValue { return ProbeValue{Kind: "string", Raw: v} }
func BoolValue(v bool) ProbeValue {
	if v {
		return ProbeValue{Kind: "boolean", Raw: "true"}
	}
	return ProbeValue{Kind: "boolean", Raw: "false"}
}
func IntegerValue(v int) ProbeValue { return ProbeValue{Kind: "integer", Raw: strconv.Itoa(v)} }
func NullValue() ProbeValue         { return ProbeValue{Kind: "null", Raw: "null"} }

type Mutation struct {
	Candidate Candidate
	Value     ProbeValue
}

type ValueObservation struct {
	Value          string       `json:"value"`
	ValueKind      string       `json:"value_kind"`
	Status         int          `json:"status"`
	Classification string       `json:"classification"`
	Evidence       []Difference `json:"evidence,omitempty"`
}

type ParameterResult struct {
	Name                 string             `json:"name"`
	Location             string             `json:"location"`
	JSONPath             string             `json:"json_path,omitempty"`
	CandidateSources     []CandidateSource  `json:"candidate_sources,omitempty"`
	DiscoveryMode        string             `json:"discovery_mode,omitempty"`
	DiscoveryValue       string             `json:"discovery_value,omitempty"`
	DiscoveryValueKind   string             `json:"discovery_value_kind,omitempty"`
	Confidence           ConfidenceScore    `json:"confidence"`
	ConfidenceLabel      string             `json:"confidence_label"`
	CandidateChanged     int                `json:"candidate_changed"`
	CandidateTrials      int                `json:"candidate_trials"`
	RandomControlChanged int                `json:"random_control_changed"`
	RandomControlTrials  int                `json:"random_control_trials"`
	Evidence             []Difference       `json:"evidence,omitempty"`
	InferredType         string             `json:"inferred_type,omitempty"`
	TypeConfidence       *ConfidenceScore   `json:"type_confidence,omitempty"`
	TypeEvidence         string             `json:"type_evidence,omitempty"`
	ValueProfile         []ValueObservation `json:"value_profile,omitempty"`
}

type ScanReport struct {
	Version    string            `json:"version"`
	Target     string            `json:"target"`
	Method     string            `json:"method"`
	Baseline   BaselineSummary   `json:"baseline"`
	Parameters []ParameterResult `json:"parameters"`
}

type BaselineSummary struct {
	Samples         int `json:"samples"`
	StableJSONPaths int `json:"stable_json_paths"`
	BodyLenMin      int `json:"body_len_min"`
	BodyLenMax      int `json:"body_len_max"`
}
