package main

import (
	"fmt"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

// validateExpectedBaselineStatus rejects impossible guard values before any
// request is sent. Zero keeps the historical, status-agnostic behavior.
func validateExpectedBaselineStatus(expected int) error {
	if expected != 0 && (expected < 100 || expected > 599) {
		return fmt.Errorf("-expect-status must be 0 (disabled) or an HTTP status from 100 to 599")
	}
	return nil
}

// checkExpectedBaselineStatus enforces the operator's intended application
// state before any candidate, control, AI, or semantic rescue probes run.
// A repeatable 401 is technically stable, but not a valid baseline if the
// operator explicitly expected an authenticated HTTP 200.
func checkExpectedBaselineStatus(expected int, profile model.BaselineProfile) error {
	if expected == 0 {
		return nil
	}
	if !profile.StatusStable {
		return fmt.Errorf("baseline status is unstable across %d samples; expected HTTP %d; refusing discovery", profile.Samples, expected)
	}
	if profile.StatusCode != expected {
		return fmt.Errorf("baseline status mismatch: expected HTTP %d, observed stable HTTP %d; refusing discovery before candidate probes", expected, profile.StatusCode)
	}
	return nil
}
