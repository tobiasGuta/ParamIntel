package decision

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

const ShadowCaptureSchemaVersion = 2

type ShadowCaptureRecord struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	Source        string `json:"source"`
	State         State  `json:"state"`
}

func StateFromParameterResult(result model.ParameterResult, remainingRequestBudget int) State {
	valueKind := strings.TrimSpace(result.DiscoveryValueKind)
	if valueKind == "" {
		valueKind = "string"
	}

	kindsSet := map[string]struct{}{}
	pathsSet := map[string]struct{}{}
	for _, evidence := range result.Evidence {
		if kind := strings.TrimSpace(evidence.Kind); kind != "" {
			kindsSet[kind] = struct{}{}
		}
		if path := strings.TrimSpace(evidence.Path); path != "" {
			pathsSet[path] = struct{}{}
		}
	}
	for _, source := range result.CandidateSources {
		if sourceName := strings.TrimSpace(source.Source); sourceName != "" {
			kindsSet["candidate_source:"+sourceName] = struct{}{}
		}
		if path := strings.TrimSpace(source.Path); path != "" {
			pathsSet[path] = struct{}{}
		}
	}

	kinds := make([]string, 0, len(kindsSet))
	for kind := range kindsSet {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)

	paths := make([]string, 0, len(pathsSet))
	for path := range pathsSet {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	return State{
		Candidate: CandidateState{
			Name:          result.Name,
			Location:      result.Location,
			DiscoveryMode: result.DiscoveryMode,
			ValueKind:     valueKind,
		},
		Verification: VerificationState{
			CandidateChanged: result.CandidateChanged,
			CandidateTrials:  result.CandidateTrials,
			ControlChanged:   result.RandomControlChanged,
			ControlTrials:    result.RandomControlTrials,
			Confidence:       float64(result.Confidence),
		},
		Evidence: EvidenceState{
			Kinds: kinds,
			Paths: paths,
		},
		RemainingRequestBudget: remainingRequestBudget,
	}
}

func NewShadowCaptureRecord(state State) (ShadowCaptureRecord, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return ShadowCaptureRecord{}, fmt.Errorf("marshal shadow decision state: %w", err)
	}
	sum := sha256.Sum256(payload)
	return ShadowCaptureRecord{
		SchemaVersion: ShadowCaptureSchemaVersion,
		ID:            hex.EncodeToString(sum[:12]),
		Source:        "paramintel_residual_pre_semantic_rescue",
		State:         state,
	}, nil
}

func AppendShadowCaptureJSONL(path string, state State) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return fmt.Errorf("shadow capture path is required")
	}
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create shadow capture directory: %w", err)
		}
	}
	record, err := NewShadowCaptureRecord(state)
	if err != nil {
		return err
	}
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal shadow capture record: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open shadow capture file: %w", err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	if _, err := w.Write(line); err != nil {
		return fmt.Errorf("write shadow capture record: %w", err)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush shadow capture record: %w", err)
	}
	return nil
}
