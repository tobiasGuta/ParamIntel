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

type ParameterResult struct {
	Name                 string          `json:"name"`
	Location             string          `json:"location"`
	Confidence           ConfidenceScore `json:"confidence"`
	ConfidenceLabel      string          `json:"confidence_label"`
	CandidateChanged     int             `json:"candidate_changed"`
	CandidateTrials      int             `json:"candidate_trials"`
	RandomControlChanged int             `json:"random_control_changed"`
	RandomControlTrials  int             `json:"random_control_trials"`
	Evidence             []Difference    `json:"evidence,omitempty"`
}

type ScanReport struct {
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
