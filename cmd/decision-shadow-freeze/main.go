package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/decision"
)

const datasetSchemaVersion = 1

type dataset struct {
	SchemaVersion   int           `json:"schema_version"`
	Source          string        `json:"source"`
	CapturedRecords int           `json:"captured_records"`
	UniqueCases     int           `json:"unique_cases"`
	Cases           []datasetCase `json:"cases"`
}

type datasetCase struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	State          decision.State  `json:"state"`
	ExpectedAction decision.Action `json:"expected_action"`
	LabelNotes     string          `json:"label_notes,omitempty"`
}

func main() {
	var inputPath, outputPath string
	flag.StringVar(&inputPath, "input", ".\.paramintel\\decision-shadow.jsonl", "shadow capture JSONL input")
	flag.StringVar(&outputPath, "output", ".\.paramintel\\decision-shadow-dataset.json", "frozen labeling dataset output")
	flag.Parse()

	d, err := freezeDataset(inputPath)
	fatal(err)

	raw, err := json.MarshalIndent(d, "", "  ")
	fatal(err)
	raw = append(raw, '\n')
	fatal(os.WriteFile(outputPath, raw, 0600))

	fmt.Printf("captured records: %d\n", d.CapturedRecords)
	fmt.Printf("unique cases: %d\n", d.UniqueCases)
	fmt.Printf("wrote %s\n", outputPath)
	fmt.Println("labels intentionally left blank; fill expected_action before replay")
}

func freezeDataset(path string) (dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return dataset{}, err
	}
	defer f.Close()

	seen := map[string]decision.ShadowCaptureRecord{}
	captured := 0
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 4*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		captured++

		var record decision.ShadowCaptureRecord
		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&record); err != nil {
			return dataset{}, fmt.Errorf("record %d: decode: %w", captured, err)
		}
		if record.SchemaVersion != decision.ShadowCaptureSchemaVersion {
			return dataset{}, fmt.Errorf("record %d: unsupported shadow schema version %d", captured, record.SchemaVersion)
		}
		expected, err := decision.NewShadowCaptureRecord(record.State)
		if err != nil {
			return dataset{}, fmt.Errorf("record %d: recompute id: %w", captured, err)
		}
		if record.ID != expected.ID {
			return dataset{}, fmt.Errorf("record %d: id mismatch: got %q want %q", captured, record.ID, expected.ID)
		}
		if existing, ok := seen[record.ID]; ok {
			if !statesEqual(existing.State, record.State) {
				return dataset{}, fmt.Errorf("record %d: duplicate id %q has different state", captured, record.ID)
			}
			continue
		}
		seen[record.ID] = record
	}
	if err := scanner.Err(); err != nil {
		return dataset{}, err
	}
	if captured == 0 {
		return dataset{}, fmt.Errorf("shadow capture has no records")
	}

	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	cases := make([]datasetCase, 0, len(ids))
	for _, id := range ids {
		record := seen[id]
		cases = append(cases, datasetCase{
			ID:             id,
			Name:           "shadow-" + id,
			State:          record.State,
			ExpectedAction: "",
		})
	}

	return dataset{
		SchemaVersion:   datasetSchemaVersion,
		Source:          "paramintel_decision_shadow_frozen",
		CapturedRecords: captured,
		UniqueCases:     len(cases),
		Cases:           cases,
	}, nil
}

func statesEqual(a, b decision.State) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(left) == string(right)
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
