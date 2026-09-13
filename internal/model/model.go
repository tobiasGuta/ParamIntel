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

// ResponseFeatures holds deterministic response observations that can later be
// promoted into baseline evidence when they prove stable across samples.
type ResponseFeatures struct {
	ContentType       string
	IsText            bool
	LineCount         int
	WordCount         int
	IsHTML            bool
	HTMLElementCount  int
	HTMLStructureHash string
}

type Snapshot struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	JSONPaths  map[string]string
	IsJSON     bool
	Features   ResponseFeatures
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

	ContentType       string
	ContentTypeStable bool

	TextMetricsAvailable bool
	LineCountMin         int
	LineCountMax         int
	WordCountMin         int
	WordCountMax         int

	HTMLAvailable        bool
	HTMLStructureStable  bool
	HTMLStructureHash    string
	HTMLElementCountMin  int
	HTMLElementCountMax  int

	// StableHeaderHashes stores only deterministic hashes of eligible response
	// header values. Raw header values are deliberately not copied into the
	// baseline evidence profile.
	StableHeaderHashes map[string]string
	SeenHeaderNames    map[string]struct{}
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
	Reason       string `json:"reason,omitempty"`
}

type Candidate struct {
	Name       string
	Location   string
	JSONParent string
	Sources    []CandidateSource

	// JSONScaffoldParent is set only for response-derived JSON candidates whose
	// immediate parent object is absent from the captured request but whose own
	// parent already exists as an object. Merely setting this field does not
	// authorize mutation; the discovery/mutation pipeline must explicitly opt in
	// before any missing object is created.
	JSONScaffoldParent string
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

func (c Candidate) RequiresJSONScaffold() bool {
	return c.Location == LocationJSON && c.JSONScaffoldParent != ""
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

	// AllowJSONScaffold is an explicit per-mutation authorization gate. A
	// scaffold-marked candidate must have this set before the mutator may create
	// its one missing object parent.
	AllowJSONScaffold bool
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

type AIAdvisorCandidateAudit struct {
	Name                 string           `json:"name"`
	Location             string           `json:"location"`
	JSONParent           string           `json:"json_parent,omitempty"`
	Priority             int              `json:"priority"`
	Reason               string           `json:"reason,omitempty"`
	Admission            string           `json:"admission"`
	RejectionReason      string           `json:"rejection_reason,omitempty"`
	Tested               bool             `json:"tested"`
	Verified             bool             `json:"verified"`
	DiscoveryOutcome     string           `json:"discovery_outcome"`
	Confidence           *ConfidenceScore `json:"confidence,omitempty"`
	CandidateChanged     *int             `json:"candidate_changed,omitempty"`
	CandidateTrials      *int             `json:"candidate_trials,omitempty"`
	RandomControlChanged *int             `json:"random_control_changed,omitempty"`
	RandomControlTrials  *int             `json:"random_control_trials,omitempty"`
}

type AIAdvisorSummary struct {
	Provider            string                    `json:"provider"`
	Model               string                    `json:"model"`
	InputPolicy         string                    `json:"input_policy"`
	ContextSource       string                    `json:"context_source"`
	SuggestedCandidates int                       `json:"suggested_candidates"`
	AcceptedCandidates  int                       `json:"accepted_candidates"`
	RejectedCandidates  int                       `json:"rejected_candidates"`
	TestedCandidates    int                       `json:"tested_candidates"`
	VerifiedCandidates  int                       `json:"verified_candidates"`
	CandidateAudit      []AIAdvisorCandidateAudit `json:"candidate_audit,omitempty"`
}

type ScanReport struct {
	Version    string            `json:"version"`
	Target     string            `json:"target"`
	Method     string            `json:"method"`
	Baseline   BaselineSummary   `json:"baseline"`
	AIAdvisor  *AIAdvisorSummary `json:"ai_advisor,omitempty"`
	Parameters []ParameterResult `json:"parameters"`
}

type BaselineSummary struct {
	Samples         int `json:"samples"`
	StableJSONPaths int `json:"stable_json_paths"`
	BodyLenMin      int `json:"body_len_min"`
	BodyLenMax      int `json:"body_len_max"`
}
